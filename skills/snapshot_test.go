package skills

import (
	"strings"
	"testing"
)

func TestBuildWorkspaceSkillSnapshot_Basic(t *testing.T) {
	entries := []*SkillEntry{
		{
			Skill: &Skill{
				Name:        "skill-1",
				Description: "First skill",
				Source:      "workspace",
				Content:     "Content for skill 1",
			},
			Metadata: &OpenClawSkillMetadata{
				PrimaryEnv: "node",
			},
			InvocationPolicy: &SkillInvocationPolicy{
				UserInvocable:          true,
				DisableModelInvocation: false,
			},
		},
		{
			Skill: &Skill{
				Name:        "skill-2",
				Description: "Second skill",
				Source:      "workspace",
				Content:     "Content for skill 2",
			},
			Metadata: &OpenClawSkillMetadata{
				PrimaryEnv: "python",
			},
			InvocationPolicy: &SkillInvocationPolicy{
				UserInvocable:          true,
				DisableModelInvocation: false,
			},
		},
	}

	opts := BuildSkillSnapshotOptions{
		Entries:         entries,
		SkillsConfig:    &SkillsConfig{},
		SkillFilter:     nil,
		Eligibility:     nil,
		SnapshotVersion: 1,
	}

	snapshot, err := BuildWorkspaceSkillSnapshot("/workspace", opts)
	if err != nil {
		t.Fatalf("failed to build snapshot: %v", err)
	}

	if snapshot.Prompt == "" {
		t.Error("expected prompt to be generated")
	}
	if len(snapshot.Skills) != 2 {
		t.Errorf("expected 2 skills in summary, got %d", len(snapshot.Skills))
	}
	if len(snapshot.ResolvedSkills) != 2 {
		t.Errorf("expected 2 resolved skills, got %d", len(snapshot.ResolvedSkills))
	}
	if snapshot.Version != 1 {
		t.Errorf("expected version 1, got %d", snapshot.Version)
	}
}

func TestBuildWorkspaceSkillSnapshot_DisableModelInvocation(t *testing.T) {
	entries := []*SkillEntry{
		{
			Skill: &Skill{
				Name:        "normal-skill",
				Description: "Normal skill",
				Content:     "Normal content",
			},
			InvocationPolicy: &SkillInvocationPolicy{
				UserInvocable:          true,
				DisableModelInvocation: false,
			},
		},
		{
			Skill: &Skill{
				Name:        "hidden-skill",
				Description: "Hidden from model",
				Content:     "Hidden content",
			},
			InvocationPolicy: &SkillInvocationPolicy{
				UserInvocable:          true,
				DisableModelInvocation: true,
			},
		},
	}

	opts := BuildSkillSnapshotOptions{
		Entries:      entries,
		SkillsConfig: &SkillsConfig{},
	}

	snapshot, err := BuildWorkspaceSkillSnapshot("/workspace", opts)
	if err != nil {
		t.Fatalf("failed to build snapshot: %v", err)
	}

	// Both skills should be in ResolvedSkills (eligible)
	if len(snapshot.ResolvedSkills) != 2 {
		t.Errorf("expected 2 resolved skills, got %d", len(snapshot.ResolvedSkills))
	}

	// But prompt should only contain the normal skill
	if !strings.Contains(snapshot.Prompt, "normal-skill") {
		t.Error("expected normal-skill in prompt")
	}
	if strings.Contains(snapshot.Prompt, "hidden-skill") {
		t.Error("hidden-skill should not appear in prompt")
	}
}

func TestFilterPromptEntries(t *testing.T) {
	entries := []*SkillEntry{
		{
			Skill: &Skill{Name: "normal"},
			InvocationPolicy: &SkillInvocationPolicy{
				UserInvocable:          true,
				DisableModelInvocation: false,
			},
		},
		{
			Skill: &Skill{Name: "no-model"},
			InvocationPolicy: &SkillInvocationPolicy{
				UserInvocable:          true,
				DisableModelInvocation: true,
			},
		},
		{
			Skill:            &Skill{Name: "nil-policy"},
			InvocationPolicy: nil,
		},
	}

	filtered := FilterPromptEntries(entries)

	if len(filtered) != 2 {
		t.Errorf("expected 2 entries, got %d", len(filtered))
	}
}

func TestBuildSkillCommandSpecs_Basic(t *testing.T) {
	entries := []*SkillEntry{
		{
			Skill: &Skill{
				Name:        "test-skill",
				Description: "A test skill",
			},
			Metadata: &OpenClawSkillMetadata{},
			Frontmatter: ParsedFrontmatter{
				"user-invocable": "true",
			},
			InvocationPolicy: &SkillInvocationPolicy{
				UserInvocable: true,
			},
		},
	}

	opts := BuildCommandSpecsOptions{
		Entries:       entries,
		SkillsConfig:  &SkillsConfig{},
		ReservedNames: []string{"help", "version"},
	}

	specs, err := BuildSkillCommandSpecs("/workspace", opts)
	if err != nil {
		t.Fatalf("failed to build command specs: %v", err)
	}

	if len(specs) != 1 {
		t.Errorf("expected 1 spec, got %d", len(specs))
	}

	spec := specs[0]
	if spec.Name != "test-skill" {
		t.Errorf("expected name 'test-skill', got '%s'", spec.Name)
	}
	if spec.SkillName != "test-skill" {
		t.Errorf("expected skill name 'test-skill', got '%s'", spec.SkillName)
	}
	if spec.Description != "A test skill" {
		t.Errorf("expected description 'A test skill', got '%s'", spec.Description)
	}
}

func TestBuildSkillCommandSpecs_ToolDispatch(t *testing.T) {
	entries := []*SkillEntry{
		{
			Skill: &Skill{
				Name:        "tool-skill",
				Description: "A tool skill",
			},
			Frontmatter: ParsedFrontmatter{
				"user-invocable":   "true",
				"command-dispatch": "tool",
				"command-tool":     "custom_tool",
				"command-arg-mode": "raw",
			},
			InvocationPolicy: &SkillInvocationPolicy{
				UserInvocable: true,
			},
		},
	}

	opts := BuildCommandSpecsOptions{
		Entries:      entries,
		SkillsConfig: &SkillsConfig{},
	}

	specs, err := BuildSkillCommandSpecs("/workspace", opts)
	if err != nil {
		t.Fatalf("failed to build command specs: %v", err)
	}

	if len(specs) != 1 {
		t.Errorf("expected 1 spec, got %d", len(specs))
	}

	spec := specs[0]
	if spec.Dispatch == nil {
		t.Fatal("expected dispatch to be present")
	}
	if spec.Dispatch.Kind != "tool" {
		t.Errorf("expected dispatch kind 'tool', got '%s'", spec.Dispatch.Kind)
	}
	if spec.Dispatch.ToolName != "custom_tool" {
		t.Errorf("expected tool name 'custom_tool', got '%s'", spec.Dispatch.ToolName)
	}
	if spec.Dispatch.ArgMode != "raw" {
		t.Errorf("expected arg mode 'raw', got '%s'", spec.Dispatch.ArgMode)
	}
}

func TestSanitizeSkillCommandName(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "simple name",
			input:    "test-skill",
			expected: "test-skill",
		},
		{
			name:     "with spaces and special chars",
			input:    "Test Skill One!",
			expected: "test_skill_one",
		},
		{
			name:     "very long name",
			input:    "this-is-a-very-long-skill-name-that-exceeds-maximum-length",
			expected: "this-is-a-very-long-skill-name-t", // max 32 chars, preserves dashes
		},
		{
			name:     "only special chars",
			input:    "!!!@@@###",
			expected: "skill",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := sanitizeSkillCommandName(tt.input)
			if result != tt.expected {
				t.Errorf("expected '%s', got '%s'", tt.expected, result)
			}
		})
	}
}

func TestFormatSkillsForPrompt(t *testing.T) {
	skills := []*Skill{
		{
			Name:        "skill-a",
			Description: "Description A",
			FilePath:    "/path/to/a.md",
		},
		{
			Name:        "skill-b",
			Description: "Description B",
			FilePath:    "/path/to/b.md",
		},
	}

	prompt, err := FormatSkillsForPrompt(skills)
	if err != nil {
		t.Fatalf("failed to format skills: %v", err)
	}

	if prompt == "" {
		t.Error("expected non-empty prompt")
	}

	// Check that skills are sorted alphabetically
	aIndex := strings.Index(prompt, "skill-a")
	bIndex := strings.Index(prompt, "skill-b")
	if aIndex > bIndex {
		t.Error("skills should be sorted alphabetically")
	}

	// Check XML structure
	if !strings.Contains(prompt, "<available_skills>") {
		t.Error("expected <available_skills> tag")
	}
	if !strings.Contains(prompt, "</available_skills>") {
		t.Error("expected closing tag")
	}
	if !strings.Contains(prompt, "<skill>") {
		t.Error("expected <skill> tag")
	}
}

func TestExtractContent_Snapshot(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name: "with frontmatter",
			input: `---
key: value
---

Content here`,
			expected: "Content here",
		},
		{
			name:     "without frontmatter",
			input:    "Just content",
			expected: "Just content",
		},
		{
			name: "no closing delimiter",
			input: `---
key: value
Content without closing`,
			expected: `---
key: value
Content without closing`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractContent(tt.input)
			if result != tt.expected {
				t.Errorf("expected '%s', got '%s'", tt.expected, result)
			}
		})
	}
}

func TestBuildSkillCommandSpecs_ReservedNames(t *testing.T) {
	entries := []*SkillEntry{
		{
			Skill: &Skill{
				Name:        "help",
				Description: "Should not collide with reserved",
			},
			InvocationPolicy: &SkillInvocationPolicy{
				UserInvocable: true,
			},
		},
	}

	opts := BuildCommandSpecsOptions{
		Entries:       entries,
		SkillsConfig:  &SkillsConfig{},
		ReservedNames: []string{"help", "version"},
	}

	specs, err := BuildSkillCommandSpecs("/workspace", opts)
	if err != nil {
		t.Fatalf("failed to build command specs: %v", err)
	}

	if len(specs) != 1 {
		t.Errorf("expected 1 spec, got %d", len(specs))
	}

	// Name should be modified to avoid collision
	spec := specs[0]
	if spec.Name == "help" || spec.Name == "version" {
		t.Error("command name should be modified to avoid reserved names")
	}
}

func TestShouldIncludeSkill_Eligibility(t *testing.T) {
	alwaysEntry := &SkillEntry{
		Skill: &Skill{
			Name: "always-skill",
		},
		Metadata: &OpenClawSkillMetadata{
			Always: true,
			Requires: &SkillRequires{
				Bins: []string{"nonexistent-xyz"},
			},
		},
	}

	opts := FilterSkillEntriesOptions{
		SkillConfig: &SkillsConfig{},
		Eligibility: nil,
	}

	// Always skills should be included regardless of missing dependencies
	if !shouldIncludeSkill(alwaysEntry, opts) {
		t.Error("expected always skill to be included")
	}
}
