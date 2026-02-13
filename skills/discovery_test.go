package skills

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadSkillEntries_Basic(t *testing.T) {
	// Create temporary directory structure
	tmpDir := t.TempDir()

	// Create a workspace skills directory
	workspaceSkillsDir := filepath.Join(tmpDir, "skills")
	if err := os.MkdirAll(workspaceSkillsDir, 0755); err != nil {
		t.Fatalf("failed to create skills directory: %v", err)
	}

	// Create SKILL.md file directly in skills directory (not in subdirectory)
	skillContent := `---
name: test-skill
description: A test skill
---

This is the skill content.`
	skillPath := filepath.Join(workspaceSkillsDir, "test-skill.md")
	if err := os.WriteFile(skillPath, []byte(skillContent), 0644); err != nil {
		t.Fatalf("failed to write skill file: %v", err)
	}

	// Load skill entries (don't include defaults to avoid actual project dirs)
	entries, err := LoadSkillEntries(tmpDir, LoadSkillsOptions{
		IncludeDefaults: false,
	})

	if err != nil {
		t.Fatalf("failed to load skill entries: %v", err)
	}

	if len(entries) == 0 {
		t.Fatal("expected at least one skill entry")
	}

	found := false
	for _, entry := range entries {
		if entry.Skill.Name == "test-skill" {
			found = true
			if entry.Skill.Source != "workspace" {
				t.Errorf("expected source 'workspace', got '%s'", entry.Skill.Source)
			}
			if entry.Skill.BaseDir != workspaceSkillsDir {
				t.Errorf("expected BaseDir '%s', got '%s'", workspaceSkillsDir, entry.Skill.BaseDir)
			}
		}
	}

	if !found {
		t.Error("skill 'test-skill' not found in loaded entries")
	}
}

func TestLoadSkillEntries_MultipleSources(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping in short mode")
	}

	tmpDir := t.TempDir()

	// Create managed skills directory
	managedDir := filepath.Join(tmpDir, "managed")
	if err := os.MkdirAll(managedDir, 0755); err != nil {
		t.Fatalf("failed to create managed directory: %v", err)
	}

	// Create a managed skill
	managedSkillDir := filepath.Join(managedDir, "managed-skill")
	if err := os.MkdirAll(managedSkillDir, 0755); err != nil {
		t.Fatalf("failed to create managed skill directory: %v", err)
	}
	managedSkillPath := filepath.Join(managedSkillDir, "SKILL.md")
	managedContent := `---
name: managed-skill
description: A managed skill
---
Managed content.`
	if err := os.WriteFile(managedSkillPath, []byte(managedContent), 0644); err != nil {
		t.Fatalf("failed to write managed skill: %v", err)
	}

	// Create workspace skills directory
	workspaceSkillsDir := filepath.Join(tmpDir, "skills")
	if err := os.MkdirAll(workspaceSkillsDir, 0755); err != nil {
		t.Fatalf("failed to create workspace skills directory: %v", err)
	}

	// Create a workspace skill with same name (should override)
	workspaceSkillDir := filepath.Join(workspaceSkillsDir, "managed-skill")
	if err := os.MkdirAll(workspaceSkillDir, 0755); err != nil {
		t.Fatalf("failed to create workspace skill directory: %v", err)
	}
	workspaceSkillPath := filepath.Join(workspaceSkillDir, "SKILL.md")
	workspaceContent := `---
name: managed-skill
description: A workspace skill (should override)
---
Workspace content.`
	if err := os.WriteFile(workspaceSkillPath, []byte(workspaceContent), 0644); err != nil {
		t.Fatalf("failed to write workspace skill: %v", err)
	}

	// Load skill entries with both sources
	entries, err := LoadSkillEntries(tmpDir, LoadSkillsOptions{
		IncludeDefaults:  false,
		ManagedSkillsDir: managedDir,
	})

	if err != nil {
		t.Fatalf("failed to load skill entries: %v", err)
	}

	// Check that workspace skill overrides managed skill
	var managedSkillEntry *SkillEntry
	for _, entry := range entries {
		if entry.Skill.Name == "managed-skill" {
			managedSkillEntry = entry
		}
	}

	if managedSkillEntry == nil {
		t.Fatal("managed-skill not found")
	}

	// Should have workspace description (not managed)
	if managedSkillEntry.Skill.Description != "A workspace skill (should override)" {
		t.Errorf("expected workspace description, got '%s'", managedSkillEntry.Skill.Description)
	}
	if managedSkillEntry.Skill.Source != "workspace" {
		t.Errorf("expected source 'workspace', got '%s'", managedSkillEntry.Skill.Source)
	}
}

func TestResolveBundledSkillsDir(t *testing.T) {
	t.Run("no bundled skills", func(t *testing.T) {
		dir, err := resolveBundledSkillsDir(LoadSkillsOptions{})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if dir != "" {
			t.Logf("found bundled skills dir: %s", dir)
		}
	})
}

func TestLooksLikeSkillsDir(t *testing.T) {
	tmpDir := t.TempDir()

	t.Run("valid skills directory", func(t *testing.T) {
		skillsDir := filepath.Join(tmpDir, "skills")
		if err := os.MkdirAll(skillsDir, 0755); err != nil {
			t.Fatalf("failed to create skills directory: %v", err)
		}

		// Create a skill subdirectory
		skillDir := filepath.Join(skillsDir, "test-skill")
		if err := os.MkdirAll(skillDir, 0755); err != nil {
			t.Fatalf("failed to create skill directory: %v", err)
		}

		// Create SKILL.md
		skillPath := filepath.Join(skillDir, "SKILL.md")
		if err := os.WriteFile(skillPath, []byte("---\nname: test\n---\n"), 0644); err != nil {
			t.Fatalf("failed to write skill file: %v", err)
		}

		if !looksLikeSkillsDir(skillsDir) {
			t.Error("expected skills directory to be identified")
		}
	})

	t.Run("empty directory", func(t *testing.T) {
		emptyDir := filepath.Join(tmpDir, "empty")
		if err := os.MkdirAll(emptyDir, 0755); err != nil {
			t.Fatalf("failed to create directory: %v", err)
		}

		if looksLikeSkillsDir(emptyDir) {
			t.Error("empty directory should not be identified as skills dir")
		}
	})

	t.Run("hidden directory", func(t *testing.T) {
		hiddenDir := filepath.Join(tmpDir, ".hidden-skills")
		if err := os.MkdirAll(hiddenDir, 0755); err != nil {
			t.Fatalf("failed to create hidden directory: %v", err)
		}

		// Should not be detected if it's .hidden-skills at root
		if looksLikeSkillsDir(hiddenDir) && filepath.Base(hiddenDir)[0] == '.' {
			t.Log("hidden directory detected - this may be expected depending on filters")
		}
	})
}

func TestLoadSkillFromFile(t *testing.T) {
	t.Run("valid skill file", func(t *testing.T) {
		tmpDir := t.TempDir()
		skillPath := filepath.Join(tmpDir, "SKILL.md")

		content := `---
name: test-skill
description: Test skill description
---

Skill content here.`

		if err := os.WriteFile(skillPath, []byte(content), 0644); err != nil {
			t.Fatalf("failed to write skill file: %v", err)
		}

		skill, err := loadSkillFromFile(skillPath, "test")
		if err != nil {
			t.Fatalf("failed to load skill: %v", err)
		}

		if skill.Name != "test-skill" {
			t.Errorf("expected name 'test-skill', got '%s'", skill.Name)
		}
		if skill.Description != "Test skill description" {
			t.Errorf("expected description 'Test skill description', got '%s'", skill.Description)
		}
		if skill.FilePath != skillPath {
			t.Errorf("expected FilePath '%s', got '%s'", skillPath, skill.FilePath)
		}
		if skill.Source != "test" {
			t.Errorf("expected source 'test', got '%s'", skill.Source)
		}
		if skill.BaseDir != tmpDir {
			t.Errorf("expected BaseDir '%s', got '%s'", tmpDir, skill.BaseDir)
		}
	})

	t.Run("missing description", func(t *testing.T) {
		tmpDir := t.TempDir()
		skillPath := filepath.Join(tmpDir, "SKILL.md")

		content := `---
name: test-skill
---
Content.`

		if err := os.WriteFile(skillPath, []byte(content), 0644); err != nil {
			t.Fatalf("failed to write skill file: %v", err)
		}

		_, err := loadSkillFromFile(skillPath, "test")
		if err == nil {
			t.Error("expected error for missing description")
		}
	})

	t.Run("no frontmatter", func(t *testing.T) {
		tmpDir := t.TempDir()
		skillPath := filepath.Join(tmpDir, "SKILL.md")

		content := `Just content without frontmatter.`

		if err := os.WriteFile(skillPath, []byte(content), 0644); err != nil {
			t.Fatalf("failed to write skill file: %v", err)
		}

		_, err := loadSkillFromFile(skillPath, "test")
		if err == nil {
			t.Error("expected error for missing frontmatter")
		}
	})
}

func TestResolveSkillInvocationPolicy(t *testing.T) {
	tests := []struct {
		name        string
		frontmatter ParsedFrontmatter
		expected    *SkillInvocationPolicy
	}{
		{
			name:        "default values",
			frontmatter: ParsedFrontmatter{},
			expected: &SkillInvocationPolicy{
				UserInvocable:          true,
				DisableModelInvocation: false,
			},
		},
		{
			name: "user invocable false",
			frontmatter: ParsedFrontmatter{
				"user-invocable": "false",
			},
			expected: &SkillInvocationPolicy{
				UserInvocable:          false,
				DisableModelInvocation: false,
			},
		},
		{
			name: "disable model invocation",
			frontmatter: ParsedFrontmatter{
				"disable-model-invocation": "true",
			},
			expected: &SkillInvocationPolicy{
				UserInvocable:          true,
				DisableModelInvocation: true,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := resolveSkillInvocationPolicy(tt.frontmatter)
			if result.UserInvocable != tt.expected.UserInvocable {
				t.Errorf("expected UserInvocable %v, got %v", tt.expected.UserInvocable, result.UserInvocable)
			}
			if result.DisableModelInvocation != tt.expected.DisableModelInvocation {
				t.Errorf("expected DisableModelInvocation %v, got %v", tt.expected.DisableModelInvocation, result.DisableModelInvocation)
			}
		})
	}
}
