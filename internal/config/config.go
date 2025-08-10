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
	DataDir          string
	AuthMode         string
	TenantsFile      string
	InMemory         bool
	EnableDashboard  bool
	AutoCreateBucket bool
	AccessKeyID      string
	SecretAccessKey  string
	Region           string
	LogLevel         string
	LogDir           string
	EnableFileLog    bool
	EnableConsoleLog bool
	LogRotationSize  int64
	MaxLogEntries    int
	MaxObjectSize    int64
	MaxBuckets       int
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
	// Get default tenants file path
	defaultTenantsFile := ""
	if homeDir, err := os.UserHomeDir(); err == nil {
		defaultTenantsFile = filepath.Join(homeDir, ".config", "s3pit", "tenants.json")
	}

	cfg := &Config{
		Host:             getEnvOrDefault("S3PIT_HOST", "0.0.0.0"),
		Port:             getEnvAsIntOrDefault("S3PIT_PORT", 3333),
		DataDir:          expandTilde(getEnvOrDefault("S3PIT_DATA_DIR", "~/s3pit")),
		AuthMode:         getEnvOrDefault("S3PIT_AUTH_MODE", "sigv4"),
		TenantsFile:      getEnvOrDefault("S3PIT_TENANTS_FILE", defaultTenantsFile),
		InMemory:         getEnvAsBoolOrDefault("S3PIT_IN_MEMORY", false),
		EnableDashboard:  getEnvAsBoolOrDefault("S3PIT_ENABLE_DASHBOARD", true),
		AutoCreateBucket: getEnvAsBoolOrDefault("S3PIT_AUTO_CREATE_BUCKET", true),
		AccessKeyID:      getEnvOrDefault("S3PIT_ACCESS_KEY_ID", "minioadmin"),
		SecretAccessKey:  getEnvOrDefault("S3PIT_SECRET_ACCESS_KEY", "minioadmin"),
		Region:           getEnvOrDefault("S3PIT_REGION", "us-east-1"),
		LogLevel:         getEnvOrDefault("S3PIT_LOG_LEVEL", "info"),
		LogDir:           getEnvOrDefault("S3PIT_LOG_DIR", "./logs"),
		EnableFileLog:    getEnvAsBoolOrDefault("S3PIT_ENABLE_FILE_LOG", true),
		EnableConsoleLog: getEnvAsBoolOrDefault("S3PIT_ENABLE_CONSOLE_LOG", true),
		LogRotationSize:  getEnvAsInt64OrDefault("S3PIT_LOG_ROTATION_SIZE", 100*1024*1024), // 100MB default
		MaxLogEntries:    getEnvAsIntOrDefault("S3PIT_MAX_LOG_ENTRIES", 10000),
		MaxObjectSize:    getEnvAsInt64OrDefault("S3PIT_MAX_OBJECT_SIZE", 5*1024*1024*1024), // 5GB default
		MaxBuckets:       getEnvAsIntOrDefault("S3PIT_MAX_BUCKETS", 100),
	}

	return cfg
}

// UpdateDataDirFromTenants updates the DataDir from tenant configuration if available
func (c *Config) UpdateDataDirFromTenants(tenantManager interface{}) {
	// Use reflection-like interface to avoid circular dependency
	if tm, ok := tenantManager.(interface {
		GetGlobalDirectory() string
	}); ok {
		if globalDirectory := tm.GetGlobalDirectory(); globalDirectory != "" {
			c.DataDir = globalDirectory
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

	// Validate data directory if not in-memory
	if !c.InMemory {
		absPath, err := filepath.Abs(c.DataDir)
		if err != nil {
			return fmt.Errorf("invalid data directory: %w", err)
		}
		c.DataDir = absPath
	}

	// Validate credentials for authentication
	if c.AuthMode == "sigv4" {
		if c.AccessKeyID == "" || c.SecretAccessKey == "" {
			return fmt.Errorf("access key and secret key are required for auth mode: %s", c.AuthMode)
		}
	}

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
		"Host: %s\nPort: %d\nAuth Mode: %s\nData Dir: %s\nIn Memory: %v\nDashboard: %v\nAuto Create Bucket: %v\nLog Level: %s\nMax Object Size: %d\nMax Buckets: %d",
		c.Host, c.Port, c.AuthMode, c.DataDir, c.InMemory, c.EnableDashboard, c.AutoCreateBucket, c.LogLevel, c.MaxObjectSize, c.MaxBuckets,
	)
}
