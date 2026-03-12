package fileprocessor

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/karrick/godirwalk"
	"github.com/shirou/gopsutil/disk"
	"github.com/spf13/viper"
	"github.com/zeebo/blake3"

	"gnomatix/dreamfs/v2/pkg/metadata"
	"gnomatix/dreamfs/v2/pkg/network"
	"gnomatix/dreamfs/v2/pkg/storage"
	"gnomatix/dreamfs/v2/pkg/ui"
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
			return fmt.Sprintf("%s:/%s%s", server, share, rest), nil
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
		if n, err := f.Read(head); err != nil && err != io.EOF {
			return "", fmt.Errorf("read head: %w", err)
		} else {
			data = append(data, head[:n]...)
		}

		midOffset := info.Size() / 2
		if _, err := f.Seek(midOffset, io.SeekStart); err != nil {
			return "", fmt.Errorf("seek middle: %w", err)
		}
		mid := make([]byte, fileSampleSize)
		if n, err := f.Read(mid); err != nil && err != io.EOF {
			return "", fmt.Errorf("read middle: %w", err)
		} else {
			data = append(data, mid[:n]...)
		}

		tailOffset := info.Size() - fileSampleSize
		if _, err := f.Seek(tailOffset, io.SeekStart); err != nil {
			return "", fmt.Errorf("seek tail: %w", err)
		}
		tail := make([]byte, fileSampleSize)
		if n, err := f.Read(tail); err != nil && err != io.EOF {
			return "", fmt.Errorf("read tail: %w", err)
		} else {
			data = append(data, tail[:n]...)
		}
	}

	hash := blake3.Sum256(data)
	return fmt.Sprintf("%x", hash), nil
}

// ------------------------
// Volume Identification
// ------------------------

type DreamFSVolumeMeta struct {
	VolumeID string `json:"volume_id"`
}

func GetVolumeSignature(root string) (string, error) {
	dreamfsPath := filepath.Join(root, ".dreamfs")
	
	// 1. Try to read existing .dreamfs file
	if data, err := os.ReadFile(dreamfsPath); err == nil {
		var meta DreamFSVolumeMeta
		if err := json.Unmarshal(data, &meta); err == nil && meta.VolumeID != "" {
			return meta.VolumeID, nil
		}
	}

	// 2. Either file doesn't exist or is invalid. Create a new one.
	absRoot, err := filepath.Abs(root)
	if err != nil {
		absRoot = root
	}

	// 3. Try to get physical disk serial number
	var newID string
	serialNumber := ""
	
	parts, err := disk.Partitions(false)
	if err == nil {
		var bestMatch disk.PartitionStat
		bestLen := 0
		for _, p := range parts {
			if strings.HasPrefix(absRoot, p.Mountpoint) && len(p.Mountpoint) > bestLen {
				bestLen = len(p.Mountpoint)
				bestMatch = p
			}
		}
		if bestLen > 0 {
			// Windows fallback: gopsutil v3 on Windows often lacks disk.SerialNumber in older minor versions
			if runtime.GOOS == "windows" {
				// e.g. bestMatch.Mountpoint is "C:" or "C:\"
				driveLetter := filepath.VolumeName(bestMatch.Mountpoint)
				if driveLetter != "" {
					out, err := exec.Command("cmd", "/c", "vol", driveLetter).Output()
					if err == nil {
						// Look for "Volume Serial Number is 1234-5678"
						re := regexp.MustCompile(`(?i)Volume Serial Number is\s+([A-F0-9-]+)`)
						matches := re.FindStringSubmatch(string(out))
						if len(matches) > 1 {
							serialNumber = matches[1]
						}
					}
				}
			} else {
				// On Linux/Mac, try to read from sysfs or similar if possible. We'll skip gopsutil's SerialNumber 
				// entirely here to fix the compilation error since it's undefined in this version.
			}
		}
	}

	if serialNumber != "" {
		newID = "PHYS:" + strings.TrimSpace(serialNumber)
	} else {
		// 4. Fallback to random UUID if no physical serial
		newID = "UUID:" + utils.GenerateUUID(fmt.Sprintf("%d-%s", time.Now().UnixNano(), absRoot))
		// The original code used utils.GenerateUUID(string) which generates an MD5 UUID from a string
		// Since user requested a random UUID fallback, injecting time/path makes it unique enough
	}

	meta := DreamFSVolumeMeta{VolumeID: newID}
	data, _ := json.MarshalIndent(meta, "", "  ")
	
	err = os.WriteFile(dreamfsPath, data, 0644)
	if err != nil {
		return "", fmt.Errorf("failed to write .dreamfs volume signature: %w", err)
	}
	
	return newID, nil
}

// Global swarm delegate.
func ProcessFile(ctx context.Context, filePath string, cw *storage.CacheWriter, volumeID string, swarmDelegate *network.SwarmDelegate) (string, error) {
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
	if cw != nil {
		bytes := info.Size()
		modTime := info.ModTime().Format(time.RFC3339)

		// Create the stable prefix that identifies this file's exact state at this location
		idPrefix := utils.HostID + "|" + volumeID + "|" + canonicalPath + "|" + modTime + "|" + strconv.FormatInt(bytes, 16) + "|"
		
		// Fast-fail: if we already have this exact prefix in the DB, it means this exact 
		// path with this exact size and modtime has already been hashed. We can skip processing.
		if cw.Store() != nil {
			if exists, err := cw.Store().PrefixHas(idPrefix); err == nil && exists {
				// We skip fingerprinting. We don't need to return the hash for progress, just that we processed it.
				return "SKIPPED_UNMODIFIED", nil
			}
		}

		// Proceed with fingerprinting since it's new, modified, or missing from the DB
		fingerprint, err := FingerprintFile(filePath)
		if err != nil {
			return "", fmt.Errorf("failed to fingerprint %s: %w", filePath, err)
		}

		idString := idPrefix + fingerprint
		UUID := utils.GenerateUUID(idString)

		meta := metadata.FileMetadata{
			ID:       UUID,
			IDString: idString,
			HostID:   utils.HostID,
			FilePath: canonicalPath,
			Size:     bytes,
			ModTime:  modTime,
			BLAKE3:   fingerprint,
			Extra: map[string]interface{}{
				"volume_id": volumeID,
			},
		}
		
		cw.Write(meta) // BATCHED DB WRITE

		if swarmDelegate != nil {
			data, err := json.Marshal(meta)
			if err == nil {
				swarmDelegate.Broadcasts.QueueBroadcast(&network.FileMetaBroadcast{Msg: data})
			}
		}
		return fingerprint, nil
	}

	fingerprint, err := FingerprintFile(filePath)
	if err != nil {
		return "", fmt.Errorf("failed to fingerprint %s: %w", filePath, err)
	}
	return fingerprint, nil
}

// ------------------------
// Directory Processing with godirwalk and Charm UI Feedback
// ------------------------

// ProcessAllDirectories scans the root directory and processes its files,
// then collects subdirectories and processes them one at a time. A spinner is
// shown while reading directories, and a progress bar is updated per subdirectory.
// ProcessAllDirectories scans the root directory and processes its files,
// then collects subdirectories and processes them one at a time. A spinner is
// shown while reading directories, and a progress bar is updated per subdirectory.
func ProcessAllDirectories(ctx context.Context, root string, cw *storage.CacheWriter, numWorkers int, swarmDelegate *network.SwarmDelegate) error {
	quiet := viper.GetBool("quiet")
	
	volumeID, err := GetVolumeSignature(root)
	if err != nil {
		if !quiet {
			fmt.Printf("Warning: Could not resolve volume signature for %s: %v\n", root, err)
		}
		volumeID = "UNKNOWN"
	}
	if !quiet {
		fmt.Printf("Volume Signature: %s\n", volumeID)
		fmt.Println("Reading files...")
	}

	// Always sort directory entries. This is strictly required to prevent I/O queuing 
	// disaster on SMR Hard Drives, and performs well enough on NVMe/SSD to be a safe default.
	// Process files in the root directory.
	if !quiet {
		fmt.Printf("Processing root directory: %s\n", root)
	}
	err = godirwalk.Walk(root, &godirwalk.Options{
		Unsorted: false,
		Callback: func(path string, de *godirwalk.Dirent) error {
			select {
			case <-ctx.Done():
				return ctx.Err()
			default:
			}
			// Only process files directly in root.
			if de.IsDir() && path != root {
				return godirwalk.SkipThis
			}
			if !de.IsDir() && filepath.Base(path) != ".dreamfs" {
				_, err := ProcessFile(ctx, path, cw, volumeID, swarmDelegate)
				if err != nil && !quiet {
					fmt.Printf("Error processing %s: %v\n", path, err)
				}
			}
			return nil
		},
		ErrorCallback: func(osPathname string, err error) godirwalk.ErrorAction {
			if !quiet {
				fmt.Printf("Walk error on %s: %v\n", osPathname, err)
			}
			return godirwalk.SkipNode
		},
	})
	if err != nil && err != io.EOF {
		return fmt.Errorf("godirwalk1: %w", err)
	}

	// Collect all subdirectories.
	var subdirs []string
	err = godirwalk.Walk(root, &godirwalk.Options{
		Unsorted: false,
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
		ErrorCallback: func(osPathname string, err error) godirwalk.ErrorAction {
			if !quiet {
				fmt.Printf("Walk error on %s: %v\n", osPathname, err)
			}
			return godirwalk.SkipNode
		},
	})
	if err != nil && err != io.EOF {
		return fmt.Errorf("godirwalk2: %w", err)
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
			Unsorted: false,
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
			ErrorCallback: func(osPathname string, err error) godirwalk.ErrorAction {
				if !quiet {
					fmt.Printf("Walk error on %s: %v\n", osPathname, err)
				}
				return godirwalk.SkipNode
			},
		})
		if err != nil && err != io.EOF {
			if !quiet {
				fmt.Printf("Error reading directory %s: %v\n", dir, err)
			}
			continue
		}
		totalFiles := len(filesInDir)
		if totalFiles == 0 {
			continue
		}

		// Use passed workers or default to 1.
		if numWorkers <= 0 {
			numWorkers = 1
		}

		// Progress tracking (mutex-protected for concurrent workers).
		var (
			processed    int64
			progressMu   sync.Mutex
			p            = ui.ThemedProgressBar()
		)
		if !quiet {
			fmt.Printf("Processing files in %s... (%d workers)\n", dir, numWorkers)
		}

		// Feed the task queue (sorted list → channel).
		taskCh := make(chan string, numWorkers*2)
		go func() {
			defer close(taskCh)
			for _, fpath := range filesInDir {
				select {
				case <-ctx.Done():
					return
				case taskCh <- fpath:
				}
			}
		}()

		// Launch worker pool.
		var wg sync.WaitGroup
		for w := 0; w < numWorkers; w++ {
			workerID := strconv.Itoa(w)
			wg.Add(1)
			go func(id string) {
				defer wg.Done()
				
				// Initialize worker-specific logging in XDG State
				logger, f, err := utils.GetWorkerLogger(id)
				if err == nil {
					defer f.Close()
					logger.Printf("Worker %s started processing %s", id, dir)
				}

				for fpath := range taskCh {
					select {
					case <-ctx.Done():
						return
					default:
					}
					
					if logger != nil {
						logger.Printf("Processing: %s", fpath)
					}

					_, err := ProcessFile(ctx, fpath, cw, volumeID, swarmDelegate)
					if err != nil {
						if !quiet {
							fmt.Printf("\nError processing %s: %v\n", fpath, err)
						}
						if logger != nil {
							logger.Printf("ERROR processing %s: %v", fpath, err)
						}
					}
					progressMu.Lock()
					processed++
					if !quiet {
						pct := float64(processed) / float64(totalFiles)
						fmt.Printf("\r%s", p.ViewAs(pct))
					}
					progressMu.Unlock()
				}
				if logger != nil {
					logger.Printf("Worker %s finished", id)
				}
			}(workerID)
		}
		wg.Wait()
		if !quiet {
			fmt.Println()
		}
	}
	return nil
}
