package handlers

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"path/filepath"
	"strconv"
	"time"

	"backend/ai"
	"backend/cache"
	"backend/db"
	"backend/storage"
	"backend/utils"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// Chunk Upload Requests
type InitUploadRequest struct {
	Filename   string  `json:"filename" binding:"required"`
	TotalSize  int64   `json:"total_size" binding:"required"`
	MimeType   string  `json:"mime_type" binding:"required"`
	TotalParts int     `json:"total_parts" binding:"required"`
	FileHash   string  `json:"file_hash" binding:"required"` // SHA256 of full file
	FolderID   *string `json:"folder_id"`
}

type MergeUploadRequest struct {
	UploadID string `json:"upload_id" binding:"required"`
}

type UploadSession struct {
	UploadID   string    `json:"upload_id"`
	UserID     string    `json:"user_id"`
	Filename   string    `json:"filename"`
	TotalSize  int64     `json:"total_size"`
	MimeType   string    `json:"mime_type"`
	TotalParts int       `json:"total_parts"`
	FileHash   string    `json:"file_hash"`
	FolderID   string    `json:"folder_id"`
	CreatedAt  time.Time `json:"created_at"`
}

// InitChunkUpload handles POST /api/files/chunks/init
func InitChunkUpload(c *gin.Context) {
	userID, _ := c.Get("userID")
	var req InitUploadRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Validate metadata (Limit: 5GB for chunked uploads)
	sanitizedName, err := utils.ValidateFileMetadata(req.Filename, req.TotalSize, 5*1024*1024*1024)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	req.Filename = sanitizedName

	ctx := context.Background()

	// Verify folder if provided
	fID := ""
	if req.FolderID != nil && *req.FolderID != "" {
		fID = *req.FolderID
		var folderExists bool
		err := db.Pool.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM folders WHERE id = $1 AND user_id = $2)", fID, userID).Scan(&folderExists)
		if err != nil || !folderExists {
			c.JSON(http.StatusNotFound, gin.H{"error": "Destination folder not found"})
			return
		}
	}

	// Generate custom upload session ID
	uploadID := uuid.New().String()

	session := UploadSession{
		UploadID:   uploadID,
		UserID:     userID.(string),
		Filename:   req.Filename,
		TotalSize:  req.TotalSize,
		MimeType:   req.MimeType,
		TotalParts: req.TotalParts,
		FileHash:   req.FileHash,
		FolderID:   fID,
		CreatedAt:  time.Now(),
	}

	// Cache session details in Redis (expires in 24 hours)
	sessionBytes, err := json.Marshal(session)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to serialize upload session"})
		return
	}

	err = cache.Set(ctx, "upload_session:"+uploadID, string(sessionBytes), 24*time.Hour)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to cache upload session"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"upload_id":   uploadID,
		"total_parts": req.TotalParts,
		"message":     "Chunk upload session initialized successfully",
	})
}

// GetChunkUploadStatus handles GET /api/files/chunks/status
func GetChunkUploadStatus(c *gin.Context) {
	userID, _ := c.Get("userID")
	uploadID := c.Query("upload_id")
	if uploadID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "upload_id parameter is required"})
		return
	}

	ctx := context.Background()

	// Retrieve session from Redis
	sessionStr, err := cache.Get(ctx, "upload_session:"+uploadID)
	if err != nil || sessionStr == "" {
		c.JSON(http.StatusNotFound, gin.H{"error": "Upload session not found or expired"})
		return
	}

	var session UploadSession
	if err := json.Unmarshal([]byte(sessionStr), &session); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to parse upload session"})
		return
	}

	// Verify owner
	if session.UserID != userID.(string) {
		c.JSON(http.StatusForbidden, gin.H{"error": "Access denied"})
		return
	}

	// Query Database for uploaded chunk numbers
	rows, err := db.Pool.Query(ctx, "SELECT chunk_number FROM chunks WHERE upload_id = $1 AND is_uploaded = TRUE", uploadID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to query database for chunks"})
		return
	}
	defer rows.Close()

	uploadedChunks := []int{}
	for rows.Next() {
		var chunkNum int
		if err := rows.Scan(&chunkNum); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to scan database rows"})
			return
		}
		uploadedChunks = append(uploadedChunks, chunkNum)
	}

	c.JSON(http.StatusOK, gin.H{
		"upload_id":       uploadID,
		"uploaded_chunks": uploadedChunks,
		"total_parts":     session.TotalParts,
	})
}

// UploadChunk handles POST /api/files/chunks/upload
func UploadChunk(c *gin.Context) {
	userID, _ := c.Get("userID")

	// Get form parameters
	uploadID := c.PostForm("upload_id")
	chunkNumStr := c.PostForm("chunk_number")
	
	if uploadID == "" || chunkNumStr == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "upload_id and chunk_number are required"})
		return
	}

	chunkNum, err := strconv.Atoi(chunkNumStr)
	if err != nil || chunkNum <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid chunk_number"})
		return
	}

	fileHeader, err := c.FormFile("chunk")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "No chunk file uploaded"})
		return
	}

	// Limit individual chunk upload size (e.g. 10MB)
	if fileHeader.Size > 10*1024*1024 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Chunk size exceeds limit of 10MB"})
		return
	}

	ctx := context.Background()

	// 1. Retrieve session from Redis
	sessionStr, err := cache.Get(ctx, "upload_session:"+uploadID)
	if err != nil || sessionStr == "" {
		c.JSON(http.StatusNotFound, gin.H{"error": "Upload session not found or expired"})
		return
	}

	var session UploadSession
	if err := json.Unmarshal([]byte(sessionStr), &session); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to parse session"})
		return
	}

	// Verify owner
	if session.UserID != userID.(string) {
		c.JSON(http.StatusForbidden, gin.H{"error": "Access denied"})
		return
	}

	chunkFile, err := fileHeader.Open()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to open chunk file"})
		return
	}
	defer chunkFile.Close()

	// Calculate SHA-256 hash of this chunk to track integrity
	hashWriter := sha256.New()
	if _, err := io.Copy(hashWriter, chunkFile); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to hash chunk"})
		return
	}
	chunkHash := hex.EncodeToString(hashWriter.Sum(nil))

	// Reset chunk file pointer back to start
	_, err = chunkFile.Seek(0, io.SeekStart)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to reset chunk file pointer"})
		return
	}

	// 2. Upload chunk directly to MinIO temp folder
	tempObjectKey := fmt.Sprintf("uploads/%s/chunks/%d", uploadID, chunkNum)
	err = storage.PutObject(ctx, tempObjectKey, chunkFile, fileHeader.Size, "application/octet-stream")
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to store chunk"})
		return
	}

	// 3. Mark in DB `chunks` table as uploaded
	_, err = db.Pool.Exec(ctx, `
		INSERT INTO chunks (upload_id, chunk_number, size, hash, is_uploaded) 
		VALUES ($1, $2, $3, $4, TRUE) 
		ON CONFLICT (upload_id, chunk_number) 
		DO UPDATE SET size = EXCLUDED.size, hash = EXCLUDED.hash, is_uploaded = TRUE, created_at = NOW()`,
		uploadID, chunkNum, fileHeader.Size, chunkHash)

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to write chunk status to DB"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message":      fmt.Sprintf("Chunk %d uploaded successfully", chunkNum),
		"chunk_number": chunkNum,
	})
}

// MergeChunks handles POST /api/files/chunks/merge
func MergeChunks(c *gin.Context) {
	userID, _ := c.Get("userID")
	var req MergeUploadRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	ctx := context.Background()

	// 1. Retrieve session from Redis
	sessionStr, err := cache.Get(ctx, "upload_session:"+req.UploadID)
	if err != nil || sessionStr == "" {
		c.JSON(http.StatusNotFound, gin.H{"error": "Upload session not found or expired"})
		return
	}

	var session UploadSession
	if err := json.Unmarshal([]byte(sessionStr), &session); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to parse session"})
		return
	}

	// Verify owner
	if session.UserID != userID.(string) {
		c.JSON(http.StatusForbidden, gin.H{"error": "Access denied"})
		return
	}

	// 2. Validate all chunk parts are uploaded
	var uploadedCount int
	err = db.Pool.QueryRow(ctx, "SELECT COUNT(*) FROM chunks WHERE upload_id = $1 AND is_uploaded = TRUE", req.UploadID).Scan(&uploadedCount)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to count chunks in database"})
		return
	}

	if uploadedCount != session.TotalParts {
		c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("Missing chunks. Uploaded %d out of %d", uploadedCount, session.TotalParts)})
		return
	}

	// 3. Perform Merge operation
	// We download chunks from S3 and stream merge them using MultiReader to write directly back to MinIO!
	// This prevents memory issues and avoids writing a huge temporary file to local disk.
	readers := make([]io.Reader, session.TotalParts)
	closers := make([]io.Closer, session.TotalParts)

	// Open read streams for all chunks in order
	for i := 1; i <= session.TotalParts; i++ {
		chunkKey := fmt.Sprintf("uploads/%s/chunks/%d", req.UploadID, i)
		reader, err := storage.GetObject(ctx, chunkKey)
		if err != nil {
			// Clean up opened readers before aborting
			for j := 0; j < i-1; j++ {
				closers[j].Close()
			}
			c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Failed to stream chunk %d", i)})
			return
		}
		readers[i-1] = reader
		closers[i-1] = reader
	}

	// Combine readers sequentially
	multiReader := io.MultiReader(readers...)

	// We pipe the MultiReader to calculate the combined SHA-256 while writing to S3!
	pr, pw := io.Pipe()
	hashWriter := sha256.New()
	
	// TeeReader writes to hashWriter automatically as data is read from the pipe
	teeReader := io.TeeReader(multiReader, hashWriter)

	// Upload in a separate goroutine so MinIO can read as we pipe
	uploadErrChan := make(chan error, 1)
	
	var finalObjectKey string
	var isDuplicate bool = false

	// Check DB if merged hash already exists (Deduplication)
	var existingKey string
	err = db.Pool.QueryRow(ctx, "SELECT key FROM file_versions WHERE hash = $1 LIMIT 1", session.FileHash).Scan(&existingKey)
	if err == nil && existingKey != "" {
		// DEDUPLICATION HITS!
		finalObjectKey = existingKey
		isDuplicate = true
	} else {
		// DEDUPLICATION MISS!
		objectUUID := uuid.New().String()
		ext := filepath.Ext(session.Filename)
		finalObjectKey = fmt.Sprintf("users/%s/files/%s%s", session.UserID, objectUUID, ext)
	}

	if !isDuplicate {
		go func() {
			err := storage.PutObject(ctx, finalObjectKey, pr, session.TotalSize, session.MimeType)
			uploadErrChan <- err
		}()

		// Copy teeReader to write end of the pipe
		_, err = io.Copy(pw, teeReader)
		pw.Close() // Close write end to release read block

		// Wait for upload to complete
		if uploadErr := <-uploadErrChan; uploadErr != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Failed to store merged file: %v", uploadErr)})
			return
		}
	} else {
		// Deduplicated, just drain and hash verify without uploading
		_, err = io.Copy(io.Discard, teeReader)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to process data stream"})
			return
		}
	}

	// Close all reader streams
	for _, cl := range closers {
		_ = cl.Close()
	}

	// 4. Verify Integrity (check SHA256 of full merged stream matches original file hash)
	mergedHash := hex.EncodeToString(hashWriter.Sum(nil))
	if mergedHash != session.FileHash {
		// Cleanup uploaded key if not duplicate and failed hash
		if !isDuplicate {
			_ = storage.DeleteObject(ctx, finalObjectKey)
		}
		c.JSON(http.StatusBadRequest, gin.H{"error": "Integrity check failed: merged file hash does not match original"})
		return
	}

	// 5. Transaction to commit database metadata
	tx, err := db.Pool.Begin(ctx)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to open database transaction"})
		return
	}
	defer tx.Rollback(ctx)

	var fileID string
	var fID interface{} = nil
	if session.FolderID != "" {
		fID = session.FolderID
	}

	// Check if file metadata exists already (overwriting / new version)
	var existingFileID string
	err = tx.QueryRow(ctx, "SELECT id FROM files WHERE name = $1 AND user_id = $2 AND folder_id IS NOT DISTINCT FROM $3 AND is_deleted = FALSE",
		session.Filename, session.UserID, fID).Scan(&existingFileID)

	if err == nil && existingFileID != "" {
		// New Version upload!
		fileID = existingFileID
		var currentVer int
		err = tx.QueryRow(ctx, "SELECT current_version FROM files WHERE id = $1 FOR UPDATE", fileID).Scan(&currentVer)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to lock file record"})
			return
		}

		newVer := currentVer + 1
		// Update files table size and version
		_, err = tx.Exec(ctx, "UPDATE files SET size = $1, current_version = $2, hash = $3, updated_at = NOW() WHERE id = $4",
			session.TotalSize, newVer, mergedHash, fileID)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update file metadata"})
			return
		}

		// Insert version history record
		_, err = tx.Exec(ctx, `
			INSERT INTO file_versions (file_id, version_number, size, key, hash, created_by) 
			VALUES ($1, $2, $3, $4, $5, $6)`,
			fileID, newVer, session.TotalSize, finalObjectKey, mergedHash, session.UserID)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to log new file version"})
			return
		}
	} else {
		// Fully new file upload!
		err = tx.QueryRow(ctx, `
			INSERT INTO files (name, folder_id, user_id, size, mime_type, current_version, hash) 
			VALUES ($1, $2, $3, $4, $5, 1, $6) 
			RETURNING id`, session.Filename, fID, session.UserID, session.TotalSize, session.MimeType, mergedHash).Scan(&fileID)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to insert file record"})
			return
		}

		_, err = tx.Exec(ctx, `
			INSERT INTO file_versions (file_id, version_number, size, key, hash, created_by) 
			VALUES ($1, 1, $2, $3, $4, $5)`,
			fileID, session.TotalSize, finalObjectKey, mergedHash, session.UserID)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to insert version history"})
			return
		}
	}

	// Commit Transaction
	if err := tx.Commit(ctx); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to commit database metadata"})
		return
	}

	// Invalidate directory cache
	cache.InvalidateDirCache(ctx, session.UserID, session.FolderID)

	// Trigger background AI processing
	ai.ProcessFileAIBackground(fileID, session.Filename, session.MimeType, finalObjectKey)

	// 6. Clean up temporary chunks asynchronously to respond fast to the user
	go func(upID string, total int) {
		bgCtx := context.Background()
		for i := 1; i <= total; i++ {
			chunkKey := fmt.Sprintf("uploads/%s/chunks/%d", upID, i)
			_ = storage.DeleteObject(bgCtx, chunkKey)
		}
		// Clear session in Redis
		_ = cache.Delete(bgCtx, "upload_session:"+upID)
		// Delete chunks rows in DB
		_, _ = db.Pool.Exec(bgCtx, "DELETE FROM chunks WHERE upload_id = $1", upID)
		log.Printf("Cleaned up temporary resources for chunk upload: %s", upID)
	}(req.UploadID, session.TotalParts)

	c.JSON(http.StatusOK, gin.H{
		"message":      "File chunks merged and verified successfully",
		"file_id":      fileID,
		"name":         session.Filename,
		"size":         session.TotalSize,
		"deduplicated": isDuplicate,
	})
}
