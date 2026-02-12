package skills

import (
	"context"
	"fmt"
	"sort"
	"strings"
)

// API provides gateway-style API methods for skill management
type API struct {
	Loader       SkillLoader
	Config       SkillsConfig
	WorkspaceDir string
	AgentDir     string
	StatusBuilder *StatusBuilder
}

// NewAPI creates a new skill API instance
func NewAPI(loader SkillLoader, config SkillsConfig, workspaceDir, agentDir string) *API {
	statusBuilder := NewStatusBuilder(loader, config, workspaceDir, agentDir)
	return &API{
		Loader:        loader,
		Config:        config,
		WorkspaceDir:  workspaceDir,
		AgentDir:      agentDir,
		StatusBuilder: statusBuilder,
	}
}

// SkillsStatusRequest represents parameters for skills.status API
type SkillsStatusRequest struct {
	AgentID *string `json:"agentId,omitempty"`
}

// SkillsStatusResponse represents the response from skills.status API
type SkillsStatusResponse struct {
	Success bool               `json:"success"`
	Data    *SkillStatusReport `json:"data,omitempty"`
	Error   *string           `json:"error,omitempty"`
}

// Status builds a complete skill status report
func (a *API) Status(ctx context.Context, req SkillsStatusRequest) SkillsStatusResponse {
	// Validate request
	if err := a.validateSkillsStatusParams(req); err != nil {
		return SkillsStatusResponse{
			Success: false,
			Error:   strPtr(err.Error()),
		}
	}

	// Build status report
	report, err := a.StatusBuilder.BuildStatus(ctx)
	if err != nil {
		return SkillsStatusResponse{
			Success: false,
			Error:   strPtr(fmt.Sprintf("failed to build status: %v", err)),
		}
	}

	return SkillsStatusResponse{
		Success: true,
		Data:    report,
	}
}

// SkillsBinsRequest represents parameters for skills.bins API
type SkillsBinsRequest struct {
	// No specific parameters needed for bins API
}

// SkillsBinsResponse represents the response from skills.bins API
type SkillsBinsResponse struct {
	Success bool     `json:"success"`
	Data    struct {
		Bins []string `json:"bins"`
	} `json:"data,omitempty"`
	Error *string   `json:"error,omitempty"`
}

// Bins collects all unique binaries required by loaded skills
func (a *API) Bins(ctx context.Context, req SkillsBinsRequest) SkillsBinsResponse {
	// Load all skills to collect binaries
	opts := LoadSkillsOptions{
		Cwd:              a.WorkspaceDir,
		AgentDir:         a.AgentDir,
		IncludeDefaults: true,
	}

	result, err := a.Loader.Load(ctx, opts)
	if err != nil {
		return SkillsBinsResponse{
			Success: false,
			Error:   strPtr(fmt.Sprintf("failed to load skills: %v", err)),
		}
	}

	// Collect all unique binaries
	bins := collectSkillBins(result.Skills)

	return SkillsBinsResponse{
		Success: true,
		Data: struct {
			Bins []string `json:"bins"`
		}{
			Bins: bins,
		},
	}
}

// SkillsInstallRequest represents parameters for skills.install API
type SkillsInstallRequest struct {
	Name      string `json:"name"`
	InstallID string `json:"installId"`
	TimeoutMs *int   `json:"timeoutMs,omitempty"`
}

// SkillsInstallResponse represents the response from skills.install API
type SkillsInstallResponse struct {
	Success bool   `json:"success"`
	Data    struct {
		OK      bool    `json:"ok"`
		Message string  `json:"message"`
	} `json:"data,omitempty"`
	Error   *string `json:"error,omitempty"`
}

// Install installs a skill dependency
func (a *API) Install(ctx context.Context, req SkillsInstallRequest) SkillsInstallResponse {
	// Validate request
	if err := a.validateSkillsInstallParams(req); err != nil {
		return SkillsInstallResponse{
			Success: false,
			Error:   strPtr(err.Error()),
		}
	}

	// Look up the skill
	skill, ok := a.Loader.Get(req.Name)
	if !ok {
		return SkillsInstallResponse{
			Success: false,
			Error:   strPtr(fmt.Sprintf("skill not found: %s", req.Name)),
		}
	}

	// Find the install specification
	installSpec := a.findInstallSpec(skill, req.InstallID)
	if installSpec == nil {
		return SkillsInstallResponse{
			Success: false,
			Error:   strPtr(fmt.Sprintf("install spec not found: %s", req.InstallID)),
		}
	}

	// Execute installation
	prefs := SkillInstallPreferences{
		PreferBrew:  a.Config.Install.PreferBrew,
		NodeManager: a.Config.Install.NodeManager,
	}

	installer := NewSkillInstaller()
	result, err := installer.Install(ctx, *installSpec, prefs)
	if err != nil {
		return SkillsInstallResponse{
			Success: false,
			Error:   strPtr(fmt.Sprintf("installation failed: %v", err)),
		}
	}

	return SkillsInstallResponse{
		Success: true,
		Data: struct {
			OK      bool   `json:"ok"`
			Message string `json:"message"`
		}{
			OK:      result.Success,
			Message: result.Message,
		},
	}
}

// SkillsUpdateRequest represents parameters for skills.update API
type SkillsUpdateRequest struct {
	SkillKey string            `json:"skillKey"`
	Enabled  *bool             `json:"enabled,omitempty"`
	APIKey   *string           `json:"apiKey,omitempty"`
	Env      map[string]string `json:"env,omitempty"`
}

// SkillsUpdateResponse represents the response from skills.update API
type SkillsUpdateResponse struct {
	Success bool `json:"success"`
	Data    struct {
		OK      bool               `json:"ok"`
		SkillKey string            `json:"skillKey"`
		Config   SkillEntryConfig  `json:"config"`
	} `json:"data,omitempty"`
	Error   *string `json:"error,omitempty"`
}

// Update updates skill configuration
func (a *API) Update(ctx context.Context, req SkillsUpdateRequest) SkillsUpdateResponse {
	// Validate request
	if err := a.validateSkillsUpdateParams(req); err != nil {
		return SkillsUpdateResponse{
			Success: false,
			Error:   strPtr(err.Error()),
		}
	}

	// Load current skills
	opts := LoadSkillsOptions{
		Cwd:              a.WorkspaceDir,
		AgentDir:         a.AgentDir,
		IncludeDefaults: true,
	}

	result, err := a.Loader.Load(ctx, opts)
	if err != nil {
		return SkillsUpdateResponse{
			Success: false,
			Error:   strPtr(fmt.Sprintf("failed to load skills: %v", err)),
		}
	}

	// Find the skill entry
	var targetSkill *SkillEntry
	for _, skill := range result.Skills {
		entry := &SkillEntry{
			Skill: skill,
			Frontmatter: skill.Frontmatter,
			Metadata:    skill.Metadata,
		}

		skillKey := resolveSkillKey(skill.Name, skill.Metadata)
		if skillKey == req.SkillKey {
			targetSkill = entry
			break
		}
	}

	if targetSkill == nil {
		return SkillsUpdateResponse{
			Success: false,
			Error:   strPtr(fmt.Sprintf("skill not found: %s", req.SkillKey)),
		}
	}

	// Update configuration
	config := a.cloneSkillsConfig()
	if config.Entries == nil {
		config.Entries = make(map[string]SkillEntryConfig)
	}

	current := config.Entries[req.SkillKey]
	if req.Enabled != nil {
		current.Enabled = *req.Enabled
	}

	if req.APIKey != nil {
		key := strings.TrimSpace(*req.APIKey)
		if key == "" {
			current.ApiKey = ""
		} else {
			current.ApiKey = key
		}
	}

	if req.Env != nil {
		if current.Env == nil {
			current.Env = make(map[string]string)
		}
		for key, value := range req.Env {
			trimmedKey := strings.TrimSpace(key)
			trimmedValue := strings.TrimSpace(value)

			if trimmedKey == "" {
				continue
			}

			if trimmedValue == "" {
				delete(current.Env, trimmedKey)
			} else {
				current.Env[trimmedKey] = trimmedValue
			}
		}
	}

	config.Entries[req.SkillKey] = current

	// Save configuration (TODO: implement config persistence)
	// For now, just verify the config updates

	return SkillsUpdateResponse{
		Success: true,
		Data: struct {
			OK      bool              `json:"ok"`
			SkillKey string           `json:"skillKey"`
			Config   SkillEntryConfig `json:"config"`
		}{
			OK:       true,
			SkillKey: req.SkillKey,
			Config:   current,
		},
	}
}

// Helper functions

func (a *API) validateSkillsStatusParams(req SkillsStatusRequest) error {
	// Basic validation - agent ID can be empty
	return nil
}

func (a *API) validateSkillsInstallParams(req SkillsInstallRequest) error {
	if req.Name == "" {
		return fmt.Errorf("name is required")
	}
	if req.InstallID == "" {
		return fmt.Errorf("installId is required")
	}
	return nil
}

func (a *API) validateSkillsUpdateParams(req SkillsUpdateRequest) error {
	if req.SkillKey == "" {
		return fmt.Errorf("skillKey is required")
	}
	return nil
}

func (a *API) findInstallSpec(skill *Skill, installID string) *SkillInstallSpec {
	if skill.Metadata == nil {
		return nil
	}

	for _, spec := range skill.Metadata.Install {
		if spec.ID == installID {
			return &spec
		}
	}

	return nil
}

func (a *API) cloneSkillsConfig() SkillsConfig {
	clone := SkillsConfig{
		AllowBundled: make([]string, len(a.Config.AllowBundled)),
		Entries:      make(map[string]SkillEntryConfig),
		Load: LoadConfig{
			ExtraDirs:      make([]string, len(a.Config.Load.ExtraDirs)),
			Watch:          a.Config.Load.Watch,
			WatchDebounceMs: a.Config.Load.WatchDebounceMs,
		},
		Install: InstallConfig{
			PreferBrew:  a.Config.Install.PreferBrew,
			NodeManager: a.Config.Install.NodeManager,
		},
	}

	copy(clone.AllowBundled, a.Config.AllowBundled)

	for key, value := range a.Config.Entries {
		entry := SkillEntryConfig{
			Enabled: value.Enabled,
			ApiKey:  value.ApiKey,
		}

		if value.Env != nil {
			entry.Env = make(map[string]string)
			for k, v := range value.Env {
				entry.Env[k] = v
			}
		}

		clone.Entries[key] = entry
	}

	copy(clone.Load.ExtraDirs, a.Config.Load.ExtraDirs)

	return clone
}

func collectSkillBins(skills []*Skill) []string {
	bins := make(map[string]struct{})

	for _, skill := range skills {
		if skill.Metadata != nil && skill.Metadata.Requires != nil {
			// Add required binaries
			for _, bin := range skill.Metadata.Requires.Bins {
				bin = strings.TrimSpace(bin)
				if bin != "" {
					bins[bin] = struct{}{}
				}
			}

			// Add anyBins requirements
			for _, bin := range skill.Metadata.Requires.AnyBins {
				bin = strings.TrimSpace(bin)
				if bin != "" {
					bins[bin] = struct{}{}
				}
			}

			// Add binaries from install specs
			for _, spec := range skill.Metadata.Install {
				for _, bin := range spec.Bins {
					binStr := strings.TrimSpace(bin)
					if binStr != "" {
						bins[binStr] = struct{}{}
					}
				}
			}
		}
	}

	// Convert to sorted slice
	binSlice := make([]string, 0, len(bins))
	for bin := range bins {
		binSlice = append(binSlice, bin)
	}
	sort.Strings(binSlice)

	return binSlice
}

func strPtr(s string) *string {
	return &s
}

// NewSkillInstaller creates a new installer (placeholder implementation)
func NewSkillInstaller() SkillInstaller {
	// TODO: Implement actual installer
	return nil
}

func resolveSkillKey(skillName string, metadata *OpenClawSkillMetadata) string {
	if metadata != nil && metadata.SkillKey != "" {
		return metadata.SkillKey
	}
	return skillName
}