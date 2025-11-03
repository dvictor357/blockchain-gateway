package blockchain

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewEVMClient(t *testing.T) {
	tests := []struct {
		name       string
		clientName string
		rpcURL     string
		httpClient *http.Client
	}{
		{
			name:       "with custom http client",
			clientName: "ethereum",
			rpcURL:     "https://eth.example.com",
			httpClient: &http.Client{Timeout: 10 * time.Second},
		},
		{
			name:       "with nil http client (should use default)",
			clientName: "polygon",
			rpcURL:     "https://polygon.example.com",
			httpClient: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := NewEVMClient(tt.clientName, tt.rpcURL, tt.httpClient)

			assert.NotNil(t, client)
			assert.Equal(t, tt.clientName, client.name)
			assert.Equal(t, tt.rpcURL, client.rpcURL)
			assert.NotNil(t, client.httpClient)
			assert.Equal(t, uint64(0), client.requestID)

			if tt.httpClient == nil {
				assert.Equal(t, http.DefaultClient, client.httpClient)
			} else {
				assert.Equal(t, tt.httpClient, client.httpClient)
			}
		})
	}
}

func TestEVMClient_Name(t *testing.T) {
	client := NewEVMClient("test-chain", "https://test.example.com", nil)
	assert.Equal(t, "test-chain", client.Name())
}

func TestEVMClient_Execute(t *testing.T) {
	tests := []struct {
		name           string
		method         string
		params         interface{}
		serverResponse string
		serverStatus   int
		expectedResult json.RawMessage
		expectedError  string
	}{
		{
			name:           "successful RPC call",
			method:         "eth_blockNumber",
			params:         []interface{}{},
			serverResponse: `{"jsonrpc":"2.0","result":"0x1234567","id":1}`,
			serverStatus:   http.StatusOK,
			expectedResult: json.RawMessage(`"0x1234567"`),
		},
		{
			name:           "successful RPC call with params",
			method:         "eth_getBalance",
			params:         []interface{}{"0x123", "latest"},
			serverResponse: `{"jsonrpc":"2.0","result":"0xde0b6b3a7640000","id":1}`,
			serverStatus:   http.StatusOK,
			expectedResult: json.RawMessage(`"0xde0b6b3a7640000"`),
		},
		{
			name:           "RPC error response",
			method:         "eth_getBalance",
			params:         []interface{}{"invalid", "latest"},
			serverResponse: `{"jsonrpc":"2.0","error":{"code":-32602,"message":"Invalid params"},"id":1}`,
			serverStatus:   http.StatusOK,
			expectedError:  "RPC error: -32602 - Invalid params",
		},
		{
			name:           "HTTP error",
			method:         "eth_blockNumber",
			params:         []interface{}{},
			serverResponse: `{"error":"Internal server error"}`,
			serverStatus:   http.StatusInternalServerError,
			expectedError:  "unexpected status code: 500",
		},
		{
			name:           "invalid JSON response",
			method:         "eth_blockNumber",
			params:         []interface{}{},
			serverResponse: `invalid json`,
			serverStatus:   http.StatusOK,
			expectedError:  "failed to unmarshal response",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a test server
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				// Verify request method and headers
				assert.Equal(t, "POST", r.Method)
				assert.Equal(t, "application/json", r.Header.Get("Content-Type"))
				assert.Equal(t, "application/json", r.Header.Get("Accept"))

				// Verify request body structure
				var request RPCRequest
				err := json.NewDecoder(r.Body).Decode(&request)
				require.NoError(t, err)
				assert.Equal(t, "2.0", request.JSONRPC)
				assert.Equal(t, tt.method, request.Method)
				assert.NotNil(t, request.ID)

				// Send response
				w.WriteHeader(tt.serverStatus)
				w.Write([]byte(tt.serverResponse))
			}))
			defer server.Close()

			// Create client with test server URL
			client := NewEVMClient("test", server.URL, nil)

			// Execute the method
			ctx := context.Background()
			result, err := client.Execute(ctx, tt.method, tt.params)

			if tt.expectedError != "" {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.expectedError)
				assert.Nil(t, result)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expectedResult, result)
			}
		})
	}
}

func TestEVMClient_Execute_RequestID(t *testing.T) {
	// Test that request IDs are incremented
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request RPCRequest
		json.NewDecoder(r.Body).Decode(&request)

		// Echo back the request ID in the response
		response := map[string]interface{}{
			"jsonrpc": "2.0",
			"result":  "success",
			"id":      request.ID,
		}
		json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	client := NewEVMClient("test", server.URL, nil)
	ctx := context.Background()

	// Make multiple requests and verify IDs increment
	for i := 1; i <= 3; i++ {
		_, err := client.Execute(ctx, "test_method", []interface{}{})
		assert.NoError(t, err)
		assert.Equal(t, uint64(i), client.requestID)
	}
}

func TestEVMClient_GetLatestBlockNumber(t *testing.T) {
	tests := []struct {
		name           string
		serverResponse string
		expectedResult uint64
		expectedError  string
	}{
		{
			name:           "valid block number",
			serverResponse: `{"jsonrpc":"2.0","result":"0x1234567","id":1}`,
			expectedResult: 0x1234567,
		},
		{
			name:           "zero block number",
			serverResponse: `{"jsonrpc":"2.0","result":"0x0","id":1}`,
			expectedResult: 0,
		},
		{
			name:           "invalid hex format",
			serverResponse: `{"jsonrpc":"2.0","result":"invalid","id":1}`,
			expectedError:  "failed to parse block number",
		},
		{
			name:           "non-string result",
			serverResponse: `{"jsonrpc":"2.0","result":123,"id":1}`,
			expectedError:  "failed to unmarshal block number",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Write([]byte(tt.serverResponse))
			}))
			defer server.Close()

			client := NewEVMClient("test", server.URL, nil)
			ctx := context.Background()

			result, err := client.GetLatestBlockNumber(ctx)

			if tt.expectedError != "" {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.expectedError)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expectedResult, result)
			}
		})
	}
}

func TestEVMClient_GetBalance(t *testing.T) {
	tests := []struct {
		name           string
		address        string
		blockNumber    string
		serverResponse string
		expectedResult string
		expectedError  string
	}{
		{
			name:           "valid balance with latest block",
			address:        "0x123",
			blockNumber:    "latest",
			serverResponse: `{"jsonrpc":"2.0","result":"0xde0b6b3a7640000","id":1}`,
			expectedResult: "0xde0b6b3a7640000",
		},
		{
			name:           "valid balance with empty block number (should default to latest)",
			address:        "0x123",
			blockNumber:    "",
			serverResponse: `{"jsonrpc":"2.0","result":"0x0","id":1}`,
			expectedResult: "0x0",
		},
		{
			name:           "valid balance with specific block number",
			address:        "0x123",
			blockNumber:    "0x1234",
			serverResponse: `{"jsonrpc":"2.0","result":"0x1000","id":1}`,
			expectedResult: "0x1000",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				// Verify the request parameters
				var request RPCRequest
				json.NewDecoder(r.Body).Decode(&request)
				assert.Equal(t, "eth_getBalance", request.Method)

				params, ok := request.Params.([]interface{})
				require.True(t, ok)
				assert.Equal(t, tt.address, params[0])

				expectedBlock := tt.blockNumber
				if expectedBlock == "" {
					expectedBlock = "latest"
				}
				assert.Equal(t, expectedBlock, params[1])

				w.Write([]byte(tt.serverResponse))
			}))
			defer server.Close()

			client := NewEVMClient("test", server.URL, nil)
			ctx := context.Background()

			result, err := client.GetBalance(ctx, tt.address)

			if tt.expectedError != "" {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.expectedError)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expectedResult, result)
			}
		})
	}
}

func TestEVMClient_GetTransactionCount(t *testing.T) {
	tests := []struct {
		name           string
		address        string
		blockNumber    string
		serverResponse string
		expectedResult uint64
		expectedError  string
	}{
		{
			name:           "valid transaction count",
			address:        "0x123",
			blockNumber:    "latest",
			serverResponse: `{"jsonrpc":"2.0","result":"0x2a","id":1}`,
			expectedResult: 42,
		},
		{
			name:           "zero transaction count",
			address:        "0x123",
			blockNumber:    "latest",
			serverResponse: `{"jsonrpc":"2.0","result":"0x0","id":1}`,
			expectedResult: 0,
		},
		{
			name:           "invalid hex format",
			address:        "0x123",
			blockNumber:    "latest",
			serverResponse: `{"jsonrpc":"2.0","result":"invalid","id":1}`,
			expectedError:  "failed to parse transaction count",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Write([]byte(tt.serverResponse))
			}))
			defer server.Close()

			client := NewEVMClient("test", server.URL, nil)
			ctx := context.Background()

			result, err := client.GetTransactionCount(ctx, tt.address)

			if tt.expectedError != "" {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.expectedError)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expectedResult, result)
			}
		})
	}
}

func TestEVMClient_SendRawTransaction(t *testing.T) {
	tests := []struct {
		name           string
		signedTxData   string
		serverResponse string
		expectedResult string
		expectedError  string
	}{
		{
			name:           "valid transaction with 0x prefix",
			signedTxData:   "0xf86c808504a817c800825208943535353535353535353535353535353535353535880de0b6b3a76400008025a0",
			serverResponse: `{"jsonrpc":"2.0","result":"0x1234567890abcdef","id":1}`,
			expectedResult: "0x1234567890abcdef",
		},
		{
			name:           "valid transaction without 0x prefix (should add it)",
			signedTxData:   "f86c808504a817c800825208943535353535353535353535353535353535353535880de0b6b3a76400008025a0",
			serverResponse: `{"jsonrpc":"2.0","result":"0x1234567890abcdef","id":1}`,
			expectedResult: "0x1234567890abcdef",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				// Verify the request parameters
				var request RPCRequest
				json.NewDecoder(r.Body).Decode(&request)
				assert.Equal(t, "eth_sendRawTransaction", request.Method)

				params, ok := request.Params.([]interface{})
				require.True(t, ok)

				// Should always have 0x prefix in the request
				txData := params[0].(string)
				assert.True(t, strings.HasPrefix(txData, "0x"))

				w.Write([]byte(tt.serverResponse))
			}))
			defer server.Close()

			client := NewEVMClient("test", server.URL, nil)
			ctx := context.Background()

			result, err := client.SendRawTransaction(ctx, tt.signedTxData)

			if tt.expectedError != "" {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.expectedError)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expectedResult, result)
			}
		})
	}
}

func TestParseHexToUint64(t *testing.T) {
	tests := []struct {
		name          string
		hexStr        string
		expectedValue uint64
		expectedError string
	}{
		{
			name:          "valid hex with 0x prefix",
			hexStr:        "0x1234",
			expectedValue: 0x1234,
		},
		{
			name:          "valid hex without 0x prefix",
			hexStr:        "1234",
			expectedValue: 0x1234,
		},
		{
			name:          "zero value",
			hexStr:        "0x0",
			expectedValue: 0,
		},
		{
			name:          "large hex value",
			hexStr:        "0xffffffffffffffff",
			expectedValue: 0xffffffffffffffff,
		},
		{
			name:          "invalid hex characters",
			hexStr:        "0xghij",
			expectedError: "invalid hex string",
		},
		{
			name:          "empty string",
			hexStr:        "",
			expectedError: "invalid hex string",
		},
		{
			name:          "non-hex string",
			hexStr:        "not_hex",
			expectedError: "invalid hex string",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := parseHexToUint64(tt.hexStr)

			if tt.expectedError != "" {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.expectedError)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expectedValue, result)
			}
		})
	}
}

func TestEVMClient_ContextCancellation(t *testing.T) {
	// Test that context cancellation is properly handled
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Simulate a slow response
		time.Sleep(100 * time.Millisecond)
		w.Write([]byte(`{"jsonrpc":"2.0","result":"0x123","id":1}`))
	}))
	defer server.Close()

	client := NewEVMClient("test", server.URL, nil)

	// Create a context that will be cancelled quickly
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()

	_, err := client.Execute(ctx, "eth_blockNumber", []interface{}{})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "context deadline exceeded")
}

func TestEVMClient_NetworkError(t *testing.T) {
	// Test handling of network errors
	client := NewEVMClient("test", "http://nonexistent.example.com", nil)
	ctx := context.Background()

	_, err := client.Execute(ctx, "eth_blockNumber", []interface{}{})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to execute request")
}
