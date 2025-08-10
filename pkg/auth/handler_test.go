package auth

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestSigV4AuthHandler(t *testing.T) {
	accessKey := "AKIAIOSFODNN7EXAMPLE"
	secretKey := "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY"
	handler, err := NewHandler("sigv4", accessKey, secretKey)
	if err != nil {
		t.Fatalf("Failed to create sigv4 auth handler: %v", err)
	}

	t.Run("MissingAuthHeader", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/test", nil)
		_, err := handler.Authenticate(req)
		if err == nil {
			t.Error("Expected error for missing auth header")
		}
	})

	t.Run("InvalidAuthHeader", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/test", nil)
		req.Header.Set("Authorization", "Bearer invalid")
		_, err := handler.Authenticate(req)
		if err == nil {
			t.Error("Expected error for invalid auth header")
		}
	})
}

func TestExtractAccessKey(t *testing.T) {
	tests := []struct {
		name          string
		authorization string
		expected      string
		expectError   bool
	}{
		{
			name:          "ValidAWSv4Header",
			authorization: "AWS4-HMAC-SHA256 Credential=AKIAIOSFODNN7EXAMPLE/20230101/us-east-1/s3/aws4_request",
			expected:      "AKIAIOSFODNN7EXAMPLE",
			expectError:   false,
		},
		{
			name:          "InvalidFormat",
			authorization: "Bearer token123",
			expected:      "",
			expectError:   true,
		},
		{
			name:          "EmptyHeader",
			authorization: "",
			expected:      "",
			expectError:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/", nil)
			if tt.authorization != "" {
				req.Header.Set("Authorization", tt.authorization)
			}

			key := extractAccessKeyFromRequest(req)

			if tt.expectError && key != "" {
				t.Errorf("Expected error but got key: %s", key)
			}

			if !tt.expectError && key != tt.expected {
				t.Errorf("Expected key %s, got %s", tt.expected, key)
			}
		})
	}
}

func extractAccessKeyFromRequest(r *http.Request) string {
	auth := r.Header.Get("Authorization")
	if auth == "" {
		return ""
	}

	// Simple extraction for AWS4-HMAC-SHA256
	const prefix = "AWS4-HMAC-SHA256 Credential="
	if len(auth) > len(prefix) && auth[:len(prefix)] == prefix {
		rest := auth[len(prefix):]
		// Find the first '/'
		for i, ch := range rest {
			if ch == '/' {
				return rest[:i]
			}
		}
	}

	return ""
}

func TestNewHandler(t *testing.T) {
	tests := []struct {
		name        string
		mode        string
		expectError bool
	}{
		{
			name:        "ValidSigV4Mode",
			mode:        "sigv4",
			expectError: false,
		},
		{
			name:        "InvalidMode",
			mode:        "invalid",
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewHandler(tt.mode, "key", "secret")

			if tt.expectError && err == nil {
				t.Error("Expected error but got nil")
			}

			if !tt.expectError && err != nil {
				t.Errorf("Expected no error but got: %v", err)
			}
		})
	}
}
