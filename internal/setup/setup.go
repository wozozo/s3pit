package setup

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/wozozo/s3pit/pkg/tenant"
)

// InitializeConfigDir creates the config directory and default tenants.json if they don't exist
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

	tenantsFile := filepath.Join(configDir, "tenants.json")

	// Check if tenants.json already exists
	if _, err := os.Stat(tenantsFile); err == nil {
		// File exists, do nothing
		return nil
	} else if !os.IsNotExist(err) {
		// Some other error occurred
		return fmt.Errorf("failed to check tenants.json: %w", err)
	}

	// Create default tenants configuration
	defaultConfig := tenant.TenantsConfig{
		GlobalDir: "~/s3pit/data",
		Tenants: []tenant.Tenant{
			{
				AccessKeyID:     "test-key",
				SecretAccessKey: "test-secret",
				Description:     "Default test tenant",
				PublicBuckets:   []string{}, // Initialize empty public buckets list
			},
		},
	}

	// Marshal to JSON with indentation
	data, err := json.MarshalIndent(defaultConfig, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal tenants: %w", err)
	}

	// Write to file
	if err := os.WriteFile(tenantsFile, data, 0644); err != nil {
		return fmt.Errorf("failed to write tenants.json: %w", err)
	}

	fmt.Printf("Created default configuration at %s\n", tenantsFile)
	return nil
}
