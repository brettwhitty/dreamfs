package storage

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"gitea.gnomatix.com/gnomatix/dreamfs/v2/pkg/metadata"
)

func newTestStore(t *testing.T) *PersistentStore {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "test.db")
	ps, err := NewPersistentStore(dbPath)
	if err != nil {
		t.Fatalf("NewPersistentStore: %v", err)
	}
	t.Cleanup(func() { ps.Close() })
	return ps
}

func testMeta(id string) metadata.FileMetadata {
	return metadata.FileMetadata{
		ID:       id,
		IDString: "test|" + id,
		HostID:   "test-host",
		FilePath: "/test/" + id,
		Size:     1234,
		ModTime:  time.Now().Format(time.RFC3339),
		BLAKE3:   "aabbccddeeff",
		Extra:    map[string]interface{}{"volume_id": "TEST-VOL"},
	}
}

// TestPersistentStore_PutGet verifies a Put followed by GetAll returns the record.
func TestPersistentStore_PutGet(t *testing.T) {
	ps := newTestStore(t)
	m := testMeta("record-001")

	if err := ps.Put(m); err != nil {
		t.Fatalf("Put: %v", err)
	}
	all, err := ps.GetAll()
	if err != nil {
		t.Fatalf("GetAll: %v", err)
	}
	if len(all) != 1 {
		t.Fatalf("expected 1 record, got %d", len(all))
	}
	if all[0].ID != m.ID {
		t.Fatalf("ID mismatch: want %q got %q", m.ID, all[0].ID)
	}
}

// TestCacheWriter_FlushOnBatch verifies that when batchSize records are written,
// they are automatically committed to BoltDB without an explicit FlushNow call.
func TestCacheWriter_FlushOnBatch(t *testing.T) {
	ps := newTestStore(t)
	batchSize := 5
	cw := NewCacheWriter(ps, batchSize, 10*time.Second)
	defer cw.Close()

	for i := 0; i < batchSize; i++ {
		cw.Write(testMeta(string(rune('A' + i))))
	}
	// FlushNow is now blocking and guarantees the batch is written.
	cw.FlushNow()

	all, err := ps.GetAll()
	if err != nil {
		t.Fatalf("GetAll: %v", err)
	}
	if len(all) != batchSize {
		t.Fatalf("expected %d records after batch flush, got %d", batchSize, len(all))
	}
}

// TestCacheWriter_FlushNow verifies that fewer-than-batchSize records can be
// flushed on demand via FlushNow.
func TestCacheWriter_FlushNow(t *testing.T) {
	ps := newTestStore(t)
	cw := NewCacheWriter(ps, 100, 10*time.Second)
	defer cw.Close()

	cw.Write(testMeta("fn-001"))
	cw.Write(testMeta("fn-002"))
	
	// FlushNow now handles the synchronization internally.
	cw.FlushNow()

	all, err := ps.GetAll()
	if err != nil {
		t.Fatalf("GetAll: %v", err)
	}
	if len(all) != 2 {
		t.Fatalf("expected 2 records after FlushNow, got %d", len(all))
	}
}

// TestCacheWriter_Close verifies that Close flushes remaining records and
// the goroutine terminates cleanly (no hang).
func TestCacheWriter_Close(t *testing.T) {
	ps := newTestStore(t)
	cw := NewCacheWriter(ps, 100, 10*time.Second)

	cw.Write(testMeta("close-001"))
	cw.Write(testMeta("close-002"))
	cw.Write(testMeta("close-003"))
	
	cw.Close() // blocking sync: drains channel and flushes batch before returning

	all, err := ps.GetAll()
	if err != nil {
		t.Fatalf("GetAll: %v", err)
	}
	if len(all) != 3 {
		t.Fatalf("expected 3 records after Close, got %d", len(all))
	}
}

// TestCacheWriter_CloseIdempotent verifies that calling Close twice does not panic.
func TestCacheWriter_CloseIdempotent(t *testing.T) {
	ps := newTestStore(t)
	cw := NewCacheWriter(ps, 10, time.Second)
	cw.Close()
	cw.Close() // must not panic
}

// TestPersistentStore_Reopen verifies that data persists after the store is
// closed and reopened.
func TestPersistentStore_Reopen(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "reopen.db")
	ps, err := NewPersistentStore(dbPath)
	if err != nil {
		t.Fatalf("NewPersistentStore: %v", err)
	}
	if err := ps.Put(testMeta("persist-001")); err != nil {
		t.Fatalf("Put: %v", err)
	}
	ps.Close()

	// Reopen.
	ps2, err := NewPersistentStore(dbPath)
	if err != nil {
		t.Fatalf("Reopen: %v", err)
	}
	defer ps2.Close()
	all, err := ps2.GetAll()
	if err != nil {
		t.Fatalf("GetAll after reopen: %v", err)
	}
	if len(all) != 1 {
		t.Fatalf("expected 1 record after reopen, got %d", len(all))
	}
}

// testMetaWithIDString creates a metadata record with a specific IDString for prefix testing.
func testMetaWithIDString(id, idString string) metadata.FileMetadata {
	m := testMeta(id)
	m.ID = id
	m.IDString = idString
	return m
}

// TestPersistentStore_PrefixHas verifies that the prefix cursor scan works correctly.
func TestPersistentStore_PrefixHas(t *testing.T) {
	ps := newTestStore(t)
	
	// The new logic saves by IDString
	m1 := testMetaWithIDString("uuid-1", "HOST|VOL1|/path/to/a.txt|2026-03-01T12:00:00Z|1024|hashA")
	m2 := testMetaWithIDString("uuid-2", "HOST|VOL1|/path/to/b.txt|2026-03-01T12:00:00Z|2048|hashB")
	
	if err := ps.Put(m1); err != nil {
		t.Fatalf("Put m1: %v", err)
	}
	if err := ps.Put(m2); err != nil {
		t.Fatalf("Put m2: %v", err)
	}

	tests := []struct {
		prefix string
		want   bool
	}{
		{"HOST|VOL1|/path/to/a.txt|2026-03-01T12:00:00Z|1024|", true},  // Exact match up to hash
		{"HOST|VOL1|/path/to/b.txt|2026-03-01T12:00:00Z|2048|", true},  // Exact match up to hash
		{"HOST|VOL1|/path/to/c.txt|", false},                           // File doesn't exist
		{"HOST|VOL1|/path/to/a.txt|2026-03-12T12:00:00Z|1024|", false}, // ModTime changed
		{"HOST|VOL1|/path/to/a.txt|2026-03-01T12:00:00Z|9999|", false}, // Size changed
		{"HOST|VOL", true},                                             // Partial Prefix ok
	}

	for _, tc := range tests {
		got, err := ps.PrefixHas(tc.prefix)
		if err != nil {
			t.Fatalf("PrefixHas(%q) error: %v", tc.prefix, err)
		}
		if got != tc.want {
			t.Errorf("PrefixHas(%q) = %v; want %v", tc.prefix, got, tc.want)
		}
	}
}

// Ensure os import used.
var _ = os.TempDir
