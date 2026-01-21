package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"sync"
	"syscall"
	"time"

	"github.com/adrg/xdg"
	"github.com/fatih/color"
	"github.com/schollz/progressbar/v3"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/zeebo/blake3"
	bolt "go.etcd.io/bbolt"
)

// FileMetadata holds the essential metadata for a file.
type FileMetadata struct {
	ID       string `json:"_id"`    // Unique document ID (the fingerprint)
	FilePath string `json:"filePath"`
	Size     int64  `json:"size"`
	ModTime  string `json:"modTime"`
}

// PersistentStore wraps a BoltDB instance for persistent storage.
type PersistentStore struct {
	db *bolt.DB
}

const bucketName = "metadata"

// NewPersistentStore opens (or creates) a BoltDB database at the given path and ensures the bucket exists.
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

// Close closes the underlying BoltDB.
func (ps *PersistentStore) Close() error {
	return ps.db.Close()
}

// Put saves a FileMetadata document to the BoltDB bucket.
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

// GetAll retrieves all FileMetadata documents from BoltDB.
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

// sampleSize in bytes (default 1MB)
const sampleSize = 1 << 20

// FingerprintFile computes a BLAKE3 hash by sampling the file.
// If the file is smaller than 3*sampleSize, it reads the entire file.
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
		// For small files, read the entire content.
		data, err = io.ReadAll(f)
		if err != nil {
			return "", fmt.Errorf("read file: %w", err)
		}
	} else {
		// For large files, sample head, middle, and tail.
		data = make([]byte, 0, 3*sampleSize)

		// Head.
		head := make([]byte, sampleSize)
		if _, err := f.Read(head); err != nil {
			return "", fmt.Errorf("read head: %w", err)
		}
		data = append(data, head...)

		// Middle.
		midOffset := info.Size() / 2
		if _, err := f.Seek(midOffset, io.SeekStart); err != nil {
			return "", fmt.Errorf("seek middle: %w", err)
		}
		mid := make([]byte, sampleSize)
		if _, err := io.ReadFull(f, mid); err != nil {
			return "", fmt.Errorf("read middle: %w", err)
		}
		data = append(data, mid...)

		// Tail.
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

// ProcessFile indexes a single file: computes its fingerprint, collects metadata, and stores it.
func ProcessFile(ctx context.Context, filePath string, ps *PersistentStore) {
	// Check if context is canceled.
	select {
	case <-ctx.Done():
		return
	default:
	}

	info, err := os.Stat(filePath)
	if err != nil {
		color.Red("failed to stat %s: %v", filePath, err)
		return
	}
	if info.IsDir() {
		return
	}

	fingerprint, err := FingerprintFile(filePath)
	if err != nil {
		color.Red("failed to fingerprint %s: %v", filePath, err)
		return
	}

	meta := FileMetadata{
		ID:       fingerprint,
		FilePath: filePath,
		Size:     info.Size(),
		ModTime:  info.ModTime().Format(time.RFC3339),
	}

	if err := ps.Put(meta); err != nil {
		color.Red("failed to store metadata for %s: %v", filePath, err)
	} else {
		color.Green("Indexed: %s (%s)", filePath, fingerprint)
	}
}

// countFiles walks the path and counts all non-directory files.
func countFiles(path string) (int, error) {
	count := 0
	err := filepath.Walk(path, func(filePath string, info os.FileInfo, err error) error {
		if err != nil {
			// Skip errors.
			return nil
		}
		if !info.IsDir() {
			count++
		}
		return nil
	})
	return count, err
}

// indexFilesSequential processes files sequentially (no worker pool).
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
			ProcessFile(ctx, filePath, ps)
			_ = bar.Add(1)
		}
		return nil
	})
	if err != nil {
		log.Fatalf("error walking path %s: %v", path, err)
	}
	color.Cyan("Indexing complete.")
}

// indexFilesConcurrent processes files concurrently using a worker pool.
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
			// Respect cancellation.
			select {
			case <-ctx.Done():
				return
			default:
			}
			ProcessFile(ctx, filePath, ps)
			_ = bar.Add(1)
		}
	}

	// Start worker pool.
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

// startHTTPServer starts an HTTP server exposing a replication endpoint (/ _changes)
// that mimics CouchDB’s _changes feed by returning all metadata as JSON.
func startHTTPServer(addr string, ps *PersistentStore) {
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

	color.Blue("Starting HTTP server on %s", addr)
	if err := http.ListenAndServe(addr, nil); err != nil {
		log.Fatalf("HTTP server error: %v", err)
	}
}

func initConfig(cfgFile string) {
	if cfgFile != "" {
		viper.SetConfigFile(cfgFile)
	} else {
		// Use OS-appropriate config directory via adrg/xdg.
		xdgConfigDir := xdg.ConfigHome
		viper.AddConfigPath(xdgConfigDir)
		viper.SetConfigName("indexer")
		viper.SetConfigType("json")
	}

	viper.AutomaticEnv() // read environment variables
	if err := viper.ReadInConfig(); err == nil {
		color.Magenta("Using config file: %s", viper.ConfigFileUsed())
	} else {
		color.Yellow("No config file found; using defaults and flags")
	}
}

func main() {
	var cfgFile string

	rootCmd := &cobra.Command{
		Use:   "indexer",
		Short: "Index files and expose a replication source endpoint",
	}

	cobra.OnInitialize(func() {
		initConfig(cfgFile)
	})

	// "index" command: scan and index files.
	indexCmd := &cobra.Command{
		Use:   "index [path]",
		Short: "Scan a file or directory, generate fingerprints, and store metadata persistently",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			path := args[0]
			dbPath := viper.GetString("dbpath")
			workers := viper.GetInt("workers")
			ps, err := NewPersistentStore(dbPath)
			if err != nil {
				color.Red("failed to open persistent store: %v", err)
				os.Exit(1)
			}
			defer ps.Close()

			// Create a cancellable context.
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			// Set up graceful shutdown.
			sigCh := make(chan os.Signal, 1)
			signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
			go func() {
				sig := <-sigCh
				color.Yellow("\nReceived signal: %v. Shutting down gracefully...", sig)
				cancel()
			}()

			if workers <= 1 {
				indexFilesSequential(ctx, path, ps)
			} else {
				indexFilesConcurrent(ctx, path, ps, workers)
			}
		},
	}

	// "serve" command: run daemon mode exposing the replication endpoint.
	serveCmd := &cobra.Command{
		Use:   "serve",
		Short: "Run in daemon mode, exposing a replication source endpoint (_changes)",
		Run: func(cmd *cobra.Command, args []string) {
			dbPath := viper.GetString("dbpath")
			addr := viper.GetString("addr")
			ps, err := NewPersistentStore(dbPath)
			if err != nil {
				color.Red("failed to open persistent store: %v", err)
				os.Exit(1)
			}
			defer ps.Close()
			startHTTPServer(addr, ps)
		},
	}

	// Global persistent flags.
	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default: indexer.json in XDG config directory)")
	rootCmd.PersistentFlags().String("dbpath", "indexer.db", "Path to the BoltDB file")
	rootCmd.PersistentFlags().String("addr", ":8080", "Address to serve the replication endpoint")
	rootCmd.PersistentFlags().Int("workers", 4, "Number of concurrent workers for indexing (set 1 for sequential)")

	// Bind flags to Viper.
	viper.BindPFlag("dbpath", rootCmd.PersistentFlags().Lookup("dbpath"))
	viper.BindPFlag("addr", rootCmd.PersistentFlags().Lookup("addr"))
	viper.BindPFlag("workers", rootCmd.PersistentFlags().Lookup("workers"))

	rootCmd.AddCommand(indexCmd)
	rootCmd.AddCommand(serveCmd)

	// Peer discovery can be added later (e.g. using Hashicorp Memberlist).

	if err := rootCmd.Execute(); err != nil {
		log.Fatal(err)
	}
}
