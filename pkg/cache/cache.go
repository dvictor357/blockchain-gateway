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

// CacheKeyGenerator generates cache keys based on request parameters
type CacheKeyGenerator struct{}

// GenerateRPCKey generates a cache key for RPC requests
func (kg *CacheKeyGenerator) GenerateRPCKey(chain, method string, params any) (string, error) {
	paramsBytes, err := json.Marshal(params)
	if err != nil {
		return "", fmt.Errorf("failed to marshal params: %w", err)
	}

	// Create a stable key from chain, method, and params
	key := fmt.Sprintf("rpc:%s:%s:%x", chain, method, paramsBytes)
	return key, nil
}

// GenerateBalanceKey generates a cache key for balance requests
func (kg *CacheKeyGenerator) GenerateBalanceKey(chain, address string) string {
	return fmt.Sprintf("balance:%s:%s", chain, address)
}

// GenerateBlockKey generates a cache key for block requests
func (kg *CacheKeyGenerator) GenerateBlockKey(chain string) string {
	return fmt.Sprintf("block:latest:%s", chain)
}

// GenerateTxKey generates a cache key for transaction requests
func (kg *CacheKeyGenerator) GenerateTxKey(chain, hash string) string {
	return fmt.Sprintf("tx:%s:%s", chain, hash)
}

// GenerateGasPriceKey generates a cache key for gas price requests
func (kg *CacheKeyGenerator) GenerateGasPriceKey(chain string) string {
	return fmt.Sprintf("gas_price:%s", chain)
}

// GenerateNonceKey generates a cache key for nonce requests
func (kg *CacheKeyGenerator) GenerateNonceKey(chain, address string) string {
	return fmt.Sprintf("nonce:%s:%s", chain, address)
}

// TTL Constants for different cache layers
const (
	// L1 (In-Memory) TTLs - Short duration for hot data
	L1BalanceTTL  = 30 * time.Second
	L1BlockTTL    = 30 * time.Second
	L1GasPriceTTL = 30 * time.Second
	L1NonceTTL    = 30 * time.Second
	L1TxTTL       = 30 * time.Second
	L1RPCTTL      = 30 * time.Second

	// L2 (Redis) TTLs - Medium duration for frequent data
	L2BalanceTTL  = 5 * time.Minute
	L2BlockTTL    = 5 * time.Minute
	L2GasPriceTTL = 5 * time.Minute
	L2NonceTTL    = 5 * time.Minute
	L2TxTTL       = 5 * time.Minute
	L2RPCTTL      = 5 * time.Minute

	// L3 (Database) TTLs - Long duration for historical data
	L3BalanceTTL  = 1 * time.Hour
	L3BlockTTL    = 1 * time.Hour
	L3GasPriceTTL = 1 * time.Hour
	L3NonceTTL    = 1 * time.Hour
	L3TxTTL       = 1 * time.Hour
	L3RPCTTL      = 1 * time.Hour
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
