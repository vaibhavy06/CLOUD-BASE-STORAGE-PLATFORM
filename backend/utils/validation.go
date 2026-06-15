package utils

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"
)

// List of dangerous executable and script file extensions
var BlockedExtensions = map[string]bool{
	".exe":  true,
	".bat":  true,
	".sh":   true,
	".cmd":  true,
	".com":  true,
	".scr":  true,
	".msi":  true,
	".dll":  true,
	".vbs":  true,
	".js":   true,
	".jar":  true,
	".sys":  true,
	".lnk":  true,
	".reg":  true,
	".ps1":  true,
	".bash": true,
}

// ValidateFileMetadata validates filename and size limits, returning sanitized filename
func ValidateFileMetadata(filename string, size int64, maxLimit int64) (string, error) {
	if size <= 0 {
		return "", errors.New("file is empty")
	}

	if size > maxLimit {
		return "", fmt.Errorf("file size %d exceeds limit of %d bytes", size, maxLimit)
	}

	// 1. Prevent path traversal by getting only the base name of the path
	sanitizedName := filepath.Base(filename)
	if sanitizedName == "." || sanitizedName == "/" || sanitizedName == "" {
		return "", errors.New("invalid file name")
	}

	// 2. Extension check
	ext := strings.ToLower(filepath.Ext(sanitizedName))
	if BlockedExtensions[ext] {
		return "", fmt.Errorf("file extension %s is restricted for security reasons", ext)
	}

	return sanitizedName, nil
}
