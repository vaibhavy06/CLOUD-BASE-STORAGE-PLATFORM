package handlers

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	"backend/cache"
	"backend/db"
	"backend/utils"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
)

type RegisterRequest struct {
	Email    string `json:"email" binding:"required,email"`
	Password string `json:"password" binding:"required,min=6"`
}

type LoginRequest struct {
	Email    string `json:"email" binding:"required,email"`
	Password string `json:"password" binding:"required"`
}

type RefreshRequest struct {
	RefreshToken string `json:"refresh_token" binding:"required"`
}

type LogoutRequest struct {
	RefreshToken string `json:"refresh_token" binding:"required"`
}

// Register handles user registration
func Register(c *gin.Context) {
	var req RegisterRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	ctx := context.Background()

	// Check if user already exists
	var exists bool
	err := db.Pool.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM users WHERE email = $1)", req.Email).Scan(&exists)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Database error"})
		return
	}
	if exists {
		c.JSON(http.StatusConflict, gin.H{"error": "User with this email already exists"})
		return
	}

	// Hash password
	passwordHash, err := utils.HashPassword(req.Password)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Password hashing failed"})
		return
	}

	// Transaction to create user and map standard role
	tx, err := db.Pool.Begin(ctx)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Transaction initiation failed"})
		return
	}
	defer tx.Rollback(ctx)

	var userID string
	err = tx.QueryRow(ctx, `
		INSERT INTO users (email, password_hash) 
		VALUES ($1, $2) 
		RETURNING id`, req.Email, passwordHash).Scan(&userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create user"})
		return
	}

	var userRoleID string
	err = tx.QueryRow(ctx, "SELECT id FROM roles WHERE name = 'User'").Scan(&userRoleID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to resolve user role"})
		return
	}

	_, err = tx.Exec(ctx, "INSERT INTO user_roles (user_id, role_id) VALUES ($1, $2)", userID, userRoleID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to map user role"})
		return
	}

	if err := tx.Commit(ctx); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to finalize registration"})
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"message": "Registration successful. You can now log in.",
		"user_id": userID,
	})
}

// Login handles user authentication and session creation
func Login(c *gin.Context) {
	var req LoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	ctx := context.Background()

	var userID string
	var passwordHash string
	var roleName string

	// Get user hash and role name
	err := db.Pool.QueryRow(ctx, `
		SELECT u.id, u.password_hash, r.name 
		FROM users u 
		JOIN user_roles ur ON u.id = ur.user_id 
		JOIN roles r ON ur.role_id = r.id 
		WHERE u.email = $1`, req.Email).Scan(&userID, &passwordHash, &roleName)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid email or password"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Database error"})
		return
	}

	// Verify password
	if !utils.CheckPasswordHash(req.Password, passwordHash) {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid email or password"})
		return
	}

	// Generate Access Token (JWT)
	accessToken, err := utils.GenerateJWT(userID, roleName)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to generate access token"})
		return
	}

	// Generate secure Refresh Token
	refreshToken, err := utils.GenerateRandomToken(32)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to generate refresh token"})
		return
	}

	// Session Expiration (7 Days)
	expiresAt := time.Now().Add(7 * 24 * time.Hour)

	// Save session in Postgres
	_, err = db.Pool.Exec(ctx, `
		INSERT INTO sessions (user_id, refresh_token, ip_address, user_agent, expires_at) 
		VALUES ($1, $2, $3, $4, $5)`,
		userID, refreshToken, c.ClientIP(), c.GetHeader("User-Agent"), expiresAt)

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create session"})
		return
	}

	// Save session mapping (refresh_token -> user_id:role) in Redis for fast caching
	redisVal := userID + ":" + roleName
	err = cache.Set(ctx, "session:"+refreshToken, redisVal, 7*24*time.Hour)
	if err != nil {
		log.Printf("Warning: Failed to cache session in Redis: %v", err)
		// Proceed anyway, we have PostgreSQL persistence as backup
	}

	c.JSON(http.StatusOK, gin.H{
		"access_token":  accessToken,
		"refresh_token": refreshToken,
		"user": gin.H{
			"id":    userID,
			"email": req.Email,
			"role":  roleName,
		},
	})
}

// Refresh issues a new Access Token using a valid Refresh Token
func Refresh(c *gin.Context) {
	var req RefreshRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	ctx := context.Background()

	var userID string
	var roleName string

	// 1. Try Redis cache-aside first
	cachedVal, err := cache.Get(ctx, "session:"+req.RefreshToken)
	if err == nil && cachedVal != "" {
		// Found in Redis cache
		// Format is: "userID:roleName"
		_, err = fmt.Sscanf(cachedVal, "%36s:%s", &userID, &roleName)
		if err == nil {
			goto generateToken
		}
	}

	// 2. Cache miss, look up in PostgreSQL
	err = db.Pool.QueryRow(ctx, `
		SELECT s.user_id, r.name, s.expires_at 
		FROM sessions s 
		JOIN user_roles ur ON s.user_id = ur.user_id 
		JOIN roles r ON ur.role_id = r.id 
		WHERE s.refresh_token = $1`, req.RefreshToken).Scan(&userID, &roleName, &err) // re-use err for scan mapping below

	// Note: We need a structural way to map variables cleanly
	{
		var dbUserID string
		var dbRoleName string
		var expiresAt time.Time
		err = db.Pool.QueryRow(ctx, `
			SELECT s.user_id, r.name, s.expires_at 
			FROM sessions s 
			JOIN user_roles ur ON s.user_id = ur.user_id 
			JOIN roles r ON ur.role_id = r.id 
			WHERE s.refresh_token = $1`, req.RefreshToken).Scan(&dbUserID, &dbRoleName, &expiresAt)

		if err != nil {
			if errors.Is(err, sql.ErrNoRows) || errors.Is(err, pgx.ErrNoRows) {
				c.JSON(http.StatusUnauthorized, gin.H{"error": "Session not found or revoked"})
				return
			}
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Database lookup error"})
			return
		}

		if time.Now().After(expiresAt) {
			// Revoke expired session
			_, _ = db.Pool.Exec(ctx, "DELETE FROM sessions WHERE refresh_token = $1", req.RefreshToken)
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Session has expired"})
			return
		}

		userID = dbUserID
		roleName = dbRoleName

		// Update Redis cache for next time
		_ = cache.Set(ctx, "session:"+req.RefreshToken, userID+":"+roleName, time.Until(expiresAt))
	}

generateToken:
	// Generate new access token
	newAccessToken, err := utils.GenerateJWT(userID, roleName)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to generate access token"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"access_token": newAccessToken,
	})
}

// Logout revokes the session and deletes credentials from DB/Redis
func Logout(c *gin.Context) {
	var req LogoutRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	ctx := context.Background()

	// Delete from Redis
	_ = cache.Delete(ctx, "session:"+req.RefreshToken)

	// Delete from PostgreSQL
	_, err := db.Pool.Exec(ctx, "DELETE FROM sessions WHERE refresh_token = $1", req.RefreshToken)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to revoke database session"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Logged out successfully"})
}

type GoogleLoginRequest struct {
	Credential string `json:"credential" binding:"required"`
}

// GoogleLogin handles user authentication using Google OAuth or Developer Bypass
func GoogleLogin(c *gin.Context) {
	var req GoogleLoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	ctx := context.Background()
	var email string

	// 1. Check for Developer Bypass
	if req.Credential == "dev-bypass-token" {
		email = "developer@cloudstore.local"
	} else {
		// Verify Google ID Token
		clientID := os.Getenv("GOOGLE_CLIENT_ID")
		claims, err := utils.VerifyGoogleIDToken(req.Credential, clientID)
		if err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": fmt.Sprintf("Google ID Token verification failed: %v", err)})
			return
		}

		var ok bool
		email, ok = claims["email"].(string)
		if !ok || email == "" {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Google ID Token does not contain a valid email"})
			return
		}
	}

	var userID string
	var roleName string

	// 2. Check if user already exists
	err := db.Pool.QueryRow(ctx, `
		SELECT u.id, r.name 
		FROM users u 
		JOIN user_roles ur ON u.id = ur.user_id 
		JOIN roles r ON ur.role_id = r.id 
		WHERE u.email = $1`, email).Scan(&userID, &roleName)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) || errors.Is(err, sql.ErrNoRows) {
			// Auto-register the user
			passwordHash := "$2a$10$placeholderhashforoauthaccounts" // database backward compatibility

			tx, err := db.Pool.Begin(ctx)
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": "Transaction initiation failed"})
				return
			}
			defer tx.Rollback(ctx)

			err = tx.QueryRow(ctx, `
				INSERT INTO users (email, password_hash) 
				VALUES ($1, $2) 
				RETURNING id`, email, passwordHash).Scan(&userID)
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to auto-register user"})
				return
			}

			var userRoleID string
			err = tx.QueryRow(ctx, "SELECT id FROM roles WHERE name = 'User'").Scan(&userRoleID)
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to resolve user role"})
				return
			}

			_, err = tx.Exec(ctx, "INSERT INTO user_roles (user_id, role_id) VALUES ($1, $2)", userID, userRoleID)
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to map user role"})
				return
			}

			if err := tx.Commit(ctx); err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to finalize auto-registration"})
				return
			}
			roleName = "User"
		} else {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Database error checking user existence"})
			return
		}
	}

	// 3. Generate Access Token (JWT)
	accessToken, err := utils.GenerateJWT(userID, roleName)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to generate access token"})
		return
	}

	// 4. Generate secure Refresh Token
	refreshToken, err := utils.GenerateRandomToken(32)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to generate refresh token"})
		return
	}

	// Session Expiration (7 Days)
	expiresAt := time.Now().Add(7 * 24 * time.Hour)

	// Save session in Postgres
	_, err = db.Pool.Exec(ctx, `
		INSERT INTO sessions (user_id, refresh_token, ip_address, user_agent, expires_at) 
		VALUES ($1, $2, $3, $4, $5)`,
		userID, refreshToken, c.ClientIP(), c.GetHeader("User-Agent"), expiresAt)

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create session"})
		return
	}

	// Save session mapping (refresh_token -> user_id:role) in Redis for fast caching
	redisVal := userID + ":" + roleName
	err = cache.Set(ctx, "session:"+refreshToken, redisVal, 7*24*time.Hour)
	if err != nil {
		log.Printf("Warning: Failed to cache session in Redis: %v", err)
	}

	c.JSON(http.StatusOK, gin.H{
		"access_token":  accessToken,
		"refresh_token": refreshToken,
		"user": gin.H{
			"id":    userID,
			"email": email,
			"role":  roleName,
		},
	})
}

