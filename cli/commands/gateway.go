package commands

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"text/template"
	"time"

	"github.com/smallnest/goclaw/bus"
	"github.com/smallnest/goclaw/channels"
	"github.com/smallnest/goclaw/config"
	"github.com/smallnest/goclaw/gateway"
	"github.com/smallnest/goclaw/internal"
	"github.com/smallnest/goclaw/internal/logger"
	"github.com/smallnest/goclaw/session"
	"github.com/spf13/cobra"
	"go.uber.org/zap"
)

var (
	gatewayPort      int
	gatewayBind      string
	gatewayToken     string
	gatewayAuth      bool
	gatewayPassword  string
	gatewayTailscale bool
	gatewayDev       bool
	gatewayReset     bool
	gatewayForce     bool
	gatewayVerbose   bool
	gatewayParams    string
)

// GatewayCommand returns the gateway command
func GatewayCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "gateway",
		Short: "Manage WebSocket Gateway",
		Long:  `Run and manage the goclaw WebSocket gateway server.`,
	}

	// Main gateway run command
	runCmd := &cobra.Command{
		Use:   "run",
		Short: "Run WebSocket Gateway",
		Run:   runGateway,
	}
	runCmd.Flags().IntVarP(&gatewayPort, "port", "p", 28789, "Gateway port")
	runCmd.Flags().StringVarP(&gatewayBind, "bind", "b", "0.0.0.0", "Bind address")
	runCmd.Flags().StringVarP(&gatewayToken, "token", "t", "", "Authentication token")
	runCmd.Flags().BoolVar(&gatewayAuth, "auth", false, "Enable authentication")
	runCmd.Flags().StringVarP(&gatewayPassword, "password", "P", "", "Password for authentication")
	runCmd.Flags().BoolVar(&gatewayTailscale, "tailscale", false, "Use Tailscale")
	runCmd.Flags().BoolVar(&gatewayDev, "dev", false, "Development mode")
	runCmd.Flags().BoolVar(&gatewayReset, "reset", false, "Reset configuration")
	runCmd.Flags().BoolVarP(&gatewayForce, "force", "f", false, "Force start")
	runCmd.Flags().BoolVarP(&gatewayVerbose, "verbose", "v", false, "Verbose output")

	// Gateway status command
	statusCmd := &cobra.Command{
		Use:   "status",
		Short: "Show gateway status",
		Run:   runGatewayStatus,
	}

	// Gateway health command
	healthCmd := &cobra.Command{
		Use:   "health",
		Short: "Check gateway health",
		Run:   runGatewayHealth,
	}

	// Gateway probe command
	probeCmd := &cobra.Command{
		Use:   "probe",
		Short: "Probe gateway connectivity",
		Run:   runGatewayProbe,
	}

	// Gateway install command
	installCmd := &cobra.Command{
		Use:   "install",
		Short: "Install gateway as service",
		Run:   runGatewayInstall,
	}
	installCmd.Flags().IntVarP(&gatewayPort, "port", "p", 28789, "Gateway port")

	// Gateway uninstall command
	uninstallCmd := &cobra.Command{
		Use:   "uninstall",
		Short: "Uninstall gateway service",
		Run:   runGatewayUninstall,
	}

	// Gateway start command
	startCmd := &cobra.Command{
		Use:   "start",
		Short: "Start gateway service",
		Run:   runGatewayStart,
	}

	// Gateway stop command
	stopCmd := &cobra.Command{
		Use:   "stop",
		Short: "Stop gateway service",
		Run:   runGatewayStop,
	}

	// Gateway restart command
	restartCmd := &cobra.Command{
		Use:   "restart",
		Short: "Restart gateway service",
		Run:   runGatewayRestart,
	}

	// Gateway call command
	callCmd := &cobra.Command{
		Use:   "call <method>",
		Short: "Make RPC call to gateway",
		Args:  cobra.ExactArgs(1),
		Run:   runGatewayCall,
	}
	callCmd.Flags().StringVarP(&gatewayParams, "params", "p", "{}", "Parameters as JSON")

	// Gateway reload command
	reloadCmd := &cobra.Command{
		Use:   "reload",
		Short: "Reload gateway configuration",
		Run:   runGatewayReload,
	}

	// Gateway config history command
	historyCmd := &cobra.Command{
		Use:   "history",
		Short: "Show configuration change history",
		Run:   runGatewayHistory,
	}

	// Gateway config rollback command
	rollbackCmd := &cobra.Command{
		Use:   "rollback [index]",
		Short: "Rollback to a previous configuration",
		Args:  cobra.MaximumNArgs(1),
		Run:   runGatewayRollback,
	}

	cmd.AddCommand(runCmd, statusCmd, healthCmd, probeCmd)
	cmd.AddCommand(installCmd, uninstallCmd, startCmd, stopCmd, restartCmd)
	cmd.AddCommand(callCmd, reloadCmd, historyCmd, rollbackCmd)

	return cmd
}

// runGateway runs the gateway server
func runGateway(cmd *cobra.Command, args []string) {
	// 日志同时输出到 stdout 与 ~/.goclaw/logs/goclaw-2006-01-02.log（按日期）
	logPath := filepath.Join(internal.GetGoclawDir(), "logs", "goclaw-"+time.Now().Format("2006-01-02")+".log")
	logLevel := "info"
	if gatewayVerbose {
		logLevel = "debug"
	}
	if err := logger.InitWithFile(logLevel, false, logPath); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to initialize logger: %v\n", err)
		os.Exit(1)
	}
	defer logger.Sync() // nolint:errcheck
	logger.Info("Log file", zap.String("path", logPath))

	fmt.Println("🚀 Starting goclaw Gateway")

	// Load configuration
	cfg, err := config.Load("")
	if err != nil {
		logger.Warn("Failed to load config, using defaults", zap.Error(err))
		cfg = &config.Config{}
	}
	configFile := config.ConfigFileUsed()
	if configFile == "" {
		configFile = "(defaults/env only)"
	}
	logger.Info("config loaded", zap.String("config_file", configFile), zap.String("agents.defaults.model", cfg.Agents.Defaults.Model))

	// Enable hot reload if config file exists
	if configFile != "" && configFile != "(defaults/env only)" {
		if err := config.EnableHotReload(configFile); err != nil {
			logger.Warn("Failed to enable config hot reload", zap.Error(err))
		} else {
			logger.Info("Config hot reload enabled", zap.String("watching", configFile))
		}
	}

	// Override config with flags
	if gatewayPort != 0 {
		cfg.Gateway.Port = gatewayPort
	}
	if gatewayBind != "" {
		cfg.Gateway.Host = gatewayBind
	}

	// Create components
	messageBus := bus.NewMessageBus(100)
	defer messageBus.Close()

	// Session 目录与 start/status 一致（Windows 兼容）
	sessionDir := filepath.Join(internal.GetGoclawDir(), "sessions")
	if cfg.Session.Store != "" {
		sessionDir = cfg.Session.Store
	}
	sessionMgr, err := session.NewManager(sessionDir)
	if err != nil {
		logger.Fatal("Failed to create session manager", zap.Error(err))
	}
	var sessionPolicy *session.ResetPolicy
	if cfg.Session.Reset != nil {
		p := session.ToResetPolicy(&session.SessionResetConfigLike{
			Mode: cfg.Session.Reset.Mode, AtHour: cfg.Session.Reset.AtHour, IdleMinutes: cfg.Session.Reset.IdleMinutes,
		})
		sessionPolicy = &p
		sessionMgr.SetResetPolicy(sessionPolicy)
	}

	channelMgr := channels.NewManager(messageBus)
	if err := channelMgr.SetupFromConfig(cfg); err != nil {
		logger.Warn("Failed to setup channels from config", zap.Error(err))
	}

	gatewayServer := gateway.NewServer(&cfg.Gateway, messageBus, channelMgr, sessionMgr)
	if sessionPolicy != nil {
		gatewayServer.SetSessionResetPolicy(sessionPolicy)
	}

	// Only override WebSocket config if CLI flags are explicitly provided
	// If no CLI flags are set, use config file settings (already loaded by NewServer)
	if gatewayPort != 0 || gatewayBind != "" || gatewayAuth || gatewayToken != "" || gatewayPassword != "" {
		wsConfig := &gateway.WebSocketConfig{
			Host:           gatewayBind,
			Port:           gatewayPort,
			Path:           "/ws",
			EnableAuth:     gatewayAuth || gatewayToken != "" || gatewayPassword != "",
			AuthToken:      gatewayToken,
			PingInterval:   30 * time.Second,
			PongTimeout:    60 * time.Second,
			ReadTimeout:    60 * time.Second,
			WriteTimeout:   10 * time.Second,
			MaxMessageSize: 10 * 1024 * 1024,
		}

		if gatewayPassword != "" {
			wsConfig.AuthToken = gatewayPassword
		}

		gatewayServer.SetWebSocketConfig(wsConfig)
	}

	// Create context for graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Handle signals
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigChan
		fmt.Println("\nShutting down gateway...")
		cancel()
	}()

	// Start gateway
	if err := gatewayServer.Start(ctx); err != nil {
		logger.Fatal("Failed to start gateway", zap.Error(err))
	}

	// Register config change handlers
	if configFile != "" && configFile != "(defaults/env only)" {
		if err := config.OnConfigChange(func(oldCfg, newCfg *config.Config) error {
			logger.Info("Configuration changed, reloading components...")

			// Update gateway configuration
			if err := gatewayServer.HandleConfigReload(oldCfg, newCfg); err != nil {
				logger.Error("Failed to reload gateway config", zap.Error(err))
				return err
			}

			// Update channel manager configuration
			if err := channelMgr.SetupFromConfig(newCfg); err != nil {
				logger.Error("Failed to reload channel config", zap.Error(err))
				return err
			}

			// Update session manager configuration
			if newCfg.Session.Reset != nil {
				p := session.ToResetPolicy(&session.SessionResetConfigLike{
					Mode:        newCfg.Session.Reset.Mode,
					AtHour:      newCfg.Session.Reset.AtHour,
					IdleMinutes: newCfg.Session.Reset.IdleMinutes,
				})
				sessionMgr.SetResetPolicy(&p)
				gatewayServer.SetSessionResetPolicy(&p)
			}

			// Broadcast config reload notification to all connected clients
			gatewayServer.BroadcastConfigReload()

			logger.Info("Configuration reloaded successfully")
			return nil
		}); err != nil {
			logger.Warn("Failed to register config change handler", zap.Error(err))
		}
	}

	// Start channels
	if err := channelMgr.Start(ctx); err != nil {
		logger.Error("Failed to start channels", zap.Error(err))
	}
	defer func() { _ = channelMgr.Stop() }()

	// Start outbound message dispatcher
	go func() {
		defer func() {
			if r := recover(); r != nil {
				logger.Error("Outbound message dispatcher panicked",
					zap.Any("panic", r))
			}
		}()
		if err := channelMgr.DispatchOutbound(ctx); err != nil {
			logger.Error("Outbound message dispatcher exited with error", zap.Error(err))
		} else {
			logger.Info("Outbound message dispatcher exited normally")
		}
	}()

	// Determine actual host and port to display
	displayHost := gatewayBind
	displayPort := gatewayPort
	if displayHost == "" {
		displayHost = cfg.Gateway.WebSocket.Host
		if displayHost == "" {
			displayHost = "0.0.0.0"
		}
	}
	if displayPort == 0 {
		displayPort = cfg.Gateway.WebSocket.Port
		if displayPort == 0 {
			displayPort = 28789
		}
	}

	fmt.Printf("Gateway listening on %s:%d\n", displayHost, displayPort)
	fmt.Printf("WebSocket: ws://%s:%d/ws\n", displayHost, displayPort)
	fmt.Printf("Health: http://%s:%d/health\n", displayHost, displayPort)

	if gatewayAuth || gatewayToken != "" || gatewayPassword != "" {
		fmt.Println("Authentication: enabled")
	}

	fmt.Println("\nPress Ctrl+C to stop")

	// Wait for context cancellation
	<-ctx.Done()

	// Stop gateway
	if err := gatewayServer.Stop(); err != nil {
		logger.Error("Failed to stop gateway", zap.Error(err))
	}

	fmt.Println("Gateway stopped")
	defer logger.Sync() // nolint:errcheck
}

// runGatewayStatus shows gateway status
func runGatewayStatus(cmd *cobra.Command, args []string) {
	// Try to connect to local gateway
	url := fmt.Sprintf("http://localhost:%d/health", gatewayPort)
	if gatewayPort == 0 {
		url = "http://localhost:28789/health"
	}

	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		fmt.Printf("Gateway status: offline\n")
		fmt.Printf("Error: %v\n", err)
		return
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	var health map[string]interface{}
	_ = json.Unmarshal(body, &health)

	fmt.Println("Gateway status: online")
	if status, ok := health["status"]; ok {
		fmt.Printf("  Status: %v\n", status)
	}
	if version, ok := health["version"]; ok {
		fmt.Printf("  Version: %v\n", version)
	}
	if timestamp, ok := health["time"]; ok {
		fmt.Printf("  Timestamp: %v\n", timestamp)
	}
}

// runGatewayHealth checks gateway health
func runGatewayHealth(cmd *cobra.Command, args []string) {
	url := fmt.Sprintf("http://localhost:%d/health", gatewayPort)
	if gatewayPort == 0 {
		url = "http://localhost:28789/health"
	}

	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		fmt.Printf("Health check failed: %v\n", err)
		os.Exit(1)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		fmt.Printf("Health check failed: status %d\n", resp.StatusCode)
		os.Exit(1)
	}

	body, _ := io.ReadAll(resp.Body)
	fmt.Printf("Health: OK\n")
	fmt.Printf("Response: %s\n", string(body))
}

// runGatewayProbe probes gateway connectivity
func runGatewayProbe(cmd *cobra.Command, args []string) {
	ports := []int{28789, 28790, 28791}
	if gatewayPort != 0 {
		ports = []int{gatewayPort}
	}

	fmt.Println("Probing for gateway...")
	for _, port := range ports {
		url := fmt.Sprintf("http://localhost:%d/health", port)
		client := &http.Client{Timeout: 2 * time.Second}

		resp, err := client.Get(url)
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				fmt.Printf("Found gateway on port %d\n", port)
				body, _ := io.ReadAll(resp.Body)
				var health map[string]interface{}
				_ = json.Unmarshal(body, &health)
				if version, ok := health["version"]; ok {
					fmt.Printf("  Version: %v\n", version)
				}
				return
			}
		}
	}

	fmt.Println("No gateway found")
	os.Exit(1)
}

// runGatewayInstall installs gateway as service
func runGatewayInstall(cmd *cobra.Command, args []string) {
	fmt.Println("Installing goclaw Gateway service...")

	// Get the executable path
	execPath, err := os.Executable()
	if err != nil {
		execPath, err = filepath.Abs(os.Args[0])
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: Cannot determine executable path: %v\n", err)
			os.Exit(1)
		}
	}

	// Verify executable exists
	if _, err := os.Stat(execPath); os.IsNotExist(err) {
		fmt.Fprintf(os.Stderr, "Error: Executable not found at %s\n", execPath)
		os.Exit(1)
	}

	switch runtime.GOOS {
	case "darwin":
		installMacOSService(execPath)
	case "linux":
		installLinuxService(execPath)
	case "windows":
		installWindowsService(execPath)
	default:
		fmt.Fprintf(os.Stderr, "Error: Unsupported operating system: %s\n", runtime.GOOS)
		fmt.Println("Please run gateway manually with: goclaw gateway run")
		os.Exit(1)
	}
}

// runGatewayUninstall uninstalls gateway service
func runGatewayUninstall(cmd *cobra.Command, args []string) {
	fmt.Println("Uninstalling goclaw Gateway service...")

	switch runtime.GOOS {
	case "darwin":
		uninstallMacOSService()
	case "linux":
		uninstallLinuxService()
	case "windows":
		uninstallWindowsService()
	default:
		fmt.Fprintf(os.Stderr, "Error: Unsupported operating system: %s\n", runtime.GOOS)
		os.Exit(1)
	}
}

// runGatewayStart starts gateway service
func runGatewayStart(cmd *cobra.Command, args []string) {
	fmt.Println("Starting goclaw Gateway service...")

	switch runtime.GOOS {
	case "darwin":
		startMacOSService()
	case "linux":
		startLinuxService()
	case "windows":
		startWindowsService()
	default:
		fmt.Fprintf(os.Stderr, "Error: Unsupported operating system: %s\n", runtime.GOOS)
		os.Exit(1)
	}
}

// runGatewayStop stops gateway service
func runGatewayStop(cmd *cobra.Command, args []string) {
	fmt.Println("Stopping goclaw Gateway service...")

	switch runtime.GOOS {
	case "darwin":
		stopMacOSService()
	case "linux":
		stopLinuxService()
	case "windows":
		stopWindowsService()
	default:
		fmt.Fprintf(os.Stderr, "Error: Unsupported operating system: %s\n", runtime.GOOS)
		os.Exit(1)
	}
}

// runGatewayRestart restarts gateway service
func runGatewayRestart(cmd *cobra.Command, args []string) {
	fmt.Println("Restarting goclaw Gateway service...")

	switch runtime.GOOS {
	case "darwin":
		restartMacOSService()
	case "linux":
		restartLinuxService()
	case "windows":
		restartWindowsService()
	default:
		fmt.Fprintf(os.Stderr, "Error: Unsupported operating system: %s\n", runtime.GOOS)
		os.Exit(1)
	}
}

// Service name constants
const (
	serviceName        = "goclaw-gateway"
	macOSDomainStyle   = "com.goclaw.gateway"
	macOSPlistDir      = "Library/LaunchAgents"
	macOSPlistFile     = "com.goclaw.gateway.plist"
	linuxServiceDir    = ".config/systemd/user"
	linuxServiceFile   = "goclaw-gateway.service"
	windowsServiceName = "GoClawGateway"
)

// macOS service functions

func installMacOSService(execPath string) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: Cannot get home directory: %v\n", err)
		os.Exit(1)
	}

	plistDir := filepath.Join(homeDir, macOSPlistDir)
	plistPath := filepath.Join(plistDir, macOSPlistFile)

	// Create directory if it doesn't exist
	if err := os.MkdirAll(plistDir, 0755); err != nil {
		fmt.Fprintf(os.Stderr, "Error: Cannot create directory %s: %v\n", plistDir, err)
		os.Exit(1)
	}

	// Check if service already exists
	if _, err := os.Stat(plistPath); err == nil {
		fmt.Printf("Service already installed at %s\n", plistPath)
		fmt.Println("Use 'goclaw gateway uninstall' first to remove it")
		os.Exit(1)
	}

	// Get working directory (use the directory containing the executable)
	workDir := filepath.Dir(execPath)

	// Create plist content
	plistContent := `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>Label</key>
    <string>{{.Label}}</string>
    <key>ProgramArguments</key>
    <array>
        <string>{{.ExecPath}}</string>
        <string>gateway</string>
        <string>run</string>
        <string>--port</string>
        <string>{{.Port}}</string>
    </array>
    <key>RunAtLoad</key>
    <true/>
    <key>KeepAlive</key>
    <true/>
    <key>WorkingDirectory</key>
    <string>{{.WorkDir}}</string>
    <key>StandardOutPath</key>
    <string>{{.HomeDir}}/.goclaw/logs/gateway.stdout.log</string>
    <key>StandardErrorPath</key>
    <string>{{.HomeDir}}/.goclaw/logs/gateway.stderr.log</string>
    <key>EnvironmentVariables</key>
    <dict>
        <key>PATH</key>
        <string>/usr/local/bin:/usr/bin:/bin:/usr/local/sbin:/usr/sbin:/sbin</string>
    </dict>
    <key>ProcessType</key>
    <string>Interactive</string>
</dict>
</plist>
`

	// Create template
	tmpl, err := template.New("plist").Parse(plistContent)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: Cannot create plist template: %v\n", err)
		os.Exit(1)
	}

	// Ensure log directory exists
	logDir := filepath.Join(homeDir, ".goclaw/logs")
	if err := os.MkdirAll(logDir, 0755); err != nil {
		fmt.Fprintf(os.Stderr, "Error: Cannot create log directory %s: %v\n", logDir, err)
		os.Exit(1)
	}

	// Execute template
	plistFile, err := os.Create(plistPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: Cannot create plist file %s: %v\n", plistPath, err)
		os.Exit(1)
	}
	defer plistFile.Close()

	data := struct {
		Label    string
		ExecPath string
		WorkDir  string
		HomeDir  string
		Port     int
	}{
		Label:    macOSDomainStyle,
		ExecPath: execPath,
		WorkDir:  workDir,
		HomeDir:  homeDir,
		Port:     gatewayPort,
	}

	if err := tmpl.Execute(plistFile, data); err != nil {
		fmt.Fprintf(os.Stderr, "Error: Cannot write plist file: %v\n", err)
		os.Exit(1)
	}

	// Load the service
	fmt.Println("Loading service...")
	if err := exec.Command("launchctl", "load", plistPath).Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: Cannot load service: %v\n", err)
		fmt.Println("Note: You may need to run: launchctl load", plistPath)
		os.Exit(1)
	}

	fmt.Printf("Gateway service installed successfully\n")
	fmt.Printf("  Config: %s\n", plistPath)
	fmt.Printf("  Executable: %s\n", execPath)
	fmt.Printf("  Port: %d\n", gatewayPort)
	fmt.Printf("  Logs: %s/gateway.stdout.log\n", logDir)
	fmt.Printf("  Logs: %s/gateway.stderr.log\n", logDir)
	fmt.Println("\nUse 'goclaw gateway start' to start the service")
}

func uninstallMacOSService() {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: Cannot get home directory: %v\n", err)
		os.Exit(1)
	}

	plistPath := filepath.Join(homeDir, macOSPlistDir, macOSPlistFile)

	// Check if service exists
	if _, err := os.Stat(plistPath); os.IsNotExist(err) {
		fmt.Println("Service not installed")
		return
	}

	// Stop the service if running
	fmt.Println("Stopping service...")
	exec.Command("launchctl", "unload", plistPath).Run() // nolint:errcheck

	// Remove the plist file
	fmt.Println("Removing service configuration...")
	if err := os.Remove(plistPath); err != nil {
		fmt.Fprintf(os.Stderr, "Error: Cannot remove plist file: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("Gateway service uninstalled successfully")
}

func startMacOSService() {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: Cannot get home directory: %v\n", err)
		os.Exit(1)
	}

	plistPath := filepath.Join(homeDir, macOSPlistDir, macOSPlistFile)

	// Check if service exists
	if _, err := os.Stat(plistPath); os.IsNotExist(err) {
		fmt.Println("Service not installed. Use 'goclaw gateway install' first")
		os.Exit(1)
	}

	// Start the service
	fmt.Println("Starting service...")
	cmd := exec.Command("launchctl", "start", macOSDomainStyle)
	output, err := cmd.CombinedOutput()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: Cannot start service: %v\n", err)
		if len(output) > 0 {
			fmt.Fprintf(os.Stderr, "Output: %s\n", string(output))
		}
		os.Exit(1)
	}

	// Check if it's actually running
	time.Sleep(500 * time.Millisecond)
	if checkGatewayRunning() {
		fmt.Println("Gateway service started successfully")
	} else {
		fmt.Println("Service started, but gateway may not be responding yet")
		fmt.Println("Check logs: ~/.goclaw/logs/gateway.stdout.log")
	}
}

func stopMacOSService() {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: Cannot get home directory: %v\n", err)
		os.Exit(1)
	}

	plistPath := filepath.Join(homeDir, macOSPlistDir, macOSPlistFile)

	// Check if service exists
	if _, err := os.Stat(plistPath); os.IsNotExist(err) {
		fmt.Println("Service not installed")
		return
	}

	// Stop the service
	fmt.Println("Stopping service...")
	cmd := exec.Command("launchctl", "stop", macOSDomainStyle)
	output, err := cmd.CombinedOutput()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: Cannot stop service: %v\n", err)
		if len(output) > 0 {
			fmt.Fprintf(os.Stderr, "Output: %s\n", string(output))
		}
		os.Exit(1)
	}

	fmt.Println("Gateway service stopped successfully")
}

func restartMacOSService() {
	fmt.Println("Restarting service...")
	stopMacOSService()
	time.Sleep(1 * time.Second)
	startMacOSService()
}

// Linux service functions

func installLinuxService(execPath string) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: Cannot get home directory: %v\n", err)
		os.Exit(1)
	}

	serviceDir := filepath.Join(homeDir, linuxServiceDir)
	servicePath := filepath.Join(serviceDir, linuxServiceFile)

	// Create directory if it doesn't exist
	if err := os.MkdirAll(serviceDir, 0755); err != nil {
		fmt.Fprintf(os.Stderr, "Error: Cannot create directory %s: %v\n", serviceDir, err)
		os.Exit(1)
	}

	// Check if service already exists
	if _, err := os.Stat(servicePath); err == nil {
		fmt.Printf("Service already installed at %s\n", servicePath)
		fmt.Println("Use 'goclaw gateway uninstall' first to remove it")
		os.Exit(1)
	}

	// Get working directory
	workDir := filepath.Dir(execPath)

	// Create systemd service content
	serviceContent := `[Unit]
Description=goclaw Gateway Service
After=network.target

[Service]
Type=simple
ExecStart={{.ExecPath}} gateway run --port {{.Port}}
WorkingDirectory={{.WorkDir}}
Restart=always
RestartSec=10
StandardOutput=append:{{.HomeDir}}/.goclaw/logs/gateway.stdout.log
StandardError=append:{{.HomeDir}}/.goclaw/logs/gateway.stderr.log

[Install]
WantedBy=default.target
`

	// Create template
	tmpl, err := template.New("service").Parse(serviceContent)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: Cannot create service template: %v\n", err)
		os.Exit(1)
	}

	// Ensure log directory exists
	logDir := filepath.Join(homeDir, ".goclaw/logs")
	if err := os.MkdirAll(logDir, 0755); err != nil {
		fmt.Fprintf(os.Stderr, "Error: Cannot create log directory %s: %v\n", logDir, err)
		os.Exit(1)
	}

	// Execute template
	serviceFile, err := os.Create(servicePath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: Cannot create service file %s: %v\n", servicePath, err)
		os.Exit(1)
	}
	defer serviceFile.Close()

	data := struct {
		ExecPath string
		WorkDir  string
		HomeDir  string
		Port     int
	}{
		ExecPath: execPath,
		WorkDir:  workDir,
		HomeDir:  homeDir,
		Port:     gatewayPort,
	}

	if err := tmpl.Execute(serviceFile, data); err != nil {
		fmt.Fprintf(os.Stderr, "Error: Cannot write service file: %v\n", err)
		os.Exit(1)
	}

	// Reload systemd daemon
	fmt.Println("Reloading systemd daemon...")
	if err := exec.Command("systemctl", "--user", "daemon-reload").Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: Cannot reload systemd daemon: %v\n", err)
		fmt.Println("You may need to run: systemctl --user daemon-reload")
	}

	// Enable the service
	fmt.Println("Enabling service...")
	if err := exec.Command("systemctl", "--user", "enable", serviceName).Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: Cannot enable service: %v\n", err)
		fmt.Println("You may need to run: systemctl --user enable", serviceName)
	}

	fmt.Printf("Gateway service installed successfully\n")
	fmt.Printf("  Config: %s\n", servicePath)
	fmt.Printf("  Executable: %s\n", execPath)
	fmt.Printf("  Port: %d\n", gatewayPort)
	fmt.Printf("  Logs: %s/gateway.stdout.log\n", logDir)
	fmt.Printf("  Logs: %s/gateway.stderr.log\n", logDir)
	fmt.Println("\nUse 'goclaw gateway start' to start the service")
}

func uninstallLinuxService() {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: Cannot get home directory: %v\n", err)
		os.Exit(1)
	}

	servicePath := filepath.Join(homeDir, linuxServiceDir, linuxServiceFile)

	// Check if service exists
	if _, err := os.Stat(servicePath); os.IsNotExist(err) {
		fmt.Println("Service not installed")
		return
	}

	// Stop and disable the service
	fmt.Println("Stopping service...")
	exec.Command("systemctl", "--user", "stop", serviceName).Run() // nolint:errcheck

	fmt.Println("Disabling service...")
	exec.Command("systemctl", "--user", "disable", serviceName).Run() // nolint:errcheck

	// Remove the service file
	fmt.Println("Removing service configuration...")
	if err := os.Remove(servicePath); err != nil {
		fmt.Fprintf(os.Stderr, "Error: Cannot remove service file: %v\n", err)
		os.Exit(1)
	}

	// Reload systemd daemon
	fmt.Println("Reloading systemd daemon...")
	exec.Command("systemctl", "--user", "daemon-reload").Run() // nolint:errcheck

	fmt.Println("Gateway service uninstalled successfully")
}

func startLinuxService() {
	// Check if service file exists
	homeDir, _ := os.UserHomeDir()
	servicePath := filepath.Join(homeDir, linuxServiceDir, linuxServiceFile)

	if _, err := os.Stat(servicePath); os.IsNotExist(err) {
		fmt.Println("Service not installed. Use 'goclaw gateway install' first")
		os.Exit(1)
	}

	// Start the service
	fmt.Println("Starting service...")
	cmd := exec.Command("systemctl", "--user", "start", serviceName)
	output, err := cmd.CombinedOutput()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: Cannot start service: %v\n", err)
		if len(output) > 0 {
			fmt.Fprintf(os.Stderr, "Output: %s\n", string(output))
		}
		os.Exit(1)
	}

	// Check if it's actually running
	time.Sleep(500 * time.Millisecond)
	if checkGatewayRunning() {
		fmt.Println("Gateway service started successfully")
	} else {
		fmt.Println("Service started, but gateway may not be responding yet")
		fmt.Println("Check with: systemctl --user status", serviceName)
	}
}

func stopLinuxService() {
	// Stop the service
	fmt.Println("Stopping service...")
	cmd := exec.Command("systemctl", "--user", "stop", serviceName)
	output, err := cmd.CombinedOutput()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: Cannot stop service: %v\n", err)
		if len(output) > 0 {
			fmt.Fprintf(os.Stderr, "Output: %s\n", string(output))
		}
		os.Exit(1)
	}

	fmt.Println("Gateway service stopped successfully")
}

func restartLinuxService() {
	fmt.Println("Restarting service...")
	cmd := exec.Command("systemctl", "--user", "restart", serviceName)
	output, err := cmd.CombinedOutput()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: Cannot restart service: %v\n", err)
		if len(output) > 0 {
			fmt.Fprintf(os.Stderr, "Output: %s\n", string(output))
		}
		os.Exit(1)
	}

	// Check if it's actually running
	time.Sleep(500 * time.Millisecond)
	if checkGatewayRunning() {
		fmt.Println("Gateway service restarted successfully")
	} else {
		fmt.Println("Service restarted, but gateway may not be responding yet")
		fmt.Println("Check with: systemctl --user status", serviceName)
	}
}

// Windows service functions

func installWindowsService(execPath string) {
	// Check if sc.exe exists
	if _, err := exec.LookPath("sc.exe"); err != nil {
		fmt.Fprintf(os.Stderr, "Error: sc.exe not found. This command requires administrator privileges.\n")
		fmt.Println("Please run Command Prompt as Administrator and try again.")
		os.Exit(1)
	}

	// Check if service already exists
	checkCmd := exec.Command("sc.exe", "query", windowsServiceName)
	if output, err := checkCmd.CombinedOutput(); err == nil {
		if strings.Contains(string(output), windowsServiceName) {
			fmt.Printf("Service already installed: %s\n", windowsServiceName)
			fmt.Println("Use 'goclaw gateway uninstall' first to remove it")
			os.Exit(1)
		}
	}

	// Create the service
	fmt.Printf("Creating service: %s\n", windowsServiceName)
	createCmd := exec.Command("sc.exe", "create", windowsServiceName,
		"binPath=", "\""+execPath+"\" gateway run --port "+fmt.Sprint(gatewayPort),
		"DisplayName= GoClaw Gateway",
		"start= auto")

	output, err := createCmd.CombinedOutput()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: Cannot create service: %v\n", err)
		if len(output) > 0 {
			fmt.Fprintf(os.Stderr, "Output: %s\n", string(output))
		}
		fmt.Println("\nNote: This command requires administrator privileges.")
		fmt.Println("Please run Command Prompt as Administrator.")
		os.Exit(1)
	}

	// Set service description
	descCmd := exec.Command("sc.exe", "description", windowsServiceName,
		"GoClaw WebSocket Gateway Service")
	descCmd.Run() // nolint:errcheck

	fmt.Printf("Gateway service installed successfully\n")
	fmt.Printf("  Service Name: %s\n", windowsServiceName)
	fmt.Printf("  Executable: %s\n", execPath)
	fmt.Printf("  Port: %d\n", gatewayPort)
	fmt.Println("\nUse 'goclaw gateway start' to start the service")
	fmt.Println("Or use: sc start", windowsServiceName)
}

func uninstallWindowsService() {
	// Check if sc.exe exists
	if _, err := exec.LookPath("sc.exe"); err != nil {
		fmt.Fprintf(os.Stderr, "Error: sc.exe not found.\n")
		os.Exit(1)
	}

	// Check if service exists
	checkCmd := exec.Command("sc.exe", "query", windowsServiceName)
	output, err := checkCmd.CombinedOutput()
	if err != nil || !strings.Contains(string(output), windowsServiceName) {
		fmt.Println("Service not installed")
		return
	}

	// Stop the service if running
	fmt.Println("Stopping service...")
	exec.Command("sc.exe", "stop", windowsServiceName).Run() // nolint:errcheck

	// Wait a bit for the service to stop
	time.Sleep(2 * time.Second)

	// Delete the service
	fmt.Println("Deleting service...")
	deleteCmd := exec.Command("sc.exe", "delete", windowsServiceName)
	output, err = deleteCmd.CombinedOutput()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: Cannot delete service: %v\n", err)
		if len(output) > 0 {
			fmt.Fprintf(os.Stderr, "Output: %s\n", string(output))
		}
		os.Exit(1)
	}

	fmt.Println("Gateway service uninstalled successfully")
}

func startWindowsService() {
	// Start the service
	fmt.Printf("Starting service: %s\n", windowsServiceName)
	cmd := exec.Command("sc.exe", "start", windowsServiceName)
	output, err := cmd.CombinedOutput()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: Cannot start service: %v\n", err)
		if len(output) > 0 {
			fmt.Fprintf(os.Stderr, "Output: %s\n", string(output))
		}
		os.Exit(1)
	}

	// Check if it's actually running
	time.Sleep(1 * time.Second)
	if checkGatewayRunning() {
		fmt.Println("Gateway service started successfully")
	} else {
		fmt.Println("Service started, but gateway may not be responding yet")
		fmt.Println("Check with: sc query", windowsServiceName)
	}
}

// stopGatewayProcessByPort finds the process listening on the gateway port and kills it.
// Used when gateway was started with "gateway run" (not as service). Windows only.
func stopGatewayProcessByPort(port int) bool {
	if port == 0 {
		port = 28789
	}
	cmd := exec.Command("netstat", "-ano")
	output, err := cmd.Output()
	if err != nil {
		return false
	}
	portStr := fmt.Sprintf(":%d", port)
	lines := strings.Split(string(output), "\n")
	for _, line := range lines {
		if !strings.Contains(line, "LISTENING") || !strings.Contains(line, portStr) {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 5 {
			continue
		}
		pidStr := fields[len(fields)-1]
		var pid int
		if _, err := fmt.Sscanf(pidStr, "%d", &pid); err != nil {
			continue
		}
		if pid <= 0 {
			continue
		}
		killCmd := exec.Command("taskkill", "/PID", fmt.Sprint(pid), "/F")
		if killErr := killCmd.Run(); killErr != nil {
			continue
		}
		return true
	}
	return false
}

func stopWindowsService() {
	// Check if service exists first (avoid confusing 1060 "service not installed")
	checkCmd := exec.Command("sc.exe", "query", windowsServiceName)
	checkOut, checkErr := checkCmd.CombinedOutput()
	if checkErr != nil || !strings.Contains(string(checkOut), windowsServiceName) {
		// Not a service: try to stop process started with "gateway run" (by port)
		ports := []int{gatewayPort}
		if gatewayPort == 0 {
			ports = []int{28789, 28790, 28791}
		}
		fmt.Println("Gateway is not installed as a Windows service.")
		for _, port := range ports {
			if port == 0 {
				continue
			}
			if stopGatewayProcessByPort(port) {
				fmt.Printf("Stopped gateway process (was listening on port %d).\n", port)
				return
			}
		}
		fmt.Println("No gateway process found on port(s)", ports)
		fmt.Println("If you started gateway with 'goclaw gateway run', stop it with Ctrl+C in that terminal,")
		fmt.Println("or end the goclaw.exe process in Task Manager.")
		fmt.Println("\nTo run as a service: goclaw gateway install")
		os.Exit(1)
	}

	// Stop the service
	fmt.Printf("Stopping service: %s\n", windowsServiceName)
	cmd := exec.Command("sc.exe", "stop", windowsServiceName)
	output, err := cmd.CombinedOutput()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: Cannot stop service: %v\n", err)
		if len(output) > 0 {
			fmt.Fprintf(os.Stderr, "Output: %s\n", string(output))
		}
		os.Exit(1)
	}

	fmt.Println("Gateway service stopped successfully")
}

func restartWindowsService() {
	fmt.Println("Restarting service...")
	stopWindowsService()
	time.Sleep(2 * time.Second)
	startWindowsService()
}

// checkGatewayRunning checks if the gateway is responding
func checkGatewayRunning() bool {
	url := fmt.Sprintf("http://localhost:%d/health", gatewayPort)
	if gatewayPort == 0 {
		url = "http://localhost:28789/health"
	}

	client := &http.Client{Timeout: 2 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return false
	}
	defer resp.Body.Close()

	return resp.StatusCode == http.StatusOK
}

// runGatewayCall makes an RPC call to gateway
func runGatewayCall(cmd *cobra.Command, args []string) {
	method := args[0]

	// Parse params
	var params map[string]interface{}
	if gatewayParams != "" {
		if err := json.Unmarshal([]byte(gatewayParams), &params); err != nil {
			fmt.Fprintf(os.Stderr, "Invalid params JSON: %v\n", err)
			os.Exit(1)
		}
	}

	// Create request
	request := map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      "1",
		"method":  method,
		"params":  params,
	}

	requestBody, _ := json.Marshal(request)

	// For WebSocket, we need a different approach
	fmt.Printf("Calling method: %s\n", method)
	fmt.Printf("Request: %s\n", string(requestBody))
	fmt.Println("\nNote: RPC calls require WebSocket connection")
	fmt.Println("Use the WebSocket API to call methods directly")
}

// runGatewayReload reloads gateway configuration
func runGatewayReload(cmd *cobra.Command, args []string) {
	fmt.Println("Reloading gateway configuration...")

	// Load configuration
	cfg, err := config.Load("")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to load config: %v\n", err)
		os.Exit(1)
	}

	configFile := config.ConfigFileUsed()
	if configFile == "" {
		fmt.Println("No config file in use, cannot reload")
		os.Exit(1)
	}

	fmt.Printf("Config file: %s\n", configFile)

	// Validate configuration
	if err := config.Validate(cfg); err != nil {
		fmt.Fprintf(os.Stderr, "Invalid configuration: %v\n", err)
		os.Exit(1)
	}

	// Update global config
	config.Set(cfg)

	fmt.Println("Configuration reloaded successfully")
	fmt.Println("\nNote: If gateway is running as a service, restart it to apply changes:")
	fmt.Println("  goclaw gateway restart")
}

// runGatewayHistory shows configuration change history
func runGatewayHistory(cmd *cobra.Command, args []string) {
	fmt.Println("Configuration Change History")
	fmt.Println("============================")
	fmt.Println()

	history := config.GetHistory(20) // 显示最近 20 条记录

	if len(history) == 0 {
		fmt.Println("No configuration changes recorded yet.")
		return
	}

	for i, change := range history {
		fmt.Printf("[%d] %s\n", i, change.Timestamp.Format("2006-01-02 15:04:05"))
		fmt.Printf("    Triggered by: %s\n", change.TriggeredBy)
		fmt.Printf("    Success: %v\n", change.Success)

		if change.Error != "" {
			fmt.Printf("    Error: %s\n", change.Error)
		}

		if len(change.Changes) > 0 {
			fmt.Println("    Changes:")
			for key, value := range change.Changes {
				if changeMap, ok := value.(map[string]interface{}); ok {
					fmt.Printf("      %s: %v -> %v\n", key, changeMap["old"], changeMap["new"])
				}
			}
		}

		fmt.Println()
	}

	fmt.Printf("Total: %d changes\n", len(history))
	fmt.Println("\nUse 'goclaw gateway rollback <index>' to rollback to a previous configuration")
}

// runGatewayRollback rollbacks to a previous configuration
func runGatewayRollback(cmd *cobra.Command, args []string) {
	if len(args) == 0 {
		// 回滚到最近一次成功的配置
		fmt.Println("Rolling back to latest successful configuration...")

		if err := config.RollbackToLatest(); err != nil {
			fmt.Fprintf(os.Stderr, "Failed to rollback: %v\n", err)
			os.Exit(1)
		}

		fmt.Println("Successfully rolled back to latest configuration")
	} else {
		// 回滚到指定索引的配置
		var index int
		if _, err := fmt.Sscanf(args[0], "%d", &index); err != nil {
			fmt.Fprintf(os.Stderr, "Invalid index: %s\n", args[0])
			os.Exit(1)
		}

		fmt.Printf("Rolling back to configuration at index %d...\n", index)

		if err := config.RollbackConfig(index); err != nil {
			fmt.Fprintf(os.Stderr, "Failed to rollback: %v\n", err)
			os.Exit(1)
		}

		fmt.Printf("Successfully rolled back to configuration at index %d\n", index)
	}

	fmt.Println("\nNote: If gateway is running, restart it to apply changes:")
	fmt.Println("  goclaw gateway restart")
}
