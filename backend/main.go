package main

import (
	"fmt"
	"log"
	"net/http"
	"os"

	"backend/cache"
	"backend/db"
	"backend/handlers"
	"backend/middleware"
	"backend/storage"
	"backend/ws"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

func main() {
	// 1. Get configurations from Env with local dev fallbacks
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	dbHost := os.Getenv("DB_HOST")
	if dbHost == "" {
		dbHost = "localhost"
	}
	dbPort := os.Getenv("DB_PORT")
	if dbPort == "" {
		dbPort = "5432"
	}
	dbUser := os.Getenv("DB_USER")
	if dbUser == "" {
		dbUser = "admin"
	}
	dbPass := os.Getenv("DB_PASSWORD")
	if dbPass == "" {
		dbPass = "adminpassword"
	}
	dbName := os.Getenv("DB_NAME")
	if dbName == "" {
		dbName = "cloudstore"
	}

	redisHost := os.Getenv("REDIS_HOST")
	if redisHost == "" {
		redisHost = "localhost"
	}
	redisPort := os.Getenv("REDIS_PORT")
	if redisPort == "" {
		redisPort = "6379"
	}

	// 2. Initialize PostgreSQL
	connStr := fmt.Sprintf("postgres://%s:%s@%s:%s/%s?sslmode=disable", dbUser, dbPass, dbHost, dbPort, dbName)
	log.Printf("Connecting to database at %s:%s...", dbHost, dbPort)
	_, err := db.InitDB(connStr)
	if err != nil {
		log.Printf("ERROR: Database connection failed: %v", err)
		log.Printf("TIP: Ensure PostgreSQL is running. You can start it using: docker compose up -d postgres")
		os.Exit(1)
	}

	// 3. Initialize Redis
	log.Printf("Connecting to Redis at %s:%s...", redisHost, redisPort)
	_, err = cache.InitRedis(redisHost, redisPort)
	if err != nil {
		log.Printf("ERROR: Redis connection failed: %v", err)
		log.Printf("TIP: Ensure Redis is running. You can start it using: docker compose up -d redis")
		os.Exit(1)
	}

	// 3.5. Initialize MinIO Object Storage
	minioEnd := os.Getenv("MINIO_ENDPOINT")
	if minioEnd == "" {
		minioEnd = "localhost:9000"
	}
	minioAccess := os.Getenv("MINIO_ACCESS_KEY")
	if minioAccess == "" {
		minioAccess = "minioadmin"
	}
	minioSecret := os.Getenv("MINIO_SECRET_KEY")
	if minioSecret == "" {
		minioSecret = "minioadminpassword"
	}

	log.Printf("Connecting to MinIO at %s...", minioEnd)
	_, err = storage.InitMinio(minioEnd, minioAccess, minioSecret)
	if err != nil {
		log.Printf("ERROR: MinIO connection failed: %v", err)
		log.Printf("TIP: Ensure MinIO is running. You can start it using: docker compose up -d minio")
		os.Exit(1)
	}

	// 3.8. Initialize WebSocket Hub
	ws.InitHub()

	// 4. Initialize Gin
	r := gin.New()
	r.Use(middleware.StructuredLogger(), middleware.PrometheusMetrics(), gin.Recovery())

	// CORS Middleware
	r.Use(func(c *gin.Context) {
		c.Writer.Header().Set("Access-Control-Allow-Origin", "*")
		c.Writer.Header().Set("Access-Control-Allow-Credentials", "true")
		c.Writer.Header().Set("Access-Control-Allow-Headers", "Content-Type, Content-Length, Accept-Encoding, X-CSRF-Token, Authorization, accept, origin, Cache-Control, X-Requested-With")
		c.Writer.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS, GET, PUT, DELETE, PATCH")

		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}
		c.Next()
	})

	// Basic route
	r.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"status":  "healthy",
			"message": "Distributed Cloud Storage API is up and running",
		})
	})

	// Prometheus Metrics Endpoint
	r.GET("/metrics", gin.WrapH(promhttp.Handler()))

	// Auth Group
	auth := r.Group("/api/auth")
	auth.Use(middleware.RateLimiter(20))
	{
		auth.POST("/google", handlers.GoogleLogin)
		auth.POST("/refresh", handlers.Refresh)
		auth.POST("/logout", handlers.Logout)
	}

	// Protected Group Example
	protected := r.Group("/api")
	protected.Use(middleware.AuthMiddleware())
	protected.Use(middleware.RateLimiter(100))
	{
		protected.GET("/test/protected", middleware.RequirePermission("Read"), func(c *gin.Context) {
			userID, _ := c.Get("userID")
			role, _ := c.Get("role")
			c.JSON(http.StatusOK, gin.H{
				"message": "Access granted to protected route!",
				"user_id": userID,
				"role":    role,
			})
		})

		// Folders Management
		protected.POST("/folders", middleware.RequirePermission("Write"), handlers.CreateFolder)
		protected.GET("/folders", middleware.RequirePermission("Read"), handlers.ListDirectory)
		protected.PATCH("/folders/:id", middleware.RequirePermission("Write"), handlers.RenameOrMoveFolder)
		protected.DELETE("/folders/:id", middleware.RequirePermission("Delete"), handlers.DeleteFolder)

		// Files Management
		protected.POST("/files/upload", middleware.RequirePermission("Write"), handlers.UploadFile)
		protected.GET("/files/:id/download", middleware.RequirePermission("Read"), handlers.DownloadFile)
		protected.PATCH("/files/:id", middleware.RequirePermission("Write"), handlers.RenameOrMoveFile)
		protected.DELETE("/files/:id", middleware.RequirePermission("Delete"), handlers.DeleteFile)

		// Resumable Chunked Uploads
		protected.POST("/files/chunks/init", middleware.RequirePermission("Write"), handlers.InitChunkUpload)
		protected.GET("/files/chunks/status", middleware.RequirePermission("Read"), handlers.GetChunkUploadStatus)
		protected.POST("/files/chunks/upload", middleware.RequirePermission("Write"), handlers.UploadChunk)
		protected.POST("/files/chunks/merge", middleware.RequirePermission("Write"), handlers.MergeChunks)

		// File Versions Management
		protected.GET("/files/:id/versions", middleware.RequirePermission("Read"), handlers.ListFileVersions)
		protected.POST("/files/:id/versions/:number/restore", middleware.RequirePermission("Write"), handlers.RestoreFileVersion)
		protected.GET("/files/:id/versions/:number/download", middleware.RequirePermission("Read"), handlers.DownloadFileVersion)

		// Share Links Creation
		protected.POST("/shares", middleware.RequirePermission("Share"), handlers.CreateShareLink)

		// Search Engine
		protected.GET("/search", middleware.RequirePermission("Read"), handlers.SearchFiles)
	}

	// Public Share Resource Resolver (Unauthenticated)
	r.GET("/api/shares/public/:token", handlers.GetSharedResource)
	r.GET("/api/shares/public/:token/download/:file_id", handlers.DownloadSharedFile)

	// WebSocket Event Stream (Token verified in query parameters)
	r.GET("/api/ws", ws.HandleWS)

	log.Printf("Server starting on port %s...", port)
	if err := r.Run(":" + port); err != nil {
		log.Fatalf("Failed to run server: %v", err)
	}
}
