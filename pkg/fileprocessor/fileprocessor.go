package fileprocessor

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/charmbracelet/bubbles/progress"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/lipgloss"
	"github.com/karrick/godirwalk"
	"github.com/spf13/viper"
	"github.com/zeebo/blake3"

	"gnomatix/dreamfs/v2/pkg/metadata"
	"gnomatix/dreamfs/v2/pkg/network"
	"gnomatix/dreamfs/v2/pkg/storage"
	"gnomatix/dreamfs/v2/pkg/utils"
)

// ------------------------
// Filesystem Partition Caching for Canonicalization
// ------------------------

var (
	partitionsCache     []disk.PartitionStat
	partitionsCacheTime time.Time
	cacheMutex          sync.Mutex
	cacheDuration       = 5 * time.Minute
)

func GetPartitions() ([]disk.PartitionStat, error) {
	cacheMutex.Lock()
	defer cacheMutex.Unlock()
	if time.Since(partitionsCacheTime) < cacheDuration && partitionsCache != nil {
		return partitionsCache, nil
	}
	parts, err := disk.Partitions(true)
	if err != nil {
		return nil, err
	}
	partitionsCache = parts
	partitionsCacheTime = time.Now()
	return parts, nil
}

// ------------------------
// Canonicalize Paths for Physical Uniqueness
// ------------------------

func CanonicalizePath(absPath string) (string, error) {
	// Windows UNC paths.
	if runtime.GOOS == "windows" {
		if strings.HasPrefix(absPath, `\\`) {
			parts := strings.SplitN(absPath[2:], `\`, 3)
			if len(parts) >= 2 {
				server := parts[0]
				share := parts[1]
				rest := ""
				if len(parts) == 3 {
					rest = "/" + parts[2]
				}
			return fmt.Sprintf("%s:%s%s", server, share, rest), nil
			}
		}
		return absPath, nil
	}

	parts, err := GetPartitions()
	if err != nil {
		return absPath, err
	}
	var bestMatch disk.PartitionStat
	bestLen := 0
	for _, p := range parts {
		if strings.HasPrefix(absPath, p.Mountpoint) && len(p.Mountpoint) > bestLen {
			bestLen = len(p.Mountpoint)
			bestMatch = p
		}
	}
	if bestLen > 0 {
		networkFSTypes := map[string]bool{
			"nfs":   true,
			"nfs4":  true,
			"cifs":  true,
			"smbfs": true,
			"afp":   true,
		}
		if networkFSTypes[strings.ToLower(bestMatch.Fstype)] {
			relPath := absPath[len(bestMatch.Mountpoint):]
			if !strings.HasPrefix(relPath, "/") {
				relPath = "/" + relPath
			}
			return fmt.Sprintf("%s:%s", bestMatch.Device, relPath), nil
		}
	}
	return absPath, nil
}

// ------------------------
// Fingerprinting and File Processing
// ------------------------

const fileSampleSize = 1 << 20

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
	if info.Size() < 3*fileSampleSize {
		data, err = io.ReadAll(f)
		if err != nil {
			return "", fmt.Errorf("read file: %w", err)
		}
	} else {
		data = make([]byte, 0, 3*fileSampleSize)
		head := make([]byte, fileSampleSize)
		if _, err := f.Read(head); err != nil {
			return "", fmt.Errorf("read head: %w", err)
		}
		data = append(data, head...)

		midOffset := info.Size() / 2
		if _, err := f.Seek(midOffset, io.SeekStart); err != nil {
			return "", fmt.Errorf("seek middle: %w", err)
		}
		mid := make([]byte, fileSampleSize)
		if _, err := io.ReadFull(f, mid); err != nil {
			return "", fmt.Errorf("read middle: %w", err)
		}
		data = append(data, mid...)

		tailOffset := info.Size() - fileSampleSize
		if _, err := f.Seek(tailOffset, io.SeekStart); err != nil {
			return "", fmt.Errorf("seek tail: %w", err)
		}
		tail := make([]byte, fileSampleSize)
		if _, err := io.ReadFull(f, tail); err != nil {
			return "", fmt.Errorf("read tail: %w", err)
		}
		data = append(data, tail...)
	}

	hash := blake3.Sum256(data)
	return fmt.Sprintf("%x", hash), nil
}

// Global swarm delegate.
var swarmDelegate *network.SwarmDelegate

func ProcessFile(ctx context.Context, filePath string, ps *storage.PersistentStore, store bool) (string, error) {
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
	absPath, err := filepath.Abs(filePath)
	if err != nil {
		return "", fmt.Errorf("failed to get absolute path for %s: %w", filePath, err)
	}
	canonicalPath, err := CanonicalizePath(absPath)
	if err != nil {
		canonicalPath = absPath
	}
	fingerprint, err := FingerprintFile(filePath)
	if err != nil {
		return "", fmt.Errorf("failed to fingerprint %s: %w", filePath, err)
	}
	if store {
		bytes := info.Size()
		modTime := info.ModTime().Format(time.RFC3339)

		idString := utils.HostID + "|" + canonicalPath + "|" + modTime + "|" + strconv.FormatInt(bytes, 16) + "|" + fingerprint
		UUID := utils.GenerateUUID(idString)

		meta := metadata.FileMetadata{
			ID:       UUID,
			IDString: idString,
			HostID:   utils.HostID,
			FilePath: canonicalPath,
			Size:     bytes,
			ModTime:  modTime,
			BLAKE3:   fingerprint,
			Extra:    map[string]interface{}{},
		}
		if err := ps.Put(meta); err != nil {
			return "", fmt.Errorf("failed to store metadata for %s: %w", filePath, err)
		}
		if swarmDelegate != nil {
			data, err := json.Marshal(meta)
			if err == nil {
				swarmDelegate.broadcasts.QueueBroadcast(&network.FileMetaBroadcast{msg: data})
			}
		}
	}
	return fingerprint, nil
}

// ------------------------
// Directory Processing with godirwalk and Charm UI Feedback
// ------------------------

// ProcessAllDirectories scans the root directory and processes its files,
// then collects subdirectories and processes them one at a time. A spinner is
// shown while reading directories, and a progress bar is updated per subdirectory.
func ProcessAllDirectories(ctx context.Context, root string, ps *storage.PersistentStore) error {
	quiet := viper.GetBool("quiet")
	if !quiet {
		fmt.Println("Reading files...")
	}
	// Process files in the root directory.
	if !quiet {
		fmt.Printf("Processing root directory: %s\n", root)
	}
	err := godirwalk.Walk(root, &godirwalk.Options{
		Unsorted: true,
		Callback: func(path string, de *godirwalk.Dirent) error {
			select {
			case <-ctx.Done():
				return ctx.Err()
			default:
			}
			// Only process files directly in root.
			if de.IsDir() && path != root {
				return godirwalk.SkipNode
			}
			if !de.IsDir() {
				_, err := ProcessFile(ctx, path, ps, true)
				if err != nil && !quiet {
					fmt.Printf("Error processing %s: %v\n", path, err)
				}
			}
			return nil
		},
	})
	if err != nil {
		return err
	}

	// Collect all subdirectories.
	var subdirs []string
	err = godirwalk.Walk(root, &godirwalk.Options{
		Unsorted: true,
		Callback: func(path string, de *godirwalk.Dirent) error {
			select {
			case <-ctx.Done():
				return ctx.Err()
			default:
			}
			if de.IsDir() && path != root {
				subdirs = append(subdirs, path)
			}
			return nil
		},
	})
	if err != nil {
		return err
	}

	// Process each subdirectory.
	for i, dir := range subdirs {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		if !quiet {
			fmt.Printf("\nProcessing directory (%d/%d): %s\n", i+1, len(subdirs), dir)
		}
		// Collect files in the subdirectory.
		var filesInDir []string
		err = godirwalk.Walk(dir, &godirwalk.Options{
			Unsorted: true,
			Callback: func(path string, de *godirwalk.Dirent) error {
				select {
				case <-ctx.Done():
					return ctx.Err()
				default:
				}
				if !de.IsDir() {
					filesInDir = append(filesInDir, path)
				}
				return nil
			},
		})
		if err != nil {
			if !quiet {
				fmt.Printf("Error reading directory %s: %v\n", dir, err)
			}
			continue
		}
		totalFiles := len(filesInDir)
		if totalFiles == 0 {
			continue
		}
		// Initialize progress bar and spinner.
		var sp spinner.Model
		if !quiet {
			sp = spinner.New()
			sp.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("205")).Bold(true)
			sp.Start()
			fmt.Printf("Processing files in %s...\n", dir)
		}
		p := progress.New(progress.WithDefaultGradient())
		var processed int64
		for _, fpath := range filesInDir {
			select {
			case <-ctx.Done():
				return ctx.Err()
			default:
			}
			_, err := ProcessFile(ctx, fpath, ps, true)
			if err != nil && !quiet {
				fmt.Printf("Error processing %s: %v\n", fpath, err)
			}
			processed++
			if !quiet {
				percent := float64(processed) / float64(totalFiles)
				fmt.Printf("\r%s", lipgloss.NewStyle().Bold(true).Render(p.View(percent)))
			}
		}
		if !quiet {
			sp.Stop()
			fmt.Println()
		}
	}
	return nil
}
