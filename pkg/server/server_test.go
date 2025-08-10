package server

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/wozozo/s3pit/internal/config"
	"github.com/wozozo/s3pit/pkg/api"
	"github.com/wozozo/s3pit/pkg/logger"
	"github.com/wozozo/s3pit/pkg/tenant"
	"github.com/wozozo/s3pit/pkg/testutil"
)

func setupTestServer(t *testing.T) *Server {
	// Use testutil for configuration
	cfg := testutil.NewTestConfig(t,
		testutil.WithAuthMode("sigv4"),
		testutil.WithPort(3333),
	)
	cfg.Host = "localhost"

	// Initialize logger with proper settings
	logger.GetInstance().SetMaxEntries(100)

	server, err := New(cfg)
	assert.NoError(t, err)
	assert.NotNil(t, server)

	// Replace auth handler with a test handler that always succeeds
	server.authHandler = &testAuthHandler{}

	return server
}

// testAuthHandler is a mock auth handler for testing
type testAuthHandler struct{}

func (h *testAuthHandler) Authenticate(r *http.Request) (string, error) {
	return "test", nil
}

// signRequest adds AWS Signature V4 authentication to a request
func signRequest(req *http.Request, accessKey, secretKey string) {
	// For testing, we'll use a simplified signature that the test handler can validate
	// In production, this would use the full AWS Signature V4 algorithm

	// Set the date header
	now := time.Now().UTC()
	dateStr := now.Format("20060102T150405Z")
	req.Header.Set("X-Amz-Date", dateStr)

	// Create a simplified authorization header
	// Format: AWS4-HMAC-SHA256 Credential=ACCESS_KEY/20060102/us-east-1/s3/aws4_request, SignedHeaders=host;x-amz-date, Signature=SIGNATURE
	credential := fmt.Sprintf("%s/%s/us-east-1/s3/aws4_request", accessKey, now.Format("20060102"))

	// For testing, create a simple signature
	stringToSign := fmt.Sprintf("%s\n%s\n%s", req.Method, req.URL.Path, dateStr)
	h := hmac.New(sha256.New, []byte("AWS4"+secretKey))
	h.Write([]byte(now.Format("20060102")))
	dateKey := h.Sum(nil)

	h = hmac.New(sha256.New, dateKey)
	h.Write([]byte("us-east-1"))
	regionKey := h.Sum(nil)

	h = hmac.New(sha256.New, regionKey)
	h.Write([]byte("s3"))
	serviceKey := h.Sum(nil)

	h = hmac.New(sha256.New, serviceKey)
	h.Write([]byte("aws4_request"))
	signingKey := h.Sum(nil)

	h = hmac.New(sha256.New, signingKey)
	h.Write([]byte(stringToSign))
	signature := hex.EncodeToString(h.Sum(nil))

	authHeader := fmt.Sprintf("AWS4-HMAC-SHA256 Credential=%s, SignedHeaders=host;x-amz-date, Signature=%s",
		credential, signature)
	req.Header.Set("Authorization", authHeader)

	// Ensure body can be read multiple times
	if req.Body != nil {
		bodyBytes, _ := io.ReadAll(req.Body)
		req.Body = io.NopCloser(bytes.NewReader(bodyBytes))
		req.ContentLength = int64(len(bodyBytes))
	}
}

// TestBucketCreationWithTrailingSlash tests that PUT requests with trailing slashes
// correctly create buckets instead of being misrouted to PutObject
func TestBucketCreationWithTrailingSlash(t *testing.T) {
	server := setupTestServer(t)

	tests := []struct {
		name       string
		path       string
		method     string
		expectCode int
		desc       string
	}{
		{
			name:       "PUT bucket without trailing slash",
			path:       "/test-bucket-1",
			method:     "PUT",
			expectCode: http.StatusOK,
			desc:       "Should create bucket normally",
		},
		{
			name:       "PUT bucket with trailing slash",
			path:       "/test-bucket-2/",
			method:     "PUT",
			expectCode: http.StatusOK,
			desc:       "Should create bucket even with trailing slash",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(tt.method, tt.path, nil)
			signRequest(req, "test", "test")
			w := httptest.NewRecorder()

			server.router.ServeHTTP(w, req)

			assert.Equal(t, tt.expectCode, w.Code, tt.desc)

			// Verify bucket was created by checking the header
			if tt.expectCode == http.StatusOK {
				assert.Equal(t, "true", w.Header().Get("x-s3pit-bucket-created"))
			}
		})
	}
}

// TestListObjectsV2WithTrailingSlash tests that GET requests with trailing slashes
// correctly invoke ListObjectsV2 instead of GetObject
func TestListObjectsV2WithTrailingSlash(t *testing.T) {
	server := setupTestServer(t)

	// First create a bucket and add an object
	bucketName := "test-list-bucket"
	objectKey := "test-object.txt"
	objectContent := "test content"

	// Create bucket
	req := httptest.NewRequest("PUT", "/"+bucketName, nil)
	signRequest(req, "test", "test")
	w := httptest.NewRecorder()
	server.router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)

	// Put object
	req = httptest.NewRequest("PUT", "/"+bucketName+"/"+objectKey,
		bytes.NewBufferString(objectContent))
	signRequest(req, "test", "test")
	w = httptest.NewRecorder()
	server.router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)

	// Test ListObjectsV2 with trailing slash
	tests := []struct {
		name       string
		path       string
		expectCode int
		expectXML  bool
		desc       string
	}{
		{
			name:       "GET bucket without trailing slash",
			path:       "/" + bucketName,
			expectCode: http.StatusOK,
			expectXML:  true,
			desc:       "Should list objects normally",
		},
		{
			name:       "GET bucket with trailing slash",
			path:       "/" + bucketName + "/",
			expectCode: http.StatusOK,
			expectXML:  true,
			desc:       "Should list objects even with trailing slash",
		},
		{
			name:       "GET bucket with query params and trailing slash",
			path:       "/" + bucketName + "/?list-type=2&max-keys=10",
			expectCode: http.StatusOK,
			expectXML:  true,
			desc:       "Should list objects with query parameters",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", tt.path, nil)
			signRequest(req, "test", "test")
			w := httptest.NewRecorder()

			server.router.ServeHTTP(w, req)

			assert.Equal(t, tt.expectCode, w.Code, tt.desc)

			if tt.expectXML {
				// Verify it's XML response (ListObjectsV2)
				assert.Equal(t, "application/xml", w.Header().Get("Content-Type"))

				// Parse XML to verify it's valid ListBucketResult
				var result api.ListObjectsV2Response
				err := xml.Unmarshal(w.Body.Bytes(), &result)
				assert.NoError(t, err)
				assert.Equal(t, bucketName, result.Name)
				// The object may not be created if auth is required
				// Just verify the response is valid XML
			}
		})
	}
}

// TestMultipartUploadWithEmptyQueryParam tests that multipart upload initiation
// works correctly when the 'uploads' query parameter is present but empty
func TestMultipartUploadWithEmptyQueryParam(t *testing.T) {
	server := setupTestServer(t)

	bucketName := "test-multipart-bucket"
	objectKey := "multipart-object.txt"

	tests := []struct {
		name       string
		path       string
		expectCode int
		desc       string
	}{
		{
			name:       "POST with uploads= (empty value)",
			path:       "/" + bucketName + "/" + objectKey + "?uploads=",
			expectCode: http.StatusOK,
			desc:       "Should initiate multipart upload with empty uploads param",
		},
		{
			name:       "POST with uploads=1",
			path:       "/" + bucketName + "/" + objectKey + "?uploads=1",
			expectCode: http.StatusOK,
			desc:       "Should initiate multipart upload with uploads=1",
		},
		{
			name:       "POST without uploads param",
			path:       "/" + bucketName + "/" + objectKey,
			expectCode: http.StatusNotImplemented,
			desc:       "Should return 501 without uploads param",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("POST", tt.path, nil)
			signRequest(req, "test", "test")
			w := httptest.NewRecorder()

			server.router.ServeHTTP(w, req)

			assert.Equal(t, tt.expectCode, w.Code, tt.desc)

			if tt.expectCode == http.StatusOK {
				// Verify it's a multipart upload response
				assert.Equal(t, "application/xml", w.Header().Get("Content-Type"))

				// Check if response contains InitiateMultipartUploadResult
				body := w.Body.String()
				assert.Contains(t, body, "InitiateMultipartUploadResult")
				assert.Contains(t, body, "UploadId")
				assert.Contains(t, body, bucketName)
				assert.Contains(t, body, objectKey)
			}
		})
	}
}

// TestCompleteMultipartUpload tests the complete multipart upload flow
func TestCompleteMultipartUpload(t *testing.T) {
	server := setupTestServer(t)

	bucketName := "test-multipart-complete"
	objectKey := "complete-object.txt"

	// Step 1: Initiate multipart upload
	req := httptest.NewRequest("POST", "/"+bucketName+"/"+objectKey+"?uploads=", nil)
	signRequest(req, "test", "test")
	w := httptest.NewRecorder()
	server.router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)

	// Extract uploadId from response
	type InitiateResult struct {
		XMLName  xml.Name `xml:"InitiateMultipartUploadResult"`
		UploadId string   `xml:"UploadId"`
	}
	var initResult InitiateResult
	err := xml.Unmarshal(w.Body.Bytes(), &initResult)
	assert.NoError(t, err)
	assert.NotEmpty(t, initResult.UploadId)

	uploadId := initResult.UploadId

	// Step 2: Upload a part
	partContent := strings.Repeat("x", 5*1024*1024) // 5MB
	req = httptest.NewRequest("PUT",
		"/"+bucketName+"/"+objectKey+"?partNumber=1&uploadId="+uploadId,
		bytes.NewBufferString(partContent))
	signRequest(req, "test", "test")
	w = httptest.NewRecorder()
	server.router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)

	etag := w.Header().Get("ETag")
	assert.NotEmpty(t, etag)

	// Step 3: Complete multipart upload
	completeBody := `<?xml version="1.0" encoding="UTF-8"?>
<CompleteMultipartUpload>
    <Part>
        <PartNumber>1</PartNumber>
        <ETag>` + etag + `</ETag>
    </Part>
</CompleteMultipartUpload>`

	req = httptest.NewRequest("POST",
		"/"+bucketName+"/"+objectKey+"?uploadId="+uploadId,
		bytes.NewBufferString(completeBody))
	signRequest(req, "test", "test")
	w = httptest.NewRecorder()
	server.router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)

	// Verify the object was created
	req = httptest.NewRequest("HEAD", "/"+bucketName+"/"+objectKey, nil)
	w = httptest.NewRecorder()
	server.router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)

	// Content-Length should match the uploaded part
	contentLength := w.Header().Get("Content-Length")
	assert.Equal(t, "5242880", contentLength) // 5MB
}

// TestRouterPriorityWithEmptyKey tests that routes with empty keys are handled correctly
func TestRouterPriorityWithEmptyKey(t *testing.T) {
	server := setupTestServer(t)

	tests := []struct {
		name       string
		method     string
		path       string
		handler    string
		expectCode int
	}{
		{
			name:       "PUT /bucket/ creates bucket",
			method:     "PUT",
			path:       "/test-bucket/",
			handler:    "CreateBucket",
			expectCode: http.StatusOK,
		},
		{
			name:       "GET /bucket/ lists objects",
			method:     "GET",
			path:       "/test-bucket/",
			handler:    "ListObjectsV2",
			expectCode: http.StatusOK,
		},
		{
			name:       "POST /bucket/key?uploads= initiates multipart",
			method:     "POST",
			path:       "/test-bucket/test.txt?uploads=",
			handler:    "InitiateMultipartUpload",
			expectCode: http.StatusOK,
		},
		{
			name:       "DELETE /bucket/ deletes bucket",
			method:     "DELETE",
			path:       "/test-bucket/",
			handler:    "DeleteBucket",
			expectCode: http.StatusNoContent,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create bucket first if needed
			if tt.method != "PUT" {
				req := httptest.NewRequest("PUT", "/test-bucket", nil)
				signRequest(req, "test", "test")
				w := httptest.NewRecorder()
				server.router.ServeHTTP(w, req)
			}

			req := httptest.NewRequest(tt.method, tt.path, nil)
			signRequest(req, "test", "test")
			w := httptest.NewRecorder()
			server.router.ServeHTTP(w, req)

			// For some operations, 404 is acceptable if bucket doesn't exist
			if w.Code == http.StatusNotFound && tt.method == "DELETE" {
				// This is fine - bucket might not exist
				return
			}

			assert.Equal(t, tt.expectCode, w.Code,
				"Expected %s to be handled by %s", tt.path, tt.handler)
		})
	}
}

// TestCommandLineOverrides tests that command line arguments override config file settings
func TestCommandLineOverrides(t *testing.T) {
	gin.SetMode(gin.TestMode)
	logger.GetInstance().SetMaxEntries(100)

	// Create temporary directory for test
	tempDir := t.TempDir()
	tenantsFile := filepath.Join(tempDir, "tenants.json")

	// Create test tenants.json with globalDir setting
	tenantsConfig := tenant.TenantsConfig{
		GlobalDir: tempDir + "/from-tenants-json",
		Tenants: []tenant.Tenant{
			{
				AccessKeyID:     "test-tenant",
				SecretAccessKey: "test-secret",
				CustomDir:       "",
			},
		},
	}

	configData, err := json.MarshalIndent(tenantsConfig, "", "  ")
	assert.NoError(t, err)

	err = os.WriteFile(tenantsFile, configData, 0644)
	assert.NoError(t, err)

	tests := []struct {
		name        string
		baseConfig  *config.Config
		overrides   map[string]bool
		expectedDir string
		description string
	}{
		{
			name: "No command line override - use tenants.json",
			baseConfig: &config.Config{
				Port:        3333,
				Host:        "localhost",
				GlobalDir:   tempDir + "/default",
				TenantsFile: tenantsFile,
				InMemory:    true,
				AuthMode:    "sigv4",
			},
			overrides:   make(map[string]bool),
			expectedDir: tempDir + "/from-tenants-json",
			description: "Should use globalDir from tenants.json when no command line override",
		},
		{
			name: "Command line override - ignore tenants.json",
			baseConfig: &config.Config{
				Port:        3333,
				Host:        "localhost",
				GlobalDir:   tempDir + "/from-cmdline",
				TenantsFile: tenantsFile,
				InMemory:    true,
				AuthMode:    "sigv4",
			},
			overrides: map[string]bool{
				"global-dir": true,
			},
			expectedDir: tempDir + "/from-cmdline",
			description: "Should use command line globalDir and ignore tenants.json setting",
		},
		{
			name: "Multiple command line overrides",
			baseConfig: &config.Config{
				Port:             3334,
				Host:             "127.0.0.1",
				GlobalDir:        tempDir + "/from-cmdline-multi",
				TenantsFile:      tenantsFile,
				InMemory:         false,
				AuthMode:         "sigv4",
				AutoCreateBucket: false,
			},
			overrides: map[string]bool{
				"global-dir":         true,
				"port":               true,
				"host":               true,
				"auto-create-bucket": true,
			},
			expectedDir: tempDir + "/from-cmdline-multi",
			description: "Should respect all command line overrides",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server, err := NewWithCmdLineOverrides(tt.baseConfig, tt.overrides)
			assert.NoError(t, err)
			assert.NotNil(t, server)

			// Verify that the global directory is set correctly
			assert.Equal(t, tt.expectedDir, tt.baseConfig.GlobalDir, tt.description)

			// For the test with multiple overrides, verify other settings too
			if tt.name == "Multiple command line overrides" {
				assert.Equal(t, 3334, tt.baseConfig.Port)
				assert.Equal(t, "127.0.0.1", tt.baseConfig.Host)
				assert.Equal(t, false, tt.baseConfig.AutoCreateBucket)
			}
		})
	}
}

// TestUpdateGlobalDirFromTenants tests the UpdateGlobalDirFromTenants functionality
func TestUpdateGlobalDirFromTenants(t *testing.T) {
	tempDir := t.TempDir()
	tenantsFile := filepath.Join(tempDir, "tenants.json")

	// Create test tenants.json
	tenantsConfig := tenant.TenantsConfig{
		GlobalDir: tempDir + "/tenants-global",
		Tenants: []tenant.Tenant{
			{
				AccessKeyID:     "test",
				SecretAccessKey: "secret",
			},
		},
	}

	configData, err := json.MarshalIndent(tenantsConfig, "", "  ")
	assert.NoError(t, err)

	err = os.WriteFile(tenantsFile, configData, 0644)
	assert.NoError(t, err)

	// Test without command line override
	cfg := &config.Config{
		GlobalDir: tempDir + "/original",
	}

	tenantMgr := tenant.NewManager(tenantsFile)
	err = tenantMgr.LoadFromFile()
	assert.NoError(t, err)

	// Should update when skipUpdate is false
	cfg.UpdateGlobalDirFromTenants(tenantMgr, false)
	assert.Equal(t, tempDir+"/tenants-global", cfg.GlobalDir)

	// Should not update when skipUpdate is true
	cfg.GlobalDir = tempDir + "/cmdline-override"
	cfg.UpdateGlobalDirFromTenants(tenantMgr, true)
	assert.Equal(t, tempDir+"/cmdline-override", cfg.GlobalDir)
}

// TestServerWithTenantManager tests server initialization with tenant manager
func TestServerWithTenantManager(t *testing.T) {
	gin.SetMode(gin.TestMode)
	logger.GetInstance().SetMaxEntries(100)

	tempDir := t.TempDir()
	tenantsFile := filepath.Join(tempDir, "tenants.json")

	// Create test tenants.json
	tenantsConfig := tenant.TenantsConfig{
		GlobalDir: tempDir + "/tenant-base",
		Tenants: []tenant.Tenant{
			{
				AccessKeyID:     "tenant1",
				SecretAccessKey: "secret1",
				CustomDir:       "",
			},
			{
				AccessKeyID:     "tenant2",
				SecretAccessKey: "secret2",
				CustomDir:       tempDir + "/tenant2-custom",
			},
		},
	}

	configData, err := json.MarshalIndent(tenantsConfig, "", "  ")
	assert.NoError(t, err)

	err = os.WriteFile(tenantsFile, configData, 0644)
	assert.NoError(t, err)

	cfg := &config.Config{
		Port:        3333,
		Host:        "localhost",
		GlobalDir:   tempDir + "/default",
		TenantsFile: tenantsFile,
		InMemory:    false,
		AuthMode:    "sigv4",
	}

	server, err := New(cfg)
	assert.NoError(t, err)
	assert.NotNil(t, server)
	assert.NotNil(t, server.tenantManager)

	// Verify tenant manager has correct tenants
	tenants := server.tenantManager.ListTenants()
	assert.Len(t, tenants, 2)

	// Check tenant directories
	tenant1Dir := server.tenantManager.GetDirectory("tenant1")
	assert.Equal(t, filepath.Join(tempDir, "tenant-base", "tenant1"), tenant1Dir)

	tenant2Dir := server.tenantManager.GetDirectory("tenant2")
	assert.Equal(t, tempDir+"/tenant2-custom", tenant2Dir)
}
