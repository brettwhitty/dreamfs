package main

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"sync"
	"syscall"
	"time"

	"github.com/adrg/xdg"
	"github.com/charmbracelet/lipgloss"
	"github.com/schollz/progressbar/v3"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/zeebo/blake3"
	bolt "go.etcd.io/bbolt"
)

// Define styled output using Lip Gloss.
var (
	successStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("2")).Bold(true)
	errorStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("1")).Bold(true)
	warnStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("3")).Bold(true)
	infoStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("4")).Bold(true)
	accentStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("5")).Bold(true)
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

		// Read head.
		head := make([]byte, sampleSize)
		if _, err := f.Read(head); err != nil {
			return "", fmt.Errorf("read head: %w", err)
		}
		data = append(data, head...)

		// Read middle.
		midOffset := info.Size() / 2
		if _, err := f.Seek(midOffset, io.SeekStart); err != nil {
			return "", fmt.Errorf("seek middle: %w", err)
		}
		mid := make([]byte, sampleSize)
		if _, err := io.ReadFull(f, mid); err != nil {
			return "", fmt.Errorf("read middle: %w", err)
		}
		data = append(data, mid...)

		// Read tail.
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

// ProcessFile indexes a single file: computes its fingerprint, collects metadata, and optionally stores it.
// If 'store' is false, it only computes and returns the fingerprint.
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
		}
		if err := ps.Put(meta); err != nil {
			return "", fmt.Errorf("failed to store metadata for %s: %w", filePath, err)
		}
	}
	return fingerprint, nil
}

// countFiles walks the path and counts all non-directory files.
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

// md5sumLike processes files sequentially and prints fingerprint and file name.
// This is the default behavior (like md5sum).
func md5sumLike(ctx context.Context, path string) {
	err := filepath.Walk(path, func(filePath string, info os.FileInfo, err error) error {
		if err != nil {
			fmt.Println(warnStyle.Render(fmt.Sprintf("walk error: %v", err)))
			return nil
		}
		if !info.IsDir() {
			fingerprint, err := ProcessFile(ctx, filePath, nil, false)
			if err != nil {
				fmt.Println(errorStyle.Render(fmt.Sprintf("Error processing %s: %v", filePath, err)))
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

// indexFilesSequential processes files sequentially (with progress bar) and stores them in DB.
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
			fmt.Println(warnStyle.Render(fmt.Sprintf("walk error: %v", err)))
			return nil
		}
		if !info.IsDir() {
			_, err := ProcessFile(ctx, filePath, ps, true)
			if err != nil {
				fmt.Println(errorStyle.Render(fmt.Sprintf("%v", err)))
			}
			_ = bar.Add(1)
		}
		return nil
	})
	if err != nil {
		log.Fatalf("error walking path %s: %v", path, err)
	}
	fmt.Println(infoStyle.Render("Indexing complete."))
}

// indexFilesConcurrent processes files concurrently using a worker pool and stores them in DB.
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
				fmt.Println(errorStyle.Render(fmt.Sprintf("%v", err)))
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
			fmt.Println(warnStyle.Render(fmt.Sprintf("walk error: %v", err)))
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
	fmt.Println(infoStyle.Render("Indexing complete."))
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
			fmt.Println(errorStyle.Render(fmt.Sprintf("failed to encode changes: %v", err)))
		}
	})

	fmt.Println(infoStyle.Render(fmt.Sprintf("Starting HTTP server on %s", addr)))
	if err := http.ListenAndServe(addr, nil); err != nil {
		log.Fatalf("HTTP server error: %v", err)
	}
}

// dumpDB dumps the contents of the persistent store in the specified format.
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

func initConfig(cfgFile string) {
	if cfgFile != "" {
		viper.SetConfigFile(cfgFile)
	} else {
		xdgConfigDir := xdg.ConfigHome
		viper.AddConfigPath(xdgConfigDir)
		viper.SetConfigName("indexer")
		viper.SetConfigType("json")
	}

	viper.AutomaticEnv()
	if err := viper.ReadInConfig(); err == nil {
		fmt.Println(accentStyle.Render(fmt.Sprintf("Using config file: %s", viper.ConfigFileUsed())))
	} else {
		fmt.Println(warnStyle.Render("No config file found; using defaults and flags"))
	}
}

func main() {
	var cfgFile string

	rootCmd := &cobra.Command{
		Use:   "indexer [path]",
		Short: "Index files and expose a replication source endpoint",
		// Default behavior (no subcommand): act like md5sum, printing fingerprint and file path.
		Run: func(cmd *cobra.Command, args []string) {
			if len(args) < 1 {
				cmd.Help()
				return
			}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			sigCh := make(chan os.Signal, 1)
			signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
			go func() {
				<-sigCh
				cancel()
			}()

			for _, path := range args {
				md5sumLike(ctx, path)
			}
		},
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
				fmt.Println(errorStyle.Render(fmt.Sprintf("failed to open persistent store: %v", err)))
				os.Exit(1)
			}
			defer ps.Close()

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			sigCh := make(chan os.Signal, 1)
			signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
			go func() {
				<-sigCh
				fmt.Println(warnStyle.Render("\nReceived shutdown signal, exiting gracefully..."))
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
				fmt.Println(errorStyle.Render(fmt.Sprintf("failed to open persistent store: %v", err)))
				os.Exit(1)
			}
			defer ps.Close()
			startHTTPServer(addr, ps)
		},
	}

	// "dump" command: dump the DB contents in the specified format.
	dumpCmd := &cobra.Command{
		Use:   "dump",
		Short: "Dump the persistent database contents",
		Run: func(cmd *cobra.Command, args []string) {
			dbPath := viper.GetString("dbpath")
			format := viper.GetString("format")
			ps, err := NewPersistentStore(dbPath)
			if err != nil {
				fmt.Println(errorStyle.Render(fmt.Sprintf("failed to open persistent store: %v", err)))
				os.Exit(1)
			}
			defer ps.Close()
			dumpDB(ps, format)
		},
	}
	dumpCmd.Flags().String("format", "json", "Dump format: json or tsv")
	viper.BindPFlag("format", dumpCmd.Flags().Lookup("format"))

	// Global persistent flags.
	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default: indexer.json in XDG config directory)")
	rootCmd.PersistentFlags().String("dbpath", "indexer.db", "Path to the BoltDB file")
	rootCmd.PersistentFlags().String("addr", ":8080", "Address to serve the replication endpoint")
	rootCmd.PersistentFlags().Int("workers", 4, "Number of concurrent workers for indexing (set 1 for sequential)")
	viper.BindPFlag("dbpath", rootCmd.PersistentFlags().Lookup("dbpath"))
	viper.BindPFlag("addr", rootCmd.PersistentFlags().Lookup("addr"))
	viper.BindPFlag("workers", rootCmd.PersistentFlags().Lookup("workers"))

	rootCmd.AddCommand(indexCmd)
	rootCmd.AddCommand(serveCmd)
	rootCmd.AddCommand(dumpCmd)

	// Peer discovery can be added later (e.g., using Hashicorp Memberlist).

	if err := rootCmd.Execute(); err != nil {
		log.Fatal(err)
	}
}
: