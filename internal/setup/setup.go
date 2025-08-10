package setup

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/pelletier/go-toml/v2"
	"github.com/wozozo/s3pit/pkg/tenant"
)

// InitializeConfigDir creates the config directory and default config.toml if they don't exist
func InitializeConfigDir() error {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("failed to get home directory: %w", err)
	}

	configDir := filepath.Join(homeDir, ".config", "s3pit")

	// Create config directory if it doesn't exist
	if err := os.MkdirAll(configDir, 0755); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	configFile := filepath.Join(configDir, "config.toml")

	// Check if config.toml already exists
	if _, err := os.Stat(configFile); err == nil {
		// File exists, do nothing
		return nil
	} else if !os.IsNotExist(err) {
		// Some other error occurred
		return fmt.Errorf("failed to check config.toml: %w", err)
	}

	// Create developer-friendly default configuration
	defaultConfig := tenant.Config{
		GlobalDir: "~/s3pit/data",
		Tenants: []tenant.Tenant{
			{
				AccessKeyID:     "local-dev",
				SecretAccessKey: "local-dev-secret",
				Description:     "Local development with public assets (public-*, static-*, cdn-*)",
				PublicBuckets:   []string{"public-*", "static-*", "cdn-*"},
			},
			{
				AccessKeyID:     "test-app",
				SecretAccessKey: "test-app-secret",
				Description:     "Test application with specific public buckets",
				PublicBuckets:   []string{"assets", "downloads"},
			},
			{
				AccessKeyID:     "private-app",
				SecretAccessKey: "private-app-secret",
				Description:     "Private application (all buckets require authentication)",
				PublicBuckets:   []string{}, // No public buckets
			},
		},
	}

	// Marshal to TOML
	data, err := toml.Marshal(defaultConfig)
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	// Write to file
	if err := os.WriteFile(configFile, data, 0644); err != nil {
		return fmt.Errorf("failed to write config.toml: %w", err)
	}

	fmt.Printf("\n✅ Created default configuration at %s\n", configFile)
	fmt.Printf("\n📋 Default tenants created:\n")
	fmt.Printf("  • 'local-dev': Development with public buckets (public-*, static-*, cdn-*)\n")
	fmt.Printf("  • 'test-app': Testing with specific public buckets (assets, downloads)\n")
	fmt.Printf("  • 'private-app': Private data only (no public buckets)\n")
	fmt.Printf("\n💡 Quick start:\n")
	fmt.Printf("  export AWS_ACCESS_KEY_ID=local-dev\n")
	fmt.Printf("  export AWS_SECRET_ACCESS_KEY=local-dev-secret\n")
	fmt.Printf("  aws s3 cp file.txt s3://public-data/ --endpoint-url http://localhost:3333\n")
	fmt.Printf("\n")
	return nil
}
