package fileprocessor

import (
	"sync"
	"time"

	"github.com/shirou/gopsutil/disk"
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