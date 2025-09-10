package main

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"sync"
	"syscall"
	"time"

	"github.com/adrg/xdg"
	"github.com/fatih/color"
	"github.com/hashicorp/mdns"
	"github.com/hashicorp/memberlist"
	"github.com/schollz/progressbar/v3"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/zeebo/blake3"
	bolt "go.etcd.io/bbolt"
)

// ------------------------
// Global peer list for HTTP-based discovery
// ------------------------

var (
	peerList      []string
	peerListMutex sync.Mutex
)

// handlePeerList is the HTTP handler for the /peerlist endpoint.
// It adds the remote client's address (using the swarmPort)
// to the peer list (if not already present) and returns the list as JSON.
func handlePeerList(w http.ResponseWriter, r *http.Request) {
	// Extract the remote IP address.
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	peerAddr := fmt.Sprintf("%s:%d", host, viper.GetInt("swarmPort"))

	peerListMutex.Lock()
	defer peerListMutex.Unlock()
	// Check if the peer is already in the list.
	found := false
	for _, p := range peerList {
		if p == peerAddr {
			found = true
			break
		}
	}
	if !found {
		peerList = append(peerList, peerAddr)
		log.Printf("Added new peer from HTTP: %s", peerAddr)
	}
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(peerList); err != nil {
		http.Error(w, "failed to encode peer list", http.StatusInternalServerError)
	}
}

// ------------------------
// Dynamic FileMetadata with extensible JSON support
// ------------------------

type FileMetadata struct {
	ID       string                 `json:"_id"`
	FilePath string                 `json:"filePath"`
	Size     int64                  `json:"size"`
	ModTime  string                 `json:"modTime"`
	Extra    map[string]interface{} `json:"-"`
}

func (fm *FileMetadata) UnmarshalJSON(data []byte) error {
	var tmp map[string]interface{}
	if err := json.Unmarshal(data, &tmp); err != nil {
		return err
	}
	if id, ok := tmp["_id"].(string); ok {
		fm.ID = id
	}
	if fp, ok := tmp["filePath"].(string); ok {
		fm.FilePath = fp
	}
	if size, ok := tmp["size"].(float64); ok {
		fm.Size = int64(size)
	}
	if mt, ok := tmp["modTime"].(string); ok {
		fm.ModTime = mt
	}
	delete(tmp, "_id")
	delete(tmp, "filePath")
	delete(tmp, "size")
	delete(tmp, "modTime")
	fm.Extra = tmp
	return nil
}

func (fm *FileMetadata) MarshalJSON() ([]byte, error) {
	m := map[string]interface{}{
		"_id":      fm.ID,
		"filePath": fm.FilePath,
		"size":     fm.Size,
		"modTime":  fm.ModTime,
	}
	for k, v := range fm.Extra {
		if _, exists := m[k]; !exists {
			m[k] = v
		}
	}
	return json.Marshal(m)
}

// ------------------------
// Persistent Storage (BoltDB)
// ------------------------

type PersistentStore struct {
	db *bolt.DB
}

const bucketName = "metadata"

func NewPersistentStore(dbPath string) (*PersistentStore, error) {
	db, err := bolt.Open(dbPath, 0600, &bolt.Options{Timeout: 1 * time.Second})
	if err != nil {
		return nil, fmt.Errorf("open bolt db: %w", err)
	}
	err = db.Update(func(tx *bolt.Tx) error {
		_, err := tx.CreateBucketIfNotExists([]byte(bucketName))
		return err
	})
	if err != nil {
		return nil, fmt.Errorf("create bucket: %w", err)
	}
	return &PersistentStore{db: db}, nil
}

func (ps *PersistentStore) Close() error {
	return ps.db.Close()
}

func (ps *PersistentStore) Put(meta FileMetadata) error {
	data, err := json.Marshal(meta)
	if err != nil {
		return fmt.Errorf("marshal metadata: %w", err)
	}
	return ps.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(bucketName))
		return b.Put([]byte(meta.ID), data)
	})
}

func (ps *PersistentStore) GetAll() ([]FileMetadata, error) {
	var results []FileMetadata
	err := ps.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(bucketName))
		return b.ForEach(func(k, v []byte) error {
			var meta FileMetadata
			if err := json.Unmarshal(v, &meta); err != nil {
				return err
			}
			results = append(results, meta)
			return nil
		})
	})
	return results, err
}

// ------------------------
// Fingerprinting and File Processing
// ------------------------

const sampleSize = 1 << 20

func FingerprintFile(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", fmt.Errorf("open file: %w", err)
	}
	defer f.Close()

	info, err := f.Stat()
	if err != nil {
		return "", fmt.Errorf("stat file: %w", err)
	}

	var data []byte
	if info.Size() < 3*sampleSize {
		data, err = io.ReadAll(f)
		if err != nil {
			return "", fmt.Errorf("read file: %w", err)
		}
	} else {
		data = make([]byte, 0, 3*sampleSize)
		head := make([]byte, sampleSize)
		if _, err := f.Read(head); err != nil {
			return "", fmt.Errorf("read head: %w", err)
		}
		data = append(data, head...)

		midOffset := info.Size() / 2
		if _, err := f.Seek(midOffset, io.SeekStart); err != nil {
			return "", fmt.Errorf("seek middle: %w", err)
		}
		mid := make([]byte, sampleSize)
		if _, err := io.ReadFull(f, mid); err != nil {
			return "", fmt.Errorf("read middle: %w", err)
		}
		data = append(data, mid...)

		tailOffset := info.Size() - sampleSize
		if _, err := f.Seek(tailOffset, io.SeekStart); err != nil {
			return "", fmt.Errorf("seek tail: %w", err)
		}
		tail := make([]byte, sampleSize)
		if _, err := io.ReadFull(f, tail); err != nil {
			return "", fmt.Errorf("read tail: %w", err)
		}
		data = append(data, tail...)
	}

	hash := blake3.Sum256(data)
	return fmt.Sprintf("%x", hash), nil
}

// Global variable for swarm delegate (if swarm mode is enabled)
var swarmDelegate *SwarmDelegate

// ProcessFile computes the fingerprint, stores metadata, and broadcasts it if needed.
func ProcessFile(ctx context.Context, filePath string, ps *PersistentStore, store bool) (string, error) {
	select {
	case <-ctx.Done():
		return "", ctx.Err()
	default:
	}
	info, err := os.Stat(filePath)
	if err != nil {
		return "", fmt.Errorf("failed to stat %s: %w", filePath, err)
	}
	if info.IsDir() {
		return "", nil
	}
	fingerprint, err := FingerprintFile(filePath)
	if err != nil {
		return "", fmt.Errorf("failed to fingerprint %s: %w", filePath, err)
	}
	if store {
		meta := FileMetadata{
			ID:       fingerprint,
			FilePath: filePath,
			Size:     info.Size(),
			ModTime:  info.ModTime().Format(time.RFC3339),
			Extra:    map[string]interface{}{},
		}
		if err := ps.Put(meta); err != nil {
			return "", fmt.Errorf("failed to store metadata for %s: %w", filePath, err)
		}
		// Broadcast metadata to peers if swarm mode is enabled.
		if swarmDelegate != nil {
			data, err := json.Marshal(meta)
			if err == nil {
				swarmDelegate.broadcasts.QueueBroadcast(&fileMetaBroadcast{msg: data})
			}
		}
	}
	return fingerprint, nil
}

// ------------------------
// File Indexing Routines
// ------------------------

func countFiles(path string) (int, error) {
	count := 0
	err := filepath.Walk(path, func(filePath string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if !info.IsDir() {
			count++
		}
		return nil
	})
	return count, err
}

func md5sumLike(ctx context.Context, path string) {
	err := filepath.Walk(path, func(filePath string, info os.FileInfo, err error) error {
		if err != nil {
			color.Yellow("walk error: %v", err)
			return nil
		}
		if !info.IsDir() {
			fingerprint, err := ProcessFile(ctx, filePath, nil, false)
			if err != nil {
				color.Red("Error processing %s: %v", filePath, err)
			} else {
				fmt.Printf("%s  %s\n", fingerprint, filePath)
			}
		}
		return nil
	})
	if err != nil {
		log.Fatalf("error walking path %s: %v", path, err)
	}
}

func indexFilesSequential(ctx context.Context, path string, ps *PersistentStore) {
	total, err := countFiles(path)
	if err != nil {
		log.Fatalf("failed to count files: %v", err)
	}
	bar := progressbar.NewOptions(total,
		progressbar.OptionSetDescription("Indexing files"),
		progressbar.OptionSetWidth(40),
		progressbar.OptionShowCount(),
		progressbar.OptionSetTheme(progressbar.Theme{
			Saucer:        "=",
			SaucerHead:    ">",
			SaucerPadding: " ",
			BarStart:      "[",
			BarEnd:        "]",
		}),
	)
	err = filepath.Walk(path, func(filePath string, info os.FileInfo, err error) error {
		if err != nil {
			color.Yellow("walk error: %v", err)
			return nil
		}
		if !info.IsDir() {
			_, err := ProcessFile(ctx, filePath, ps, true)
			if err != nil {
				color.Red("%v", err)
			}
			_ = bar.Add(1)
		}
		return nil
	})
	if err != nil {
		log.Fatalf("error walking path %s: %v", path, err)
	}
	color.Cyan("Indexing complete.")
}

func indexFilesConcurrent(ctx context.Context, path string, ps *PersistentStore, workers int) {
	total, err := countFiles(path)
	if err != nil {
		log.Fatalf("failed to count files: %v", err)
	}
	bar := progressbar.NewOptions(total,
		progressbar.OptionSetDescription("Indexing files"),
		progressbar.OptionSetWidth(40),
		progressbar.OptionShowCount(),
		progressbar.OptionSetTheme(progressbar.Theme{
			Saucer:        "=",
			SaucerHead:    ">",
			SaucerPadding: " ",
			BarStart:      "[",
			BarEnd:        "]",
		}),
	)
	fileCh := make(chan string, workers)
	var wg sync.WaitGroup
	workerFunc := func() {
		defer wg.Done()
		for filePath := range fileCh {
			select {
			case <-ctx.Done():
				return
			default:
			}
			_, err := ProcessFile(ctx, filePath, ps, true)
			if err != nil {
				color.Red("%v", err)
			}
			_ = bar.Add(1)
		}
	}
	wg.Add(workers)
	for i := 0; i < workers; i++ {
		go workerFunc()
	}
	err = filepath.Walk(path, func(filePath string, info os.FileInfo, err error) error {
		if err != nil {
			color.Yellow("walk error: %v", err)
			return nil
		}
		if !info.IsDir() {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case fileCh <- filePath:
			}
		}
		return nil
	})
	if err != nil {
		log.Fatalf("error walking path %s: %v", path, err)
	}
	close(fileCh)
	wg.Wait()
	color.Cyan("Indexing complete.")
}

// ------------------------
// HTTP Server: replication endpoint and peer list endpoint
// ------------------------

func startHTTPServer(addr string, ps *PersistentStore) {
	// Replication endpoint.
	http.HandleFunc("/_changes", func(w http.ResponseWriter, r *http.Request) {
		metas, err := ps.GetAll()
		if err != nil {
			http.Error(w, "failed to get metadata", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(metas); err != nil {
			color.Red("failed to encode changes: %v", err)
		}
	})
	// Peer list endpoint.
	http.HandleFunc("/peerlist", handlePeerList)

	color.Blue("Starting HTTP server on %s", addr)
	if err := http.ListenAndServe(addr, nil); err != nil {
		log.Fatalf("HTTP server error: %v", err)
	}
}

// ------------------------
// Database Dump
// ------------------------

func dumpDB(ps *PersistentStore, format string) {
	metas, err := ps.GetAll()
	if err != nil {
		log.Fatalf("failed to get metadata: %v", err)
	}
	switch format {
	case "json":
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(metas); err != nil {
			log.Fatalf("failed to encode JSON: %v", err)
		}
	case "tsv":
		w := csv.NewWriter(os.Stdout)
		w.Comma = '\t'
		w.Write([]string{"_id", "filePath", "size", "modTime"})
		for _, meta := range metas {
			w.Write([]string{meta.ID, meta.FilePath, strconv.FormatInt(meta.Size, 10), meta.ModTime})
		}
		w.Flush()
	default:
		log.Fatalf("unknown dump format: %s", format)
	}
}

// ------------------------
// Swarm Replication using memberlist with mDNS or HTTP-based peer discovery
// ------------------------

type fileMetaBroadcast struct {
	msg []byte
}

func (f *fileMetaBroadcast) Message() []byte { return f.msg }
func (f *fileMetaBroadcast) Finished()       {}

type SwarmDelegate struct {
	ps         *PersistentStore
	broadcasts *memberlist.TransmitLimitedQueue
}

func NewSwarmDelegate(ps *PersistentStore, ml *memberlist.Memberlist) *SwarmDelegate {
	d := &SwarmDelegate{ps: ps}
	d.broadcasts = &memb
