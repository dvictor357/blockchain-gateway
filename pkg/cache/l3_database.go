package cache

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	_ "github.com/lib/pq" // PostgreSQL driver
)

// DBCache is an L3 database-based cache with TTL support
type DBCache struct {
	db         *sql.DB
	stats      CacheStats
	tableName  string
	cleanupInt time.Duration
}

// NewDBCache creates a new L3 database cache
func NewDBCache(db *sql.DB) (*DBCache, error) {
	// Create the cache table if it doesn't exist
	if err := createCacheTable(db); err != nil {
		return nil, fmt.Errorf("failed to create cache table: %w", err)
	}

	cache := &DBCache{
		db:         db,
		stats:      CacheStats{},
		tableName:  "rpc_cache",
		cleanupInt: 1 * time.Hour, // Cleanup expired entries every hour
	}

	return cache, nil
}

// Get retrieves a value from the cache
func (c *DBCache) Get(ctx context.Context, key string) (*CacheValue, error) {
	query := `
		SELECT data, created_at, expires_at, hits, chain, method
		FROM rpc_cache
		WHERE cache_key = $1 AND expires_at > NOW()
	`

	var data []byte
	var createdAt, expiresAt time.Time
	var hits int64
	var chain, method string

	err := c.db.QueryRowContext(ctx, query, key).Scan(&data, &createdAt, &expiresAt, &hits, &chain, &method)
	if err != nil {
		if err == sql.ErrNoRows {
			c.updateStats(false)
			return nil, ErrCacheMiss
		}
		return nil, fmt.Errorf("database query error: %w", err)
	}

	var cacheVal CacheValue
	if err := json.Unmarshal(data, &cacheVal); err != nil {
		return nil, fmt.Errorf("failed to unmarshal cached value: %w", err)
	}

	// Update the value with metadata from DB
	cacheVal.CreatedAt = createdAt
	cacheVal.ExpiresAt = expiresAt
	cacheVal.Hits = hits
	cacheVal.Chain = chain
	cacheVal.Method = method

	// Check if the value has expired
	if time.Now().After(expiresAt) {
		c.Delete(ctx, key)
		c.updateStats(false)
		return nil, ErrCacheExpired
	}

	// Update hit count in database
	go c.incrementHit(ctx, key)

	// Update hit statistics
	c.updateStats(true)

	return &cacheVal, nil
}

// Set stores a value in the cache
func (c *DBCache) Set(ctx context.Context, key string, value *CacheValue, ttl time.Duration) error {
	data, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("failed to marshal cached value: %w", err)
	}

	expiresAt := time.Now().Add(ttl)

	query := `
		INSERT INTO rpc_cache (cache_key, data, created_at, expires_at, hits, chain, method)
		VALUES ($1, $2, NOW(), $3, $4, $5, $6)
		ON CONFLICT (cache_key) DO UPDATE
		SET data = $2, created_at = NOW(), expires_at = $3, hits = $4, chain = $5, method = $6
	`

	_, err = c.db.ExecContext(ctx, query, key, data, expiresAt, value.Hits, value.Chain, value.Method)
	if err != nil {
		return fmt.Errorf("database insert error: %w", err)
	}

	return nil
}

// Delete removes a value from the cache
func (c *DBCache) Delete(ctx context.Context, key string) error {
	query := "DELETE FROM rpc_cache WHERE cache_key = $1"
	_, err := c.db.ExecContext(ctx, query, key)
	if err != nil {
		return fmt.Errorf("database delete error: %w", err)
	}

	return nil
}

// Clear removes all values from the cache
func (c *DBCache) Clear(ctx context.Context) error {
	query := "DELETE FROM rpc_cache"
	_, err := c.db.ExecContext(ctx, query)
	if err != nil {
		return fmt.Errorf("database clear error: %w", err)
	}

	return nil
}

// Name returns the name of the cache layer
func (c *DBCache) Name() string {
	return "L3-Database"
}

// GetStats returns statistics about the cache
func (c *DBCache) GetStats(ctx context.Context) (map[string]interface{}, error) {
	// Get total count
	var count int64
	err := c.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM rpc_cache WHERE expires_at > NOW()").Scan(&count)
	if err != nil {
		return nil, fmt.Errorf("failed to get count: %w", err)
	}

	// Get total size
	var size int64
	err = c.db.QueryRowContext(ctx, "SELECT COALESCE(SUM(LENGTH(data)), 0) FROM rpc_cache WHERE expires_at > NOW()").Scan(&size)
	if err != nil {
		return nil, fmt.Errorf("failed to get size: %w", err)
	}

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
		"items":        count,
		"bytes_used":   size,
		"last_updated": c.stats.LastUpdated,
	}, nil
}

// incrementHit updates the hit count for a cache entry
func (c *DBCache) incrementHit(ctx context.Context, key string) {
	query := "UPDATE rpc_cache SET hits = hits + 1 WHERE cache_key = $1"
	c.db.ExecContext(ctx, query, key)
}

// StartCleanup starts a background goroutine to clean up expired entries
func (c *DBCache) StartCleanup() {
	go func() {
		ticker := time.NewTicker(c.cleanupInt)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				c.cleanupExpired()
			case <-context.Background().Done():
				return
			}
		}
	}()
}

// cleanupExpired removes all expired entries from the cache
func (c *DBCache) cleanupExpired() {
	query := "DELETE FROM rpc_cache WHERE expires_at <= NOW()"
	_, err := c.db.ExecContext(context.Background(), query)
	if err != nil {
		// Log error but don't fail
		fmt.Printf("Failed to cleanup expired cache entries: %v\n", err)
	}
}

// updateStats updates cache statistics
func (c *DBCache) updateStats(hit bool) {
	c.stats.LastUpdated = time.Now()
	if hit {
		c.stats.Hits++
	} else {
		c.stats.Misses++
	}
}

// createCacheTable creates the cache table if it doesn't exist
func createCacheTable(db *sql.DB) error {
	query := `
		CREATE TABLE IF NOT EXISTS rpc_cache (
			cache_key VARCHAR(255) PRIMARY KEY,
			data BYTEA NOT NULL,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			expires_at TIMESTAMPTZ NOT NULL,
			hits BIGINT NOT NULL DEFAULT 0,
			chain VARCHAR(50),
			method VARCHAR(100)
		);

		CREATE INDEX IF NOT EXISTS idx_rpc_cache_expires_at ON rpc_cache(expires_at);
		CREATE INDEX IF NOT EXISTS idx_rpc_cache_chain_method ON rpc_cache(chain, method);
		CREATE INDEX IF NOT EXISTS idx_rpc_cache_hits ON rpc_cache(hits DESC);
	`

	_, err := db.Exec(query)
	if err != nil {
		return fmt.Errorf("failed to create cache table: %w", err)
	}

	return nil
}
