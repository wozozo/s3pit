package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/wozozo/s3pit/internal/config"
	"github.com/wozozo/s3pit/internal/setup"
	"github.com/wozozo/s3pit/pkg/server"
)

var serveCmd = &cobra.Command{
	Use:   "serve",
	Short: "Start the S3pit server",
	Long:  `Start the S3pit server after validating the configuration.`,
	RunE:  runServe,
}

func init() {
	rootCmd.AddCommand(serveCmd)

	// Add server-specific flags
	serveCmd.Flags().IntP("port", "p", 3333, "Server port")
	serveCmd.Flags().StringP("host", "H", "0.0.0.0", "Server host")
	serveCmd.Flags().String("global-directory", "", "Override global directory path")
	serveCmd.Flags().String("auth-mode", "sigv4", "Authentication mode (sigv4 only)")
	serveCmd.Flags().String("tenants-file", "", "Path to tenants.json file for multi-tenancy")
	serveCmd.Flags().Bool("in-memory", false, "Use in-memory storage instead of filesystem")
	serveCmd.Flags().Bool("dashboard", true, "Enable web dashboard")
	serveCmd.Flags().Bool("auto-create-bucket", true, "Automatically create buckets on first upload")
}

func runServe(cmd *cobra.Command, args []string) error {
	// Load configuration
	serveCfg := config.LoadFromEnv()

	// Override with command line flags
	if port, _ := cmd.Flags().GetInt("port"); cmd.Flags().Changed("port") {
		serveCfg.Port = port
	}
	if host, _ := cmd.Flags().GetString("host"); cmd.Flags().Changed("host") {
		serveCfg.Host = host
	}
	if globalDir, _ := cmd.Flags().GetString("global-directory"); cmd.Flags().Changed("global-directory") {
		serveCfg.GlobalDir = globalDir
	}
	if authMode, _ := cmd.Flags().GetString("auth-mode"); cmd.Flags().Changed("auth-mode") {
		serveCfg.AuthMode = authMode
	}
	if cmd.Flags().Changed("tenants-file") {
		tenantsFile, _ := cmd.Flags().GetString("tenants-file")
		serveCfg.TenantsFile = tenantsFile
	}
	if inMemory, _ := cmd.Flags().GetBool("in-memory"); cmd.Flags().Changed("in-memory") {
		serveCfg.InMemory = inMemory
	}
	if dashboard, _ := cmd.Flags().GetBool("dashboard"); cmd.Flags().Changed("dashboard") {
		serveCfg.EnableDashboard = dashboard
	}
	if autoCreate, _ := cmd.Flags().GetBool("auto-create-bucket"); cmd.Flags().Changed("auto-create-bucket") {
		serveCfg.AutoCreateBucket = autoCreate
	}

	// Initialize config directory and default tenants.json if needed
	if err := setup.InitializeConfigDir(); err != nil {
		// Log the error but don't fail - it's not critical
		cmd.Printf("Warning: Failed to initialize config directory: %v\n", err)
	}

	// Print validation header
	fmt.Println("Validating configuration...")

	// Validate configuration
	if err := serveCfg.Validate(); err != nil {
		fmt.Printf("❌ Configuration validation failed: %v\n", err)
		return err
	}

	// If tenants file is configured, validate it
	if serveCfg.TenantsFile != "" {
		if _, err := os.Stat(serveCfg.TenantsFile); err == nil {
			fmt.Printf("Validating tenants file: %s\n", serveCfg.TenantsFile)
			if err := validateTenantsFile(serveCfg.TenantsFile); err != nil {
				fmt.Printf("❌ Tenants file validation failed: %v\n", err)
				return err
			}
			fmt.Println("✅ Tenants file validation successful")
		}
	}

	fmt.Println("✅ Configuration validation successful")
	fmt.Println("")

	// Print configuration in debug mode
	if serveCfg.LogLevel == "debug" {
		cmd.Printf("Starting S3pit with configuration:\n%s\n", serveCfg.String())
	}

	// Create and start server
	srv, err := server.New(serveCfg)
	if err != nil {
		return err
	}

	return srv.Start()
}
