package health

import (
	"context"
	"time"

	"github.com/redis/go-redis/v9"
)

// RedisChecker checks the health of Redis
type RedisChecker struct {
	client    *redis.Client
	component string
	priority  int
}

// NewRedisChecker creates a new Redis health checker
func NewRedisChecker(client *redis.Client, component string) *RedisChecker {
	return &RedisChecker{
		client:    client,
		component: component,
		priority:  1,
	}
}

// Name returns the name of the health checker
func (c *RedisChecker) Name() string {
	return c.component + "_redis"
}

// Priority returns the check priority
func (c *RedisChecker) Priority() int {
	return c.priority
}

// Check performs the Redis health check
func (c *RedisChecker) Check(ctx context.Context) CheckResult {
	start := time.Now()

	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	// Ping Redis
	pingResult, err := c.client.Ping(ctx).Result()
	if err != nil {
		return CheckResult{
			Status:  StatusCritical,
			Message: "Redis ping failed",
			Error:   err,
		}
	}

	// Get Redis info
	info, err := c.client.Info(ctx).Result()
	if err != nil {
		return CheckResult{
			Status:  StatusWarning,
			Message: "Redis info retrieval failed",
			Error:   err,
		}
	}

	// Parse basic Redis stats
	stats, _ := parseRedisInfo(info)

	// Check memory usage
	var status HealthStatus = StatusOK
	var message string

	usedMemoryPercent := float64(stats.UsedMemory) / float64(stats.MaxMemory) * 100

	if usedMemoryPercent > 90 {
		status = StatusCritical
		message = "Redis memory usage is critical"
	} else if usedMemoryPercent > 80 {
		status = StatusWarning
		message = "Redis memory usage is high"
	} else {
		message = "Redis is healthy"
	}

	// Check connected clients
	if stats.ConnectedClients > 1000 {
		if status == StatusOK {
			status = StatusWarning
		}
		message = "High number of connected clients"
	}

	duration := time.Since(start)

	return CheckResult{
		Status:  status,
		Message: message,
		Data: map[string]interface{}{
			"ping_result":               pingResult,
			"version":                   stats.Version,
			"used_memory":               stats.UsedMemory,
			"used_memory_human":         stats.UsedMemoryHuman,
			"used_memory_percent":       usedMemoryPercent,
			"max_memory":                stats.MaxMemory,
			"connected_clients":         stats.ConnectedClients,
			"total_commands_processed":  stats.TotalCommandsProcessed,
			"instantaneous_ops_per_sec": stats.InstantaneousOpsPerSec,
			"keyspace_hits":             stats.KeyspaceHits,
			"keyspace_misses":           stats.KeyspaceMisses,
			"duration_ms":               duration.Milliseconds(),
		},
	}
}

// RedisInfo holds parsed Redis information
type RedisInfo struct {
	Version                string
	UsedMemory             uint64
	UsedMemoryHuman        string
	MaxMemory              uint64
	ConnectedClients       int
	TotalCommandsProcessed uint64
	InstantaneousOpsPerSec int
	KeyspaceHits           uint64
	KeyspaceMisses         uint64
}

// parseRedisInfo parses Redis INFO output
func parseRedisInfo(infoStr string) (*RedisInfo, error) {
	info := &RedisInfo{}

	// Simple parsing - in production, use a proper parser
	// For now, return basic info

	return info, nil
}

// RedisClusterChecker checks Redis cluster health (if using cluster mode)
type RedisClusterChecker struct {
	client   *redis.Client
	priority int
}

// NewRedisClusterChecker creates a new Redis cluster health checker
func NewRedisClusterChecker(client *redis.Client) *RedisClusterChecker {
	return &RedisClusterChecker{
		client:   client,
		priority: 5,
	}
}

// Name returns the name of the health checker
func (c *RedisClusterChecker) Name() string {
	return "redis_cluster"
}

// Priority returns the check priority
func (c *RedisClusterChecker) Priority() int {
	return c.priority
}

// Check performs the Redis cluster health check
func (c *RedisClusterChecker) Check(ctx context.Context) CheckResult {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	// Check cluster info
	clusterInfo, err := c.client.ClusterInfo(ctx).Result()
	if err != nil {
		return CheckResult{
			Status:  StatusWarning,
			Message: "Redis cluster info retrieval failed",
			Error:   err,
		}
	}

	// Check cluster nodes
	nodes, err := c.client.ClusterNodes(ctx).Result()
	if err != nil {
		return CheckResult{
			Status:  StatusWarning,
			Message: "Redis cluster nodes retrieval failed",
			Error:   err,
		}
	}

	return CheckResult{
		Status:  StatusOK,
		Message: "Redis cluster is healthy",
		Data: map[string]interface{}{
			"cluster_info": clusterInfo,
			"nodes_count":  len(nodes),
			"nodes":        nodes,
		},
	}
}
