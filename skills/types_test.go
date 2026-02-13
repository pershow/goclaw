package skills

import (
	"testing"
)

func TestSkillLoadResult_Validation(t *testing.T) {
	t.Run("empty result", func(t *testing.T) {
		result := SkillLoadResult{}
		// In Go, slices default to nil which is idiomatic
		if result.Skills == nil {
			t.Log("Skills is nil (expected for empty result)")
		}
		if result.Diagnostics == nil {
			t.Log("Diagnostics is nil (expected for empty result)")
		}
		if result.Collisions == nil {
			t.Log("Collisions is nil (expected for empty result)")
		}
		// Test that we can use len() on nil slices safely
		if len(result.Skills) != 0 {
			t.Errorf("expected 0 skills, got %d", len(result.Skills))
		}
	})

	t.Run("with skills", func(t *testing.T) {
		result := SkillLoadResult{
			Skills: []*Skill{
				{
					Name:        "test-skill",
					Description: "A test skill",
					FilePath:    "/path/to/SKILL.md",
					BaseDir:     "/path/to",
					Source:      "workspace",
				},
			},
		}
		if len(result.Skills) != 1 {
			t.Errorf("expected 1 skill, got %d", len(result.Skills))
		}
		if result.Skills[0].Name != "test-skill" {
			t.Errorf("expected skill name 'test-skill', got '%s'", result.Skills[0].Name)
		}
	})
}

func TestDiagnostic_Validation(t *testing.T) {
	tests := []struct {
		name string
		diag Diagnostic
	}{
		{
			name: "warning diagnostic",
			diag: Diagnostic{
				Type:    DiagnosticTypeWarning,
				Message: "Test warning",
				Path:    "/path/to/file.md",
			},
		},
		{
			name: "error diagnostic",
			diag: Diagnostic{
				Type:    DiagnosticTypeError,
				Message: "Test error",
				Path:    "/path/to/file.md",
			},
		},
		{
			name: "collision diagnostic",
			diag: Diagnostic{
				Type:    DiagnosticTypeCollision,
				Message: "Test collision",
				Path:    "/path/to/file.md",
				Collision: &CollisionInfo{
					ResourceType: "skill",
					Name:         "test-skill",
					WinnerPath:   "/winner/skill.md",
					LoserPath:    "/loser/skill.md",
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.diag.Type == "" {
				t.Error("diagnostic type should not be empty")
			}
			if tt.diag.Message == "" {
				t.Error("diagnostic message should not be empty")
			}
			if tt.diag.Collision != nil {
				if tt.diag.Collision.Name == "" {
					t.Error("collision name should not be empty")
				}
				if tt.diag.Collision.WinnerPath == "" {
					t.Error("collision winner path should not be empty")
				}
				if tt.diag.Collision.LoserPath == "" {
					t.Error("collision loser path should not be empty")
				}
			}
		})
	}
}

func TestSkillInstallResult_Validation(t *testing.T) {
	t.Run("success result", func(t *testing.T) {
		result := SkillInstallResult{
			Success:       true,
			InstallID:     "brew-0",
			Message:       "Installed successfully",
			InstalledBins: []string{"git", "node"},
		}
		if !result.Success {
			t.Error("expected success to be true")
		}
		if len(result.InstalledBins) != 2 {
			t.Errorf("expected 2 installed bins, got %d", len(result.InstalledBins))
		}
	})

	t.Run("failure result", func(t *testing.T) {
		result := SkillInstallResult{
			Success: false,
			Message: "Installation failed",
		}
		if result.Success {
			t.Error("expected success to be false")
		}
	})
}

func TestSkillSourcePriority_Validation(t *testing.T) {
	tests := []struct {
		name     string
		source   string
		priority int
	}{
		{
			name:     "workspace source",
			source:   "workspace",
			priority: 100,
		},
		{
			name:     "managed source",
			source:   "managed",
			priority: 50,
		},
		{
			name:     "bundled source",
			source:   "bundled",
			priority: 10,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			skill := Skill{
				Name:   "test-skill",
				Source: tt.source,
			}
			if skill.Source != tt.source {
				t.Errorf("expected source '%s', got '%s'", tt.source, skill.Source)
			}
		})
	}
}
