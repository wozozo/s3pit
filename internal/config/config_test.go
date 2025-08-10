package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/pelletier/go-toml/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/wozozo/s3pit/pkg/tenant"
)

func TestLoadFromEnv(t *testing.T) {
	// Save original env vars
	originalVars := map[string]string{
		"S3PIT_HOST":             os.Getenv("S3PIT_HOST"),
		"S3PIT_PORT":             os.Getenv("S3PIT_PORT"),
		"S3PIT_GLOBAL_DIRECTORY": os.Getenv("S3PIT_GLOBAL_DIRECTORY"),
		"S3PIT_AUTH_MODE":        os.Getenv("S3PIT_AUTH_MODE"),
		"S3PIT_LOG_LEVEL":        os.Getenv("S3PIT_LOG_LEVEL"),
	}

	// Clean env vars for test
	for key := range originalVars {
		os.Unsetenv(key)
	}

	// Restore after test
	defer func() {
		for key, val := range originalVars {
			if val != "" {
				os.Setenv(key, val)
			} else {
				os.Unsetenv(key)
			}
		}
	}()

	tests := []struct {
		name    string
		envVars map[string]string
		checkFn func(*testing.T, *Config)
	}{
		{
			name:    "Default values",
			envVars: map[string]string{},
			checkFn: func(t *testing.T, cfg *Config) {
				assert.Equal(t, "0.0.0.0", cfg.Host)
				assert.Equal(t, 3333, cfg.Port)
				assert.Contains(t, cfg.GlobalDir, "s3pit")
				assert.Equal(t, "sigv4", cfg.AuthMode)
				assert.Equal(t, "info", cfg.LogLevel)
				assert.True(t, cfg.EnableDashboard)
				assert.True(t, cfg.AutoCreateBucket)
			},
		},
		{
			name: "Custom env values",
			envVars: map[string]string{
				"S3PIT_HOST":               "127.0.0.1",
				"S3PIT_PORT":               "8080",
				"S3PIT_GLOBAL_DIRECTORY":   "/custom/path",
				"S3PIT_AUTH_MODE":          "sigv4",
				"S3PIT_LOG_LEVEL":          "debug",
				"S3PIT_ENABLE_DASHBOARD":   "false",
				"S3PIT_AUTO_CREATE_BUCKET": "false",
			},
			checkFn: func(t *testing.T, cfg *Config) {
				assert.Equal(t, "127.0.0.1", cfg.Host)
				assert.Equal(t, 8080, cfg.Port)
				assert.Equal(t, "/custom/path", cfg.GlobalDir)
				assert.Equal(t, "sigv4", cfg.AuthMode)
				assert.Equal(t, "debug", cfg.LogLevel)
				assert.False(t, cfg.EnableDashboard)
				assert.False(t, cfg.AutoCreateBucket)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set env vars for this test
			for key, val := range tt.envVars {
				os.Setenv(key, val)
			}

			cfg := LoadFromEnv()
			tt.checkFn(t, cfg)

			// Clean up env vars
			for key := range tt.envVars {
				os.Unsetenv(key)
			}
		})
	}
}

func TestUpdateGlobalDirFromTenants(t *testing.T) {
	tempDir := t.TempDir()
	configFile := filepath.Join(tempDir, "config.toml")

	// Create test config.toml
	tenantsConfig := tenant.Config{
		GlobalDir: tempDir + "/tenants-global",
		Tenants: []tenant.Tenant{
			{
				AccessKeyID:     "test-tenant",
				SecretAccessKey: "test-secret",
			},
		},
	}

	configData, err := toml.Marshal(tenantsConfig)
	require.NoError(t, err)

	err = os.WriteFile(configFile, configData, 0644)
	require.NoError(t, err)

	tests := []struct {
		name             string
		initialGlobalDir string
		skipUpdate       bool
		expectedDir      string
		description      string
	}{
		{
			name:             "Update when skipUpdate is false",
			initialGlobalDir: tempDir + "/original",
			skipUpdate:       false,
			expectedDir:      tempDir + "/tenants-global",
			description:      "Should update GlobalDir from config.toml when skipUpdate is false",
		},
		{
			name:             "Don't update when skipUpdate is true",
			initialGlobalDir: tempDir + "/cmdline-set",
			skipUpdate:       true,
			expectedDir:      tempDir + "/cmdline-set",
			description:      "Should not update GlobalDir when skipUpdate is true (command line override)",
		},
		{
			name:             "Handle empty tenants globalDir",
			initialGlobalDir: tempDir + "/original",
			skipUpdate:       false,
			expectedDir:      tempDir + "/original", // Should remain unchanged
			description:      "Should not update when config.toml has empty globalDir",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &Config{
				GlobalDir: tt.initialGlobalDir,
			}

			tenantMgr := tenant.NewManager(configFile)
			err := tenantMgr.LoadFromFile()
			require.NoError(t, err)

			// For the empty globalDir test, temporarily clear it
			if tt.name == "Handle empty tenants globalDir" {
				// Create a tenant manager with empty globalDir
				emptyTenantsConfig := tenant.Config{
					GlobalDir: "", // Empty
					Tenants:   tenantsConfig.Tenants,
				}
				emptyConfigData, err := toml.Marshal(emptyTenantsConfig)
				require.NoError(t, err)

				emptyTenantsFile := filepath.Join(tempDir, "empty-config.toml")
				err = os.WriteFile(emptyTenantsFile, emptyConfigData, 0644)
				require.NoError(t, err)

				tenantMgr = tenant.NewManager(emptyTenantsFile)
				err = tenantMgr.LoadFromFile()
				require.NoError(t, err)
			}

			cfg.UpdateGlobalDirFromTenants(tenantMgr, tt.skipUpdate)
			assert.Equal(t, tt.expectedDir, cfg.GlobalDir, tt.description)
		})
	}
}

func TestConfigValidation(t *testing.T) {
	tests := []struct {
		name        string
		config      *Config
		expectError bool
		errorMsg    string
	}{
		{
			name: "Valid config",
			config: &Config{
				Host:      "localhost",
				Port:      3333,
				GlobalDir: "/tmp/s3pit",
				AuthMode:  "sigv4",
				LogLevel:  "info",
				InMemory:  false,
			},
			expectError: false,
		},
		{
			name: "Invalid auth mode",
			config: &Config{
				Host:      "localhost",
				Port:      3333,
				GlobalDir: "/tmp/s3pit",
				AuthMode:  "invalid",
				LogLevel:  "info",
				InMemory:  false,
			},
			expectError: true,
			errorMsg:    "invalid auth mode",
		},
		{
			name: "Invalid port - too low",
			config: &Config{
				Host:      "localhost",
				Port:      0,
				GlobalDir: "/tmp/s3pit",
				AuthMode:  "sigv4",
				LogLevel:  "info",
				InMemory:  false,
			},
			expectError: true,
			errorMsg:    "invalid port",
		},
		{
			name: "Invalid port - too high",
			config: &Config{
				Host:      "localhost",
				Port:      99999,
				GlobalDir: "/tmp/s3pit",
				AuthMode:  "sigv4",
				LogLevel:  "info",
				InMemory:  false,
			},
			expectError: true,
			errorMsg:    "invalid port",
		},
		{
			name: "Invalid log level",
			config: &Config{
				Host:      "localhost",
				Port:      3333,
				GlobalDir: "/tmp/s3pit",
				AuthMode:  "sigv4",
				LogLevel:  "invalid",
				InMemory:  false,
			},
			expectError: true,
			errorMsg:    "invalid log level",
		},
		{
			name: "In-memory config - no directory validation",
			config: &Config{
				Host:     "localhost",
				Port:     3333,
				AuthMode: "sigv4",
				LogLevel: "info",
				InMemory: true,
			},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()

			if tt.expectError {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.errorMsg)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestConfigString(t *testing.T) {
	cfg := &Config{
		Host:             "localhost",
		Port:             3333,
		AuthMode:         "sigv4",
		GlobalDir:        "/tmp/s3pit",
		InMemory:         false,
		EnableDashboard:  true,
		AutoCreateBucket: true,
		LogLevel:         "info",
		MaxObjectSize:    5368709120,
	}

	str := cfg.String()

	// Check that the string contains expected values
	assert.Contains(t, str, "Host: localhost")
	assert.Contains(t, str, "Port: 3333")
	assert.Contains(t, str, "Auth Mode: sigv4")
	assert.Contains(t, str, "Global Dir: /tmp/s3pit")
	assert.Contains(t, str, "In Memory: false")
	assert.Contains(t, str, "Dashboard: true")
	assert.Contains(t, str, "Auto Create Bucket: true")
	assert.Contains(t, str, "Log Level: info")
	assert.Contains(t, str, "Max Object Size: 5368709120")
}

func TestExpandTilde(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "Path with tilde",
			input:    "~/test/path",
			expected: "", // Will be set to actual home dir + "/test/path" in test
		},
		{
			name:     "Path without tilde",
			input:    "/absolute/path",
			expected: "/absolute/path",
		},
		{
			name:     "Relative path",
			input:    "relative/path",
			expected: "relative/path",
		},
		{
			name:     "Just tilde",
			input:    "~",
			expected: "~", // expandTilde only handles ~/
		},
	}

	// Get home directory for the tilde test
	homeDir, err := os.UserHomeDir()
	require.NoError(t, err)
	tests[0].expected = filepath.Join(homeDir, "test", "path")

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := expandTilde(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}
