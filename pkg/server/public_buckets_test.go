package server

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/wozozo/s3pit/internal/config"
	"github.com/wozozo/s3pit/pkg/storage"
	"github.com/wozozo/s3pit/pkg/tenant"
)

// testAuthHandlerForPublicBuckets implements Handler interface for testing with tenant support
type testAuthHandlerForPublicBuckets struct {
	tenantManager *tenant.Manager
}

func (h *testAuthHandlerForPublicBuckets) Authenticate(r *http.Request) (string, error) {
	// Check for Authorization header
	authHeader := r.Header.Get("Authorization")
	if authHeader == "" {
		// Check for presigned URL
		if r.URL.Query().Get("X-Amz-Signature") != "" {
			// Extract access key from credential
			credential := r.URL.Query().Get("X-Amz-Credential")
			if credential != "" {
				parts := strings.Split(credential, "/")
				if len(parts) > 0 {
					accessKey := parts[0]
					// Verify the tenant exists
					if h.tenantManager != nil {
						if _, exists := h.tenantManager.GetTenant(accessKey); exists {
							return accessKey, nil
						}
					}
				}
			}
		}
		return "", fmt.Errorf("missing access key in request")
	}

	// Parse the authorization header for testing
	if strings.HasPrefix(authHeader, "AWS4-HMAC-SHA256") {
		// Extract credential from header
		parts := strings.Split(authHeader, " ")
		for _, part := range parts {
			if strings.HasPrefix(part, "Credential=") {
				credential := strings.TrimPrefix(part, "Credential=")
				credential = strings.TrimSuffix(credential, ",")
				credParts := strings.Split(credential, "/")
				if len(credParts) > 0 {
					accessKey := credParts[0]
					// Verify the tenant exists
					if h.tenantManager != nil {
						if _, exists := h.tenantManager.GetTenant(accessKey); exists {
							return accessKey, nil
						}
					}
					return accessKey, nil
				}
			}
		}
	}

	return "", fmt.Errorf("invalid authorization")
}


func setupTestServerWithPublicBuckets(t *testing.T) (*Server, string, func()) {
	// Create temporary directory for test
	tmpDir, err := os.MkdirTemp("", "s3pit-public-test")
	require.NoError(t, err)

	// Create test tenants.json
	tenantsFile := filepath.Join(tmpDir, "tenants.json")
	tenantsConfig := tenant.TenantsConfig{
		GlobalDir: tmpDir,
		Tenants: []tenant.Tenant{
			{
				AccessKeyID:     "public-tenant",
				SecretAccessKey: "public-secret",
				CustomDir:       "tenant-public",
				Description:     "Tenant with public buckets",
				PublicBuckets:   []string{"public-bucket", "public-*"},
			},
			{
				AccessKeyID:     "private-tenant",
				SecretAccessKey: "private-secret",
				CustomDir:       "tenant-private",
				Description:     "Tenant without public buckets",
				PublicBuckets:   []string{},
			},
		},
	}

	data, err := json.Marshal(tenantsConfig)
	require.NoError(t, err)
	err = os.WriteFile(tenantsFile, data, 0644)
	require.NoError(t, err)

	// Load tenant manager
	tenantMgr := tenant.NewManager(tenantsFile)
	err = tenantMgr.LoadFromFile()
	require.NoError(t, err)

	// Create tenant-aware storage
	storageBackend := storage.NewTenantAwareStorage(tmpDir, tenantMgr, false)

	// Use test auth handler with tenant manager
	authHandler := &testAuthHandlerForPublicBuckets{
		tenantManager: tenantMgr,
	}

	// Set up config
	cfg := &config.Config{
		Host:             "localhost",
		Port:             3333,
		GlobalDir:        tmpDir,
		AuthMode:         "sigv4",
		TenantsFile:      tenantsFile,
		InMemory:         false,
		EnableDashboard:  false,
		AutoCreateBucket: true,
	}

	// Create server
	server := &Server{
		config:        cfg,
		router:        gin.New(),
		storage:       storageBackend,
		authHandler:   authHandler,
		tenantManager: tenantMgr,
	}

	// Setup routes
	server.setupRoutes()

	// Setup test data using tenant-specific storage
	tenantStorage, ok := server.storage.(*storage.TenantAwareStorage)
	require.True(t, ok, "Expected TenantAwareStorage")

	// Get storage for public tenant
	publicStorage, err := tenantStorage.GetStorageForTenant("public-tenant")
	require.NoError(t, err)

	// Create public bucket with test object
	_, err = publicStorage.CreateBucket("public-bucket")
	require.NoError(t, err)
	content := []byte("public content")
	_, err = publicStorage.PutObject("public-bucket", "test.txt",
		bytes.NewReader(content), int64(len(content)), "text/plain")
	require.NoError(t, err)

	// Create public-prefixed bucket
	_, err = publicStorage.CreateBucket("public-data")
	require.NoError(t, err)
	content = []byte("prefixed public content")
	_, err = publicStorage.PutObject("public-data", "data.txt",
		bytes.NewReader(content), int64(len(content)), "text/plain")
	require.NoError(t, err)

	// Get storage for private tenant
	privateStorage, err := tenantStorage.GetStorageForTenant("private-tenant")
	require.NoError(t, err)

	// Create private bucket with test object
	_, err = privateStorage.CreateBucket("private-bucket")
	require.NoError(t, err)
	content = []byte("private content")
	_, err = privateStorage.PutObject("private-bucket", "secret.txt",
		bytes.NewReader(content), int64(len(content)), "text/plain")
	require.NoError(t, err)

	cleanup := func() {
		os.RemoveAll(tmpDir)
	}

	return server, tmpDir, cleanup
}

func TestPublicBucketReadAccess(t *testing.T) {
	server, _, cleanup := setupTestServerWithPublicBuckets(t)
	defer cleanup()

	gin.SetMode(gin.TestMode)

	t.Run("GET request to public bucket without auth succeeds", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/public-bucket/test.txt", nil)
		w := httptest.NewRecorder()

		server.router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
		assert.Equal(t, "public content", w.Body.String())
	})

	t.Run("HEAD request to public bucket without auth succeeds", func(t *testing.T) {
		req := httptest.NewRequest("HEAD", "/public-bucket/test.txt", nil)
		w := httptest.NewRecorder()

		server.router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
	})

	t.Run("GET request to public-prefixed bucket without auth succeeds", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/public-data/data.txt", nil)
		w := httptest.NewRecorder()

		server.router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
		assert.Equal(t, "prefixed public content", w.Body.String())
	})
}

func TestPublicBucketWriteRestriction(t *testing.T) {
	server, _, cleanup := setupTestServerWithPublicBuckets(t)
	defer cleanup()

	gin.SetMode(gin.TestMode)

	t.Run("PUT request to public bucket without auth is denied", func(t *testing.T) {
		body := bytes.NewBufferString("new content")
		req := httptest.NewRequest("PUT", "/public-bucket/new.txt", body)
		w := httptest.NewRecorder()

		server.router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusForbidden, w.Code)

		// Check error response - handle both XML formats
		responseBody := w.Body.String()
		assert.Contains(t, responseBody, "AccessDenied")
		assert.Contains(t, responseBody, "Public buckets are read-only")
	})

	t.Run("DELETE request to public bucket without auth is denied", func(t *testing.T) {
		req := httptest.NewRequest("DELETE", "/public-bucket/test.txt", nil)
		w := httptest.NewRecorder()

		server.router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusForbidden, w.Code)

		// Check error response - handle both XML formats
		responseBody := w.Body.String()
		assert.Contains(t, responseBody, "AccessDenied")
		assert.Contains(t, responseBody, "Public buckets are read-only")
	})

	t.Run("POST request to public bucket without auth is denied", func(t *testing.T) {
		req := httptest.NewRequest("POST", "/public-bucket?delete", bytes.NewBufferString(""))
		w := httptest.NewRecorder()

		server.router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusForbidden, w.Code)
	})

	t.Run("PUT request to public bucket WITH auth is also denied", func(t *testing.T) {
		body := bytes.NewBufferString("new content")
		req := httptest.NewRequest("PUT", "/public-bucket/new.txt", body)

		// Add authentication as public-tenant
		signRequestSimple(req, "public-tenant")

		w := httptest.NewRecorder()
		server.router.ServeHTTP(w, req)

		// Public buckets should be read-only even with authentication
		assert.Equal(t, http.StatusForbidden, w.Code)
		responseBody := w.Body.String()
		assert.Contains(t, responseBody, "Public buckets are read-only")
	})
}

func TestNonPublicBucketAccess(t *testing.T) {
	server, _, cleanup := setupTestServerWithPublicBuckets(t)
	defer cleanup()

	gin.SetMode(gin.TestMode)

	t.Run("GET request to non-public bucket without auth is denied", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/private-bucket/secret.txt", nil)
		w := httptest.NewRecorder()

		server.router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusForbidden, w.Code)

		// Check error response
		responseBody := w.Body.String()
		assert.Contains(t, responseBody, "AccessDenied")
	})

	t.Run("GET request to non-public bucket with valid auth succeeds", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/private-bucket/secret.txt", nil)

		// Add authentication header for private-tenant
		signRequestSimple(req, "private-tenant")

		w := httptest.NewRecorder()
		server.router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
		assert.Equal(t, "private content", w.Body.String())
	})

	t.Run("PUT request to non-public bucket with auth succeeds", func(t *testing.T) {
		body := bytes.NewBufferString("updated content")
		req := httptest.NewRequest("PUT", "/private-bucket/new-file.txt", body)

		// Add authentication header for private-tenant
		signRequestSimple(req, "private-tenant")

		w := httptest.NewRecorder()
		server.router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		// Verify the file was created
		req = httptest.NewRequest("GET", "/private-bucket/new-file.txt", nil)
		signRequestSimple(req, "private-tenant")
		w = httptest.NewRecorder()
		server.router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
		assert.Equal(t, "updated content", w.Body.String())
	})
}

func TestPresignedURLAccess(t *testing.T) {
	server, _, cleanup := setupTestServerWithPublicBuckets(t)
	defer cleanup()

	gin.SetMode(gin.TestMode)

	t.Run("Presigned URL for private bucket works", func(t *testing.T) {
		// Generate presigned URL parameters
		accessKey := "private-tenant"
		bucket := "private-bucket"
		key := "secret.txt"
		expires := 3600

		// Create base URL
		baseURL := fmt.Sprintf("http://localhost:3333/%s/%s", bucket, key)
		u, err := url.Parse(baseURL)
		require.NoError(t, err)

		// Add presigned URL parameters
		now := time.Now().UTC()
		dateStr := now.Format("20060102T150405Z")
		shortDateStr := now.Format("20060102")
		credential := fmt.Sprintf("%s/%s/us-east-1/s3/aws4_request", accessKey, shortDateStr)

		q := u.Query()
		q.Set("X-Amz-Algorithm", "AWS4-HMAC-SHA256")
		q.Set("X-Amz-Credential", credential)
		q.Set("X-Amz-Date", dateStr)
		q.Set("X-Amz-Expires", fmt.Sprintf("%d", expires))
		q.Set("X-Amz-SignedHeaders", "host")
		q.Set("X-Amz-Signature", "test-signature") // Test handler doesn't validate signature
		u.RawQuery = q.Encode()

		// Make request with presigned URL
		req := httptest.NewRequest("GET", u.String(), nil)
		w := httptest.NewRecorder()

		server.router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
		assert.Equal(t, "private content", w.Body.String())
	})

	t.Run("Presigned URL for public bucket also works", func(t *testing.T) {
		// Even though public buckets don't need auth, presigned URLs should still work
		accessKey := "public-tenant"
		bucket := "public-bucket"
		key := "test.txt"
		expires := 3600

		// Create base URL
		baseURL := fmt.Sprintf("http://localhost:3333/%s/%s", bucket, key)
		u, err := url.Parse(baseURL)
		require.NoError(t, err)

		// Add presigned URL parameters
		now := time.Now().UTC()
		dateStr := now.Format("20060102T150405Z")
		shortDateStr := now.Format("20060102")
		credential := fmt.Sprintf("%s/%s/us-east-1/s3/aws4_request", accessKey, shortDateStr)

		q := u.Query()
		q.Set("X-Amz-Algorithm", "AWS4-HMAC-SHA256")
		q.Set("X-Amz-Credential", credential)
		q.Set("X-Amz-Date", dateStr)
		q.Set("X-Amz-Expires", fmt.Sprintf("%d", expires))
		q.Set("X-Amz-SignedHeaders", "host")
		q.Set("X-Amz-Signature", "test-signature")
		u.RawQuery = q.Encode()

		// Make request with presigned URL
		req := httptest.NewRequest("GET", u.String(), nil)
		w := httptest.NewRecorder()

		server.router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
		assert.Equal(t, "public content", w.Body.String())
	})
}

func TestAccessLogging(t *testing.T) {
	server, _, cleanup := setupTestServerWithPublicBuckets(t)
	defer cleanup()

	gin.SetMode(gin.TestMode)

	t.Run("Public access is logged correctly", func(t *testing.T) {
		// Capture log output using a custom writer
		var logOutput strings.Builder
		log.SetOutput(&logOutput)
		defer log.SetOutput(os.Stderr) // Reset log output

		req := httptest.NewRequest("GET", "/public-bucket/test.txt", nil)
		resp := httptest.NewRecorder()

		server.router.ServeHTTP(resp, req)

		// Check response
		assert.Equal(t, http.StatusOK, resp.Code)

		// Check log contains correct access type
		logs := logOutput.String()
		assert.Contains(t, logs, "Type: public")
		assert.Contains(t, logs, "Method: GET")
		assert.Contains(t, logs, "Bucket: public-bucket")
	})

	t.Run("Authenticated access is logged correctly", func(t *testing.T) {
		// Capture log output
		var logOutput strings.Builder
		log.SetOutput(&logOutput)
		defer log.SetOutput(os.Stderr)

		req := httptest.NewRequest("GET", "/private-bucket/secret.txt", nil)
		signRequestSimple(req, "private-tenant")
		resp := httptest.NewRecorder()

		server.router.ServeHTTP(resp, req)

		// Check response
		assert.Equal(t, http.StatusOK, resp.Code)

		// Check log contains correct access type
		logs := logOutput.String()
		assert.Contains(t, logs, "Type: sigv4")
		assert.Contains(t, logs, "Method: GET")
		assert.Contains(t, logs, "AccessKey: private-tenant")
	})

	t.Run("Presigned URL access is logged correctly", func(t *testing.T) {
		// Capture log output
		var logOutput strings.Builder
		log.SetOutput(&logOutput)
		defer log.SetOutput(os.Stderr)

		// Create presigned URL request
		baseURL := "http://localhost:3333/private-bucket/secret.txt"
		u, _ := url.Parse(baseURL)
		q := u.Query()
		q.Set("X-Amz-Algorithm", "AWS4-HMAC-SHA256")
		q.Set("X-Amz-Credential", fmt.Sprintf("private-tenant/%s/us-east-1/s3/aws4_request",
			time.Now().UTC().Format("20060102")))
		q.Set("X-Amz-Date", time.Now().UTC().Format("20060102T150405Z"))
		q.Set("X-Amz-Expires", "3600")
		q.Set("X-Amz-SignedHeaders", "host")
		q.Set("X-Amz-Signature", "test-signature")
		u.RawQuery = q.Encode()

		req := httptest.NewRequest("GET", u.String(), nil)
		resp := httptest.NewRecorder()

		server.router.ServeHTTP(resp, req)

		// Check response
		assert.Equal(t, http.StatusOK, resp.Code)

		// Check log contains correct access type
		logs := logOutput.String()
		assert.Contains(t, logs, "Type: presigned")
		assert.Contains(t, logs, "Method: GET")
		assert.Contains(t, logs, "AccessKey: private-tenant")
	})
}

func TestWildcardPublicBuckets(t *testing.T) {
	server, _, cleanup := setupTestServerWithPublicBuckets(t)
	defer cleanup()

	gin.SetMode(gin.TestMode)

	// Test that wildcard "public-*" matches "public-data"
	t.Run("Wildcard pattern matches bucket prefix", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/public-data/data.txt", nil)
		w := httptest.NewRecorder()

		server.router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
		assert.Equal(t, "prefixed public content", w.Body.String())
	})

	t.Run("Wildcard pattern does not match non-prefixed bucket", func(t *testing.T) {
		// This should fail because "private-bucket" doesn't match "public-*"
		req := httptest.NewRequest("GET", "/private-bucket/secret.txt", nil)
		w := httptest.NewRecorder()

		server.router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusForbidden, w.Code)
	})
}

// Helper function for simple request signing (for test auth handler)
func signRequestSimple(req *http.Request, accessKey string) {
	now := time.Now().UTC()
	dateStr := now.Format("20060102T150405Z")
	req.Header.Set("X-Amz-Date", dateStr)

	credential := fmt.Sprintf("%s/%s/us-east-1/s3/aws4_request", accessKey, now.Format("20060102"))
	authHeader := fmt.Sprintf("AWS4-HMAC-SHA256 Credential=%s, SignedHeaders=host;x-amz-date, Signature=test-signature",
		credential)
	req.Header.Set("Authorization", authHeader)
}
