package cmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/wozozo/s3pit/internal/config"
	"github.com/wozozo/s3pit/pkg/tenant"
)

func TestCmdLineOverrideTracking(t *testing.T) {
	// Create a temporary directory for test
	tempDir := t.TempDir()
	tenantsFile := filepath.Join(tempDir, "tenants.json")

	// Create test tenants.json
	tenantsConfig := tenant.TenantsConfig{
		GlobalDir: tempDir + "/from-tenants",
		Tenants: []tenant.Tenant{
			{
				AccessKeyID:     "test-tenant",
				SecretAccessKey: "test-secret",
			},
		},
	}
	
	configData, err := json.MarshalIndent(tenantsConfig, "", "  ")
	require.NoError(t, err)
	
	err = os.WriteFile(tenantsFile, configData, 0644)
	require.NoError(t, err)

	tests := []struct {
		name                    string
		args                    []string
		expectedGlobalDir       string
		expectedPort            int
		expectedHost            string
		expectedLogLevel        string
		expectedAutoCreate      bool
		expectedOverrides       map[string]bool
		description             string
	}{
		{
			name: "No command line flags",
			args: []string{"serve"},
			expectedGlobalDir: "",  // Will be set from env default
			expectedPort: 0,       // Will be set from env default
			expectedHost: "",      // Will be set from env default
			expectedLogLevel: "",  // Will be set from env default
			expectedAutoCreate: false, // Will be set from env default
			expectedOverrides: map[string]bool{},
			description: "No flags should result in no overrides",
		},
		{
			name: "Global dir flag only",
			args: []string{"serve", "--global-dir", tempDir + "/cmdline"},
			expectedGlobalDir: tempDir + "/cmdline",
			expectedOverrides: map[string]bool{
				"global-dir": true,
			},
			description: "Only global-dir should be marked as override",
		},
		{
			name: "Multiple flags",
			args: []string{"serve", 
				"--global-dir", tempDir + "/multi",
				"--port", "8080",
				"--host", "127.0.0.1",
				"--log-level", "debug",
				"--no-dashboard",
			},
			expectedGlobalDir: tempDir + "/multi",
			expectedPort: 8080,
			expectedHost: "127.0.0.1",
			expectedLogLevel: "debug",
			expectedOverrides: map[string]bool{
				"global-dir": true,
				"port": true,
				"host": true,
				"log-level": true,
				"no-dashboard": true,
			},
			description: "All specified flags should be marked as overrides",
		},
		{
			name: "Boolean flags",
			args: []string{"serve",
				"--in-memory",
				"--auto-create-bucket=false",
			},
			expectedAutoCreate: false,
			expectedOverrides: map[string]bool{
				"in-memory": true,
				"auto-create-bucket": true,
			},
			description: "Boolean flags should be tracked correctly",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a fresh command for each test
			cmd := &cobra.Command{
				Use: "serve",
				RunE: func(cmd *cobra.Command, args []string) error {
					// Load base config
					serveCfg := config.LoadFromEnv()
					
					// Track which options were set via command line
					cmdLineOverrides := make(map[string]bool)
					
					// Check each flag - this mimics the actual serve.go logic
					if port, _ := cmd.Flags().GetInt("port"); cmd.Flags().Changed("port") {
						serveCfg.Port = port
						cmdLineOverrides["port"] = true
					}
					if host, _ := cmd.Flags().GetString("host"); cmd.Flags().Changed("host") {
						serveCfg.Host = host
						cmdLineOverrides["host"] = true
					}
					if globalDir, _ := cmd.Flags().GetString("global-dir"); cmd.Flags().Changed("global-dir") {
						serveCfg.GlobalDir = globalDir
						cmdLineOverrides["global-dir"] = true
					}
					if authMode, _ := cmd.Flags().GetString("auth-mode"); cmd.Flags().Changed("auth-mode") {
						serveCfg.AuthMode = authMode
						cmdLineOverrides["auth-mode"] = true
					}
					if cmd.Flags().Changed("tenants-file") {
						tenantsFile, _ := cmd.Flags().GetString("tenants-file")
						serveCfg.TenantsFile = tenantsFile
						cmdLineOverrides["tenants-file"] = true
					}
					if inMemory, _ := cmd.Flags().GetBool("in-memory"); cmd.Flags().Changed("in-memory") {
						serveCfg.InMemory = inMemory
						cmdLineOverrides["in-memory"] = true
					}
					if dashboard, _ := cmd.Flags().GetBool("dashboard"); cmd.Flags().Changed("dashboard") {
						serveCfg.EnableDashboard = dashboard
						cmdLineOverrides["dashboard"] = true
					}
					if autoCreate, _ := cmd.Flags().GetBool("auto-create-bucket"); cmd.Flags().Changed("auto-create-bucket") {
						serveCfg.AutoCreateBucket = autoCreate
						cmdLineOverrides["auto-create-bucket"] = true
					}
					if logLevel, _ := cmd.Flags().GetString("log-level"); cmd.Flags().Changed("log-level") {
						serveCfg.LogLevel = logLevel
						cmdLineOverrides["log-level"] = true
					}
					if logDir, _ := cmd.Flags().GetString("log-dir"); cmd.Flags().Changed("log-dir") {
						serveCfg.LogDir = logDir
						cmdLineOverrides["log-dir"] = true
					}
					if noDashboard, _ := cmd.Flags().GetBool("no-dashboard"); cmd.Flags().Changed("no-dashboard") {
						serveCfg.EnableDashboard = !noDashboard
						cmdLineOverrides["no-dashboard"] = true
					}
					if maxObjectSize, _ := cmd.Flags().GetInt64("max-object-size"); cmd.Flags().Changed("max-object-size") {
						serveCfg.MaxObjectSize = maxObjectSize
						cmdLineOverrides["max-object-size"] = true
					}

					// Verify expected overrides
					assert.Equal(t, tt.expectedOverrides, cmdLineOverrides, tt.description)
					
					// Verify specific values if specified in test
					if tt.expectedGlobalDir != "" {
						assert.Equal(t, tt.expectedGlobalDir, serveCfg.GlobalDir)
					}
					if tt.expectedPort != 0 {
						assert.Equal(t, tt.expectedPort, serveCfg.Port)
					}
					if tt.expectedHost != "" {
						assert.Equal(t, tt.expectedHost, serveCfg.Host)
					}
					if tt.expectedLogLevel != "" {
						assert.Equal(t, tt.expectedLogLevel, serveCfg.LogLevel)
					}
					if tt.name == "Boolean flags" {
						assert.Equal(t, tt.expectedAutoCreate, serveCfg.AutoCreateBucket)
					}

					return nil
				},
			}

			// Add all the flags - this mirrors the actual serve command flags
			cmd.Flags().IntP("port", "p", 3333, "Server port")
			cmd.Flags().StringP("host", "H", "0.0.0.0", "Server host")
			cmd.Flags().String("global-dir", "", "Override global directory path")
			cmd.Flags().String("auth-mode", "sigv4", "Authentication mode (sigv4 only)")
			cmd.Flags().String("tenants-file", "", "Path to tenants.json file for multi-tenancy")
			cmd.Flags().Bool("in-memory", false, "Use in-memory storage instead of filesystem")
			cmd.Flags().Bool("dashboard", true, "Enable web dashboard")
			cmd.Flags().Bool("auto-create-bucket", true, "Automatically create buckets on first upload")
			cmd.Flags().String("log-level", "info", "Log level: debug|info|warn|error")
			cmd.Flags().String("log-dir", "./logs", "Directory for log files")
			cmd.Flags().Bool("no-dashboard", false, "Disable web dashboard")
			cmd.Flags().Int64("max-object-size", 5368709120, "Maximum object size in bytes")

			// Set the args and execute
			cmd.SetArgs(tt.args[1:]) // Remove "serve" from args
			err := cmd.Execute()
			assert.NoError(t, err)
		})
	}
}

// TestFlagParsing tests that flags are parsed correctly
func TestFlagParsing(t *testing.T) {
	tests := []struct {
		name     string
		args     []string
		checkFn  func(*testing.T, *cobra.Command)
	}{
		{
			name: "Port flag parsing",
			args: []string{"serve", "--port", "8080"},
			checkFn: func(t *testing.T, cmd *cobra.Command) {
				port, _ := cmd.Flags().GetInt("port")
				assert.Equal(t, 8080, port)
				assert.True(t, cmd.Flags().Changed("port"))
			},
		},
		{
			name: "Global dir flag parsing", 
			args: []string{"serve", "--global-dir", "/tmp/test"},
			checkFn: func(t *testing.T, cmd *cobra.Command) {
				globalDir, _ := cmd.Flags().GetString("global-dir")
				assert.Equal(t, "/tmp/test", globalDir)
				assert.True(t, cmd.Flags().Changed("global-dir"))
			},
		},
		{
			name: "Boolean flag parsing - enabled",
			args: []string{"serve", "--in-memory"},
			checkFn: func(t *testing.T, cmd *cobra.Command) {
				inMemory, _ := cmd.Flags().GetBool("in-memory")
				assert.True(t, inMemory)
				assert.True(t, cmd.Flags().Changed("in-memory"))
			},
		},
		{
			name: "Boolean flag parsing - disabled",
			args: []string{"serve", "--auto-create-bucket=false"},
			checkFn: func(t *testing.T, cmd *cobra.Command) {
				autoCreate, _ := cmd.Flags().GetBool("auto-create-bucket")
				assert.False(t, autoCreate)
				assert.True(t, cmd.Flags().Changed("auto-create-bucket"))
			},
		},
		{
			name: "Multiple flags",
			args: []string{"serve", "--port", "9000", "--host", "localhost", "--log-level", "debug"},
			checkFn: func(t *testing.T, cmd *cobra.Command) {
				port, _ := cmd.Flags().GetInt("port")
				host, _ := cmd.Flags().GetString("host")
				logLevel, _ := cmd.Flags().GetString("log-level")
				
				assert.Equal(t, 9000, port)
				assert.Equal(t, "localhost", host)
				assert.Equal(t, "debug", logLevel)
				
				assert.True(t, cmd.Flags().Changed("port"))
				assert.True(t, cmd.Flags().Changed("host"))
				assert.True(t, cmd.Flags().Changed("log-level"))
				assert.False(t, cmd.Flags().Changed("global-dir"))
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := &cobra.Command{
				Use: "serve",
				RunE: func(cmd *cobra.Command, args []string) error {
					tt.checkFn(t, cmd)
					return nil
				},
			}

			// Add flags
			cmd.Flags().IntP("port", "p", 3333, "Server port")
			cmd.Flags().StringP("host", "H", "0.0.0.0", "Server host")
			cmd.Flags().String("global-dir", "", "Override global directory path")
			cmd.Flags().String("auth-mode", "sigv4", "Authentication mode")
			cmd.Flags().String("tenants-file", "", "Path to tenants.json")
			cmd.Flags().Bool("in-memory", false, "Use in-memory storage")
			cmd.Flags().Bool("dashboard", true, "Enable web dashboard")
			cmd.Flags().Bool("auto-create-bucket", true, "Auto create buckets")
			cmd.Flags().String("log-level", "info", "Log level")
			cmd.Flags().String("log-dir", "./logs", "Log directory")
			cmd.Flags().Bool("no-dashboard", false, "Disable web dashboard")
			cmd.Flags().Int64("max-object-size", 5368709120, "Max object size")

			cmd.SetArgs(tt.args[1:])
			err := cmd.Execute()
			assert.NoError(t, err)
		})
	}
}