package skills

// Skill represents a loaded skill with its metadata and content.
// It follows the Agent Skills standard (https://agentskills.io/specification).
type Skill struct {
	// Name is the skill identifier (hyphen-case, max 64 chars)
	Name string

	// Description describes what the skill does and when to use it (max 1024 chars)
	Description string

	// FilePath is the absolute path to the SKILL.md file
	FilePath string

	// BaseDir is the directory containing the skill (parent of SKILL.md)
	// All relative paths in the skill should be resolved against this directory.
	BaseDir string

	// Source indicates where the skill was loaded from
	// Possible values: "user" (global skills dir), "project" (workspace skills),
	// "path" (explicit path), "bundled" (built-in skills)
	Source string

	// Frontmatter contains the parsed YAML/JSON frontmatter from SKILL.md
	Frontmatter ParsedFrontmatter

	// Metadata contains OpenClaw/goclaw specific metadata
	Metadata *OpenClawSkillMetadata

	// InvocationPolicy controls when and how the skill can be invoked
	InvocationPolicy SkillInvocationPolicy

	// Content is the markdown body of the skill (without frontmatter)
	Content string

	// MissingDeps contains information about missing dependencies
	MissingDeps *MissingDependencies
}

// MissingDependencies tracks which dependencies a skill is missing.
type MissingDependencies struct {
	// Bins are required binary dependencies that are not available
	Bins []string

	// AnyBins are optional binary dependencies (at least one required)
	AnyBins []string

	// Env are required environment variables that are not set
	Env []string

	// PythonPkgs are Python packages that are not installed
	PythonPkgs []string

	// NodePkgs are Node.js packages that are not installed
	NodePkgs []string
}

// SkillSnapshot represents the current state of skills for agent context.
// It supports progressive disclosure - minimal info first, full content on demand.
type SkillSnapshot struct {
	// Prompt is the formatted skills section for system prompt
	Prompt string

	// Skills is a list of available skill summaries
	Skills []SkillSummary

	// ResolvedSkills contains the full skill objects (optional, for lazy loading)
	ResolvedSkills []*Skill

	// Version is incremented when skills change (for cache invalidation)
	Version int
}

// SkillSummary provides minimal information about a skill for the initial prompt.
type SkillSummary struct {
	Name       string `json:"name"`
	Description string `json:"description"`
	PrimaryEnv  string `json:"primaryEnv,omitempty"`
}

// SkillLoadResult contains the results of loading skills from a source.
type SkillLoadResult struct {
	// Skills contains all successfully loaded skills
	Skills []*Skill

	// Diagnostics contains validation warnings and errors
	Diagnostics []Diagnostic

	// Collisions tracks name conflicts between skills from different sources
	Collisions []CollisionInfo
}

// Diagnostic represents a validation issue found during skill loading.
type Diagnostic struct {
	// Type is the severity level: "warning", "error", "collision"
	Type string

	// Message is a human-readable description of the issue
	Message string

	// Path is the file path where the issue was found
	Path string

	// Collision contains details for collision-type diagnostics
	Collision *CollisionInfo
}

// CollisionInfo describes a skill name collision.
type CollisionInfo struct {
	// ResourceType is always "skill" for now
	ResourceType string

	// Name is the conflicting skill name
	Name string

	// WinnerPath is the skill file that was kept
	WinnerPath string

	// LoserPath is the skill file that was ignored
	LoserPath string
}

// SkillInstallResult contains the result of installing a dependency.
type SkillInstallResult struct {
	// Success indicates whether installation completed successfully
	Success bool

	// InstallID is the identifier of the install spec used
	InstallID string

	// Message describes the outcome
	Message string

	// Warnings contains non-fatal issues found during installation
	Warnings []string

	// InstalledBins is a list of binaries now available after installation
	InstalledBins []string
}

// SkillInstallPreferences configures how installations should be performed.
type SkillInstallPreferences struct {
	// PreferBrew indicates brew should be preferred when available
	PreferBrew bool

	// NodeManager is the npm-compatible package manager to use
	// Options: "npm", "pnpm", "yarn", "bun"
	NodeManager string
}

// SkillSourcePriority defines the precedence order for skill sources.
// Higher priority sources override lower priority ones.
type SkillSourcePriority struct {
	// ExtraDirs is the lowest priority (user-configured extra paths)
	ExtraDirs int

	// Bundled is for built-in skills
	Bundled int

	// Managed is for ~/.goclaw/skills/
	Managed int

	// Workspace is the highest priority (current project skills)
	Workspace int
}

// DefaultSourcePriorities returns the default precedence for skill sources.
func DefaultSourcePriorities() SkillSourcePriority {
	return SkillSourcePriority{
		ExtraDirs: 0,
		Bundled:   1,
		Managed:    2,
		Workspace:  3,
	}
}

// SkillEligibilityContext provides information for checking skill eligibility
// in different environments (local vs remote).
type SkillEligibilityContext struct {
	// Remote contains information about a remote execution environment
	Remote *RemoteContext
}

// RemoteContext describes a remote execution environment for skill eligibility.
type RemoteContext struct {
	// Platforms is the list of platforms available remotely
	Platforms []string

	// HasBin checks if a binary is available remotely
	HasBin func(bin string) bool

	// HasAnyBin checks if at least one of the binaries is available remotely
	HasAnyBin func(bins []string) bool

	// Note is a message to prepend when using remote context
	Note string
}

// LoadSkillsOptions configures how skills are loaded.
type LoadSkillsOptions struct {
	// Cwd is the current working directory for project-local skills
	Cwd string

	// AgentDir is the agent config directory for global skills
	AgentDir string

	// SkillsConfig is the skills-specific configuration
	SkillsConfig *SkillsConfig

	// ManagedSkillsDir is the managed skills directory location
	ManagedSkillsDir string

	// BundledSkillsDir is the bundled skills directory location
	BundledSkillsDir string

	// SkillPaths are explicit file or directory paths to load
	SkillPaths []string

	// IncludeDefaults indicates whether to include default skill directories
	IncludeDefaults bool

	// ExtraDirs are additional directories to scan for skills
	ExtraDirs []string
}

// SkillCommandSpec defines a slash-command for skill invocation.
type SkillCommandSpec struct {
	// Name is the sanitized command name
	Name string

	// SkillName is the original skill name this command invokes
	SkillName string

	// Description is shown to users (max 100 chars for Discord limits)
	Description string

	// Dispatch specifies deterministic behavior for this command
	Dispatch *SkillCommandDispatch
}

// SkillCommandDispatch specifies how to route a command invocation.
type SkillCommandDispatch struct {
	// Kind is always "tool" for now
	Kind string

	// ToolName is the name of the tool to invoke
	ToolName string

	// ArgMode specifies how to forward user arguments
	// "raw" forwards the raw args string without core parsing
	ArgMode string
}

// SkillsConfig contains configuration for the skill system.
// This mirrors the config schema structure.
type SkillsConfig struct {
	// Entries maps skill names to their specific configurations
	Entries map[string]SkillEntryConfig

	// AllowBundled is a whitelist of bundled skills to load
	// Empty means all bundled skills are allowed
	AllowBundled []string

	// Disabled is a map of skill names that are explicitly disabled
	Disabled map[string]bool

	// Load configures skill loading behavior
	Load LoadConfig

	// Install configures skill installation behavior
	Install InstallConfig

	// Filter configures skill filtering behavior
	Filter SkillsFilterConfig
}

// SkillsFilterConfig configures skill eligibility filtering
type SkillsFilterConfig struct {
	MinPriority          int
	MaxPriority          int
	IncludeUnprioritized bool
}

// SkillEntryConfig contains per-skill configuration overrides.
type SkillEntryConfig struct {
	// Enabled determines if this skill is loaded (default: true)
	Enabled bool

	// ApiKey provides an API key for the skill
	ApiKey string

	// Env provides additional environment variables for the skill
	Env map[string]string
}

// LoadConfig configures skill discovery and loading.
type LoadConfig struct {
	// ExtraDirs are additional directories to scan for skills
	ExtraDirs []string

	// ExtraPatterns are glob patterns for skill filtering (!pattern, +path, -path)
	ExtraPatterns []string

	// Watch enables hot-reload when skill files change
	Watch bool

	// WatchDebounceMs is the delay before processing file changes
	WatchDebounceMs int
}

// InstallConfig configures skill dependency installation.
type InstallConfig struct {
	// PreferBrew indicates brew should be preferred when available
	PreferBrew bool

	// NodeManager is the npm-compatible package manager to use
	// Options: "npm", "pnpm", "yarn", "bun"
	NodeManager string
}

// SkillEntry combines a skill with its parsed metadata for internal use.
// This is similar to openclaw's SkillEntry type.
type SkillEntry struct {
	// Skill is the core skill definition
	Skill *Skill

	// Frontmatter is the raw parsed frontmatter
	Frontmatter ParsedFrontmatter

	// Metadata contains OpenClaw/goclaw specific metadata
	Metadata *OpenClawSkillMetadata

	// InvocationPolicy controls when and how the skill can be invoked
	InvocationPolicy *SkillInvocationPolicy
}

// IsEnabled checks if a skill entry should be loaded based on its config.
func (e *SkillEntry) IsEnabled(config SkillsConfig) bool {
	// Check if skill is explicitly disabled in config
	if config.Entries != nil {
		if entryConfig, ok := config.Entries[e.Skill.Name]; ok {
			return entryConfig.Enabled
		}
	}
	// Check bundled skill allowlist
	if len(config.AllowBundled) > 0 && e.Skill.Source == "bundled" {
		for _, allowed := range config.AllowBundled {
			if allowed == e.Skill.Name {
				return true
			}
		}
		return false
	}
	return true
}

// PrimaryEnv returns the primary environment variable for this skill.
func (e *SkillEntry) PrimaryEnv() string {
	if e.Metadata != nil && e.Metadata.PrimaryEnv != "" {
		return e.Metadata.PrimaryEnv
	}
	// Try to find primaryEnv from frontmatter metadata
	if raw, ok := e.Frontmatter["metadata"]; ok {
		// Check goclaw section
		if val := extractJSONField(raw, "goclaw", "primaryEnv"); val != "" {
			return val
		}
		// Check openclaw section for compatibility
		if val := extractJSONField(raw, "openclaw", "primaryEnv"); val != "" {
			return val
		}
	}
	return ""
}

// extractJSONField extracts a nested field from a JSON string.
func extractJSONField(jsonStr, section, field string) string {
	// Simple key-based extraction for gclaw/openclaw metadata
	// This is a simplified version - in real implementation, use gjson or similar
	// For now, return empty and rely on the metadata parser
	return ""
}

// SkillLoader is the interface for loading and managing skills.

// SkillEligibilityChecker determines if a skill should be loaded.
type SkillEligibilityChecker interface {
	// ShouldInclude determines if a skill passes all eligibility checks
	ShouldInclude(skill *SkillEntry, config SkillsConfig, remoteCtx *SkillEligibilityContext) bool

	// CheckBinaryAvailability verifies required binaries are present
	CheckBinaryAvailability(skill *SkillEntry) []string

	// CheckEnvVariables verifies required environment variables are set
	CheckEnvVariables(skill *SkillEntry) []string

	// CheckOSCompatibility verifies the skill works on current OS
	CheckOSCompatibility(skill *SkillEntry) bool
}

// SnapshotOptions configures what to include in a skill snapshot.
type SnapshotOptions struct {
	// IncludeFullContent includes full skill content (not just summaries)
	IncludeFullContent bool

	// SelectedSkills are skills to include with full content
	SelectedSkills []string

	// MinimalMode omits verbose information
	MinimalMode bool
}

// SkillChangeEvent represents a change to a skill file.
type SkillChangeEvent struct {
	// Type is the kind of change: "created", "modified", "deleted"
	Type string

	// Path is the file that changed
	Path string

	// SkillName is the name of the affected skill
	SkillName string
}
