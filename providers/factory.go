package providers

import (
	"fmt"
	"strings"

	"github.com/smallnest/goclaw/config"
	"github.com/smallnest/goclaw/types"
)

// ProviderType 提供商类型
type ProviderType string

const (
	ProviderTypeOpenAI     ProviderType = "openai"
	ProviderTypeAnthropic  ProviderType = "anthropic"
	ProviderTypeOpenRouter ProviderType = "openrouter"
	ProviderTypeMoonshot   ProviderType = "moonshot" // Kimi 月之暗面，OpenAI 兼容 API
)

// NewProvider 创建提供商（支持故障转移和配置轮换）
func NewProvider(cfg *config.Config) (Provider, error) {
	// 如果启用了故障转移且配置了多个配置，使用轮换提供商
	if cfg.Providers.Failover.Enabled && len(cfg.Providers.Profiles) > 0 {
		return NewRotationProviderFromConfig(cfg)
	}

	// 否则使用单一提供商
	return NewSimpleProvider(cfg)
}

// NewSimpleProvider 创建单一提供商
func NewSimpleProvider(cfg *config.Config) (Provider, error) {
	// 确定使用哪个提供商
	providerType, model, err := determineProvider(cfg)
	if err != nil {
		return nil, err
	}

	switch providerType {
	case ProviderTypeOpenAI:
		streaming := true
		if cfg.Providers.OpenAI.Streaming != nil {
			streaming = *cfg.Providers.OpenAI.Streaming
		}
		return NewOpenAIProviderWithStreaming(
			cfg.Providers.OpenAI.APIKey,
			cfg.Providers.OpenAI.BaseURL,
			model,
			cfg.Agents.Defaults.MaxTokens,
			cfg.Providers.OpenAI.ExtraBody,
			streaming,
		)
	case ProviderTypeAnthropic:
		return NewAnthropicProvider(cfg.Providers.Anthropic.APIKey, cfg.Providers.Anthropic.BaseURL, model, cfg.Agents.Defaults.MaxTokens)
	case ProviderTypeOpenRouter:
		streaming := true
		if cfg.Providers.OpenRouter.Streaming != nil {
			streaming = *cfg.Providers.OpenRouter.Streaming
		}
		return NewOpenRouterProviderWithStreaming(cfg.Providers.OpenRouter.APIKey, cfg.Providers.OpenRouter.BaseURL, model, cfg.Agents.Defaults.MaxTokens, streaming)
	case ProviderTypeMoonshot:
		baseURL := cfg.Providers.Moonshot.BaseURL
		if baseURL == "" {
			baseURL = "https://api.moonshot.cn/v1"
		}
		streaming := true
		if cfg.Providers.Moonshot.Streaming != nil {
			streaming = *cfg.Providers.Moonshot.Streaming
		}
		return NewOpenAIProviderWithStreaming(
			cfg.Providers.Moonshot.APIKey,
			baseURL,
			model,
			cfg.Agents.Defaults.MaxTokens,
			nil,
			streaming,
		)
	default:
		return nil, fmt.Errorf("unsupported provider type: %s", providerType)
	}
}

// NewRotationProviderFromConfig 从配置创建轮换提供商
func NewRotationProviderFromConfig(cfg *config.Config) (Provider, error) {
	// 创建错误分类器
	errorClassifier := types.NewSimpleErrorClassifier()

	// 确定轮换策略
	strategy := RotationStrategy(cfg.Providers.Failover.Strategy)
	if strategy == "" {
		strategy = RotationStrategyRoundRobin
	}

	// 创建轮换提供商
	rotation := NewRotationProvider(
		strategy,
		cfg.Providers.Failover.DefaultCooldown,
		errorClassifier,
	)

	// 添加所有配置
	for _, profileCfg := range cfg.Providers.Profiles {
		// 获取流式配置，默认为 true
		streaming := true
		if profileCfg.Streaming != nil {
			streaming = *profileCfg.Streaming
		}

		prov, err := createProviderByTypeWithStreaming(
			profileCfg.Provider,
			profileCfg.APIKey,
			profileCfg.BaseURL,
			cfg.Agents.Defaults.Model,
			cfg.Agents.Defaults.MaxTokens,
			profileCfg.ExtraBody,
			streaming,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to create provider for profile %s: %w", profileCfg.Name, err)
		}

		priority := profileCfg.Priority
		if priority == 0 {
			priority = 1
		}

		rotation.AddProfile(profileCfg.Name, prov, profileCfg.APIKey, priority)
	}

	// 如果只有一个配置，返回第一个提供商
	if len(cfg.Providers.Profiles) == 1 {
		p := cfg.Providers.Profiles[0]
		// 获取流式配置，默认为 true
		streaming := true
		if p.Streaming != nil {
			streaming = *p.Streaming
		}
		prov, err := createProviderByTypeWithStreaming(
			p.Provider,
			p.APIKey,
			p.BaseURL,
			cfg.Agents.Defaults.Model,
			cfg.Agents.Defaults.MaxTokens,
			p.ExtraBody,
			streaming,
		)
		if err != nil {
			return nil, err
		}
		return prov, nil
	}

	return rotation, nil
}

// createProviderByType 根据类型创建提供商
func createProviderByType(providerType, apiKey, baseURL, model string, maxTokens int, extraBody map[string]interface{}) (Provider, error) {
	return createProviderByTypeWithStreaming(providerType, apiKey, baseURL, model, maxTokens, extraBody, true)
}

// createProviderByTypeWithStreaming 根据类型创建提供商（带流式配置）
func createProviderByTypeWithStreaming(providerType, apiKey, baseURL, model string, maxTokens int, extraBody map[string]interface{}, streaming bool) (Provider, error) {
	switch ProviderType(providerType) {
	case ProviderTypeOpenAI:
		return NewOpenAIProviderWithStreaming(apiKey, baseURL, model, maxTokens, extraBody, streaming)
	case ProviderTypeAnthropic:
		return NewAnthropicProvider(apiKey, baseURL, model, maxTokens)
	case ProviderTypeOpenRouter:
		return NewOpenRouterProviderWithStreaming(apiKey, baseURL, model, maxTokens, streaming)
	case ProviderTypeMoonshot:
		return NewOpenAIProviderWithStreaming(apiKey, baseURL, model, maxTokens, nil, streaming)
	default:
		return nil, fmt.Errorf("unsupported provider type: %s", providerType)
	}
}

// determineProvider 确定提供商
func determineProvider(cfg *config.Config) (ProviderType, string, error) {
	model := cfg.Agents.Defaults.Model

	// 检查模型名称前缀
	if strings.HasPrefix(model, "openrouter:") {
		return ProviderTypeOpenRouter, strings.TrimPrefix(model, "openrouter:"), nil
	}

	if strings.HasPrefix(model, "anthropic:") || strings.HasPrefix(model, "claude-") {
		return ProviderTypeAnthropic, model, nil
	}

	if strings.HasPrefix(model, "openai:") || strings.HasPrefix(model, "gpt-") {
		return ProviderTypeOpenAI, model, nil
	}

	if strings.HasPrefix(model, "moonshot:") {
		return ProviderTypeMoonshot, strings.TrimPrefix(model, "moonshot:"), nil
	}
	if strings.HasPrefix(model, "kimi-") {
		return ProviderTypeMoonshot, model, nil
	}

	// 根据可用的 API key 决定
	if cfg.Providers.OpenRouter.APIKey != "" {
		return ProviderTypeOpenRouter, model, nil
	}

	if cfg.Providers.Anthropic.APIKey != "" {
		return ProviderTypeAnthropic, model, nil
	}

	if cfg.Providers.OpenAI.APIKey != "" {
		return ProviderTypeOpenAI, model, nil
	}

	if cfg.Providers.Moonshot.APIKey != "" {
		return ProviderTypeMoonshot, model, nil
	}

	return "", "", fmt.Errorf("no LLM provider API key configured")
}
