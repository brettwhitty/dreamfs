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

	"github.com/spf13/cobra"
	"github.com/zeebo/blake3"
	bolt "go.etcd.io/bbolt"
)

// FileMetadata holds the essential metadata for a file.
type FileMetadata struct {
	ID       string `json:"_id"`    // Unique document ID (the fingerprint)
	FilePath string `json:"filePath"`
	Size     int64  `json:"size"`
	ModTime  string `json:"modTime"`
	// Future extensions: additional metadata, revision history, etc.
}

// PersistentStore wraps a BoltDB instance for persistent storage.
type PersistentStore struct {
	db *bolt.DB
}

const bucketName = "metadata"

// NewPersistentStore opens (or creates) a BoltDB database at the given path and ensures the metadata bucket exists.
func NewPersistentStore(dbPath string) (*PersistentStore, error) {
	db, err := bolt.Open(dbPath, 0600, &bolt.Options{Timeout: 1 * time.Second})
	if err != nil {
		return nil, fmt.Errorf("open bolt db: %w", err)
	}
	// Ensure the bucket exists.
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

// FingerprintFile computes the BLAKE3 hash for a file at the given path.
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
		log.Printf("failed to stat %s: %v", filePath, err)
		return
	}
	// Skip directories.
	if info.IsDir() {
		return
	}

	fingerprint, err := FingerprintFile(filePath)
	if err != nil {
		log.Printf("failed to fingerprint %s: %v", filePath, err)
		return
	}

	meta := FileMetadata{
		ID:       fingerprint,
		FilePath: filePath,
		Size:     info.Size(),
		ModTime:  info.ModTime().Format(time.RFC3339),
	}

	if err := ps.Put(meta); err != nil {
		log.Printf("failed to store metadata for %s: %v", filePath, err)
	} else {
		log.Printf("Indexed: %s (%s)", filePath, fingerprint)
	}
}

// indexFiles recursively scans the given path and processes all files.
func indexFiles(path string, ps *PersistentStore) {
	ctx := context.Background()

	info, err := os.Stat(path)
	if err != nil {
		log.Fatalf("failed to stat path: %v", err)
	}

	if info.IsDir() {
		err = filepath.Walk(path, func(filePath string, info os.FileInfo, err error) error {
			if err != nil {
				log.Printf("walk error: %v", err)
				return nil
			}
			if !info.IsDir() {
				ProcessFile(ctx, filePath, ps)
			}
			return nil
		})
		if err != nil {
			log.Fatalf("error walking path %s: %v", path, err)
		}
	} else {
		ProcessFile(ctx, path, ps)
	}

	log.Println("Indexing complete.")
}

// startHTTPServer starts a daemon HTTP server exposing a replication endpoint.
// The endpoint mimics CouchDB's _changes feed: returning all metadata as JSON.
func startHTTPServer(addr string, ps *PersistentStore) {
	http.HandleFunc("/_changes", func(w http.ResponseWriter, r *http.Request) {
		metas, err := ps.GetAll()
		if err != nil {
			http.Error(w, "failed to get metadata", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(metas); err != nil {
			log.Printf("failed to encode changes: %v", err)
		}
	})

	log.Printf("Starting HTTP server on %s", addr)
	if err := http.ListenAndServe(addr, nil); err != nil {
		log.Fatalf("HTTP server error: %v", err)
	}
}

func main() {
	var addr, dbPath string

	rootCmd := &cobra.Command{
		Use:   "indexer",
		Short: "Index files and expose a replication source endpoint",
	}

	// Command: index
	// Scans a file or directory and indexes files.
	indexCmd := &cobra.Command{
		Use:   "index [path]",
		Short: "Scan a file or directory, generate fingerprints, and store metadata persistently",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			path := args[0]
			ps, err := NewPersistentStore(dbPath)
			if err != nil {
				log.Fatalf("failed to open persistent store: %v", err)
			}
			defer ps.Close()

			indexFiles(path, ps)
		},
	}

	// Command: serve
	// Runs the tool in daemon mode, exposing a CouchDB replication source endpoint.
	serveCmd := &cobra.Command{
		Use:   "serve",
		Short: "Run in daemon mode, exposing a replication source endpoint (_changes)",
		Run: func(cmd *cobra.Command, args []string) {
			ps, err := NewPersistentStore(dbPath)
			if err != nil {
				log.Fatalf("failed to open persistent store: %v", err)
			}
			defer ps.Close()

			startHTTPServer(addr, ps)
		},
	}
	serveCmd.Flags().StringVar(&addr, "addr", ":8080", "Address to serve the replication endpoint")
	rootCmd.PersistentFlags().StringVar(&dbPath, "dbpath", "indexer.db", "Path to the BoltDB file")

	rootCmd.AddCommand(indexCmd)
	rootCmd.AddCommand(serveCmd)

	if err := rootCmd.Execute(); err != nil {
		log.Fatal(err)
	}
}
