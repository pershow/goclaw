package skills

import (
	"encoding/json"
	"fmt"
	"strings"
	"unicode"

	"github.com/tidwall/gjson"
	"gopkg.in/yaml.v3"
)

const (
	// ManifestKey is the primary metadata key for goclaw-specific metadata
	ManifestKey = "goclaw"
)

// ParsedFrontmatter represents the parsed YAML frontmatter from a markdown file.
// All values are coerced to strings for consistent handling.
type ParsedFrontmatter map[string]string

// OpenClawSkillMetadata represents metadata extracted from the metadata.openclaw field.
// This is compatible with openclaw skill format for cross-compatibility.
type OpenClawSkillMetadata struct {
	Always     bool               `yaml:"always" json:"always"`
	SkillKey   string             `yaml:"skillKey" json:"skillKey"`
	PrimaryEnv string             `yaml:"primaryEnv" json:"primaryEnv"`
	Emoji      string             `yaml:"emoji" json:"emoji"`
	Homepage   string             `yaml:"homepage" json:"homepage"`
	OS         []string           `yaml:"os" json:"os"`
	Requires   *SkillRequires     `yaml:"requires" json:"requires"`
	Install    []SkillInstallSpec `yaml:"install" json:"install"`
}

// SkillRequires defines requirements for a skill to function.
type SkillRequires struct {
	Bins    []string `yaml:"bins" json:"bins"`
	AnyBins []string `yaml:"anyBins" json:"anyBins"`
	Env     []string `yaml:"env" json:"env"`
	Config  []string `yaml:"config" json:"config"`
	OS      []string `yaml:"os" json:"os"`
}

// SkillInstallSpec defines how to install a dependency for a skill.
type SkillInstallSpec struct {
	ID              string   `yaml:"id" json:"id"`
	Kind            string   `yaml:"kind" json:"kind"`
	Type            string   `yaml:"type" json:"type"` // Legacy alias for Kind
	Label           string   `yaml:"label" json:"label"`
	Bins            []string `yaml:"bins" json:"bins"`
	OS              []string `yaml:"os" json:"os"`
	Formula         string   `yaml:"formula" json:"formula"`
	Package         string   `yaml:"package" json:"package"`
	Module          string   `yaml:"module" json:"module"`
	URL             string   `yaml:"url" json:"url"`
	Archive         string   `yaml:"archive" json:"archive"`
	Extract         bool     `yaml:"extract" json:"extract"`
	StripComponents int      `yaml:"stripComponents" json:"stripComponents"`
	TargetDir       string   `yaml:"targetDir" json:"targetDir"`
}

// SkillInvocationPolicy defines when and how a skill can be invoked.
type SkillInvocationPolicy struct {
	UserInvocable          bool `yaml:"userInvocable" json:"userInvocable"`
	DisableModelInvocation bool `yaml:"disableModelInvocation" json:"disableModelInvocation"`
}

// ParseFrontmatter parses YAML frontmatter from markdown content.
// It extracts content between --- delimiters and returns a map of string values.
func ParseFrontmatter(content string) ParsedFrontmatter {
	return parseFrontmatterBlock(content)
}

// parseFrontmatterBlock parses the YAML frontmatter block from content.
func parseFrontmatterBlock(content string) ParsedFrontmatter {
	normalized := normalizeNewlines(content)

	if !strings.HasPrefix(normalized, "---") {
		return ParsedFrontmatter{}
	}

	endIndex := strings.Index(normalized[3:], "\n---")
	if endIndex == -1 {
		return ParsedFrontmatter{}
	}

	block := normalized[4 : endIndex+3]

	// Try YAML parsing first
	yamlParsed := parseYamlFrontmatter(block)
	lineParsed := parseLineFrontmatter(block)

	// If YAML parsing failed, return line-parsed results
	if yamlParsed == nil {
		return lineParsed
	}

	// Merge results - line-parsed JSON objects take precedence for certain keys
	merged := make(ParsedFrontmatter)
	for k, v := range yamlParsed {
		merged[k] = v
	}
	for k, v := range lineParsed {
		// Line parser wins for JSON-like values (metadata field, etc)
		if strings.HasPrefix(v, "{") || strings.HasPrefix(v, "[") {
			merged[k] = v
		}
	}

	return merged
}

// normalizeNewlines converts all newline variants to \n
func normalizeNewlines(value string) string {
	result := strings.ReplaceAll(value, "\r\n", "\n")
	result = strings.ReplaceAll(result, "\r", "\n")
	return result
}

// parseYamlFrontmatter parses YAML content into a frontmatter map.
func parseYamlFrontmatter(block string) ParsedFrontmatter {
	var raw map[string]interface{}
	if err := yaml.Unmarshal([]byte(block), &raw); err != nil {
		return nil
	}

	if raw == nil {
		return nil
	}

	result := make(ParsedFrontmatter)
	for key, value := range raw {
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}

		coerced := coerceFrontmatterValue(value)
		if coerced != "" {
			result[key] = coerced
		}
	}

	return result
}

// parseLineFrontmatter parses simple key:value line format as a fallback.
// Handles multi-line values with indentation.
func parseLineFrontmatter(block string) ParsedFrontmatter {
	frontmatter := make(ParsedFrontmatter)
	lines := strings.Split(block, "\n")
	i := 0

	for i < len(lines) {
		line := lines[i]
		key, inlineValue := parseKeyValueLine(line)
		if key == "" {
			i++
			continue
		}

		// Check for multi-line value
		if inlineValue == "" && i+1 < len(lines) {
			nextLine := lines[i+1]
			if len(nextLine) > 0 && (nextLine[0] == ' ' || nextLine[0] == '\t') {
				value, linesConsumed := extractMultiLineValue(lines, i)
				if value != "" {
					frontmatter[key] = value
				}
				i += linesConsumed
				continue
			}
		}

		value := stripQuotes(inlineValue)
		if value != "" {
			frontmatter[key] = value
		}
		i++
	}

	return frontmatter
}

// extractMultiLineValue extracts a multi-line value starting at startIndex.
// Returns the combined value and number of lines consumed.
func extractMultiLineValue(lines []string, startIndex int) (string, int) {
	if startIndex >= len(lines) {
		return "", 1
	}

	startLine := lines[startIndex]
	_, inlineValue := parseKeyValueLine(startLine)
	if inlineValue != "" {
		return inlineValue, 1
	}

	var valueLines []string
	i := startIndex + 1

	for i < len(lines) {
		line := lines[i]
		if len(line) > 0 && !unicode.IsSpace(rune(line[0])) {
			break
		}
		valueLines = append(valueLines, line)
		i++
	}

	combined := strings.Trim(strings.Join(valueLines, "\n"), " \t\n")
	return combined, i - startIndex
}

// coerceFrontmatterValue converts various YAML types to string representation.
func coerceFrontmatterValue(value interface{}) string {
	if value == nil {
		return ""
	}

	switch v := value.(type) {
	case string:
		// Don't trim - preserve YAML's output exactly (including multiline newlines)
		// The YAML parser already handles indentation and formatting
		return v
	case bool, int, int64, float64:
		return fmt.Sprintf("%v", v)
	case map[string]interface{}, []interface{}:
		// Serialize objects and arrays as JSON for metadata field
		j, err := json.Marshal(value)
		if err != nil {
			return ""
		}
		return string(j)
	default:
		return fmt.Sprintf("%v", value)
	}
}

// stripQuotes removes surrounding quotes from a string.
func stripQuotes(value string) string {
	value = strings.TrimSpace(value)
	if len(value) >= 2 {
		if (value[0] == '"' && value[len(value)-1] == '"') ||
			(value[0] == '\'' && value[len(value)-1] == '\'') {
			return value[1 : len(value)-1]
		}
	}
	return value
}

// ParseOpenClawMetadata extracts OpenClaw-specific metadata from the frontmatter.
// It parses the metadata field as JSON5 and extracts the goclaw/openclaw section.
func ParseOpenClawMetadata(frontmatter ParsedFrontmatter) *OpenClawSkillMetadata {
	raw, ok := frontmatter["metadata"]
	if !ok || raw == "" {
		return nil
	}

	// Try to parse as JSON (JSON5-compatible for our purposes)
	var parsed map[string]interface{}
	if err := json.Unmarshal([]byte(raw), &parsed); err != nil {
		// Try with quotes around keys for YAML-like JSON
		return nil
	}

	if parsed == nil {
		return nil
	}

	// Check for manifest key in order: goclaw, openclaw
	manifestKeys := []string{ManifestKey, "openclaw"}
	var metadataRaw interface{}
	for _, key := range manifestKeys {
		if val, ok := parsed[key]; ok {
			if obj, ok := val.(map[string]interface{}); ok && obj != nil {
				metadataRaw = obj
				break
			}
		}
	}

	if metadataRaw == nil {
		return nil
	}

	metadataObj, ok := metadataRaw.(map[string]interface{})
	if !ok {
		return nil
	}

	result := &OpenClawSkillMetadata{}

	// Parse simple fields
	if v, ok := metadataObj["always"].(bool); ok {
		result.Always = v
	}
	if v, ok := metadataObj["skillKey"].(string); ok {
		result.SkillKey = v
	}
	if v, ok := metadataObj["primaryEnv"].(string); ok {
		result.PrimaryEnv = v
	}
	if v, ok := metadataObj["emoji"].(string); ok {
		result.Emoji = v
	}
	if v, ok := metadataObj["homepage"].(string); ok {
		result.Homepage = v
	}

	// Parse OS list
	if v := normalizeStringList(metadataObj["os"]); len(v) > 0 {
		result.OS = v
	}

	// Parse requires
	if requiresRaw, ok := metadataObj["requires"].(map[string]interface{}); ok && requiresRaw != nil {
		result.Requires = &SkillRequires{
			Bins:    normalizeStringList(requiresRaw["bins"]),
			AnyBins: normalizeStringList(requiresRaw["anyBins"]),
			Env:     normalizeStringList(requiresRaw["env"]),
			Config:  normalizeStringList(requiresRaw["config"]),
		}
	}

	// Parse install specs
	if installRaw, ok := metadataObj["install"].([]interface{}); ok && len(installRaw) > 0 {
		for _, spec := range installRaw {
			if parsed := parseInstallSpec(spec); parsed != nil {
				result.Install = append(result.Install, *parsed)
			}
		}
	}

	// Return nil if empty
	if result.Always || result.SkillKey != "" || result.Emoji != "" ||
		result.Homepage != "" || result.PrimaryEnv != "" ||
		len(result.OS) > 0 || result.Requires != nil || len(result.Install) > 0 {
		return result
	}
	return nil
}

// parseInstallSpec parses a single install specification.
func parseInstallSpec(raw interface{}) *SkillInstallSpec {
	if raw == nil {
		return nil
	}

	obj, ok := raw.(map[string]interface{})
	if !ok {
		return nil
	}

	spec := &SkillInstallSpec{}

	// Determine kind - check both kind and type for legacy compatibility
	var kindRaw interface{}
	if k, ok := obj["kind"]; ok {
		kindRaw = k
	} else if k, ok := obj["type"]; ok {
		kindRaw = k
	}

	kind := ""
	if k, ok := kindRaw.(string); ok {
		kind = strings.ToLower(strings.TrimSpace(k))
	}

	// Validate kind
	if kind != "brew" && kind != "node" && kind != "go" &&
		kind != "uv" && kind != "download" {
		return nil
	}
	spec.Kind = kind

	// Parse optional fields
	if v, ok := obj["id"].(string); ok {
		spec.ID = v
	}
	if v, ok := obj["label"].(string); ok {
		spec.Label = v
	}
	if v := normalizeStringList(obj["bins"]); len(v) > 0 {
		spec.Bins = v
	}
	if v := normalizeStringList(obj["os"]); len(v) > 0 {
		spec.OS = v
	}
	if v, ok := obj["formula"].(string); ok {
		spec.Formula = v
	}
	if v, ok := obj["package"].(string); ok {
		spec.Package = v
	}
	if v, ok := obj["module"].(string); ok {
		spec.Module = v
	}
	if v, ok := obj["url"].(string); ok {
		spec.URL = v
	}
	if v, ok := obj["archive"].(string); ok {
		spec.Archive = v
	}
	if v, ok := obj["extract"].(bool); ok {
		spec.Extract = v
	}
	if v, ok := obj["stripComponents"].(float64); ok {
		spec.StripComponents = int(v)
	}
	if v, ok := obj["targetDir"].(string); ok {
		spec.TargetDir = v
	}
	// Store type as legacy alias
	if v, ok := obj["type"].(string); ok {
		spec.Type = v
	}

	return spec
}

// ParseSkillInvocationPolicy extracts invocation policy from frontmatter.
func ParseSkillInvocationPolicy(frontmatter ParsedFrontmatter) SkillInvocationPolicy {
	return SkillInvocationPolicy{
		UserInvocable:          parseFrontmatterBool(frontmatter["user-invocable"], true),
		DisableModelInvocation: parseFrontmatterBool(frontmatter["disable-model-invocation"], false),
	}
}

// parseFrontmatterBool parses a boolean value from a frontmatter string.
func parseFrontmatterBool(value string, fallback bool) bool {
	parsed := parseBooleanValue(value)
	if parsed == nil {
		return fallback
	}
	return *parsed
}

// parseBooleanValue parses a boolean value from various input types.
func parseBooleanValue(value interface{}) *bool {
	switch v := value.(type) {
	case bool:
		return &v
	case string:
		normalized := strings.ToLower(strings.TrimSpace(v))
		if normalized == "" {
			return nil
		}
		// Truthy values
		if normalized == "true" || normalized == "1" || normalized == "yes" || normalized == "on" {
			trueVal := true
			return &trueVal
		}
		// Falsy values
		if normalized == "false" || normalized == "0" || normalized == "no" || normalized == "off" {
			falseVal := false
			return &falseVal
		}
		return nil
	default:
		return nil
	}
}

// normalizeStringList converts various input types to a string slice.
// Handles arrays, comma-separated strings, and single values.
func normalizeStringList(input interface{}) []string {
	if input == nil {
		return nil
	}

	switch v := input.(type) {
	case []interface{}:
		var result []string
		for _, item := range v {
			if str, ok := item.(string); ok {
				str = strings.TrimSpace(str)
				if str != "" {
					result = append(result, str)
				}
			}
		}
		return result
	case []string:
		var result []string
		for _, str := range v {
			str = strings.TrimSpace(str)
			if str != "" {
				result = append(result, str)
			}
		}
		return result
	case string:
		parts := strings.Split(v, ",")
		var result []string
		for _, part := range parts {
			part = strings.TrimSpace(part)
			if part != "" {
				result = append(result, part)
			}
		}
		return result
	default:
		return nil
	}
}

// ResolveSkillKey generates a consistent skill key for config lookups.
// Uses metadata.skillKey if available, otherwise falls back to skillName.
func ResolveSkillKey(skillName string, metadata *OpenClawSkillMetadata) string {
	if metadata != nil && metadata.SkillKey != "" {
		return metadata.SkillKey
	}
	return skillName
}

// GetFrontmatterValue safely retrieves a string value from frontmatter.
func GetFrontmatterValue(frontmatter ParsedFrontmatter, key string) string {
	return frontmatter[key]
}

// GetFrontmatterValueJSON extracts a JSON value using gjson path notation.
// This allows accessing nested values within YAML/JSON frontmatter fields.
func GetFrontmatterValueJSON(frontmatter ParsedFrontmatter, key, gjsonPath string) (gjson.Result, bool) {
	value, ok := frontmatter[key]
	if !ok {
		return gjson.Result{}, false
	}
	result := gjson.Get(value, gjsonPath)
	return result, result.Exists()
}

// parseKeyValueLine parses a single key:value line, returning the key and value.
// Handles keys with word characters and hyphens.
func parseKeyValueLine(line string) (key, value string) {
	// Find the colon separator
	colonIdx := strings.Index(line, ":")
	if colonIdx == -1 {
		return "", ""
	}

	// Extract key (before colon, may include hyphens)
	key = strings.TrimSpace(line[:colonIdx])
	// Validate key contains only valid characters
	for _, r := range key {
		if !unicode.IsLetter(r) && !unicode.IsDigit(r) && r != '-' && r != '_' {
			return "", ""
		}
	}

	// Extract value (after colon)
	value = strings.TrimSpace(line[colonIdx+1:])
	return key, value
}

// StripFrontmatter removes the frontmatter block and returns the body content.
func StripFrontmatter(content string) string {
	normalized := normalizeNewlines(content)

	if !strings.HasPrefix(normalized, "---") {
		return normalized
	}

	endIndex := strings.Index(normalized[3:], "\n---")
	if endIndex == -1 {
		return normalized
	}

	// endIndex is relative to normalized[3:], so add back for position in normalized string
	// We need to skip past "\n---" which is endIndex + 3 + 4 = endIndex + 7
	body := normalized[endIndex+7:]
	return strings.TrimSpace(body)
}
