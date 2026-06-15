package handlers

import (
	"context"
	"database/sql"
	"net/http"
	"time"

	"backend/db"

	"github.com/gin-gonic/gin"
)

type SearchResult struct {
	ID             string    `json:"id"`
	Name           string    `json:"name"`
	FolderID       *string   `json:"folder_id"`
	Size           int64     `json:"size"`
	MimeType       string    `json:"mime_type"`
	CurrentVersion int       `json:"current_version"`
	Summary        *string   `json:"summary"`
	Tags           []string  `json:"tags"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

// SearchFiles handles GET /api/search
func SearchFiles(c *gin.Context) {
	userID, _ := c.Get("userID")
	q := c.Query("q")

	if q == "" {
		c.JSON(http.StatusOK, []SearchResult{})
		return
	}

	ctx := context.Background()
	pattern := "%" + q + "%"

	// Query DB using case-insensitive search on Name, Summary, OCR Text, or Tags list
	rows, err := db.Pool.Query(ctx, `
		SELECT id, name, folder_id, size, mime_type, current_version, summary, tags, created_at, updated_at 
		FROM files 
		WHERE user_id = $1 
		  AND is_deleted = FALSE 
		  AND (
		      name ILIKE $2 
		      OR summary ILIKE $2 
		      OR extracted_text ILIKE $2 
		      OR ARRAY_TO_STRING(tags, ',') ILIKE $2
		  ) 
		ORDER BY name ASC`, userID, pattern)

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to execute search query"})
		return
	}
	defer rows.Close()

	results := []SearchResult{}
	for rows.Next() {
		var r SearchResult
		var folderID sql.NullString
		var summary sql.NullString
		var dbTags []string

		err := rows.Scan(&r.ID, &r.Name, &folderID, &r.Size, &r.MimeType, &r.CurrentVersion, &summary, &dbTags, &r.CreatedAt, &r.UpdatedAt)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to parse search results"})
			return
		}

		if folderID.Valid {
			s := folderID.String
			r.FolderID = &s
		}
		if summary.Valid {
			s := summary.String
			r.Summary = &s
		}
		r.Tags = dbTags
		if r.Tags == nil {
			r.Tags = []string{}
		}

		results = append(results, r)
	}

	c.JSON(http.StatusOK, results)
}
