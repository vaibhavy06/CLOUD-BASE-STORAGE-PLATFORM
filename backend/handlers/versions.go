package handlers

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"time"

	"backend/db"
	"backend/storage"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
)

type FileVersionResponse struct {
	ID            string    `json:"id"`
	VersionNumber int       `json:"version_number"`
	Size          int64     `json:"size"`
	Hash          string    `json:"hash"`
	CreatedAt     time.Time `json:"created_at"`
}

// ListFileVersions handles GET /api/files/:id/versions
func ListFileVersions(c *gin.Context) {
	userID, _ := c.Get("userID")
	fileID := c.Param("id")

	ctx := context.Background()

	// Verify file ownership
	var exists bool
	err := db.Pool.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM files WHERE id = $1 AND user_id = $2 AND is_deleted = FALSE)", fileID, userID).Scan(&exists)
	if err != nil || !exists {
		c.JSON(http.StatusNotFound, gin.H{"error": "File not found or access denied"})
		return
	}

	// Fetch all versions
	rows, err := db.Pool.Query(ctx, `
		SELECT id, version_number, size, hash, created_at 
		FROM file_versions 
		WHERE file_id = $1 
		ORDER BY version_number DESC`, fileID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to query file versions"})
		return
	}
	defer rows.Close()

	var versions []FileVersionResponse
	for rows.Next() {
		var v FileVersionResponse
		err := rows.Scan(&v.ID, &v.VersionNumber, &v.Size, &v.Hash, &v.CreatedAt)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to parse database rows"})
			return
		}
		versions = append(versions, v)
	}

	c.JSON(http.StatusOK, versions)
}

// RestoreFileVersion handles POST /api/files/:id/versions/:number/restore
func RestoreFileVersion(c *gin.Context) {
	userID, _ := c.Get("userID")
	fileID := c.Param("id")
	versionNumStr := c.Param("number")

	versionNum, err := strconv.Atoi(versionNumStr)
	if err != nil || versionNum <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid version number"})
		return
	}

	ctx := context.Background()

	// Verify file ownership
	var exists bool
	err = db.Pool.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM files WHERE id = $1 AND user_id = $2 AND is_deleted = FALSE)", fileID, userID).Scan(&exists)
	if err != nil || !exists {
		c.JSON(http.StatusNotFound, gin.H{"error": "File not found or access denied"})
		return
	}

	// Verify the target version exists
	var size int64
	var hash string
	err = db.Pool.QueryRow(ctx, "SELECT size, hash FROM file_versions WHERE file_id = $1 AND version_number = $2", fileID, versionNum).Scan(&size, &hash)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			c.JSON(http.StatusNotFound, gin.H{"error": "Specified version not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Database lookup error"})
		return
	}

	// Transaction to update active version pointer
	tx, err := db.Pool.Begin(ctx)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to initiate transaction"})
		return
	}
	defer tx.Rollback(ctx)

	_, err = tx.Exec(ctx, `
		UPDATE files 
		SET current_version = $1, size = $2, hash = $3, updated_at = NOW() 
		WHERE id = $4`, versionNum, size, hash, fileID)

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to restore version"})
		return
	}

	if err := tx.Commit(ctx); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to finalize restore transaction"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message":        "File version restored successfully",
		"active_version": versionNum,
		"size":           size,
	})
}

// DownloadFileVersion handles GET /api/files/:id/versions/:number/download
func DownloadFileVersion(c *gin.Context) {
	userID, _ := c.Get("userID")
	fileID := c.Param("id")
	versionNumStr := c.Param("number")

	versionNum, err := strconv.Atoi(versionNumStr)
	if err != nil || versionNum <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid version number"})
		return
	}

	ctx := context.Background()

	// Verify file ownership and get name
	var filename string
	err = db.Pool.QueryRow(ctx, "SELECT name FROM files WHERE id = $1 AND user_id = $2 AND is_deleted = FALSE", fileID, userID).Scan(&filename)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			c.JSON(http.StatusNotFound, gin.H{"error": "File not found or access denied"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Database lookup error"})
		return
	}

	// Fetch version storage key
	var objectKey string
	err = db.Pool.QueryRow(ctx, "SELECT key FROM file_versions WHERE file_id = $1 AND version_number = $2", fileID, versionNum).Scan(&objectKey)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			c.JSON(http.StatusNotFound, gin.H{"error": "Version key not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Database lookup error"})
		return
	}

	// Generate secure download URL (expiring in 15 minutes)
	presignedURL, err := storage.GetPresignedDownloadURL(ctx, objectKey, filename, 15*time.Minute)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to generate download URL"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"download_url": presignedURL,
	})
}
