package redis

import (
	"context"
	"time"

	"github.com/dvictor357/blockchain-gateway/pkg/config"
	"github.com/redis/go-redis/v9"
)

// NewClient creates a new Redis client based on configuration
func NewClient(redisConfig config.RedisConfig) *redis.Client {
	addr := redisConfig.Host + ":" + redisConfig.Port

	client := redis.NewClient(&redis.Options{
		Addr:         addr,
		Password:     redisConfig.Password,
		DB:           redisConfig.DB,
		DialTimeout:  10 * time.Second,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
		PoolSize:     10,
		MinIdleConns: 5,
		MaxRetries:   3,
		PoolTimeout:  60 * time.Second,
	})

	return client
}

// Ping checks if Redis is accessible
func Ping(ctx context.Context, client *redis.Client) error {
	_, err := client.Ping(ctx).Result()
	return err
}

// Close gracefully closes the Redis client
func Close(client *redis.Client) error {
	return client.Close()
}
