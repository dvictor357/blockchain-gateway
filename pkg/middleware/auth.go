package middleware

import (
	"net/http"
	"strings"

	"github.com/dvictor357/blockchain-gateway/pkg/auth"
	"github.com/gin-gonic/gin"
)

// AuthMiddleware validates API keys
func AuthMiddleware(authManager *auth.AuthManager) gin.HandlerFunc {
	return func(c *gin.Context) {
		// Get API key from header
		apiKey := extractAPIKey(c)

		if apiKey == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"error":   "Unauthorized",
				"code":    "MISSING_API_KEY",
				"message": "API key is required. Please provide it in the X-API-Key header.",
			})
			return
		}

		// Validate API key
		keyInfo, err := authManager.ValidateAPIKey(apiKey)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"error":   "Unauthorized",
				"code":    "INVALID_API_KEY",
				"message": "Invalid or expired API key",
			})
			return
		}

		// Store key info in context
		c.Set("api_key", apiKey)
		c.Set("api_key_info", keyInfo)

		// Add rate limit headers
		c.Header("X-RateLimit-Limit", "0")     // Will be set by rate limiter
		c.Header("X-RateLimit-Remaining", "0") // Will be set by rate limiter

		c.Next()
	}
}

// OptionalAuthMiddleware validates API key if provided (but doesn't require it)
func OptionalAuthMiddleware(authManager *auth.AuthManager) gin.HandlerFunc {
	return func(c *gin.Context) {
		apiKey := extractAPIKey(c)

		if apiKey != "" {
			keyInfo, err := authManager.ValidateAPIKey(apiKey)
			if err != nil {
				// Key provided but invalid - log but continue
				c.Header("X-API-Key-Status", "invalid")
			} else {
				c.Set("api_key", apiKey)
				c.Set("api_key_info", keyInfo)
				c.Header("X-API-Key-Status", "valid")
			}
		} else {
			c.Header("X-API-Key-Status", "not_provided")
		}

		c.Next()
	}
}

// ScopeMiddleware checks if the API key has the required scope
func ScopeMiddleware(requiredScope string) gin.HandlerFunc {
	return func(c *gin.Context) {
		keyInfo, exists := c.Get("api_key_info")
		if !exists {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"error":   "Unauthorized",
				"code":    "NO_API_KEY",
				"message": "API key is required",
			})
			return
		}

		info, ok := keyInfo.(*auth.APIKeyInfo)
		if !ok {
			c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{
				"error":   "Internal Error",
				"code":    "INVALID_KEY_INFO",
				"message": "Invalid API key info in context",
			})
			return
		}

		// Check if key has required scope
		hasScope := false
		for _, scope := range info.Scope {
			if scope == requiredScope || scope == "*" {
				hasScope = true
				break
			}
		}

		if !hasScope {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{
				"error":   "Forbidden",
				"code":    "INSUFFICIENT_SCOPE",
				"message": "API key does not have the required scope: " + requiredScope,
			})
			return
		}

		c.Next()
	}
}

// ChainAccessMiddleware checks if the API key can access the specified chain
func ChainAccessMiddleware(chainParam string) gin.HandlerFunc {
	return func(c *gin.Context) {
		keyInfo, exists := c.Get("api_key_info")
		if !exists {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"error":   "Unauthorized",
				"code":    "NO_API_KEY",
				"message": "API key is required",
			})
			return
		}

		info, ok := keyInfo.(*auth.APIKeyInfo)
		if !ok {
			c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{
				"error":   "Internal Error",
				"code":    "INVALID_KEY_INFO",
				"message": "Invalid API key info in context",
			})
			return
		}

		chain := c.Param(chainParam)

		// If no specific chains are defined, allow all
		if len(info.AllowedChains) == 0 {
			c.Next()
			return
		}

		// Check if key can access this chain
		canAccess := false
		for _, allowedChain := range info.AllowedChains {
			if allowedChain == chain {
				canAccess = true
				break
			}
		}

		if !canAccess {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{
				"error":   "Forbidden",
				"code":    "CHAIN_NOT_ALLOWED",
				"message": "API key does not have access to chain: " + chain,
			})
			return
		}

		c.Next()
	}
}

// MethodAccessMiddleware checks if the API key can use the specified HTTP method
func MethodAccessMiddleware(requiredMethod string) gin.HandlerFunc {
	return func(c *gin.Context) {
		keyInfo, exists := c.Get("api_key_info")
		if !exists {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"error":   "Unauthorized",
				"code":    "NO_API_KEY",
				"message": "API key is required",
			})
			return
		}

		info, ok := keyInfo.(*auth.APIKeyInfo)
		if !ok {
			c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{
				"error":   "Internal Error",
				"code":    "INVALID_KEY_INFO",
				"message": "Invalid API key info in context",
			})
			return
		}

		// If no specific methods are defined, allow all
		if len(info.AllowedMethods) == 0 {
			c.Next()
			return
		}

		// Check if key can use this method
		canUse := false
		for _, allowedMethod := range info.AllowedMethods {
			if allowedMethod == requiredMethod {
				canUse = true
				break
			}
		}

		if !canUse {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{
				"error":   "Forbidden",
				"code":    "METHOD_NOT_ALLOWED",
				"message": "API key does not have permission to use method: " + requiredMethod,
			})
			return
		}

		c.Next()
	}
}

// GetAPIKey returns the API key from the context
func GetAPIKey(c *gin.Context) (string, bool) {
	apiKey, exists := c.Get("api_key")
	if !exists {
		return "", false
	}
	return apiKey.(string), true
}

// GetAPIKeyInfo returns the API key info from the context
func GetAPIKeyInfo(c *gin.Context) (*auth.APIKeyInfo, bool) {
	info, exists := c.Get("api_key_info")
	if !exists {
		return nil, false
	}
	keyInfo, ok := info.(*auth.APIKeyInfo)
	return keyInfo, ok
}

// extractAPIKey extracts API key from request headers
func extractAPIKey(c *gin.Context) string {
	// Try X-API-Key header
	if key := c.GetHeader("X-API-Key"); key != "" {
		return key
	}

	// Try Authorization header with Bearer token
	if authHeader := c.GetHeader("Authorization"); authHeader != "" {
		if strings.HasPrefix(authHeader, "Bearer ") {
			return strings.TrimPrefix(authHeader, "Bearer ")
		}
	}

	// Try query parameter as fallback
	if key := c.Query("api_key"); key != "" {
		return key
	}

	return ""
}

// AdminAuthMiddleware for admin-only endpoints
func AdminAuthMiddleware(authManager *auth.AuthManager) gin.HandlerFunc {
	return func(c *gin.Context) {
		// First check for valid API key
		keyInfo, exists := c.Get("api_key_info")
		if !exists {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"error":   "Unauthorized",
				"code":    "NO_API_KEY",
				"message": "Admin access requires API key",
			})
			return
		}

		info, ok := keyInfo.(*auth.APIKeyInfo)
		if !ok {
			c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{
				"error":   "Internal Error",
				"code":    "INVALID_KEY_INFO",
				"message": "Invalid API key info",
			})
			return
		}

		// Check if key has admin scope
		hasAdminScope := false
		for _, scope := range info.Scope {
			if scope == "admin" || scope == "*" {
				hasAdminScope = true
				break
			}
		}

		if !hasAdminScope {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{
				"error":   "Forbidden",
				"code":    "INSUFFICIENT_PRIVILEGES",
				"message": "Admin access required",
			})
			return
		}

		c.Next()
	}
}
