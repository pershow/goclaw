package skills

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// UVInstaller implements UV/Python tool installations
type UVInstaller struct {
	// BrewInstaller is used to install uv if not available
	BrewInstaller *BrewInstaller
}

// CanInstall checks if uv is available or can be installed
func (u *UVInstaller) CanInstall(spec *SkillInstallSpec) bool {
	if spec.Kind != "uv" {
		return false
	}

	// Check if uv is available
	if HasBinary("uv") {
		return spec.Package != ""
	}

	// Check if brew is available to install uv
	if u.BrewInstaller == nil {
		u.BrewInstaller = &BrewInstaller{}
	}

	brewExe := u.BrewInstaller.getExecutable()
	return brewExe != "" && spec.Package != ""
}

// Install installs a package using uv
func (u *UVInstaller) Install(ctx context.Context, spec *SkillInstallSpec) InstallResult {
	// Ensure uv is available
	if !HasBinary("uv") {
		// Try to install uv via brew
		if u.BrewInstaller == nil {
			u.BrewInstaller = &BrewInstaller{}
		}
		brewExe := u.BrewInstaller.getExecutable()
		if brewExe == "" {
			return InstallResult{
				Success: false,
				Message: "uv not installed (install via: brew install uv)",
			}
		}

		// Install uv via brew
		argv := []string{brewExe, "install", "uv"}
		stdout, stderr, exitCode, err := RunCommandWithTimeout(ctx, argv, nil)
		if exitCode != nil && *exitCode != 0 {
			return InstallResult{
				Success:  false,
				Message:  "Failed to install uv via brew",
				Stdout:   stdout,
				Stderr:   stderr,
				ExitCode: exitCode,
			}
		} else if err != nil {
			return InstallResult{
				Success:  false,
				Message:  fmt.Sprintf("Failed to install uv: %v", err),
				Stdout:   stdout,
				Stderr:   stderr,
				ExitCode: exitCode,
			}
		}

		// Check if uv is now available
		if !HasBinary("uv") {
			return InstallResult{
				Success: false,
				Message: "uv installation completed but uv not found on PATH",
			}
		}
	}

	// Validate package
	if spec.Package == "" {
		return InstallResult{
			Success: false,
			Message: "missing uv package",
		}
	}

	// Check if already installed
	if !spec.Extract {
		toolName := u.extractToolName(spec.Package)
		if toolName != "" && u.isInstalled(ctx, toolName) {
			return InstallResult{
				Success: true,
				Message: fmt.Sprintf("Already installed: %s", spec.Package),
			}
		}
	}

	// Build command
	argv := []string{"uv", "tool", "install", spec.Package}

	// Run installation
	stdout, stderr, exitCode, _ := RunCommandWithTimeout(ctx, argv, nil)

	success := exitCode != nil && *exitCode == 0
	var message string
	if success {
		message = fmt.Sprintf("Installed: %s", spec.Package)
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

// extractToolName extracts the tool name from a package name
// E.g., "ruff" -> "ruff"
// E.g., "black" -> "black"
func (u *UVInstaller) extractToolName(pkg string) string {
	// Remove version spec if present
	if idx := strings.Index(pkg, " @ "); idx > 0 {
		pkg = pkg[:idx]
	}
	if idx := strings.Index(pkg, "=="); idx > 0 {
		pkg = pkg[:idx]
	}
	if idx := strings.Index(pkg, ">"); idx > 0 {
		pkg = pkg[:idx]
	}
	if idx := strings.Index(pkg, "<"); idx > 0 {
		pkg = pkg[:idx]
	}

	// Extract last component after /
	parts := strings.Split(pkg, "/")
	return parts[len(parts)-1]
}

// isInstalled checks if a uv tool is installed
func (u *UVInstaller) isInstalled(ctx context.Context, toolName string) bool {
	// Try to find the tool on PATH
	if HasBinary(toolName) {
		return true
	}

	// Check uv's tool directory
	argv := []string{"uv", "tool", "dir"}
	stdout, _, _, _ := RunCommandWithTimeout(ctx, argv, nil)
	if stdout != "" {
		toolDir := strings.TrimSpace(stdout)
		if toolDir != "" {
			path := filepath.Join(toolDir, toolName)
			if _, err := os.Stat(path); err == nil {
				return true
			}
		}
	}

	return false
}
