package skills

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestIntegration_FullSkillLifecycle(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	tmpDir := t.TempDir()

	// 1. Create a skills directory structure
	workspaceDir := tmpDir
	skillsDir := filepath.Join(workspaceDir, "skills")
	if err := os.MkdirAll(skillsDir, 0755); err != nil {
		t.Fatalf("failed to create skills directory: %v", err)
	}

	// 2. Create a skill file
	skillContent := `---
name: test-skill
description: A test skill for integration testing
metadata: '{"goclaw":{"emoji":"🧪","always":false,"primaryEnv":"node"}}'
user-invocable: true
---

This is a test skill for integration testing.

## Usage

Use this skill for testing purposes.

## Requirements

- Node.js installed
- npm available`

	skillPath := filepath.Join(skillsDir, "test-skill.md")
	if err := os.WriteFile(skillPath, []byte(skillContent), 0644); err != nil {
		t.Fatalf("failed to write skill file: %v", err)
	}

	// 3. Load skill entries
	entries, err := LoadSkillEntries(workspaceDir, LoadSkillsOptions{
		IncludeDefaults: true,
	})
	if err != nil {
		t.Fatalf("failed to load skill entries: %v", err)
	}

	// 4. Verify skill was loaded
	var entry *SkillEntry
	for _, e := range entries {
		if e.Skill.Name == "test-skill" {
			entry = e
			break
		}
	}
	if entry == nil {
		t.Fatal("test-skill not found in loaded entries")
	}

	// 5. Verify skill properties
	if entry.Skill.Name != "test-skill" {
		t.Errorf("expected name 'test-skill', got '%s'", entry.Skill.Name)
	}
	if entry.Skill.Description != "A test skill for integration testing" {
		t.Errorf("unexpected description: %s", entry.Skill.Description)
	}
	if entry.Skill.Source != "workspace" {
		t.Errorf("expected source 'workspace', got '%s'", entry.Skill.Source)
	}

	// 6. Build skill snapshot
	opts := BuildSkillSnapshotOptions{
		Entries:      entries,
		SkillsConfig: &SkillsConfig{},
	}
	snapshot, err := BuildWorkspaceSkillSnapshot(workspaceDir, opts)
	if err != nil {
		t.Fatalf("failed to build snapshot: %v", err)
	}

	// 7. Verify snapshot
	if snapshot.Prompt == "" {
		t.Error("expected non-empty prompt")
	}
	if len(snapshot.ResolvedSkills) == 0 {
		t.Error("expected resolved skills")
	}

	// 8. Build command specs
	cmdSpecs, err := BuildSkillCommandSpecs(workspaceDir, BuildCommandSpecsOptions{
		Entries:               entries,
		SkillsConfig:          &SkillsConfig{},
	})
	if err != nil {
		t.Fatalf("failed to build command specs: %v", err)
	}

	// 9. Verify command spec
	var testSpec *SkillCommandSpec
	for _, spec := range cmdSpecs {
		if spec.SkillName == "test-skill" {
			testSpec = spec
			break
		}
	}
	if testSpec == nil {
		t.Fatal("test-skill command spec not found")
	}
	if testSpec.Name == "" {
		t.Error("expected command name to be set")
	}
}

func TestIntegration_MultipleSkills(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	tmpDir := t.TempDir()

	workspaceDir := tmpDir
	skillsDir := filepath.Join(workspaceDir, "skills")
	if err := os.MkdirAll(skillsDir, 0755); err != nil {
		t.Fatalf("failed to create skills directory: %v", err)
	}

	// Create multiple skills
	skills := []struct {
		name    string
		content string
	}{
		{
			name: "skill-a",
			content: `---
name: skill-a
description: First skill
---
Content A`,
		},
		{
			name: "skill-b",
			content: `---
name: skill-b
description: Second skill
---
Content B`,
		},
		{
			name: "skill-c",
			content: `---
name: skill-c
description: Third skill
---
Content C`,
		},
	}

	for _, skill := range skills {
		skillDir := filepath.Join(skillsDir, skill.name)
		if err := os.MkdirAll(skillDir, 0755); err != nil {
			t.Fatalf("failed to create skill directory: %v", err)
		}
		skillPath := filepath.Join(skillDir, "SKILL.md")
		if err := os.WriteFile(skillPath, []byte(skill.content), 0644); err != nil {
			t.Fatalf("failed to write skill file: %v", err)
		}
	}

	// Load all skills
	entries, err := LoadSkillEntries(workspaceDir, LoadSkillsOptions{
		IncludeDefaults: true,
	})
	if err != nil {
		t.Fatalf("failed to load skill entries: %v", err)
	}

	// Verify all skills were loaded
	skillNames := make(map[string]bool)
	for _, entry := range entries {
		if entry.Skill.Source == "workspace" {
			skillNames[entry.Skill.Name] = true
		}
	}

	expectedSkills := []string{"skill-a", "skill-b", "skill-c"}
	for _, name := range expectedSkills {
		if !skillNames[name] {
			t.Errorf("expected skill '%s' to be loaded", name)
		}
	}

	// Build snapshot with all skills
	snapshot, err := BuildWorkspaceSkillSnapshot(workspaceDir, BuildSkillSnapshotOptions{
		Entries:      entries,
		SkillsConfig: &SkillsConfig{},
	})
	if err != nil {
		t.Fatalf("failed to build snapshot: %v", err)
	}

	if len(snapshot.Skills) < 3 {
		t.Errorf("expected at least 3 skills, got %d", len(snapshot.Skills))
	}
}

func TestIntegration_SkillHierarchy(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	tmpDir := t.TempDir()

	workspaceDir := tmpDir

	// Create managed skills directory
	managedDir := filepath.Join(tmpDir, "managed")
	if err := os.MkdirAll(managedDir, 0755); err != nil {
		t.Fatalf("failed to create managed directory: %v", err)
	}

	// Create a managed skill
	managedSkillDir := filepath.Join(managedDir, "common-skill")
	if err := os.MkdirAll(managedSkillDir, 0755); err != nil {
		t.Fatalf("failed to create managed skill directory: %v", err)
	}
	managedSkillPath := filepath.Join(managedSkillDir, "SKILL.md")
	managedContent := `---
name: common-skill
description: Common managed skill (should be overridden)
---
Managed version.`
	if err := os.WriteFile(managedSkillPath, []byte(managedContent), 0644); err != nil {
		t.Fatalf("failed to write managed skill: %v", err)
	}

	// Create workspace skills directory
	workspaceSkillsDir := filepath.Join(workspaceDir, "skills")
	if err := os.MkdirAll(workspaceSkillsDir, 0755); err != nil {
		t.Fatalf("failed to create workspace skills directory: %v", err)
	}

	// Create workspace skill with same name
	workspaceSkillDir := filepath.Join(workspaceSkillsDir, "common-skill")
	if err := os.MkdirAll(workspaceSkillDir, 0755); err != nil {
		t.Fatalf("failed to create workspace skill directory: %v", err)
	}
	workspaceSkillPath := filepath.Join(workspaceSkillDir, "SKILL.md")
	workspaceContent := `---
name: common-skill
description: Common skill (workspace version)
---
Workspace version - should override managed.`
	if err := os.WriteFile(workspaceSkillPath, []byte(workspaceContent), 0644); err != nil {
		t.Fatalf("failed to write workspace skill: %v", err)
	}

	// Load skills with both sources
	entries, err := LoadSkillEntries(workspaceDir, LoadSkillsOptions{
		IncludeDefaults:  true,
		ManagedSkillsDir: managedDir,
	})
	if err != nil {
		t.Fatalf("failed to load skill entries: %v", err)
	}

	// Verify workspace overrides managed
	var commonEntry *SkillEntry
	for _, entry := range entries {
		if entry.Skill.Name == "common-skill" {
			commonEntry = entry
		}
	}

	if commonEntry == nil {
		t.Fatal("common-skill not found")
	}

	// Should have workspace description
	if commonEntry.Skill.Description != "Common skill (workspace version)" {
		t.Errorf("expected workspace description, got '%s'", commonEntry.Skill.Description)
	}

	// Should be from workspace source
	if commonEntry.Skill.Source != "workspace" {
		t.Errorf("expected source 'workspace', got '%s'", commonEntry.Skill.Source)
	}
}

func TestIntegration_SkillFiltering(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	tmpDir := t.TempDir()

	workspaceDir := tmpDir
	skillsDir := filepath.Join(workspaceDir, "skills")
	if err := os.MkdirAll(skillsDir, 0755); err != nil {
		t.Fatalf("failed to create skills directory: %v", err)
	}

	// Create skill with dependencies
	skillContent := `---
name: dependent-skill
description: A skill that requires git
metadata: '{"goclaw":{"requires":{"bins":["git"]}}}'
---
Content.`

	skillDir := filepath.Join(skillsDir, "dependent-skill")
	if err := os.MkdirAll(skillDir, 0755); err != nil {
		t.Fatalf("failed to create skill directory: %v", err)
	}
	skillPath := filepath.Join(skillDir, "SKILL.md")
	if err := os.WriteFile(skillPath, []byte(skillContent), 0644); err != nil {
		t.Fatalf("failed to write skill file: %v", err)
	}

	// Load skills
	entries, err := LoadSkillEntries(workspaceDir, LoadSkillsOptions{
		IncludeDefaults: true,
	})
	if err != nil {
		t.Fatalf("failed to load skill entries: %v", err)
	}

	// Build snapshot with filtering
	opts := BuildSkillSnapshotOptions{
		Entries:      entries,
		SkillsConfig: &SkillsConfig{},
	}

	// If git is available, skill should be included
	// If git is not available, skill should be excluded
	snapshot, err := BuildWorkspaceSkillSnapshot(workspaceDir, opts)
	if err != nil {
		t.Fatalf("failed to build snapshot: %v", err)
	}

	// Check git availability
	hasGit := HasBinary("git")

	// Verify filtering behavior
	eligibleSkillsCount := 0
	for _, summary := range snapshot.Skills {
		if summary.Name == "dependent-skill" {
			eligibleSkillsCount++
		}
	}

	if hasGit {
		if eligibleSkillsCount == 0 {
			t.Error("skill should be eligible when git is available")
		}
	} else {
		if eligibleSkillsCount > 0 {
			t.Error("skill should not be eligible when git is not available")
		}
	}
}

func TestIntegration_WatcherIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	tmpDir := t.TempDir()

	workspaceDir := tmpDir
	skillsDir := filepath.Join(workspaceDir, "skills")
	if err := os.MkdirAll(skillsDir, 0755); err != nil {
		t.Fatalf("failed to create skills directory: %v", err)
	}

	// Create initial skill
	skillContent := `---
name: watch-test
description: Initial version
---
Content.`

	skillPath := filepath.Join(skillsDir, "watch-test.md")
	if err := os.WriteFile(skillPath, []byte(skillContent), 0644); err != nil {
		t.Fatalf("failed to write skill file: %v", err)
	}

	// Register version tracking
	initialVersion := GetSkillsSnapshotVersion(workspaceDir)

	// Simulate a skill change
	newContent := `---
name: watch-test
description: Updated version
---
Updated content.`

	// Allow some time for file system
	time.Sleep(10 * time.Millisecond)

	if err := os.WriteFile(skillPath, []byte(newContent), 0644); err != nil {
		t.Fatalf("failed to write updated skill file: %v", err)
	}

	// Bump version manually
	BumpSkillsSnapshotVersion(BumpVersionParams{
		WorkspaceDir: workspaceDir,
		Reason:       "manual-test-bump",
	})

	// Verify version changed
	newVersion := GetSkillsSnapshotVersion(workspaceDir)
	if newVersion <= initialVersion {
		t.Error("version should have increased")
	}
}

func TestIntegration_SkillPriority(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	tmpDir := t.TempDir()

	// Create a config with filter
	config := &SkillsConfig{
		Filter: SkillsFilterConfig{
			MinPriority:          1,
			MaxPriority:          10,
			IncludeUnprioritized: true,
		},
	}

	workspaceDir := tmpDir
	skillsDir := filepath.Join(workspaceDir, "skills")
	if err := os.MkdirAll(skillsDir, 0755); err != nil {
		t.Fatalf("failed to create skills directory: %v", err)
	}

	// Create unprioritized skill
	skillPath := filepath.Join(skillsDir, "unprioritized.md")
	skillContent := `---
name: unprioritized-skill
description: Skill without priority
---
Content.`
	if err := os.WriteFile(skillPath, []byte(skillContent), 0644); err != nil {
		t.Fatalf("failed to write skill file: %v", err)
	}

	// Load and snapshot with config
	entries, err := LoadSkillEntries(workspaceDir, LoadSkillsOptions{
		IncludeDefaults: true,
	})
	if err != nil {
		t.Fatalf("failed to load skill entries: %v", err)
	}

	opts := BuildSkillSnapshotOptions{
		Entries:      entries,
		SkillsConfig: config,
	}

	_, err = BuildWorkspaceSkillSnapshot(workspaceDir, opts)
	if err != nil {
		t.Errorf(" failed to build snapshot with config: %v", err)
	}
}

// Benchmark tests

func BenchmarkLoadSkillEntries(b *testing.B) {
	if testing.Short() {
		b.Skip("skipping benchmark in short mode")
	}

	tmpDir := b.TempDir()
	skillsDir := filepath.Join(tmpDir, "skills")
	if err := os.MkdirAll(skillsDir, 0755); err != nil {
		b.Fatalf("failed to create skills directory: %v", err)
	}

	// Create some test skills
	for i := 0; i < 10; i++ {
		skillDir := filepath.Join(skillsDir, "skill-"+string(rune(i)))
		if err := os.MkdirAll(skillDir, 0755); err != nil {
			continue
		}
		skillPath := filepath.Join(skillDir, "SKILL.md")
		content := `---
name: test-skill
description: Test skill
---
Content.`
		if err := os.WriteFile(skillPath, []byte(content), 0644); err != nil {
			continue
		}
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = LoadSkillEntries(tmpDir, LoadSkillsOptions{})
	}
}

func BenchmarkBuildSkillSnapshot(b *testing.B) {
	_ = b.TempDir()

	// Create entries
	entries := make([]*SkillEntry, 10)
	for i := 0; i < 10; i++ {
		entries[i] = &SkillEntry{
			Skill: &Skill{
				Name:        "skill-" + string(rune(i)),
				Description: "Test skill",
				Content:     "Content",
			},
			Metadata: &OpenClawSkillMetadata{
				PrimaryEnv: "node",
			},
			InvocationPolicy: &SkillInvocationPolicy{
				UserInvocable:          true,
				DisableModelInvocation: false,
			},
		}
	}

	opts := BuildSkillSnapshotOptions{
		Entries:      entries,
		SkillsConfig: &SkillsConfig{},
		SnapshotVersion: 1,
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = BuildWorkspaceSkillSnapshot("/workspace", opts)
	}
}
