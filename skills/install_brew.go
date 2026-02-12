package skills

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// BrewInstaller implements Homebrew-based installations
type BrewInstaller struct {
	// Executable to use (empty to use "brew" on PATH)
	Executable string
}

// CanInstall checks if brew is available
func (b *BrewInstaller) CanInstall(spec *SkillInstallSpec) bool {
	if spec.Kind != "brew" {
		return false
	}
	brewExe := b.getExecutable()
	if brewExe == "" {
		return false
	}
	// Check if formula is specified
	return spec.Formula != ""
}

// Install installs a package using Homebrew
func (b *BrewInstaller) Install(ctx context.Context, spec *SkillInstallSpec) InstallResult {
	brewExe := b.getExecutable()
	if brewExe == "" {
		return InstallResult{
			Success: false,
			Message: "brew not installed",
		}
	}

	// Validate formula
	if spec.Formula == "" {
		return InstallResult{
			Success: false,
			Message: "missing brew formula",
		}
	}

	// Check if already installed
	if !spec.Extract {
		isInstalled := b.isInstalled(ctx, brewExe, spec.Formula)
		if isInstalled {
			return InstallResult{
				Success: true,
				Message: fmt.Sprintf("Already installed: %s", spec.Formula),
			}
		}
	}

	// Build command
	argv := []string{brewExe, "install", spec.Formula}

	// Run installation
	stdout, stderr, exitCode, _ := RunCommandWithTimeout(ctx, argv, nil)

	success := exitCode != nil && *exitCode == 0
	var message string
	if success {
		message = fmt.Sprintf("Installed: %s", spec.Formula)
	} else {
		message = FormatInstallFailureMessage(stdout, stderr, exitCode)
	}

	return InstallResult{
		Success:  success,
		Message:  message,
		Stdout:   stdout,
		Stderr:   stderr,
		ExitCode: exitCode,
	}
}

// getExecutable returns the brew executable path
func (b *BrewInstaller) getExecutable() string {
	// Use configured executable if set
	if b.Executable != "" {
		return b.Executable
	}

	// Check for brew on PATH
	if HasBinary("brew") {
		return "brew"
	}

	// Check common installation paths
	for _, path := range []string{
		"/opt/homebrew/bin/brew",
		"/usr/local/bin/brew",
		"/home/linuxbrew/.linuxbrew/bin/brew",
	} {
		if _, err := os.Stat(path); err == nil {
			return path
		}
	}

	return ""
}

// isInstalled checks if a brew formula is installed
func (b *BrewInstaller) isInstalled(ctx context.Context, brewExe, formula string) bool {
	argv := []string{brewExe, "list", formula}
	stdout, _, _, _ := RunCommandWithTimeout(ctx, argv, nil)
	return stdout != ""
}

// ResolveBrewBinDir resolves the Homebrew bin directory
func (b *BrewInstaller) ResolveBrewBinDir(ctx context.Context) (string, error) {
	brewExe := b.getExecutable()
	if brewExe == "" {
		return "", fmt.Errorf("brew not found")
	}

	// Try to get prefix using brew --prefix
	argv := []string{brewExe, "--prefix"}
	stdout, _, exitCode, _ := RunCommandWithTimeout(ctx, argv, nil)

	if exitCode != nil && *exitCode == 0 && stdout != "" {
		prefix := strings.TrimSpace(stdout)
		if prefix != "" {
			return filepath.Join(prefix, "bin"), nil
		}
	}

	// Fall back to environment variable
	if prefix := os.Getenv("HOMEBREW_PREFIX"); prefix != "" {
		return filepath.Join(prefix, "bin"), nil
	}

	// Fall back to common paths
	for _, prefix := range []string{"/opt/homebrew", "/usr/local", "/home/linuxbrew/.linuxbrew"} {
		if _, err := os.Stat(filepath.Join(prefix, "bin", "brew")); err == nil {
			return filepath.Join(prefix, "bin"), nil
		}
	}

	return "", fmt.Errorf("failed to resolve brew bin directory")
}
