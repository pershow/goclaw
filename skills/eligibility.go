package skills

import (
	"path/filepath"
	"regexp"
	"strings"
)

// PatternFilter implements pi-mono style pattern-based skill filtering
// Supports !pattern (exclude), +path (force include), -path (force exclude)
type PatternFilter struct {
	// Raw patterns from config
	RawPatterns []string

	// Compiled patterns
	IncludePatterns   []*regexp.Regexp
	ExcludePatterns   []*regexp.Regexp
	ForceIncludePaths []string
	ForceExcludePaths []string

	// Files to always skip
	AlwaysSkipPatterns []*regexp.Regexp
}

// NewPatternFilter creates a new pattern filter from configuration
func NewPatternFilter(patterns []string) *PatternFilter {
	filter := &PatternFilter{
		RawPatterns: patterns,
		AlwaysSkipPatterns: compilePatterns([]string{
			`^\.git$`,
			`^node_modules$`,
			`^\.DS_Store$`,
			`^\..*`, // Hidden files/dirs
		}),
	}

	filter.compilePatterns()
	return filter
}

// compilePatterns compiles patterns into their respective categories
func (f *PatternFilter) compilePatterns() {
	var includePatterns, excludePatterns []string

	for _, pattern := range f.RawPatterns {
		pattern = strings.TrimSpace(pattern)
		if pattern == "" {
			continue
		}

		switch {
		case strings.HasPrefix(pattern, "!"):
			// Exclusion pattern: !pattern
			excludePatterns = append(excludePatterns, strings.TrimPrefix(pattern, "!"))
		case strings.HasPrefix(pattern, "+"):
			// Force inclusion pattern: +pattern
			f.ForceIncludePaths = append(f.ForceIncludePaths, strings.TrimPrefix(pattern, "+"))
		case strings.HasPrefix(pattern, "-"):
			// Force exclusion pattern: -pattern
			f.ForceExcludePaths = append(f.ForceExcludePaths, strings.TrimPrefix(pattern, "-"))
		default:
			// Regular inclusion pattern
			includePatterns = append(includePatterns, pattern)
		}
	}

	f.IncludePatterns = compilePatterns(includePatterns)
	f.ExcludePatterns = compilePatterns(excludePatterns)
}

// compilePatterns converts string patterns to regex patterns
func compilePatterns(patterns []string) []*regexp.Regexp {
	var compiled []*regexp.Regexp

	for _, pattern := range patterns {
		// Convert glob-like patterns to regex
		regexPattern := globToRegex(pattern)
		if !strings.HasPrefix(regexPattern, "^") {
			regexPattern = ".*" + regexPattern
		}
		if !strings.HasSuffix(regexPattern, "$") {
			regexPattern += ".*"
		}

		if re, err := regexp.Compile(regexPattern); err == nil {
			compiled = append(compiled, re)
		}
	}

	return compiled
}

// globToRegex converts glob-like patterns to regex patterns
func globToRegex(glob string) string {
	// Escape regex special characters
	regex := regexp.QuoteMeta(glob)

	// Convert glob patterns to regex
	regex = strings.ReplaceAll(regex, `*`, ".*")
	regex = strings.ReplaceAll(regex, `?`, ".")
	regex = strings.ReplaceAll(regex, `\[\\\]`, "[\\]")
	regex = strings.ReplaceAll(regex, `\{\\\}`, "\\{\\}")

	return regex
}

// ShouldInclude determines if a skill path should be included
func (f *PatternFilter) ShouldInclude(skillPath string) bool {
	// Check force exclusions first (highest priority)
	if f.isForceExcluded(skillPath) {
		return false
	}

	// Check force inclusions (override everything except force exclusions)
	if f.isForceIncluded(skillPath) {
		return true
	}

	// Check always-skip patterns
	if f.isAlwaysSkipped(skillPath) {
		return false
	}

	// Check explicit exclusions
	if f.isExcluded(skillPath) {
		return false
	}

	// Check inclusions (if any patterns exist, must match at least one)
	if len(f.IncludePatterns) > 0 && !f.isIncluded(skillPath) {
		return false
	}

	return true
}

// isForceIncluded checks if path matches a force inclusion pattern
func (f *PatternFilter) isForceIncluded(path string) bool {
	for _, forcePath := range f.ForceIncludePaths {
		if f.pathMatches(path, forcePath) {
			return true
		}
	}
	return false
}

// isForceExcluded checks if path matches a force exclusion pattern
func (f *PatternFilter) isForceExcluded(path string) bool {
	for _, forcePath := range f.ForceExcludePaths {
		if f.pathMatches(path, forcePath) {
			return true
		}
	}
	return false
}

// isAlwaysSkipped checks if path matches always-skip patterns
func (f *PatternFilter) isAlwaysSkipped(path string) bool {
	base := filepath.Base(path)
	for _, pattern := range f.AlwaysSkipPatterns {
		if pattern.MatchString(base) {
			return true
		}
	}
	return false
}

// isIncluded checks if path matches inclusion patterns
func (f *PatternFilter) isIncluded(path string) bool {
	if len(f.IncludePatterns) == 0 {
		return true // No inclusion patterns means include all
	}

	for _, pattern := range f.IncludePatterns {
		if pattern.MatchString(path) {
			return true
		}
	}
	return false
}

// isExcluded checks if path matches exclusion patterns
func (f *PatternFilter) isExcluded(path string) bool {
	for _, pattern := range f.ExcludePatterns {
		if pattern.MatchString(path) {
			return true
		}
	}
	return false
}

// pathMatches checks if file path matches a pattern path
func (f *PatternFilter) pathMatches(filePath, patternPath string) bool {
	// Convert both to absolute paths for comparison
	absFilePath, _ := filepath.Abs(filePath)
	absPatternPath, _ := filepath.Abs(patternPath)

	// Direct path match
	if absFilePath == absPatternPath {
		return true
	}

	// Directory contains pattern path or vice versa
	return strings.Contains(absFilePath, absPatternPath) ||
		strings.Contains(absPatternPath, absFilePath)
}

// PatternFilterConfig holds configuration for pattern-based filtering
type PatternFilterConfig struct {
	// Patterns for filtering skills
	Patterns []string `json:"patterns" yaml:"patterns"`

	// Minimum priority to include (higher number = higher priority)
	MinPriority int `json:"minPriority" yaml:"minPriority"`

	// Maximum priority to include
	MaxPriority int `json:"maxPriority" yaml:"maxPriority"`

	// Include skills without priority
	IncludeUnprioritized bool `json:"includeUnprioritized" yaml:"includeUnprioritized"`
}

// PriorityFilter filters skills based on priority values
type PriorityFilter struct {
	Config PatternFilterConfig
}

// NewPriorityFilter creates a new priority filter
func NewPriorityFilter(config PatternFilterConfig) *PriorityFilter {
	return &PriorityFilter{Config: config}
}

// ShouldInclude determines if a skill should be included based on priority
func (f *PriorityFilter) ShouldInclude(skill *SkillEntry, metadata *OpenClawSkillMetadata) bool {
	if metadata == nil {
		// Skill has no priority metadata
		return f.Config.IncludeUnprioritized
	}

	// Extract priority from metadata
	priority := f.extractPriority(metadata)

	// Check against min/max bounds
	if f.Config.MinPriority > 0 && priority < f.Config.MinPriority {
		return false
	}
	if f.Config.MaxPriority > 0 && priority > f.Config.MaxPriority {
		return false
	}

	return true
}

// extractPriority extracts the priority value from metadata
func (f *PriorityFilter) extractPriority(metadata *OpenClawSkillMetadata) int {
	// Priority can be stored in various metadata fields
	// Default priority if not specified
	defaultPriority := 50

	// Check for priority field in metadata
	// This depends on how priority is stored in your metadata structure
	// For now, return default priority
	return defaultPriority
}

// CombinedFilter combines pattern and priority filtering
type CombinedFilter struct {
	PatternFilter  *PatternFilter
	PriorityFilter *PriorityFilter
	Config         SkillsConfig
}

// NewCombinedFilter creates a combined filter
func NewCombinedFilter(config SkillsConfig) *CombinedFilter {
	patternConfig := PatternFilterConfig{
		Patterns:             config.Load.ExtraPatterns,
		MinPriority:          config.Filter.MinPriority,
		MaxPriority:          config.Filter.MaxPriority,
		IncludeUnprioritized: config.Filter.IncludeUnprioritized,
	}

	return &CombinedFilter{
		PatternFilter:  NewPatternFilter(config.Load.ExtraPatterns),
		PriorityFilter: NewPriorityFilter(patternConfig),
		Config:         config,
	}
}

// FilterSkills applies all filters to a list of skills
func (f *CombinedFilter) FilterSkills(skills []*SkillEntry) []*SkillEntry {
	var filtered []*SkillEntry

	for _, skill := range skills {
		if f.ShouldInclude(skill) {
			filtered = append(filtered, skill)
		}
	}

	return filtered
}

// ShouldInclude determines if a skill passes all filter criteria
func (f *CombinedFilter) ShouldInclude(entry *SkillEntry) bool {
	// Check pattern-based filtering
	if f.PatternFilter != nil && !f.PatternFilter.ShouldInclude(entry.Skill.FilePath) {
		return false
	}

	// Check priority-based filtering
	if f.PriorityFilter != nil && !f.PriorityFilter.ShouldInclude(entry, entry.Metadata) {
		return false
	}

	// Check individual skill configuration
	if !entry.IsEnabled(f.Config) {
		return false
	}

	return true
}

// FilterConfig extends SkillsConfig with filtering options
type FilterConfig struct {
	// Minimum priority to include
	MinPriority int `json:"minPriority" yaml:"minPriority"`

	// Maximum priority to include
	MaxPriority int `json:"maxPriority" yaml:"maxPriority"`

	// Include skills without priority
	IncludeUnprioritized bool `json:"includeUnprioritized" yaml:"includeUnprioritized"`
}

// Extend SkillsConfig with filtering fields
func (c *SkillsConfig) GetPatterns() []string {
	// Extract patterns from configuration
	var patterns []string

	// Check for patterns in skills configuration
	// This depends on your configuration structure

	return patterns
}

// String returns a string representation of the pattern filter for debugging
func (f *PatternFilter) String() string {
	var parts []string

	if len(f.RawPatterns) > 0 {
		parts = append(parts, "Patterns: "+strings.Join(f.RawPatterns, ", "))
	}
	if len(f.ForceIncludePaths) > 0 {
		parts = append(parts, "Force Includes: "+strings.Join(f.ForceIncludePaths, ", "))
	}
	if len(f.ForceExcludePaths) > 0 {
		parts = append(parts, "Force Excludes: "+strings.Join(f.ForceExcludePaths, ", "))
	}

	if len(parts) == 0 {
		return "PatternFilter{no patterns}"
	}

	return "PatternFilter{" + strings.Join(parts, "; ") + "}"
}

// IsEmpty checks if the filter has any active patterns
func (f *PatternFilter) IsEmpty() bool {
	return len(f.RawPatterns) == 0 &&
		len(f.ForceIncludePaths) == 0 &&
		len(f.ForceExcludePaths) == 0
}
