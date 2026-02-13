package cli

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/smallnest/goclaw/config"
	"github.com/smallnest/goclaw/internal"
	"github.com/spf13/cobra"
)

var (
	onboardAPIKey       string
	onboardBaseURL      string
	onboardModel        string
	onboardProvider     string
	onboardSkipPrompts  bool
	onboardReconfigure  bool
	onboardReset        bool
)

// 模型供应商列表（顺序与编号对应，与 OpenClaw 一致）
var providerOptions = []struct {
	Value string
	Label string
}{
	{"openai", "OpenAI (GPT-4o, GPT-4o-mini, O1, ...)"},
	{"anthropic", "Anthropic (Claude Sonnet/Opus, ...)"},
	{"openrouter", "OpenRouter (多模型聚合)"},
	{"kimi", "Kimi / 月之暗面 (Kimi K2.5, ...)"},
}

// 各 provider 的推荐模型列表（与 OpenClaw 常用选项对齐）
var suggestedModelsByProvider = map[string][]string{
	"openai": {
		"gpt-4o", "gpt-4o-mini", "gpt-4-turbo", "gpt-4o-nano",
		"o1", "o1-mini",
	},
	"anthropic": {
		"claude-sonnet-4-20250514", "claude-opus-4-20250514",
		"claude-3-5-sonnet-20241022", "claude-3-opus-20240229",
	},
	"openrouter": {
		"anthropic/claude-sonnet-4",
		"anthropic/claude-opus-4-20250514",
		"openai/gpt-4o",
		"deepseek/deepseek-chat",
		"google/gemini-2.0-flash-001",
		"meta-llama/llama-3.3-70b-instruct",
	},
	"kimi": {
		"kimi-k2.5", "kimi-k2-0905-preview", "kimi-k2-turbo-preview",
		"kimi-k2-thinking", "kimi-k2-thinking-turbo",
	},
}

var onboardCmd = &cobra.Command{
	Use:   "onboard",
	Short: "Interactive setup wizard for goclaw",
	Long: `Guided setup wizard for goclaw.

This command helps you:
1. Initialize the config file and built-in skills
2. Configure your API key and model
3. Set up your workspace

Run again with --reconfigure to re-set API key and model; use --reset to clear config/sessions/workspace (like OpenClaw). Or use 'goclaw configure' to only update provider/model.`,
	Run: runOnboard,
}

func init() {
	// Non-interactive flags
	onboardCmd.Flags().StringVarP(&onboardAPIKey, "api-key", "k", "", "API key for the provider (required in non-interactive mode)")
	onboardCmd.Flags().StringVarP(&onboardBaseURL, "base-url", "u", "", "Base URL for the provider API")
	onboardCmd.Flags().StringVarP(&onboardModel, "model", "m", "", "Model name to use")
	onboardCmd.Flags().StringVarP(&onboardProvider, "provider", "p", "openai", "Provider: openai, anthropic, or openrouter")
	onboardCmd.Flags().BoolVar(&onboardSkipPrompts, "skip-prompts", false, "Skip all prompts (use defaults)")
	onboardCmd.Flags().BoolVar(&onboardReconfigure, "reconfigure", false, "Re-run API key and model setup even when config exists (like OpenClaw)")
	onboardCmd.Flags().BoolVar(&onboardReset, "reset", false, "Full reset: remove config, sessions, and workspace, then run onboarding (like OpenClaw --reset)")
}

// configureCmd 仅重新设置 API Key 与模型（与 OpenClaw 的 configure 对齐）
var configureCmd = &cobra.Command{
	Use:   "configure",
	Short: "Re-set API key and model (interactive)",
	Long:  `Interactive flow to update provider, API key, base URL, and model. Does not create config or skills. Use 'goclaw config show' to view after.`,
	Run:   runConfigure,
}

// handleReset 按 scope 删除配置/会话/工作区（与 OpenClaw reset 对齐）
// scope: "config" | "config+sessions" | "full". cfg 可为 nil，为 nil 时用默认路径。
func handleReset(scope string, cfg *config.Config) {
	configPath := internal.GetConfigPath()
	goclawDir := internal.GetGoclawDir()
	sessionDir := filepath.Join(goclawDir, "sessions")
	workspaceDir := filepath.Join(goclawDir, "workspace")
	if cfg != nil {
		if cfg.Session.Store != "" {
			sessionDir = cfg.Session.Store
		}
		if p, err := config.GetWorkspacePath(cfg); err == nil && p != "" {
			workspaceDir = p
		}
	}

	removePath := func(name, path string) {
		if path == "" {
			return
		}
		if err := os.RemoveAll(path); err != nil {
			fmt.Fprintf(os.Stderr, "  Warning: failed to remove %s: %v\n", name, err)
		} else {
			fmt.Printf("  ✓ Removed: %s\n", path)
		}
	}

	removePath("config", configPath)
	if scope == "config" {
		return
	}
	removePath("sessions", sessionDir)
	if scope != "full" {
		return
	}
	removePath("workspace", workspaceDir)
}

func runConfigure(cmd *cobra.Command, args []string) {
	cfg, err := config.Load("")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: Failed to load config: %v\n", err)
		os.Exit(1)
	}
	// configure 仅更新 provider/model，不提示渠道与网关（与 OpenClaw 一致）
	if err := interactiveSetup(cfg, false); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	configPath := internal.GetConfigPath()
	if err := config.Save(cfg, configPath); err != nil {
		fmt.Fprintf(os.Stderr, "Error: Failed to save config: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("  ✓ Config updated. Run 'goclaw config show' to view.")
}

func runOnboard(cmd *cobra.Command, args []string) {
	fmt.Println()
	fmt.Println("╔════════════════════════════════════════════════════════╗")
	fmt.Println("║                    GoClaw Onboarding                      ║")
	fmt.Println("╚════════════════════════════════════════════════════════╝")
	fmt.Println()

	// --reset：先执行完整重置再继续（与 OpenClaw --reset 对齐）
	if onboardReset {
		fmt.Println("Step 0: Full reset (config + sessions + workspace)...")
		handleReset("full", nil)
		fmt.Println()
	}

	// 1. Initialize config file and built-in skills
	fmt.Println("Step 1: Initializing goclaw environment...")
	goclawDir := internal.GetGoclawDir()
	fmt.Printf("  Config directory: %s\n", goclawDir)

	// Ensure config file exists
	configCreated, err := internal.EnsureConfig()
	if err != nil {
		fmt.Fprintf(os.Stderr, "  Error: Failed to ensure config: %v\n", err)
		os.Exit(1)
	}
	if configCreated {
		fmt.Println("  ✓ Config file created")
	} else {
		fmt.Println("  ✓ Config file already exists")
	}

	// Ensure built-in skills exist
	if err := internal.EnsureBuiltinSkills(); err != nil {
		fmt.Fprintf(os.Stderr, "  Warning: Failed to ensure built-in skills: %v\n", err)
	} else {
		fmt.Println("  ✓ Built-in skills ready")
	}
	// Ensure memory directory exists (.goclaw/memory for builtin store.db)
	if err := internal.EnsureMemoryDir(); err != nil {
		fmt.Fprintf(os.Stderr, "  Warning: Failed to ensure memory directory: %v\n", err)
	} else {
		fmt.Println("  ✓ Memory directory ready")
	}
	fmt.Println()

	// 2. Load existing config
	cfg, err := config.Load("")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: Failed to load config: %v\n", err)
		os.Exit(1)
	}

	// 3. Interactive or non-interactive setup（支持重新配置与 Reset，与 OpenClaw 对齐）
	runSetup := true
	if !configCreated && !cmd.Flags().Changed("api-key") && !onboardReconfigure {
		fmt.Println("  Config already exists.")
		fmt.Println("  1) Use existing values  2) Update values (reconfigure)  3) Reset")
		choice := strings.TrimSpace(promptString("Choice [1]: ", "1", false))
		switch choice {
		case "2":
			runSetup = true
		case "3":
			fmt.Println("  Reset scope: 1) config only  2) config+sessions  3) full")
			scopeChoice := strings.TrimSpace(promptString("Scope [1]: ", "1", false))
			var scopeStr string
			switch scopeChoice {
			case "2":
				scopeStr = "config+sessions"
			case "3":
				scopeStr = "full"
			default:
				scopeStr = "config"
			}
			handleReset(scopeStr, cfg)
			_, ensureErr := internal.EnsureConfig()
			if ensureErr != nil {
				fmt.Fprintf(os.Stderr, "  Error: Failed to ensure config after reset: %v\n", ensureErr)
				os.Exit(1)
			}
			cfg, err = config.Load("")
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error: Failed to load config after reset: %v\n", err)
				os.Exit(1)
			}
			runSetup = true
		default:
			runSetup = false
			fmt.Println("  Skipping configuration (existing config unchanged).")
		}
	}
	if runSetup {
		if cmd.Flags().Changed("api-key") {
			if err := nonInteractiveSetup(cfg); err != nil {
				fmt.Fprintf(os.Stderr, "Error: %v\n", err)
				os.Exit(1)
			}
		} else {
			// onboard 完整流程：provider + model + 渠道 + 网关（与 OpenClaw 一致）
			if err := interactiveSetup(cfg, true); err != nil {
				fmt.Fprintf(os.Stderr, "Error: %v\n", err)
				os.Exit(1)
			}
		}
	}

	// 4. Save config
	configPath := internal.GetConfigPath()
	if err := config.Save(cfg, configPath); err != nil {
		fmt.Fprintf(os.Stderr, "Error: Failed to save config: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("  ✓ Config saved")
	fmt.Println()

	// 5. Print summary
	printSummary(cfg)
}

func nonInteractiveSetup(cfg *config.Config) error {
	fmt.Println("Step 2: Non-interactive configuration...")

	if onboardAPIKey == "" {
		return fmt.Errorf("--api-key is required in non-interactive mode")
	}

	provider := strings.ToLower(onboardProvider)
	switch provider {
	case "openai":
		cfg.Providers.OpenAI.APIKey = onboardAPIKey
		if onboardBaseURL != "" {
			cfg.Providers.OpenAI.BaseURL = onboardBaseURL
		}
		if onboardModel != "" {
			cfg.Agents.Defaults.Model = onboardModel
		}
		// 启用记忆嵌入（与 config.example.json 对齐）
		if cfg.Memory.Builtin.Embedding == nil {
			cfg.Memory.Builtin.Embedding = &config.BuiltinEmbeddingConfig{Provider: "openai"}
		}
	case "anthropic":
		cfg.Providers.Anthropic.APIKey = onboardAPIKey
		if onboardBaseURL != "" {
			cfg.Providers.Anthropic.BaseURL = onboardBaseURL
		}
		if onboardModel != "" {
			cfg.Agents.Defaults.Model = onboardModel
		}
	case "openrouter":
		cfg.Providers.OpenRouter.APIKey = onboardAPIKey
		if onboardBaseURL != "" {
			cfg.Providers.OpenRouter.BaseURL = onboardBaseURL
		}
		if onboardModel != "" {
			cfg.Agents.Defaults.Model = onboardModel
		}
	case "kimi":
		cfg.Providers.Moonshot.APIKey = onboardAPIKey
		if onboardBaseURL != "" {
			cfg.Providers.Moonshot.BaseURL = onboardBaseURL
		}
		if onboardModel != "" {
			cfg.Agents.Defaults.Model = onboardModel
		}
	default:
		return fmt.Errorf("invalid provider: %s (must be openai, anthropic, openrouter, or kimi)", provider)
	}

	fmt.Printf("  ✓ Provider configured: %s\n", provider)
	return nil
}

// interactiveSetup 交互式配置。fullFlow=true 时包含渠道与网关（onboard）；false 时仅 provider/model（configure）。
func interactiveSetup(cfg *config.Config, fullFlow bool) error {
	fmt.Println("Step 2: Interactive configuration")
	fmt.Println()

	// Check if any provider already has an API key
	hasAPIKey := cfg.Providers.OpenAI.APIKey != "" ||
		cfg.Providers.Anthropic.APIKey != "" ||
		cfg.Providers.OpenRouter.APIKey != "" ||
		cfg.Providers.Moonshot.APIKey != ""

	if hasAPIKey {
		fmt.Println("  API key already configured. Press Enter to keep or enter new value:")
	} else {
		fmt.Println("  Let's configure your API key and model.")
	}

	// 1) 选模型供应商（编号或名称）
	provider := promptProviderChoice(cfg)

	// Default API key 按当前选择的 provider 取（支持重新配置时保留该 provider 的 key）
	apiKeyDefault := getAPIKeyForProvider(cfg, provider)
	apiKey := promptString("API Key", apiKeyDefault, true)

	// Prompt for base URL (optional)
	defaultBaseURL := ""
	switch provider {
	case "openai":
		if cfg.Providers.OpenAI.BaseURL != "" {
			defaultBaseURL = cfg.Providers.OpenAI.BaseURL
		} else {
			defaultBaseURL = "https://api.openai.com/v1"
		}
	case "anthropic":
		if cfg.Providers.Anthropic.BaseURL != "" {
			defaultBaseURL = cfg.Providers.Anthropic.BaseURL
		} else {
			defaultBaseURL = "https://api.anthropic.com"
		}
	case "openrouter":
		if cfg.Providers.OpenRouter.BaseURL != "" {
			defaultBaseURL = cfg.Providers.OpenRouter.BaseURL
		} else {
			defaultBaseURL = "https://openrouter.ai/api/v1"
		}
	case "kimi":
		if cfg.Providers.Moonshot.BaseURL != "" {
			defaultBaseURL = cfg.Providers.Moonshot.BaseURL
		} else {
			defaultBaseURL = "https://api.moonshot.cn/v1"
		}
	}
	baseURL := promptString("Base URL (press Enter for default)", defaultBaseURL, false)

	// 2) 选模型：编号选择或自定义（与 OpenClaw model-picker 对齐）
	fmt.Println()
	defaultModel := cfg.Agents.Defaults.Model
	suggested := suggestedModelsByProvider[provider]
	if defaultModel == "" && len(suggested) > 0 {
		defaultModel = suggested[0]
	}
	model := promptModelChoice(suggested, defaultModel)
	if model == "" && defaultModel != "" {
		model = defaultModel
	}

	// Apply configuration
	switch provider {
	case "openai":
		cfg.Providers.OpenAI.APIKey = apiKey
		cfg.Providers.OpenAI.BaseURL = baseURL
		cfg.Agents.Defaults.Model = model
		// 启用记忆嵌入（与 config.example.json 对齐）
		if cfg.Memory.Builtin.Embedding == nil {
			cfg.Memory.Builtin.Embedding = &config.BuiltinEmbeddingConfig{Provider: "openai"}
		}
	case "anthropic":
		cfg.Providers.Anthropic.APIKey = apiKey
		cfg.Providers.Anthropic.BaseURL = baseURL
		cfg.Agents.Defaults.Model = model
	case "openrouter":
		cfg.Providers.OpenRouter.APIKey = apiKey
		cfg.Providers.OpenRouter.BaseURL = baseURL
		cfg.Agents.Defaults.Model = model
	case "kimi":
		cfg.Providers.Moonshot.APIKey = apiKey
		cfg.Providers.Moonshot.BaseURL = baseURL
		cfg.Agents.Defaults.Model = model
	default:
		return fmt.Errorf("invalid provider: %s (must be openai, anthropic, openrouter, or kimi)", provider)
	}

	if fullFlow {
		// 渠道选择（与 OpenClaw 对齐）
		if err := promptChannelChoice(cfg); err != nil {
			return err
		}
		// 网关端口（可选，与 OpenClaw 对齐）
		promptGatewayPort(cfg)
	}

	fmt.Println("  ✓ Configuration saved")
	return nil
}

// promptProviderChoice 选模型供应商：编号选择或输入名称（与 OpenClaw 一致）
func promptProviderChoice(cfg *config.Config) string {
	defaultProvider := "openai"
	if cfg.Providers.OpenAI.APIKey != "" {
		defaultProvider = "openai"
	} else if cfg.Providers.Anthropic.APIKey != "" {
		defaultProvider = "anthropic"
	} else if cfg.Providers.OpenRouter.APIKey != "" {
		defaultProvider = "openrouter"
	} else if cfg.Providers.Moonshot.APIKey != "" {
		defaultProvider = "kimi"
	}
	fmt.Println("  模型供应商 (enter number or name):")
	for i, opt := range providerOptions {
		fmt.Printf("    %d) %s\n", i+1, opt.Label)
	}
	fmt.Printf("  Choice [%s]: ", defaultProvider)
	reader := bufio.NewReader(os.Stdin)
	line, err := reader.ReadString('\n')
	if err != nil {
		return defaultProvider
	}
	line = strings.TrimSpace(strings.ToLower(line))
	if line == "" {
		return defaultProvider
	}
	if n, err := strconv.Atoi(line); err == nil && n >= 1 && n <= len(providerOptions) {
		return providerOptions[n-1].Value
	}
	// 名称映射
	switch line {
	case "openai":
		return "openai"
	case "anthropic":
		return "anthropic"
	case "openrouter":
		return "openrouter"
	case "kimi", "moonshot":
		return "kimi"
	}
	return line
}

// promptModelChoice 选模型：编号选择推荐模型或输入自定义 model id（与 OpenClaw model-picker 一致）
func promptModelChoice(suggested []string, defaultModel string) string {
	if len(suggested) == 0 {
		return promptString("Model", defaultModel, false)
	}
	fmt.Println("  Suggested models (enter number or model id):")
	for i, m := range suggested {
		fmt.Printf("    %d) %s\n", i+1, m)
	}
	fmt.Printf("    Or type custom model id (Enter = %s)\n", defaultModel)
	reader := bufio.NewReader(os.Stdin)
	fmt.Print("  Model choice: ")
	line, err := reader.ReadString('\n')
	if err != nil {
		return defaultModel
	}
	line = strings.TrimSpace(line)
	if line == "" {
		return defaultModel
	}
	if n, err := strconv.Atoi(line); err == nil && n >= 1 && n <= len(suggested) {
		return suggested[n-1]
	}
	return line
}

// 渠道选项（与 config.Channels 对齐）
const (
	channelNone     = "none"
	channelTelegram = "telegram"
	channelFeishu   = "feishu"
	channelDingTalk = "dingtalk"
	channelWeWork   = "wework"
	channelQQ       = "qq"
	channelWhatsApp = "whatsapp"
	channelInfoflow = "infoflow"
)

func promptChannelChoice(cfg *config.Config) error {
	fmt.Println()
	fmt.Println("  Channel (optional, like OpenClaw): 1=None  2=Telegram  3=Feishu  4=DingTalk  5=WeWork  6=QQ  7=WhatsApp  8=Infoflow")
	choice := strings.TrimSpace(promptString("Choice [1]: ", "1", false))
	var channel string
	switch choice {
	case "2":
		channel = channelTelegram
	case "3":
		channel = channelFeishu
	case "4":
		channel = channelDingTalk
	case "5":
		channel = channelWeWork
	case "6":
		channel = channelQQ
	case "7":
		channel = channelWhatsApp
	case "8":
		channel = channelInfoflow
	default:
		return nil
	}
	switch channel {
	case channelTelegram:
		cfg.Channels.Telegram.Enabled = true
		cfg.Channels.Telegram.Token = promptString("Telegram Bot Token", cfg.Channels.Telegram.Token, true)
	case channelFeishu:
		cfg.Channels.Feishu.Enabled = true
		cfg.Channels.Feishu.AppID = promptString("Feishu App ID", cfg.Channels.Feishu.AppID, true)
		cfg.Channels.Feishu.AppSecret = promptString("Feishu App Secret", cfg.Channels.Feishu.AppSecret, true)
	case channelDingTalk:
		cfg.Channels.DingTalk.Enabled = true
		cfg.Channels.DingTalk.ClientID = promptString("DingTalk Client ID", cfg.Channels.DingTalk.ClientID, true)
		cfg.Channels.DingTalk.ClientSecret = promptString("DingTalk Client Secret", cfg.Channels.DingTalk.ClientSecret, true)
	case channelWeWork:
		cfg.Channels.WeWork.Enabled = true
		cfg.Channels.WeWork.CorpID = promptString("WeWork Corp ID", cfg.Channels.WeWork.CorpID, true)
		cfg.Channels.WeWork.AgentID = promptString("WeWork Agent ID", cfg.Channels.WeWork.AgentID, true)
		cfg.Channels.WeWork.Secret = promptString("WeWork Secret", cfg.Channels.WeWork.Secret, true)
	case channelQQ:
		cfg.Channels.QQ.Enabled = true
		cfg.Channels.QQ.AppID = promptString("QQ App ID", cfg.Channels.QQ.AppID, true)
		cfg.Channels.QQ.AppSecret = promptString("QQ App Secret", cfg.Channels.QQ.AppSecret, true)
	case channelWhatsApp:
		cfg.Channels.WhatsApp.Enabled = true
		cfg.Channels.WhatsApp.BridgeURL = promptString("WhatsApp Bridge URL", cfg.Channels.WhatsApp.BridgeURL, true)
	case channelInfoflow:
		cfg.Channels.Infoflow.Enabled = true
		cfg.Channels.Infoflow.WebhookURL = promptString("Infoflow Webhook URL", cfg.Channels.Infoflow.WebhookURL, true)
		cfg.Channels.Infoflow.Token = promptString("Infoflow Token", cfg.Channels.Infoflow.Token, false)
	}
	if channel != "" {
		fmt.Printf("  ✓ Channel enabled: %s\n", channel)
	}
	return nil
}

func promptGatewayPort(cfg *config.Config) {
	def := strconv.Itoa(cfg.Gateway.Port)
	if cfg.Gateway.Port == 0 {
		def = "8080"
	}
	in := strings.TrimSpace(promptString("Gateway port (Enter to keep)", def, false))
	if in == "" {
		if cfg.Gateway.Port == 0 {
			cfg.Gateway.Port = 8080
		}
		return
	}
	if p, err := strconv.Atoi(in); err == nil && p > 0 && p < 65536 {
		cfg.Gateway.Port = p
	}
}

// promptConfirm 询问 y/n，defaultValue 为未输入时的默认值
func promptConfirm(question string, defaultValue bool) bool {
	reader := bufio.NewReader(os.Stdin)
	defStr := "y"
	if !defaultValue {
		defStr = "n"
	}
	fmt.Printf("  %s [%s]: ", question, defStr)
	line, err := reader.ReadString('\n')
	if err != nil {
		return defaultValue
	}
	line = strings.TrimSpace(strings.ToLower(line))
	if line == "" {
		return defaultValue
	}
	return line == "y" || line == "yes" || line == "1"
}

func getAPIKeyForProvider(cfg *config.Config, provider string) string {
	switch provider {
	case "openai":
		return cfg.Providers.OpenAI.APIKey
	case "anthropic":
		return cfg.Providers.Anthropic.APIKey
	case "openrouter":
		return cfg.Providers.OpenRouter.APIKey
	case "kimi":
		return cfg.Providers.Moonshot.APIKey
	}
	return ""
}

func promptString(prompt, defaultValue string, required bool) string {
	reader := bufio.NewReader(os.Stdin)

	if defaultValue != "" {
		fmt.Printf("  %s [%s]: ", prompt, defaultValue)
	} else {
		fmt.Printf("  %s: ", prompt)
	}

	input, err := reader.ReadString('\n')
	if err != nil {
		if required {
			fmt.Printf("    Error reading input, using default: %s\n", defaultValue)
		}
		return defaultValue
	}

	input = strings.TrimSpace(input)
	if input == "" {
		if defaultValue != "" {
			return defaultValue
		}
		if required {
			fmt.Printf("    Required field, using default: %s\n", defaultValue)
			return defaultValue
		}
	}

	// Mask API key in output
	if strings.Contains(strings.ToLower(prompt), "api") && strings.Contains(strings.ToLower(prompt), "key") {
		masked := maskAPIKey(input)
		fmt.Printf("    Set to: %s\n", masked)
	}

	return input
}

func maskAPIKey(key string) string {
	if len(key) <= 8 {
		return "****"
	}
	return key[:4] + "****" + key[len(key)-4:]
}

func printSummary(cfg *config.Config) {
	fmt.Println("═════════════════════════════════════════════════════════")
	fmt.Println("                         Summary")
	fmt.Println("═════════════════════════════════════════════════════════")
	fmt.Println()

	// Provider info
	var providerName, providerAPIKey string
	if cfg.Providers.OpenAI.APIKey != "" {
		providerName = "OpenAI"
		providerAPIKey = maskAPIKey(cfg.Providers.OpenAI.APIKey)
	} else if cfg.Providers.Anthropic.APIKey != "" {
		providerName = "Anthropic"
		providerAPIKey = maskAPIKey(cfg.Providers.Anthropic.APIKey)
	} else if cfg.Providers.OpenRouter.APIKey != "" {
		providerName = "OpenRouter"
		providerAPIKey = maskAPIKey(cfg.Providers.OpenRouter.APIKey)
	} else if cfg.Providers.Moonshot.APIKey != "" {
		providerName = "Kimi (Moonshot)"
		providerAPIKey = maskAPIKey(cfg.Providers.Moonshot.APIKey)
	}

	if providerName != "" {
		fmt.Printf("  Provider:  %s\n", providerName)
		fmt.Printf("  API Key:   %s\n", providerAPIKey)
	}

	fmt.Printf("  Model:     %s\n", cfg.Agents.Defaults.Model)

	// Channels（与 OpenClaw 对齐）
	channels := []string{}
	if cfg.Channels.Telegram.Enabled {
		channels = append(channels, "telegram")
	}
	if cfg.Channels.Feishu.Enabled {
		channels = append(channels, "feishu")
	}
	if cfg.Channels.DingTalk.Enabled {
		channels = append(channels, "dingtalk")
	}
	if cfg.Channels.WeWork.Enabled {
		channels = append(channels, "wework")
	}
	if cfg.Channels.QQ.Enabled {
		channels = append(channels, "qq")
	}
	if cfg.Channels.WhatsApp.Enabled {
		channels = append(channels, "whatsapp")
	}
	if cfg.Channels.Infoflow.Enabled {
		channels = append(channels, "infoflow")
	}
	if len(channels) > 0 {
		fmt.Printf("  Channels:  %s\n", strings.Join(channels, ", "))
	} else {
		fmt.Printf("  Channels:  none (Web only)\n")
	}

	// Workspace path
	workspacePath, _ := config.GetWorkspacePath(cfg)
	fmt.Printf("  Workspace: %s\n", workspacePath)

	// Memory embedding (when builtin + embedding configured)
	if cfg.Memory.Builtin.Embedding != nil && cfg.Memory.Builtin.Embedding.Provider != "" {
		fmt.Printf("  Memory:    builtin, embedding provider=%s\n", cfg.Memory.Builtin.Embedding.Provider)
	}

	// Gateway info
	fmt.Printf("  Gateway:   http://%s:%d\n", cfg.Gateway.Host, cfg.Gateway.Port)

	fmt.Println()
	fmt.Println("═════════════════════════════════════════════════════════")
	fmt.Println("                     Next Steps")
	fmt.Println("═════════════════════════════════════════════════════════")
	fmt.Println()
	fmt.Println("  1. Start goclaw (gateway + Web UI):")
	fmt.Println("     $ goclaw start")
	fmt.Println("     or: $ goclaw gateway run --port 28789")
	fmt.Println()
	fmt.Println("  2. Connect via HTTP:")
	fmt.Printf("     $ curl http://localhost:%d/health\n", cfg.Gateway.Port)
	fmt.Println()
	fmt.Println("  3. WebSocket / Web UI:")
	fmt.Printf("     ws://localhost:%d/ws  (or open http://localhost:28789/)\n", cfg.Gateway.Port)
	fmt.Println()
	fmt.Println("  4. View configuration:")
	fmt.Printf("     $ goclaw config show\n")
	fmt.Println()
	fmt.Println("  5. List available skills:")
	fmt.Println("     $ goclaw skills list")
	fmt.Println()
	fmt.Println("  6. Memory (if embedding configured):")
	fmt.Println("     $ goclaw memory status")
	fmt.Println("     $ goclaw memory index   # index workspace MEMORY.md")
	fmt.Println()
	fmt.Println("For more information, visit: https://github.com/smallnest/goclaw")
	fmt.Println()
}
