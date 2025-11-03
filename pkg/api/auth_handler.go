package api

import (
	"net/http"
	"time"

	"github.com/dvictor357/blockchain-gateway/pkg/auth"
	"github.com/gin-gonic/gin"
)

// GenerateAPIKeyRequest represents a request to generate an API key
type GenerateAPIKeyRequest struct {
	Name      string   `json:"name" binding:"required,min=3,max=50"`
	Scope     []string `json:"scope" binding:"required,min=1"`
	RateLimit int      `json:"rate_limit" binding:"min=1,max=10000"`
}

// GenerateAPIKeyResponse represents a response with a newly generated API key
type GenerateAPIKeyResponse struct {
	APIKey    string            `json:"api_key"`
	Metadata  *auth.KeyMetadata `json:"metadata"`
	CreatedAt time.Time         `json:"created_at"`
	Warning   string            `json:"warning,omitempty"`
}

// RevokeAPIKeyRequest represents a request to revoke an API key
type RevokeAPIKeyRequest struct {
	APIKey string `json:"api_key" binding:"required"`
}

// ListAPIKeysResponse represents a response listing API keys
type ListAPIKeysResponse struct {
	Keys     []*auth.KeyMetadata `json:"keys"`
	Total    int                 `json:"total"`
	Active   int                 `json:"active"`
	Inactive int                 `json:"inactive"`
}

// AuthHandler handles authentication-related API endpoints
type AuthHandler struct {
	authManager *auth.AuthManager
}

// NewAuthHandler creates a new authentication handler
func NewAuthHandler(authManager *auth.AuthManager) *AuthHandler {
	return &AuthHandler{
		authManager: authManager,
	}
}

// GenerateAPIKey generates a new API key
// @Summary      Generate API Key
// @Description  Generate a new API key for accessing the blockchain gateway
// @Tags         auth
// @Accept       json
// @Produce      json
// @Param        request  body     GenerateAPIKeyRequest  true  "API key generation request"
// @Success      200      {object} GenerateAPIKeyResponse
// @Failure      400      {object} api.SwaggerErrorResponse
// @Failure      401      {object} api.SwaggerErrorResponse
// @Failure      500      {object} api.SwaggerErrorResponse
// @Router       /api/v1/auth/api-keys [post]
func (h *AuthHandler) GenerateAPIKey(c *gin.Context) {
	var req GenerateAPIKeyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Code:  "INVALID_REQUEST",
			Error: err.Error(),
		})
		return
	}

	// Validate scope
	allowedScopes := map[string]bool{
		"read":       true,
		"write":      true,
		"admin":      true,
		"blockchain": true,
	}

	for _, scope := range req.Scope {
		if !allowedScopes[scope] {
			c.JSON(http.StatusBadRequest, ErrorResponse{
				Code:  "INVALID_SCOPE",
				Error: "Invalid scope: " + scope,
			})
			return
		}
	}

	// Use default rate limit if not specified
	rateLimit := req.RateLimit
	if rateLimit == 0 {
		rateLimit = h.authManager.GetConfig().DefaultRateLimit
	}

	// Generate API key
	apiKey, metadata, err := h.authManager.GenerateAPIKey(req.Name, req.Scope, rateLimit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Code:  "GENERATION_FAILED",
			Error: err.Error(),
		})
		return
	}

	c.JSON(http.StatusCreated, GenerateAPIKeyResponse{
		APIKey:    apiKey,
		Metadata:  metadata,
		CreatedAt: time.Now(),
		Warning:   "Save this API key now. You won't be able to see it again!",
	})
}

// RevokeAPIKey revokes an API key
// @Summary      Revoke API Key
// @Description  Revoke an API key, making it unusable
// @Tags         auth
// @Accept       json
// @Produce      json
// @Param        request  body     RevokeAPIKeyRequest  true  "Revocation request"
// @Success      200      {object} SuccessResponse
// @Failure      400      {object} api.SwaggerErrorResponse
// @Failure      401      {object} api.SwaggerErrorResponse
// @Failure      500      {object} api.SwaggerErrorResponse
// @Router       /api/v1/auth/api-keys/revoke [post]
func (h *AuthHandler) RevokeAPIKey(c *gin.Context) {
	var req RevokeAPIKeyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Code:  "INVALID_REQUEST",
			Error: err.Error(),
		})
		return
	}

	// Revoke the API key
	if err := h.authManager.RevokeAPIKey(req.APIKey); err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Code:  "REVOCATION_FAILED",
			Error: "Failed to revoke API key",
		})
		return
	}

	c.JSON(http.StatusOK, SuccessResponse{
		Success: true,
	})
}

// ListAPIKeys lists all API keys
// @Summary      List API Keys
// @Description  List all API keys (metadata only, not the actual keys)
// @Tags         auth
// @Produce      json
// @Success      200      {object} ListAPIKeysResponse
// @Failure      401      {object} api.SwaggerErrorResponse
// @Failure      500      {object} api.SwaggerErrorResponse
// @Router       /api/v1/auth/api-keys [get]
func (h *AuthHandler) ListAPIKeys(c *gin.Context) {
	keys := h.authManager.ListAPIKeys()

	// Count active/inactive
	active := 0
	inactive := 0
	for _, key := range keys {
		if key.IsActive {
			active++
		} else {
			inactive++
		}
	}

	c.JSON(http.StatusOK, ListAPIKeysResponse{
		Keys:     keys,
		Total:    len(keys),
		Active:   active,
		Inactive: inactive,
	})
}

// GetAPIKeyInfo returns information about a specific API key
// @Summary      Get API Key Info
// @Description  Get metadata about a specific API key
// @Tags         auth
// @Produce      json
// @Param        api_key  query     string  true  "API key to get info for"
// @Success      200      {object} auth.KeyMetadata
// @Failure      400      {object} api.SwaggerErrorResponse
// @Failure      401      {object} api.SwaggerErrorResponse
// @Failure      404      {object} api.SwaggerErrorResponse
// @Router       /api/v1/auth/api-keys/info [get]
func (h *AuthHandler) GetAPIKeyInfo(c *gin.Context) {
	apiKey := c.Query("api_key")
	if apiKey == "" {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Code:  "MISSING_KEY",
			Error: "API key query parameter is required",
		})
		return
	}

	metadata, err := h.authManager.GetKeyMetadata(apiKey)
	if err != nil {
		c.JSON(http.StatusNotFound, ErrorResponse{
			Code:  "KEY_NOT_FOUND",
			Error: "API key not found",
		})
		return
	}

	c.JSON(http.StatusOK, metadata)
}

// ValidateAPIKey validates an API key
// @Summary      Validate API Key
// @Description  Validate an API key without making any blockchain calls
// @Tags         auth
// @Produce      json
// @Param        api_key  query     string  true  "API key to validate"
// @Success      200      {object} ValidationResponse
// @Failure      400      {object} api.SwaggerErrorResponse
// @Router       /api/v1/auth/validate [get]
func (h *AuthHandler) ValidateAPIKey(c *gin.Context) {
	apiKey := c.Query("api_key")
	if apiKey == "" {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Code:  "MISSING_KEY",
			Error: "API key query parameter is required",
		})
		return
	}

	keyInfo, err := h.authManager.ValidateAPIKey(apiKey)

	response := ValidationResponse{
		Valid: false,
	}

	if err != nil {
		response.Valid = false
		response.Message = err.Error()
	} else {
		response.Valid = true
		response.Message = "API key is valid"
		response.RateLimit = keyInfo.RateLimit
		response.Scope = keyInfo.Scope
		response.IsActive = keyInfo.IsActive
		if keyInfo.ExpiresAt != nil {
			response.ExpiresAt = *keyInfo.ExpiresAt
		}
	}

	c.JSON(http.StatusOK, response)
}

// ValidationResponse represents an API key validation response
type ValidationResponse struct {
	Valid     bool      `json:"valid"`
	Message   string    `json:"message"`
	RateLimit int       `json:"rate_limit,omitempty"`
	Scope     []string  `json:"scope,omitempty"`
	IsActive  bool      `json:"is_active,omitempty"`
	ExpiresAt time.Time `json:"expires_at,omitempty"`
}

// SuccessResponse represents a successful operation response
type SuccessResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
}
