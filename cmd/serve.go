package cmd

import (
	"fmt"
	"net"
	"os"
	"strings"

	"github.com/pelletier/go-toml/v2"
	"github.com/spf13/cobra"
	"github.com/wozozo/s3pit/internal/config"
	"github.com/wozozo/s3pit/internal/setup"
	"github.com/wozozo/s3pit/pkg/server"
	"github.com/wozozo/s3pit/pkg/tenant"
)

// ANSI color codes
const (
	ColorReset  = "\033[0m"
	ColorRed    = "\033[31m"
	ColorGreen  = "\033[32m"
	ColorYellow = "\033[33m"
	ColorBlue   = "\033[34m"
	ColorPurple = "\033[35m"
	ColorCyan   = "\033[36m"
	ColorWhite  = "\033[37m"
	ColorBold   = "\033[1m"
	ColorDim    = "\033[2m"
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
	serveCmd.Flags().String("config-file", "", "Path to config.toml file for multi-tenancy")
	serveCmd.Flags().Bool("in-memory", false, "Use in-memory storage instead of filesystem")
	serveCmd.Flags().Bool("dashboard", true, "Enable web dashboard")
	serveCmd.Flags().Bool("auto-create-bucket", true, "Automatically create buckets on first upload")
	serveCmd.Flags().String("log-level", "info", "Log level: debug|info|warn|error")
	serveCmd.Flags().String("log-dir", "", "Directory for log files (empty = console only)")
	serveCmd.Flags().Bool("no-dashboard", false, "Disable web dashboard")
	serveCmd.Flags().Int64("max-object-size", 5368709120, "Maximum object size in bytes")

	// Delay configuration flags
	serveCmd.Flags().Int("read-delay-ms", 0, "Fixed delay for read operations in milliseconds")
	serveCmd.Flags().Int("read-delay-random-min", 0, "Minimum random delay for read operations in milliseconds")
	serveCmd.Flags().Int("read-delay-random-max", 0, "Maximum random delay for read operations in milliseconds")
	serveCmd.Flags().Int("write-delay-ms", 0, "Fixed delay for write operations in milliseconds")
	serveCmd.Flags().Int("write-delay-random-min", 0, "Minimum random delay for write operations in milliseconds")
	serveCmd.Flags().Int("write-delay-random-max", 0, "Maximum random delay for write operations in milliseconds")
}

// formatMainConfig formats the main configuration for display
func formatMainConfig(cfg *config.Config) string {
	var parts []string

	// Header
	parts = append(parts, fmt.Sprintf("%s%s[SERVER CONFIGURATION]%s", ColorBold, ColorBlue, ColorReset))
	parts = append(parts, fmt.Sprintf("%s%s", strings.Repeat("-", 50), ColorReset))

	// Network settings
	parts = append(parts, fmt.Sprintf("%s%sNetwork:%s", ColorBold, ColorGreen, ColorReset))
	parts = append(parts, fmt.Sprintf("  %sHost:%s %s%s%s", ColorBlue, ColorReset, ColorWhite, cfg.Host, ColorReset))
	parts = append(parts, fmt.Sprintf("  %sPort:%s %s%d%s", ColorBlue, ColorReset, ColorWhite, cfg.Port, ColorReset))
	parts = append(parts, "")

	// Storage settings
	parts = append(parts, fmt.Sprintf("%s%sStorage:%s", ColorBold, ColorGreen, ColorReset))
	if cfg.InMemory {
		parts = append(parts, fmt.Sprintf("  %sMode:%s %sIn-Memory%s", ColorBlue, ColorReset, ColorYellow, ColorReset))
	} else {
		parts = append(parts, fmt.Sprintf("  %sMode:%s %sFilesystem%s", ColorBlue, ColorReset, ColorYellow, ColorReset))
		parts = append(parts, fmt.Sprintf("  %sDirectory:%s %s%s%s", ColorBlue, ColorReset, ColorYellow, cfg.GlobalDir, ColorReset))
	}
	parts = append(parts, fmt.Sprintf("  %sAuto Create Buckets:%s %s%v%s", ColorBlue, ColorReset, ColorWhite, cfg.AutoCreateBucket, ColorReset))
	parts = append(parts, fmt.Sprintf("  %sMax Object Size:%s %s%d bytes%s", ColorBlue, ColorReset, ColorWhite, cfg.MaxObjectSize, ColorReset))
	parts = append(parts, "")

	// Authentication
	parts = append(parts, fmt.Sprintf("%s%sAuthentication:%s", ColorBold, ColorGreen, ColorReset))
	parts = append(parts, fmt.Sprintf("  %sMode:%s %s%s%s", ColorBlue, ColorReset, ColorCyan, cfg.AuthMode, ColorReset))
	if cfg.ConfigFile != "" {
		parts = append(parts, fmt.Sprintf("  %sConfig File:%s %s%s%s", ColorBlue, ColorReset, ColorDim, cfg.ConfigFile, ColorReset))
	}
	parts = append(parts, "")

	// Features
	parts = append(parts, fmt.Sprintf("%s%sFeatures:%s", ColorBold, ColorGreen, ColorReset))
	dashboardStatus := "Disabled"
	if cfg.EnableDashboard {
		dashboardStatus = "Enabled"
	}
	parts = append(parts, fmt.Sprintf("  %sDashboard:%s %s%s%s", ColorBlue, ColorReset, ColorWhite, dashboardStatus, ColorReset))
	parts = append(parts, fmt.Sprintf("  %sRegion:%s %s%s%s", ColorBlue, ColorReset, ColorWhite, cfg.Region, ColorReset))
	parts = append(parts, "")

	// Logging
	parts = append(parts, fmt.Sprintf("%s%sLogging:%s", ColorBold, ColorGreen, ColorReset))
	parts = append(parts, fmt.Sprintf("  %sLevel:%s %s%s%s", ColorBlue, ColorReset, ColorWhite, cfg.LogLevel, ColorReset))
	parts = append(parts, fmt.Sprintf("  %sDirectory:%s %s%s%s", ColorBlue, ColorReset, ColorDim, cfg.LogDir, ColorReset))
	parts = append(parts, fmt.Sprintf("  %sFile Logging:%s %s%v%s", ColorBlue, ColorReset, ColorWhite, cfg.EnableFileLog, ColorReset))
	parts = append(parts, fmt.Sprintf("  %sConsole Logging:%s %s%v%s", ColorBlue, ColorReset, ColorWhite, cfg.EnableConsoleLog, ColorReset))
	parts = append(parts, "")

	// Delay Configuration
	parts = append(parts, fmt.Sprintf("%s%sDelay Configuration:%s", ColorBold, ColorGreen, ColorReset))

	// Read delays
	if cfg.ReadDelayMs > 0 {
		parts = append(parts, fmt.Sprintf("  %sRead Delay:%s %s%dms (fixed)%s", ColorBlue, ColorReset, ColorYellow, cfg.ReadDelayMs, ColorReset))
	} else if cfg.ReadDelayRandomMin > 0 && cfg.ReadDelayRandomMax > 0 {
		parts = append(parts, fmt.Sprintf("  %sRead Delay:%s %s%d-%dms (random)%s", ColorBlue, ColorReset, ColorYellow, cfg.ReadDelayRandomMin, cfg.ReadDelayRandomMax, ColorReset))
	} else {
		parts = append(parts, fmt.Sprintf("  %sRead Delay:%s %sNone%s", ColorBlue, ColorReset, ColorDim, ColorReset))
	}

	// Write delays
	if cfg.WriteDelayMs > 0 {
		parts = append(parts, fmt.Sprintf("  %sWrite Delay:%s %s%dms (fixed)%s", ColorBlue, ColorReset, ColorYellow, cfg.WriteDelayMs, ColorReset))
	} else if cfg.WriteDelayRandomMin > 0 && cfg.WriteDelayRandomMax > 0 {
		parts = append(parts, fmt.Sprintf("  %sWrite Delay:%s %s%d-%dms (random)%s", ColorBlue, ColorReset, ColorYellow, cfg.WriteDelayRandomMin, cfg.WriteDelayRandomMax, ColorReset))
	} else {
		parts = append(parts, fmt.Sprintf("  %sWrite Delay:%s %sNone%s", ColorBlue, ColorReset, ColorDim, ColorReset))
	}
	parts = append(parts, "")

	// Available Options
	parts = append(parts, fmt.Sprintf("%s%sConfigurable Options:%s", ColorBold, ColorGreen, ColorReset))
	parts = append(parts, fmt.Sprintf("  %s--read-delay-ms:%s Fixed delay for read operations (ms)", ColorBlue, ColorReset))
	parts = append(parts, fmt.Sprintf("  %s--read-delay-random-min/max:%s Random delay range for reads", ColorBlue, ColorReset))
	parts = append(parts, fmt.Sprintf("  %s--write-delay-ms:%s Fixed delay for write operations (ms)", ColorBlue, ColorReset))
	parts = append(parts, fmt.Sprintf("  %s--write-delay-random-min/max:%s Random delay range for writes", ColorBlue, ColorReset))
	parts = append(parts, fmt.Sprintf("  %s--port:%s Server port (default: 3333)", ColorBlue, ColorReset))
	parts = append(parts, fmt.Sprintf("  %s--host:%s Server host (default: 0.0.0.0)", ColorBlue, ColorReset))
	parts = append(parts, fmt.Sprintf("  %s--global-dir:%s Storage directory", ColorBlue, ColorReset))
	parts = append(parts, fmt.Sprintf("  %s--auth-mode:%s Authentication mode (sigv4)", ColorBlue, ColorReset))
	parts = append(parts, fmt.Sprintf("  %s--in-memory:%s Use in-memory storage", ColorBlue, ColorReset))
	parts = append(parts, fmt.Sprintf("  %s--auto-create-bucket:%s Auto-create buckets on upload", ColorBlue, ColorReset))

	return strings.Join(parts, "\n")
}

// formatTenantsConfig formats the tenants configuration for display
func formatTenantsConfig(config *tenant.Config) string {
	var parts []string

	// Header
	parts = append(parts, fmt.Sprintf("%s%s[TENANT CONFIGURATION]%s", ColorBold, ColorCyan, ColorReset))
	parts = append(parts, fmt.Sprintf("%s%s", strings.Repeat("-", 50), ColorReset))

	// Global settings
	parts = append(parts, fmt.Sprintf("%sGlobal Directory:%s %s%s%s", ColorBlue, ColorReset, ColorYellow, config.GlobalDir, ColorReset))
	parts = append(parts, fmt.Sprintf("%sTotal Tenants:%s %s%d%s", ColorBlue, ColorReset, ColorWhite, len(config.Tenants), ColorReset))
	parts = append(parts, "")

	// Tenant details
	for i, tenant := range config.Tenants {
		parts = append(parts, fmt.Sprintf("%s%sTenant %d:%s", ColorBold, ColorPurple, i+1, ColorReset))
		parts = append(parts, fmt.Sprintf("  %sAccess Key:%s %s%s%s", ColorBlue, ColorReset, ColorGreen, tenant.AccessKeyID, ColorReset))
		parts = append(parts, fmt.Sprintf("  %sSecret Key:%s %s%s%s", ColorBlue, ColorReset, ColorDim, tenant.SecretAccessKey, ColorReset))

		if tenant.CustomDir != "" {
			parts = append(parts, fmt.Sprintf("  %sCustom Dir:%s %s%s%s", ColorBlue, ColorReset, ColorYellow, tenant.CustomDir, ColorReset))
		}
		if tenant.Description != "" {
			parts = append(parts, fmt.Sprintf("  %sDescription:%s %s", ColorBlue, ColorReset, tenant.Description))
		}
		if len(tenant.PublicBuckets) > 0 {
			parts = append(parts, fmt.Sprintf("  %sPublic Buckets:%s %s%s%s", ColorBlue, ColorReset, ColorCyan, strings.Join(tenant.PublicBuckets, ", "), ColorReset))
		}

		if i < len(config.Tenants)-1 {
			parts = append(parts, "")
		}
	}

	return strings.Join(parts, "\n")
}

// loadTenantsConfig loads the tenants configuration from a file
func loadTenantsConfig(filePath string) (*tenant.Config, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read file: %w", err)
	}

	var config tenant.Config
	if err := toml.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("invalid TOML format: %w", err)
	}

	return &config, nil
}

// getLocalIPAddresses returns a list of local non-loopback IP addresses
func getLocalIPAddresses() []string {
	var ips []string

	interfaces, err := net.InterfaceAddrs()
	if err != nil {
		return ips
	}

	for _, addr := range interfaces {
		if ipnet, ok := addr.(*net.IPNet); ok && !ipnet.IP.IsLoopback() {
			if ipnet.IP.To4() != nil {
				ips = append(ips, ipnet.IP.String())
			}
		}
	}

	return ips
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
	if cmd.Flags().Changed("config-file") {
		configFile, _ := cmd.Flags().GetString("config-file")
		serveCfg.ConfigFile = configFile
		cmdLineOverrides["config-file"] = true
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
		serveCfg.EnableFileLog = logDir != ""
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

	// Delay configuration flags
	if readDelayMs, _ := cmd.Flags().GetInt("read-delay-ms"); cmd.Flags().Changed("read-delay-ms") {
		serveCfg.ReadDelayMs = readDelayMs
		cmdLineOverrides["read-delay-ms"] = true
	}
	if readDelayMin, _ := cmd.Flags().GetInt("read-delay-random-min"); cmd.Flags().Changed("read-delay-random-min") {
		serveCfg.ReadDelayRandomMin = readDelayMin
		cmdLineOverrides["read-delay-random-min"] = true
	}
	if readDelayMax, _ := cmd.Flags().GetInt("read-delay-random-max"); cmd.Flags().Changed("read-delay-random-max") {
		serveCfg.ReadDelayRandomMax = readDelayMax
		cmdLineOverrides["read-delay-random-max"] = true
	}
	if writeDelayMs, _ := cmd.Flags().GetInt("write-delay-ms"); cmd.Flags().Changed("write-delay-ms") {
		serveCfg.WriteDelayMs = writeDelayMs
		cmdLineOverrides["write-delay-ms"] = true
	}
	if writeDelayMin, _ := cmd.Flags().GetInt("write-delay-random-min"); cmd.Flags().Changed("write-delay-random-min") {
		serveCfg.WriteDelayRandomMin = writeDelayMin
		cmdLineOverrides["write-delay-random-min"] = true
	}
	if writeDelayMax, _ := cmd.Flags().GetInt("write-delay-random-max"); cmd.Flags().Changed("write-delay-random-max") {
		serveCfg.WriteDelayRandomMax = writeDelayMax
		cmdLineOverrides["write-delay-random-max"] = true
	}

	// Initialize config directory and default config.toml if needed
	if err := setup.InitializeConfigDir(); err != nil {
		// Log the error but don't fail - it's not critical
		cmd.Printf("Warning: Failed to initialize config directory: %v\n", err)
	}

	// Print startup header
	fmt.Printf("\n%s%s=== S3PIT SERVER STARTUP ===%s\n\n", ColorBold, ColorCyan, ColorReset)

	// Print validation header
	fmt.Printf("%s%sValidation:%s\n", ColorBold, ColorYellow, ColorReset)
	fmt.Printf("%s%s", strings.Repeat("-", 30), ColorReset)
	fmt.Printf("\n%sConfiguration validation%s... ", ColorBlue, ColorReset)

	// Validate configuration
	if err := serveCfg.Validate(); err != nil {
		fmt.Printf("%sFAILED%s\n", ColorRed, ColorReset)
		fmt.Printf("%sError:%s %s\n", ColorRed, ColorReset, err.Error())
		return err
	}
	fmt.Printf("%sOK%s\n", ColorGreen, ColorReset)

	// If config file is configured, validate it
	if serveCfg.ConfigFile != "" {
		if _, err := os.Stat(serveCfg.ConfigFile); err == nil {
			fmt.Printf("%sConfig file validation%s... ", ColorBlue, ColorReset)
			if err := validateConfigFile(serveCfg.ConfigFile); err != nil {
				fmt.Printf("%sFAILED%s\n", ColorRed, ColorReset)
				fmt.Printf("%sError:%s %s\n", ColorRed, ColorReset, err.Error())
				return err
			}
			fmt.Printf("%sOK%s\n", ColorGreen, ColorReset)
		}
	}

	fmt.Printf("\n%sAll validations completed successfully!%s\n\n", ColorGreen, ColorReset)

	// Display configurations
	fmt.Printf("%s\n\n", formatMainConfig(serveCfg))

	// Display tenants configuration if available
	if serveCfg.ConfigFile != "" {
		if _, err := os.Stat(serveCfg.ConfigFile); err == nil {
			if tenantsConfig, err := loadTenantsConfig(serveCfg.ConfigFile); err == nil {
				fmt.Printf("%s\n\n", formatTenantsConfig(tenantsConfig))
			}
		}
	}

	// Print server start message
	fmt.Printf("%s%sStarting server...%s\n", ColorBold, ColorGreen, ColorReset)
	// Print server start message with multiple address formats
	fmt.Printf("%sServer listening on:%s\n", ColorBold, ColorReset)

	// Show localhost address
	fmt.Printf("  %s• http://localhost:%d%s\n", ColorYellow, serveCfg.Port, ColorReset)

	// Show 0.0.0.0 address
	if serveCfg.Host == "0.0.0.0" {
		fmt.Printf("  %s• http://0.0.0.0:%d%s (all interfaces)\n", ColorYellow, serveCfg.Port, ColorReset)

		// Get local IP addresses
		if localIPs := getLocalIPAddresses(); len(localIPs) > 0 {
			for _, ip := range localIPs {
				fmt.Printf("  %s• http://%s:%d%s (local network)\n", ColorYellow, ip, serveCfg.Port, ColorReset)
			}
		}
	} else {
		// Show the configured host
		fmt.Printf("  %s• http://%s:%d%s\n", ColorYellow, serveCfg.Host, serveCfg.Port, ColorReset)
	}

	// Show dashboard URLs if enabled
	if serveCfg.EnableDashboard {
		fmt.Printf("\n%sDashboard:%s\n", ColorBold, ColorReset)
		fmt.Printf("  %s• http://localhost:%d/dashboard%s\n", ColorCyan, serveCfg.Port, ColorReset)

		if serveCfg.Host == "0.0.0.0" {
			fmt.Printf("  %s• http://0.0.0.0:%d/dashboard%s (all interfaces)\n", ColorCyan, serveCfg.Port, ColorReset)

			// Get local IP addresses for dashboard
			if localIPs := getLocalIPAddresses(); len(localIPs) > 0 {
				for _, ip := range localIPs {
					fmt.Printf("  %s• http://%s:%d/dashboard%s (local network)\n", ColorCyan, ip, serveCfg.Port, ColorReset)
				}
			}
		} else {
			fmt.Printf("  %s• http://%s:%d/dashboard%s\n", ColorCyan, serveCfg.Host, serveCfg.Port, ColorReset)
		}
	}
	fmt.Printf("\n")

	// Create and start server
	srv, err := server.NewWithCmdLineOverrides(serveCfg, cmdLineOverrides)
	if err != nil {
		return err
	}

	return srv.Start()
}
