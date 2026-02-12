package skills

import (
	"strings"
	"testing"
)

func TestFindInstallSpec(t *testing.T) {
	entry := &SkillEntry{
		Skill: &Skill{
			Name: "test-skill",
		},
		Metadata: &OpenClawSkillMetadata{
			Install: []SkillInstallSpec{
				{
					Kind:    "brew",
					Formula: "git",
					Bins:    []string{"git"},
				},
				{
					ID:      "custom-id",
					Kind:    "node",
					Package: "typescript",
					Bins:    []string{"tsc"},
				},
			},
		},
	}

	tests := []struct {
		name     string
		installID string
		kind     string
		found    bool
	}{
		{
			name:     "index default",
			installID: "brew-0",
			kind:     "brew",
			found:    true,
		},
		{
			name:     "custom ID",
			installID: "custom-id",
			kind:     "node",
			found:    true,
		},
		{
			name:     "not found",
			installID: "nonexistent",
			kind:     "",
			found:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			spec := FindInstallSpec(entry, tt.installID)
			if tt.found {
				if spec == nil {
					t.Fatal("expected spec to be found")
				}
				if spec.Kind != tt.kind {
					t.Errorf("expected kind '%s', got '%s'", tt.kind, spec.Kind)
				}
			} else {
				if spec != nil {
					t.Error("expected spec to be nil")
				}
			}
		})
	}
}

func TestResolveInstallPreferences(t *testing.T) {
	t.Run("nil config", func(t *testing.T) {
		prefs := ResolveInstallPreferences(nil)
		if !prefs.PreferBrew {
			t.Error("expected PreferBrew to be true by default")
		}
		if prefs.NodeManager != "npm" {
			t.Errorf("expected NodeManager 'npm', got '%s'", prefs.NodeManager)
		}
	})

	t.Run("with config", func(t *testing.T) {
		config := &SkillsConfig{
			Install: InstallConfig{
				PreferBrew:  false,
				NodeManager: "pnpm",
			},
		}
		prefs := ResolveInstallPreferences(config)
		if prefs.PreferBrew {
			t.Error("expected PreferBrew to be false")
		}
		if prefs.NodeManager != "pnpm" {
			t.Errorf("expected NodeManager 'pnpm', got '%s'", prefs.NodeManager)
		}
	})
}

func TestGetInstaller(t *testing.T) {
	tests := []struct {
		name     string
		spec     SkillInstallSpec
		prefs    InstallConfig
		found    bool
	}{
		{
			name: "brew installer",
			spec: SkillInstallSpec{
				Kind:    "brew",
				Formula: "test",
			},
			prefs: InstallConfig{},
			found: true,
		},
		{
			name: "node installer",
			spec: SkillInstallSpec{
				Kind:    "node",
				Package: "test",
			},
			prefs: InstallConfig{NodeManager: "npm"},
			found: true,
		},
		{
			name: "go installer",
			spec: SkillInstallSpec{
				Kind:   "go",
				Module: "test/module",
			},
			prefs: InstallConfig{},
			found: true,
		},
		{
			name: "uv installer",
			spec: SkillInstallSpec{
				Kind:    "uv",
				Package: "test-package",
			},
			prefs: InstallConfig{},
			found: true,
		},
		{
			name: "download installer",
			spec: SkillInstallSpec{
				Kind: "download",
				URL:  "https://example.com/file.gz",
			},
			prefs: InstallConfig{},
			found: true,
		},
		{
			name:  "unsupported installer",
			spec:  SkillInstallSpec{Kind: "invalid"},
			prefs: InstallConfig{},
			found: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			installer, err := GetInstaller(&tt.spec, tt.prefs)
			if tt.found {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
				if installer == nil {
					t.Error("expected installer to be returned")
				}
			} else {
				if err == nil {
					t.Error("expected error for unsupported installer")
				}
			}
		})
	}
}

func TestWithWarnings(t *testing.T) {
	result := InstallResult{
		Success: true,
		Message: "Success",
	}

	warnings := []string{"Warning 1", "Warning 2"}

	resultWithWarnings := WithWarnings(result, warnings)

	if resultWithWarnings.Success != result.Success {
		t.Error("Success should be preserved")
	}
	if resultWithWarnings.Warnings == nil {
		t.Error("Warnings should be set")
	}
	if len(resultWithWarnings.Warnings) != 2 {
		t.Errorf("expected 2 warnings, got %d", len(resultWithWarnings.Warnings))
	}
}

func TestSummarizeInstallOutput(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "empty",
			input:    "",
			expected: "",
		},
		{
			name:     "single line",
			input:    "Single line output",
			expected: "Single line output",
		},
		{
			name: "error line",
			input: `Line 1
Error: something went wrong
Line 3`,
			expected: "Error: something went wrong",
		},
		{
			name: "failure keyword",
			input: `Line 1
failed to install
Line 3`,
			expected: "failed to install",
		},
		{
			name: "last line for normal output",
			input: `Line 1
Line 2
Success`,
			expected: "Success",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := SummarizeInstallOutput(tt.input)
			if result != tt.expected {
				t.Errorf("expected '%s', got '%s'", tt.expected, result)
			}
		})
	}
}

func TestFormatInstallFailureMessage(t *testing.T) {
	tests := []struct {
		name     string
		stdout   string
		stderr   string
		code     *int
		contains string
	}{
		{
			name:     "with error code",
			stdout:   "",
			stderr:   "error message",
			code:     intPtr(1),
			contains: "exit 1",
		},
		{
			name:     "with stderr",
			stdout:   "",
			stderr:   "Error: something failed",
			code:     intPtr(1),
			contains: "something failed",
		},
		{
			name:     "no code",
			stdout:   "some output",
			stderr:   "",
			code:     nil,
			contains: "unknown exit",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			message := FormatInstallFailureMessage(tt.stdout, tt.stderr, tt.code)
			if !stringContains(message, tt.contains) {
				t.Errorf("expected message to contain '%s', got '%s'", tt.contains, message)
			}
		})
	}
}

// Test installers

func TestBrewInstaller_CanInstall(t *testing.T) {
	installer := &BrewInstaller{}

	t.Run("valid brew spec", func(t *testing.T) {
		spec := &SkillInstallSpec{
			Kind:    "brew",
			Formula: "git",
		}
		if HasBinary("brew") {
			if !installer.CanInstall(spec) {
				t.Error("brew spec should be installable when brew is available")
			}
		}
	})

	t.Run("non-brew spec", func(t *testing.T) {
		spec := &SkillInstallSpec{
			Kind: "node",
		}
		if installer.CanInstall(spec) {
			t.Error("non-brew spec should not be installable by brew installer")
		}
	})

	t.Run("missing formula", func(t *testing.T) {
		spec := &SkillInstallSpec{
			Kind: "brew",
		}
		if installer.CanInstall(spec) {
			t.Error("brew spec without formula should not be installable")
		}
	})
}

func TestDownloadInstaller_CanInstall(t *testing.T) {
	installer := &DownloadInstaller{}

	t.Run("valid download spec", func(t *testing.T) {
		spec := &SkillInstallSpec{
			Kind: "download",
			URL:  "https://example.com/file.tar.gz",
		}
		if !installer.CanInstall(spec) {
			t.Error("download spec with URL should be installable")
		}
	})

	t.Run("missing URL", func(t *testing.T) {
		spec := &SkillInstallSpec{
			Kind: "download",
		}
		if installer.CanInstall(spec) {
			t.Error("download spec without URL should not be installable")
		}
	})

	t.Run("non-download spec", func(t *testing.T) {
		spec := &SkillInstallSpec{
			Kind: "brew",
		}
		if installer.CanInstall(spec) {
			t.Error("non-download spec should not be installable by download installer")
		}
	})
}

func TestDetectInstalledBinaries(t *testing.T) {
	t.Run("common binaries", func(t *testing.T) {
		bins := []string{"go", "node", "bash"}
		installed := detectInstalledBinaries(bins)

		// Check that all detected binaries are valid
		for _, bin := range installed {
			if !HasBinary(bin) {
				t.Errorf("binary '%s' reported as installed but not found", bin)
			}
		}
	})

	t.Run("nonexistent binaries", func(t *testing.T) {
		bins := []string{"this-binary-definitely-does-not-exist-12345"}
		installed := detectInstalledBinaries(bins)

		if len(installed) > 0 {
			t.Error("nonexistent binaries should not be detected as installed")
		}
	})
}

// Helper functions
func intPtr(i int) *int {
	return &i
}

func stringContains(s, substr string) bool {
	return strings.Contains(s, substr)
}
