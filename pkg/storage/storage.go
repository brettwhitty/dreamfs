package storage

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
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