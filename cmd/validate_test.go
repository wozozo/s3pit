package cmd

import (
	"os"
	"testing"
)

func TestValidateTenantsFile(t *testing.T) {
	tests := []struct {
		name        string
		content     string
		expectError bool
		errorMsg    string
	}{
		{
			name: "valid configuration",
			content: `{
				"globalDir": "~/s3pit",
				"tenants": [
					{
						"accessKeyId": "TEST_KEY",
						"secretAccessKey": "test_secret_key",
						"customDir": "/tmp/test",
						"description": "Test tenant"
					}
				]
			}`,
			expectError: false,
		},
		{
			name: "multiple valid tenants",
			content: `{
		"globalDir": "~/s3pit",
				"tenants": [
					{
						"accessKeyId": "TENANT1_KEY",
						"secretAccessKey": "tenant1_secret",
						"customDir": "/tmp/tenant1",
						"description": "Tenant 1"
					},
					{
						"accessKeyId": "TENANT2_KEY",
						"secretAccessKey": "tenant2_secret",
						"customDir": "/tmp/tenant2",
						"description": "Tenant 2"
					}
				]
			}`,
			expectError: false,
		},
		{
			name: "invalid JSON syntax",
			content: `{
				"tenants": [
					{
						"accessKeyId": "TEST_KEY",
						"secretAccessKey": "test_secret"
						"customDir": "/tmp/test"
					}
				]
			}`,
			expectError: true,
			errorMsg:    "invalid JSON format",
		},
		{
			name: "empty tenants array",
			content: `{
				"tenants": []
			}`,
			expectError: true,
			errorMsg:    "no tenants defined",
		},
		{
			name: "missing accessKeyId",
			content: `{
		"globalDir": "~/s3pit",
				"tenants": [
					{
						"secretAccessKey": "test_secret",
						"customDir": "/tmp/test"
					}
				]
			}`,
			expectError: true,
			errorMsg:    "accessKeyId is required",
		},
		{
			name: "empty accessKeyId",
			content: `{
		"globalDir": "~/s3pit",
				"tenants": [
					{
						"accessKeyId": "",
						"secretAccessKey": "test_secret",
						"customDir": "/tmp/test"
					}
				]
			}`,
			expectError: true,
			errorMsg:    "accessKeyId is required",
		},
		{
			name: "missing secretAccessKey",
			content: `{
		"globalDir": "~/s3pit",
				"tenants": [
					{
						"accessKeyId": "TEST_KEY",
						"customDir": "/tmp/test"
					}
				]
			}`,
			expectError: true,
			errorMsg:    "secretAccessKey is required",
		},
		{
			name: "empty secretAccessKey",
			content: `{
		"globalDir": "~/s3pit",
				"tenants": [
					{
						"accessKeyId": "TEST_KEY",
						"secretAccessKey": "",
						"customDir": "/tmp/test"
					}
				]
			}`,
			expectError: true,
			errorMsg:    "secretAccessKey is required",
		},
		{
			name: "missing global dataDir",
			content: `{
				"tenants": [
					{
						"accessKeyId": "TEST_KEY",
						"secretAccessKey": "test_secret"
					}
				]
			}`,
			expectError: true,
			errorMsg:    "global globalDir is required",
		},
		{
			name: "missing directory with global dataDir (valid)",
			content: `{
		"globalDir": "~/s3pit",
				"tenants": [
					{
						"accessKeyId": "TEST_KEY",
						"secretAccessKey": "test_secret"
					}
				]
			}`,
			expectError: false,
		},
		{
			name: "empty directory with global dataDir (valid)",
			content: `{
		"globalDir": "~/s3pit",
				"tenants": [
					{
						"accessKeyId": "TEST_KEY",
						"secretAccessKey": "test_secret",
						"customDir": ""
					}
				]
			}`,
			expectError: false,
		},
		{
			name: "invalid global dataDir path",
			content: `{
		"globalDir": "relative/path",
				"tenants": [
					{
						"accessKeyId": "TEST_KEY",
						"secretAccessKey": "test_secret"
					}
				]
			}`,
			expectError: true,
			errorMsg:    "global globalDir must be an absolute path",
		},
		{
			name: "invalid accessKeyId characters",
			content: `{
		"globalDir": "~/s3pit",
				"tenants": [
					{
						"accessKeyId": "INVALID@KEY",
						"secretAccessKey": "test_secret",
						"customDir": "/tmp/test"
					}
				]
			}`,
			expectError: true,
			errorMsg:    "accessKeyId contains invalid characters",
		},
		{
			name: "valid with tilde directory",
			content: `{
		"globalDir": "~/s3pit",
				"tenants": [
					{
						"accessKeyId": "TEST_KEY",
						"secretAccessKey": "test_secret",
						"customDir": "~/test_data"
					}
				]
			}`,
			expectError: false,
		},
		{
			name: "invalid relative directory path",
			content: `{
		"globalDir": "~/s3pit",
				"tenants": [
					{
						"accessKeyId": "TEST_KEY",
						"secretAccessKey": "test_secret",
						"customDir": "relative/path"
					}
				]
			}`,
			expectError: true,
			errorMsg:    "customDir must be an absolute path",
		},
		{
			name: "invalid relative directory (single dir)",
			content: `{
		"globalDir": "~/s3pit",
				"tenants": [
					{
						"accessKeyId": "TEST_KEY",
						"secretAccessKey": "test_secret",
						"customDir": "data"
					}
				]
			}`,
			expectError: true,
			errorMsg:    "customDir must be an absolute path",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create temporary file
			tmpFile, err := os.CreateTemp("", "tenants_test_*.json")
			if err != nil {
				t.Fatalf("Failed to create temp file: %v", err)
			}
			defer os.Remove(tmpFile.Name())

			// Write test content
			if _, err := tmpFile.WriteString(tt.content); err != nil {
				t.Fatalf("Failed to write temp file: %v", err)
			}
			tmpFile.Close()

			// Validate the file
			err = validateTenantsFile(tmpFile.Name())

			if tt.expectError {
				if err == nil {
					t.Errorf("Expected error but got none")
				} else if tt.errorMsg != "" && !containsString(err.Error(), tt.errorMsg) {
					t.Errorf("Expected error message to contain '%s', got: %v", tt.errorMsg, err)
				}
			} else {
				if err != nil {
					t.Errorf("Expected no error but got: %v", err)
				}
			}
		})
	}
}

func TestValidateTenantsFileNotFound(t *testing.T) {
	err := validateTenantsFile("/nonexistent/path/tenants.json")
	if err == nil {
		t.Error("Expected error for non-existent file but got none")
	}
	if !containsString(err.Error(), "failed to read file") {
		t.Errorf("Expected 'failed to read file' error, got: %v", err)
	}
}

func TestIsValidAccessKey(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected bool
	}{
		{
			name:     "valid alphanumeric",
			input:    "TEST123KEY",
			expected: true,
		},
		{
			name:     "valid with underscores",
			input:    "TEST_KEY_123",
			expected: true,
		},
		{
			name:     "valid with hyphens",
			input:    "TEST-KEY-123",
			expected: true,
		},
		{
			name:     "invalid with at symbol",
			input:    "TEST@KEY",
			expected: false,
		},
		{
			name:     "invalid with spaces",
			input:    "TEST KEY",
			expected: false,
		},
		{
			name:     "invalid with special characters",
			input:    "TEST!KEY#",
			expected: false,
		},
		{
			name:     "empty string",
			input:    "",
			expected: true, // Empty string has no invalid characters
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isValidAccessKey(tt.input)
			if result != tt.expected {
				t.Errorf("isValidAccessKey(%s) = %v, expected %v", tt.input, result, tt.expected)
			}
		})
	}
}

func TestIsValidDirectoryPath(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected bool
	}{
		{
			name:     "valid absolute path",
			input:    "/home/user/data",
			expected: true,
		},
		{
			name:     "valid home directory path",
			input:    "~/data",
			expected: true,
		},
		{
			name:     "valid home directory nested path",
			input:    "~/documents/s3pit",
			expected: true,
		},
		{
			name:     "invalid relative path",
			input:    "relative/path",
			expected: false,
		},
		{
			name:     "invalid single directory",
			input:    "data",
			expected: false,
		},
		{
			name:     "invalid path starting with tilde but no slash",
			input:    "~data",
			expected: false,
		},
		{
			name:     "empty path",
			input:    "",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isValidDirectoryPath(tt.input)
			if result != tt.expected {
				t.Errorf("isValidDirectoryPath(%s) = %v, expected %v", tt.input, result, tt.expected)
			}
		})
	}
}

// Helper function to check if a string contains a substring
func containsString(s, substr string) bool {
	return len(substr) == 0 || (len(s) >= len(substr) &&
		func() bool {
			for i := 0; i <= len(s)-len(substr); i++ {
				if s[i:i+len(substr)] == substr {
					return true
				}
			}
			return false
		}())
}
