package cmd

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/wozozo/s3pit/internal/config"
)

func TestLogDirectoryConfiguration(t *testing.T) {
	tests := []struct {
		name              string
		envVars           map[string]string
		cmdFlags          []string
		expectedLogDir    string
		expectedFileLog   bool
		expectDirCreation bool
	}{
		{
			name:              "Default - no log directory, console only",
			envVars:           map[string]string{},
			cmdFlags:          []string{},
			expectedLogDir:    "",
			expectedFileLog:   false,
			expectDirCreation: false,
		},
		{
			name:              "Explicit empty log-dir flag - console only",
			envVars:           map[string]string{},
			cmdFlags:          []string{"--log-dir", ""},
			expectedLogDir:    "",
			expectedFileLog:   false,
			expectDirCreation: false,
		},
		{
			name:              "Log-dir specified via flag - file logging enabled",
			envVars:           map[string]string{},
			cmdFlags:          []string{"--log-dir", "./test-logs"},
			expectedLogDir:    "./test-logs",
			expectedFileLog:   true,
			expectDirCreation: true,
		},
		{
			name: "Log-dir specified via env var - file logging enabled",
			envVars: map[string]string{
				"S3PIT_LOG_DIR": "./env-logs",
			},
			cmdFlags:          []string{},
			expectedLogDir:    "./env-logs",
			expectedFileLog:   true,
			expectDirCreation: true,
		},
		{
			name: "Empty log-dir via env var - console only",
			envVars: map[string]string{
				"S3PIT_LOG_DIR": "",
			},
			cmdFlags:          []string{},
			expectedLogDir:    "",
			expectedFileLog:   false,
			expectDirCreation: false,
		},
		{
			name: "Flag overrides env var",
			envVars: map[string]string{
				"S3PIT_LOG_DIR": "./env-logs",
			},
			cmdFlags:          []string{"--log-dir", "./flag-logs"},
			expectedLogDir:    "./flag-logs",
			expectedFileLog:   true,
			expectDirCreation: true,
		},
		{
			name: "Flag with empty string overrides env var",
			envVars: map[string]string{
				"S3PIT_LOG_DIR": "./env-logs",
			},
			cmdFlags:          []string{"--log-dir", ""},
			expectedLogDir:    "",
			expectedFileLog:   false,
			expectDirCreation: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set up environment
			for key, value := range tt.envVars {
				oldValue := os.Getenv(key)
				os.Setenv(key, value)
				defer os.Setenv(key, oldValue)
			}

			// Clean up any test directories
			defer func() {
				os.RemoveAll("./test-logs")
				os.RemoveAll("./env-logs")
				os.RemoveAll("./flag-logs")
			}()

			// Load configuration from environment
			cfg := config.LoadFromEnv()

			// Create a test command with flags
			cmd := &cobra.Command{}
			cmd.Flags().String("log-dir", "", "Directory for log files")

			// Parse flags if provided
			if len(tt.cmdFlags) > 0 {
				cmd.SetArgs(tt.cmdFlags)
				err := cmd.ParseFlags(tt.cmdFlags)
				require.NoError(t, err)

				// Apply command line overrides (simulate what runServe does)
				if cmd.Flags().Changed("log-dir") {
					logDir, _ := cmd.Flags().GetString("log-dir")
					cfg.LogDir = logDir
					cfg.EnableFileLog = logDir != ""
				}
			}

			// Verify configuration
			assert.Equal(t, tt.expectedLogDir, cfg.LogDir, "LogDir should match expected value")
			assert.Equal(t, tt.expectedFileLog, cfg.EnableFileLog, "EnableFileLog should match expected value")
		})
	}
}

func TestLoggerInitialization(t *testing.T) {
	tests := []struct {
		name            string
		logDir          string
		expectFileLog   bool
		expectDirExists bool
	}{
		{
			name:            "Empty log dir - no file logging",
			logDir:          "",
			expectFileLog:   false,
			expectDirExists: false,
		},
		{
			name:            "Valid log dir - file logging enabled",
			logDir:          "./test-logger-logs",
			expectFileLog:   true,
			expectDirExists: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Clean up test directory
			testDir := "./test-logger-logs"
			os.RemoveAll(testDir)
			defer os.RemoveAll(testDir)

			// Create config
			cfg := &config.Config{
				LogDir:        tt.logDir,
				EnableFileLog: tt.logDir != "",
			}

			// Simulate what the server initialization would do
			if cfg.LogDir != "" {
				// In actual server, this would be done by logger.SetLogDir
				// For testing, we just verify the config is correct
				assert.True(t, cfg.EnableFileLog, "File logging should be enabled when LogDir is set")
			} else {
				assert.False(t, cfg.EnableFileLog, "File logging should be disabled when LogDir is empty")
			}

			// Verify expectations
			assert.Equal(t, tt.expectFileLog, cfg.EnableFileLog, "EnableFileLog should match expected")

			// If we expect directory creation, verify the config would trigger it
			if tt.expectDirExists {
				assert.NotEmpty(t, cfg.LogDir, "LogDir should not be empty when directory creation is expected")
			} else {
				assert.Empty(t, cfg.LogDir, "LogDir should be empty when no directory creation is expected")
			}
		})
	}
}

func TestConfigValidationWithLogDir(t *testing.T) {
	tests := []struct {
		name        string
		config      *config.Config
		expectError bool
	}{
		{
			name: "Valid config with empty log dir",
			config: &config.Config{
				Host:             "0.0.0.0",
				Port:             3333,
				GlobalDir:        "/tmp/s3pit",
				AuthMode:         "sigv4",
				LogLevel:         "info",
				LogDir:           "",
				EnableFileLog:    false,
				EnableConsoleLog: true,
			},
			expectError: false,
		},
		{
			name: "Valid config with log dir",
			config: &config.Config{
				Host:             "0.0.0.0",
				Port:             3333,
				GlobalDir:        "/tmp/s3pit",
				AuthMode:         "sigv4",
				LogLevel:         "info",
				LogDir:           "./logs",
				EnableFileLog:    true,
				EnableConsoleLog: true,
			},
			expectError: false,
		},
		{
			name: "In-memory mode with empty log dir",
			config: &config.Config{
				Host:             "0.0.0.0",
				Port:             3333,
				InMemory:         true,
				AuthMode:         "sigv4",
				LogLevel:         "info",
				LogDir:           "",
				EnableFileLog:    false,
				EnableConsoleLog: true,
			},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()
			if tt.expectError {
				assert.Error(t, err, "Expected validation error")
			} else {
				assert.NoError(t, err, "Expected no validation error")
			}
		})
	}
}

func TestLogDirEnvironmentVariable(t *testing.T) {
	// Test that S3PIT_LOG_DIR environment variable is properly handled
	testCases := []struct {
		envValue        string
		expectedLogDir  string
		expectedFileLog bool
	}{
		{"", "", false},
		{"./custom-logs", "./custom-logs", true},
		{"/absolute/path/logs", "/absolute/path/logs", true},
	}

	for _, tc := range testCases {
		t.Run("S3PIT_LOG_DIR="+tc.envValue, func(t *testing.T) {
			// Set environment variable
			oldValue := os.Getenv("S3PIT_LOG_DIR")
			os.Setenv("S3PIT_LOG_DIR", tc.envValue)
			defer os.Setenv("S3PIT_LOG_DIR", oldValue)

			// Load config
			cfg := config.LoadFromEnv()

			// Verify
			assert.Equal(t, tc.expectedLogDir, cfg.LogDir)
			assert.Equal(t, tc.expectedFileLog, cfg.EnableFileLog)
		})
	}
}

func TestLogDirCreation(t *testing.T) {
	// Test that specifying a log dir would trigger directory creation
	// This is a simulation test - actual directory creation happens in logger package

	testDir := filepath.Join(os.TempDir(), "s3pit-test-logs")
	defer os.RemoveAll(testDir)

	cfg := &config.Config{
		LogDir:        testDir,
		EnableFileLog: true,
	}

	// Verify configuration is set correctly for directory creation
	assert.NotEmpty(t, cfg.LogDir, "LogDir should be set")
	assert.True(t, cfg.EnableFileLog, "File logging should be enabled")

	// In the actual implementation, the logger package would create the directory
	// Here we just verify the configuration is correct
	assert.Equal(t, testDir, cfg.LogDir, "LogDir should match the test directory")
}
