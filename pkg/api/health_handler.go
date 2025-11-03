package api

import (
	"log"
	"net/http"
	"time"

	"github.com/dvictor357/blockchain-gateway/pkg/health"
	"github.com/gin-gonic/gin"
)

// HealthHandler handles health check API endpoints
type HealthHandler struct {
	healthChecker *health.HealthChecker
	logger        *log.Logger
}

// NewHealthHandler creates a new health handler
func NewHealthHandler(healthChecker *health.HealthChecker, logger *log.Logger) *HealthHandler {
	return &HealthHandler{
		healthChecker: healthChecker,
		logger:        logger,
	}
}

// HealthCheck godoc
// @Summary      Health Check
// @Description  Get basic health status of the service
// @Tags         health
// @Produce      json
// @Success      200  {object}  HealthResponse
// @Router       /health [get]
func (h *HealthHandler) HealthCheck(c *gin.Context) {
	// Run basic health checks
	ctx := c.Request.Context()
	report := h.healthChecker.RunChecks(ctx)

	// Return overall status
	status := http.StatusOK
	if report.OverallStatus == health.StatusCritical {
		status = http.StatusServiceUnavailable
	} else if report.OverallStatus == health.StatusWarning {
		status = http.StatusOK // Warnings don't change HTTP status
	}

	c.JSON(status, HealthResponse{
		Status:    string(report.OverallStatus),
		Message:   report.OverallStatus.GetStatusString(),
		Timestamp: report.Timestamp.Format(time.RFC3339),
		Uptime:    report.Uptime.String(),
		Summary:   report.Summary,
	})
}

// DeepHealthCheck godoc
// @Summary      Deep Health Check
// @Description  Get detailed health status of all components
// @Tags         health
// @Produce      json
// @Success      200  {object}  health.HealthReport
// @Router       /health/detailed [get]
func (h *HealthHandler) DeepHealthCheck(c *gin.Context) {
	ctx := c.Request.Context()
	report := h.healthChecker.RunChecks(ctx)

	// Return detailed report
	status := http.StatusOK
	if report.OverallStatus == health.StatusCritical {
		status = http.StatusServiceUnavailable
	}

	c.JSON(status, report)
}

// HealthCheckComponent godoc
// @Summary      Check Specific Component
// @Description  Get health status of a specific component
// @Tags         health
// @Produce      json
// @Param        component  path      string  true  "Component name"
// @Success      200  {object}  health.Check
// @Failure      404  {object}  api.SwaggerErrorResponse
// @Router       /health/{component} [get]
func (h *HealthHandler) HealthCheckComponent(c *gin.Context) {
	component := c.Param("component")

	ctx := c.Request.Context()
	checker := h.healthChecker.GetChecker(component)

	if checker == nil {
		c.JSON(http.StatusNotFound, ErrorResponse{
			Code:  "COMPONENT_NOT_FOUND",
			Error: "Health checker not found for component: " + component,
		})
		return
	}

	// Run the specific check
	result := checker.Check(ctx)
	check := health.ToCheck(checker, result, 0)

	status := http.StatusOK
	if check.Status == health.StatusCritical {
		status = http.StatusServiceUnavailable
	} else if check.Status == health.StatusWarning {
		status = http.StatusOK
	}

	c.JSON(status, check)
}

// ListHealthChecks godoc
// @Summary      List Health Checks
// @Description  List all registered health checks
// @Tags         health
// @Produce      json
// @Success      200  {object}  HealthChecksListResponse
// @Router       /health/checks [get]
func (h *HealthHandler) ListHealthChecks(c *gin.Context) {
	checkers := h.healthChecker.GetCheckers()

	checks := make([]HealthCheckInfo, 0, len(checkers))
	for _, checker := range checkers {
		checks = append(checks, HealthCheckInfo{
			Name:     checker.Name(),
			Priority: checker.Priority(),
		})
	}

	c.JSON(http.StatusOK, HealthChecksListResponse{
		Checks: checks,
		Total:  len(checks),
	})
}

// ReadyCheck godoc
// @Summary      Readiness Check
// @Description  Check if the service is ready to receive traffic
// @Tags         health
// @Produce      json
// @Success      200  {object}  ReadyResponse
// @Failure      503  {object}  ReadyResponse
// @Router       /ready [get]
func (h *HealthHandler) ReadyCheck(c *gin.Context) {
	ctx := c.Request.Context()

	// For readiness, we need critical components to be healthy
	report := h.healthChecker.RunChecks(ctx)

	ready := report.OverallStatus != health.StatusCritical

	if ready {
		c.JSON(http.StatusOK, ReadyResponse{
			Ready:     true,
			Message:   "Service is ready",
			Timestamp: time.Now().Format(time.RFC3339),
		})
	} else {
		c.JSON(http.StatusServiceUnavailable, ReadyResponse{
			Ready:     false,
			Message:   "Service is not ready",
			Timestamp: time.Now().Format(time.RFC3339),
			Checks:    report.Checks,
		})
	}
}

// LiveCheck godoc
// @Summary      Liveness Check
// @Description  Check if the service is alive
// @Tags         health
// @Produce      json
// @Success      200  {object}  LiveResponse
// @Router       /live [get]
func (h *HealthHandler) LiveCheck(c *gin.Context) {
	// Liveness is just a simple ping - the service is alive if it responds
	c.JSON(http.StatusOK, LiveResponse{
		Alive:     true,
		Message:   "Service is alive",
		Timestamp: time.Now().Format(time.RFC3339),
	})
}

// HealthResponse represents a basic health check response
type HealthResponse struct {
	Status    string         `json:"status"`
	Message   string         `json:"message"`
	Timestamp string         `json:"timestamp"`
	Uptime    string         `json:"uptime"`
	Summary   health.Summary `json:"summary"`
}

// HealthCheckInfo represents information about a health check
type HealthCheckInfo struct {
	Name     string `json:"name"`
	Priority int    `json:"priority"`
}

// HealthChecksListResponse represents a list of health checks
type HealthChecksListResponse struct {
	Checks []HealthCheckInfo `json:"checks"`
	Total  int               `json:"total"`
}

// ReadyResponse represents a readiness check response
type ReadyResponse struct {
	Ready     bool           `json:"ready"`
	Message   string         `json:"message"`
	Timestamp string         `json:"timestamp"`
	Checks    []health.Check `json:"checks,omitempty"`
}

// LiveResponse represents a liveness check response
type LiveResponse struct {
	Alive     bool   `json:"alive"`
	Message   string `json:"message"`
	Timestamp string `json:"timestamp"`
}
