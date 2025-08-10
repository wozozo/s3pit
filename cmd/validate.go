package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"github.com/wozozo/s3pit/pkg/tenant"
)

var validateCmd = &cobra.Command{
	Use:   "validate [path]",
	Short: "Validate tenants.json configuration file",
	Long: `Validate the tenants.json configuration file for syntax and semantic errors.
If no path is provided, validates the default file at ~/.config/s3pit/tenants.json`,
	Args: cobra.MaximumNArgs(1),
	RunE: runValidate,
}

func init() {
	rootCmd.AddCommand(validateCmd)
}

func runValidate(cmd *cobra.Command, args []string) error {
	var tenantsFile string

	if len(args) > 0 {
		tenantsFile = args[0]
	} else {
		homeDir, err := os.UserHomeDir()
		if err != nil {
			return fmt.Errorf("failed to get home directory: %w", err)
		}
		tenantsFile = filepath.Join(homeDir, ".config", "s3pit", "tenants.json")
	}

	// Check if file exists
	if _, err := os.Stat(tenantsFile); os.IsNotExist(err) {
		return fmt.Errorf("tenants.json file not found at: %s", tenantsFile)
	}

	fmt.Printf("Validating tenants configuration: %s\n", tenantsFile)

	// Validate the file
	if err := validateTenantsFile(tenantsFile); err != nil {
		fmt.Printf("❌ Validation failed: %v\n", err)
		return err
	}

	fmt.Println("✅ Validation successful: tenants.json is valid")
	return nil
}

func validateTenantsFile(filePath string) error {
	// Read and parse the JSON file
	data, err := os.ReadFile(filePath)
	if err != nil {
		return fmt.Errorf("failed to read file: %w", err)
	}

	var config tenant.TenantsConfig
	if err := json.Unmarshal(data, &config); err != nil {
		return fmt.Errorf("invalid JSON format: %w", err)
	}

	// Validate structure and content
	if len(config.Tenants) == 0 {
		return fmt.Errorf("no tenants defined in configuration")
	}

	// Validate global directory (required)
	if config.GlobalDir == "" {
		return fmt.Errorf("global globalDir is required")
	}
	if !isValidDirectoryPath(config.GlobalDir) {
		return fmt.Errorf("global globalDir must be an absolute path (starting with /) or home directory path (starting with ~/), got: %s", config.GlobalDir)
	}

	for i, tenant := range config.Tenants {
		// Validate required fields
		if tenant.AccessKeyID == "" {
			return fmt.Errorf("tenant %d: accessKeyId is required", i)
		}
		if tenant.SecretAccessKey == "" {
			return fmt.Errorf("tenant %d: secretAccessKey is required", i)
		}

		// Validate access key format (basic alphanumeric with underscores and hyphens)
		if !isValidAccessKey(tenant.AccessKeyID) {
			return fmt.Errorf("tenant %d: accessKeyId contains invalid characters (only alphanumeric, underscore, and hyphen allowed)", i)
		}

		// Validate custom directory path if provided - must be absolute or start with ~/
		if tenant.CustomDir != "" && !isValidDirectoryPath(tenant.CustomDir) {
			return fmt.Errorf("tenant %d: customDir must be an absolute path (starting with /) or home directory path (starting with ~/), got: %s", i, tenant.CustomDir)
		}
	}

	return nil
}

// isValidAccessKey checks if access key contains only valid characters
func isValidAccessKey(key string) bool {
	for _, r := range key {
		if !((r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '_' || r == '-') {
			return false
		}
	}
	return true
}

// isValidDirectoryPath checks if directory path is absolute or starts with ~/
func isValidDirectoryPath(path string) bool {
	return filepath.IsAbs(path) || strings.HasPrefix(path, "~/")
}
