package handlers

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"time"

	"backend/ai"
	"backend/cache"
	"backend/db"
	"backend/storage"
	"backend/utils"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// Folder Requests
type CreateFolderRequest struct {
	Name     string  `json:"name" binding:"required"`
	ParentID *string `json:"parent_id"` // Pointer allows nil/null in JSON
}

type UpdateFolderRequest struct {
	Name     *string `json:"name"`
	ParentID *string `json:"parent_id"`
}

// File Requests
type UpdateFileRequest struct {
	Name     *string `json:"name"`
	FolderID *string `json:"folder_id"`
}

// CreateFolder handles POST /api/folders
func CreateFolder(c *gin.Context) {
	userID, _ := c.Get("userID")
	var req CreateFolderRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	ctx := context.Background()

	// If parent ID is provided, verify it exists and belongs to the user
	if req.ParentID != nil && *req.ParentID != "" {
		var parentExists bool
		err := db.Pool.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM folders WHERE id = $1 AND user_id = $2)", *req.ParentID, userID).Scan(&parentExists)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Database lookup error"})
			return
		}
		if !parentExists {
			c.JSON(http.StatusNotFound, gin.H{"error": "Parent folder not found"})
			return
		}
	}

	// Insert folder
	var folderID string
	var err error
	if req.ParentID == nil || *req.ParentID == "" {
		err = db.Pool.QueryRow(ctx, `
			INSERT INTO folders (name, parent_id, user_id) 
			VALUES ($1, NULL, $2) 
			RETURNING id`, req.Name, userID).Scan(&folderID)
	} else {
		err = db.Pool.QueryRow(ctx, `
			INSERT INTO folders (name, parent_id, user_id) 
			VALUES ($1, $2, $3) 
			RETURNING id`, req.Name, *req.ParentID, userID).Scan(&folderID)
	}

	if err != nil {
		c.JSON(http.StatusConflict, gin.H{"error": "A folder with this name already exists in this directory"})
		return
	}

	// Invalidate parent directory cache
	pID := ""
	if req.ParentID != nil {
		pID = *req.ParentID
	}
	cache.InvalidateDirCache(ctx, userID.(string), pID)

	c.JSON(http.StatusCreated, gin.H{
		"message":   "Folder created successfully",
		"folder_id": folderID,
		"name":      req.Name,
	})
}

// ListDirectory handles GET /api/folders
func ListDirectory(c *gin.Context) {
	userID, _ := c.Get("userID")
	parentID := c.Query("parent_id")

	ctx := context.Background()

	// 1. Try checking Redis Cache first
	cacheKey := fmt.Sprintf("dir_cache:%s:%s", userID, parentID)
	cachedVal, err := cache.Get(ctx, cacheKey)
	if err == nil && cachedVal != "" {
		// Cache Hit! Respond directly with JSON bytes
		c.Data(http.StatusOK, "application/json", []byte(cachedVal))
		return
	}

	var foldersRows pgx.Rows
	var filesRows pgx.Rows

	// 1. Fetch Subfolders
	if parentID == "" {
		foldersRows, err = db.Pool.Query(ctx, `
			SELECT id, name, parent_id, created_at, updated_at 
			FROM folders 
			WHERE user_id = $1 AND parent_id IS NULL 
			ORDER BY name ASC`, userID)
	} else {
		foldersRows, err = db.Pool.Query(ctx, `
			SELECT id, name, parent_id, created_at, updated_at 
			FROM folders 
			WHERE user_id = $1 AND parent_id = $2 
			ORDER BY name ASC`, userID, parentID)
	}

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch folders"})
		return
	}
	defer foldersRows.Close()

	type FolderResponse struct {
		ID        string    `json:"id"`
		Name      string    `json:"name"`
		ParentID  *string   `json:"parent_id"`
		CreatedAt time.Time `json:"created_at"`
		UpdatedAt time.Time `json:"updated_at"`
	}

	var foldersList []FolderResponse
	for foldersRows.Next() {
		var f FolderResponse
		var pID sql.NullString
		err := foldersRows.Scan(&f.ID, &f.Name, &pID, &f.CreatedAt, &f.UpdatedAt)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to scan folder rows"})
			return
		}
		if pID.Valid {
			s := pID.String
			f.ParentID = &s
		}
		foldersList = append(foldersList, f)
	}

	// 2. Fetch Files (Not deleted)
	if parentID == "" {
		filesRows, err = db.Pool.Query(ctx, `
			SELECT id, name, folder_id, size, mime_type, current_version, created_at, updated_at 
			FROM files 
			WHERE user_id = $1 AND folder_id IS NULL AND is_deleted = FALSE 
			ORDER BY name ASC`, userID)
	} else {
		filesRows, err = db.Pool.Query(ctx, `
			SELECT id, name, folder_id, size, mime_type, current_version, created_at, updated_at 
			FROM files 
			WHERE user_id = $1 AND folder_id = $2 AND is_deleted = FALSE 
			ORDER BY name ASC`, userID, parentID)
	}

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch files"})
		return
	}
	defer filesRows.Close()

	type FileResponse struct {
		ID             string    `json:"id"`
		Name           string    `json:"name"`
		FolderID       *string   `json:"folder_id"`
		Size           int64     `json:"size"`
		MimeType       string    `json:"mime_type"`
		CurrentVersion int       `json:"current_version"`
		CreatedAt      time.Time `json:"created_at"`
		UpdatedAt      time.Time `json:"updated_at"`
	}

	var filesList []FileResponse
	for filesRows.Next() {
		var f FileResponse
		var fID sql.NullString
		err := filesRows.Scan(&f.ID, &f.Name, &fID, &f.Size, &f.MimeType, &f.CurrentVersion, &f.CreatedAt, &f.UpdatedAt)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to scan file rows"})
			return
		}
		if fID.Valid {
			s := fID.String
			f.FolderID = &s
		}
		filesList = append(filesList, f)
	}

	respData := gin.H{
		"folders": foldersList,
		"files":   filesList,
	}

	// Save to Redis Cache (expires in 5 minutes)
	respBytes, err := json.Marshal(respData)
	if err == nil {
		_ = cache.Set(ctx, cacheKey, string(respBytes), 5*time.Minute)
	}

	c.JSON(http.StatusOK, respData)
}

// UploadFile handles POST /api/files/upload
func UploadFile(c *gin.Context) {
	userID, _ := c.Get("userID")

	// Parse file from request form
	fileHeader, err := c.FormFile("file")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "No file uploaded"})
		return
	}

	// Validate file size and type (Limit: 50MB for non-chunked uploads)
	sanitizedName, err := utils.ValidateFileMetadata(fileHeader.Filename, fileHeader.Size, 50*1024*1024)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	folderID := c.PostForm("folder_id")
	if folderID == "" {
		folderID = "" // Treat as null
	}

	ctx := context.Background()

	// If folder ID provided, verify it belongs to user
	if folderID != "" {
		var folderExists bool
		err = db.Pool.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM folders WHERE id = $1 AND user_id = $2)", folderID, userID).Scan(&folderExists)
		if err != nil || !folderExists {
			c.JSON(http.StatusNotFound, gin.H{"error": "Destination folder not found"})
			return
		}
	}

	file, err := fileHeader.Open()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to open uploaded file"})
		return
	}
	defer file.Close()

	// 1. Calculate file SHA-256 hash for deduplication
	hashWriter := sha256.New()
	if _, err := io.Copy(hashWriter, file); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to hash file"})
		return
	}
	fileHash := hex.EncodeToString(hashWriter.Sum(nil))

	// Reset file pointer reader back to the beginning of the file
	_, err = file.Seek(0, io.SeekStart)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to reset file pointer"})
		return
	}

	// Transaction to insert file records
	tx, err := db.Pool.Begin(ctx)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Transaction boot failed"})
		return
	}
	defer tx.Rollback(ctx)

	// 2. Check Deduplication: Has anyone uploaded this EXACT hash?
	var existingKey string
	err = tx.QueryRow(ctx, "SELECT key FROM file_versions WHERE hash = $1 LIMIT 1", fileHash).Scan(&existingKey)

	var objectKey string
	isDuplicate := false

	if err == nil && existingKey != "" {
		// DEDUPLICATION HIT! We already have this block stored in S3/MinIO.
		objectKey = existingKey
		isDuplicate = true
	} else {
		// DEDUPLICATION MISS! We need to upload to S3/MinIO
		objectUUID := uuid.New().String()
		ext := filepath.Ext(sanitizedName)
		objectKey = fmt.Sprintf("users/%s/files/%s%s", userID, objectUUID, ext)
	}

	// Resolve folder ID mapping
	var fID interface{} = nil
	if folderID != "" {
		fID = folderID
	}

	// Check if file metadata exists already (overwriting / new version)
	var existingFileID string
	var currentVer int
	err = tx.QueryRow(ctx, "SELECT id, current_version FROM files WHERE name = $1 AND user_id = $2 AND folder_id IS NOT DISTINCT FROM $3 AND is_deleted = FALSE",
		sanitizedName, userID, fID).Scan(&existingFileID, &currentVer)

	var newFileID string
	var newVer int

	if err == nil && existingFileID != "" {
		// OVERWRITE / NEW VERSION HIT!
		newFileID = existingFileID
		newVer = currentVer + 1

		// Update files table size, version, and hash
		_, err = tx.Exec(ctx, "UPDATE files SET size = $1, current_version = $2, hash = $3, updated_at = NOW() WHERE id = $4",
			fileHeader.Size, newVer, fileHash, newFileID)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update file record version"})
			return
		}
	} else {
		// FULLY NEW FILE UPLOAD!
		newVer = 1
		if folderID == "" {
			err = tx.QueryRow(ctx, `
				INSERT INTO files (name, folder_id, user_id, size, mime_type, current_version, hash) 
				VALUES ($1, NULL, $2, $3, $4, 1, $5) 
				RETURNING id`, sanitizedName, userID, fileHeader.Size, fileHeader.Header.Get("Content-Type"), fileHash).Scan(&newFileID)
		} else {
			err = tx.QueryRow(ctx, `
				INSERT INTO files (name, folder_id, user_id, size, mime_type, current_version, hash) 
				VALUES ($1, $2, $3, $4, $5, 1, $6) 
				RETURNING id`, sanitizedName, folderID, userID, fileHeader.Size, fileHeader.Header.Get("Content-Type"), fileHash).Scan(&newFileID)
		}

		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create file record"})
			return
		}
	}

	// Insert into file versions table
	_, err = tx.Exec(ctx, `
		INSERT INTO file_versions (file_id, version_number, size, key, hash, created_by) 
		VALUES ($1, $2, $3, $4, $5, $6)`,
		newFileID, newVer, fileHeader.Size, objectKey, fileHash, userID)

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create file version record"})
		return
	}

	// If not duplicate, upload to MinIO S3
	if !isDuplicate {
		err = storage.PutObject(ctx, objectKey, file, fileHeader.Size, fileHeader.Header.Get("Content-Type"))
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to store file in cloud storage"})
			return
		}
	}

	// Commit Transaction
	if err := tx.Commit(ctx); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to finalize file upload transaction"})
		return
	}

	// Trigger background AI processing
	ai.ProcessFileAIBackground(newFileID, sanitizedName, fileHeader.Header.Get("Content-Type"), objectKey)

	// Invalidate directory cache
	cache.InvalidateDirCache(ctx, userID.(string), folderID)

	c.JSON(http.StatusCreated, gin.H{
		"message":      "File uploaded successfully",
		"file_id":      newFileID,
		"name":         sanitizedName,
		"size":         fileHeader.Size,
		"version":      newVer,
		"deduplicated": isDuplicate,
	})
}

// DownloadFile handles GET /api/files/:id/download
func DownloadFile(c *gin.Context) {
	userID, _ := c.Get("userID")
	fileID := c.Param("id")

	ctx := context.Background()

	// Verify ownership and get latest version S3 Key
	var objectKey string
	var filename string
	err := db.Pool.QueryRow(ctx, `
		SELECT f.name, fv.key 
		FROM files f 
		JOIN file_versions fv ON f.id = fv.file_id AND f.current_version = fv.version_number 
		WHERE f.id = $1 AND f.user_id = $2 AND f.is_deleted = FALSE`, fileID, userID).Scan(&filename, &objectKey)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			c.JSON(http.StatusNotFound, gin.H{"error": "File not found or access denied"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Database lookup error"})
		return
	}

	// Generate secure S3 presigned URL expiring in 15 minutes
	presignedURL, err := storage.GetPresignedDownloadURL(ctx, objectKey, filename, 15*time.Minute)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to generate download URL"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"download_url": presignedURL,
	})
}

// DeleteFile handles DELETE /api/files/:id (Soft delete)
func DeleteFile(c *gin.Context) {
	userID, _ := c.Get("userID")
	fileID := c.Param("id")

	ctx := context.Background()

	// Set is_deleted = true, soft delete and return the folder_id of the file to invalidate the cache
	var folderID sql.NullString
	err := db.Pool.QueryRow(ctx, `
		UPDATE files 
		SET is_deleted = TRUE, deleted_at = NOW() 
		WHERE id = $1 AND user_id = $2 AND is_deleted = FALSE 
		RETURNING folder_id`, fileID, userID).Scan(&folderID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			c.JSON(http.StatusNotFound, gin.H{"error": "File not found or access denied"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to delete file"})
		return
	}

	// Invalidate directory cache
	fID := ""
	if folderID.Valid {
		fID = folderID.String
	}
	cache.InvalidateDirCache(ctx, userID.(string), fID)

	c.JSON(http.StatusOK, gin.H{"message": "File moved to trash successfully"})
}

// RenameOrMoveFile handles PATCH /api/files/:id
func RenameOrMoveFile(c *gin.Context) {
	userID, _ := c.Get("userID")
	fileID := c.Param("id")

	var req UpdateFileRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	ctx := context.Background()

	// Get current folder_id to invalidate cache afterwards
	var currentFolderID sql.NullString
	err := db.Pool.QueryRow(ctx, "SELECT folder_id FROM files WHERE id = $1 AND user_id = $2 AND is_deleted = FALSE", fileID, userID).Scan(&currentFolderID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			c.JSON(http.StatusNotFound, gin.H{"error": "File not found or access denied"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Database lookup error"})
		return
	}

	// Update columns dynamically
	if req.Name != nil && *req.Name != "" {
		// Prevent path traversal sequences in name
		sanitizedName := filepath.Base(*req.Name)
		if sanitizedName == "." || sanitizedName == "/" || sanitizedName == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid file name"})
			return
		}

		_, err = db.Pool.Exec(ctx, "UPDATE files SET name = $1, updated_at = NOW() WHERE id = $2", sanitizedName, fileID)
		if err != nil {
			c.JSON(http.StatusConflict, gin.H{"error": "A file with this name already exists in this folder"})
			return
		}
	}

	if req.FolderID != nil {
		fID := *req.FolderID
		if fID == "" {
			// Move to root
			_, err = db.Pool.Exec(ctx, "UPDATE files SET folder_id = NULL, updated_at = NOW() WHERE id = $1", fileID)
		} else {
			// Verify destination folder
			var folderExists bool
			err = db.Pool.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM folders WHERE id = $1 AND user_id = $2)", fID, userID).Scan(&folderExists)
			if err != nil || !folderExists {
				c.JSON(http.StatusNotFound, gin.H{"error": "Destination folder not found"})
				return
			}
			_, err = db.Pool.Exec(ctx, "UPDATE files SET folder_id = $1, updated_at = NOW() WHERE id = $2", fID, fileID)
		}

		if err != nil {
			c.JSON(http.StatusConflict, gin.H{"error": "A file with this name already exists in the destination folder"})
			return
		}
	}

	// Invalidate caches
	oldFID := ""
	if currentFolderID.Valid {
		oldFID = currentFolderID.String
	}
	cache.InvalidateDirCache(ctx, userID.(string), oldFID)

	if req.FolderID != nil && *req.FolderID != oldFID {
		cache.InvalidateDirCache(ctx, userID.(string), *req.FolderID)
	}

	c.JSON(http.StatusOK, gin.H{"message": "File updated successfully"})
}

// RenameOrMoveFolder handles PATCH /api/folders/:id
func RenameOrMoveFolder(c *gin.Context) {
	userID, _ := c.Get("userID")
	folderID := c.Param("id")

	var req UpdateFolderRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	ctx := context.Background()

	// Get current parent_id to invalidate cache afterwards
	var currentParentID sql.NullString
	err := db.Pool.QueryRow(ctx, "SELECT parent_id FROM folders WHERE id = $1 AND user_id = $2", folderID, userID).Scan(&currentParentID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			c.JSON(http.StatusNotFound, gin.H{"error": "Folder not found or access denied"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Database lookup error"})
		return
	}

	// Update columns dynamically
	if req.Name != nil && *req.Name != "" {
		// Prevent path traversal/empty names
		sanitizedName := filepath.Base(*req.Name)
		if sanitizedName == "." || sanitizedName == "/" || sanitizedName == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid folder name"})
			return
		}

		_, err = db.Pool.Exec(ctx, "UPDATE folders SET name = $1, updated_at = NOW() WHERE id = $2", sanitizedName, folderID)
		if err != nil {
			c.JSON(http.StatusConflict, gin.H{"error": "A folder with this name already exists in this parent folder"})
			return
		}
	}

	if req.ParentID != nil {
		pID := *req.ParentID
		if pID == folderID {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Cannot move a folder into itself"})
			return
		}

		if pID == "" {
			// Move to root
			_, err = db.Pool.Exec(ctx, "UPDATE folders SET parent_id = NULL, updated_at = NOW() WHERE id = $1", folderID)
		} else {
			// Verify destination folder
			var parentExists bool
			err = db.Pool.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM folders WHERE id = $1 AND user_id = $2)", pID, userID).Scan(&parentExists)
			if err != nil || !parentExists {
				c.JSON(http.StatusNotFound, gin.H{"error": "Parent folder not found"})
				return
			}
			_, err = db.Pool.Exec(ctx, "UPDATE folders SET parent_id = $1, updated_at = NOW() WHERE id = $2", pID, folderID)
		}

		if err != nil {
			c.JSON(http.StatusConflict, gin.H{"error": "A folder with this name already exists in the destination folder"})
			return
		}
	}

	// Invalidate caches
	oldPID := ""
	if currentParentID.Valid {
		oldPID = currentParentID.String
	}
	cache.InvalidateDirCache(ctx, userID.(string), oldPID)

	if req.ParentID != nil && *req.ParentID != oldPID {
		cache.InvalidateDirCache(ctx, userID.(string), *req.ParentID)
	}

	c.JSON(http.StatusOK, gin.H{"message": "Folder updated successfully"})
}

// DeleteFolder handles DELETE /api/folders/:id
func DeleteFolder(c *gin.Context) {
	userID, _ := c.Get("userID")
	folderID := c.Param("id")

	ctx := context.Background()

	// Retrieve parent_id first to invalidate parent folder's cache list
	var parentID sql.NullString
	err := db.Pool.QueryRow(ctx, "SELECT parent_id FROM folders WHERE id = $1 AND user_id = $2", folderID, userID).Scan(&parentID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			c.JSON(http.StatusNotFound, gin.H{"error": "Folder not found or access denied"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Database lookup error"})
		return
	}

	// Delete folder. Due to cascade references (ON DELETE CASCADE in migrations)
	// all subfolders and files linked in database are deleted as well.
	// Note: We might want to soft delete files recursively or delete objects in MinIO.
	// For production level, we would queue background jobs to delete objects in MinIO.
	// Here, we delete DB folder row.
	_, err = db.Pool.Exec(ctx, "DELETE FROM folders WHERE id = $1", folderID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to delete folder"})
		return
	}

	// Invalidate parent directory cache
	pID := ""
	if parentID.Valid {
		pID = parentID.String
	}
	cache.InvalidateDirCache(ctx, userID.(string), pID)

	c.JSON(http.StatusOK, gin.H{"message": "Folder and all its contents deleted successfully"})
}
