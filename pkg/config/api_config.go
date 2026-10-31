package config

import (
	"time"
)

// APIConfig holds API-specific configuration
type APIConfig struct {
	DefaultTimeout time.Duration
	BatchTimeout   time.Duration
	// MaxBatchSize is the maximum number of RPC requests in one batch, summed across all chains.
	MaxBatchSize int
	// MaxRequestBodyBytes is the maximum size of a JSON request body.
	MaxRequestBodyBytes  int64
	DefaultPageLimit     int
	MaxPageLimit         int
	AllowedOrderByFields []string
}

// NewAPIConfig creates a new API configuration with sensible defaults
func NewAPIConfig() *APIConfig {
	return &APIConfig{
		DefaultTimeout:      30 * time.Second,
		BatchTimeout:        60 * time.Second,
		MaxBatchSize:        10,
		MaxRequestBodyBytes: 1 << 20,
		DefaultPageLimit:    20,
		MaxPageLimit:        100,
		AllowedOrderByFields: []string{
			"market_cap_rank",
			"name",
			"current_price",
			"last_updated",
			"data_fetched_at",
		},
	}
}

// NewAPIConfigFromServer derives request budgets from the server's REQUEST_TIMEOUT:
// ordinary requests get the timeout itself and batches get twice that.
func NewAPIConfigFromServer(server ServerConfig) *APIConfig {
	cfg := NewAPIConfig()
	if server.RequestTimeout > 0 {
		cfg.DefaultTimeout = server.RequestTimeout
		cfg.BatchTimeout = 2 * server.RequestTimeout
	}
	return cfg
}

// LongestTimeout returns the largest request budget, which bounds how long a handler can run.
func (c *APIConfig) LongestTimeout() time.Duration {
	return max(c.DefaultTimeout, c.BatchTimeout)
}

// GetTimeout returns the appropriate timeout for the operation type
func (c *APIConfig) GetTimeout(operationType string) time.Duration {
	switch operationType {
	case "batch":
		return c.BatchTimeout
	default:
		return c.DefaultTimeout
	}
}

// ValidatePageLimit ensures the page limit is within bounds
func (c *APIConfig) ValidatePageLimit(limit int) int {
	if limit <= 0 {
		return c.DefaultPageLimit
	}
	if limit > c.MaxPageLimit {
		return c.MaxPageLimit
	}
	return limit
}

// IsValidOrderByField checks if the field is allowed for ordering
func (c *APIConfig) IsValidOrderByField(field string) bool {
	for _, allowed := range c.AllowedOrderByFields {
		if field == allowed {
			return true
		}
	}
	return false
}
