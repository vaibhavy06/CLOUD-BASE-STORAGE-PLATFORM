package cache

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/redis/go-redis/v9"
)

var RedisClient *redis.Client

// InitRedis initializes the Redis connection client
func InitRedis(host, port string) (*redis.Client, error) {
	addr := fmt.Sprintf("%s:%s", host, port)
	client := redis.NewClient(&redis.Options{
		Addr:         addr,
		Password:     "", // Default empty
		DB:           0,  // Default DB 0
		DialTimeout:  5 * time.Second,
		ReadTimeout:  3 * time.Second,
		WriteTimeout: 3 * time.Second,
		PoolSize:     10,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Ping check
	if err := client.Ping(ctx).Err(); err != nil {
		client.Close()
		return nil, fmt.Errorf("redis connection failed ping: %w", err)
	}

	log.Printf("Connected to Redis successfully at %s", addr)
	RedisClient = client
	return RedisClient, nil
}

// Set stores a key-value pair in Redis with a TTL
func Set(ctx context.Context, key string, value interface{}, expiration time.Duration) error {
	if RedisClient == nil {
		return fmt.Errorf("redis client is not initialized")
	}
	return RedisClient.Set(ctx, key, value, expiration).Err()
}

// Get retrieves a string value from Redis
func Get(ctx context.Context, key string) (string, error) {
	if RedisClient == nil {
		return "", fmt.Errorf("redis client is not initialized")
	}
	return RedisClient.Get(ctx, key).Result()
}

// Delete removes a key from Redis
func Delete(ctx context.Context, key string) error {
	if RedisClient == nil {
		return fmt.Errorf("redis client is not initialized")
	}
	return RedisClient.Del(ctx, key).Err()
}

// Exists checks if a key exists in Redis
func Exists(ctx context.Context, key string) (bool, error) {
	if RedisClient == nil {
		return false, fmt.Errorf("redis client is not initialized")
	}
	val, err := RedisClient.Exists(ctx, key).Result()
	if err != nil {
		return false, err
	}
	return val > 0, nil
}

// InvalidateDirCache removes cached directory listings for a user folder
func InvalidateDirCache(ctx context.Context, userID string, folderID string) {
	key := fmt.Sprintf("dir_cache:%s:%s", userID, folderID)
	_ = Delete(ctx, key)
}
