package api

import (
	"errors"
	"fmt"
	"log"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/user/blockchain-gateway/pkg/blockchain"
	"github.com/user/blockchain-gateway/pkg/validation"
)

// ErrorResponse represents a standardized error response
type ErrorResponse struct {
	Error   string                       `json:"error"`
	Code    string                       `json:"code,omitempty"`
	Details []validation.ValidationError `json:"details,omitempty"`
}

// ErrorHandler provides common error handling methods
type ErrorHandler struct {
	logger *log.Logger
}

// NewErrorHandler creates a new error handler
func NewErrorHandler(logger *log.Logger) *ErrorHandler {
	return &ErrorHandler{
		logger: logger,
	}
}

// HandleValidationErrors handles validation errors and returns appropriate HTTP response
func (eh *ErrorHandler) HandleValidationErrors(c *gin.Context, errors validation.ValidationErrors) {
	if !errors.HasErrors() {
		return
	}

	response := ErrorResponse{
		Error:   "Validation failed",
		Code:    "VALIDATION_ERROR",
		Details: errors,
	}

	c.JSON(http.StatusBadRequest, response)
}

// HandleBlockchainError handles blockchain-specific errors
func (eh *ErrorHandler) HandleBlockchainError(c *gin.Context, err error, operation string, chain string) {
	switch {
	case errors.Is(err, blockchain.ErrChainNotSupported):
		eh.RespondWithError(c, http.StatusNotFound, "CHAIN_NOT_SUPPORTED",
			"Blockchain '%s' is not supported", chain)

	case errors.Is(err, blockchain.ErrInvalidRequest):
		eh.RespondWithError(c, http.StatusBadRequest, "INVALID_REQUEST",
			"Invalid RPC request")

	case errors.Is(err, blockchain.ErrRPCTimeout):
		eh.RespondWithError(c, http.StatusGatewayTimeout, "RPC_TIMEOUT",
			"RPC request timed out")

	case strings.Contains(err.Error(), "not found"):
		eh.RespondWithError(c, http.StatusNotFound, "NOT_FOUND",
			"Resource not found")

	case strings.Contains(err.Error(), "not applicable"):
		eh.RespondWithError(c, http.StatusBadRequest, "NOT_APPLICABLE",
			"Operation not applicable for blockchain: %s", chain)

	default:
		// Log the detailed error but return a generic message to the client
		eh.logger.Printf("Error during %s for %s: %v", operation, chain, err)
		eh.RespondWithError(c, http.StatusInternalServerError, "INTERNAL_ERROR",
			"Failed to %s", operation)
	}
}

// RespondWithError sends a standardized error response
func (eh *ErrorHandler) RespondWithError(c *gin.Context, statusCode int, code string, format string, args ...interface{}) {
	var message string
	if len(args) > 0 {
		message = fmt.Sprintf(format, args...)
	} else {
		message = format
	}

	response := ErrorResponse{
		Error: message,
		Code:  code,
	}

	c.JSON(statusCode, response)
}

// HandleGenericError handles generic errors with logging
func (eh *ErrorHandler) HandleGenericError(c *gin.Context, err error, operation string) {
	eh.logger.Printf("Error during %s: %v", operation, err)
	eh.RespondWithError(c, http.StatusInternalServerError, "INTERNAL_ERROR",
		"Failed to %s", operation)
}

// HandleBindingError handles JSON binding errors
func (eh *ErrorHandler) HandleBindingError(c *gin.Context, err error) {
	eh.logger.Printf("JSON binding error: %v", err)
	eh.RespondWithError(c, http.StatusBadRequest, "INVALID_JSON",
		"Invalid request body format")
}

// RequireParams validates that required parameters are present
func (eh *ErrorHandler) RequireParams(c *gin.Context, params map[string]string) bool {
	var missing []string

	for name, value := range params {
		if value == "" {
			missing = append(missing, name)
		}
	}

	if len(missing) > 0 {
		eh.RespondWithError(c, http.StatusBadRequest, "MISSING_PARAMS",
			"Missing required parameters: %s", strings.Join(missing, ", "))
		return false
	}

	return true
}
