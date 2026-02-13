package skills

import (
	"github.com/smallnest/goclaw/config"
)

// getSkillsConfig extracts skills configuration from the main config.
func getSkillsConfig(cfg *config.Config) SkillsConfig {
	var result SkillsConfig

	if cfg == nil || cfg.Skills == nil {
		return result
	}

	// Skills is already map[string]interface{}
	skillsMap := cfg.Skills

	// Parse Entries
	if entries, ok := skillsMap["entries"].(map[string]interface{}); ok {
		result.Entries = make(map[string]SkillEntryConfig)
		for name, entry := range entries {
			if entryMap, ok := entry.(map[string]interface{}); ok {
				skillEntry := SkillEntryConfig{
					Enabled: true, // Default to enabled
				}
				if enabled, ok := entryMap["enabled"].(bool); ok {
					skillEntry.Enabled = enabled
				}
				if apiKey, ok := entryMap["apiKey"].(string); ok {
					skillEntry.ApiKey = apiKey
				}
				if env, ok := entryMap["env"].(map[string]interface{}); ok {
					skillEntry.Env = make(map[string]string)
					for key, value := range env {
						if str, ok := value.(string); ok {
							skillEntry.Env[key] = str
						}
					}
				}
				result.Entries[name] = skillEntry
			}
		}
	}

	// Parse AllowBundled
	if allowBundled, ok := skillsMap["allowBundled"].([]string); ok {
		result.AllowBundled = allowBundled
	}

	// Parse Load config
	if load, ok := skillsMap["load"].(map[string]interface{}); ok {
		loadConfig := LoadConfig{
			Watch:           false,
			WatchDebounceMs: 500,
		}
		if enabled, ok := load["watch"].(bool); ok {
			loadConfig.Watch = enabled
		}
		if debounce, ok := load["watchDebounceMs"].(int); ok {
			loadConfig.WatchDebounceMs = debounce
		}
		if extraDirs, ok := load["extraDirs"].([]string); ok {
			loadConfig.ExtraDirs = extraDirs
		}
		if extraPatterns, ok := load["extraPatterns"].([]string); ok {
			loadConfig.ExtraPatterns = extraPatterns
		}
		result.Load = loadConfig
	}

	// Parse Install config
	if install, ok := skillsMap["install"].(map[string]interface{}); ok {
		installConfig := InstallConfig{
			PreferBrew:  true,
			NodeManager: "npm",
		}
		if preferBrew, ok := install["preferBrew"].(bool); ok {
			installConfig.PreferBrew = preferBrew
		}
		if nodeManager, ok := install["nodeManager"].(string); ok {
			installConfig.NodeManager = nodeManager
		}
		result.Install = installConfig
	}

	// Parse Filter config
	if filter, ok := skillsMap["filter"].(map[string]interface{}); ok {
		filterConfig := SkillsFilterConfig{}
		if minPri, ok := filter["minPriority"].(int); ok {
			filterConfig.MinPriority = minPri
		}
		if maxPri, ok := filter["maxPriority"].(int); ok {
			filterConfig.MaxPriority = maxPri
		}
		if includeUnprioritized, ok := filter["includeUnprioritized"].(bool); ok {
			filterConfig.IncludeUnprioritized = includeUnprioritized
		}
		result.Filter = filterConfig
	}

	return result
}
