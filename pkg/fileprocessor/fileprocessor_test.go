package fileprocessor

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"gitea.gnomatix.com/gnomatix/dreamfs/v2/pkg/storage"
	"gitea.gnomatix.com/gnomatix/dreamfs/v2/pkg/utils"
)

func init() {
	// Ensure HostID is set for deterministic UUID generation in tests.
	utils.SetHostID("test-host-id")
}

// TestGetVolumeSignature_CreateNew verifies that calling GetVolumeSignature on a
// fresh directory creates a .dreamfs file and returns a non-empty volume_id.
func TestGetVolumeSignature_CreateNew(t *testing.T) {
	dir := t.TempDir()
	id, err := GetVolumeSignature(dir)
	if err != nil {
		t.Fatalf("GetVolumeSignature returned error: %v", err)
	}
	if id == "" {
		t.Fatal("expected non-empty volume ID")
	}

	// .dreamfs file must exist.
	df := filepath.Join(dir, ".dreamfs")
	data, err := os.ReadFile(df)
	if err != nil {
		t.Fatalf(".dreamfs file not created: %v", err)
	}

	// Must parse as valid JSON with a volume_id field.
	var meta DreamFSVolumeMeta
	if err := json.Unmarshal(data, &meta); err != nil {
		t.Fatalf(".dreamfs contains invalid JSON: %v", err)
	}
	if meta.VolumeID == "" {
		t.Fatal("volume_id in .dreamfs is empty")
	}
	if meta.VolumeID != id {
		t.Fatalf("returned ID %q does not match .dreamfs ID %q", id, meta.VolumeID)
	}
}

// TestGetVolumeSignature_ReadExisting verifies that a pre-existing .dreamfs file
// is read correctly and the stored ID is returned without modification.
func TestGetVolumeSignature_ReadExisting(t *testing.T) {
	dir := t.TempDir()
	existingID := "PHYS:TEST-SERIAL-12345"
	meta := DreamFSVolumeMeta{VolumeID: existingID}
	data, _ := json.MarshalIndent(meta, "", "  ")
	if err := os.WriteFile(filepath.Join(dir, ".dreamfs"), data, 0644); err != nil {
		t.Fatalf("could not create test .dreamfs: %v", err)
	}

	id, err := GetVolumeSignature(dir)
	if err != nil {
		t.Fatalf("GetVolumeSignature returned error: %v", err)
	}
	if id != existingID {
		t.Fatalf("expected %q, got %q", existingID, id)
	}
}

// TestGetVolumeSignature_Idempotent verifies that calling GetVolumeSignature twice
// on the same directory always returns the same ID.
func TestGetVolumeSignature_Idempotent(t *testing.T) {
	dir := t.TempDir()
	id1, err := GetVolumeSignature(dir)
	if err != nil {
		t.Fatalf("first call error: %v", err)
	}
	id2, err := GetVolumeSignature(dir)
	if err != nil {
		t.Fatalf("second call error: %v", err)
	}
	if id1 != id2 {
		t.Fatalf("volume IDs differ between calls: %q vs %q", id1, id2)
	}
}

// TestFingerprintFile_Small verifies that a file smaller than 3×1MB is
// fingerprinted without error and returns a non-empty hex hash.
func TestFingerprintFile_Small(t *testing.T) {
	f, err := os.CreateTemp(t.TempDir(), "small-*.bin")
	if err != nil {
		t.Fatal(err)
	}
	f.WriteString(strings.Repeat("A", 512))
	f.Close()

	hash, err := FingerprintFile(f.Name())
	if err != nil {
		t.Fatalf("FingerprintFile error: %v", err)
	}
	if hash == "" {
		t.Fatal("expected non-empty hash")
	}
}

// TestFingerprintFile_Large verifies that the 3-sample head/mid/tail path is
// exercised for files > 3MB and produces a stable hash.
func TestFingerprintFile_Large(t *testing.T) {
	f, err := os.CreateTemp(t.TempDir(), "large-*.bin")
	if err != nil {
		t.Fatal(err)
	}
	// Write 4MB of data.
	chunk := make([]byte, 1<<20) // 1MB
	for i := 0; i < 4; i++ {
		f.Write(chunk)
	}
	f.Close()

	h1, err := FingerprintFile(f.Name())
	if err != nil {
		t.Fatalf("FingerprintFile error: %v", err)
	}
	// Must be deterministic.
	h2, err := FingerprintFile(f.Name())
	if err != nil {
		t.Fatalf("FingerprintFile error on second call: %v", err)
	}
	if h1 != h2 {
		t.Fatalf("non-deterministic fingerprint: %q vs %q", h1, h2)
	}
}

// TestFingerprintFile_Empty verifies that a zero-byte file produces no error.
func TestFingerprintFile_Empty(t *testing.T) {
	f, err := os.CreateTemp(t.TempDir(), "empty-*.bin")
	if err != nil {
		t.Fatal(err)
	}
	f.Close()

	hash, err := FingerprintFile(f.Name())
	if err != nil {
		t.Fatalf("FingerprintFile error on empty file: %v", err)
	}
	if hash == "" {
		t.Fatal("expected non-empty hash for empty file (hash of empty bytes)")
	}
}

// TestProcessFile_NilWriter verifies that ProcessFile with a nil CacheWriter
// still computes and returns a fingerprint without panicking or erroring.
func TestProcessFile_NilWriter(t *testing.T) {
	f, err := os.CreateTemp(t.TempDir(), "proc-*.txt")
	if err != nil {
		t.Fatal(err)
	}
	f.WriteString("hello dreamfs")
	f.Close()

	fingerprint, err := ProcessFile(context.Background(), f.Name(), nil, "TEST-VOL-ID", nil)
	if err != nil {
		t.Fatalf("ProcessFile error: %v", err)
	}
	if fingerprint == "" {
		t.Fatal("expected non-empty fingerprint")
	}
}
// TestProcessAllDirectories exercises the worker pool concurrency.
func TestProcessAllDirectories(t *testing.T) {
	// Setup a temporary directory with multiple subdirectories and files.
	root := t.TempDir()
	files := map[string]string{
		"a.txt": "content a",
		"b.txt": "content b",
		"sub/c.txt": "content c",
		"sub/d.txt": "content d",
		"other/e.txt": "content e",
	}
	for path, content := range files {
		fullPath := filepath.Join(root, path)
		os.MkdirAll(filepath.Dir(fullPath), 0755)
		os.WriteFile(fullPath, []byte(content), 0644)
	}

	// Setup PersistentStore and CacheWriter.
	dbPath := filepath.Join(t.TempDir(), "test-concurrency.db")
	ps, _ := storage.NewPersistentStore(dbPath)
	defer ps.Close()
	cw := storage.NewCacheWriter(ps, 2, 100*time.Millisecond)
	defer cw.Close()

	// Run processing with multiple workers.
	ctx := context.Background()
	numWorkers := 4
	err := ProcessAllDirectories(ctx, root, cw, numWorkers, nil)
	if err != nil {
		t.Fatalf("ProcessAllDirectories error: %v", err)
	}

	// Close the writer to flush all records.
	cw.Close()

	// Verify all files were processed and stored.
	all, err := ps.GetAll()
	if err != nil {
		t.Fatalf("GetAll error: %v", err)
	}
	
	if len(all) != len(files) {
		t.Errorf("expected %d records, got %d", len(files), len(all))
	}
}


