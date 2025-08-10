package dashboard

import (
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/wozozo/s3pit/pkg/auth"
	"github.com/wozozo/s3pit/pkg/logger"
	"github.com/wozozo/s3pit/pkg/storage"
	"github.com/wozozo/s3pit/pkg/tenant"
)

//go:embed templates/*
var templateFS embed.FS

//go:embed static/*
var staticFS embed.FS

type Handler struct {
	storage         storage.Storage
	tenant          *tenant.Manager
	authMode        string
	region          string
}

func NewHandler(s storage.Storage, tm *tenant.Manager, authMode, region string) *Handler {
	if region == "" {
		region = "us-east-1"
	}
	return &Handler{
		storage:         s,
		tenant:          tm,
		authMode:        authMode,
		region:          region,
	}
}

// getStorage returns the appropriate storage for the current request
// Similar to API handler's getStorage method
func (h *Handler) getStorage(c *gin.Context) storage.Storage {
	// If using tenant-aware storage, get the tenant-specific storage
	if tenantStorage, ok := h.storage.(*storage.TenantAwareStorage); ok {
		// Get access key from context (set by our tenant detection)
		if accessKey, exists := c.Get("accessKey"); exists {
			if key, ok := accessKey.(string); ok && key != "" {
				// Get the tenant-specific storage (thread-safe)
				storage, err := tenantStorage.GetStorageForTenant(key)
				if err != nil {
					// Log error but return the base storage as fallback
					fmt.Printf("[DASHBOARD STORAGE ERROR] Failed to get tenant storage for key %s: %v\n", key, err)
					return h.storage
				}
				return storage
			}
		}
	}
	return h.storage
}

func (h *Handler) RegisterRoutes(r *gin.Engine) {
	// Serve static files from the embedded filesystem
	staticContent, _ := fs.Sub(staticFS, "static")
	r.StaticFS("/static", http.FS(staticContent))

	// Dashboard routes
	dashboard := r.Group("/dashboard")
	{
		dashboard.GET("/", h.handleDashboard)
		dashboard.GET("/api/buckets", h.handleListBuckets)
		dashboard.POST("/api/buckets", h.handleCreateBucket)
		dashboard.DELETE("/api/buckets/:bucket", h.handleDeleteBucket)
		dashboard.GET("/api/buckets/:bucket/objects", h.handleListObjects)
		dashboard.POST("/api/buckets/:bucket/objects", h.handleUploadObject)
		dashboard.DELETE("/api/buckets/:bucket/objects/*key", h.handleDeleteObject)
		dashboard.POST("/api/presigned-url", h.handleGeneratePresignedURL)
		dashboard.GET("/api/auth-config", h.handleGetAuthConfig)
		dashboard.GET("/api/tenants", h.handleListTenants)
		dashboard.GET("/api/logs", h.handleGetLogs)
	}
}

func (h *Handler) handleDashboard(c *gin.Context) {
	// Serve the HTML file directly without template parsing
	htmlContent, err := templateFS.ReadFile("templates/index.html")
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}

	c.Header("Content-Type", "text/html; charset=utf-8")
	_, _ = c.Writer.Write(htmlContent)
}

func (h *Handler) handleListBuckets(c *gin.Context) {
	// Get all buckets from all tenants
	allBuckets := make([]map[string]interface{}, 0)

	if h.tenant != nil {
		// First, get buckets from the global directory
		globalDir := h.tenant.GetGlobalDir()
		if globalDir != "" {
			// Read global directory directly to find buckets
			entries, err := os.ReadDir(globalDir)
			if err == nil {
				for _, entry := range entries {
					if entry.IsDir() && !strings.HasPrefix(entry.Name(), ".") {
						info, err := entry.Info()
						if err != nil {
							continue
						}

						creationTime := info.ModTime()

						// Try to read bucket metadata
						metaPath := filepath.Join(globalDir, entry.Name(), ".s3pit_bucket_meta.json")
						if data, err := os.ReadFile(metaPath); err == nil {
							var meta map[string]interface{}
							if json.Unmarshal(data, &meta) == nil {
								if created, ok := meta["created"].(string); ok {
									if t, err := time.Parse(time.RFC3339, created); err == nil {
										creationTime = t
									}
								}
							}
						}

						allBuckets = append(allBuckets, map[string]interface{}{
							"name":         entry.Name(),
							"creationDate": creationTime,
							"tenant":       "global",
							"tenantDesc":   "Global Directory",
							"path":         filepath.Join(globalDir, entry.Name()),
						})
					}
				}
			}
		}

		// List buckets for each tenant by reading their directories directly
		for _, tenant := range h.tenant.ListTenants() {
			tenantDir := h.tenant.GetDirectory(tenant.AccessKeyID)

			// Read tenant directory to find buckets
			entries, err := os.ReadDir(tenantDir)
			if err != nil {
				continue // Skip this tenant on error
			}

			for _, entry := range entries {
				if entry.IsDir() && !strings.HasPrefix(entry.Name(), ".") {
					info, err := entry.Info()
					if err != nil {
						continue
					}

					creationTime := info.ModTime()

					// Try to read bucket metadata
					metaPath := filepath.Join(tenantDir, entry.Name(), ".s3pit_bucket_meta.json")
					if data, err := os.ReadFile(metaPath); err == nil {
						var meta map[string]interface{}
						if json.Unmarshal(data, &meta) == nil {
							if created, ok := meta["created"].(string); ok {
								if t, err := time.Parse(time.RFC3339, created); err == nil {
									creationTime = t
								}
							}
						}
					}

					allBuckets = append(allBuckets, map[string]interface{}{
						"name":         entry.Name(),
						"creationDate": creationTime,
						"tenant":       tenant.AccessKeyID,
						"tenantDesc":   tenant.Description,
						"path":         filepath.Join(tenantDir, entry.Name()),
					})
				}
			}
		}
	} else {
		// Fallback to regular bucket listing
		buckets, err := h.storage.ListBuckets()
		if err != nil {
			c.JSON(500, gin.H{"error": err.Error()})
			return
		}

		for _, b := range buckets {
			allBuckets = append(allBuckets, map[string]interface{}{
				"name":         b.Name,
				"creationDate": b.CreationDate,
			})
		}
	}

	c.JSON(200, gin.H{"buckets": allBuckets})
}

func (h *Handler) handleCreateBucket(c *gin.Context) {
	var req struct {
		Name            string `json:"name"`
		TenantAccessKey string `json:"tenantAccessKey,omitempty"` // Allow specifying tenant
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}

	// Set tenant context if specified, otherwise use first tenant
	var tenantAccessKey string
	if req.TenantAccessKey != "" {
		tenantAccessKey = req.TenantAccessKey
	} else if h.tenant != nil {
		// Default to first tenant if not specified
		tenants := h.tenant.ListTenants()
		if len(tenants) > 0 {
			tenantAccessKey = tenants[0].AccessKeyID
		}
	}

	// Set tenant context if found
	if tenantAccessKey != "" {
		c.Set("accessKey", tenantAccessKey)
		c.Set("tenantDirectory", h.tenant.GetDirectory(tenantAccessKey))
	}

	// Use getStorage method
	storage := h.getStorage(c)
	_, err := storage.CreateBucket(req.Name)
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}

	c.JSON(200, gin.H{"message": "Bucket created", "name": req.Name})
}

func (h *Handler) handleDeleteBucket(c *gin.Context) {
	bucket := c.Param("bucket")

	// Find which tenant owns this bucket and set context
	var tenantAccessKey string
	if h.tenant != nil {
		for _, tenant := range h.tenant.ListTenants() {
			tenantDir := h.tenant.GetDirectory(tenant.AccessKeyID)
			bucketPath := filepath.Join(tenantDir, bucket)
			if _, err := os.Stat(bucketPath); err == nil {
				tenantAccessKey = tenant.AccessKeyID
				break
			}
		}
	}

	// Set tenant context if found
	if tenantAccessKey != "" {
		c.Set("accessKey", tenantAccessKey)
		c.Set("tenantDirectory", h.tenant.GetDirectory(tenantAccessKey))
	}

	// Use getStorage method
	storage := h.getStorage(c)
	if err := storage.DeleteBucket(bucket); err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}

	c.JSON(200, gin.H{"message": "Bucket deleted"})
}

func (h *Handler) handleListObjects(c *gin.Context) {
	bucket := c.Param("bucket")
	prefix := c.Query("prefix")

	// Find which tenant owns this bucket and read objects directly
	var objResult []map[string]interface{}
	var commonPrefixes []string
	found := false

	if h.tenant != nil {
		for _, tenant := range h.tenant.ListTenants() {
			tenantDir := h.tenant.GetDirectory(tenant.AccessKeyID)
			bucketPath := filepath.Join(tenantDir, bucket)

			if _, err := os.Stat(bucketPath); err == nil {
				found = true
				// Read objects directly from filesystem
				err := filepath.Walk(bucketPath, func(path string, info os.FileInfo, err error) error {
					if err != nil {
						return nil
					}

					if info.IsDir() || strings.Contains(info.Name(), ".s3pit_") {
						return nil
					}

					relPath, err := filepath.Rel(bucketPath, path)
					if err != nil {
						return nil
					}

					key := filepath.ToSlash(relPath)

					if prefix != "" && !strings.HasPrefix(key, prefix) {
						return nil
					}

					// Get metadata
					var etag string
					metaPath := path + ".s3pit_meta.json"
					if data, err := os.ReadFile(metaPath); err == nil {
						var meta map[string]interface{}
						if json.Unmarshal(data, &meta) == nil {
							if e, ok := meta["etag"].(string); ok {
								if !strings.HasPrefix(e, "\"") {
									etag = fmt.Sprintf("\"%s\"", e)
								} else {
									etag = e
								}
							}
						}
					}

					objResult = append(objResult, map[string]interface{}{
						"key":          key,
						"size":         info.Size(),
						"lastModified": info.ModTime(),
						"etag":         etag,
					})

					return nil
				})

				if err != nil {
					c.JSON(500, gin.H{"error": err.Error()})
					return
				}
				break
			}
		}
	}

	if !found {
		c.JSON(404, gin.H{"error": "bucket not found"})
		return
	}

	c.JSON(200, gin.H{
		"objects":        objResult,
		"commonPrefixes": commonPrefixes,
	})
}

func (h *Handler) handleUploadObject(c *gin.Context) {
	bucket := c.Param("bucket")

	file, header, err := c.Request.FormFile("file")
	if err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}
	defer file.Close()

	key := c.Request.FormValue("key")
	if key == "" {
		key = header.Filename
	}

	contentType := header.Header.Get("Content-Type")
	if contentType == "" {
		contentType = "application/octet-stream"
	}

	// Find which tenant owns this bucket and set context
	var tenantAccessKey string
	if h.tenant != nil {
		for _, tenant := range h.tenant.ListTenants() {
			tenantDir := h.tenant.GetDirectory(tenant.AccessKeyID)
			bucketPath := filepath.Join(tenantDir, bucket)
			if _, err := os.Stat(bucketPath); err == nil {
				tenantAccessKey = tenant.AccessKeyID
				break
			}
		}
	}

	// Set tenant context if found
	if tenantAccessKey != "" {
		c.Set("accessKey", tenantAccessKey)
		c.Set("tenantDirectory", h.tenant.GetDirectory(tenantAccessKey))
	}

	// Use getStorage method
	storage := h.getStorage(c)
	_, err = storage.PutObject(bucket, key, file, header.Size, contentType)
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}

	c.JSON(200, gin.H{"message": "Object uploaded", "key": key})
}

func (h *Handler) handleDeleteObject(c *gin.Context) {
	bucket := c.Param("bucket")
	key := c.Param("key")
	if key != "" && key[0] == '/' {
		key = key[1:]
	}

	// Find which tenant owns this bucket and set context
	var tenantAccessKey string
	if h.tenant != nil {
		for _, tenant := range h.tenant.ListTenants() {
			tenantDir := h.tenant.GetDirectory(tenant.AccessKeyID)
			bucketPath := filepath.Join(tenantDir, bucket)
			if _, err := os.Stat(bucketPath); err == nil {
				tenantAccessKey = tenant.AccessKeyID
				break
			}
		}
	}

	// Set tenant context if found
	if tenantAccessKey != "" {
		c.Set("accessKey", tenantAccessKey)
		c.Set("tenantDirectory", h.tenant.GetDirectory(tenantAccessKey))
	}

	// Use getStorage method like API handler does
	storage := h.getStorage(c)
	if err := storage.DeleteObject(bucket, key); err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}

	c.JSON(200, gin.H{"message": "Object deleted"})
}

func (h *Handler) handleGeneratePresignedURL(c *gin.Context) {
	var req struct {
		Bucket      string            `json:"bucket"`
		Key         string            `json:"key"`
		Operation   string            `json:"operation"` // "GET" or "PUT"
		Expires     int               `json:"expires"`   // seconds
		ContentType string            `json:"contentType,omitempty"`
		Headers     map[string]string `json:"headers,omitempty"`
		// Optional: Allow client to provide credentials
		AccessKeyID     string `json:"accessKeyId,omitempty"`
		SecretAccessKey string `json:"secretAccessKey,omitempty"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}

	if req.Expires == 0 {
		req.Expires = 3600 // Default 1 hour
	}

	// Use provided credentials from request
	accessKeyID := req.AccessKeyID
	secretAccessKey := req.SecretAccessKey

	// Check if we have credentials for SigV4
	if accessKeyID == "" || secretAccessKey == "" {
		c.JSON(400, gin.H{"error": "AccessKeyID and SecretAccessKey required for SigV4 presigned URLs"})
		return
	}

	// Generate presigned URL with SigV4
	signer := auth.NewSigV4Signer(accessKeyID, secretAccessKey, h.region)
	opts := auth.PresignedURLOptions{
		Method:      req.Operation,
		Bucket:      req.Bucket,
		Key:         req.Key,
		Expires:     req.Expires,
		ContentType: req.ContentType,
		Headers:     req.Headers,
	}

	host := c.Request.Host
	if c.Request.TLS != nil {
		host = "https://" + host
	}

	url, err := signer.GeneratePresignedURL(host, opts)
	if err != nil {
		c.JSON(500, gin.H{"error": "Failed to generate presigned URL: " + err.Error()})
		return
	}

	c.JSON(200, gin.H{
		"url":     url,
		"expires": req.Expires,
		"method":  req.Operation,
	})
}

func (h *Handler) handleListTenants(c *gin.Context) {
	tenants := h.tenant.GetAllTenants()

	var result []map[string]string
	for accessKey, rootDir := range tenants {
		result = append(result, map[string]string{
			"accessKey": accessKey,
			"rootDir":   rootDir,
		})
	}

	c.JSON(200, gin.H{"tenants": result})
}

func (h *Handler) handleGetLogs(c *gin.Context) {
	limit := 100
	if l := c.Query("limit"); l != "" {
		_, _ = fmt.Sscanf(l, "%d", &limit)
	}

	// Get filter parameters
	level := c.Query("level")
	operation := c.Query("operation")

	// Parse time range if provided
	var startTime, endTime *time.Time
	if st := c.Query("start_time"); st != "" {
		if t, err := time.Parse(time.RFC3339, st); err == nil {
			startTime = &t
		}
	}
	if et := c.Query("end_time"); et != "" {
		if t, err := time.Parse(time.RFC3339, et); err == nil {
			endTime = &t
		}
	}

	// Use the new enhanced logger
	logs := logger.GetInstance().GetEntries(limit, level, operation, startTime, endTime)
	c.JSON(200, gin.H{"logs": logs})
}

func (h *Handler) handleGetAuthConfig(c *gin.Context) {
	config := gin.H{
		"authMode": h.authMode,
		"region":   h.region,
	}

	// No static access key - all authentication is via tenant configuration

	c.JSON(200, config)
}

// LoggingMiddleware returns the new enhanced logging middleware
func LoggingMiddleware() gin.HandlerFunc {
	return logger.DashboardLoggingMiddleware()
}
