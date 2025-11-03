package api

import (
	"fmt"
	"log"
	"net/http"

	"github.com/dvictor357/blockchain-gateway/pkg/blockchain"
	"github.com/gin-gonic/gin"
)

// CacheHandler manages cache-related API operations
type CacheHandler struct {
	cachedClientManager *blockchain.CachedClientManager
	logger              *log.Logger
}

// NewCacheHandler creates a new cache handler
func NewCacheHandler(cachedClientManager *blockchain.CachedClientManager, logger *log.Logger) *CacheHandler {
	return &CacheHandler{
		cachedClientManager: cachedClientManager,
		logger:              logger,
	}
}

// GetCacheStats returns comprehensive cache statistics
// @Summary      Get Cache Statistics
// @Description  Get detailed statistics for all cache layers (L1, L2, L3)
// @Tags         cache
// @Produce      json
// @Success      200  {object}  map[string]interface{}
// @Failure      500  {object}  api.SwaggerErrorResponse
// @Router       /api/v1/cache/stats [get]
func (h *CacheHandler) GetCacheStats(c *gin.Context) {
	stats, err := h.cachedClientManager.GetCacheStats(c.Request.Context())
	if err != nil {
		h.logger.Printf("Error getting cache stats: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": fmt.Sprintf("failed to get cache stats: %v", err),
		})
		return
	}
	c.JSON(http.StatusOK, stats)
}

// InvalidateCache invalidates cache entries matching a pattern
// @Summary      Invalidate Cache
// @Description  Invalidate cache entries that match the specified pattern
// @Tags         cache
// @Produce      json
// @Param        pattern  query     string  true  "Pattern to match cache keys (e.g., 'balance:ethereum:*')"
// @Success      200      {object}  api.CacheInvalidateResponse
// @Failure      400      {object}  api.SwaggerErrorResponse
// @Failure      500      {object}  api.SwaggerErrorResponse
// @Router       /api/v1/cache/invalidate [delete]
func (h *CacheHandler) InvalidateCache(c *gin.Context) {
	pattern := c.Query("pattern")
	if pattern == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "pattern query parameter is required",
		})
		return
	}

	h.logger.Printf("Invalidating cache with pattern: %s", pattern)
	err := h.cachedClientManager.InvalidateCache(c.Request.Context(), pattern)
	if err != nil {
		h.logger.Printf("Error invalidating cache with pattern %s: %v", pattern, err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": fmt.Sprintf("failed to invalidate cache: %v", err),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "cache invalidated successfully",
		"pattern": pattern,
	})
}

// ClearAllCache clears all cache entries from all layers
// @Summary      Clear All Cache
// @Description  Clear all cache entries from L1, L2, and L3 layers
// @Tags         cache
// @Produce      json
// @Success      200  {object}  api.CacheClearResponse
// @Failure      500  {object}  api.SwaggerErrorResponse
// @Router       /api/v1/cache/clear [delete]
func (h *CacheHandler) ClearAllCache(c *gin.Context) {
	ctx := c.Request.Context()

	h.logger.Println("Clearing all cache layers")

	// Invalidate with wildcard pattern to clear everything
	patterns := []string{
		"rpc:*",
		"balance:*",
		"block:*",
		"gas_price:*",
		"nonce:*",
		"tx:*",
	}

	for _, pattern := range patterns {
		err := h.cachedClientManager.InvalidateCache(ctx, pattern)
		if err != nil {
			h.logger.Printf("Warning: failed to clear pattern %s: %v", pattern, err)
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"message":          "all cache layers cleared successfully",
		"patterns_cleared": patterns,
	})
}

// GetLayerStats returns statistics for a specific cache layer
// @Summary      Get Layer Statistics
// @Description  Get statistics for a specific cache layer (l1, l2, or l3)
// @Tags         cache
// @Produce      json
// @Param        layer  path      string  true  "Cache layer name" Enums(l1, l2, l3)
// @Success      200    {object}  map[string]interface{}
// @Failure      400    {object}  api.SwaggerErrorResponse
// @Failure      500    {object}  api.SwaggerErrorResponse
// @Router       /api/v1/cache/layer/{layer} [get]
func (h *CacheHandler) GetLayerStats(c *gin.Context) {
	layer := c.Param("layer")

	stats, err := h.cachedClientManager.GetCacheStats(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": fmt.Sprintf("failed to get cache stats: %v", err),
		})
		return
	}

	layerStats, exists := stats[layer]
	if !exists {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": fmt.Sprintf("invalid layer: %s. Valid layers: l1, l2, l3", layer),
		})
		return
	}

	c.JSON(http.StatusOK, layerStats)
}
