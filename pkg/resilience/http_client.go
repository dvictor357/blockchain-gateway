package resilience

import (
	"math"
	"net/http"
	"sync"
	"time"
)

// HTTPClientConfig holds configuration for HTTP client
type HTTPClientConfig struct {
	// Connection pool settings
	MaxIdleConns        int
	MaxIdleConnsPerHost int
	IdleConnTimeout     time.Duration

	// Timeout settings
	DialTimeout           time.Duration
	TLSHandshakeTimeout   time.Duration
	ResponseHeaderTimeout time.Duration
	WriteTimeout          time.Duration

	// Keep-alive settings
	DisableCompression bool
	ForceAttemptHTTP2  bool

	// Retry settings
	MaxRetries   int
	RetryDelay   time.Duration
	RetryBackoff float64

	// Health check settings
	HealthCheckInterval time.Duration
}

// DefaultHTTPClientConfig returns default HTTP client configuration
func DefaultHTTPClientConfig() HTTPClientConfig {
	return HTTPClientConfig{
		MaxIdleConns:          100,
		MaxIdleConnsPerHost:   10,
		IdleConnTimeout:       90 * time.Second,
		DialTimeout:           10 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ResponseHeaderTimeout: 30 * time.Second,
		WriteTimeout:          30 * time.Second,
		DisableCompression:    false,
		ForceAttemptHTTP2:     true,
		MaxRetries:            3,
		RetryDelay:            1 * time.Second,
		RetryBackoff:          2.0,
		HealthCheckInterval:   60 * time.Second,
	}
}

// OptimizedHTTPClient creates an optimized HTTP client
func OptimizedHTTPClient(config HTTPClientConfig) *http.Client {
	transport := &http.Transport{
		MaxIdleConns:        config.MaxIdleConns,
		MaxIdleConnsPerHost: config.MaxIdleConnsPerHost,
		IdleConnTimeout:     config.IdleConnTimeout,
		DisableCompression:  config.DisableCompression,
		ForceAttemptHTTP2:   config.ForceAttemptHTTP2,

		// Proxy settings
		Proxy: http.ProxyFromEnvironment,

		// TLS settings
		TLSHandshakeTimeout: config.TLSHandshakeTimeout,

		// Response header timeout
		ResponseHeaderTimeout: config.ResponseHeaderTimeout,
	}

	client := &http.Client{
		Transport: transport,
		Timeout:   config.WriteTimeout,
	}

	// Add retry capability with RoundTripper
	client.Transport = &RetryableTransport{
		inner:        transport,
		maxRetries:   config.MaxRetries,
		retryDelay:   config.RetryDelay,
		retryBackoff: config.RetryBackoff,
	}

	return client
}

// RetryableTransport wraps an HTTP transport with retry capability
type RetryableTransport struct {
	inner        *http.Transport
	maxRetries   int
	retryDelay   time.Duration
	retryBackoff float64
	mutex        sync.Mutex
}

// RoundTrip implements http.RoundTrip with retry capability
func (rt *RetryableTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	var lastErr error

	for attempt := 0; attempt <= rt.maxRetries; attempt++ {
		resp, err := rt.inner.RoundTrip(req)

		if err == nil {
			// Success
			if attempt > 0 {
				// Log successful retry (add logger if needed)
			}
			return resp, nil
		}

		lastErr = err

		// Check if this is the last attempt
		if attempt == rt.maxRetries {
			break
		}

		// Check if error is retryable
		if !isRetryableHTTPError(err) {
			break
		}

		// Calculate delay with exponential backoff
		delay := time.Duration(float64(rt.retryDelay) * math.Pow(rt.retryBackoff, float64(attempt)))
		if delay > 30*time.Second {
			delay = 30 * time.Second
		}

		time.Sleep(delay)
	}

	return nil, lastErr
}

// HTTPClientPool manages a pool of HTTP clients
type HTTPClientPool struct {
	config HTTPClientConfig
	client *http.Client
	mutex  sync.RWMutex
}

// NewHTTPClientPool creates a new HTTP client pool
func NewHTTPClientPool(config HTTPClientConfig) *HTTPClientPool {
	return &HTTPClientPool{
		config: config,
		client: OptimizedHTTPClient(config),
	}
}

// Get returns the HTTP client from the pool
func (p *HTTPClientPool) Get() *http.Client {
	p.mutex.RLock()
	defer p.mutex.RUnlock()
	return p.client
}

// Close closes the HTTP client
func (p *HTTPClientPool) Close() error {
	p.mutex.Lock()
	defer p.mutex.Unlock()
	if p.client != nil {
		// Note: http.Client doesn't have a Close method
		// The underlying transport connections will be closed when garbage collected
		p.client = nil
	}
	return nil
}

// GetConfig returns the configuration
func (p *HTTPClientPool) GetConfig() HTTPClientConfig {
	p.mutex.RLock()
	defer p.mutex.RUnlock()
	return p.config
}

// HealthCheckClient performs health checks on HTTP endpoints
type HealthCheckClient struct {
	client   *http.Client
	config   HTTPClientConfig
	checkers map[string]*HealthChecker
	mutex    sync.RWMutex
	done     chan struct{}
}

// HealthChecker represents a health check for an endpoint
type HealthChecker struct {
	URL                 string
	Method              string
	StatusCode          int
	LastCheck           time.Time
	IsHealthy           bool
	ConsecutiveFailures int
}

// NewHealthCheckClient creates a new health check client
func NewHealthCheckClient(config HTTPClientConfig) *HealthCheckClient {
	return &HealthCheckClient{
		client:   OptimizedHTTPClient(config),
		config:   config,
		checkers: make(map[string]*HealthChecker),
		done:     make(chan struct{}),
	}
}

// AddChecker adds a health checker for an endpoint
func (hc *HealthCheckClient) AddChecker(name, url string) *HealthChecker {
	hc.mutex.Lock()
	defer hc.mutex.Unlock()

	checker := &HealthChecker{
		URL:        url,
		Method:     "GET",
		StatusCode: 200,
		IsHealthy:  true,
	}

	hc.checkers[name] = checker
	return checker
}

// StartHealthChecks starts periodic health checks
func (hc *HealthCheckClient) StartHealthChecks() {
	go func() {
		ticker := time.NewTicker(hc.config.HealthCheckInterval)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				hc.performHealthChecks()
			case <-hc.done:
				return
			}
		}
	}()
}

// StopHealthChecks stops the health check routine
func (hc *HealthCheckClient) StopHealthChecks() {
	close(hc.done)
}

// performHealthChecks performs health checks on all endpoints
func (hc *HealthCheckClient) performHealthChecks() {
	hc.mutex.RLock()
	checkersCopy := make(map[string]*HealthChecker)
	for name, checker := range hc.checkers {
		checkersCopy[name] = checker
	}
	hc.mutex.RUnlock()

	for name, checker := range checkersCopy {
		go hc.checkEndpoint(name, checker)
	}
}

// checkEndpoint checks the health of a single endpoint
func (hc *HealthCheckClient) checkEndpoint(name string, checker *HealthChecker) {
	req, err := http.NewRequest(checker.Method, checker.URL, nil)
	if err != nil {
		return
	}

	resp, err := hc.client.Do(req)
	if err != nil {
		hc.recordFailure(checker)
		return
	}
	defer resp.Body.Close()

	checker.LastCheck = time.Now()

	if resp.StatusCode == checker.StatusCode {
		hc.recordSuccess(checker)
	} else {
		hc.recordFailure(checker)
	}
}

// recordSuccess records a successful health check
func (hc *HealthCheckClient) recordSuccess(checker *HealthChecker) {
	hc.mutex.Lock()
	defer hc.mutex.Unlock()

	checker.ConsecutiveFailures = 0
	checker.IsHealthy = true
}

// recordFailure records a failed health check
func (hc *HealthCheckClient) recordFailure(checker *HealthChecker) {
	hc.mutex.Lock()
	defer hc.mutex.Unlock()

	checker.ConsecutiveFailures++
	if checker.ConsecutiveFailures >= 3 {
		checker.IsHealthy = false
	}
}

// GetChecker returns a health checker by name
func (hc *HealthCheckClient) GetChecker(name string) (*HealthChecker, bool) {
	hc.mutex.RLock()
	defer hc.mutex.RUnlock()

	checker, exists := hc.checkers[name]
	return checker, exists
}

// isRetryableHTTPError checks if an HTTP error is retryable
func isRetryableHTTPError(err error) bool {
	// Common retryable HTTP errors
	retryablePatterns := []string{
		"connection refused",
		"connection reset",
		"timeout",
		"temporary failure",
		"service unavailable",
		"gateway timeout",
		"EOF",
		"broken pipe",
		"network unreachable",
		"no route to host",
	}

	errStr := err.Error()
	for _, pattern := range retryablePatterns {
		if containsPattern(errStr, pattern) {
			return true
		}
	}

	return false
}
