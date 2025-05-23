package config

import (
	"time"
)

// APIConfig holds API-specific configuration
type APIConfig struct {
	DefaultTimeout       time.Duration
	BatchTimeout         time.Duration
	MaxBatchSize         int
	DefaultPageLimit     int
	MaxPageLimit         int
	AllowedOrderByFields []string
}

// NewAPIConfig creates a new API configuration with sensible defaults
func NewAPIConfig() *APIConfig {
	return &APIConfig{
		DefaultTimeout:   30 * time.Second,
		BatchTimeout:     60 * time.Second,
		MaxBatchSize:     10,
		DefaultPageLimit: 20,
		MaxPageLimit:     100,
		AllowedOrderByFields: []string{
			"market_cap_rank",
			"name",
			"current_price",
			"last_updated",
			"data_fetched_at",
		},
	}
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
