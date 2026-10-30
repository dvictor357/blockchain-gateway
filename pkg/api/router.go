package api

import (
	"log"
	"net/http"
	"sort"
	"sync"
	"time"

	"github.com/dvictor357/blaze"
	_ "github.com/dvictor357/blockchain-gateway/docs"
	"github.com/dvictor357/blockchain-gateway/pkg/config"
	swaggerFiles "github.com/swaggo/files"
	"github.com/swaggo/swag"
)

// Render once: swag.Spec.ReadDoc mutates the registered spec while rendering it.
var swaggerDocument = sync.OnceValues(func() ([]byte, error) {
	doc, err := swag.ReadDoc()
	return []byte(doc), err
})

// NewRouter builds the gateway's Blaze routes and shared request middleware.
func NewRouter(handler *Handler, logger *log.Logger, serverConfig config.ServerConfig) http.Handler {
	engine := blaze.New()
	routes := make([]string, 0, 13)
	register := func(group *blaze.Group, prefix, method, path string, h blaze.HandlerFunc) {
		group.Handle(method, path, h)
		routes = append(routes, method+" "+prefix+path)
	}

	root := engine.Group("")
	register(root, "", http.MethodGet, "/health", healthCheckHandler)
	register(root, "", http.MethodGet, "/swagger", func(c *blaze.Context) error {
		return c.Redirect(http.StatusMovedPermanently, "/swagger/index.html")
	})
	register(root, "", http.MethodGet, "/swagger/*any", swaggerHandler(logger))

	apiV1 := engine.Group("/api/v1")
	for _, route := range []struct {
		method  string
		path    string
		handler blaze.HandlerFunc
	}{
		{http.MethodGet, "/chains", handler.ListChains},
		{http.MethodPost, "/chains/:chain/query", handler.QueryChain},
		{http.MethodPost, "/batch", handler.BatchQuery},
		{http.MethodGet, "/chains/:chain/address/:address/balance", handler.GetBalance},
		{http.MethodGet, "/chains/:chain/block/latest", handler.GetLatestBlock},
		{http.MethodGet, "/chains/:chain/tx/:hash", handler.GetTransaction},
		{http.MethodGet, "/chains/:chain/gas-price", handler.GetGasPrice},
		{http.MethodGet, "/chains/:chain/address/:address/nonce", handler.GetTransactionCount},
		{http.MethodGet, "/markets", handler.GetCoinMarkets},
	} {
		register(apiV1, "/api/v1", route.method, route.path, route.handler)
	}

	if serverConfig.Mode == "debug" {
		logger.Println("Running in development mode (debug)")
		register(root, "", http.MethodGet, "/debug/routes", func(c *blaze.Context) error {
			return c.JSON(http.StatusOK, map[string]any{
				"routes": routes,
				"count":  len(routes),
				"mode":   "debug",
			})
		})
	}
	sort.Strings(routes)

	// Blaze applies Engine.Use only to matched routes. Keep the shared middleware
	// at the HTTP boundary so unknown paths and methods are logged and limited too.
	serve := LoggingMiddleware(logger)(blaze.Recovery()(RateLimit(serverConfig.RateLimit)(func(c *blaze.Context) error {
		engine.ServeHTTP(c.ResponseWriter, c.Request)
		return nil
	})))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := serve(&blaze.Context{ResponseWriter: w, Request: r}); err != nil {
			logger.Printf("Failed to write HTTP response: %v", err)
		}
	})
}

// healthCheckHandler handles health check requests.
// @Summary      Health Check
// @Description  Check if the API is running and healthy
// @Tags         health
// @Accept       json
// @Produce      json
// @Success      200  {object}  api.HealthResponse
// @Router       /health [get]
func healthCheckHandler(c *blaze.Context) error {
	return c.JSON(http.StatusOK, HealthResponse{
		Status: "ok",
		Time:   time.Now().Format(time.RFC3339),
	})
}

func swaggerHandler(logger *log.Logger) blaze.HandlerFunc {
	doc, docErr := swaggerDocument()
	if docErr != nil {
		logger.Printf("Failed to render Swagger documentation: %v", docErr)
	}
	assets := http.StripPrefix("/swagger", swaggerFiles.Handler)
	return func(c *blaze.Context) error {
		switch c.Param("any") {
		case "index.html":
			return c.HTML(http.StatusOK, swaggerIndexHTML)
		case "doc.json":
			if docErr != nil {
				return c.JSON(http.StatusInternalServerError, ErrorResponse{
					Error: "Failed to retrieve API documentation",
					Code:  "INTERNAL_ERROR",
				})
			}
			c.SetHeader("Content-Type", "application/json; charset=utf-8")
			_, err := c.ResponseWriter.Write(doc)
			return err
		default:
			assets.ServeHTTP(c.ResponseWriter, c.Request)
			return nil
		}
	}
}

const swaggerIndexHTML = `<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>Blockchain Gateway API</title>
  <link rel="stylesheet" href="swagger-ui.css">
  <link rel="icon" type="image/png" href="favicon-32x32.png">
</head>
<body>
  <div id="swagger-ui"></div>
  <script src="swagger-ui-bundle.js"></script>
  <script src="swagger-ui-standalone-preset.js"></script>
  <script>
    window.ui = SwaggerUIBundle({
      url: "doc.json",
      dom_id: "#swagger-ui",
      deepLinking: true,
      presets: [SwaggerUIBundle.presets.apis, SwaggerUIStandalonePreset],
      layout: "StandaloneLayout"
    });
  </script>
</body>
</html>`
