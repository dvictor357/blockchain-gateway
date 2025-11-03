package health

import (
	"context"
	"net/http"
	"time"
)

// ExternalAPIChecker checks the health of external APIs
type ExternalAPIChecker struct {
	name     string
	apiURL   string
	client   *http.Client
	priority int
	timeout  time.Duration
	headers  map[string]string
}

// NewExternalAPIChecker creates a new external API health checker
func NewExternalAPIChecker(name, apiURL string, client *http.Client, headers map[string]string) *ExternalAPIChecker {
	return &ExternalAPIChecker{
		name:     name,
		apiURL:   apiURL,
		client:   client,
		priority: 5,
		timeout:  5 * time.Second,
		headers:  headers,
	}
}

// Name returns the name of the health checker
func (c *ExternalAPIChecker) Name() string {
	return c.name
}

// Priority returns the check priority
func (c *ExternalAPIChecker) Priority() int {
	return c.priority
}

// Check performs the external API health check
func (c *ExternalAPIChecker) Check(ctx context.Context) CheckResult {
	start := time.Now()

	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()

	// Create request
	req, err := http.NewRequestWithContext(ctx, "GET", c.apiURL, nil)
	if err != nil {
		return CheckResult{
			Status:  StatusCritical,
			Message: "Failed to create request",
			Error:   err,
		}
	}

	// Add headers
	for key, value := range c.headers {
		req.Header.Set(key, value)
	}

	// Make request
	resp, err := c.client.Do(req)
	if err != nil {
		return CheckResult{
			Status:  StatusCritical,
			Message: "External API request failed",
			Error:   err,
		}
	}
	defer resp.Body.Close()

	duration := time.Since(start)

	// Check status code
	var status HealthStatus
	var message string

	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		status = StatusOK
		message = "External API is healthy"
	} else if resp.StatusCode >= 400 && resp.StatusCode < 500 {
		status = StatusWarning
		message = "External API returned client error"
	} else if resp.StatusCode >= 500 {
		status = StatusCritical
		message = "External API returned server error"
	}

	return CheckResult{
		Status:  status,
		Message: message,
		Data: map[string]interface{}{
			"status_code":      resp.StatusCode,
			"response_time_ms": duration.Milliseconds(),
			"api_url":          c.apiURL,
		},
	}
}

// CoinGeckoChecker checks CoinGecko API health
type CoinGeckoChecker struct {
	apiURL   string
	client   *http.Client
	priority int
}

// NewCoinGeckoChecker creates a new CoinGecko health checker
func NewCoinGeckoChecker(apiURL string, client *http.Client) *CoinGeckoChecker {
	return &CoinGeckoChecker{
		apiURL:   apiURL,
		client:   client,
		priority: 3,
	}
}

// Name returns the name of the health checker
func (c *CoinGeckoChecker) Name() string {
	return "coingecko_api"
}

// Priority returns the check priority
func (c *CoinGeckoChecker) Priority() int {
	return c.priority
}

// Check performs the CoinGecko API health check
func (c *CoinGeckoChecker) Check(ctx context.Context) CheckResult {
	start := time.Now()

	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	// Ping CoinGecko with a simple request
	pingURL := c.apiURL + "/ping"
	req, err := http.NewRequestWithContext(ctx, "GET", pingURL, nil)
	if err != nil {
		return CheckResult{
			Status:  StatusCritical,
			Message: "Failed to create request",
			Error:   err,
		}
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return CheckResult{
			Status:  StatusCritical,
			Message: "CoinGecko API request failed",
			Error:   err,
		}
	}
	defer resp.Body.Close()

	duration := time.Since(start)

	// Check status
	var status HealthStatus
	if resp.StatusCode == http.StatusOK {
		status = StatusOK
	} else if resp.StatusCode == http.StatusTooManyRequests {
		status = StatusWarning
	} else {
		status = StatusCritical
	}

	return CheckResult{
		Status:  status,
		Message: getCoinGeckoMessage(status),
		Data: map[string]interface{}{
			"status_code":      resp.StatusCode,
			"response_time_ms": duration.Milliseconds(),
			"ping_endpoint":    pingURL,
		},
	}
}

// getCoinGeckoMessage returns a message based on health status
func getCoinGeckoMessage(status HealthStatus) string {
	switch status {
	case StatusOK:
		return "CoinGecko API is healthy"
	case StatusWarning:
		return "CoinGecko API rate limiting"
	case StatusCritical:
		return "CoinGecko API is not responding"
	default:
		return "CoinGecko API status unknown"
	}
}

// HTTPStatusChecker checks generic HTTP endpoint health
type HTTPStatusChecker struct {
	name         string
	url          string
	client       *http.Client
	priority     int
	timeout      time.Duration
	expectedCode int
}

// NewHTTPStatusChecker creates a new HTTP status health checker
func NewHTTPStatusChecker(name, url string, client *http.Client, expectedCode int) *HTTPStatusChecker {
	return &HTTPStatusChecker{
		name:         name,
		url:          url,
		client:       client,
		priority:     5,
		timeout:      5 * time.Second,
		expectedCode: expectedCode,
	}
}

// Name returns the name of the health checker
func (c *HTTPStatusChecker) Name() string {
	return c.name
}

// Priority returns the check priority
func (c *HTTPStatusChecker) Priority() int {
	return c.priority
}

// Check performs the HTTP status health check
func (c *HTTPStatusChecker) Check(ctx context.Context) CheckResult {
	start := time.Now()

	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "GET", c.url, nil)
	if err != nil {
		return CheckResult{
			Status:  StatusCritical,
			Message: "Failed to create request",
			Error:   err,
		}
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return CheckResult{
			Status:  StatusCritical,
			Message: "HTTP request failed",
			Error:   err,
		}
	}
	defer resp.Body.Close()

	duration := time.Since(start)

	// Check status code
	var status HealthStatus
	var message string

	if resp.StatusCode == c.expectedCode {
		status = StatusOK
		message = "HTTP endpoint is healthy"
	} else if resp.StatusCode >= 500 {
		status = StatusCritical
		message = "HTTP endpoint server error"
	} else {
		status = StatusWarning
		message = "HTTP endpoint returned unexpected status"
	}

	return CheckResult{
		Status:  status,
		Message: message,
		Data: map[string]interface{}{
			"status_code":      resp.StatusCode,
			"expected_code":    c.expectedCode,
			"response_time_ms": duration.Milliseconds(),
			"url":              c.url,
		},
	}
}
