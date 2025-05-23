package config

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestNewAPIConfig(t *testing.T) {
	config := NewAPIConfig()

	assert.NotNil(t, config)
	assert.Equal(t, 30*time.Second, config.DefaultTimeout)
	assert.Equal(t, 60*time.Second, config.BatchTimeout)
	assert.Equal(t, 10, config.MaxBatchSize)
	assert.Equal(t, 20, config.DefaultPageLimit)
	assert.Equal(t, 100, config.MaxPageLimit)

	expectedFields := []string{
		"market_cap_rank",
		"name",
		"current_price",
		"last_updated",
		"data_fetched_at",
	}
	assert.Equal(t, expectedFields, config.AllowedOrderByFields)
}

func TestAPIConfig_GetTimeout(t *testing.T) {
	config := NewAPIConfig()

	tests := []struct {
		name          string
		operationType string
		expected      time.Duration
	}{
		{
			name:          "batch operation timeout",
			operationType: "batch",
			expected:      60 * time.Second,
		},
		{
			name:          "default operation timeout",
			operationType: "default",
			expected:      30 * time.Second,
		},
		{
			name:          "unknown operation timeout (should return default)",
			operationType: "unknown",
			expected:      30 * time.Second,
		},
		{
			name:          "empty operation type (should return default)",
			operationType: "",
			expected:      30 * time.Second,
		},
		{
			name:          "custom operation type (should return default)",
			operationType: "custom_operation",
			expected:      30 * time.Second,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := config.GetTimeout(tt.operationType)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestAPIConfig_ValidatePageLimit(t *testing.T) {
	config := NewAPIConfig()

	tests := []struct {
		name     string
		limit    int
		expected int
	}{
		{
			name:     "valid limit within bounds",
			limit:    50,
			expected: 50,
		},
		{
			name:     "limit at default",
			limit:    20,
			expected: 20,
		},
		{
			name:     "limit at maximum",
			limit:    100,
			expected: 100,
		},
		{
			name:     "limit too small (should return default)",
			limit:    0,
			expected: 20,
		},
		{
			name:     "negative limit (should return default)",
			limit:    -5,
			expected: 20,
		},
		{
			name:     "limit too large (should return maximum)",
			limit:    150,
			expected: 100,
		},
		{
			name:     "limit way too large (should return maximum)",
			limit:    1000,
			expected: 100,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := config.ValidatePageLimit(tt.limit)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestAPIConfig_IsValidOrderByField(t *testing.T) {
	config := NewAPIConfig()

	tests := []struct {
		name     string
		field    string
		expected bool
	}{
		{
			name:     "valid field - market_cap_rank",
			field:    "market_cap_rank",
			expected: true,
		},
		{
			name:     "valid field - name",
			field:    "name",
			expected: true,
		},
		{
			name:     "valid field - current_price",
			field:    "current_price",
			expected: true,
		},
		{
			name:     "valid field - last_updated",
			field:    "last_updated",
			expected: true,
		},
		{
			name:     "valid field - data_fetched_at",
			field:    "data_fetched_at",
			expected: true,
		},
		{
			name:     "invalid field",
			field:    "invalid_field",
			expected: false,
		},
		{
			name:     "empty field",
			field:    "",
			expected: false,
		},
		{
			name:     "case sensitive - should be false for wrong case",
			field:    "MARKET_CAP_RANK",
			expected: false,
		},
		{
			name:     "partial match - should be false",
			field:    "market_cap",
			expected: false,
		},
		{
			name:     "field with extra characters",
			field:    "market_cap_rank_extra",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := config.IsValidOrderByField(tt.field)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestAPIConfig_DefaultValues(t *testing.T) {
	// Test that the default values are reasonable
	config := NewAPIConfig()

	// Timeout values should be positive
	assert.Greater(t, config.DefaultTimeout, time.Duration(0))
	assert.Greater(t, config.BatchTimeout, time.Duration(0))

	// Batch timeout should be longer than default timeout
	assert.Greater(t, config.BatchTimeout, config.DefaultTimeout)

	// Batch size should be reasonable
	assert.Greater(t, config.MaxBatchSize, 0)
	assert.LessOrEqual(t, config.MaxBatchSize, 100) // Reasonable upper bound

	// Page limits should be reasonable
	assert.Greater(t, config.DefaultPageLimit, 0)
	assert.Greater(t, config.MaxPageLimit, config.DefaultPageLimit)
	assert.LessOrEqual(t, config.MaxPageLimit, 1000) // Reasonable upper bound

	// Should have at least one allowed order by field
	assert.Greater(t, len(config.AllowedOrderByFields), 0)
}

func TestAPIConfig_Immutability(t *testing.T) {
	// Test that modifying the returned slice doesn't affect the original config
	config := NewAPIConfig()
	originalFields := config.AllowedOrderByFields
	originalLength := len(originalFields)

	// Try to modify the returned slice
	modifiedFields := config.AllowedOrderByFields
	modifiedFields = append(modifiedFields, "new_field")

	// Original config should be unchanged
	assert.Equal(t, originalLength, len(config.AllowedOrderByFields))
	assert.NotContains(t, config.AllowedOrderByFields, "new_field")
}

func TestAPIConfig_EdgeCases(t *testing.T) {
	config := NewAPIConfig()

	// Test edge cases for ValidatePageLimit
	assert.Equal(t, config.DefaultPageLimit, config.ValidatePageLimit(-1))
	assert.Equal(t, config.DefaultPageLimit, config.ValidatePageLimit(0))
	assert.Equal(t, 1, config.ValidatePageLimit(1))
	assert.Equal(t, config.MaxPageLimit, config.ValidatePageLimit(config.MaxPageLimit))
	assert.Equal(t, config.MaxPageLimit, config.ValidatePageLimit(config.MaxPageLimit+1))

	// Test edge cases for IsValidOrderByField
	assert.False(t, config.IsValidOrderByField(""))
	assert.False(t, config.IsValidOrderByField(" "))
	assert.False(t, config.IsValidOrderByField("market_cap_rank ")) // trailing space
	assert.False(t, config.IsValidOrderByField(" market_cap_rank")) // leading space
}

func TestAPIConfig_ConfigurationConsistency(t *testing.T) {
	// Test that configuration values are consistent with each other
	config := NewAPIConfig()

	// Default page limit should be less than or equal to max page limit
	assert.LessOrEqual(t, config.DefaultPageLimit, config.MaxPageLimit)

	// Default timeout should be less than batch timeout (for most use cases)
	assert.LessOrEqual(t, config.DefaultTimeout, config.BatchTimeout)

	// Max batch size should be reasonable (not too large to cause memory issues)
	assert.LessOrEqual(t, config.MaxBatchSize, 50) // Conservative upper bound

	// All allowed order by fields should be non-empty strings
	for _, field := range config.AllowedOrderByFields {
		assert.NotEmpty(t, field)
		assert.NotContains(t, field, " ") // No spaces in field names
	}
}
