package api

import (
	"errors"
	"fmt"
	"log"
	"net/http"
	"strings"

	"github.com/dvictor357/blaze"
	"github.com/dvictor357/blockchain-gateway/pkg/blockchain"
	"github.com/dvictor357/blockchain-gateway/pkg/validation"
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
func (eh *ErrorHandler) HandleValidationErrors(c *blaze.Context, errors validation.ValidationErrors) error {
	if !errors.HasErrors() {
		return nil
	}

	response := ErrorResponse{
		Error:   "Validation failed",
		Code:    "VALIDATION_ERROR",
		Details: errors,
	}

	return c.JSON(http.StatusBadRequest, response)
}

// HandleBlockchainError handles blockchain-specific errors
func (eh *ErrorHandler) HandleBlockchainError(c *blaze.Context, err error, operation string, chain string) error {
	switch {
	case errors.Is(err, blockchain.ErrChainNotSupported):
		return eh.RespondWithError(c, http.StatusNotFound, "CHAIN_NOT_SUPPORTED",
			"Blockchain '%s' is not supported", chain)

	case errors.Is(err, blockchain.ErrInvalidRequest):
		return eh.RespondWithError(c, http.StatusBadRequest, "INVALID_REQUEST",
			"Invalid RPC request")

	case errors.Is(err, blockchain.ErrRPCTimeout):
		return eh.RespondWithError(c, http.StatusGatewayTimeout, "RPC_TIMEOUT",
			"RPC request timed out")

	case strings.Contains(err.Error(), "not found"):
		return eh.RespondWithError(c, http.StatusNotFound, "NOT_FOUND",
			"Resource not found")

	case strings.Contains(err.Error(), "not applicable"):
		return eh.RespondWithError(c, http.StatusBadRequest, "NOT_APPLICABLE",
			"Operation not applicable for blockchain: %s", chain)

	default:
		// Log the detailed error but return a generic message to the client
		eh.logger.Printf("Error during %s for %s: %v", operation, chain, err)
		return eh.RespondWithError(c, http.StatusInternalServerError, "INTERNAL_ERROR",
			"Failed to %s", operation)
	}
}

// RespondWithError sends a standardized error response
func (eh *ErrorHandler) RespondWithError(c *blaze.Context, statusCode int, code string, format string, args ...interface{}) error {
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

	return c.JSON(statusCode, response)
}

// HandleGenericError handles generic errors with logging
func (eh *ErrorHandler) HandleGenericError(c *blaze.Context, err error, operation string) error {
	eh.logger.Printf("Error during %s: %v", operation, err)
	return eh.RespondWithError(c, http.StatusInternalServerError, "INTERNAL_ERROR",
		"Failed to %s", operation)
}

// HandleBindingError handles JSON binding errors
func (eh *ErrorHandler) HandleBindingError(c *blaze.Context, err error) error {
	eh.logger.Printf("JSON binding error: %v", err)
	return eh.RespondWithError(c, http.StatusBadRequest, "INVALID_JSON",
		"Invalid request body format")
}
