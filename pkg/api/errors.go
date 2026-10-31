package api

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"

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

// HandleBlockchainError handles blockchain-specific errors. Upstream details are logged but
// never included in the response.
func (eh *ErrorHandler) HandleBlockchainError(c *blaze.Context, err error, operation string, chain string) error {
	var rpcErr *blockchain.RPCError

	switch {
	case errors.Is(err, blockchain.ErrChainNotSupported):
		var unsupported *blockchain.UnsupportedChainError
		if errors.As(err, &unsupported) {
			chain = unsupported.Chain
		}
		return eh.RespondWithError(c, http.StatusNotFound, "CHAIN_NOT_SUPPORTED",
			"Blockchain '%s' is not supported", chain)

	case errors.Is(err, blockchain.ErrInvalidRequest):
		return eh.RespondWithError(c, http.StatusBadRequest, "INVALID_REQUEST",
			"Invalid RPC request")

	case errors.Is(err, blockchain.ErrOperationNotSupported):
		return eh.RespondWithError(c, http.StatusNotImplemented, "OPERATION_NOT_SUPPORTED",
			"Failed to %s: not supported for blockchain '%s'", operation, chain)

	case errors.Is(err, blockchain.ErrNotApplicable):
		return eh.RespondWithError(c, http.StatusBadRequest, "NOT_APPLICABLE",
			"Operation not applicable for blockchain: %s", chain)

	case errors.Is(err, blockchain.ErrNotFound):
		return eh.RespondWithError(c, http.StatusNotFound, "NOT_FOUND",
			"Resource not found")

	case blockchain.IsTimeout(err):
		return eh.RespondWithError(c, http.StatusGatewayTimeout, "RPC_TIMEOUT",
			"RPC request timed out")

	case errors.Is(err, context.Canceled):
		// The client went away; this is not a server timeout or an upstream fault.
		return eh.internalError(c, err, operation, chain)

	case errors.As(err, &rpcErr):
		eh.logger.Printf("Upstream RPC error during %s for %s: code=%d message=%q", operation, chain, rpcErr.Code, rpcErr.Message)
		return eh.RespondWithError(c, http.StatusBadGateway, "UPSTREAM_RPC_ERROR",
			"Upstream node returned an RPC error")

	case errors.Is(err, blockchain.ErrUpstream):
		eh.logger.Printf("Upstream failure during %s for %s: %v", operation, chain, err)
		return eh.RespondWithError(c, http.StatusBadGateway, "UPSTREAM_ERROR",
			"Upstream node request failed")

	default:
		return eh.internalError(c, err, operation, chain)
	}
}

// internalError logs the detailed error but returns a generic message to the client.
func (eh *ErrorHandler) internalError(c *blaze.Context, err error, operation string, chain string) error {
	eh.logger.Printf("Error during %s for %s: %v", operation, chain, err)
	return eh.RespondWithError(c, http.StatusInternalServerError, "INTERNAL_ERROR",
		"Failed to %s", operation)
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

// HandleBindingError handles request body errors: oversized bodies are 413, everything else 400.
func (eh *ErrorHandler) HandleBindingError(c *blaze.Context, err error) error {
	var tooLarge *http.MaxBytesError
	if errors.As(err, &tooLarge) {
		return eh.RespondWithError(c, http.StatusRequestEntityTooLarge, "REQUEST_TOO_LARGE",
			"Request body exceeds the limit of %d bytes", tooLarge.Limit)
	}

	eh.logger.Printf("JSON binding error: %v", err)
	return eh.RespondWithError(c, http.StatusBadRequest, "INVALID_JSON",
		"Invalid request body format")
}
