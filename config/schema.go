package config

import (
	"time"
)

// Config 是主配置结构
type Config struct {
	Workspace WorkspaceConfig `mapstructure:"workspace" json:"workspace"`
	Agents    AgentsConfig    `mapstructure:"agents" json:"agents"`
	Channels  ChannelsConfig  `mapstructure:"channels" json:"channels"`
	Providers ProvidersConfig `mapstructure:"providers" json:"providers"`
	Gateway   GatewayConfig   `mapstructure:"gateway" json:"gateway"`
	Session   SessionConfig   `mapstructure:"session" json:"session"`
	Tools     ToolsConfig     `mapstructure:"tools" json:"tools"`
	Approvals ApprovalsConfig `mapstructure:"approvals" json:"approvals"`
	Memory    MemoryConfig    `mapstructure:"memory" json:"memory"`
	// Skills configuration (map[string]interface{} to be parsed by skills package)
	Skills map[string]interface{} `mapstructure:"skills" json:"skills"`
	// Agent 绑定配置
	Bindings []BindingConfig `mapstructure:"bindings" json:"bindings"`
}

// SessionConfig 会话配置
type SessionConfig struct {
	Scope           string                    `mapstructure:"scope" json:"scope"`                       // per-sender | global，默认 per-sender
	Store           string                    `mapstructure:"store" json:"store"`                       // 存储路径
	MainKey         string                    `mapstructure:"main_key" json:"main_key"`                  // 主会话键（用于 per-sender 时的规范 key）
	Reset           *SessionResetConfig      `mapstructure:"reset" json:"reset"`                        // 全局重置策略
	ResetByChannel  map[string]SessionResetConfig `mapstructure:"reset_by_channel" json:"reset_by_channel"` // 按 channel 覆盖
}

// SessionResetConfig 会话重置策略
type SessionResetConfig struct {
	Mode        string `mapstructure:"mode" json:"mode"`               // daily | idle
	AtHour      int    `mapstructure:"at_hour" json:"at_hour"`         // daily 时 0-23，默认 4
	IdleMinutes int    `mapstructure:"idle_minutes" json:"idle_minutes"` // idle 时多少分钟无活动则视为不新鲜
}

// WorkspaceConfig Workspace 配置
type WorkspaceConfig struct {
	Path string `mapstructure:"path" json:"path"` // Workspace 目录路径，空则使用默认路径
}

// AgentsConfig Agent 配置
type AgentsConfig struct {
	Defaults AgentDefaults `mapstructure:"defaults" json:"defaults"`
	List     []AgentConfig `mapstructure:"list" json:"list"`
}

// AgentDefaults Agent 默认配置
type AgentDefaults struct {
	Model             string           `mapstructure:"model" json:"model"`
	MaxIterations     int              `mapstructure:"max_iterations" json:"max_iterations"`
	Temperature       float64          `mapstructure:"temperature" json:"temperature"`
	MaxTokens         int              `mapstructure:"max_tokens" json:"max_tokens"`
	ContextTokens     int              `mapstructure:"context_tokens" json:"context_tokens"`           // 模型上下文窗口 token 数，0 表示用 provider 默认
	LimitHistoryTurns int              `mapstructure:"limit_history_turns" json:"limit_history_turns"` // 发给 LLM 时最多保留的 user 轮次，0 表示不限制（与 OpenClaw 对齐）
	RunTimeoutSeconds         int          `mapstructure:"run_timeout_seconds" json:"run_timeout_seconds"`                   // 单次 Run 最大耗时（秒），0 表示不限制；超时后 ctx 取消，避免模型 API 断开时无限卡住
	ModelRequestIntervalSeconds int       `mapstructure:"model_request_interval_seconds" json:"model_request_interval_seconds"` // 同一会话内两次调用模型的最小间隔（秒），0 表示不限制；用于缓解 406/限流（OpenClaw 无此配置，为 goclaw 扩展）
	Retry             *RetryConfig     `mapstructure:"retry" json:"retry"`                             // 重试配置
	Subagents         *SubagentsConfig `mapstructure:"subagents" json:"subagents"`
}

// RetryConfig 重试配置
type RetryConfig struct {
	Enabled       bool          `mapstructure:"enabled" json:"enabled"`
	MaxRetries    int           `mapstructure:"max_retries" json:"max_retries"`
	InitialDelay  time.Duration `mapstructure:"initial_delay" json:"initial_delay"`
	MaxDelay      time.Duration `mapstructure:"max_delay" json:"max_delay"`
	BackoffFactor float64       `mapstructure:"backoff_factor" json:"backoff_factor"`
}

// SubagentsConfig 分身配置
type SubagentsConfig struct {
	MaxConcurrent       int    `mapstructure:"max_concurrent" json:"max_concurrent"`
	ArchiveAfterMinutes int    `mapstructure:"archive_after_minutes" json:"archive_after_minutes"`
	Model               string `mapstructure:"model" json:"model"`
	Thinking            string `mapstructure:"thinking" json:"thinking"`
	TimeoutSeconds      int    `mapstructure:"timeout_seconds" json:"timeout_seconds"`
}

// AgentSubagentConfig 单 Agent 分身配置
type AgentSubagentConfig struct {
	AllowAgents    []string `mapstructure:"allow_agents" json:"allow_agents"` // 允许跨 Agent 创建
	Model          string   `mapstructure:"model" json:"model"`
	Thinking       string   `mapstructure:"thinking" json:"thinking"`
	TimeoutSeconds int      `mapstructure:"timeout_seconds" json:"timeout_seconds"`
	DenyTools      []string `mapstructure:"deny_tools" json:"deny_tools"`
	AllowTools     []string `mapstructure:"allow_tools" json:"allow_tools"`
}

// AgentConfig Agent 配置
type AgentConfig struct {
	ID           string                 `mapstructure:"id" json:"id"`                       // Agent 唯一ID
	Name         string                 `mapstructure:"name" json:"name"`                   // Agent 显示名称
	Default      bool                   `mapstructure:"default" json:"default"`             // 是否为默认Agent
	Model        string                 `mapstructure:"model" json:"model"`                 // 使用的模型
	Workspace    string                 `mapstructure:"workspace" json:"workspace"`         // 独立工作区路径
	Identity     *AgentIdentity         `mapstructure:"identity" json:"identity"`           // Agent 身份配置
	SystemPrompt string                 `mapstructure:"system_prompt" json:"system_prompt"` // 系统提示词
	Metadata     map[string]interface{} `mapstructure:"metadata" json:"metadata"`           // 额外元数据
	Subagents    *AgentSubagentConfig   `mapstructure:"subagents" json:"subagents"`         // 分身配置
}

// AgentIdentity Agent 身份配置
type AgentIdentity struct {
	Name  string `mapstructure:"name" json:"name"`   // 身份名称
	Emoji string `mapstructure:"emoji" json:"emoji"` // 表情符号
}

// BindingConfig Agent 绑定配置
type BindingConfig struct {
	AgentID string       `mapstructure:"agent_id" json:"agent_id"` // Agent ID
	Match   BindingMatch `mapstructure:"match" json:"match"`       // 匹配规则
}

// BindingMatch 绑定匹配规则
type BindingMatch struct {
	Channel   string `mapstructure:"channel" json:"channel"`       // 通道类型
	AccountID string `mapstructure:"account_id" json:"account_id"` // 账号ID
}

// ChannelsConfig 通道配置
type ChannelsConfig struct {
	Telegram TelegramChannelConfig `mapstructure:"telegram" json:"telegram"`
	WhatsApp WhatsAppChannelConfig `mapstructure:"whatsapp" json:"whatsapp"`
	Feishu   FeishuChannelConfig   `mapstructure:"feishu" json:"feishu"`
	DingTalk DingTalkChannelConfig `mapstructure:"dingtalk" json:"dingtalk"`
	QQ       QQChannelConfig       `mapstructure:"qq" json:"qq"`
	WeWork   WeWorkChannelConfig   `mapstructure:"wework" json:"wework"`
	Infoflow InfoflowChannelConfig `mapstructure:"infoflow" json:"infoflow"`
}

// ChannelAccountConfig 通道账号配置（支持多账号）
type ChannelAccountConfig struct {
	Enabled           bool     `mapstructure:"enabled" json:"enabled"`
	Name              string   `mapstructure:"name" json:"name"`                             // 账号显示名称
	Token             string   `mapstructure:"token" json:"token"`                           // Telegram token
	AppID             string   `mapstructure:"app_id" json:"app_id"`                         // QQ/Feishu/WeWork app_id
	AppSecret         string   `mapstructure:"app_secret" json:"app_secret"`                 // QQ/Feishu app_secret
	CorpID            string   `mapstructure:"corp_id" json:"corp_id"`                       // 企业微信 corp_id
	AgentID           string   `mapstructure:"agent_id" json:"agent_id"`                     // 企业微信 agent_id
	ClientID          string   `mapstructure:"client_id" json:"client_id"`                   // 钉钉 client_id
	ClientSecret      string   `mapstructure:"client_secret" json:"client_secret"`           // 钉钉 client_secret
	BridgeURL         string   `mapstructure:"bridge_url" json:"bridge_url"`                 // WhatsApp bridge url
	WebhookURL        string   `mapstructure:"webhook_url" json:"webhook_url"`               // Infoflow/Feishu webhook url
	AESKey            string   `mapstructure:"aes_key" json:"aes_key"`                       // Infoflow AES key
	EncryptKey        string   `mapstructure:"encrypt_key" json:"encrypt_key"`               // Feishu encrypt key
	VerificationToken string   `mapstructure:"verification_token" json:"verification_token"` // Feishu verification token
	EventMode         string   `mapstructure:"event_mode" json:"event_mode"`                 // Feishu event mode: webhook/long_connection
	WebhookPort       int      `mapstructure:"webhook_port" json:"webhook_port"`             // Infoflow/Feishu webhook port
	AllowedIDs        []string `mapstructure:"allowed_ids" json:"allowed_ids"`
}

// ChannelTypeAccountConfig 通道类型的多账号配置
type ChannelTypeAccountConfig struct {
	Enabled  bool                            `mapstructure:"enabled" json:"enabled"`
	Accounts map[string]ChannelAccountConfig `mapstructure:"accounts" json:"accounts"`
}

// TelegramChannelConfig Telegram 通道配置
type TelegramChannelConfig struct {
	Enabled    bool     `mapstructure:"enabled" json:"enabled"`
	Token      string   `mapstructure:"token" json:"token"`
	AllowedIDs []string `mapstructure:"allowed_ids" json:"allowed_ids"`
	// 多账号配置（新格式）
	Accounts map[string]ChannelAccountConfig `mapstructure:"accounts" json:"accounts"`
}

// WhatsAppChannelConfig WhatsApp 通道配置
type WhatsAppChannelConfig struct {
	Enabled    bool     `mapstructure:"enabled" json:"enabled"`
	BridgeURL  string   `mapstructure:"bridge_url" json:"bridge_url"`
	AllowedIDs []string `mapstructure:"allowed_ids" json:"allowed_ids"`
	// 多账号配置（新格式）
	Accounts map[string]ChannelAccountConfig `mapstructure:"accounts" json:"accounts"`
}

// FeishuChannelConfig 飞书通道配置
type FeishuChannelConfig struct {
	Enabled           bool     `mapstructure:"enabled" json:"enabled"`
	AppID             string   `mapstructure:"app_id" json:"app_id"`
	AppSecret         string   `mapstructure:"app_secret" json:"app_secret"`
	EncryptKey        string   `mapstructure:"encrypt_key" json:"encrypt_key"`
	VerificationToken string   `mapstructure:"verification_token" json:"verification_token"`
	EventMode         string   `mapstructure:"event_mode" json:"event_mode"` // webhook / long_connection
	WebhookPort       int      `mapstructure:"webhook_port" json:"webhook_port"`
	AllowedIDs        []string `mapstructure:"allowed_ids" json:"allowed_ids"`
	// 多账号配置（新格式）
	Accounts map[string]ChannelAccountConfig `mapstructure:"accounts" json:"accounts"`
}

// QQChannelConfig QQ 通道配置 (QQ 开放平台官方 Bot API)
type QQChannelConfig struct {
	Enabled    bool     `mapstructure:"enabled" json:"enabled"`
	AppID      string   `mapstructure:"app_id" json:"app_id"`           // QQ 机器人 AppID
	AppSecret  string   `mapstructure:"app_secret" json:"app_secret"`   // AppSecret (ClientSecret)
	AllowedIDs []string `mapstructure:"allowed_ids" json:"allowed_ids"` // 允许的用户/群ID列表
	// 多账号配置（新格式）
	Accounts map[string]ChannelAccountConfig `mapstructure:"accounts" json:"accounts"`
}

// WeWorkChannelConfig 企业微信通道配置
type WeWorkChannelConfig struct {
	Enabled        bool     `mapstructure:"enabled" json:"enabled"`
	CorpID         string   `mapstructure:"corp_id" json:"corp_id"`
	AgentID        string   `mapstructure:"agent_id" json:"agent_id"`
	Secret         string   `mapstructure:"secret" json:"secret"`
	Token          string   `mapstructure:"token" json:"token"`
	EncodingAESKey string   `mapstructure:"encoding_aes_key" json:"encoding_aes_key"`
	WebhookPort    int      `mapstructure:"webhook_port" json:"webhook_port"`
	AllowedIDs     []string `mapstructure:"allowed_ids" json:"allowed_ids"`
	// 多账号配置（新格式）
	Accounts map[string]ChannelAccountConfig `mapstructure:"accounts" json:"accounts"`
}

// DingTalkChannelConfig 钉钉通道配置
type DingTalkChannelConfig struct {
	Enabled      bool     `mapstructure:"enabled" json:"enabled"`
	ClientID     string   `mapstructure:"client_id" json:"client_id"`
	ClientSecret string   `mapstructure:"secret" json:"secret"`
	AllowedIDs   []string `mapstructure:"allowed_ids" json:"allowed_ids"`
	// 多账号配置（新格式）
	Accounts map[string]ChannelAccountConfig `mapstructure:"accounts" json:"accounts"`
}

// InfoflowChannelConfig 如流通道配置
type InfoflowChannelConfig struct {
	Enabled     bool     `mapstructure:"enabled" json:"enabled"`
	WebhookURL  string   `mapstructure:"webhook_url" json:"webhook_url"`
	Token       string   `mapstructure:"token" json:"token"`
	AESKey      string   `mapstructure:"aes_key" json:"aes_key"`
	WebhookPort int      `mapstructure:"webhook_port" json:"webhook_port"`
	AllowedIDs  []string `mapstructure:"allowed_ids" json:"allowed_ids"`
	// 多账号配置（新格式）
	Accounts map[string]ChannelAccountConfig `mapstructure:"accounts" json:"accounts"`
}

// ProvidersConfig LLM 提供商配置
type ProvidersConfig struct {
	OpenRouter         OpenRouterProviderConfig `mapstructure:"openrouter" json:"openrouter"`
	OpenAI             OpenAIProviderConfig     `mapstructure:"openai" json:"openai"`
	Anthropic          AnthropicProviderConfig  `mapstructure:"anthropic" json:"anthropic"`
	Moonshot           MoonshotProviderConfig   `mapstructure:"moonshot" json:"moonshot"`   // Kimi（月之暗面）OpenAI 兼容 API，与 OpenClaw 对齐
	Router9            Router9ProviderConfig   `mapstructure:"9router" json:"9router"`       // 9router 本地代理，OpenAI 兼容 API
	Profiles           []ProviderProfileConfig  `mapstructure:"profiles" json:"profiles"`
	Failover           FailoverConfig           `mapstructure:"failover" json:"failover"`
	MaxConcurrentCalls int                      `mapstructure:"max_concurrent_calls" json:"max_concurrent_calls"` // 全局并发 LLM 调用上限，0=不限制，1=串行（多 agent 时建议 1 防卡死）
}

// ProviderProfileConfig 提供商配置
type ProviderProfileConfig struct {
	Name          string                 `mapstructure:"name" json:"name"`
	Provider      string                 `mapstructure:"provider" json:"provider"` // openai, anthropic, openrouter, moonshot
	APIKey        string                 `mapstructure:"api_key" json:"api_key"`
	BaseURL       string                 `mapstructure:"base_url" json:"base_url"`
	ExtraBody     map[string]interface{} `mapstructure:"extra_body" json:"extra_body"`
	Priority      int                    `mapstructure:"priority" json:"priority"`
	ContextWindow int                    `mapstructure:"context_window" json:"context_window"` // 该 profile 的模型上下文窗口 token 数，0 表示默认
	Streaming     *bool                  `mapstructure:"streaming" json:"streaming"`           // 是否启用流式输出，默认 true，某些模型（如 Ollama）可能需要禁用
}

// FailoverConfig 故障转移配置
type FailoverConfig struct {
	Enabled         bool                 `mapstructure:"enabled" json:"enabled"`
	Strategy        string               `mapstructure:"strategy" json:"strategy"` // round_robin, least_used, random
	DefaultCooldown time.Duration        `mapstructure:"default_cooldown" json:"default_cooldown"`
	CircuitBreaker  CircuitBreakerConfig `mapstructure:"circuit_breaker" json:"circuit_breaker"`
}

// CircuitBreakerConfig 断路器配置
type CircuitBreakerConfig struct {
	FailureThreshold int           `mapstructure:"failure_threshold" json:"failure_threshold"`
	Timeout          time.Duration `mapstructure:"timeout" json:"timeout"`
}

// OpenRouterProviderConfig OpenRouter 配置
type OpenRouterProviderConfig struct {
	APIKey     string `mapstructure:"api_key" json:"api_key"`
	BaseURL    string `mapstructure:"base_url" json:"base_url"`
	Timeout    int    `mapstructure:"timeout" json:"timeout"`
	MaxRetries int    `mapstructure:"max_retries" json:"max_retries"`
	Streaming  *bool  `mapstructure:"streaming" json:"streaming"` // 是否启用流式输出，默认 true（当前实现暂不支持流式，预留）
}

// OpenAIProviderConfig OpenAI 配置
type OpenAIProviderConfig struct {
	APIKey    string                 `mapstructure:"api_key" json:"api_key"`
	BaseURL   string                 `mapstructure:"base_url" json:"base_url"`
	Timeout   int                    `mapstructure:"timeout" json:"timeout"`
	ExtraBody map[string]interface{} `mapstructure:"extra_body" json:"extra_body"`
	Streaming *bool                  `mapstructure:"streaming" json:"streaming"` // 是否启用流式输出，默认 true
}

// AnthropicProviderConfig Anthropic 配置
type AnthropicProviderConfig struct {
	APIKey  string `mapstructure:"api_key" json:"api_key"`
	BaseURL string `mapstructure:"base_url" json:"base_url"`
	Timeout int    `mapstructure:"timeout" json:"timeout"`
}

// MoonshotProviderConfig 月之暗面 Kimi 配置（OpenAI 兼容 API）
type MoonshotProviderConfig struct {
	APIKey    string                 `mapstructure:"api_key" json:"api_key"`
	BaseURL   string                 `mapstructure:"base_url" json:"base_url"`
	Timeout   int                    `mapstructure:"timeout" json:"timeout"`
	Streaming *bool                  `mapstructure:"streaming" json:"streaming"` // 是否启用流式输出，默认 true
	ExtraBody map[string]interface{} `mapstructure:"extra_body" json:"extra_body"` // 请求体扩展，如关闭 thinking: {"thinking":{"type":"disabled"}}
}

// Router9ProviderConfig 9router 本地代理配置（OpenAI 兼容 API）
type Router9ProviderConfig struct {
	APIKey       string                 `mapstructure:"api_key" json:"api_key"`               // 通常为 "sk_9router"
	BaseURL      string                 `mapstructure:"base_url" json:"base_url"`              // 默认 "http://localhost:20128/v1"
	Timeout      int                    `mapstructure:"timeout" json:"timeout"`
	Streaming    *bool                  `mapstructure:"streaming" json:"streaming"`           // 是否启用流式输出，默认 true
	ToolsEnabled *bool                  `mapstructure:"tools_enabled" json:"tools_enabled"`     // 是否向 9router 传 tools，默认 true；若 406 可试 false 排查
	ExtraBody    map[string]interface{} `mapstructure:"extra_body" json:"extra_body"`
}

// GatewayConfig 网关配置
type GatewayConfig struct {
	Host         string          `mapstructure:"host" json:"host"`
	Port         int             `mapstructure:"port" json:"port"`
	ReadTimeout  time.Duration   `mapstructure:"read_timeout" json:"read_timeout"`
	WriteTimeout time.Duration   `mapstructure:"write_timeout" json:"write_timeout"`
	WebSocket    WebSocketConfig `mapstructure:"websocket" json:"websocket"`
}

// WebSocketConfig WebSocket 配置
type WebSocketConfig struct {
	Host         string        `mapstructure:"host" json:"host"`
	Port         int           `mapstructure:"port" json:"port"`
	Path         string        `mapstructure:"path" json:"path"`
	EnableAuth   bool          `mapstructure:"enable_auth" json:"enable_auth"`
	AuthToken    string        `mapstructure:"auth_token" json:"auth_token"`
	PingInterval time.Duration `mapstructure:"ping_interval" json:"ping_interval"`
	PongTimeout  time.Duration `mapstructure:"pong_timeout" json:"pong_timeout"`
	ReadTimeout  time.Duration `mapstructure:"read_timeout" json:"read_timeout"`
	WriteTimeout time.Duration `mapstructure:"write_timeout" json:"write_timeout"`
}

// ToolsConfig 工具配置
type ToolsConfig struct {
	FileSystem FileSystemToolConfig `mapstructure:"filesystem" json:"filesystem"`
	Shell      ShellToolConfig      `mapstructure:"shell" json:"shell"`
	Web        WebToolConfig        `mapstructure:"web" json:"web"`
	Browser    BrowserToolConfig    `mapstructure:"browser" json:"browser"`
}

// FileSystemToolConfig 文件系统工具配置
type FileSystemToolConfig struct {
	AllowedPaths []string `mapstructure:"allowed_paths" json:"allowed_paths"`
	DeniedPaths  []string `mapstructure:"denied_paths" json:"denied_paths"`
}

// ShellToolConfig Shell 工具配置
type ShellToolConfig struct {
	Enabled     bool          `mapstructure:"enabled" json:"enabled"`
	AllowedCmds []string      `mapstructure:"allowed_cmds" json:"allowed_cmds"`
	DeniedCmds  []string      `mapstructure:"denied_cmds" json:"denied_cmds"`
	Timeout     int           `mapstructure:"timeout" json:"timeout"`
	WorkingDir  string        `mapstructure:"working_dir" json:"working_dir"`
	Sandbox     SandboxConfig `mapstructure:"sandbox" json:"sandbox"`
}

// SandboxConfig Docker 沙箱配置
type SandboxConfig struct {
	Enabled    bool   `mapstructure:"enabled" json:"enabled"`
	Image      string `mapstructure:"image" json:"image"`
	Workdir    string `mapstructure:"workdir" json:"workdir"`
	Remove     bool   `mapstructure:"remove" json:"remove"`
	Network    string `mapstructure:"network" json:"network"`
	Privileged bool   `mapstructure:"privileged" json:"privileged"`
}

// WebToolConfig Web 工具配置
type WebToolConfig struct {
	SearchAPIKey string `mapstructure:"search_api_key" json:"search_api_key"`
	SearchEngine string `mapstructure:"search_engine" json:"search_engine"`
	Timeout      int    `mapstructure:"timeout" json:"timeout"`
}

// BrowserToolConfig 浏览器工具配置
type BrowserToolConfig struct {
	Enabled  bool `mapstructure:"enabled" json:"enabled"`
	Headless bool `mapstructure:"headless" json:"headless"`
	Timeout  int  `mapstructure:"timeout" json:"timeout"`
}

// ApprovalsConfig 审批配置
type ApprovalsConfig struct {
	Behavior  string   `mapstructure:"behavior" json:"behavior"`   // auto, manual, prompt
	Allowlist []string `mapstructure:"allowlist" json:"allowlist"` // 工具允许列表
}

// MemoryConfig 记忆配置
type MemoryConfig struct {
	Backend string              `mapstructure:"backend" json:"backend"` // "builtin" | "qmd"
	Builtin BuiltinMemoryConfig `mapstructure:"builtin" json:"builtin"`
	QMD     QMDConfig           `mapstructure:"qmd" json:"qmd"`
}

// BuiltinMemoryConfig 内置 SQLite 记忆配置
type BuiltinMemoryConfig struct {
	Enabled      bool                    `mapstructure:"enabled" json:"enabled"`
	DatabasePath string                  `mapstructure:"database_path" json:"database_path"`
	AutoIndex    bool                    `mapstructure:"auto_index" json:"auto_index"`
	Embedding    *BuiltinEmbeddingConfig `mapstructure:"embedding" json:"embedding"` // 可选，与 OpenClaw provider/fallback 对齐
	Sync         *BuiltinSyncConfig      `mapstructure:"sync" json:"sync"`           // 与 OpenClaw sync.watch / onSearch 对齐
}

// BuiltinSyncConfig 内置记忆同步配置（watch / onSearch 等，与 OpenClaw 一致）
type BuiltinSyncConfig struct {
	Watch           bool `mapstructure:"watch" json:"watch"`                       // 监听 workspace/memory 变更后自动重索引，默认 true
	WatchDebounceMs int  `mapstructure:"watch_debounce_ms" json:"watch_debounce_ms"` // 去抖毫秒，默认 1500
}

// BuiltinEmbeddingConfig 内置记忆嵌入配置（主 provider + 备用顺序）
type BuiltinEmbeddingConfig struct {
	Provider string `mapstructure:"provider" json:"provider"` // 主提供商，如 openai
	Fallback string `mapstructure:"fallback" json:"fallback"`   // 备用提供商，如 gemini；空表示无备用
}

// QMDConfig QMD 记忆配置
type QMDConfig struct {
	Command        string      `mapstructure:"command" json:"command"`                 // "qmd"
	Enabled        bool        `mapstructure:"enabled" json:"enabled"`                 // 默认 false（需显式启用）
	IncludeDefault bool        `mapstructure:"include_default" json:"include_default"` // 是否索引默认记忆文件
	Paths          []QMDPath   `mapstructure:"paths" json:"paths"`                     // 额外索引路径
	Sessions       QMDSessions `mapstructure:"sessions" json:"sessions"`               // 会话索引配置
	Update         QMDUpdate   `mapstructure:"update" json:"update"`                   // 更新配置
	Limits         QMDLimits   `mapstructure:"limits" json:"limits"`                   // 搜索限制
}

// QMDPath QMD 索引路径配置
type QMDPath struct {
	Name    string `mapstructure:"name" json:"name"`
	Path    string `mapstructure:"path" json:"path"`
	Pattern string `mapstructure:"pattern" json:"pattern"` // 如 "**/*.md"
}

// QMDSessions QMD 会话索引配置
type QMDSessions struct {
	Enabled       bool   `mapstructure:"enabled" json:"enabled"`
	ExportDir     string `mapstructure:"export_dir" json:"export_dir"`
	RetentionDays int    `mapstructure:"retention_days" json:"retention_days"` // 默认 30
}

// QMDUpdate QMD 更新配置
type QMDUpdate struct {
	Interval       time.Duration `mapstructure:"interval" json:"interval"`               // 默认 5m
	OnBoot         bool          `mapstructure:"on_boot" json:"on_boot"`                 // 默认 true
	EmbedInterval  time.Duration `mapstructure:"embed_interval" json:"embed_interval"`   // 默认 60m
	CommandTimeout time.Duration `mapstructure:"command_timeout" json:"command_timeout"` // 默认 30s
	UpdateTimeout  time.Duration `mapstructure:"update_timeout" json:"update_timeout"`   // 默认 120s
}

// QMDLimits QMD 搜索限制配置
type QMDLimits struct {
	MaxResults      int `mapstructure:"max_results" json:"max_results"`             // 默认 6
	MaxSnippetChars int `mapstructure:"max_snippet_chars" json:"max_snippet_chars"` // 默认 700
	TimeoutMs       int `mapstructure:"timeout_ms" json:"timeout_ms"`               // 默认 4000
}
