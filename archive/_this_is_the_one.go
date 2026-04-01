package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/adrg/xdg"
	"github.com/charmbracelet/bubbles/progress"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/lipgloss"
	"github.com/fatih/color"
	"github.com/hashicorp/memberlist"
	"github.com/karrick/godirwalk"
	"github.com/shirou/gopsutil/disk"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/zeebo/blake3"
	bolt "go.etcd.io/bbolt"
)

//
// CONFIGURABLE DEFAULTS
//
const (
	defaultSwarmPort  = 7946
	defaultWorkers    = 1
	defaultQuiet      = false
	defaultStealth    = false
	defaultPeerListURL = ""
	defaultSyncInterval = 1 * time.Second
	defaultBatchSize  = 100
)

func DefaultBoltDBPath() string {
	return filepath.Join(xdg.DataHome, "indexer", "indexer.db")
}

//
// CACHE WRITER (In-Memory Caching to Batch Writes)
//
type CacheWriter struct {
	ps            *PersistentStore
	ch            chan FileMetadata
	batchSize     int
	flushInterval time.Duration
	flushNowCh    chan struct{}
	quit          chan struct{}
	wg            sync.WaitGroup
}

func NewCacheWriter(ps *PersistentStore, batchSize int, flushInterval time.Duration) *CacheWriter {
	cw := &CacheWriter{
		ps:            ps,
		ch:            make(chan FileMetadata, batchSize*2),
		batchSize:     batchSize,
		flushInterval: flushInterval,
		flushNowCh:    make(chan struct{}),
		quit:          make(chan struct{}),
	}
	cw.wg.Add(1)
	go cw.run()
	return cw
}

func (cw *CacheWriter) run() {
	var batch []FileMetadata
	timer := time.NewTimer(cw.flushInterval)
	for {
		select {
		case meta := <-cw.ch:
			batch = append(batch, meta)
			if len(batch) >= cw.batchSize {
				cw.flush(batch)
				batch = nil
				if !timer.Stop() {
					<-timer.C
				}
				timer.Reset(cw.flushInterval)
			}
		case <-timer.C:
			if len(batch) > 0 {
				cw.flush(batch)
				batch = nil
			}
			timer.Reset(cw.flushInterval)
		case <-cw.flushNowCh:
			if len(batch) > 0 {
				cw.flush(batch)
				batch = nil
			}
		case <-cw.quit:
			if len(batch) > 0 {
				cw.flush(batch)
			}
			return
		}
	}
}

func (cw *CacheWriter) flush(batch []FileMetadata) {
	err := cw.ps.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte("metadata"))
		for _, meta := range batch {
			data, err := json.Marshal(meta)
			if err != nil {
				return err
			}
			if err := b.Put([]byte(meta.ID), data); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		log.Printf("CacheWriter flush error: %v", err)
	}
}

func (cw *CacheWriter) Write(meta FileMetadata) {
	cw.ch <- meta
}

func (cw *CacheWriter) FlushNow() {
	cw.flushNowCh <- struct{}{}
}

func (cw *CacheWriter) Close() {
	close(cw.quit)
	cw.wg.Wait()
}

//
// HTTP SERVER (Only Runs in Serve Mode)
//
func startHTTPServer(addr string, ps *PersistentStore, cw *CacheWriter) {
	http.HandleFunc("/_sync", func(w http.ResponseWriter, r *http.Request) {
		if cw != nil {
			cw.FlushNow()
		}
		metas, err := ps.GetAll()
		if err != nil {
			http.Error(w, "failed to get metadata", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(metas)
	})

	color.Blue("Starting HTTP server on %s", addr)
	if err := http.ListenAndServe(addr, nil); err != nil {
		log.Fatalf("HTTP server error: %v", err)
	}
}

//
// MAIN FUNCTION
//
func main() {
	var cfgFile string

	rootCmd := &cobra.Command{
		Use:   "indexer [path]",
		Short: "Index files and expose a replication source endpoint",
	}

	cobra.OnInitialize(func() {
		viper.SetDefault("dbpath", DefaultBoltDBPath())
		viper.SetDefault("sync-interval", defaultSyncInterval)
		viper.SetDefault("batch-size", defaultBatchSize)
	})

	indexCmd := &cobra.Command{
		Use:   "index [directory]",
		Short: "Scan a directory and index files",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			dir := args[0]
			dbPath := viper.GetString("dbpath")

			ps, err := NewPersistentStore(dbPath)
			if err != nil {
				log.Fatalf("failed to open persistent store: %v", err)
			}
			defer ps.Close()

			syncInterval := viper.GetDuration("sync-interval")
			batchSize := viper.GetInt("batch-size")

			cw := NewCacheWriter(ps, batchSize, syncInterval)
			defer cw.Close()

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			sigCh := make(chan os.Signal, 1)
			signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
			go func() {
				<-sigCh
				log.Println("Shutting down gracefully...")
				cancel()
			}()

			err = processAllDirectories(ctx, dir, ps, cw)
			if err != nil {
				log.Fatalf("Error indexing directory: %v", err)
			}
		},
	}

	serveCmd := &cobra.Command{
		Use:   "serve",
		Short: "Run in daemon mode, exposing replication (_sync)",
		Run: func(cmd *cobra.Command, args []string) {
			dbPath := viper.GetString("dbpath")

			ps, err := NewPersistentStore(dbPath)
			if err != nil {
				log.Fatalf("failed to open persistent store: %v", err)
			}
			defer ps.Close()

			syncInterval := viper.GetDuration("sync-interval")
			batchSize := viper.GetInt("batch-size")

			cw := NewCacheWriter(ps, batchSize, syncInterval)
			defer cw.Close()

			addr := viper.GetString("addr")
			startHTTPServer(addr, ps, cw)
		},
	}

	rootCmd.AddCommand(indexCmd, serveCmd)

	if err := rootCmd.Execute(); err != nil {
		log.Fatal(err)
	}
}
