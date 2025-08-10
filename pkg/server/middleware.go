package server

import (
	"log"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

// authMiddleware performs authentication for S3 API requests
func (s *Server) authMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		// Skip authentication for dashboard, static files, and health check endpoints
		if strings.HasPrefix(c.Request.URL.Path, "/dashboard") ||
			strings.HasPrefix(c.Request.URL.Path, "/static/") ||
			c.Request.URL.Path == "/health" {
			c.Next()
			return
		}

		// Check if this is a request for a public bucket
	if c.Request.Method == "GET" || c.Request.Method == "HEAD" {
		bucket := c.Param("bucket")
		if bucket != "" && s.tenantManager != nil {
			// Check if this bucket is public for any tenant
			isPublic, tenantAccessKey := s.tenantManager.IsPublicBucket(bucket)
			if isPublic {
				// Allow public access for GET/HEAD requests to public buckets
				accessType := "public"
				log.Printf("[AUTH] Access granted - Method: %s, Bucket: %s, Type: %s, Tenant: %s", 
					c.Request.Method, bucket, accessType, tenantAccessKey)
				// Set a special marker for public access
				c.Set("publicAccess", true)
				// Set the tenant's access key for proper storage routing
				c.Set("accessKey", tenantAccessKey)
				c.Set("tenantDirectory", s.tenantManager.GetDirectory(tenantAccessKey))
				c.Next()
				return
			}
		}
	} else {
		// For non-GET/HEAD methods, check if this is a public bucket and deny access
		bucket := c.Param("bucket")
		if bucket != "" && s.tenantManager != nil {
			isPublic, _ := s.tenantManager.IsPublicBucket(bucket)
			if isPublic {
				// Public buckets are read-only
				log.Printf("[AUTH] Access denied - Method: %s, Bucket: %s, Reason: Public buckets are read-only", 
					c.Request.Method, bucket)
				c.Header("Content-Type", "application/xml")
				c.XML(http.StatusForbidden, gin.H{
					"Error": gin.H{
						"Code":    "AccessDenied",
						"Message": "Public buckets are read-only",
					},
				})
				c.Abort()
				return
			}
		}
	}

	// Perform authentication
	accessKey, err := s.authHandler.Authenticate(c.Request)
	if err != nil {
		// Send S3-compatible error response
		c.Header("Content-Type", "application/xml")
		c.XML(http.StatusForbidden, gin.H{
			"Error": gin.H{
				"Code":    "AccessDenied",
				"Message": err.Error(),
			},
		})
		c.Abort()
		return
	}

	// Determine authentication type
	authType := "sigv4"
	if c.Request.URL.Query().Get("X-Amz-Signature") != "" {
		authType = "presigned"
	}

	// Log access with authentication type
	bucket := c.Param("bucket")
	log.Printf("[AUTH] Access granted - Method: %s, Bucket: %s, Type: %s, AccessKey: %s", 
		c.Request.Method, bucket, authType, accessKey)

		// Store access key in context for later use
		c.Set("accessKey", accessKey)

		// If using tenant manager, set tenant directory
		if s.tenantManager != nil {
			c.Set("tenantDirectory", s.tenantManager.GetDirectory(accessKey))
		}

		c.Next()
	}
}
