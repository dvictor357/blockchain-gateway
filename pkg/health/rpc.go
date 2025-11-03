package health

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"time"
)

// RPCChecker checks the health of a blockchain RPC endpoint
type RPCChecker struct {
	name      string
	rpcURL    string
	client    *http.Client
	priority  int
	chainType string // "evm", "bitcoin", etc.
}

// NewRPCChecker creates a new RPC health checker
func NewRPCChecker(name, rpcURL string, chainType string, client *http.Client) *RPCChecker {
	return &RPCChecker{
		name:      name,
		rpcURL:    rpcURL,
		client:    client,
		priority:  1,
		chainType: chainType,
	}
}

// Name returns the name of the health checker
func (c *RPCChecker) Name() string {
	return c.name
}

// Priority returns the check priority
func (c *RPCChecker) Priority() int {
	return c.priority
}

// Check performs the RPC health check
func (c *RPCChecker) Check(ctx context.Context) CheckResult {
	start := time.Now()

	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	// Perform different checks based on chain type
	var result CheckResult

	switch c.chainType {
	case "evm":
		result = c.checkEVM(ctx)
	case "bitcoin":
		result = c.checkBitcoin(ctx)
	default:
		result = c.checkGeneric(ctx)
	}

	// Add timing information
	duration := time.Since(start)
	result.Data["duration_ms"] = duration.Milliseconds()
	result.Data["rpc_url"] = c.rpcURL
	result.Data["chain_type"] = c.chainType

	return result
}

// checkEVM performs EVM-specific health checks
func (c *RPCChecker) checkEVM(ctx context.Context) CheckResult {
	// Test basic RPC connectivity with eth_blockNumber
	payload := map[string]interface{}{
		"jsonrpc": "2.0",
		"method":  "eth_blockNumber",
		"params":  []interface{}{},
		"id":      1,
	}

	response, statusCode, err := c.makeRPCRequest(ctx, payload)
	if err != nil {
		return CheckResult{
			Status:  StatusCritical,
			Message: "RPC request failed",
			Error:   err,
		}
	}

	if statusCode != http.StatusOK {
		return CheckResult{
			Status:  StatusCritical,
			Message: "RPC request returned non-OK status",
			Error:   err,
		}
	}

	// Parse response
	var rpcResponse map[string]interface{}
	if err := json.Unmarshal(response, &rpcResponse); err != nil {
		return CheckResult{
			Status:  StatusCritical,
			Message: "Failed to parse RPC response",
			Error:   err,
		}
	}

	// Check for error in response
	if errorObj, exists := rpcResponse["error"]; exists {
		return CheckResult{
			Status:  StatusCritical,
			Message: "RPC returned error",
			Data:    map[string]interface{}{"rpc_error": errorObj},
		}
	}

	// Check result
	result, exists := rpcResponse["result"]
	if !exists {
		return CheckResult{
			Status:  StatusCritical,
			Message: "RPC response missing result",
		}
	}

	// Additional EVM checks could be added here
	// - Check latest block number is recent
	// - Check gas price
	// - Check syncing status

	return CheckResult{
		Status:  StatusOK,
		Message: "EVM RPC endpoint is healthy",
		Data: map[string]interface{}{
			"block_number": result,
			"method":       "eth_blockNumber",
		},
	}
}

// checkBitcoin performs Bitcoin-specific health checks
func (c *RPCChecker) checkBitcoin(ctx context.Context) CheckResult {
	// Bitcoin RPC health check would go here
	return CheckResult{
		Status:  StatusWarning,
		Message: "Bitcoin health check not implemented",
	}
}

// checkGeneric performs generic health checks
func (c *RPCChecker) checkGeneric(ctx context.Context) CheckResult {
	// For generic chains, test with a simple method
	payload := map[string]interface{}{
		"jsonrpc": "2.0",
		"method":  "getblockchaininfo",
		"params":  []interface{}{},
		"id":      1,
	}

	response, statusCode, err := c.makeRPCRequest(ctx, payload)
	if err != nil {
		return CheckResult{
			Status:  StatusCritical,
			Message: "RPC request failed",
			Error:   err,
		}
	}

	if statusCode != http.StatusOK {
		return CheckResult{
			Status:  StatusCritical,
			Message: "RPC request returned non-OK status",
			Error:   err,
		}
	}

	return CheckResult{
		Status:  StatusOK,
		Message: "RPC endpoint is responding",
		Data: map[string]interface{}{
			"response_size": len(response),
		},
	}
}

// makeRPCRequest makes an HTTP request to the RPC endpoint
func (c *RPCChecker) makeRPCRequest(ctx context.Context, payload map[string]interface{}) ([]byte, int, error) {
	// Marshal payload
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, 0, err
	}

	// Create request
	req, err := http.NewRequestWithContext(ctx, "POST", c.rpcURL, strings.NewReader(string(body)))
	if err != nil {
		return nil, 0, err
	}

	req.Header.Set("Content-Type", "application/json")

	// Make request
	resp, err := c.client.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()

	// Read response
	response, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, resp.StatusCode, err
	}

	return response, resp.StatusCode, nil
}

// RPCLatencyChecker measures RPC endpoint latency
type RPCLatencyChecker struct {
	rpcCheckers []*RPCChecker
	priority    int
}

// NewRPCLatencyChecker creates a new RPC latency checker
func NewRPCLatencyChecker(rpcCheckers []*RPCChecker) *RPCLatencyChecker {
	return &RPCLatencyChecker{
		rpcCheckers: rpcCheckers,
		priority:    10,
	}
}

// Name returns the name of the health checker
func (c *RPCLatencyChecker) Name() string {
	return "rpc_latency"
}

// Priority returns the check priority
func (c *RPCLatencyChecker) Priority() int {
	return c.priority
}

// Check performs RPC latency checks
func (c *RPCLatencyChecker) Check(ctx context.Context) CheckResult {
	latencies := make(map[string]int64)

	for _, checker := range c.rpcCheckers {
		start := time.Now()
		_ = checker.Check(ctx) // We only care about latency, not the result
		duration := time.Since(start).Milliseconds()

		latencies[checker.name] = duration

		// Log high latencies
		if duration > 5000 { // 5 seconds
			// This could trigger an alert
		}
	}

	return CheckResult{
		Status:  StatusOK,
		Message: "RPC latency check completed",
		Data: map[string]interface{}{
			"latencies": latencies,
		},
	}
}
