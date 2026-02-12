package skills

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"time"
)

// Core interfaces for the skill system

// -----------------------------------------------------------------------------
// Loading and Discovery
// -----------------------------------------------------------------------------

// SkillLoader loads and manages skills from various sources.
type SkillLoader interface {
	// Load loads skills from configured sources
	Load(ctx context.Context, opts LoadSkillsOptions) (SkillLoadResult, error)

	// Reload reloads all skills from their sources
	Reload(ctx context.Context) error

	// Get returns a skill by name
	Get(name string) (*Skill, bool)

	// List returns all loaded skills
	List() []*Skill

	// GetSnapshot returns a skill snapshot for system prompts
	GetSnapshot(ctx context.Context, opts SnapshotOptions) (SkillSnapshot, error)

	// Watch enables hot-reload for skill file changes
	Watch(ctx context.Context) (<-chan SkillChangeEvent, error)

	// FindByPath finds a skill by its file path
	FindByPath(path string) (*Skill, bool)

	// RegisterSource adds a custom skill source
	RegisterSource(source SkillSource) error

	// UnregisterSource removes a skill source
	UnregisterSource(name string) error
}

// SkillSource represents a source for loading skills.
type SkillSource interface {
	// Name returns the source identifier
	Name() string

	// Priority returns the precedence of this source
	Priority() int

	// Load loads skills from this source
	Load(ctx context.Context) ([]*SkillEntry, error)

	// Watch starts watching for changes
	Watch(ctx context.Context) (<-chan SkillSourceChange, error)
}

// SkillSourceChange represents a change in a skill source.
type SkillSourceChange struct {
	// Type indicates the change type
	Type ChangeType

	// SkillName is the affected skill
	SkillName string

	// Path is the file that changed
	Path string
}

// ChangeType enumerates possible skill changes.
type ChangeType string

const (
	// ChangeTypeCreated indicates a new skill was created
	ChangeTypeCreated ChangeType = "created"

	// ChangeTypeModified indicates a skill was modified
	ChangeTypeModified ChangeType = "modified"

	// ChangeTypeDeleted indicates a skill was deleted
	ChangeTypeDeleted ChangeType = "deleted"
)

// -----------------------------------------------------------------------------
// Validation
// -----------------------------------------------------------------------------

// SkillValidator validates skill definitions and metadata.
type SkillValidator interface {
	// ValidateSkill validates a full skill definition
	ValidateSkill(skill *Skill) []Diagnostic

	// ValidateName validates a skill name against the spec
	ValidateName(name string, parentDirName string) []Diagnostic

	// ValidateDescription validates a skill description
	ValidateDescription(description string) []Diagnostic

	// ValidateFrontmatter validates parsed frontmatter
	ValidateFrontmatter(frontmatter ParsedFrontmatter, filePath string) []Diagnostic

	// ValidateMetadata validates OpenClaw metadata
	ValidateMetadata(metadata *OpenClawSkillMetadata) []Diagnostic
}

// -----------------------------------------------------------------------------
// Installation
// -----------------------------------------------------------------------------

// SkillInstaller handles dependency installation for skills.
type SkillInstaller interface {
	// Install executes a dependency installation
	Install(ctx context.Context, spec SkillInstallSpec, prefs SkillInstallPreferences) (SkillInstallResult, error)

	// CanInstall checks if the installer supports the given spec
	CanInstall(spec SkillInstallSpec) bool

	// DetectBinary checks if a binary exists in the system PATH
	DetectBinary(bin string) (string, error)

	// InstallSkill installs a complete skill with all its dependencies
	InstallSkill(ctx context.Context, skill *SkillEntry, prefs SkillInstallPreferences) ([]SkillInstallResult, error)
}

// -----------------------------------------------------------------------------
// Frontmatter Parsing
// -----------------------------------------------------------------------------

// FrontmatterParser parses YAML/JSON frontmatter from skill files.
type FrontmatterParser interface {
	// Parse parses frontmatter from markdown content
	Parse(content string) ParsedFrontmatter

	// ParseFile reads and parses frontmatter from a file
	ParseFile(filePath string) (ParsedFrontmatter, error)

	// Strip removes frontmatter and returns the content body
	Strip(content string) string

	// ParseOpenClawMetadata extracts OpenClaw-specific metadata
	ParseOpenClawMetadata(frontmatter ParsedFrontmatter) *OpenClawSkillMetadata

	// ParseInvocationPolicy extracts invocation policy from frontmatter
	ParseInvocationPolicy(frontmatter ParsedFrontmatter) SkillInvocationPolicy
}

// -----------------------------------------------------------------------------
// Eligibility Checking
// -----------------------------------------------------------------------------

// EligibilityChecker determines if a skill should be included based on context.
type EligibilityChecker interface {
	// ShouldInclude determines if a skill passes all eligibility checks
	ShouldInclude(skill *SkillEntry, config SkillsConfig, ctx *SkillEligibilityContext) bool

	// CheckOSCompatibility checks if the skill is compatible with current OS
	CheckOSCompatibility(skill *SkillEntry) bool

	// CheckBinaryAvailability checks which required binaries are missing
	CheckBinaryAvailability(skill *SkillEntry) []string

	// CheckEnvVariables checks which required environment variables are missing
	CheckEnvVariables(skill *SkillEntry) []string

	// CheckConfigPaths checks which required config paths are missing
	CheckConfigPaths(skill *SkillEntry) []string
}

// -----------------------------------------------------------------------------
// Skill Storage (Persistence)
// -----------------------------------------------------------------------------

// SkillStorage persists and retrieves skill data.
type SkillStorage interface {
	// SaveSkill persists a skill to storage
	SaveSkill(skill *Skill) error

	// LoadSkill loads a skill from storage
	LoadSkill(name string) (*Skill, error)

	// DeleteSkill removes a skill from storage
	DeleteSkill(name string) error

	// ListSkills lists all stored skills
	ListSkills() ([]*Skill, error)

	// SaveSnapshot saves a skill snapshot for quick retrieval
	SaveSnapshot(snapshot SkillSnapshot) error

	// LoadSnapshot loads the latest skill snapshot
	LoadSnapshot() (*SkillSnapshot, error)
}

// -----------------------------------------------------------------------------
// Prompt Generation
// -----------------------------------------------------------------------------

// PromptGenerator creates skill-related prompts for the LLM.
type PromptGenerator interface {
	// GenerateSkillsSection creates the skills section for system prompt
	GenerateSkillsSection(skills []*Skill, selected []string) string

	// FormatSkillForPrompt formats a skill for inclusion in prompts
	FormatSkillForPrompt(skill *Skill, fullContent bool) string

	// GenerateSummary creates a summary of available skills
	GenerateSummary(skills []*Skill) string

	// GenerateMissingDepsWarning creates warnings about missing dependencies
	GenerateMissingDepsWarning(missing *MissingDependencies) string
}

// -----------------------------------------------------------------------------
// Command Generation
// -----------------------------------------------------------------------------

// CommandGenerator creates slash commands for skills.
type CommandGenerator interface {
	// GenerateCommands creates slash commands from loaded skills
	GenerateCommands(skills []*SkillEntry) []SkillCommandSpec

	// SanitizeCommandName sanitizes a skill name for use as a command
	SanitizeCommandName(skillName string) string

	// TruncateDescription truncates description to fit command limitations
	TruncateDescription(description string) string

	// ParseDispatchSpec extracts dispatch specifications from frontmatter
	ParseDispatchSpec(frontmatter ParsedFrontmatter) *SkillCommandDispatch
}

// -----------------------------------------------------------------------------
// Error Handling
// -----------------------------------------------------------------------------

// Common errors used throughout the skill system.
var (
	// ErrSkillNotFound indicates the requested skill doesn't exist
	ErrSkillNotFound = errors.New("skill not found")

	// ErrSkillLoadFailed indicates a skill failed to load
	ErrSkillLoadFailed = errors.New("failed to load skill")

	// ErrSkillValidationFailed indicates a skill failed validation
	ErrSkillValidationFailed = errors.New("skill validation failed")

	// ErrSkillCollision indicates a skill name conflict
	ErrSkillCollision = errors.New("skill name collision")

	// ErrSkillInstallFailed indicates dependency installation failed
	ErrSkillInstallFailed = errors.New("skill installation failed")

	// ErrSkillNotEligible indicates the skill is not eligible for current context
	ErrSkillNotEligible = errors.New("skill not eligible for current context")

	// ErrSkillWatcherFailed indicates the file watcher failed
	ErrSkillWatcherFailed = errors.New("failed to watch skill files")
)

// -----------------------------------------------------------------------------
// Utility Types
// -----------------------------------------------------------------------------

// DirectoryScanner scans directories for skill files.
type DirectoryScanner interface {
	// ScanDirectory recursively scans a directory for skill files
	ScanDirectory(dir string) ([]string, error)

	// IsSkillFile checks if a file is a skill file
	IsSkillFile(path string) bool

	// ShouldSkipPath determines if a path should be excluded from scanning
	ShouldSkipPath(path string) bool
}

// DefaultDirectoryScanner implements a basic directory scanner.
type DefaultDirectoryScanner struct{
	// IgnorePatterns is a list of patterns to ignore
	IgnorePatterns []string
}

// ScanDirectory scans a directory for skill files.
func (s *DefaultDirectoryScanner) ScanDirectory(dir string) ([]string, error) {
	var skillFiles []string

	err := filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}

		// Skip hidden files and directories
		if d.Name()[0] == '.' && d.Name() != "." && d.Name() != ".." {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}

		// Skip node_modules and other common directories
		if d.IsDir() && s.ShouldSkipPath(path) {
			return filepath.SkipDir
		}

		// Check if it's a skill file
		if !d.IsDir() && s.IsSkillFile(path) {
			skillFiles = append(skillFiles, path)
		}

		return nil
	})

	return skillFiles, err
}

// IsSkillFile checks if a file is a skill file.
func (s *DefaultDirectoryScanner) IsSkillFile(path string) bool {
	name := filepath.Base(path)
	return name == "SKILL.md" || name == "skill.md" || (filepath.Ext(name) == ".md" && filepath.Dir(path) != "")
}

// ShouldSkipPath determines if a path should be skipped.
func (s *DefaultDirectoryScanner) ShouldSkipPath(path string) bool {
	name := filepath.Base(path)
	// Always skip these directories
	if name == "node_modules" || name == ".git" || name == ".DS_Store" {
		return true
	}
	// Check ignore patterns
	for _, pattern := range s.IgnorePatterns {
		matched, _ := filepath.Match(pattern, name)
		if matched {
			return true
		}
	}
	return false
}

// -----------------------------------------------------------------------------
// Watcher Interface
// -----------------------------------------------------------------------------

// FileWatcher monitors file system changes.
type FileWatcher interface {
	// Add adds a path to watch
	Add(path string) error

	// Remove stops watching a path
	Remove(path string) error

	// Events returns a channel of file change events
	Events() <-chan FileEvent

	// Errors returns a channel of watcher errors
	Errors() <-chan error

	// Close stops the watcher
	Close() error
}

// FileEvent represents a file system change.
type FileEvent struct {
	// Op is the file operation
	Op FileOp

	// Path is the affected file path
	Path string

	// IsDir indicates if the path is a directory
	IsDir bool

	// Timestamp is when the event occurred
	Timestamp time.Time
}

// FileOp enumerates file operations.
type FileOp string

const (
	// FileOpCreate indicates a file was created
	FileOpCreate FileOp = "create"

	// FileOpWrite indicates a file was written to
	FileOpWrite FileOp = "write"

	// FileOpRemove indicates a file was removed
	FileOpRemove FileOp = "remove"

	// FileOpRename indicates a file was renamed
	FileOpRename FileOp = "rename"

	// FileOpChmod indicates a file's permissions changed
	FileOpChmod FileOp = "chmod"
)

// -----------------------------------------------------------------------------
// Factory Interfaces
// -----------------------------------------------------------------------------

// SkillFactory creates skill-related components.
type SkillFactory interface {
	// NewLoader creates a skill loader
	NewLoader(config SkillsConfig) SkillLoader

	// NewValidator creates a skill validator
	NewValidator() SkillValidator

	// NewInstaller creates a skill installer
	NewInstaller() SkillInstaller

	// NewEligibilityChecker creates an eligibility checker
	NewEligibilityChecker() EligibilityChecker

	// NewPromptGenerator creates a prompt generator
	NewPromptGenerator() PromptGenerator

	// NewCommandGenerator creates a command generator
	NewCommandGenerator() CommandGenerator
}

// DefaultFactory is the default implementation of SkillFactory.
type DefaultFactory struct{
	// Config is the skills configuration
	Config SkillsConfig
}

// NewLoader creates a new skill loader.
func (f *DefaultFactory) NewLoader(config SkillsConfig) SkillLoader {
	// Return a concrete implementation
	return nil // To be implemented
}

// NewValidator creates a new skill validator.
func (f *DefaultFactory) NewValidator() SkillValidator {
	return nil // To be implemented
}

// NewInstaller creates a new skill installer.
func (f *DefaultFactory) NewInstaller() SkillInstaller {
	return nil // To be implemented
}

// NewEligibilityChecker creates a new eligibility checker.
func (f *DefaultFactory) NewEligibilityChecker() EligibilityChecker {
	return nil // To be implemented
}

// NewPromptGenerator creates a new prompt generator.
func (f *DefaultFactory) NewPromptGenerator() PromptGenerator {
	return nil // To be implemented
}

// NewCommandGenerator creates a new command generator.
func (f *DefaultFactory) NewCommandGenerator() CommandGenerator {
	return nil // To be implemented
}
