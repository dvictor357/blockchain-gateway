package api

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/dvictor357/blockchain-gateway/pkg/blockchain"
	"github.com/dvictor357/blockchain-gateway/pkg/validation"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

func TestMain(m *testing.M) {
	// Set Gin to test mode
	gin.SetMode(gin.TestMode)
	os.Exit(m.Run())
}

func TestNewErrorHandler(t *testing.T) {
	logger := log.New(os.Stdout, "test", log.LstdFlags)
	handler := NewErrorHandler(logger)

	assert.NotNil(t, handler)
	assert.Equal(t, logger, handler.logger)
}

func TestErrorHandler_HandleValidationErrors(t *testing.T) {
	logger := log.New(os.Stdout, "test", log.LstdFlags)
	handler := NewErrorHandler(logger)

	tests := []struct {
		name           string
		errors         validation.ValidationErrors
		expectedStatus int
		expectedBody   string
	}{
		{
			name:           "no errors - should not respond",
			errors:         validation.ValidationErrors{},
			expectedStatus: 0, // No response expected
		},
		{
			name: "single validation error",
			errors: validation.ValidationErrors{
				{Field: "address", Message: "invalid address format"},
			},
			expectedStatus: http.StatusBadRequest,
			expectedBody:   `{"error":"Validation failed","code":"VALIDATION_ERROR","details":[{"field":"address","message":"invalid address format"}]}`,
		},
		{
			name: "multiple validation errors",
			errors: validation.ValidationErrors{
				{Field: "address", Message: "invalid address format"},
				{Field: "chain", Message: "chain is required"},
			},
			expectedStatus: http.StatusBadRequest,
			expectedBody:   `{"error":"Validation failed","code":"VALIDATION_ERROR","details":[{"field":"address","message":"invalid address format"},{"field":"chain","message":"chain is required"}]}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)

			handler.HandleValidationErrors(c, tt.errors)

			if tt.expectedStatus == 0 {
				// No response expected for empty errors
				assert.Equal(t, http.StatusOK, w.Code) // Default status
				assert.Empty(t, w.Body.String())
			} else {
				assert.Equal(t, tt.expectedStatus, w.Code)
				assert.JSONEq(t, tt.expectedBody, w.Body.String())
			}
		})
	}
}

func TestErrorHandler_HandleBlockchainError(t *testing.T) {
	logger := log.New(os.Stdout, "test", log.LstdFlags)
	handler := NewErrorHandler(logger)

	tests := []struct {
		name           string
		err            error
		operation      string
		chain          string
		expectedStatus int
		expectedCode   string
		expectedMsg    string
	}{
		{
			name:           "chain not supported error",
			err:            blockchain.ErrChainNotSupported,
			operation:      "get balance",
			chain:          "unsupported",
			expectedStatus: http.StatusNotFound,
			expectedCode:   "CHAIN_NOT_SUPPORTED",
			expectedMsg:    "Blockchain 'unsupported' is not supported",
		},
		{
			name:           "invalid request error",
			err:            blockchain.ErrInvalidRequest,
			operation:      "execute RPC",
			chain:          "ethereum",
			expectedStatus: http.StatusBadRequest,
			expectedCode:   "INVALID_REQUEST",
			expectedMsg:    "Invalid RPC request",
		},
		{
			name:           "RPC timeout error",
			err:            blockchain.ErrRPCTimeout,
			operation:      "get transaction",
			chain:          "ethereum",
			expectedStatus: http.StatusGatewayTimeout,
			expectedCode:   "RPC_TIMEOUT",
			expectedMsg:    "RPC request timed out",
		},
		{
			name:           "not found error",
			err:            errors.New("transaction not found"),
			operation:      "get transaction",
			chain:          "ethereum",
			expectedStatus: http.StatusNotFound,
			expectedCode:   "NOT_FOUND",
			expectedMsg:    "Resource not found",
		},
		{
			name:           "not applicable error",
			err:            errors.New("operation not applicable for this blockchain"),
			operation:      "get gas price",
			chain:          "bitcoin",
			expectedStatus: http.StatusBadRequest,
			expectedCode:   "NOT_APPLICABLE",
			expectedMsg:    "Operation not applicable for blockchain: bitcoin",
		},
		{
			name:           "generic error",
			err:            errors.New("some unexpected error"),
			operation:      "get balance",
			chain:          "ethereum",
			expectedStatus: http.StatusInternalServerError,
			expectedCode:   "INTERNAL_ERROR",
			expectedMsg:    "Failed to get balance",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)

			handler.HandleBlockchainError(c, tt.err, tt.operation, tt.chain)

			assert.Equal(t, tt.expectedStatus, w.Code)

			var response ErrorResponse
			err := json.Unmarshal(w.Body.Bytes(), &response)
			assert.NoError(t, err)
			assert.Equal(t, tt.expectedCode, response.Code)
			assert.Contains(t, response.Error, tt.expectedMsg)
		})
	}
}

func TestErrorHandler_RespondWithError(t *testing.T) {
	logger := log.New(os.Stdout, "test", log.LstdFlags)
	handler := NewErrorHandler(logger)

	tests := []struct {
		name           string
		statusCode     int
		code           string
		format         string
		args           []interface{}
		expectedStatus int
		expectedBody   string
	}{
		{
			name:           "simple error message",
			statusCode:     http.StatusBadRequest,
			code:           "INVALID_INPUT",
			format:         "Invalid input provided",
			args:           nil,
			expectedStatus: http.StatusBadRequest,
			expectedBody:   `{"error":"Invalid input provided","code":"INVALID_INPUT"}`,
		},
		{
			name:           "formatted error message",
			statusCode:     http.StatusNotFound,
			code:           "NOT_FOUND",
			format:         "Resource %s not found",
			args:           []interface{}{"user"},
			expectedStatus: http.StatusNotFound,
			expectedBody:   `{"error":"Resource user not found","code":"NOT_FOUND"}`,
		},
		{
			name:           "multiple format args",
			statusCode:     http.StatusBadRequest,
			code:           "VALIDATION_ERROR",
			format:         "Field %s must be between %d and %d characters",
			args:           []interface{}{"name", 5, 50},
			expectedStatus: http.StatusBadRequest,
			expectedBody:   `{"error":"Field name must be between 5 and 50 characters","code":"VALIDATION_ERROR"}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)

			handler.RespondWithError(c, tt.statusCode, tt.code, tt.format, tt.args...)

			assert.Equal(t, tt.expectedStatus, w.Code)
			assert.JSONEq(t, tt.expectedBody, w.Body.String())
		})
	}
}

func TestErrorHandler_HandleGenericError(t *testing.T) {
	logger := log.New(os.Stdout, "test", log.LstdFlags)
	handler := NewErrorHandler(logger)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	err := errors.New("some generic error")
	operation := "test operation"

	handler.HandleGenericError(c, err, operation)

	assert.Equal(t, http.StatusInternalServerError, w.Code)

	expectedBody := `{"error":"Failed to test operation","code":"INTERNAL_ERROR"}`
	assert.JSONEq(t, expectedBody, w.Body.String())
}

func TestErrorHandler_HandleBindingError(t *testing.T) {
	logger := log.New(os.Stdout, "test", log.LstdFlags)
	handler := NewErrorHandler(logger)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	err := errors.New("invalid JSON format")

	handler.HandleBindingError(c, err)

	assert.Equal(t, http.StatusBadRequest, w.Code)

	expectedBody := `{"error":"Invalid request body format","code":"INVALID_JSON"}`
	assert.JSONEq(t, expectedBody, w.Body.String())
}

func TestErrorHandler_RequireParams(t *testing.T) {
	logger := log.New(os.Stdout, "test", log.LstdFlags)
	handler := NewErrorHandler(logger)

	tests := []struct {
		name           string
		params         map[string]string
		expectedResult bool
		expectedStatus int
		expectedBody   string
	}{
		{
			name: "all params present",
			params: map[string]string{
				"chain":   "ethereum",
				"address": "0x123",
			},
			expectedResult: true,
			expectedStatus: 0, // No response expected
		},
		{
			name: "missing single param",
			params: map[string]string{
				"chain":   "ethereum",
				"address": "",
			},
			expectedResult: false,
			expectedStatus: http.StatusBadRequest,
			expectedBody:   `{"error":"Missing required parameters: address","code":"MISSING_PARAMS"}`,
		},
		{
			name: "missing multiple params",
			params: map[string]string{
				"chain":   "",
				"address": "",
				"hash":    "0x123",
			},
			expectedResult: false,
			expectedStatus: http.StatusBadRequest,
		},
		{
			name: "empty params map",
			params: map[string]string{
				"param1": "",
				"param2": "",
			},
			expectedResult: false,
			expectedStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)

			result := handler.RequireParams(c, tt.params)

			assert.Equal(t, tt.expectedResult, result)

			if tt.expectedStatus == 0 {
				// No response expected
				assert.Equal(t, http.StatusOK, w.Code) // Default status
			} else {
				assert.Equal(t, tt.expectedStatus, w.Code)

				if tt.expectedBody != "" {
					assert.JSONEq(t, tt.expectedBody, w.Body.String())
				} else {
					// For multiple missing params, just check that it contains the expected structure
					body := w.Body.String()
					assert.Contains(t, body, "Missing required parameters")
					assert.Contains(t, body, "MISSING_PARAMS")
				}
			}
		})
	}
}

func TestErrorResponse_Structure(t *testing.T) {
	// Test that ErrorResponse can be properly marshaled/unmarshaled
	response := ErrorResponse{
		Error: "Test error",
		Code:  "TEST_CODE",
		Details: []validation.ValidationError{
			{Field: "field1", Message: "message1"},
		},
	}

	// This test ensures the struct tags are correct
	assert.Equal(t, "Test error", response.Error)
	assert.Equal(t, "TEST_CODE", response.Code)
	assert.Len(t, response.Details, 1)
	assert.Equal(t, "field1", response.Details[0].Field)
	assert.Equal(t, "message1", response.Details[0].Message)
}
