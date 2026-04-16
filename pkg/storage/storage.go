package storage

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"

	bolt "go.etcd.io/bbolt"

	"gitea.gnomatix.com/gnomatix/dreamfs/v2/pkg/metadata"
)

// ------------------------
// Persistent Storage (BoltDB)
// ------------------------

type PersistentStore struct {
	db *bolt.DB
}

const boltBucketName = "metadata"

func NewPersistentStore(dbPath string) (*PersistentStore, error) {
	// Ensure the parent directory exists.
	if err := os.MkdirAll(filepath.Dir(dbPath), 0755); err != nil {
		return nil, err
	}
	db, err := bolt.Open(dbPath, 0600, &bolt.Options{Timeout: 1 * time.Second})
	if err != nil {
		return nil, fmt.Errorf("open bolt db: %w", err)
	}
	err = db.Update(func(tx *bolt.Tx) error {
		_, err := tx.CreateBucketIfNotExists([]byte(boltBucketName))
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

func (ps *PersistentStore) Put(meta metadata.FileMetadata) error {
	data, err := json.Marshal(meta)
	if err != nil {
		return fmt.Errorf("marshal metadata: %w", err)
	}
	return ps.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(boltBucketName))
		return b.Put([]byte(meta.IDString), data) // Note: using IDString as the key makes prefix scans much faster
	})
}

// PrefixHas checks if the store contains any key that starts with the given prefix.
func (ps *PersistentStore) PrefixHas(prefix string) (bool, error) {
	var found bool
	err := ps.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(boltBucketName))
		if b == nil {
			return nil
		}
		c := b.Cursor()
		prefixBytes := []byte(prefix)
		k, _ := c.Seek(prefixBytes)
		if k != nil {
			// Compare up to the length of prefixBytes. Since we are looking for a prefix match,
			// the first len(prefixBytes) bytes of k must equal prefixBytes.
			if len(k) >= len(prefixBytes) && string(k[:len(prefixBytes)]) == string(prefixBytes) {
				found = true
			}
		}
		return nil
	})
	return found, err
}

func (ps *PersistentStore) GetAll() ([]metadata.FileMetadata, error) {
	var results []metadata.FileMetadata
	err := ps.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(boltBucketName))
		return b.ForEach(func(k, v []byte) error {
			var meta metadata.FileMetadata
			if err := json.Unmarshal(v, &meta); err != nil {
				return err
			}
			results = append(results, meta)
			return nil
		})
	})
	return results, err
}

// CACHE WRITER (In-Memory Caching to Batch Writes)
type CacheWriter struct {
	ps            *PersistentStore           // Reference to PersistentStore in this package
	ch            chan metadata.FileMetadata // Reference to FileMetadata from metadata package
	batchSize     int
	flushInterval time.Duration
	flushNowCh    chan struct{}
	flushDoneCh   chan struct{} // Used for testing to wait for flush completion
	quit          chan struct{}
	wg            sync.WaitGroup
}

func NewCacheWriter(ps *PersistentStore, batchSize int, flushInterval time.Duration) *CacheWriter {
	cw := &CacheWriter{
		ps:            ps,
		ch:            make(chan metadata.FileMetadata, batchSize*2),
		batchSize:     batchSize,
		flushInterval: flushInterval,
		flushNowCh:    make(chan struct{}, 1),
		flushDoneCh:   make(chan struct{}, 1),
		quit:          make(chan struct{}),
	}
	cw.wg.Add(1)
	go cw.run()
	return cw
}

// Store returns the underlying persistent store, allowing for reads while reusing the cache writer instance.
func (cw *CacheWriter) Store() *PersistentStore {
	return cw.ps
}

func (cw *CacheWriter) run() {
	defer cw.wg.Done()
	var batch []metadata.FileMetadata
	timer := time.NewTimer(cw.flushInterval)
	defer timer.Stop()

	for {
		select {
		case meta, ok := <-cw.ch:
			if !ok {
				// Channel closed, process everything in the final batch and return.
				if len(batch) > 0 {
					cw.flush(batch)
				}
				return
			}
			batch = append(batch, meta)
			if len(batch) >= cw.batchSize {
				cw.flush(batch)
				batch = nil
				if !timer.Stop() {
					select {
					case <-timer.C:
					default:
					}
				}
				timer.Reset(cw.flushInterval)
			}
		case <-timer.C:
			if len(batch) > 0 {
				cw.flush(batch)
				batch = nil
			}
			timer.Reset(cw.flushInterval)
		case <-cw.flushNowCh:
			// To make FlushNow deterministic in tests, we should drain the current channel as much as possible 
			// before flushing. 
			drain := true
			for drain {
				select {
				case m, ok := <-cw.ch:
					if !ok {
						drain = false
					} else {
						batch = append(batch, m)
					}
				default:
					drain = false
				}
			}
			if len(batch) > 0 {
				cw.flush(batch)
				batch = nil
			}
			timer.Reset(cw.flushInterval)
			// Signal done
			select {
			case cw.flushDoneCh <- struct{}{}:
			default:
			}
// Note: We no longer check <-cw.quit here. 
		// Close() closes cw.ch, so we exit via the 'ok == false' case above 
		// after all pending items are processed.
		}
	}
}

func (cw *CacheWriter) flush(batch []metadata.FileMetadata) {
	err := cw.ps.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(boltBucketName))
		if b == nil {
			return fmt.Errorf("bucket %s not found", boltBucketName)
		}
		for _, meta := range batch {
			data, err := json.Marshal(meta)
			if err != nil {
				return err
			}
			if err := b.Put([]byte(meta.IDString), data); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		log.Printf("CacheWriter flush error: %v", err)
	}
}

func (cw *CacheWriter) Write(meta metadata.FileMetadata) {
	cw.ch <- meta
}

func (cw *CacheWriter) FlushNow() {
	cw.flushNowCh <- struct{}{}
	<-cw.flushDoneCh
}

func (cw *CacheWriter) Close() {
	select {
	case <-cw.quit:
		// already closed
	default:
		// 1. Signal immediate stop for the timer loop and others
		close(cw.quit)
		// 2. Close the channel to signal no more writes and trigger drainage in run()
		close(cw.ch)
		// 3. Wait for run() to finish flushing everything
		cw.wg.Wait()
	}
}
