package cache

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/dvictor357/blockchain-gateway/pkg/config"
	redisCache "github.com/dvictor357/blockchain-gateway/pkg/rediscache"
	redisv9 "github.com/redis/go-redis/v9"
)

// RedisCache is an L2 Redis-based cache with TTL support
type RedisCache struct {
	client *redisv9.Client
	stats  CacheStats
	ctx    context.Context
}

// NewRedisCache creates a new L2 Redis cache
func NewRedisCache(redisConfig config.RedisConfig) (*RedisCache, error) {
	if !redisConfig.Enabled {
		return nil, fmt.Errorf("redis is not enabled in configuration")
	}

	client := redisCache.NewClient(redisConfig)
	ctx := context.Background()

	// Test the connection
	if err := redisCache.Ping(ctx, client); err != nil {
		return nil, fmt.Errorf("failed to connect to Redis: %w", err)
	}

	return &RedisCache{
		client: client,
		stats: CacheStats{
			Hits:        0,
			Misses:      0,
			HitRatio:    0,
			Items:       0,
			BytesUsed:   0,
			LastUpdated: time.Now(),
		},
		ctx: ctx,
	}, nil
}

// Get retrieves a value from the cache
func (c *RedisCache) Get(ctx context.Context, key string) (*CacheValue, error) {
	val, err := c.client.Get(c.ctx, key).Result()
	if err != nil {
		if err == redisv9.Nil {
			c.updateStats(false)
			return nil, ErrCacheMiss
		}
		return nil, fmt.Errorf("redis get error: %w", err)
	}

	// Deserialize the value
	var cacheVal CacheValue
	if err := json.Unmarshal([]byte(val), &cacheVal); err != nil {
		return nil, fmt.Errorf("failed to unmarshal cached value: %w", err)
	}

	// Check if the value has expired
	if time.Now().After(cacheVal.ExpiresAt) {
		c.client.Del(c.ctx, key)
		c.updateStats(false)
		return nil, ErrCacheExpired
	}

	// Update hit statistics
	c.updateStats(true)
	cacheVal.Hits++

	return &cacheVal, nil
}

// Set stores a value in the cache
func (c *RedisCache) Set(ctx context.Context, key string, value *CacheValue, ttl time.Duration) error {
	val, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("failed to marshal cached value: %w", err)
	}

	// Set with TTL in Redis
	if err := c.client.Set(c.ctx, key, val, ttl).Err(); err != nil {
		return fmt.Errorf("redis set error: %w", err)
	}

	return nil
}

// Delete removes a value from the cache
func (c *RedisCache) Delete(ctx context.Context, key string) error {
	err := c.client.Del(c.ctx, key).Err()
	if err != nil {
		return fmt.Errorf("redis delete error: %w", err)
	}
	return nil
}

// Clear removes all values from the cache
func (c *RedisCache) Clear(ctx context.Context) error {
	// Get all keys with a pattern
	pattern := "*"
	iter := c.client.Scan(c.ctx, 0, pattern, 100).Iterator()

	var keys []string
	for iter.Next(c.ctx) {
		keys = append(keys, iter.Val())
	}

	if len(keys) > 0 {
		err := c.client.Del(c.ctx, keys...).Err()
		if err != nil {
			return fmt.Errorf("redis clear error: %w", err)
		}
	}

	return nil
}

// Name returns the name of the cache layer
func (c *RedisCache) Name() string {
	return "L2-Redis"
}

// GetStats returns statistics about the cache
func (c *RedisCache) GetStats(ctx context.Context) (map[string]interface{}, error) {
	// Get Redis info
	info, err := c.client.Info(c.ctx).Result()
	if err != nil {
		return nil, fmt.Errorf("failed to get Redis info: %w", err)
	}

	// Parse memory usage
	var memoryUsed int64
	var keyCount int64

	// Extract memory_used_bytes
	if val := parseRedisInfo(info, "used_memory_bytes"); val != "" {
		fmt.Sscanf(val, "%d", &memoryUsed)
	}

	// Extract keyspace info
	if keyspace := parseRedisInfo(info, "db0"); keyspace != "" {
		fmt.Sscanf(keyspace, "keys=%d", &keyCount)
	}

	totalRequests := c.stats.Hits + c.stats.Misses
	var hitRatio float64
	if totalRequests > 0 {
		hitRatio = float64(c.stats.Hits) / float64(totalRequests)
	}

	return map[string]interface{}{
		"name":              c.Name(),
		"hits":              c.stats.Hits,
		"misses":            c.stats.Misses,
		"hit_ratio":         hitRatio,
		"items":             keyCount,
		"bytes_used":        memoryUsed,
		"last_updated":      c.stats.LastUpdated,
		"redis_version":     parseRedisInfo(info, "redis_version"),
		"connected_clients": parseRedisInfo(info, "connected_clients"),
	}, nil
}

// parseRedisInfo extracts a specific value from Redis INFO output
func parseRedisInfo(info, key string) string {
	lines := splitLines(info)
	for _, line := range lines {
		if len(line) > 0 && line[0] != '#' {
			if idx := findKeyInLine(line, key); idx != -1 {
				if value := extractValue(line, idx); value != "" {
					return value
				}
			}
		}
	}
	return ""
}

func splitLines(info string) []string {
	var lines []string
	start := 0
	for i, ch := range info {
		if ch == '\n' {
			lines = append(lines, info[start:i])
			start = i + 1
		}
	}
	if start < len(info) {
		lines = append(lines, info[start:])
	}
	return lines
}

func findKeyInLine(line, key string) int {
	if len(line) < len(key) {
		return -1
	}

	for i := 0; i <= len(line)-len(key); i++ {
		if line[i:i+len(key)] == key {
			return i
		}
	}
	return -1
}

func extractValue(line string, keyIdx int) string {
	// Find the colon after the key
	colonIdx := -1
	for i := keyIdx; i < len(line); i++ {
		if line[i] == ':' {
			colonIdx = i
			break
		}
	}

	if colonIdx == -1 {
		return ""
	}

	// Extract value until comma or end of line
	valueStart := colonIdx + 1
	valueEnd := len(line)
	for i := valueStart; i < len(line); i++ {
		if line[i] == ',' || line[i] == '\r' {
			valueEnd = i
			break
		}
	}

	return line[valueStart:valueEnd]
}

// updateStats updates cache statistics
func (c *RedisCache) updateStats(hit bool) {
	// This is a simplified stats tracking
	// In a production system, you might want to use Redis itself for tracking
	c.stats.LastUpdated = time.Now()
}

// Close closes the Redis connection
func (c *RedisCache) Close() error {
	return c.client.Close()
}
