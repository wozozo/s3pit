package logger

import (
	"bytes"
	"io"
	"time"

	"github.com/gin-gonic/gin"
)

// responseWriter wraps gin.ResponseWriter to capture response body
type responseWriter struct {
	gin.ResponseWriter
	body *bytes.Buffer
}

func (w *responseWriter) Write(data []byte) (int, error) {
	w.body.Write(data)
	return w.ResponseWriter.Write(data)
}

// S3APILoggingMiddleware provides detailed logging for S3 API requests
func S3APILoggingMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		// Skip dashboard and static paths
		if len(c.Request.URL.Path) >= 10 && c.Request.URL.Path[:10] == "/dashboard" {
			c.Next()
			return
		}
		if len(c.Request.URL.Path) >= 7 && c.Request.URL.Path[:7] == "/static" {
			c.Next()
			return
		}
		// Skip favicon requests
		if c.Request.URL.Path == "/favicon.ico" {
			c.Next()
			return
		}

		start := time.Now()

		// Capture request body
		var requestBody []byte
		if c.Request.Body != nil {
			requestBody, _ = io.ReadAll(c.Request.Body)
			c.Request.Body = io.NopCloser(bytes.NewReader(requestBody))
		}

		// Wrap response writer to capture response
		rw := &responseWriter{
			ResponseWriter: c.Writer,
			body:           bytes.NewBuffer(nil),
		}
		c.Writer = rw

		// Process request
		c.Next()

		// Log the request
		GetInstance().LogRequest(c, start, rw.body.Bytes())
	}
}

// DashboardLoggingMiddleware provides lightweight logging for dashboard requests
func DashboardLoggingMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		// Skip logging for dashboard API calls - just process the request
		c.Next()
	}
}
