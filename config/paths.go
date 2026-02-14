package config

import (
	"fmt"
	"os"
	"path/filepath"
)

// GetDefaultDataDir returns the default data directory path for goclaw (~/.goclaw).
func GetDefaultDataDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("failed to get home directory: %w", err)
	}
	return filepath.Join(home, ".goclaw"), nil
}

// GetAgentsDir returns the agents directory path
func GetAgentsDir(dataDir string) string {
	return filepath.Join(dataDir, "agents")
}

// GetMainAgentDir returns the main agent directory
func GetMainAgentDir(dataDir string) string {
	return filepath.Join(GetAgentsDir(dataDir), "main")
}

// GetAgentDir returns a specific agent's directory
func GetAgentDir(dataDir, agentID string) string {
	return filepath.Join(GetAgentsDir(dataDir), agentID)
}

// GetSessionsDir returns the sessions directory for an agent
func GetSessionsDir(dataDir, agentID string) string {
	return filepath.Join(GetAgentDir(dataDir, agentID), "sessions")
}

// GetWorkspaceDir returns the workspace directory
func GetWorkspaceDir(dataDir string) string {
	return filepath.Join(dataDir, "workspace")
}

// GetBrowserDir returns the browser data directory
func GetBrowserDir(dataDir string) string {
	return filepath.Join(dataDir, "browser")
}

// GetMediaDir returns the media directory
func GetMediaDir(dataDir string) string {
	return filepath.Join(dataDir, "media")
}

// GetInboundMediaDir returns the inbound media directory
func GetInboundMediaDir(dataDir string) string {
	return filepath.Join(GetMediaDir(dataDir), "inbound")
}

// GetIdentityDir returns the identity directory
func GetIdentityDir(dataDir string) string {
	return filepath.Join(dataDir, "identity")
}

// GetDevicesDir returns the devices directory
func GetDevicesDir(dataDir string) string {
	return filepath.Join(dataDir, "devices")
}

// GetCredentialsDir returns the credentials directory
func GetCredentialsDir(dataDir string) string {
	return filepath.Join(dataDir, "credentials")
}

// GetSkillsDir returns the skills directory
func GetSkillsDir(dataDir string) string {
	return filepath.Join(dataDir, "skills")
}

// GetSubagentsDir returns the subagents directory
func GetSubagentsDir(dataDir string) string {
	return filepath.Join(dataDir, "subagents")
}

// GetCronDir returns the cron directory
func GetCronDir(dataDir string) string {
	return filepath.Join(dataDir, "cron")
}

// GetCanvasDir returns the canvas directory
func GetCanvasDir(dataDir string) string {
	return filepath.Join(dataDir, "canvas")
}

// GetCompletionsDir returns the completions cache directory
func GetCompletionsDir(dataDir string) string {
	return filepath.Join(dataDir, "completions")
}

// EnsureDataDirs creates all necessary data directories
func EnsureDataDirs(dataDir string) error {
	dirs := []string{
		dataDir,
		GetAgentsDir(dataDir),
		GetMainAgentDir(dataDir),
		GetSessionsDir(dataDir, "main"),
		GetWorkspaceDir(dataDir),
		GetBrowserDir(dataDir),
		GetMediaDir(dataDir),
		GetInboundMediaDir(dataDir),
		GetIdentityDir(dataDir),
		GetDevicesDir(dataDir),
		GetCredentialsDir(dataDir),
		GetSkillsDir(dataDir),
		GetSubagentsDir(dataDir),
		GetCronDir(dataDir),
		GetCanvasDir(dataDir),
		GetCompletionsDir(dataDir),
	}

	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("failed to create directory %s: %w", dir, err)
		}
	}

	return nil
}

// GetConfigFilePath returns the main config file path (config.json under dataDir).
func GetConfigFilePath(dataDir string) string {
	return filepath.Join(dataDir, "config.json")
}

// GetSessionIndexPath returns the session index file path
func GetSessionIndexPath(dataDir, agentID string) string {
	return filepath.Join(GetSessionsDir(dataDir, agentID), "sessions.json")
}

// GetSubagentRunsPath returns the subagent runs file path
func GetSubagentRunsPath(dataDir string) string {
	return filepath.Join(GetSubagentsDir(dataDir), "runs.json")
}

// GetDeviceIdentityPath returns the device identity file path
func GetDeviceIdentityPath(dataDir string) string {
	return filepath.Join(GetIdentityDir(dataDir), "device.json")
}

// GetDeviceAuthPath returns the device auth file path
func GetDeviceAuthPath(dataDir string) string {
	return filepath.Join(GetIdentityDir(dataDir), "device-auth.json")
}

// GetPairedDevicesPath returns the paired devices file path
func GetPairedDevicesPath(dataDir string) string {
	return filepath.Join(GetDevicesDir(dataDir), "paired.json")
}

// GetPendingDevicesPath returns the pending devices file path
func GetPendingDevicesPath(dataDir string) string {
	return filepath.Join(GetDevicesDir(dataDir), "pending.json")
}
