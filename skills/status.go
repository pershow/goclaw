package skills

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
)

// SkillStatusConfigCheck represents a configuration check for a skill
type SkillStatusConfigCheck struct {
	Path       string      `json:"path"`
	Value      interface{} `json:"value"`
	Satisfied  bool        `json:"satisfied"`
}

// SkillInstallOption represents an install option for a skill
type SkillInstallOption struct {
	ID    string `json:"id"`
	Kind  string `json:"kind"`
	Label string `json:"label"`
	Bins  []string `json:"bins"`
}

// SkillStatusEntry represents the status of a single skill
type SkillStatusEntry struct {
	Name             string   `json:"name"`
	Description      string   `json:"description"`
	Source           string   `json:"source"`
	Bundled          bool     `json:"bundled"`
	FilePath         string   `json:"filePath"`
	BaseDir          string   `json:"baseDir"`
	SkillKey         string   `json:"skillKey"`
	PrimaryEnv       *string   `json:"primaryEnv,omitempty"`
	Emoji            *string   `json:"emoji,omitempty"`
	Homepage         *string   `json:"homepage,omitempty"`
	Always           bool     `json:"always"`
	Disabled         bool     `json:"disabled"`
	BlockedByAllowlist bool   `json:"blockedByAllowlist"`
	Eligible         bool     `json:"eligible"`

	// Requirements
	Requirements struct {
		Bins     []string `json:"bins"`
		AnyBins  []string `json:"anyBins"`
		Env      []string `json:"env"`
		Config   []string `json:"config"`
		OS       []string `json:"os"`
	} `json:"requirements"`

	// Missing dependencies
	Missing struct {
		Bins     []string `json:"bins"`
		AnyBins  []string `json:"anyBins"`
		Env      []string `json:"env"`
		Config   []string `json:"config"`
		OS       []string `json:"os"`
	} `json:"missing"`

	ConfigChecks []SkillStatusConfigCheck `json:"configChecks"`
	Install      []SkillInstallOption     `json:"install"`
}

// SkillStatusReport represents a full status report
type SkillStatusReport struct {
	WorkspaceDir    string             `json:"workspaceDir"`
	ManagedSkillsDir string            `json:"managedSkillsDir"`
	Skills          []SkillStatusEntry `json:"skills"`
}

// StatusBuilder builds skill status reports
type StatusBuilder struct {
	Loader       SkillLoader
	Config       SkillsConfig
	WorkspaceDir string
	AgentDir     string
}

// NewStatusBuilder creates a new status builder
func NewStatusBuilder(loader SkillLoader, config SkillsConfig, workspaceDir, agentDir string) *StatusBuilder {
	return &StatusBuilder{
		Loader: loader,
		Config: config,
		WorkspaceDir: workspaceDir,
		AgentDir: agentDir,
	}
}

// BuildStatus builds a complete skill status report
func (b *StatusBuilder) BuildStatus(ctx context.Context) (*SkillStatusReport, error) {
	// Load all skills
	opts := LoadSkillsOptions{
		Cwd:              b.WorkspaceDir,
		AgentDir:         b.AgentDir,
		IncludeDefaults: true,
	}

	result, err := b.Loader.Load(ctx, opts)
	if err != nil {
		return nil, fmt.Errorf("failed to load skills: %w", err)
	}

	// Convert to entries for processing
	var entries []*SkillEntry
	for _, skill := range result.Skills {
		entry := &SkillEntry{
			Skill:            skill,
			Frontmatter:      skill.Frontmatter,
			Metadata:         skill.Metadata,
			InvocationPolicy: &skill.InvocationPolicy,
		}
		entries = append(entries, entry)
	}

	// Generate status for each entry
	var statusEntries []SkillStatusEntry
	for _, entry := range entries {
		statusEntry := b.buildSkillStatus(entry)
		statusEntries = append(statusEntries, statusEntry)
	}

	// Sort by name
	sort.Slice(statusEntries, func(i, j int) bool {
		return statusEntries[i].Name < statusEntries[j].Name
	})

	return &SkillStatusReport{
		WorkspaceDir:     b.WorkspaceDir,
		ManagedSkillsDir: filepath.Join(b.AgentDir, "skills"),
		Skills:          statusEntries,
	}, nil
}

// buildSkillStatus builds status for a single skill
func (b *StatusBuilder) buildSkillStatus(entry *SkillEntry) SkillStatusEntry {
	skillKey := resolveSkillKey(entry.Skill.Name, entry.Metadata)
	disabled := !entry.IsEnabled(b.Config)
	blockedByAllowlist := b.isBlockedByAllowlist(entry)
	always := entry.Metadata != nil && entry.Metadata.Always

	// Extract requirements
	requirements := b.extractRequirements(entry)

	// Check what's missing
	missing := b.checkMissingDependencies(entry, requirements)

	// Check config paths
	configChecks := b.checkConfigPaths(entry)

	// Check eligibility
	eligible := b.checkEligibility(entry, disabled, blockedByAllowlist, always, missing)

	// Generate install options
	installOptions := b.generateInstallOptions(entry)

	status := SkillStatusEntry{
		Name:              entry.Skill.Name,
		Description:       entry.Skill.Description,
		Source:            entry.Skill.Source,
		Bundled:           entry.Skill.Source == "bundled",
		FilePath:          entry.Skill.FilePath,
		BaseDir:           entry.Skill.BaseDir,
		SkillKey:          skillKey,
		Always:            always,
		Disabled:          disabled,
		BlockedByAllowlist: blockedByAllowlist,
		Eligible:          eligible,
		Install:           installOptions,
		ConfigChecks:      configChecks,
	}

	// Set optional fields
	if entry.Metadata != nil {
		if entry.Metadata.PrimaryEnv != "" {
			status.PrimaryEnv = &entry.Metadata.PrimaryEnv
		}
		if entry.Metadata.Emoji != "" {
			status.Emoji = &entry.Metadata.Emoji
		}
		if entry.Metadata.Homepage != "" {
			status.Homepage = &entry.Metadata.Homepage
		}
	}

	// Set requirements and missing
	status.Requirements.Bins = requirements.Bins
	status.Requirements.AnyBins = requirements.AnyBins
	status.Requirements.Env = requirements.Env
	status.Requirements.Config = requirements.Config
	status.Requirements.OS = requirements.OS

	status.Missing.Bins = missing.Bins
	status.Missing.AnyBins = missing.AnyBins
	status.Missing.Env = missing.Env
	status.Missing.Config = missing.Config
	status.Missing.OS = missing.OS

	return status
}

// extractRequirements extracts requirements from skill metadata
func (b *StatusBuilder) extractRequirements(entry *SkillEntry) struct {
	Bins     []string
	AnyBins  []string
	Env      []string
	Config   []string
	OS       []string
} {
	var requirements struct {
		Bins     []string
		AnyBins  []string
		Env      []string
		Config   []string
		OS       []string
	}

	if entry.Metadata != nil && entry.Metadata.Requires != nil {
		requirements.Bins = entry.Metadata.Requires.Bins
		requirements.AnyBins = entry.Metadata.Requires.AnyBins
		requirements.Env = entry.Metadata.Requires.Env
		requirements.Config = entry.Metadata.Requires.Config

		if entry.Metadata.OS != nil {
			requirements.OS = entry.Metadata.OS
		}
	}

	return requirements
}

// checkMissingDependencies checks what requirements are missing
func (b *StatusBuilder) checkMissingDependencies(
	entry *SkillEntry,
	requirements struct {
		Bins     []string
		AnyBins  []string
		Env      []string
		Config   []string
		OS       []string
	},
) struct {
	Bins     []string
	AnyBins  []string
	Env      []string
	Config   []string
	OS       []string
} {
	var missing struct {
		Bins     []string
		AnyBins  []string
		Env      []string
		Config   []string
		OS       []string
	}

	// Check binary dependencies
	for _, bin := range requirements.Bins {
		if _, err := exec.LookPath(bin); err != nil {
			missing.Bins = append(missing.Bins, bin)
		}
	}

	// Check anyBins requirements
	if len(requirements.AnyBins) > 0 {
		anyFound := false
		for _, bin := range requirements.AnyBins {
			if _, err := exec.LookPath(bin); err == nil {
				anyFound = true
				break
			}
		}
		if !anyFound {
			missing.AnyBins = requirements.AnyBins
		}
	}

	// Check environment variables
	for _, envName := range requirements.Env {
		if os.Getenv(envName) == "" {
			// Check if env is provided via skill config
			if !b.hasSkillEnv(entry, envName) {
				missing.Env = append(missing.Env, envName)
			}
		}
	}

	// Check OS compatibility
	if len(requirements.OS) > 0 {
		currentOS := runtime.GOOS
		osCompatible := false
		for _, osName := range requirements.OS {
			if osName == currentOS {
				osCompatible = true
				break
			}
		}
		if !osCompatible {
			missing.OS = requirements.OS
		}
	}

	return missing
}

// checkConfigPaths checks configuration file requirements
func (b *StatusBuilder) checkConfigPaths(entry *SkillEntry) []SkillStatusConfigCheck {
	var checks []SkillStatusConfigCheck

	if entry.Metadata != nil && entry.Metadata.Requires != nil {
		for _, configPath := range entry.Metadata.Requires.Config {
			value := b.resolveConfigPath(configPath)
			satisfied := b.isConfigPathTruthy(configPath)
			checks = append(checks, SkillStatusConfigCheck{
				Path:      configPath,
				Value:     value,
				Satisfied: satisfied,
			})
		}
	}

	return checks
}

// checkEligibility determines if a skill is eligible for use
func (b *StatusBuilder) checkEligibility(
	entry *SkillEntry,
	disabled bool,
	blockedByAllowlist bool,
	always bool,
	missing struct {
		Bins     []string
		AnyBins  []string
		Env      []string
		Config   []string
		OS       []string
	},
) bool {
	// Check if explicitly disabled or blocked
	if disabled || blockedByAllowlist {
		return false
	}

	// Always-skills are always eligible
	if always {
		return true
	}

	// Check if all dependencies are satisfied
	return len(missing.Bins) == 0 &&
		len(missing.AnyBins) == 0 &&
		len(missing.Env) == 0 &&
		len(missing.Config) == 0 &&
		len(missing.OS) == 0
}

// generateInstallOptions generates install options for a skill
func (b *StatusBuilder) generateInstallOptions(entry *SkillEntry) []SkillInstallOption {
	if entry.Metadata == nil || len(entry.Metadata.Install) == 0 {
		return nil
	}

	// Filter install specs for current platform
	var filtered []SkillInstallSpec
	for _, spec := range entry.Metadata.Install {
		if len(spec.OS) == 0 || contains(spec.OS, runtime.GOOS) {
			filtered = append(filtered, spec)
		}
	}

	if len(filtered) == 0 {
		return nil
	}

	// Select preferred install spec
	preferred := b.selectPreferredInstallSpec(filtered)
	if preferred == nil {
		return nil
	}

	// Convert to options
	options := []SkillInstallOption{b.buildInstallOption(preferred)}

	// Include downloads as backup options
	for _, spec := range filtered {
		if spec.Kind == "download" {
			options = append(options, b.buildInstallOption(&spec))
		}
	}

	return options
}

// selectPreferredInstallSpec selects the best installation spec
func (b *StatusBuilder) selectPreferredInstallSpec(specs []SkillInstallSpec) *SkillInstallSpec {
	if len(specs) == 0 {
		return nil
	}

	// Check preferences from config
	prefs := b.Config.Install

	// Search in order of preference
	for _, kind := range []string{"brew", "uv", "node", "go", "download"} {
		for _, spec := range specs {
			if spec.Kind == kind {
				// Check if this package manager is available
				if (kind == "brew" && !prefs.PreferBrew) ||
				   func() bool {
					   _, err := exec.LookPath(kind)
					   return err != nil
				   }() {
					continue
				}
				return &spec
			}
		}
	}

	// Return the first available
	return &specs[0]
}

// buildInstallOption builds an install option from a spec
func (b *StatusBuilder) buildInstallOption(spec *SkillInstallSpec) SkillInstallOption {
	option := SkillInstallOption{
		ID:   spec.ID,
		Kind: spec.Kind,
		Bins: spec.Bins,
	}

	// Build label
	if spec.Label != "" {
		option.Label = spec.Label
	} else {
		switch spec.Kind {
		case "brew":
			option.Label = fmt.Sprintf("Install %s (brew)", spec.Formula)
		case "node":
			manager := b.Config.Install.NodeManager
			option.Label = fmt.Sprintf("Install %s (%s)", spec.Package, manager)
		case "go":
			option.Label = fmt.Sprintf("Install %s (go)", spec.Module)
		case "uv":
			option.Label = fmt.Sprintf("Install %s (uv)", spec.Package)
		case "download":
			fileName := filepath.Base(spec.URL)
			option.Label = fmt.Sprintf("Download %s", fileName)
		default:
			option.Label = "Run installer"
		}
	}

	// Ensure ID
	if option.ID == "" {
		option.ID = fmt.Sprintf("%s-%s", spec.Kind, spec.Formula)
	}

	return option
}

// Helper functions

func (b *StatusBuilder) isBlockedByAllowlist(entry *SkillEntry) bool {
	if entry.Skill.Source != "bundled" {
		return false
	}
	if len(b.Config.AllowBundled) == 0 {
		return false
	}
	for _, allowed := range b.Config.AllowBundled {
		if allowed == entry.Skill.Name {
			return false
		}
	}
	return true
}

func (b *StatusBuilder) hasSkillEnv(entry *SkillEntry, envName string) bool {
	if b.Config.Entries == nil {
		return false
	}
	if entryConfig, ok := b.Config.Entries[entry.Skill.Name]; ok && entryConfig.Env != nil {
		if val, ok := entryConfig.Env[envName]; ok && val != "" {
			return true
		}
	}
	return false
}

func (b *StatusBuilder) resolveConfigPath(configPath string) interface{} {
	// TODO: Implement config path resolution
	return nil
}

func (b *StatusBuilder) isConfigPathTruthy(configPath string) bool {
	// TODO: Implement config path truthiness checking
	return false
}

func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}