package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

type Config struct {
	Host             string
	Port             int
	GlobalDir        string
	AuthMode         string
	ConfigFile       string
	InMemory         bool
	EnableDashboard  bool
	AutoCreateBucket bool
	Region           string
	LogLevel         string
	LogDir           string
	EnableFileLog    bool
	EnableConsoleLog bool
	LogRotationSize  int64
	MaxLogEntries    int
	MaxObjectSize    int64

	// Delay configuration for read operations
	ReadDelayMs        int // Fixed delay in milliseconds (0 = disabled)
	ReadDelayRandomMin int // Min delay for random mode (milliseconds)
	ReadDelayRandomMax int // Max delay for random mode (milliseconds)

	// Delay configuration for write operations
	WriteDelayMs        int // Fixed delay in milliseconds (0 = disabled)
	WriteDelayRandomMin int // Min delay for random mode (milliseconds)
	WriteDelayRandomMax int // Max delay for random mode (milliseconds)
}

// expandTilde expands the tilde (~) in a path to the user's home directory
func expandTilde(path string) string {
	if strings.HasPrefix(path, "~/") {
		home, err := os.UserHomeDir()
		if err == nil {
			return filepath.Join(home, path[2:])
		}
	}
	return path
}

func LoadFromEnv() *Config {
	// Get default config file path
	defaultConfigFile := ""
	if homeDir, err := os.UserHomeDir(); err == nil {
		defaultConfigFile = filepath.Join(homeDir, ".config", "s3pit", "config.toml")
	}

	cfg := &Config{
		Host:             getEnvOrDefault("S3PIT_HOST", "0.0.0.0"),
		Port:             getEnvAsIntOrDefault("S3PIT_PORT", 3333),
		GlobalDir:        expandTilde(getEnvOrDefault("S3PIT_GLOBAL_DIRECTORY", "~/s3pit")),
		AuthMode:         getEnvOrDefault("S3PIT_AUTH_MODE", "sigv4"),
		ConfigFile:       getEnvOrDefault("S3PIT_CONFIG_FILE", defaultConfigFile),
		InMemory:         getEnvAsBoolOrDefault("S3PIT_IN_MEMORY", false),
		EnableDashboard:  getEnvAsBoolOrDefault("S3PIT_ENABLE_DASHBOARD", true),
		AutoCreateBucket: getEnvAsBoolOrDefault("S3PIT_AUTO_CREATE_BUCKET", true),
		Region:           getEnvOrDefault("S3PIT_REGION", "us-east-1"),
		LogLevel:         getEnvOrDefault("S3PIT_LOG_LEVEL", "info"),
		LogDir:           getEnvOrDefault("S3PIT_LOG_DIR", "./logs"),
		EnableFileLog:    getEnvAsBoolOrDefault("S3PIT_ENABLE_FILE_LOG", true),
		EnableConsoleLog: getEnvAsBoolOrDefault("S3PIT_ENABLE_CONSOLE_LOG", true),
		LogRotationSize:  getEnvAsInt64OrDefault("S3PIT_LOG_ROTATION_SIZE", 100*1024*1024), // 100MB default
		MaxLogEntries:    getEnvAsIntOrDefault("S3PIT_MAX_LOG_ENTRIES", 10000),
		MaxObjectSize:    getEnvAsInt64OrDefault("S3PIT_MAX_OBJECT_SIZE", 5*1024*1024*1024), // 5GB default

		// Read delay configuration
		ReadDelayMs:        getEnvAsIntOrDefault("S3PIT_READ_DELAY_MS", 0),
		ReadDelayRandomMin: getEnvAsIntOrDefault("S3PIT_READ_DELAY_RANDOM_MIN_MS", 0),
		ReadDelayRandomMax: getEnvAsIntOrDefault("S3PIT_READ_DELAY_RANDOM_MAX_MS", 0),

		// Write delay configuration
		WriteDelayMs:        getEnvAsIntOrDefault("S3PIT_WRITE_DELAY_MS", 0),
		WriteDelayRandomMin: getEnvAsIntOrDefault("S3PIT_WRITE_DELAY_RANDOM_MIN_MS", 0),
		WriteDelayRandomMax: getEnvAsIntOrDefault("S3PIT_WRITE_DELAY_RANDOM_MAX_MS", 0),
	}

	return cfg
}

// UpdateGlobalDirFromTenants updates the GlobalDir from tenant configuration if available
// skipUpdate should be true if GlobalDir was explicitly set (e.g., via command line)
func (c *Config) UpdateGlobalDirFromTenants(tenantManager interface{}, skipUpdate bool) {
	if skipUpdate {
		return // Don't override explicit setting
	}

	// Use reflection-like interface to avoid circular dependency
	if tm, ok := tenantManager.(interface {
		GetGlobalDir() string
	}); ok {
		if globalDir := tm.GetGlobalDir(); globalDir != "" {
			c.GlobalDir = globalDir
		}
	}
}

func getEnvOrDefault(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func getEnvAsIntOrDefault(key string, defaultValue int) int {
	if value := os.Getenv(key); value != "" {
		if intVal, err := strconv.Atoi(value); err == nil {
			return intVal
		}
	}
	return defaultValue
}

func getEnvAsBoolOrDefault(key string, defaultValue bool) bool {
	if value := os.Getenv(key); value != "" {
		if boolVal, err := strconv.ParseBool(value); err == nil {
			return boolVal
		}
	}
	return defaultValue
}

func getEnvAsInt64OrDefault(key string, defaultValue int64) int64 {
	if value := os.Getenv(key); value != "" {
		if intVal, err := strconv.ParseInt(value, 10, 64); err == nil {
			return intVal
		}
	}
	return defaultValue
}

// Validate validates the configuration
func (c *Config) Validate() error {
	// Validate auth mode
	validAuthModes := []string{"sigv4"} // Only sigv4 is supported now
	if !contains(validAuthModes, c.AuthMode) {
		return fmt.Errorf("invalid auth mode: %s, must be: sigv4", c.AuthMode)
	}

	// Validate port
	if c.Port < 1 || c.Port > 65535 {
		return fmt.Errorf("invalid port: %d, must be between 1 and 65535", c.Port)
	}

	// Validate log level
	validLogLevels := []string{"debug", "info", "warn", "error"}
	if !contains(validLogLevels, strings.ToLower(c.LogLevel)) {
		return fmt.Errorf("invalid log level: %s, must be one of: %s", c.LogLevel, strings.Join(validLogLevels, ", "))
	}

	// Validate global directory if not in-memory
	if !c.InMemory {
		absPath, err := filepath.Abs(c.GlobalDir)
		if err != nil {
			return fmt.Errorf("invalid global directory: %w", err)
		}
		c.GlobalDir = absPath
	}

	// Note: Credentials are validated from config.toml file, not from static config

	return nil
}

func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}

// String returns a string representation of the configuration
func (c *Config) String() string {
	return fmt.Sprintf(
		"Host: %s\nPort: %d\nAuth Mode: %s\nGlobal Dir: %s\nIn Memory: %v\nDashboard: %v\nAuto Create Bucket: %v\nLog Level: %s\nMax Object Size: %d",
		c.Host, c.Port, c.AuthMode, c.GlobalDir, c.InMemory, c.EnableDashboard, c.AutoCreateBucket, c.LogLevel, c.MaxObjectSize,
	)
}
