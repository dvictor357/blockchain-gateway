package resilience

import (
	"context"
	"time"

	"github.com/redis/go-redis/v9"
)

// ResilienceLibraryConfig provides minimal configuration for resilience library usage
type ResilienceLibraryConfig struct {
	// Circuit breaker configuration
	FailureThreshold int
	RecoveryTimeout  time.Duration
	SuccessThreshold int
	Timeout          time.Duration

	// Rate limiter configuration
	RateLimitRequests int
	RateLimitWindow   time.Duration

	// Retry configuration
	MaxRetries    int
	InitialDelay  time.Duration
	MaxDelay      time.Duration
	BackoffFactor float64
}

// ResilienceLibraryOption represents a configuration option for resilience library usage
type ResilienceLibraryOption func(*ResilienceLibraryConfig)

// WithCircuitBreakerSettings configures circuit breaker settings
func WithCircuitBreakerSettings(failureThreshold int, recoveryTimeout, timeout time.Duration) ResilienceLibraryOption {
	return func(c *ResilienceLibraryConfig) {
		c.FailureThreshold = failureThreshold
		c.RecoveryTimeout = recoveryTimeout
		c.Timeout = timeout
		c.SuccessThreshold = 3 // Default success threshold
	}
}

// WithRateLimit configures rate limiting settings
func WithRateLimit(requests int, window time.Duration) ResilienceLibraryOption {
	return func(c *ResilienceLibraryConfig) {
		c.RateLimitRequests = requests
		c.RateLimitWindow = window
	}
}

// WithRetry configures retry settings
func WithRetry(maxRetries int, initialDelay, maxDelay time.Duration, backoffFactor float64) ResilienceLibraryOption {
	return func(c *ResilienceLibraryConfig) {
		c.MaxRetries = maxRetries
		c.InitialDelay = initialDelay
		c.MaxDelay = maxDelay
		c.BackoffFactor = backoffFactor
	}
}

// defaultResilienceLibraryConfig returns default configuration for resilience library usage
func defaultResilienceLibraryConfig() *ResilienceLibraryConfig {
	return &ResilienceLibraryConfig{
		FailureThreshold:  5,
		RecoveryTimeout:   60 * time.Second,
		SuccessThreshold:  3,
		Timeout:           30 * time.Second,
		RateLimitRequests: 100,
		RateLimitWindow:   time.Minute,
		MaxRetries:        3,
		InitialDelay:      100 * time.Millisecond,
		MaxDelay:          5 * time.Second,
		BackoffFactor:     2.0,
	}
}

// NewCircuitBreakerLibrary creates a new circuit breaker for library usage
func NewCircuitBreakerLibrary(opts ...ResilienceLibraryOption) *CircuitBreaker {
	config := defaultResilienceLibraryConfig()
	for _, opt := range opts {
		opt(config)
	}

	cbConfig := CircuitBreakerConfig{
		FailureThreshold: config.FailureThreshold,
		RecoveryTimeout:  config.RecoveryTimeout,
		SuccessThreshold: config.SuccessThreshold,
		Timeout:          config.Timeout,
		MonitoreInterval: 10 * time.Second,
	}

	return NewCircuitBreaker(cbConfig)
}

// NewCircuitBreakerPoolLibrary creates a new circuit breaker pool for library usage
func NewCircuitBreakerPoolLibrary(opts ...ResilienceLibraryOption) *CircuitBreakerPool {
	config := defaultResilienceLibraryConfig()
	for _, opt := range opts {
		opt(config)
	}

	cbConfig := CircuitBreakerConfig{
		FailureThreshold: config.FailureThreshold,
		RecoveryTimeout:  config.RecoveryTimeout,
		SuccessThreshold: config.SuccessThreshold,
		Timeout:          config.Timeout,
		MonitoreInterval: 10 * time.Second,
	}

	return NewCircuitBreakerPool(cbConfig)
}

// NewRedisRateLimiterLibrary creates a new Redis-based rate limiter for library usage
func NewRedisRateLimiterLibrary(client *redis.Client, opts ...ResilienceLibraryOption) *RedisRateLimiter {
	config := defaultResilienceLibraryConfig()
	for _, opt := range opts {
		opt(config)
	}

	rlConfig := RateLimitConfig{
		Requests: config.RateLimitRequests,
		Window:   config.RateLimitWindow,
		Burst:    config.RateLimitRequests / 4, // 25% of limit as burst
	}

	return NewRedisRateLimiter(client, rlConfig)
}

// NewSlidingWindowRateLimiterLibrary creates a new sliding window rate limiter for library usage
func NewSlidingWindowRateLimiterLibrary(client *redis.Client, opts ...ResilienceLibraryOption) *SlidingWindowRateLimiter {
	config := defaultResilienceLibraryConfig()
	for _, opt := range opts {
		opt(config)
	}

	rlConfig := RateLimitConfig{
		Requests: config.RateLimitRequests,
		Window:   config.RateLimitWindow,
		Burst:    config.RateLimitRequests / 4,
	}

	return NewSlidingWindowRateLimiter(client, rlConfig)
}

// NewFixedWindowRateLimiterLibrary creates a new fixed window rate limiter for library usage
func NewFixedWindowRateLimiterLibrary(client *redis.Client, opts ...ResilienceLibraryOption) *FixedWindowRateLimiter {
	config := defaultResilienceLibraryConfig()
	for _, opt := range opts {
		opt(config)
	}

	rlConfig := RateLimitConfig{
		Requests: config.RateLimitRequests,
		Window:   config.RateLimitWindow,
		Burst:    config.RateLimitRequests / 4,
	}

	return NewFixedWindowRateLimiter(client, rlConfig)
}

// SimpleResilienceManager provides a simplified interface for common resilience patterns
type SimpleResilienceManager struct {
	circuitBreakerPool *CircuitBreakerPool
	rateLimiter        *RedisRateLimiter
	retryConfig        RetryConfig
}

// NewSimpleResilienceManager creates a simple resilience manager with sensible defaults
func NewSimpleResilienceManager(redisClient *redis.Client, opts ...ResilienceLibraryOption) *SimpleResilienceManager {
	config := defaultResilienceLibraryConfig()
	for _, opt := range opts {
		opt(config)
	}

	manager := &SimpleResilienceManager{
		circuitBreakerPool: NewCircuitBreakerPoolLibrary(opts...),
		rateLimiter:        NewRedisRateLimiterLibrary(redisClient, opts...),
		retryConfig: RetryConfig{
			MaxRetries:        config.MaxRetries,
			InitialDelay:      config.InitialDelay,
			MaxDelay:          config.MaxDelay,
			BackoffMultiplier: config.BackoffFactor,
			RetryableErrors:   []string{"timeout", "circuit open"},
		},
	}

	return manager
}

// ExecuteWithProtection executes an operation with all resilience protections
func (srm *SimpleResilienceManager) ExecuteWithProtection(ctx context.Context, key string, operation func() (interface{}, error)) (interface{}, error) {
	// Apply retry logic
	return RetryWithResult(ctx, srm.retryConfig, func() (interface{}, error) {
		// Apply circuit breaker protection
		return srm.circuitBreakerPool.Execute(key, operation)
	})
}

// CheckRateLimit checks if a request is allowed under the rate limit
func (srm *SimpleResilienceManager) CheckRateLimit(ctx context.Context, key string) (bool, error) {
	return srm.rateLimiter.Allow(ctx, key)
}

// GetRemainingRateLimit returns the number of remaining requests
func (srm *SimpleResilienceManager) GetRemainingRateLimit(ctx context.Context, key string) (int, error) {
	return srm.rateLimiter.GetRemaining(ctx, key)
}

// GetCircuitBreakerStats returns circuit breaker statistics
func (srm *SimpleResilienceManager) GetCircuitBreakerStats() map[string]map[string]interface{} {
	return srm.circuitBreakerPool.GetStats()
}

// ResetCircuitBreaker resets a specific circuit breaker
func (srm *SimpleResilienceManager) ResetCircuitBreaker(key string) {
	if cb := srm.circuitBreakerPool.GetOrCreate(key); cb != nil {
		cb.Reset()
	}
}

// ResetAllCircuitBreakers resets all circuit breakers
func (srm *SimpleResilienceManager) ResetAllCircuitBreakers() {
	// This would require adding a ResetAll method to CircuitBreakerPool
	// For now, users can create a new manager if needed
}
