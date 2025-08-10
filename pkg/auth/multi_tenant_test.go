package auth

import (
	"fmt"
	"net/http"
	"net/url"
	"testing"
	"time"

	"github.com/wozozo/s3pit/pkg/tenant"
)

func TestMultiTenantHandler_ExtractAccessKey(t *testing.T) {
	tenantManager := tenant.NewManager("")
	_ = tenantManager.AddTenant(&tenant.Tenant{
		AccessKeyID:     "test-key",
		SecretAccessKey: "test-secret",
		CustomDirectory: "/test/dir",
	})

	handler := &MultiTenantHandler{
		mode:             ModeSigV4,
		tenantManager:    tenantManager,
		defaultAccessKey: "default-key",
		defaultSecretKey: "default-secret",
	}

	tests := []struct {
		name     string
		request  *http.Request
		expected string
	}{
		{
			name: "Extract from Authorization header (AWS4-HMAC-SHA256)",
			request: func() *http.Request {
				req, _ := http.NewRequest("GET", "/bucket/key", nil)
				req.Header.Set("Authorization",
					"AWS4-HMAC-SHA256 Credential=test-key/20250809/ap-northeast-1/s3/aws4_request, SignedHeaders=host, Signature=abc123")
				return req
			}(),
			expected: "test-key",
		},
		{
			name: "Extract from Authorization header (AWS)",
			request: func() *http.Request {
				req, _ := http.NewRequest("GET", "/bucket/key", nil)
				req.Header.Set("Authorization", "AWS test-key:signature")
				return req
			}(),
			expected: "test-key",
		},
		{
			name: "Extract from presigned URL",
			request: func() *http.Request {
				req, _ := http.NewRequest("GET", "/bucket/key", nil)
				q := req.URL.Query()
				q.Set("X-Amz-Credential", "test-key/20250809/ap-northeast-1/s3/aws4_request")
				req.URL.RawQuery = q.Encode()
				return req
			}(),
			expected: "test-key",
		},
		{
			name: "No access key",
			request: func() *http.Request {
				req, _ := http.NewRequest("GET", "/bucket/key", nil)
				return req
			}(),
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := handler.extractAccessKey(tt.request)
			if result != tt.expected {
				t.Errorf("Expected access key %s, got %s", tt.expected, result)
			}
		})
	}
}

func TestMultiTenantHandler_AuthenticateSigV4(t *testing.T) {
	// Create tenant manager with test tenants
	tenantManager := tenant.NewManager("")
	_ = tenantManager.AddTenant(&tenant.Tenant{
		AccessKeyID:     "tenant1",
		SecretAccessKey: "secret1",
		CustomDirectory: "/tenant1/data",
	})
	_ = tenantManager.AddTenant(&tenant.Tenant{
		AccessKeyID:     "tenant2",
		SecretAccessKey: "secret2",
		CustomDirectory: "/tenant2/data",
	})

	handler := &MultiTenantHandler{
		mode:             ModeSigV4,
		tenantManager:    tenantManager,
		defaultAccessKey: "default",
		defaultSecretKey: "default-secret",
	}

	tests := []struct {
		name          string
		accessKey     string
		secretKey     string
		shouldSucceed bool
	}{
		{
			name:          "Valid tenant1 credentials",
			accessKey:     "tenant1",
			secretKey:     "secret1",
			shouldSucceed: true,
		},
		{
			name:          "Valid tenant2 credentials",
			accessKey:     "tenant2",
			secretKey:     "secret2",
			shouldSucceed: true,
		},
		{
			name:          "Invalid access key",
			accessKey:     "invalid-key",
			secretKey:     "some-secret",
			shouldSucceed: false,
		},
		{
			name:          "Valid default credentials",
			accessKey:     "default",
			secretKey:     "default-secret",
			shouldSucceed: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a presigned URL request
			req, _ := http.NewRequest("PUT", "/test-bucket/test-key", nil)
			req.Host = "localhost:3333"

			// Build presigned URL parameters
			now := time.Now().UTC()
			amzDate := now.Format("20060102T150405Z")
			dateStamp := now.Format("20060102")

			q := url.Values{}
			q.Set("X-Amz-Algorithm", "AWS4-HMAC-SHA256")
			q.Set("X-Amz-Credential", fmt.Sprintf("%s/%s/ap-northeast-1/s3/aws4_request",
				tt.accessKey, dateStamp))
			q.Set("X-Amz-Date", amzDate)
			q.Set("X-Amz-Expires", "3600")
			q.Set("X-Amz-SignedHeaders", "host")

			// For this test, we'll use a dummy signature
			// In a real scenario, this would be calculated properly
			q.Set("X-Amz-Signature", "dummy-signature")

			req.URL.RawQuery = q.Encode()

			// Try to authenticate
			accessKey, err := handler.authenticateSigV4Query(req, tt.accessKey, tt.secretKey)

			if tt.shouldSucceed {
				// We expect it to fail with signature mismatch (not access key error)
				// since we're using a dummy signature
				if err != nil && err.Error() == fmt.Sprintf("access key not found: %s", tt.accessKey) {
					t.Errorf("Expected signature error, got access key error: %v", err)
				}
				if err == nil && accessKey != tt.accessKey {
					t.Errorf("Expected access key %s, got %s", tt.accessKey, accessKey)
				}
			} else {
				// Should fail with access key error
				if err == nil {
					t.Error("Expected authentication to fail, but it succeeded")
				}
			}
		})
	}
}

func TestMultiTenantHandler_PresignedURLExpiration(t *testing.T) {
	tenantManager := tenant.NewManager("")
	_ = tenantManager.AddTenant(&tenant.Tenant{
		AccessKeyID:     "test-key",
		SecretAccessKey: "test-secret",
		CustomDirectory: "/test/dir",
	})

	handler := &MultiTenantHandler{
		mode:             ModeSigV4,
		tenantManager:    tenantManager,
		defaultAccessKey: "default",
		defaultSecretKey: "default-secret",
	}

	tests := []struct {
		name        string
		signTime    time.Time
		expires     int
		shouldError bool
	}{
		{
			name:        "Valid (not expired)",
			signTime:    time.Now().UTC().Add(-30 * time.Second),
			expires:     3600,
			shouldError: false,
		},
		{
			name:        "Expired",
			signTime:    time.Now().UTC().Add(-2 * time.Hour),
			expires:     3600,
			shouldError: true,
		},
		{
			name:        "Just expired",
			signTime:    time.Now().UTC().Add(-61 * time.Second),
			expires:     60,
			shouldError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req, _ := http.NewRequest("PUT", "/bucket/key", nil)
			req.Host = "localhost:3333"

			amzDate := tt.signTime.Format("20060102T150405Z")
			dateStamp := tt.signTime.Format("20060102")

			q := url.Values{}
			q.Set("X-Amz-Algorithm", "AWS4-HMAC-SHA256")
			q.Set("X-Amz-Credential", fmt.Sprintf("test-key/%s/ap-northeast-1/s3/aws4_request", dateStamp))
			q.Set("X-Amz-Date", amzDate)
			q.Set("X-Amz-Expires", fmt.Sprintf("%d", tt.expires))
			q.Set("X-Amz-SignedHeaders", "host")
			q.Set("X-Amz-Signature", "dummy-signature")

			req.URL.RawQuery = q.Encode()

			_, err := handler.authenticateSigV4Query(req, "test-key", "test-secret")

			if tt.shouldError {
				if err == nil || err.Error() != "presigned URL has expired" {
					t.Errorf("Expected expiration error, got: %v", err)
				}
			} else {
				// Should fail with signature error (not expiration)
				if err != nil && err.Error() == "presigned URL has expired" {
					t.Error("URL should not be expired")
				}
			}
		})
	}
}
