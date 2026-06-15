package middleware

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"backend/cache"
	"backend/db"
	"backend/utils"

	"github.com/gin-gonic/gin"
)

// AuthMiddleware validates the JWT token in Authorization header
func AuthMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		authHeader := c.GetHeader("Authorization")
		if authHeader == "" {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Authorization header is required"})
			c.Abort()
			return
		}

		parts := strings.Split(authHeader, " ")
		if len(parts) != 2 || parts[0] != "Bearer" {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Authorization header must be in Format 'Bearer <token>'"})
			c.Abort()
			return
		}

		tokenStr := parts[1]
		claims, err := utils.ParseJWT(tokenStr)
		if err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": fmt.Sprintf("Invalid or expired token: %v", err)})
			c.Abort()
			return
		}

		// Extract claims
		userID, ok1 := claims["sub"].(string)
		role, ok2 := claims["role"].(string)
		if !ok1 || !ok2 {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid token claims structure"})
			c.Abort()
			return
		}

		// Save in Gin Context
		c.Set("userID", userID)
		c.Set("role", role)

		c.Next()
	}
}

// RequirePermission restricts access to users whose role possesses a specific permission
func RequirePermission(permission string) gin.HandlerFunc {
	return func(c *gin.Context) {
		userRole, exists := c.Get("role")
		if !exists {
			c.JSON(http.StatusForbidden, gin.H{"error": "User role context missing"})
			c.Abort()
			return
		}

		roleStr := userRole.(string)
		ctx := context.Background()

		// 1. Try checking Redis Cache first
		cacheKey := fmt.Sprintf("role_perm:%s:%s", roleStr, permission)
		cachedVal, err := cache.Get(ctx, cacheKey)
		if err == nil {
			if cachedVal == "true" {
				c.Next()
				return
			}
			c.JSON(http.StatusForbidden, gin.H{"error": fmt.Sprintf("Permission denied: role '%s' does not have '%s' permission", roleStr, permission)})
			c.Abort()
			return
		}

		// 2. Query Postgres database
		var hasPermission bool
		query := `
			SELECT EXISTS (
				SELECT 1 
				FROM role_permissions rp
				JOIN roles r ON rp.role_id = r.id
				JOIN permissions p ON rp.permission_id = p.id
				WHERE r.name = $1 AND p.name = $2
			)
		`
		err = db.Pool.QueryRow(ctx, query, roleStr, permission).Scan(&hasPermission)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to verify permissions"})
			c.Abort()
			return
		}

		// Cache resolution in Redis (expires in 1 hour)
		cacheValStr := "false"
		if hasPermission {
			cacheValStr = "true"
		}
		_ = cache.Set(ctx, cacheKey, cacheValStr, 1*time.Hour)

		if !hasPermission {
			c.JSON(http.StatusForbidden, gin.H{"error": fmt.Sprintf("Permission denied: role '%s' does not have '%s' permission", roleStr, permission)})
			c.Abort()
			return
		}

		c.Next()
	}
}
