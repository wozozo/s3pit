package cmd

import (
	"github.com/spf13/cobra"
	"github.com/wozozo/s3pit/internal/config"
	"github.com/wozozo/s3pit/internal/setup"
	"github.com/wozozo/s3pit/pkg/server"
)

var (
	cfg     *config.Config
	rootCmd = &cobra.Command{
		Use:   "s3pit",
		Short: "S3-compatible storage server for development",
		Long:  `S3pit is a lightweight S3-compatible storage server designed for local development and testing.`,
		RunE:  runServer,
	}
)

func init() {
	cobra.OnInitialize(initConfig)

	rootCmd.PersistentFlags().IntP("port", "p", 3333, "Server port")
	rootCmd.PersistentFlags().StringP("host", "H", "0.0.0.0", "Server host")
	rootCmd.PersistentFlags().String("data-dir", "./data", "Data directory path")
	rootCmd.PersistentFlags().String("auth-mode", "sigv4", "Authentication mode (sigv4 only)")
	rootCmd.PersistentFlags().String("tenants-file", "", "Path to tenants.json file for multi-tenancy")
	rootCmd.PersistentFlags().Bool("in-memory", false, "Use in-memory storage instead of filesystem")
	rootCmd.PersistentFlags().Bool("dashboard", true, "Enable web dashboard")
	rootCmd.PersistentFlags().Bool("auto-create-bucket", true, "Automatically create buckets on first upload")
}

func initConfig() {
	cfg = config.LoadFromEnv()

	if port, _ := rootCmd.Flags().GetInt("port"); rootCmd.Flags().Changed("port") {
		cfg.Port = port
	}
	if host, _ := rootCmd.Flags().GetString("host"); rootCmd.Flags().Changed("host") {
		cfg.Host = host
	}
	if dataDir, _ := rootCmd.Flags().GetString("data-dir"); rootCmd.Flags().Changed("data-dir") {
		cfg.DataDir = dataDir
	}
	if authMode, _ := rootCmd.Flags().GetString("auth-mode"); rootCmd.Flags().Changed("auth-mode") {
		cfg.AuthMode = authMode
	}
	// Only override tenants file if explicitly provided
	// The Changed() check should prevent overriding when not specified
	if rootCmd.Flags().Changed("tenants-file") {
		tenantsFile, _ := rootCmd.Flags().GetString("tenants-file")
		cfg.TenantsFile = tenantsFile
	}
	if inMemory, _ := rootCmd.Flags().GetBool("in-memory"); rootCmd.Flags().Changed("in-memory") {
		cfg.InMemory = inMemory
	}
	if dashboard, _ := rootCmd.Flags().GetBool("dashboard"); rootCmd.Flags().Changed("dashboard") {
		cfg.EnableDashboard = dashboard
	}
	if autoCreate, _ := rootCmd.Flags().GetBool("auto-create-bucket"); rootCmd.Flags().Changed("auto-create-bucket") {
		cfg.AutoCreateBucket = autoCreate
	}
}

func runServer(cmd *cobra.Command, args []string) error {
	// Initialize config directory and default tenants.json if needed
	if err := setup.InitializeConfigDir(); err != nil {
		// Log the error but don't fail - it's not critical
		cmd.Printf("Warning: Failed to initialize config directory: %v\n", err)
	}

	// Validate configuration
	if err := cfg.Validate(); err != nil {
		return err
	}

	// Print configuration in debug mode
	if cfg.LogLevel == "debug" {
		cmd.Printf("Starting S3pit with configuration:\n%s\n", cfg.String())
	}

	srv, err := server.New(cfg)
	if err != nil {
		return err
	}

	return srv.Start()
}

func Execute() error {
	return rootCmd.Execute()
}
