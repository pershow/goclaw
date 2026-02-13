package skills

import (
	"runtime"
	"testing"
)

func TestShouldIncludeSkill_Basic(t *testing.T) {
	entry := &SkillEntry{
		Skill: &Skill{
			Name:        "test-skill",
			Description: "Test skill",
			Source:      "workspace",
		},
		Frontmatter: ParsedFrontmatter{
			"name": "test-skill",
		},
		Metadata: &OpenClawSkillMetadata{
			Always: false,
		},
	}

	opts := FilterSkillEntriesOptions{
		SkillConfig: &SkillsConfig{},
		Eligibility: nil,
	}

	if !shouldIncludeSkill(entry, opts) {
		t.Error("expected skill to be included")
	}
}

func TestShouldIncludeSkill_ConfigDisabled(t *testing.T) {
	entry := &SkillEntry{
		Skill: &Skill{
			Name:   "disabled-skill",
			Source: "bundled",
		},
		Metadata: &OpenClawSkillMetadata{},
	}

	opts := FilterSkillEntriesOptions{
		SkillConfig: &SkillsConfig{
			Disabled: map[string]bool{
				"disabled-skill": true,
			},
		},
		Eligibility: nil,
	}

	if shouldIncludeSkill(entry, opts) {
		t.Error("expected disabled skill to be excluded")
	}
}

func TestShouldIncludeSkill_BundledAllowlist(t *testing.T) {
	tests := []struct {
		name      string
		allowlist []string
		allowed   bool
	}{
		{
			name:      "empty allowlist allows all",
			allowlist: []string{},
			allowed:   true,
		},
		{
			name:      "skill in allowlist",
			allowlist: []string{"allowed-skill"},
			allowed:   true,
		},
		{
			name:      "skill not in allowlist",
			allowlist: []string{"other-skill"},
			allowed:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			entry := &SkillEntry{
				Skill: &Skill{
					Name:   "allowed-skill",
					Source: "bundled",
				},
				Metadata: &OpenClawSkillMetadata{},
			}

			opts := FilterSkillEntriesOptions{
				SkillConfig: &SkillsConfig{
					AllowBundled: tt.allowlist,
				},
				Eligibility: nil,
			}

			result := shouldIncludeSkill(entry, opts)
			if result != tt.allowed {
				t.Errorf("expected allowed %v, got %v", tt.allowed, result)
			}
		})
	}
}

func TestShouldIncludeSkill_OSCompatibility(t *testing.T) {
	tests := []struct {
		name       string
		osList     []string
		compatible bool
	}{
		{
			name:       "matches current OS",
			osList:     []string{runtime.GOOS},
			compatible: true,
		},
		{
			name:       "compatible with multiple OS",
			osList:     []string{"darwin", "linux", "windows"},
			compatible: true,
		},
		{
			name:       "incompatible OS",
			osList:     []string{"unknown-os"},
			compatible: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			entry := &SkillEntry{
				Skill: &Skill{
					Name: "test-skill",
				},
				Metadata: &OpenClawSkillMetadata{
					OS: tt.osList,
				},
			}

			opts := FilterSkillEntriesOptions{
				SkillConfig: &SkillsConfig{},
				Eligibility: nil,
			}

			result := shouldIncludeSkill(entry, opts)
			if result != tt.compatible {
				t.Errorf("expected compatible %v, got %v", tt.compatible, result)
			}
		})
	}
}

func TestShouldIncludeSkill_Always(t *testing.T) {
	entry := &SkillEntry{
		Skill: &Skill{
			Name: "always-skill",
		},
		Metadata: &OpenClawSkillMetadata{
			Always: true,
			Requires: &SkillRequires{
				Bins: []string{"this-should-not-exist-xyz"},
			},
		},
	}

	opts := FilterSkillEntriesOptions{
		SkillConfig: &SkillsConfig{},
		Eligibility: nil,
	}

	// Always skills should be included regardless of missing dependencies
	if !shouldIncludeSkill(entry, opts) {
		t.Error("expected always skill to be included")
	}
}

func TestShouldIncludeSkill_MissingBins(t *testing.T) {
	entry := &SkillEntry{
		Skill: &Skill{
			Name: "tool-skill",
		},
		Metadata: &OpenClawSkillMetadata{
			Requires: &SkillRequires{
				Bins: []string{"this-should-not-exist-xyz-123"},
			},
		},
	}

	opts := FilterSkillEntriesOptions{
		SkillConfig: &SkillsConfig{},
		Eligibility: nil,
	}

	if shouldIncludeSkill(entry, opts) {
		t.Error("expected skill with missing bins to be excluded")
	}
}

func TestShouldIncludeSkill_MissingAnyBins(t *testing.T) {
	entry := &SkillEntry{
		Skill: &Skill{
			Name: "anybin-skill",
		},
		Metadata: &OpenClawSkillMetadata{
			Requires: &SkillRequires{
				AnyBins: []string{"nonexistent-xyz-1", "nonexistent-xyz-2"},
			},
		},
	}

	opts := FilterSkillEntriesOptions{
		SkillConfig: &SkillsConfig{},
		Eligibility: nil,
	}

	if shouldIncludeSkill(entry, opts) {
		t.Error("expected skill with missing anybins to be excluded")
	}
}

func TestShouldIncludeSkill_MissingEnv(t *testing.T) {
	entry := &SkillEntry{
		Skill: &Skill{
			Name: "env-skill",
		},
		Metadata: &OpenClawSkillMetadata{
			Requires: &SkillRequires{
				Env: []string{"SHOULD_NOT_EXIST_ENV_VAR_12345"},
			},
		},
	}

	opts := FilterSkillEntriesOptions{
		SkillConfig: &SkillsConfig{},
		Eligibility: nil,
	}

	if shouldIncludeSkill(entry, opts) {
		t.Error("expected skill with missing env vars to be excluded")
	}
}

func TestSkillEntry_IsEnabled(t *testing.T) {
	tests := []struct {
		name    string
		config  SkillsConfig
		enabled bool
	}{
		{
			name: "default enabled",
			config: SkillsConfig{
				Entries: map[string]SkillEntryConfig{},
			},
			enabled: true,
		},
		{
			name: "explicitly enabled",
			config: SkillsConfig{
				Entries: map[string]SkillEntryConfig{
					"test-skill": {Enabled: true},
				},
			},
			enabled: true,
		},
		{
			name: "explicitly disabled",
			config: SkillsConfig{
				Entries: map[string]SkillEntryConfig{
					"test-skill": {Enabled: false},
				},
			},
			enabled: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			entry := &SkillEntry{
				Skill: &Skill{
					Name:   "test-skill",
					Source: "workspace",
				},
				Metadata: &OpenClawSkillMetadata{},
			}

			result := entry.IsEnabled(tt.config)
			if result != tt.enabled {
				t.Errorf("expected enabled %v, got %v", tt.enabled, result)
			}
		})
	}
}

func TestSkillEntry_PrimaryEnv(t *testing.T) {
	tests := []struct {
		name        string
		metadata    *OpenClawSkillMetadata
		frontmatter ParsedFrontmatter
		expected    string
	}{
		{
			name:        "no metadata",
			metadata:    nil,
			frontmatter: ParsedFrontmatter{},
			expected:    "",
		},
		{
			name: "metadata with primary env",
			metadata: &OpenClawSkillMetadata{
				PrimaryEnv: "node",
			},
			frontmatter: ParsedFrontmatter{},
			expected:    "node",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			entry := &SkillEntry{
				Skill: &Skill{
					Name: "test-skill",
				},
				Metadata:    tt.metadata,
				Frontmatter: tt.frontmatter,
			}

			result := entry.PrimaryEnv()
			if result != tt.expected {
				t.Errorf("expected '%s', got '%s'", tt.expected, result)
			}
		})
	}
}
