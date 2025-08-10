package cmd

import (
	"os"
	"testing"
)

func TestValidateConfigFile(t *testing.T) {
	tests := []struct {
		name        string
		content     string
		expectError bool
		errorMsg    string
	}{
		{
			name: "valid configuration",
			content: `globalDir = "~/s3pit"

[[tenants]]
accessKeyId = "TEST_KEY"
secretAccessKey = "test_secret_key"
customDir = "/tmp/test"
description = "Test tenant"
`,
			expectError: false,
		},
		{
			name: "multiple valid tenants",
			content: `globalDir = "~/s3pit"

[[tenants]]
accessKeyId = "TENANT1_KEY"
secretAccessKey = "tenant1_secret"
customDir = "/tmp/tenant1"
description = "Tenant 1"

[[tenants]]
accessKeyId = "TENANT2_KEY"
secretAccessKey = "tenant2_secret"
customDir = "/tmp/tenant2"
description = "Tenant 2"
`,
			expectError: false,
		},
		{
			name: "invalid TOML syntax",
			content: `globalDir = "~/s3pit"

[[tenants]]
accessKeyId = "TEST_KEY"
secretAccessKey = "test_secret"
customDir = /tmp/test  # Missing quotes
`,
			expectError: true,
			errorMsg:    "invalid TOML format",
		},
		{
			name: "empty tenants array",
			content: `globalDir = "~/s3pit"
tenants = []
`,
			expectError: true,
			errorMsg:    "no tenants defined",
		},
		{
			name: "missing accessKeyId",
			content: `globalDir = "~/s3pit"

[[tenants]]
secretAccessKey = "test_secret"
customDir = "/tmp/test"
`,
			expectError: true,
			errorMsg:    "accessKeyId is required",
		},
		{
			name: "empty accessKeyId",
			content: `globalDir = "~/s3pit"

[[tenants]]
accessKeyId = ""
secretAccessKey = "test_secret"
customDir = "/tmp/test"
`,
			expectError: true,
			errorMsg:    "accessKeyId is required",
		},
		{
			name: "missing secretAccessKey",
			content: `globalDir = "~/s3pit"

[[tenants]]
accessKeyId = "TEST_KEY"
customDir = "/tmp/test"
`,
			expectError: true,
			errorMsg:    "secretAccessKey is required",
		},
		{
			name: "empty secretAccessKey",
			content: `globalDir = "~/s3pit"

[[tenants]]
accessKeyId = "TEST_KEY"
secretAccessKey = ""
customDir = "/tmp/test"
`,
			expectError: true,
			errorMsg:    "secretAccessKey is required",
		},
		{
			name: "invalid customDir - relative path",
			content: `globalDir = "~/s3pit"

[[tenants]]
accessKeyId = "TEST_KEY"
secretAccessKey = "test_secret"
customDir = "relative/path"
`,
			expectError: true,
			errorMsg:    "customDir must be an absolute path",
		},
		{
			name: "invalid customDir - not a path",
			content: `globalDir = "~/s3pit"

[[tenants]]
accessKeyId = "TEST_KEY"
secretAccessKey = "test_secret"
customDir = "../relative/path"
`,
			expectError: true,
			errorMsg:    "customDir must be an absolute path",
		},
		{
			name: "invalid accessKeyId format",
			content: `globalDir = "~/s3pit"

[[tenants]]
accessKeyId = "TEST@KEY#INVALID"
secretAccessKey = "test_secret"
customDir = "/tmp/test"
`,
			expectError: true,
			errorMsg:    "accessKeyId contains invalid characters",
		},
		{
			name: "valid with tilde expansion",
			content: `globalDir = "~/s3pit"

[[tenants]]
accessKeyId = "TEST_KEY"
secretAccessKey = "test_secret"
customDir = "~/test/tenant"
`,
			expectError: false,
		},
		{
			name: "valid with underscores and hyphens",
			content: `globalDir = "~/s3pit"

[[tenants]]
accessKeyId = "TEST_KEY-123"
secretAccessKey = "test_secret"
customDir = "/tmp/test"
`,
			expectError: false,
		},
		{
			name: "missing globalDir",
			content: `[[tenants]]
accessKeyId = "TEST_KEY"
secretAccessKey = "test_secret"
customDir = "/tmp/test"
`,
			expectError: true,
			errorMsg:    "global globalDir is required",
		},
		{
			name: "invalid globalDir - relative path",
			content: `globalDir = "relative/path"

[[tenants]]
accessKeyId = "TEST_KEY"
secretAccessKey = "test_secret"
`,
			expectError: true,
			errorMsg:    "global globalDir must be an absolute path",
		},
		{
			name: "valid with public buckets",
			content: `globalDir = "~/s3pit"

[[tenants]]
accessKeyId = "TEST_KEY"
secretAccessKey = "test_secret"
publicBuckets = ["public-*", "static-*"]
`,
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create temp file
			tmpFile, err := os.CreateTemp("", "config-*.toml")
			if err != nil {
				t.Fatalf("Failed to create temp file: %v", err)
			}
			defer os.Remove(tmpFile.Name())

			// Write test content
			if _, err := tmpFile.WriteString(tt.content); err != nil {
				t.Fatalf("Failed to write test content: %v", err)
			}
			tmpFile.Close()

			// Validate the file
			err = validateConfigFile(tmpFile.Name())

			if tt.expectError {
				if err == nil {
					t.Errorf("Expected error but got none")
				} else if tt.errorMsg != "" && !containsString(err.Error(), tt.errorMsg) {
					t.Errorf("Expected error containing '%s', got '%v'", tt.errorMsg, err)
				}
			} else {
				if err != nil {
					t.Errorf("Expected no error but got: %v", err)
				}
			}
		})
	}
}

func TestValidateConfigFileNotFound(t *testing.T) {
	err := validateConfigFile("/nonexistent/path/config.toml")
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
			name:     "valid with alphanumeric",
			input:    "TEST_KEY123",
			expected: true,
		},
		{
			name:     "valid with underscores",
			input:    "test_key_123",
			expected: true,
		},
		{
			name:     "valid with hyphens",
			input:    "test-key-123",
			expected: true,
		},
		{
			name:     "valid mixed case",
			input:    "TestKey123",
			expected: true,
		},
		{
			name:     "invalid with spaces",
			input:    "test key",
			expected: false,
		},
		{
			name:     "invalid with special chars",
			input:    "test@key#123",
			expected: false,
		},
		{
			name:     "invalid with dots",
			input:    "test.key",
			expected: false,
		},
		{
			name:     "empty string",
			input:    "",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isValidAccessKey(tt.input)
			if result != tt.expected {
				t.Errorf("isValidAccessKey(%q) = %v, want %v", tt.input, result, tt.expected)
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
			name:     "absolute path",
			input:    "/tmp/test",
			expected: true,
		},
		{
			name:     "home directory path",
			input:    "~/test",
			expected: true,
		},
		{
			name:     "relative path",
			input:    "test/dir",
			expected: false,
		},
		{
			name:     "parent directory path",
			input:    "../test",
			expected: false,
		},
		{
			name:     "current directory",
			input:    "./test",
			expected: false,
		},
		{
			name:     "empty string",
			input:    "",
			expected: false,
		},
		{
			name:     "single dot",
			input:    ".",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isValidDirectoryPath(tt.input)
			if result != tt.expected {
				t.Errorf("isValidDirectoryPath(%q) = %v, want %v", tt.input, result, tt.expected)
			}
		})
	}
}

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