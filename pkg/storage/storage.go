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

	"gnomatix/dreamfs/v2/pkg/metadata"
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
		return b.Put([]byte(meta.ID), data)
	})
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
	quit          chan struct{}
	wg            sync.WaitGroup
}

func NewCacheWriter(ps *PersistentStore, batchSize int, flushInterval time.Duration) *CacheWriter {
	cw := &CacheWriter{
		ps:            ps,
		ch:            make(chan metadata.FileMetadata, batchSize*2),
		batchSize:     batchSize,
		flushInterval: flushInterval,
		flushNowCh:    make(chan struct{}),
		quit:          make(chan struct{}),
	}
	cw.wg.Add(1)
	go cw.run()
	return cw
}

func (cw *CacheWriter) run() {
	var batch []metadata.FileMetadata
	timer := time.NewTimer(cw.flushInterval)
	for {
		select {
		case meta := <-cw.ch:
			batch = append(batch, meta)
			if len(batch) >= cw.batchSize {
				cw.flush(batch)
				batch = nil
				if !timer.Stop() {
					<-timer.C
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
			if len(batch) > 0 {
				cw.flush(batch)
				batch = nil
			}
		case <-cw.quit:
			if len(batch) > 0 {
				cw.flush(batch)
			}
			return
		}
	}
}

func (cw *CacheWriter) flush(batch []metadata.FileMetadata) {
	err := cw.ps.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte("metadata"))
		for _, meta := range batch {
			data, err := json.Marshal(meta)
			if err != nil {
				return err
			}
			if err := b.Put([]byte(meta.ID), data); err != nil {
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
}

func (cw *CacheWriter) Close() {
	close(cw.quit)
	cw.wg.Wait()
}
