package resilience

import (
	"sync"
	"time"
)

// State represents the state of the circuit breaker
type State int

const (
	StateClosed State = iota
	StateOpen
	StateHalfOpen
)

// String returns the string representation of the state
func (s State) String() string {
	switch s {
	case StateClosed:
		return "closed"
	case StateOpen:
		return "open"
	case StateHalfOpen:
		return "half-open"
	default:
		return "unknown"
	}
}

// CircuitBreakerConfig holds configuration for the circuit breaker
type CircuitBreakerConfig struct {
	FailureThreshold int           // Number of failures to open the circuit
	RecoveryTimeout  time.Duration // Time to wait before attempting recovery
	SuccessThreshold int           // Number of successes to close the circuit from half-open
	Timeout          time.Duration // Timeout for operations
	MonitoreInterval time.Duration // Interval to check the state
}

// CircuitBreaker implements the circuit breaker pattern
type CircuitBreaker struct {
	config CircuitBreakerConfig
	state  State

	failures        int
	successes       int
	lastFailureTime time.Time
	lastSuccessTime time.Time

	mutex sync.Mutex
}

// NewCircuitBreaker creates a new circuit breaker
func NewCircuitBreaker(config CircuitBreakerConfig) *CircuitBreaker {
	return &CircuitBreaker{
		config:          config,
		state:           StateClosed,
		failures:        0,
		successes:       0,
		lastFailureTime: time.Time{},
		lastSuccessTime: time.Time{},
	}
}

// Execute executes a function with circuit breaker protection
func (cb *CircuitBreaker) Execute(operation func() (interface{}, error)) (interface{}, error) {
	cb.mutex.Lock()
	defer cb.mutex.Unlock()

	// Check state transitions
	if cb.state == StateOpen {
		// Check if it's time to try half-open
		if time.Since(cb.lastFailureTime) >= cb.config.RecoveryTimeout {
			cb.state = StateHalfOpen
			cb.successes = 0
			cb.mutex.Unlock()
		} else {
			cb.mutex.Unlock()
			return nil, ErrCircuitOpen
		}
	} else {
		cb.mutex.Unlock()
	}

	// Create a channel for the result
	resultChan := make(chan struct {
		value interface{}
		err   error
	}, 1)

	// Execute the operation with timeout
	go func() {
		value, err := operation()
		resultChan <- struct {
			value interface{}
			err   error
		}{value, err}
	}()

	// Wait for completion or timeout
	select {
	case result := <-resultChan:
		cb.recordResult(result.err == nil)
		return result.value, result.err
	case <-time.After(cb.config.Timeout):
		cb.recordResult(false)
		return nil, ErrTimeout
	}
}

// recordResult records the result of an operation
func (cb *CircuitBreaker) recordResult(success bool) {
	cb.mutex.Lock()
	defer cb.mutex.Unlock()

	if success {
		cb.lastSuccessTime = time.Now()

		if cb.state == StateHalfOpen {
			cb.successes++
			if cb.successes >= cb.config.SuccessThreshold {
				cb.state = StateClosed
				cb.failures = 0
			}
		} else if cb.state == StateClosed {
			// Reset failures on success in closed state
			cb.failures = 0
		}
	} else {
		cb.lastFailureTime = time.Now()
		cb.failures++

		if cb.state == StateHalfOpen {
			// Any failure in half-open goes back to open
			cb.state = StateOpen
		} else if cb.state == StateClosed && cb.failures >= cb.config.FailureThreshold {
			// Failures exceeded threshold, open the circuit
			cb.state = StateOpen
		}
	}
}

// GetState returns the current state of the circuit breaker
func (cb *CircuitBreaker) GetState() State {
	cb.mutex.Lock()
	defer cb.mutex.Unlock()
	return cb.state
}

// GetStats returns statistics about the circuit breaker
func (cb *CircuitBreaker) GetStats() map[string]interface{} {
	cb.mutex.Lock()
	defer cb.mutex.Unlock()

	return map[string]interface{}{
		"state":             cb.state.String(),
		"failures":          cb.failures,
		"successes":         cb.successes,
		"last_failure_time": cb.lastFailureTime,
		"last_success_time": cb.lastSuccessTime,
		"failure_threshold": cb.config.FailureThreshold,
		"recovery_timeout":  cb.config.RecoveryTimeout.String(),
		"success_threshold": cb.config.SuccessThreshold,
	}
}

// Reset resets the circuit breaker to closed state
func (cb *CircuitBreaker) Reset() {
	cb.mutex.Lock()
	defer cb.mutex.Unlock()

	cb.state = StateClosed
	cb.failures = 0
	cb.successes = 0
	cb.lastFailureTime = time.Time{}
	cb.lastSuccessTime = time.Time{}
}

// errCircuitOpen represents a circuit open error
type errCircuitOpen struct {
	state     State
	timestamp time.Time
}

func (e errCircuitOpen) Error() string {
	return "circuit breaker is open"
}

// ErrCircuitOpen is returned when the circuit is open
var ErrCircuitOpen = errCircuitOpen{}

// errTimeout represents a timeout error
type errTimeout struct {
	duration time.Duration
}

func (e errTimeout) Error() string {
	return "operation timed out"
}

// ErrTimeout is returned when an operation times out
var ErrTimeout = errTimeout{}

// WithCircuitBreaker wraps an operation with circuit breaker protection
func WithCircuitBreaker(cb *CircuitBreaker, operation func() (interface{}, error)) (interface{}, error) {
	return cb.Execute(operation)
}

// CircuitBreakerPool manages multiple circuit breakers
type CircuitBreakerPool struct {
	breakers map[string]*CircuitBreaker
	mutex    sync.RWMutex
	config   CircuitBreakerConfig
}

// NewCircuitBreakerPool creates a new circuit breaker pool
func NewCircuitBreakerPool(config CircuitBreakerConfig) *CircuitBreakerPool {
	return &CircuitBreakerPool{
		breakers: make(map[string]*CircuitBreaker),
		config:   config,
	}
}

// GetOrCreate returns an existing circuit breaker or creates a new one
func (p *CircuitBreakerPool) GetOrCreate(key string) *CircuitBreaker {
	p.mutex.Lock()
	defer p.mutex.Unlock()

	if cb, exists := p.breakers[key]; exists {
		return cb
	}

	cb := NewCircuitBreaker(p.config)
	p.breakers[key] = cb
	return cb
}

// Execute executes an operation with circuit breaker protection using the key
func (p *CircuitBreakerPool) Execute(key string, operation func() (interface{}, error)) (interface{}, error) {
	cb := p.GetOrCreate(key)
	return cb.Execute(operation)
}

// GetStats returns statistics for all circuit breakers
func (p *CircuitBreakerPool) GetStats() map[string]map[string]interface{} {
	p.mutex.RLock()
	defer p.mutex.RUnlock()

	stats := make(map[string]map[string]interface{})
	for key, cb := range p.breakers {
		stats[key] = cb.GetStats()
	}
	return stats
}

// DefaultCircuitBreakerConfig returns a default configuration
func DefaultCircuitBreakerConfig() CircuitBreakerConfig {
	return CircuitBreakerConfig{
		FailureThreshold: 5,
		RecoveryTimeout:  60 * time.Second,
		SuccessThreshold: 3,
		Timeout:          30 * time.Second,
		MonitoreInterval: 10 * time.Second,
	}
}
