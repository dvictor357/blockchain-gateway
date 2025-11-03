package cache

import (
	"context"
	"encoding/json"
	"fmt"
	"time"
)

// CacheValue represents a cached value with metadata
type CacheValue struct {
	Data      json.RawMessage `json:"data"`
	CreatedAt time.Time       `json:"created_at"`
	ExpiresAt time.Time       `json:"expires_at"`
	Hits      int64           `json:"hits"`
	Chain     string          `json:"chain,omitempty"`
	Method    string          `json:"method,omitempty"`
}

// CacheLayer represents a cache storage layer
type CacheLayer interface {
	// Get retrieves a value from the cache
	Get(ctx context.Context, key string) (*CacheValue, error)

	// Set stores a value in the cache
	Set(ctx context.Context, key string, value *CacheValue, ttl time.Duration) error

	// Delete removes a value from the cache
	Delete(ctx context.Context, key string) error

	// Clear removes all values from the cache
	Clear(ctx context.Context) error

	// Name returns the name of the cache layer
	Name() string

	// GetStats returns statistics about the cache
	GetStats(ctx context.Context) (map[string]interface{}, error)
}

// TTL Constants for cache layers
const (
	// L1 (In-Memory) TTLs - Short duration for hot data
	L1BalanceTTL = 30 * time.Second
	L1BlockTTL   = 30 * time.Second
	L1TxTTL      = 5 * time.Minute
)

// CacheStats holds statistics for a cache layer
type CacheStats struct {
	Hits        int64     `json:"hits"`
	Misses      int64     `json:"misses"`
	HitRatio    float64   `json:"hit_ratio"`
	Items       int       `json:"items"`
	BytesUsed   int64     `json:"bytes_used"`
	LastUpdated time.Time `json:"last_updated"`
}

// MergeCacheStats merges cache statistics from multiple layers
func MergeCacheStats(statsList []CacheStats) CacheStats {
	var totalHits, totalMisses, totalItems int64
	var totalBytes int64

	for _, stats := range statsList {
		totalHits += stats.Hits
		totalMisses += stats.Misses
		totalItems += int64(stats.Items)
		totalBytes += stats.BytesUsed
	}

	totalRequests := totalHits + totalMisses
	var hitRatio float64
	if totalRequests > 0 {
		hitRatio = float64(totalHits) / float64(totalRequests)
	}

	return CacheStats{
		Hits:        totalHits,
		Misses:      totalMisses,
		HitRatio:    hitRatio,
		Items:       int(totalItems),
		BytesUsed:   totalBytes,
		LastUpdated: time.Now(),
	}
}

// Common cache errors
var (
	ErrCacheMiss    = fmt.Errorf("cache miss")
	ErrCacheExpired = fmt.Errorf("cache entry expired")
)
