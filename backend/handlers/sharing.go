package handlers

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"time"

	"backend/db"
	"backend/storage"
	"backend/utils"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
)

type CreateShareRequest struct {
	FileID         *string `json:"file_id"`
	FolderID       *string `json:"folder_id"`
	ExpiresInHours *int    `json:"expires_in_hours"`
	Password       *string `json:"password"`
	MaxDownloads   *int    `json:"max_downloads"`
}

type AccessShareRequest struct {
	Password string `json:"password"`
}

// CreateShareLink handles POST /api/shares
func CreateShareLink(c *gin.Context) {
	userID, _ := c.Get("userID")
	var req CreateShareRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Validate target
	if (req.FileID == nil && req.FolderID == nil) || (req.FileID != nil && req.FolderID != nil) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Provide either file_id or folder_id, not both"})
		return
	}

	ctx := context.Background()

	// Verify ownership
	if req.FileID != nil {
		var exists bool
		err := db.Pool.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM files WHERE id = $1 AND user_id = $2 AND is_deleted = FALSE)", *req.FileID, userID).Scan(&exists)
		if err != nil || !exists {
			c.JSON(http.StatusNotFound, gin.H{"error": "File not found or access denied"})
			return
		}
	} else if req.FolderID != nil {
		var exists bool
		err := db.Pool.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM folders WHERE id = $1 AND user_id = $2)", *req.FolderID, userID).Scan(&exists)
		if err != nil || !exists {
			c.JSON(http.StatusNotFound, gin.H{"error": "Folder not found or access denied"})
			return
		}
	}

	// Generate secure token
	token, err := utils.GenerateRandomToken(24)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to generate share link token"})
		return
	}

	// Hash password if set
	var passHash *string = nil
	if req.Password != nil && *req.Password != "" {
		hash, err := utils.HashPassword(*req.Password)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to encrypt password"})
			return
		}
		passHash = &hash
	}

	// Compute expiration
	var expiresAt *time.Time = nil
	if req.ExpiresInHours != nil && *req.ExpiresInHours > 0 {
		t := time.Now().Add(time.Duration(*req.ExpiresInHours) * time.Hour)
		expiresAt = &t
	}

	// Insert into DB
	var shareID string
	err = db.Pool.QueryRow(ctx, `
		INSERT INTO shares (file_id, folder_id, shared_by, token, password_hash, expires_at, max_downloads) 
		VALUES ($1, $2, $3, $4, $5, $6, $7) 
		RETURNING id`,
		req.FileID, req.FolderID, userID, token, passHash, expiresAt, req.MaxDownloads).Scan(&shareID)

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to register share link record"})
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"share_token": token,
		"share_url":   fmt.Sprintf("http://localhost:3000/share/%s", token),
		"expires_at":  expiresAt,
	})
}

// GetSharedResource handles GET /api/shares/public/:token (Public Access)
func GetSharedResource(c *gin.Context) {
	token := c.Param("token")
	passwordQuery := c.Query("password")

	ctx := context.Background()

	var fileID sql.NullString
	var folderID sql.NullString
	var passwordHash sql.NullString
	var expiresAt sql.NullTime
	var maxDownloads sql.NullInt32
	var downloadCount int

	// Retrieve Share metadata
	err := db.Pool.QueryRow(ctx, `
		SELECT file_id, folder_id, password_hash, expires_at, max_downloads, download_count 
		FROM shares 
		WHERE token = $1`, token).Scan(&fileID, &folderID, &passwordHash, &expiresAt, &maxDownloads, &downloadCount)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			c.JSON(http.StatusNotFound, gin.H{"error": "Share link invalid or has been revoked"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Database lookup error"})
		return
	}

	// 1. Verify Expiration
	if expiresAt.Valid && time.Now().After(expiresAt.Time) {
		c.JSON(http.StatusGone, gin.H{"error": "This share link has expired"})
		return
	}

	// 2. Verify Download Limits
	if maxDownloads.Valid && int32(downloadCount) >= maxDownloads.Int32 {
		c.JSON(http.StatusGone, gin.H{"error": "This share link has reached its download limit"})
		return
	}

	// 3. Verify Password Protection
	if passwordHash.Valid && passwordHash.String != "" {
		if passwordQuery == "" {
			c.JSON(http.StatusForbidden, gin.H{"password_required": true, "error": "This share link is password protected"})
			return
		}
		if !utils.CheckPasswordHash(passwordQuery, passwordHash.String) {
			c.JSON(http.StatusUnauthorized, gin.H{"password_required": true, "error": "Incorrect password"})
			return
		}
	}

	// Increment access count
	_, _ = db.Pool.Exec(ctx, "UPDATE shares SET download_count = download_count + 1 WHERE token = $1", token)

	// 4. Resolve shared file or folder
	if fileID.Valid {
		var filename string
		var size int64
		var objectKey string
		err = db.Pool.QueryRow(ctx, `
			SELECT f.name, f.size, fv.key 
			FROM files f 
			JOIN file_versions fv ON f.id = fv.file_id AND f.current_version = fv.version_number 
			WHERE f.id = $1`, fileID.String).Scan(&filename, &size, &objectKey)

		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "Shared file resource not found"})
			return
		}

		presignedURL, err := storage.GetPresignedDownloadURL(ctx, objectKey, filename, 15*time.Minute)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to generate download URL"})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"type":         "file",
			"name":         filename,
			"size":         size,
			"download_url": presignedURL,
		})
	} else if folderID.Valid {
		var folderName string
		err = db.Pool.QueryRow(ctx, "SELECT name FROM folders WHERE id = $1", folderID.String).Scan(&folderName)
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "Shared folder resource not found"})
			return
		}

		// Fetch folder items (Read-only list for shared guests)
		filesRows, err := db.Pool.Query(ctx, `
			SELECT id, name, size, mime_type, created_at 
			FROM files 
			WHERE folder_id = $1 AND is_deleted = FALSE 
			ORDER BY name ASC`, folderID.String)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to retrieve folder contents"})
			return
		}
		defer filesRows.Close()

		type SharedFileItem struct {
			ID        string    `json:"id"`
			Name      string    `json:"name"`
			Size      int64     `json:"size"`
			MimeType  string    `json:"mime_type"`
			CreatedAt time.Time `json:"created_at"`
		}

		var files []SharedFileItem
		for filesRows.Next() {
			var item SharedFileItem
			if err := filesRows.Scan(&item.ID, &item.Name, &item.Size, &item.MimeType, &item.CreatedAt); err == nil {
				files = append(files, item)
			}
		}

		c.JSON(http.StatusOK, gin.H{
			"type":  "folder",
			"name":  folderName,
			"files": files,
		})
	}
}

// DownloadSharedFile handles GET /api/shares/public/:token/download/:file_id
func DownloadSharedFile(c *gin.Context) {
	token := c.Param("token")
	fileID := c.Param("file_id")
	passwordQuery := c.Query("password")

	ctx := context.Background()

	var shareFileID sql.NullString
	var shareFolderID sql.NullString
	var passwordHash sql.NullString
	var expiresAt sql.NullTime
	var maxDownloads sql.NullInt32
	var downloadCount int

	// 1. Retrieve Share metadata
	err := db.Pool.QueryRow(ctx, `
		SELECT file_id, folder_id, password_hash, expires_at, max_downloads, download_count 
		FROM shares 
		WHERE token = $1`, token).Scan(&shareFileID, &shareFolderID, &passwordHash, &expiresAt, &maxDownloads, &downloadCount)

	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Share link invalid"})
		return
	}

	// Verify Expiration
	if expiresAt.Valid && time.Now().After(expiresAt.Time) {
		c.JSON(http.StatusGone, gin.H{"error": "Share link expired"})
		return
	}

	// Verify Download Limits
	if maxDownloads.Valid && int32(downloadCount) >= maxDownloads.Int32 {
		c.JSON(http.StatusGone, gin.H{"error": "Share limit reached"})
		return
	}

	// Verify Password Protection
	if passwordHash.Valid && passwordHash.String != "" {
		if passwordQuery == "" || !utils.CheckPasswordHash(passwordQuery, passwordHash.String) {
			c.JSON(http.StatusForbidden, gin.H{"error": "Access denied. Password required."})
			return
		}
	}

	// 2. Verify File is actually part of the Shared Folder OR is the Shared File itself
	authorized := false
	if shareFileID.Valid && shareFileID.String == fileID {
		authorized = true
	} else if shareFolderID.Valid {
		var inFolder bool
		err = db.Pool.QueryRow(ctx, `
			SELECT EXISTS(
				SELECT 1 FROM files 
				WHERE id = $1 AND folder_id = $2 AND is_deleted = FALSE
			)`, fileID, shareFolderID.String).Scan(&inFolder)
		if err == nil && inFolder {
			authorized = true
		}
	}

	if !authorized {
		c.JSON(http.StatusForbidden, gin.H{"error": "Access denied to file resource"})
		return
	}

	// Increment downloads counter
	_, _ = db.Pool.Exec(ctx, "UPDATE shares SET download_count = download_count + 1 WHERE token = $1", token)

	// Get file download S3 details
	var filename string
	var objectKey string
	err = db.Pool.QueryRow(ctx, `
		SELECT f.name, fv.key 
		FROM files f 
		JOIN file_versions fv ON f.id = fv.file_id AND f.current_version = fv.version_number 
		WHERE f.id = $1`, fileID).Scan(&filename, &objectKey)

	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "File resource not found"})
		return
	}

	// Generate download link
	presignedURL, err := storage.GetPresignedDownloadURL(ctx, objectKey, filename, 15*time.Minute)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to generate download url"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"download_url": presignedURL,
	})
}
