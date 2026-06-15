package middleware

import (
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"github.com/gin-gonic/gin"
)

var Logger *slog.Logger

func init() {
	var writer io.Writer = os.Stdout

	// Try creating log directory and file
	logPath := "/app/logs/backend.log"
	if os.Getenv("LOG_PATH") != "" {
		logPath = os.Getenv("LOG_PATH")
	}

	dir := filepath.Dir(logPath)
	if err := os.MkdirAll(dir, 0755); err == nil {
		if file, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666); err == nil {
			writer = io.MultiWriter(os.Stdout, file)
		}
	}

	// Initialize slog handler writing JSON to multi-writer
	handler := slog.NewJSONHandler(writer, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	})
	Logger = slog.New(handler)
	slog.SetDefault(Logger)
}

// StructuredLogger returns a Gin middleware that logs HTTP requests in structured JSON format
func StructuredLogger() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		path := c.Request.URL.Path
		raw := c.Request.URL.RawQuery

		// Process request
		c.Next()

		// Collect request details
		latency := time.Since(start)
		statusCode := c.Writer.Status()
		clientIP := c.ClientIP()
		method := c.Request.Method
		errorMessage := c.Errors.ByType(gin.ErrorTypePrivate).String()

		if raw != "" {
			path = path + "?" + raw
		}

		attributes := []slog.Attr{
			slog.Int("status", statusCode),
			slog.String("method", method),
			slog.String("path", path),
			slog.String("ip", clientIP),
			slog.Duration("latency", latency),
			slog.String("user_agent", c.Request.UserAgent()),
		}

		if errorMessage != "" {
			attributes = append(attributes, slog.String("error", errorMessage))
		}

		// Log using structured slog
		if statusCode >= 500 {
			Logger.LogAttrs(c, slog.LevelError, "HTTP request failed", attributes...)
		} else if statusCode >= 400 {
			Logger.LogAttrs(c, slog.LevelWarn, "HTTP request completed with warning", attributes...)
		} else {
			Logger.LogAttrs(c, slog.LevelInfo, "HTTP request succeeded", attributes...)
		}
	}
}
