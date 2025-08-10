package server

import (
	"log"
	"math/rand"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

// delayMiddleware adds configurable delays to S3 operations for testing purposes
func (s *Server) delayMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		// Skip delay for dashboard and static files
		if strings.HasPrefix(c.Request.URL.Path, "/dashboard") ||
			strings.HasPrefix(c.Request.URL.Path, "/static/") ||
			c.Request.URL.Path == "/health" {
			c.Next()
			return
		}

		// Determine if this is a read or write operation
		isRead := isReadOperation(c)

		// Calculate and apply delay
		var delayMs int
		if isRead {
			delayMs = calculateDelay(
				s.config.ReadDelayMs,
				s.config.ReadDelayRandomMin,
				s.config.ReadDelayRandomMax,
			)
			if delayMs > 0 {
				log.Printf("[DELAY] Applying %dms delay to READ operation: %s %s",
					delayMs, c.Request.Method, c.Request.URL.Path)
			}
		} else {
			delayMs = calculateDelay(
				s.config.WriteDelayMs,
				s.config.WriteDelayRandomMin,
				s.config.WriteDelayRandomMax,
			)
			if delayMs > 0 {
				log.Printf("[DELAY] Applying %dms delay to WRITE operation: %s %s",
					delayMs, c.Request.Method, c.Request.URL.Path)
			}
		}

		// Apply the delay
		if delayMs > 0 {
			time.Sleep(time.Duration(delayMs) * time.Millisecond)
		}

		c.Next()
	}
}

// isReadOperation determines if the current request is a read operation
func isReadOperation(c *gin.Context) bool {
	method := c.Request.Method

	// GET and HEAD are always read operations
	if method == "GET" || method == "HEAD" {
		return true
	}

	// PUT can be either read or write
	if method == "PUT" {
		// Check if it's a copy operation (which reads from source)
		// For simplicity, we'll treat copy as a write operation
		// since it creates/overwrites the destination object
		return false
	}

	// DELETE and POST are write operations
	if method == "DELETE" || method == "POST" {
		return false
	}

	// Default to read for unknown methods
	return true
}

// calculateDelay calculates the delay based on configuration
func calculateDelay(fixedMs, randomMinMs, randomMaxMs int) int {
	// If random range is configured, use it (takes precedence over fixed)
	if randomMinMs > 0 && randomMaxMs > 0 && randomMaxMs >= randomMinMs {
		// Generate random delay between min and max
		if randomMinMs == randomMaxMs {
			return randomMinMs
		}
		return randomMinMs + rand.Intn(randomMaxMs-randomMinMs+1)
	}

	// Otherwise use fixed delay
	return fixedMs
}

// OperationType represents the type of S3 operation
type OperationType string

const (
	OpTypeRead  OperationType = "READ"
	OpTypeWrite OperationType = "WRITE"
)

// GetOperationType returns the operation type for a given request
// This is a more detailed classification that can be used for fine-grained control
func GetOperationType(c *gin.Context) OperationType {
	method := c.Request.Method

	// Parse the path to understand the operation
	// S3 API patterns:
	// GET / - ListBuckets (read)
	// HEAD /{bucket} - HeadBucket (read)
	// GET /{bucket} - ListObjectsV2 (read)
	// PUT /{bucket} - CreateBucket (write)
	// DELETE /{bucket} - DeleteBucket (write)
	// HEAD /{bucket}/{key} - HeadObject (read)
	// GET /{bucket}/{key} - GetObject (read)
	// PUT /{bucket}/{key} - PutObject (write) or CopyObject (write)
	// DELETE /{bucket}/{key} - DeleteObject (write)
	// POST /{bucket} - DeleteObjects or CompleteMultipartUpload (write)
	// POST /{bucket}/{key} - InitiateMultipartUpload or UploadPart (write)

	switch method {
	case "GET", "HEAD":
		return OpTypeRead
	case "PUT", "DELETE", "POST":
		return OpTypeWrite
	default:
		// Default to read for unknown methods
		return OpTypeRead
	}
}
