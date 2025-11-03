package health

import (
	"context"
	"log"
	"sort"
	"sync"
	"time"
)

// HealthChecker aggregates multiple health checkers
type HealthChecker struct {
	checkers  []Checker
	startTime time.Time
	mutex     sync.RWMutex
	logger    *log.Logger
}

// NewHealthChecker creates a new health checker aggregator
func NewHealthChecker(logger *log.Logger) *HealthChecker {
	return &HealthChecker{
		checkers:  make([]Checker, 0),
		startTime: time.Now(),
		logger:    logger,
	}
}

// AddChecker adds a health checker to the aggregator
func (hc *HealthChecker) AddChecker(checker Checker) {
	hc.mutex.Lock()
	defer hc.mutex.Unlock()
	hc.checkers = append(hc.checkers, checker)
}

// RemoveChecker removes a health checker from the aggregator
func (hc *HealthChecker) RemoveChecker(name string) {
	hc.mutex.Lock()
	defer hc.mutex.Unlock()

	for i, checker := range hc.checkers {
		if checker.Name() == name {
			hc.checkers = append(hc.checkers[:i], hc.checkers[i+1:]...)
			break
		}
	}
}

// GetCheckers returns a copy of all checkers
func (hc *HealthChecker) GetCheckers() []Checker {
	hc.mutex.RLock()
	defer hc.mutex.RUnlock()

	checkers := make([]Checker, len(hc.checkers))
	copy(checkers, hc.checkers)
	return checkers
}

// RunChecks runs all health checks concurrently and returns the results
func (hc *HealthChecker) RunChecks(ctx context.Context) *HealthReport {
	checkers := hc.GetCheckers()

	// Sort checkers by priority (lower number = higher priority)
	sort.Slice(checkers, func(i, j int) bool {
		return checkers[i].Priority() < checkers[j].Priority()
	})

	// Run checks concurrently
	results := make(chan *Check, len(checkers))
	var wg sync.WaitGroup

	for _, checker := range checkers {
		wg.Add(1)
		go func(c Checker) {
			defer wg.Done()

			checkCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
			defer cancel()

			start := time.Now()
			result := c.Check(checkCtx)
			duration := time.Since(start)

			check := ToCheck(c, result, duration)
			results <- check
		}(checker)
	}

	// Wait for all checks to complete
	wg.Wait()
	close(results)

	// Collect results
	checks := make([]Check, 0, len(checkers))
	for check := range results {
		checks = append(checks, *check)

		// Log critical issues
		if check.Status == StatusCritical {
			hc.logger.Printf("[HEALTH] Critical: %s - %s", check.Name, check.Error)
		}
	}

	// Merge results into report
	uptime := time.Since(hc.startTime)
	report := MergeResults(checks, uptime)

	// Log overall status
	hc.logger.Printf("[HEALTH] Overall status: %s (OK: %d, Warning: %d, Critical: %d, Unknown: %d)",
		report.OverallStatus.GetStatusString(),
		report.Summary.OK,
		report.Summary.Warning,
		report.Summary.Critical,
		report.Summary.Unknown)

	return report
}

// RunChecksSequential runs all health checks sequentially (useful for debugging)
func (hc *HealthChecker) RunChecksSequential(ctx context.Context) *HealthReport {
	checkers := hc.GetCheckers()

	// Sort checkers by priority
	sort.Slice(checkers, func(i, j int) bool {
		return checkers[i].Priority() < checkers[j].Priority()
	})

	checks := make([]Check, 0, len(checkers))

	for _, checker := range checkers {
		start := time.Now()
		result := checker.Check(ctx)
		duration := time.Since(start)

		check := ToCheck(checker, result, duration)
		checks = append(checks, *check)

		// Log result
		hc.logger.Printf("[HEALTH] %s: %s - %s (%.2fms)",
			check.Name,
			check.Status.GetStatusString(),
			check.Message,
			float64(duration.Nanoseconds())/1000000)
	}

	uptime := time.Since(hc.startTime)
	return MergeResults(checks, uptime)
}

// GetChecker returns a specific checker by name
func (hc *HealthChecker) GetChecker(name string) Checker {
	hc.mutex.RLock()
	defer hc.mutex.RUnlock()

	for _, checker := range hc.checkers {
		if checker.Name() == name {
			return checker
		}
	}
	return nil
}

// GetHealthyCheckers returns all checkers that are healthy
func (hc *HealthChecker) GetHealthyCheckers(ctx context.Context) []Checker {
	report := hc.RunChecks(ctx)

	healthy := make([]Checker, 0)
	for _, check := range report.Checks {
		if check.Status == StatusOK || check.Status == StatusWarning {
			checker := hc.GetChecker(check.Name)
			if checker != nil {
				healthy = append(healthy, checker)
			}
		}
	}

	return healthy
}

// GetUnhealthyCheckers returns all checkers that are unhealthy
func (hc *HealthChecker) GetUnhealthyCheckers(ctx context.Context) []Checker {
	report := hc.RunChecks(ctx)

	unhealthy := make([]Checker, 0)
	for _, check := range report.Checks {
		if check.Status == StatusCritical || check.Status == StatusUnknown {
			checker := hc.GetChecker(check.Name)
			if checker != nil {
				unhealthy = append(unhealthy, checker)
			}
		}
	}

	return unhealthy
}

// ClearCheckers removes all checkers
func (hc *HealthChecker) ClearCheckers() {
	hc.mutex.Lock()
	defer hc.mutex.Unlock()
	hc.checkers = make([]Checker, 0)
}

// CountCheckers returns the number of checkers
func (hc *HealthChecker) CountCheckers() int {
	hc.mutex.RLock()
	defer hc.mutex.RUnlock()
	return len(hc.checkers)
}

// GetUptime returns the uptime of the health checker
func (hc *HealthChecker) GetUptime() time.Duration {
	return time.Since(hc.startTime)
}

// ResetUptime resets the start time
func (hc *HealthChecker) ResetUptime() {
	hc.startTime = time.Now()
}
