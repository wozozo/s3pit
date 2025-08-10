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
	serveCmd.Flags().String("global-dir", "", "Override global directory path")
	serveCmd.Flags().String("auth-mode", "sigv4", "Authentication mode (sigv4 only)")
	serveCmd.Flags().String("tenants-file", "", "Path to tenants.json file for multi-tenancy")
	serveCmd.Flags().Bool("in-memory", false, "Use in-memory storage instead of filesystem")
	serveCmd.Flags().Bool("dashboard", true, "Enable web dashboard")
	serveCmd.Flags().Bool("auto-create-bucket", true, "Automatically create buckets on first upload")
	serveCmd.Flags().String("log-level", "info", "Log level: debug|info|warn|error")
	serveCmd.Flags().String("log-dir", "./logs", "Directory for log files")
	serveCmd.Flags().Bool("no-dashboard", false, "Disable web dashboard")
	serveCmd.Flags().Int64("max-object-size", 5368709120, "Maximum object size in bytes")
}

func runServe(cmd *cobra.Command, args []string) error {
	// Load configuration
	serveCfg := config.LoadFromEnv()

	// Track which options were set via command line
	cmdLineOverrides := make(map[string]bool)
	
	// Override with command line flags
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
	srv, err := server.NewWithCmdLineOverrides(serveCfg, cmdLineOverrides)
	if err != nil {
		return err
	}

	return srv.Start()
}
