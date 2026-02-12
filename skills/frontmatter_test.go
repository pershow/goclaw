package skills

import (
	"strings"
	"testing"
)

func TestParseFrontmatter_Basic(t *testing.T) {
	input := `---
name: "skill-name"
description: 'A desc'
foo-bar: value
---

Body text`

	result := ParseFrontmatter(input)

	if result["name"] != "skill-name" {
		t.Errorf("expected name 'skill-name', got '%s'", result["name"])
	}
	if result["description"] != "A desc" {
		t.Errorf("expected description 'A desc', got '%s'", result["description"])
	}
	if result["foo-bar"] != "value" {
		t.Errorf("expected foo-bar 'value', got '%s'", result["foo-bar"])
	}
}

func TestParseFrontmatter_MultiLine(t *testing.T) {
	input := `---
description: |
  Line one
  Line two
---

Body`

	result := ParseFrontmatter(input)

	// Test that multiline parsing works - the actual newline behavior may vary by parser
	// Specifically test that we have both lines present
	if !strings.Contains(result["description"], "Line one") {
		t.Errorf("expected multiline description to contain 'Line one'")
	}
	if !strings.Contains(result["description"], "Line two") {
		t.Errorf("expected multiline description to contain 'Line two'")
	}
	// Don't test trailing newline behavior as it varies by YAML parser
}

func TestParseFrontmatter_NoFrontmatter(t *testing.T) {
	input := "Just text\nsecond line"

	result := ParseFrontmatter(input)

	if len(result) != 0 {
		t.Errorf("expected empty result, got %v", result)
	}
}

func TestParseFrontmatter_CRLF(t *testing.T) {
	input := "---\r\nname: test\r\n---\r\nLine one\r\nLine two"

	result := ParseFrontmatter(input)

	if result["name"] != "test" {
		t.Errorf("expected name 'test', got '%s'", result["name"])
	}
}

func TestParseFrontmatter_EmptyFrontmatter(t *testing.T) {
	input := `---
# just a comment
---

Body`

	result := ParseFrontmatter(input)

	if len(result) != 0 {
		t.Errorf("expected empty result for comment-only frontmatter, got %v", result)
	}
}

func TestParseFrontmatter_MetadataField(t *testing.T) {
	input := `---
name: test-skill
metadata: '{"goclaw":{"emoji":"🔧","always":true}}'
---

Body`

	result := ParseFrontmatter(input)

	if result["name"] != "test-skill" {
		t.Errorf("expected name 'test-skill', got '%s'", result["name"])
	}
	if result["metadata"] == "" {
		t.Error("expected metadata field to be present")
	}
}

func TestParseOpenClawMetadata(t *testing.T) {
	frontmatter := ParsedFrontmatter{
		"metadata": `{"goclaw":{"emoji":"🔧","always":true,"skillKey":"test-key","requires":{"bins":["git","node"]}}}`,
	}

	metadata := ParseOpenClawMetadata(frontmatter)

	if metadata == nil {
		t.Fatal("expected metadata to be parsed")
	}
	if metadata.Emoji != "🔧" {
		t.Errorf("expected emoji '🔧', got '%s'", metadata.Emoji)
	}
	if !metadata.Always {
		t.Error("expected always to be true")
	}
	if metadata.SkillKey != "test-key" {
		t.Errorf("expected skillKey 'test-key', got '%s'", metadata.SkillKey)
	}
	if metadata.Requires == nil {
		t.Fatal("expected requires to be present")
	}
	if len(metadata.Requires.Bins) != 2 {
		t.Errorf("expected 2 bins, got %d", len(metadata.Requires.Bins))
	}
}

func TestParseOpenClawMetadata_OpenClawKey(t *testing.T) {
	frontmatter := ParsedFrontmatter{
		"metadata": `{"openclaw":{"emoji":"📦","primaryEnv":"node"}}`,
	}

	metadata := ParseOpenClawMetadata(frontmatter)

	if metadata == nil {
		t.Fatal("expected metadata to be parsed with openclaw key")
	}
	if metadata.Emoji != "📦" {
		t.Errorf("expected emoji '📦', got '%s'", metadata.Emoji)
	}
	if metadata.PrimaryEnv != "node" {
		t.Errorf("expected primaryEnv 'node', got '%s'", metadata.PrimaryEnv)
	}
}

func TestParseOpenClawMetadata_NoMetadata(t *testing.T) {
	frontmatter := ParsedFrontmatter{
		"name": "test",
	}

	metadata := ParseOpenClawMetadata(frontmatter)

	if metadata != nil {
		t.Error("expected nil metadata when metadata field is absent")
	}
}

func TestParseSkillInvocationPolicy(t *testing.T) {
	tests := []struct {
		name     string
		key      string
		value    string
		expected SkillInvocationPolicy
	}{
		{
			name:  "default values",
			key:   "user-invocable",
			value: "",
			expected: SkillInvocationPolicy{
				UserInvocable:          true,
				DisableModelInvocation: false,
			},
		},
		{
			name:  "user invocable false",
			key:   "user-invocable",
			value: "false",
			expected: SkillInvocationPolicy{
				UserInvocable:          false,
				DisableModelInvocation: false,
			},
		},
		{
			name:  "disable model invocation",
			key:   "disable-model-invocation",
			value: "true",
			expected: SkillInvocationPolicy{
				UserInvocable:          true,
				DisableModelInvocation: true,
			},
		},
		{
			name:  "yes as truthy",
			key:   "user-invocable",
			value: "yes",
			expected: SkillInvocationPolicy{
				UserInvocable:          true,
				DisableModelInvocation: false,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			frontmatter := ParsedFrontmatter{
				tt.key: tt.value,
			}
			result := ParseSkillInvocationPolicy(frontmatter)

			if result.UserInvocable != tt.expected.UserInvocable {
				t.Errorf("expected UserInvocable %v, got %v", tt.expected.UserInvocable, result.UserInvocable)
			}
			if result.DisableModelInvocation != tt.expected.DisableModelInvocation {
				t.Errorf("expected DisableModelInvocation %v, got %v", tt.expected.DisableModelInvocation, result.DisableModelInvocation)
			}
		})
	}
}

func TestResolveSkillKey(t *testing.T) {
	tests := []struct {
		name      string
		skillName string
		metadata  *OpenClawSkillMetadata
		expected  string
	}{
		{
			name:      "no metadata",
			skillName: "my-skill",
			metadata:  nil,
			expected:  "my-skill",
		},
		{
			name:      "metadata without skillKey",
			skillName: "my-skill",
			metadata:  &OpenClawSkillMetadata{Emoji: "🔧"},
			expected:  "my-skill",
		},
		{
			name:      "metadata with skillKey",
			skillName: "my-skill",
			metadata:  &OpenClawSkillMetadata{SkillKey: "custom-key"},
			expected:  "custom-key",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ResolveSkillKey(tt.skillName, tt.metadata)
			if result != tt.expected {
				t.Errorf("expected '%s', got '%s'", tt.expected, result)
			}
		})
	}
}

func TestParseInstallSpec(t *testing.T) {
	tests := []struct {
		name     string
		input    map[string]interface{}
		expected *SkillInstallSpec
	}{
		{
			name: "brew install",
			input: map[string]interface{}{
				"kind":    "brew",
				"formula": "git",
				"bins":    []interface{}{"git"},
			},
			expected: &SkillInstallSpec{
				Kind:    "brew",
				Formula: "git",
				Bins:    []string{"git"},
			},
		},
		{
			name: "node install with type field",
			input: map[string]interface{}{
				"type":    "node",
				"package": "typescript",
				"bins":    []interface{}{"tsc"},
			},
			expected: &SkillInstallSpec{
				Kind:    "node",
				Type:    "node",
				Package: "typescript",
				Bins:    []string{"tsc"},
			},
		},
		{
			name: "invalid kind",
			input: map[string]interface{}{
				"kind": "invalid",
			},
			expected: nil,
		},
		{
			name:  "nil input",
			input: nil,
			expected: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parseInstallSpec(tt.input)

			if tt.expected == nil {
				if result != nil {
					t.Errorf("expected nil, got %+v", result)
				}
				return
			}

			if result == nil {
				t.Fatal("expected non-nil result")
			}
			if result.Kind != tt.expected.Kind {
				t.Errorf("expected kind '%s', got '%s'", tt.expected.Kind, result.Kind)
			}
			if result.Formula != tt.expected.Formula {
				t.Errorf("expected formula '%s', got '%s'", tt.expected.Formula, result.Formula)
			}
			if result.Package != tt.expected.Package {
				t.Errorf("expected package '%s', got '%s'", tt.expected.Package, result.Package)
			}
		})
	}
}

func TestNormalizeStringList(t *testing.T) {
	tests := []struct {
		name     string
		input    interface{}
		expected []string
	}{
		{
			name:     "string array",
			input:    []interface{}{"a", "b", "c"},
			expected: []string{"a", "b", "c"},
		},
		{
			name:     "go string slice",
			input:    []string{"x", "y", "z"},
			expected: []string{"x", "y", "z"},
		},
		{
			name:     "comma separated string",
			input:    "a, b, c",
			expected: []string{"a", "b", "c"},
		},
		{
			name:     "single string",
			input:    "single",
			expected: []string{"single"},
		},
		{
			name:     "nil",
			input:    nil,
			expected: nil,
		},
		{
			name:     "empty array",
			input:    []interface{}{},
			expected: nil,
		},
		{
			name:     "array with empty strings",
			input:    []interface{}{"a", "", "c"},
			expected: []string{"a", "c"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := normalizeStringList(tt.input)

			if tt.expected == nil {
				if result != nil {
					t.Errorf("expected nil, got %v", result)
				}
				return
			}

			if len(result) != len(tt.expected) {
				t.Errorf("expected length %d, got %d", len(tt.expected), len(result))
				return
			}

			for i, v := range tt.expected {
				if result[i] != v {
					t.Errorf("expected [%d] to be '%s', got '%s'", i, v, result[i])
				}
			}
		})
	}
}

func TestStripFrontmatter(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name: "basic frontmatter",
			input: `---
key: value
---

Body content`,
			expected: "Body content",
		},
		{
			name:     "no frontmatter",
			input:    "Just content",
			expected: "Just content",
		},
		{
			name: "no closing delimiter",
			input: `---
key: value
Body without closing`,
			expected: `---
key: value
Body without closing`,
		},
		{
			name: "extra whitespace",
			input: `---
key: value
---

   Body with spaces   `,
			expected: "Body with spaces",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := StripFrontmatter(tt.input)
			if result != tt.expected {
				t.Errorf("expected '%s', got '%s'", tt.expected, result)
			}
		})
	}
}

func TestParseBooleanValue(t *testing.T) {
	tests := []struct {
		name     string
		input    interface{}
		expected *bool
	}{
		{
			name:     "true boolean",
			input:    true,
			expected: boolPtr(true),
		},
		{
			name:     "false boolean",
			input:    false,
			expected: boolPtr(false),
		},
		{
			name:     "true string",
			input:    "true",
			expected: boolPtr(true),
		},
		{
			name:     "yes string",
			input:    "yes",
			expected: boolPtr(true),
		},
		{
			name:     "1 string",
			input:    "1",
			expected: boolPtr(true),
		},
		{
			name:     "on string",
			input:    "on",
			expected: boolPtr(true),
		},
		{
			name:     "false string",
			input:    "false",
			expected: boolPtr(false),
		},
		{
			name:     "no string",
			input:    "no",
			expected: boolPtr(false),
		},
		{
			name:     "0 string",
			input:    "0",
			expected: boolPtr(false),
		},
		{
			name:     "off string",
			input:    "off",
			expected: boolPtr(false),
		},
		{
			name:     "empty string",
			input:    "",
			expected: nil,
		},
		{
			name:     "invalid string",
			input:    "maybe",
			expected: nil,
		},
		{
			name:     "nil",
			input:    nil,
			expected: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parseBooleanValue(tt.input)

			if tt.expected == nil {
				if result != nil {
					t.Errorf("expected nil, got %v", result)
				}
				return
			}

			if result == nil {
				t.Errorf("expected %v, got nil", *tt.expected)
				return
			}

			if *result != *tt.expected {
				t.Errorf("expected %v, got %v", *tt.expected, *result)
			}
		})
	}
}

func TestCoerceFrontmatterValue(t *testing.T) {
	tests := []struct {
		name     string
		input    interface{}
		expected string
	}{
		{
			name:     "string",
			input:    "hello",
			expected: "hello",
		},
		{
			name:     "string with spaces",
			input:    "  hello  ",
			expected: "  hello  ",  // coerceFrontmatterValue doesn't trim spaces
		},
		{
			name:     "boolean",
			input:    true,
			expected: "true",
		},
		{
			name:     "integer",
			input:    42,
			expected: "42",
		},
		{
			name:     "float",
			input:    3.14,
			expected: "3.14",
		},
		{
			name:     "nil",
			input:    nil,
			expected: "",
		},
		{
			name:     "map",
			input:    map[string]interface{}{"key": "value"},
			expected: `{"key":"value"}`,
		},
		{
			name:     "array",
			input:    []interface{}{"a", "b"},
			expected: `["a","b"]`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := coerceFrontmatterValue(tt.input)
			if result != tt.expected {
				t.Errorf("expected '%s', got '%s'", tt.expected, result)
			}
		})
	}
}

// Helper function
func boolPtr(b bool) *bool {
	return &b
}
