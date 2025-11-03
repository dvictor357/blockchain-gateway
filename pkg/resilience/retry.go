package resilience

import (
	"context"
	"fmt"
	"math"
	"math/rand"
	"time"
)

// RetryConfig holds configuration for retry logic
type RetryConfig struct {
	MaxRetries        int
	InitialDelay      time.Duration
	MaxDelay          time.Duration
	BackoffMultiplier float64
	MaxJitter         time.Duration
	RetryableErrors   []string // Errors that should trigger a retry
}

// DefaultRetryConfig returns default retry configuration
func DefaultRetryConfig() RetryConfig {
	return RetryConfig{
		MaxRetries:        3,
		InitialDelay:      1 * time.Second,
		MaxDelay:          30 * time.Second,
		BackoffMultiplier: 2.0,
		MaxJitter:         500 * time.Millisecond,
	}
}

// RetryableError represents an error that can be retried
type RetryableError struct {
	message string
	cause   error
}

// Error returns the error message
func (e RetryableError) Error() string {
	return e.message
}

// Unwrap returns the underlying error
func (e RetryableError) Unwrap() error {
	return e.cause
}

// NewRetryableError creates a new retryable error
func NewRetryableError(message string, cause error) error {
	return RetryableError{
		message: message,
		cause:   cause,
	}
}

// Retry function with exponential backoff and jitter
func Retry(ctx context.Context, config RetryConfig, operation func() error) error {
	var lastErr error
	delay := config.InitialDelay

	for attempt := 0; attempt <= config.MaxRetries; attempt++ {
		// Execute the operation
		err := operation()

		// Success, no retry needed
		if err == nil {
			if attempt > 0 {
				// Log successful retry (you might want to use a logger here)
			}
			return nil
		}

		lastErr = err

		// Check if this is the last attempt
		if attempt == config.MaxRetries {
			break
		}

		// Check if error is retryable
		if !isRetryableError(err, config.RetryableErrors) {
			break
		}

		// Check if context is done
		select {
		case <-ctx.Done():
			return fmt.Errorf("context cancelled during retry: %w", ctx.Err())
		default:
		}

		// Calculate delay with exponential backoff and jitter
		backoffDelay := time.Duration(float64(delay) * math.Pow(config.BackoffMultiplier, float64(attempt)))
		if backoffDelay > config.MaxDelay {
			backoffDelay = config.MaxDelay
		}

		// Add jitter to prevent thundering herd
		if config.MaxJitter > 0 {
			jitter := time.Duration(rand.Int63n(int64(config.MaxJitter)))
			backoffDelay += jitter
		}

		// Wait before retrying
		select {
		case <-ctx.Done():
			return fmt.Errorf("context cancelled during retry delay: %w", ctx.Err())
		case <-time.After(backoffDelay):
		}

		// Log retry attempt (you might want to use a logger here)
	}

	return fmt.Errorf("max retries (%d) exceeded, last error: %w", config.MaxRetries, lastErr)
}

// RetryWithResult retries an operation and returns the result
func RetryWithResult(ctx context.Context, config RetryConfig, operation func() (interface{}, error)) (interface{}, error) {
	var lastErr error
	delay := config.InitialDelay

	for attempt := 0; attempt <= config.MaxRetries; attempt++ {
		// Execute the operation
		result, err := operation()

		// Success, no retry needed
		if err == nil {
			if attempt > 0 {
				// Log successful retry
			}
			return result, nil
		}

		lastErr = err

		// Check if this is the last attempt
		if attempt == config.MaxRetries {
			break
		}

		// Check if error is retryable
		if !isRetryableError(err, config.RetryableErrors) {
			break
		}

		// Check if context is done
		select {
		case <-ctx.Done():
			return nil, fmt.Errorf("context cancelled during retry: %w", ctx.Err())
		default:
		}

		// Calculate delay with exponential backoff and jitter
		backoffDelay := time.Duration(float64(delay) * math.Pow(config.BackoffMultiplier, float64(attempt)))
		if backoffDelay > config.MaxDelay {
			backoffDelay = config.MaxDelay
		}

		// Add jitter to prevent thundering herd
		if config.MaxJitter > 0 {
			jitter := time.Duration(rand.Int63n(int64(config.MaxJitter)))
			backoffDelay += jitter
		}

		// Wait before retrying
		select {
		case <-ctx.Done():
			return nil, fmt.Errorf("context cancelled during retry delay: %w", ctx.Err())
		case <-time.After(backoffDelay):
		}

		// Log retry attempt
	}

	return nil, fmt.Errorf("max retries (%d) exceeded, last error: %w", config.MaxRetries, lastErr)
}

// isRetryableError checks if an error is retryable
func isRetryableError(err error, retryableErrors []string) bool {
	// Check if error is explicitly marked as retryable
	if _, ok := err.(RetryableError); ok {
		return true
	}

	// Check if error matches any of the retryable error patterns
	errStr := err.Error()
	for _, pattern := range retryableErrors {
		if containsPattern(errStr, pattern) {
			return true
		}
	}

	// Common retryable error patterns
	retryablePatterns := []string{
		"connection refused",
		"connection reset",
		"timeout",
		"temporary failure",
		"service unavailable",
		"gateway timeout",
		"rate limit",
		"too many requests",
		"network unreachable",
		"no route to host",
		"dial tcp",
		"dial udp",
		"EOF",
		"broken pipe",
	}

	for _, pattern := range retryablePatterns {
		if containsPattern(errStr, pattern) {
			return true
		}
	}

	return false
}

// containsPattern checks if the error string contains the pattern (case-insensitive)
func containsPattern(str, pattern string) bool {
	return len(pattern) > 0 && len(str) >= len(pattern) &&
		(equalsIgnoreCase(str, pattern) ||
			indexOfIgnoreCase(str, pattern) >= 0)
}

func equalsIgnoreCase(a, b string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := 0; i < len(a); i++ {
		if toLower(a[i]) != toLower(b[i]) {
			return false
		}
	}
	return true
}

func indexOfIgnoreCase(s, pattern string) int {
	if len(pattern) == 0 || len(s) < len(pattern) {
		return -1
	}
	for i := 0; i <= len(s)-len(pattern); i++ {
		if equalsIgnoreCase(s[i:i+len(pattern)], pattern) {
			return i
		}
	}
	return -1
}

func toLower(b byte) byte {
	if 'A' <= b && b <= 'Z' {
		return b + ('a' - 'A')
	}
	return b
}

// CircuitBreakerWithRetry combines circuit breaker with retry logic
type CircuitBreakerWithRetry struct {
	circuitBreaker *CircuitBreaker
	retryConfig    RetryConfig
}

// NewCircuitBreakerWithRetry creates a new circuit breaker with retry
func NewCircuitBreakerWithRetry(cbConfig CircuitBreakerConfig, retryConfig RetryConfig) *CircuitBreakerWithRetry {
	return &CircuitBreakerWithRetry{
		circuitBreaker: NewCircuitBreaker(cbConfig),
		retryConfig:    retryConfig,
	}
}

// Execute executes an operation with circuit breaker and retry protection
func (cbr *CircuitBreakerWithRetry) Execute(operation func() (interface{}, error)) (interface{}, error) {
	// Use circuit breaker with retry
	result, err := RetryWithResult(
		context.Background(),
		cbr.retryConfig,
		func() (interface{}, error) {
			return cbr.circuitBreaker.Execute(operation)
		},
	)

	return result, err
}

// GetCircuitBreakerState returns the circuit breaker state
func (cbr *CircuitBreakerWithRetry) GetCircuitBreakerState() State {
	return cbr.circuitBreaker.GetState()
}

// GetCircuitBreakerStats returns circuit breaker statistics
func (cbr *CircuitBreakerWithRetry) GetCircuitBreakerStats() map[string]interface{} {
	return cbr.circuitBreaker.GetStats()
}

// RetryableOperation represents an operation that can be retried
type RetryableOperation struct {
	Attempt     int
	Delay       time.Duration
	MaxRetries  int
	NextRetryAt time.Time
}

// GetNextRetryDelay calculates the delay for the next retry
func (ro *RetryableOperation) GetNextRetryDelay() time.Duration {
	if ro.Attempt >= ro.MaxRetries {
		return 0
	}

	// Exponential backoff
	delay := time.Duration(math.Pow(2, float64(ro.Attempt)) * float64(time.Second))
	if delay > 30*time.Second {
		delay = 30 * time.Second
	}

	return delay
}

// RetryPolicy defines the retry strategy
type RetryPolicy struct {
	MaxAttempts   int           // Maximum number of retry attempts
	InitialDelay  time.Duration // Initial delay between retries
	MaxDelay      time.Duration // Maximum delay between retries
	BackoffFactor float64       // Multiplier for each retry
	Jitter        bool          // Whether to add random jitter
}

// LinearBackoff retry policy
func LinearBackoff(maxAttempts int, delay time.Duration) RetryPolicy {
	return RetryPolicy{
		MaxAttempts:   maxAttempts,
		InitialDelay:  delay,
		MaxDelay:      delay * time.Duration(maxAttempts),
		BackoffFactor: 1.0,
		Jitter:        true,
	}
}

// ExponentialBackoff retry policy
func ExponentialBackoff(maxAttempts int, initialDelay time.Duration) RetryPolicy {
	return RetryPolicy{
		MaxAttempts:   maxAttempts,
		InitialDelay:  initialDelay,
		MaxDelay:      30 * time.Second,
		BackoffFactor: 2.0,
		Jitter:        true,
	}
}

// FibonacciBackoff retry policy
func FibonacciBackoff(maxAttempts int, initialDelay time.Duration) RetryPolicy {
	return RetryPolicy{
		MaxAttempts:   maxAttempts,
		InitialDelay:  initialDelay,
		MaxDelay:      30 * time.Second,
		BackoffFactor: 0.0, // Use fibonacci sequence
		Jitter:        true,
	}
}
