package cache

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"
)

// CacheAggregator implements a 2-layer cache with cache-aside pattern
type CacheAggregator struct {
	l1      *MemoryCache
	l2      *RedisCache
	fetcher DataFetcher
	mu      sync.RWMutex
	stats   AggregatorStats
}

// DataFetcher is a function that fetches data from the source
type DataFetcher func(ctx context.Context) (json.RawMessage, error)

// AggregatorStats holds statistics for all cache layers
type AggregatorStats struct {
	TotalHits   int64            `json:"total_hits"`
	TotalMisses int64            `json:"total_misses"`
	LayerStats  map[string]int64 `json:"layer_stats"`
	LastUpdated time.Time        `json:"last_updated"`
}

// CacheOptions holds configuration options for the cache aggregator
type CacheOptions struct {
	L1MaxItems  int
	L2Enabled   bool
	L3Enabled   bool
	EnableStats bool
}

// NewCacheAggregator creates a new cache aggregator
func NewCacheAggregator(
	l1 *MemoryCache,
	l2 *RedisCache,
	fetcher DataFetcher,
) *CacheAggregator {
	return &CacheAggregator{
		l1:      l1,
		l2:      l2,
		fetcher: fetcher,
		stats: AggregatorStats{
			LayerStats: make(map[string]int64),
		},
	}
}

// Get retrieves data from the cache, falling back to the source
func (c *CacheAggregator) Get(ctx context.Context, key string, ttl time.Duration) (json.RawMessage, error) {
	// Try L1 cache first
	if c.l1 != nil {
		cacheVal, err := c.l1.Get(ctx, key)
		if err == nil {
			c.updateStats("L1", true)
			return cacheVal.Data, nil
		}
		if err != ErrCacheMiss && err != ErrCacheExpired {
			fmt.Printf("L1 cache error: %v\n", err)
		}
	}

	// Try L2 cache (Redis)
	if c.l2 != nil {
		cacheVal, err := c.l2.Get(ctx, key)
		if err == nil {
			// Populate L1 cache synchronously
			c.populateL1(ctx, key, cacheVal, ttl)
			c.updateStats("L2", true)
			return cacheVal.Data, nil
		}
		if err != ErrCacheMiss && err != ErrCacheExpired {
			fmt.Printf("L2 cache error: %v\n", err)
		}
	}

	// Cache miss - fetch from source
	c.updateStats("SOURCE", false)

	result, err := c.fetcher(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch data: %w", err)
	}

	// Create cache value
	cacheVal := &CacheValue{
		Data:      result,
		CreatedAt: time.Now(),
		ExpiresAt: time.Now().Add(ttl),
		Hits:      0,
		Chain:     extractChainFromKey(key),
		Method:    extractMethodFromKey(key),
	}

	// Populate both cache layers
	if c.l2 != nil {
		c.l2.Set(ctx, key, cacheVal, ttl)
	}
	if c.l1 != nil {
		c.l1.Set(ctx, key, cacheVal, ttl)
	}

	return result, nil
}

// Set manually sets a value in both cache layers
func (c *CacheAggregator) Set(ctx context.Context, key string, data json.RawMessage, ttl time.Duration) error {
	cacheVal := &CacheValue{
		Data:      data,
		CreatedAt: time.Now(),
		ExpiresAt: time.Now().Add(ttl),
		Hits:      0,
		Chain:     extractChainFromKey(key),
		Method:    extractMethodFromKey(key),
	}

	// Set in both layers
	if c.l2 != nil {
		if err := c.l2.Set(ctx, key, cacheVal, ttl); err != nil {
			return fmt.Errorf("L2 set error: %w", err)
		}
	}
	if c.l1 != nil {
		if err := c.l1.Set(ctx, key, cacheVal, ttl); err != nil {
			return fmt.Errorf("L1 set error: %w", err)
		}
	}

	return nil
}

// Invalidate removes a value from both cache layers
func (c *CacheAggregator) Invalidate(ctx context.Context, key string) error {
	if c.l1 != nil {
		c.l1.Delete(ctx, key)
	}
	if c.l2 != nil {
		c.l2.Delete(ctx, key)
	}
	return nil
}

// InvalidateByPattern removes values matching a pattern from both cache layers
func (c *CacheAggregator) InvalidateByPattern(ctx context.Context, pattern string) error {
	if c.l2 != nil {
		// Redis supports pattern matching
		iter := c.l2.client.Scan(ctx, 0, pattern, 100).Iterator()
		var keys []string
		for iter.Next(ctx) {
			keys = append(keys, iter.Val())
		}
		if len(keys) > 0 {
			c.l2.client.Del(ctx, keys...)
		}
	}

	// For L1, clear all keys (simple approach)
	if c.l1 != nil {
		c.l1.Clear(ctx)
	}

	return nil
}

// GetStats returns statistics for all cache layers
func (c *CacheAggregator) GetStats(ctx context.Context) (map[string]interface{}, error) {
	stats := make(map[string]interface{})

	// Get L1 stats
	if c.l1 != nil {
		l1Stats, err := c.l1.GetStats(ctx)
		if err == nil {
			stats["l1"] = l1Stats
		}
	}

	// Get L2 stats
	if c.l2 != nil {
		l2Stats, err := c.l2.GetStats(ctx)
		if err == nil {
			stats["l2"] = l2Stats
		}
	}

	// Get aggregator stats
	c.mu.RLock()
	aggregatorStats := map[string]any{
		"total_hits":   c.stats.TotalHits,
		"total_misses": c.stats.TotalMisses,
		"layer_stats":  c.stats.LayerStats,
		"last_updated": c.stats.LastUpdated,
	}
	c.mu.RUnlock()

	stats["aggregator"] = aggregatorStats

	return stats, nil
}

// populateL1 populates the L1 cache with a value from L2
func (c *CacheAggregator) populateL1(ctx context.Context, key string, cacheVal *CacheValue, ttl time.Duration) {
	if c.l1 == nil {
		return
	}

	ttlRemaining := time.Until(cacheVal.ExpiresAt)
	if ttlRemaining > 0 {
		c.l1.Set(ctx, key, cacheVal, ttlRemaining)
	}
}

// updateStats updates cache statistics
func (c *CacheAggregator) updateStats(layer string, hit bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if hit {
		c.stats.TotalHits++
	} else {
		c.stats.TotalMisses++
	}

	c.stats.LayerStats[layer]++
	c.stats.LastUpdated = time.Now()
}

// extractChainFromKey extracts the chain name from a cache key
func extractChainFromKey(key string) string {
	// Keys are formatted as "rpc:chain:method:params" or specific formats like "balance:chain:address"
	parts := splitKey(key)
	if len(parts) >= 2 {
		return parts[1]
	}
	return ""
}

// extractMethodFromKey extracts the method from a cache key
func extractMethodFromKey(key string) string {
	parts := splitKey(key)
	if len(parts) >= 3 {
		return parts[2]
	}
	return ""
}

func splitKey(key string) []string {
	var parts []string
	start := 0
	for i, ch := range key {
		if ch == ':' {
			parts = append(parts, key[start:i])
			start = i + 1
		}
	}
	if start < len(key) {
		parts = append(parts, key[start:])
	}
	return parts
}
