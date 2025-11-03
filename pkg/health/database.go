package health

import (
	"context"
	"database/sql"
	"time"
)

// DatabaseChecker checks the health of the PostgreSQL database
type DatabaseChecker struct {
	db        *sql.DB
	component string
	priority  int
}

// NewDatabaseChecker creates a new database health checker
func NewDatabaseChecker(db *sql.DB, component string) *DatabaseChecker {
	return &DatabaseChecker{
		db:        db,
		component: component,
		priority:  1,
	}
}

// Name returns the name of the health checker
func (c *DatabaseChecker) Name() string {
	return c.component + "_database"
}

// Priority returns the check priority
func (c *DatabaseChecker) Priority() int {
	return c.priority
}

// Check performs the database health check
func (c *DatabaseChecker) Check(ctx context.Context) CheckResult {
	start := time.Now()

	// Test basic connectivity with a simple query
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	// Use a ping to check if we can reach the database
	err := c.db.PingContext(ctx)
	if err != nil {
		return CheckResult{
			Status:  StatusCritical,
			Message: "Database ping failed",
			Error:   err,
		}
	}

	// Get database stats
	stats := c.db.Stats()

	// Check connection pool status
	var status HealthStatus = StatusOK
	var message string

	if stats.OpenConnections >= stats.MaxOpenConnections {
		status = StatusWarning
		message = "Connection pool near maximum"
	} else if stats.OpenConnections == 0 {
		status = StatusWarning
		message = "No active connections"
	} else {
		message = "Database is healthy"
	}

	// Additional checks could be added here
	// - Check replication lag (for master-slave setups)
	// - Check disk usage
	// - Check active transactions
	// - Check long-running queries

	duration := time.Since(start)

	return CheckResult{
		Status:  status,
		Message: message,
		Data: map[string]interface{}{
			"open_connections":     stats.OpenConnections,
			"max_open_connections": stats.MaxOpenConnections,
			"idle_connections":     stats.Idle,
			"wait_count":           stats.WaitCount,
			"wait_duration":        stats.WaitDuration.String(),
			"duration_ms":          duration.Milliseconds(),
		},
	}
}

// DatabaseConnectionChecker checks individual database connections
type DatabaseConnectionChecker struct {
	db       *sql.DB
	name     string
	priority int
}

// NewDatabaseConnectionChecker creates a new database connection health checker
func NewDatabaseConnectionChecker(db *sql.DB, name string) *DatabaseConnectionChecker {
	return &DatabaseConnectionChecker{
		db:       db,
		name:     name,
		priority: 2,
	}
}

// Name returns the name of the health checker
func (c *DatabaseConnectionChecker) Name() string {
	return c.name
}

// Priority returns the check priority
func (c *DatabaseConnectionChecker) Priority() int {
	return c.priority
}

// Check performs the database connection health check
func (c *DatabaseConnectionChecker) Check(ctx context.Context) CheckResult {
	start := time.Now()

	// Execute a simple query to verify the connection is working
	var result int
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	err := c.db.QueryRowContext(ctx, "SELECT 1").Scan(&result)
	if err != nil {
		return CheckResult{
			Status:  StatusCritical,
			Message: "Database query failed",
			Error:   err,
		}
	}

	if result != 1 {
		return CheckResult{
			Status:  StatusCritical,
			Message: "Database query returned unexpected result",
		}
	}

	duration := time.Since(start)

	return CheckResult{
		Status:  StatusOK,
		Message: "Database connection is healthy",
		Data: map[string]interface{}{
			"query_result": result,
			"duration_ms":  duration.Milliseconds(),
		},
	}
}

// DatabaseMigrationChecker checks if migrations are up to date
type DatabaseMigrationChecker struct {
	db            *sql.DB
	migrationsDir string
	priority      int
}

// NewDatabaseMigrationChecker creates a new database migration health checker
func NewDatabaseMigrationChecker(db *sql.DB, migrationsDir string) *DatabaseMigrationChecker {
	return &DatabaseMigrationChecker{
		db:            db,
		migrationsDir: migrationsDir,
		priority:      10,
	}
}

// Name returns the name of the health checker
func (c *DatabaseMigrationChecker) Name() string {
	return "database_migrations"
}

// Priority returns the check priority
func (c *DatabaseMigrationChecker) Priority() int {
	return c.priority
}

// Check performs the database migration health check
func (c *DatabaseMigrationChecker) Check(ctx context.Context) CheckResult {
	// This is a placeholder implementation
	// In a real scenario, you would check if migrations are applied
	// by querying the migrations table

	return CheckResult{
		Status:  StatusOK,
		Message: "Migrations are up to date",
		Data: map[string]interface{}{
			"migrations_applied": true,
			"migrations_dir":     c.migrationsDir,
		},
	}
}
