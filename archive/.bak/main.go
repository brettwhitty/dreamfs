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
	"sync"
	"time"

	"github.com/spf13/cobra"
	"github.com/zeebo/blake3"
)

// FileMetadata holds the metadata for a file.
type FileMetadata struct {
	ID       string `json:"_id"` // Document ID, here the fingerprint.
	FilePath string `json:"filePath"`
	Size     int64  `json:"size"`
	ModTime  string `json:"modTime"`
}

// In-memory store for file metadata.
var metadataStore sync.Map

// FingerprintFile computes the BLAKE3 hash of a file.
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

// ProcessFile generates a fingerprint for a file, gathers metadata, and stores it.
func ProcessFile(ctx context.Context, filePath string) {
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

	metadataStore.Store(fingerprint, meta)
	log.Printf("Indexed: %s (%s)", filePath, fingerprint)
}

// indexFiles walks the provided path (file or directory) and processes files.
func indexFiles(path string) {
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
			// Process only files.
			if !info.IsDir() {
				ProcessFile(ctx, filePath)
			}
			return nil
		})
		if err != nil {
			log.Fatalf("error walking path %s: %v", path, err)
		}
	} else {
		ProcessFile(ctx, path)
	}

	log.Println("Indexing complete.")
}

// startHTTPServer starts an HTTP server exposing a simple replication endpoint.
func startHTTPServer(addr string) {
	// The /_changes endpoint returns all metadata as a JSON array.
	http.HandleFunc("/_changes", func(w http.ResponseWriter, r *http.Request) {
		changes := []FileMetadata{}
		metadataStore.Range(func(_, value interface{}) bool {
			if meta, ok := value.(FileMetadata); ok {
				changes = append(changes, meta)
			}
			return true
		})
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(changes); err != nil {
			log.Printf("failed to encode changes: %v", err)
		}
	})

	log.Printf("Starting HTTP server on %s", addr)
	if err := http.ListenAndServe(addr, nil); err != nil {
		log.Fatalf("HTTP server error: %v", err)
	}
}

func main() {
	var addr string

	// Root command.
	var rootCmd = &cobra.Command{
		Use:   "indexer",
		Short: "Index files and expose a replication source endpoint",
	}

	// "index" command: perform indexing.
	var indexCmd = &cobra.Command{
		Use:   "index [path]",
		Short: "Scan a file or directory and index files",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			path := args[0]
			indexFiles(path)
		},
	}

	// "serve" command: run in daemon mode exposing the replication API.
	var serveCmd = &cobra.Command{
		Use:   "serve",
		Short: "Run in daemon mode, exposing a replication source endpoint",
		Run: func(cmd *cobra.Command, args []string) {
			startHTTPServer(addr)
		},
	}
	serveCmd.Flags().StringVar(&addr, "addr", ":8080", "Address to serve the replication endpoint")

	rootCmd.AddCommand(indexCmd)
	rootCmd.AddCommand(serveCmd)

	if err := rootCmd.Execute(); err != nil {
		log.Fatal(err)
	}
}
