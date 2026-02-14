package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/spf13/viper"
)

var globalConfig *Config
var globalConfigMu sync.RWMutex
var lastConfigFile string // 上次 Load 实际使用的配置文件路径，供排查用
var configWatcher *Watcher
var configHistory *ConfigHistory

// ConfigFileUsed 返回上次 Load 时实际使用的配置文件路径（可能为空，如仅用默认值或环境变量）
func ConfigFileUsed() string {
	return lastConfigFile
}

// Load 加载配置文件
func Load(configPath string) (*Config, error) {
	// 创建 viper 实例
	v := viper.New()

	// 设置配置文件路径
	if configPath != "" {
		v.SetConfigFile(configPath)
	} else {
		// 默认配置文件路径：与 internal.GetConfigPath() 一致，使用 ~/.goclaw/config.json
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, fmt.Errorf("failed to get home directory: %w", err)
		}
		goclawDir := filepath.Join(home, ".goclaw")
		v.AddConfigPath(goclawDir)
		v.AddConfigPath(".")
		v.SetConfigName("config")
		v.SetConfigType("json")
	}

	// 设置环境变量前缀（如 GOSKILLS_AGENTS_DEFAULTS_MODEL 会覆盖 agents.defaults.model）
	v.SetEnvPrefix("GOSKILLS")
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()

	// 设置默认值
	setDefaults(v)

	// 读取配置文件
	if err := v.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
			return nil, fmt.Errorf("failed to read config: %w", err)
		}
		// 配置文件不存在，使用默认值和环境变量
	}
	lastConfigFile = v.ConfigFileUsed()

	// 解析配置
	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("failed to unmarshal config: %w", err)
	}

	globalConfigMu.Lock()
	globalConfig = &cfg
	globalConfigMu.Unlock()

	return &cfg, nil
}

// setDefaults 设置默认配置值
func setDefaults(v *viper.Viper) {
	// Agent 默认配置
	v.SetDefault("agents.defaults.model", "openrouter:anthropic/claude-opus-4-5")
	v.SetDefault("agents.defaults.max_iterations", 15)
	// 不设置 temperature 默认值：配置里没写则不传该参数（由 API 默认）
	v.SetDefault("agents.defaults.max_tokens", 8192)
	v.SetDefault("agents.defaults.context_tokens", 0)
	v.SetDefault("agents.defaults.limit_history_turns", 0)
	v.SetDefault("memory.builtin.sync.watch", true)
	v.SetDefault("memory.builtin.sync.watch_debounce_ms", 1500)
	v.SetDefault("session.scope", "per-sender")
	v.SetDefault("session.reset.mode", "daily")
	v.SetDefault("session.reset.at_hour", 4)
	v.SetDefault("session.reset.idle_minutes", 60)
	v.SetDefault("channels.feishu.event_mode", "long_connection")

	// Gateway 默认配置
	v.SetDefault("gateway.host", "localhost")
	v.SetDefault("gateway.port", 28789)
	v.SetDefault("gateway.read_timeout", 30)
	v.SetDefault("gateway.write_timeout", 30)

	// 工具默认配置
	v.SetDefault("tools.shell.enabled", true)
	v.SetDefault("tools.shell.timeout", 120)
	v.SetDefault("tools.shell.sandbox.enabled", false)
	v.SetDefault("tools.shell.sandbox.image", "goclaw/sandbox:latest")
	v.SetDefault("tools.shell.sandbox.workdir", "/workspace")
	v.SetDefault("tools.shell.sandbox.remove", true)
	v.SetDefault("tools.shell.sandbox.network", "none")
	v.SetDefault("tools.shell.sandbox.privileged", false)
	v.SetDefault("tools.web.search_engine", "travily")
	v.SetDefault("tools.web.timeout", 10)
	v.SetDefault("tools.browser.enabled", false)
	v.SetDefault("browser.headless", true)
	v.SetDefault("browser.timeout", 30)
}

// Save 保存配置到文件
func Save(cfg *Config, path string) error {
	// 确保目录存在
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	// 转换为 JSON（带缩进）
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	// 写入文件
	if err := os.WriteFile(path, data, 0600); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}

	return nil
}

// Get 获取全局配置
func Get() *Config {
	globalConfigMu.RLock()
	defer globalConfigMu.RUnlock()
	return globalConfig
}

// Set 设置全局配置（用于热重载）
func Set(cfg *Config) {
	globalConfigMu.Lock()
	defer globalConfigMu.Unlock()
	globalConfig = cfg
}

// EnableHotReload 启用配置热重载
func EnableHotReload(configPath string) error {
	if configWatcher != nil {
		return fmt.Errorf("hot reload already enabled")
	}

	watcher, err := NewWatcher(configPath)
	if err != nil {
		return fmt.Errorf("failed to create config watcher: %w", err)
	}

	configWatcher = watcher
	configWatcher.Start()

	// 初始化配置历史记录
	home, err := os.UserHomeDir()
	if err == nil {
		historyFile := filepath.Join(home, ".goclaw", "config_history.json")
		history, err := NewConfigHistory(historyFile, 100)
		if err == nil {
			configHistory = history
		}
	}

	return nil
}

// DisableHotReload 禁用配置热重载
func DisableHotReload() error {
	if configWatcher == nil {
		return nil
	}

	if err := configWatcher.Stop(); err != nil {
		return fmt.Errorf("failed to stop config watcher: %w", err)
	}

	configWatcher = nil
	return nil
}

// OnConfigChange 注册配置变更处理函数
func OnConfigChange(handler ChangeHandler) error {
	if configWatcher == nil {
		return fmt.Errorf("hot reload not enabled")
	}

	configWatcher.OnChange(handler)
	return nil
}

// GetHistory 获取配置变更历史
func GetHistory(limit int) []ConfigChange {
	if configHistory == nil {
		return []ConfigChange{}
	}
	return configHistory.GetHistory(limit)
}

// GetLatestChange 获取最新的配置变更
func GetLatestChange() *ConfigChange {
	if configHistory == nil {
		return nil
	}
	return configHistory.GetLatest()
}

// ClearHistory 清空配置历史
func ClearHistory() error {
	if configHistory == nil {
		return fmt.Errorf("config history not initialized")
	}
	return configHistory.Clear()
}

// RollbackConfig 回滚到指定索引的配置
func RollbackConfig(index int) error {
	if configHistory == nil {
		return fmt.Errorf("config history not initialized")
	}

	oldCfg, err := configHistory.Rollback(index)
	if err != nil {
		return err
	}

	// 验证配置
	if err := Validate(oldCfg); err != nil {
		return fmt.Errorf("rollback config is invalid: %w", err)
	}

	// 保存配置到文件
	if lastConfigFile != "" {
		if err := Save(oldCfg, lastConfigFile); err != nil {
			return fmt.Errorf("failed to save rollback config: %w", err)
		}
	}

	// 更新全局配置
	Set(oldCfg)

	return nil
}

// RollbackToLatest 回滚到最近一次成功的配置
func RollbackToLatest() error {
	if configHistory == nil {
		return fmt.Errorf("config history not initialized")
	}

	oldCfg, err := configHistory.RollbackToLatest()
	if err != nil {
		return err
	}

	// 验证配置
	if err := Validate(oldCfg); err != nil {
		return fmt.Errorf("rollback config is invalid: %w", err)
	}

	// 保存配置到文件
	if lastConfigFile != "" {
		if err := Save(oldCfg, lastConfigFile); err != nil {
			return fmt.Errorf("failed to save rollback config: %w", err)
		}
	}

	// 更新全局配置
	Set(oldCfg)

	return nil
}

// GetDefaultConfigPath 获取默认配置文件路径（与 internal.GetConfigPath 一致）
func GetDefaultConfigPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("failed to get home directory: %w", err)
	}
	return filepath.Join(home, ".goclaw", "config.json"), nil
}

// GetWorkspacePath 获取 workspace 目录路径
func GetWorkspacePath(cfg *Config) (string, error) {
	if cfg.Workspace.Path != "" {
		// 使用配置中的自定义路径
		return cfg.Workspace.Path, nil
	}
	// 使用默认路径：~/.goclaw/workspace
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("failed to get home directory: %w", err)
	}
	return filepath.Join(home, ".goclaw", "workspace"), nil
}

// Validate 验证配置
func Validate(cfg *Config) error {
	if err := validateAgents(cfg); err != nil {
		return fmt.Errorf("agents config invalid: %w", err)
	}

	if err := validateProviders(cfg); err != nil {
		return fmt.Errorf("providers config invalid: %w", err)
	}

	if err := validateChannels(cfg); err != nil {
		return fmt.Errorf("channels config invalid: %w", err)
	}

	if err := validateTools(cfg); err != nil {
		return fmt.Errorf("tools config invalid: %w", err)
	}

	if err := validateGateway(cfg); err != nil {
		return fmt.Errorf("gateway config invalid: %w", err)
	}

	return nil
}

// validateAgents 验证 Agent 配置
func validateAgents(cfg *Config) error {
	if cfg.Agents.Defaults.Model == "" {
		return fmt.Errorf("model cannot be empty")
	}

	if cfg.Agents.Defaults.MaxIterations <= 0 {
		return fmt.Errorf("max_iterations must be positive")
	}

	if cfg.Agents.Defaults.Temperature < 0 || cfg.Agents.Defaults.Temperature > 2 {
		return fmt.Errorf("temperature must be between 0 and 2")
	}

	if cfg.Agents.Defaults.MaxTokens <= 0 {
		return fmt.Errorf("max_tokens must be positive")
	}

	return nil
}

// validateProviders 验证 LLM 提供商配置
func validateProviders(cfg *Config) error {
	// 至少需要一个提供商配置了 API 密钥
	hasProvider := false

	if cfg.Providers.OpenRouter.APIKey != "" {
		hasProvider = true
		if err := validateAPIKey(cfg.Providers.OpenRouter.APIKey); err != nil {
			return fmt.Errorf("openrouter: %w", err)
		}
	}

	if cfg.Providers.OpenAI.APIKey != "" {
		hasProvider = true
		if err := validateAPIKey(cfg.Providers.OpenAI.APIKey); err != nil {
			return fmt.Errorf("openai: %w", err)
		}
	}

	if cfg.Providers.Anthropic.APIKey != "" {
		hasProvider = true
		if err := validateAPIKey(cfg.Providers.Anthropic.APIKey); err != nil {
			return fmt.Errorf("anthropic: %w", err)
		}
	}

	if cfg.Providers.Moonshot.APIKey != "" {
		hasProvider = true
		if err := validateAPIKey(cfg.Providers.Moonshot.APIKey); err != nil {
			return fmt.Errorf("moonshot (kimi): %w", err)
		}
	}

	if !hasProvider {
		return fmt.Errorf("at least one provider must be configured with an API key")
	}

	return nil
}

// validateChannels 验证通道配置
func validateChannels(cfg *Config) error {
	// Telegram
	if cfg.Channels.Telegram.Enabled {
		if cfg.Channels.Telegram.Token == "" {
			return fmt.Errorf("telegram token is required when enabled")
		}
		// Telegram Bot Token format: <bot_id>:<api_key>
		// Example: 123456789:ABCDEF1234ghIkl-zyx57W2v1u123ew11
		// The token will be validated by the Telegram API when connecting
	}

	// WhatsApp
	if cfg.Channels.WhatsApp.Enabled {
		if cfg.Channels.WhatsApp.BridgeURL == "" {
			return fmt.Errorf("whatsapp bridge_url is required when enabled")
		}
		if !strings.HasPrefix(cfg.Channels.WhatsApp.BridgeURL, "http") {
			return fmt.Errorf("whatsapp bridge_url must be a valid URL")
		}
	}

	// Feishu
	if cfg.Channels.Feishu.Enabled {
		if cfg.Channels.Feishu.AppID == "" {
			return fmt.Errorf("feishu app_id is required when enabled")
		}
		if cfg.Channels.Feishu.AppSecret == "" {
			return fmt.Errorf("feishu app_secret is required when enabled")
		}
		mode, err := normalizeFeishuEventMode(cfg.Channels.Feishu.EventMode)
		if err != nil {
			return err
		}
		cfg.Channels.Feishu.EventMode = mode
		if mode == "webhook" && cfg.Channels.Feishu.VerificationToken == "" {
			return fmt.Errorf("feishu verification_token is required when event_mode=webhook")
		}
		if cfg.Channels.Feishu.WebhookPort < 0 || cfg.Channels.Feishu.WebhookPort > 65535 {
			return fmt.Errorf("feishu webhook_port must be between 0 and 65535")
		}
	}

	// QQ
	if cfg.Channels.QQ.Enabled {
		if cfg.Channels.QQ.AppID == "" {
			return fmt.Errorf("qq app_id is required when enabled")
		}
		if cfg.Channels.QQ.AppSecret == "" {
			return fmt.Errorf("qq app_secret is required when enabled")
		}
	}

	// WeWork (企业微信)
	if cfg.Channels.WeWork.Enabled {
		if cfg.Channels.WeWork.CorpID == "" {
			return fmt.Errorf("wework corp_id is required when enabled")
		}
		if cfg.Channels.WeWork.Secret == "" {
			return fmt.Errorf("wework secret is required when enabled")
		}
		if cfg.Channels.WeWork.AgentID == "" {
			return fmt.Errorf("wework agent_id is required when enabled")
		}
		if cfg.Channels.WeWork.WebhookPort < 0 || cfg.Channels.WeWork.WebhookPort > 65535 {
			return fmt.Errorf("wework webhook_port must be between 0 and 65535")
		}
	}

	return nil
}

// validateTools 验证工具配置
func validateTools(cfg *Config) error {
	// Shell 工具配置验证
	if cfg.Tools.Shell.Enabled {
		// 检查危险命令是否在拒绝列表中
		dangerousCmds := []string{"rm -rf", "dd", "mkfs"}
		for _, dangerous := range dangerousCmds {
			found := false
			for _, denied := range cfg.Tools.Shell.DeniedCmds {
				if strings.Contains(denied, dangerous) {
					found = true
					break
				}
			}
			if !found {
				return fmt.Errorf("shell tool: dangerous command '%s' should be in denied_cmds", dangerous)
			}
		}

		if cfg.Tools.Shell.Timeout <= 0 {
			return fmt.Errorf("shell timeout must be positive")
		}
	}

	// Web 工具配置验证
	if cfg.Tools.Web.SearchAPIKey != "" {
		if cfg.Tools.Web.SearchEngine == "" {
			return fmt.Errorf("web search_engine is required when search_api_key is set")
		}
	}

	if cfg.Tools.Web.Timeout <= 0 {
		return fmt.Errorf("web timeout must be positive")
	}

	// 浏览器工具配置验证
	if cfg.Tools.Browser.Enabled {
		if cfg.Tools.Browser.Timeout <= 0 {
			return fmt.Errorf("browser timeout must be positive")
		}
	}

	return nil
}

// validateGateway 验证网关配置
func validateGateway(cfg *Config) error {
	if cfg.Gateway.Port <= 0 || cfg.Gateway.Port > 65535 {
		return fmt.Errorf("gateway port must be between 1 and 65535")
	}

	if cfg.Gateway.ReadTimeout <= 0 {
		return fmt.Errorf("gateway read_timeout must be positive")
	}

	if cfg.Gateway.WriteTimeout <= 0 {
		return fmt.Errorf("gateway write_timeout must be positive")
	}

	return nil
}

// validateAPIKey 验证 API 密钥格式
func validateAPIKey(key string) error {
	key = strings.TrimSpace(key)

	if len(key) < 10 {
		return fmt.Errorf("API key too short (minimum 10 characters)")
	}

	// 检查是否包含空格
	if strings.Contains(key, " ") {
		return fmt.Errorf("API key cannot contain spaces")
	}

	return nil
}

func normalizeFeishuEventMode(mode string) (string, error) {
	mode = strings.TrimSpace(strings.ToLower(mode))
	if mode == "" || mode == "webhook" {
		return "webhook", nil
	}
	if mode == "long_connection" {
		return mode, nil
	}
	return "", fmt.Errorf("feishu event_mode must be one of: webhook, long_connection")
}
