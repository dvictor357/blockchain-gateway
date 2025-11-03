package health

import (
	"context"
	"time"
)

// HealthStatus represents the status of a component
type HealthStatus string

const (
	StatusOK       HealthStatus = "ok"
	StatusWarning  HealthStatus = "warning"
	StatusCritical HealthStatus = "critical"
	StatusUnknown  HealthStatus = "unknown"
)

// Check represents a single health check
type Check struct {
	Name      string                 `json:"name"`
	Status    HealthStatus           `json:"status"`
	Duration  time.Duration          `json:"duration_ms,omitempty"`
	Message   string                 `json:"message,omitempty"`
	Error     string                 `json:"error,omitempty"`
	Timestamp time.Time              `json:"timestamp"`
	Data      map[string]interface{} `json:"data,omitempty"`
}

// HealthReport represents a complete health report
type HealthReport struct {
	OverallStatus HealthStatus  `json:"overall_status"`
	Timestamp     time.Time     `json:"timestamp"`
	Uptime        time.Duration `json:"uptime,omitempty"`
	Checks        []Check       `json:"checks"`
	Summary       Summary       `json:"summary"`
}

// Summary provides a summary of health checks
type Summary struct {
	Total    int `json:"total"`
	OK       int `json:"ok"`
	Warning  int `json:"warning"`
	Critical int `json:"critical"`
	Unknown  int `json:"unknown"`
}

// CheckResult represents the result of a health check
type CheckResult struct {
	Status  HealthStatus
	Message string
	Data    map[string]interface{}
	Error   error
}

// Checker interface for all health checkers
type Checker interface {
	// Name returns the name of the health checker
	Name() string

	// Check performs the health check
	Check(ctx context.Context) CheckResult

	// Priority returns the check priority (lower number = higher priority)
	Priority() int
}

// NewCheck creates a new Check with default values
func NewCheck(name string) *Check {
	return &Check{
		Name:      name,
		Status:    StatusUnknown,
		Timestamp: time.Now(),
		Data:      make(map[string]interface{}),
	}
}

// ToCheck converts a CheckResult to a Check
func ToCheck(checker Checker, result CheckResult, duration time.Duration) *Check {
	check := NewCheck(checker.Name())
	check.Status = result.Status
	check.Duration = duration
	check.Message = result.Message
	check.Timestamp = time.Now()

	if result.Error != nil {
		check.Error = result.Error.Error()
	}

	if result.Data != nil {
		check.Data = result.Data
	}

	return check
}

// MergeResults merges multiple CheckResults into a single HealthReport
func MergeResults(checks []Check, uptime time.Duration) *HealthReport {
	// Determine overall status
	overallStatus := StatusOK
	summary := Summary{
		Total: len(checks),
	}

	for _, check := range checks {
		// Update summary counts
		switch check.Status {
		case StatusOK:
			summary.OK++
		case StatusWarning:
			summary.Warning++
			if overallStatus == StatusOK {
				overallStatus = StatusWarning
			}
		case StatusCritical:
			summary.Critical++
			overallStatus = StatusCritical
		case StatusUnknown:
			summary.Unknown++
			if overallStatus == StatusOK {
				overallStatus = StatusUnknown
			}
		}
	}

	return &HealthReport{
		OverallStatus: overallStatus,
		Timestamp:     time.Now(),
		Uptime:        uptime,
		Checks:        checks,
		Summary:       summary,
	}
}

// GetOverallStatus determines the overall health status from multiple checks
func GetOverallStatus(checks []Check) HealthStatus {
	hasCritical := false
	hasWarning := false

	for _, check := range checks {
		switch check.Status {
		case StatusCritical:
			hasCritical = true
		case StatusWarning:
			hasWarning = true
		case StatusUnknown:
			if !hasCritical && !hasWarning {
				return StatusUnknown
			}
		}
	}

	if hasCritical {
		return StatusCritical
	}
	if hasWarning {
		return StatusWarning
	}
	return StatusOK
}

// IsHealthy checks if the overall status is OK or Warning
func (hr *HealthReport) IsHealthy() bool {
	return hr.OverallStatus == StatusOK || hr.OverallStatus == StatusWarning
}

// IsCritical checks if the overall status is Critical
func (hr *HealthReport) IsCritical() bool {
	return hr.OverallStatus == StatusCritical
}

// GetStatusString returns a human-readable status string
func (s HealthStatus) GetStatusString() string {
	switch s {
	case StatusOK:
		return "Healthy"
	case StatusWarning:
		return "Warning"
	case StatusCritical:
		return "Critical"
	case StatusUnknown:
		return "Unknown"
	default:
		return "Unknown"
	}
}

// Color returns ANSI color code for the status
func (s HealthStatus) Color() string {
	switch s {
	case StatusOK:
		return "\033[32m" // Green
	case StatusWarning:
		return "\033[33m" // Yellow
	case StatusCritical:
		return "\033[31m" // Red
	case StatusUnknown:
		return "\033[36m" // Cyan
	default:
		return "\033[0m" // Default
	}
}

// ResetColor resets ANSI color
func (s HealthStatus) ResetColor() string {
	return "\033[0m"
}
