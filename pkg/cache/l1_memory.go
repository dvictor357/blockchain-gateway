package cache

import (
	"context"
	"encoding/json"
	"sync"
	"time"
)

// MemoryCache is an L1 in-memory cache with TTL support
type MemoryCache struct {
	mu       sync.RWMutex
	items    map[string]*CacheItem
	stats    CacheStats
	maxItems int
}

// CacheItem wraps a CacheValue with an expiry time
type CacheItem struct {
	Value     *CacheValue
	ExpiresAt time.Time
}

// NewMemoryCache creates a new L1 in-memory cache
func NewMemoryCache(maxItems int) *MemoryCache {
	return &MemoryCache{
		items:    make(map[string]*CacheItem),
		maxItems: maxItems,
		stats: CacheStats{
			Hits:        0,
			Misses:      0,
			HitRatio:    0,
			Items:       0,
			BytesUsed:   0,
			LastUpdated: time.Now(),
		},
	}
}

// Get retrieves a value from the cache
func (c *MemoryCache) Get(ctx context.Context, key string) (*CacheValue, error) {
	c.mu.RLock()
	item, exists := c.items[key]
	c.mu.RUnlock()

	if !exists {
		c.mu.Lock()
		c.stats.Misses++
		c.stats.LastUpdated = time.Now()
		c.mu.Unlock()
		return nil, ErrCacheMiss
	}

	// Check if item has expired
	if time.Now().After(item.ExpiresAt) {
		c.mu.Lock()
		delete(c.items, key)
		c.stats.Items--
		c.stats.Misses++
		c.stats.LastUpdated = time.Now()
		c.mu.Unlock()
		return nil, ErrCacheExpired
	}

	// Update hit statistics
	c.mu.Lock()
	c.stats.Hits++
	c.stats.LastUpdated = time.Now()
	item.Value.Hits++
	c.mu.Unlock()

	return item.Value, nil
}

// Set stores a value in the cache
func (c *MemoryCache) Set(ctx context.Context, key string, value *CacheValue, ttl time.Duration) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Calculate memory usage
	valueBytes, _ := json.Marshal(value)
	bytesUsed := int64(len(valueBytes))

	// Check if we need to evict items
	if len(c.items) >= c.maxItems && c.items[key] == nil {
		c.evictOldest()
	}

	c.items[key] = &CacheItem{
		Value:     value,
		ExpiresAt: time.Now().Add(ttl),
	}

	c.stats.Items = len(c.items)
	c.stats.BytesUsed += bytesUsed
	c.stats.LastUpdated = time.Now()

	return nil
}

// Delete removes a value from the cache
func (c *MemoryCache) Delete(ctx context.Context, key string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if _, exists := c.items[key]; exists {
		delete(c.items, key)
		c.stats.Items = len(c.items)
		c.stats.LastUpdated = time.Now()
	}

	return nil
}

// Clear removes all values from the cache
func (c *MemoryCache) Clear(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.items = make(map[string]*CacheItem)
	c.stats.Items = 0
	c.stats.BytesUsed = 0
	c.stats.LastUpdated = time.Now()

	return nil
}

// Name returns the name of the cache layer
func (c *MemoryCache) Name() string {
	return "L1-Memory"
}

// GetStats returns statistics about the cache
func (c *MemoryCache) GetStats(ctx context.Context) (map[string]interface{}, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	totalRequests := c.stats.Hits + c.stats.Misses
	var hitRatio float64
	if totalRequests > 0 {
		hitRatio = float64(c.stats.Hits) / float64(totalRequests)
	}

	return map[string]interface{}{
		"name":         c.Name(),
		"hits":         c.stats.Hits,
		"misses":       c.stats.Misses,
		"hit_ratio":    hitRatio,
		"items":        c.stats.Items,
		"bytes_used":   c.stats.BytesUsed,
		"max_items":    c.maxItems,
		"last_updated": c.stats.LastUpdated,
	}, nil
}

// evictOldest removes the oldest item from the cache
func (c *MemoryCache) evictOldest() {
	if len(c.items) == 0 {
		return
	}

	var oldestKey string
	var oldestTime time.Time

	for key, item := range c.items {
		if oldestKey == "" || item.ExpiresAt.Before(oldestTime) {
			oldestKey = key
			oldestTime = item.ExpiresAt
		}
	}

	if oldestKey != "" {
		delete(c.items, oldestKey)
	}
}

// PeriodicCleanup removes expired items from the cache
func (c *MemoryCache) PeriodicCleanup(interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			c.cleanupExpired()
		case <-context.Background().Done():
			return
		}
	}
}

// cleanupExpired removes all expired items from the cache
func (c *MemoryCache) cleanupExpired() {
	now := time.Now()
	c.mu.Lock()
	defer c.mu.Unlock()

	expired := 0
	for key, item := range c.items {
		if now.After(item.ExpiresAt) {
			delete(c.items, key)
			expired++
		}
	}

	if expired > 0 {
		c.stats.Items = len(c.items)
		c.stats.LastUpdated = time.Now()
	}
}
