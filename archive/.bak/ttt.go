package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
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

// FingerprintFile computes the BLAKE3 hash for the file at the given path.
func FingerprintFile(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", fmt.Errorf("open file: %w", err)
	}
	defer f.Close()

	hasher := blake3.New()
	if _, err := io.Copy(hasher, f); err != nil {
		return "", fmt.Errorf("hashing file: %w", err)
	}
	return fmt.Sprintf("%x", hasher.Sum(nil)), nil
}

// ProcessFile indexes a single file: computes its fingerprint, gathers metadata, and stores it persistently.
func ProcessFile(ctx context.Context, filePath string, ps *PersistentStore) {
	info, err := os.Stat(filePath)
	if err != nil {
		color.Red("failed to stat %s: %v", filePath, err)
		return
	}
	// Skip directories.
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

// countFiles walks the path and returns the total number of files (non-directories).
func countFiles(path string) (int, error) {
	count := 0
	err := filepath.Walk(path, func(filePath string, info os.FileInfo, err error) error {
		if err != nil {
			// Log and skip errors.
			return nil
		}
		if !info.IsDir() {
			count++
		}
		return nil
	})
	return count, err
}

// indexFiles recursively scans the given path and processes all files,
// updating the progress bar as it goes.
func indexFiles(path string, ps *PersistentStore) {
	ctx := context.Background()

	info, err := os.Stat(path)
	if err != nil {
		log.Fatalf("failed to stat path: %v", err)
	}

	if info.IsDir() {
		// First, count total files.
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

		// Walk directory and process files.
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
	} else {
		// If a single file is provided, process without progress bar.
		ProcessFile(ctx, path, ps)
	}

	color.Cyan("Indexing complete.")
}

// startHTTPServer starts an HTTP server exposing a simple replication endpoint.
// The endpoint (/_changes) mimics CouchDB’s _changes feed by returning all metadata as JSON.
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
		// Use the OS-appropriate config directory via adrg/xdg.
		xdgConfigDir := xdg.ConfigHome
		viper.AddConfigPath(xdgConfigDir)
		viper.SetConfigName("indexer")
		viper.SetConfigType("json")
	}

	viper.AutomaticEnv() // Read in environment variables that match

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

	// "index" command: perform indexing.
	indexCmd := &cobra.Command{
		Use:   "index [path]",
		Short: "Scan a file or directory, generate fingerprints, and store metadata persistently",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			path := args[0]
			dbPath := viper.GetString("dbpath")
			ps, err := NewPersistentStore(dbPath)
			if err != nil {
				color.Red("failed to open persistent store: %v", err)
				os.Exit(1)
			}
			defer ps.Close()

			indexFiles(path, ps)
		},
	}

	// "serve" command: run in daemon mode exposing the replication API.
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

	// Bind flags to Viper.
	viper.BindPFlag("dbpath", rootCmd.PersistentFlags().Lookup("dbpath"))
	viper.BindPFlag("addr", rootCmd.PersistentFlags().Lookup("addr"))

	rootCmd.AddCommand(indexCmd)
	rootCmd.AddCommand(serveCmd)

	// Peer discovery can be added later using libraries like Hashicorp Memberlist.

	if err := rootCmd.Execute(); err != nil {
		log.Fatal(err)
	}
}
