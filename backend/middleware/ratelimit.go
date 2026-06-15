package middleware

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"backend/cache"

	"github.com/gin-gonic/gin"
)

// RateLimiter returns a middleware that limits requests per client using Redis
func RateLimiter(requestsPerMinute int) gin.HandlerFunc {
	return func(c *gin.Context) {
		// Use User ID if authenticated, fallback to IP address
		identifier := c.ClientIP()
		if val, exists := c.Get("userID"); exists {
			identifier = val.(string)
		}

		ctx := context.Background()
		redisKey := fmt.Sprintf("rate:%s:%d", identifier, time.Now().Minute())

		// Increment request counter in Redis
		count, err := cache.RedisClient.Incr(ctx, redisKey).Result()
		if err != nil {
			// Fail open on cache failures in production, but log warning
			c.Next()
			return
		}

		// Set short TTL if this is the first request in this minute window
		if count == 1 {
			_ = cache.RedisClient.Expire(ctx, redisKey, 59*time.Second)
		}

		// Exceeded limit
		if count > int64(requestsPerMinute) {
			c.JSON(http.StatusTooManyRequests, gin.H{
				"error":       "Too many requests. Please try again in a minute.",
				"retry_after": 60 - time.Now().Second(),
			})
			c.Abort()
			return
		}

		c.Next()
	}
}
