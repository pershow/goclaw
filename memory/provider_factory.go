package memory

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/smallnest/goclaw/config"
)

// NewEmbeddingProviderFromConfig 根据配置创建嵌入 Provider；支持主 provider + 备用（与 OpenClaw createEmbeddingProvider fallback 对齐）
// 若 embedCfg 为 nil 或 provider 名为空，返回 (nil, nil)。
func NewEmbeddingProviderFromConfig(cfg *config.Config, embedCfg *config.BuiltinEmbeddingConfig) (EmbeddingProvider, error) {
	if embedCfg == nil || strings.TrimSpace(embedCfg.Provider) == "" {
		return nil, nil
	}
	primary, primaryName, err := createProviderByName(cfg, strings.TrimSpace(strings.ToLower(embedCfg.Provider)))
	if err != nil {
		return nil, err
	}
	fallbackName := strings.TrimSpace(strings.ToLower(embedCfg.Fallback))
	if fallbackName == "" || fallbackName == embedCfg.Provider {
		return primary, nil
	}
	fallback, _, err := createProviderByName(cfg, fallbackName)
	if err != nil {
		// 备用创建失败时仍返回主 provider
		return primary, nil
	}
	return NewFailoverProvider(primary, primaryName, FailoverProviderOption{Provider: fallback, Name: fallbackName}), nil
}

func createProviderByName(cfg *config.Config, name string) (EmbeddingProvider, string, error) {
	switch name {
	case "openai":
		apiKey := ""
		if cfg != nil {
			apiKey = strings.TrimSpace(cfg.Providers.OpenAI.APIKey)
		}
		if apiKey == "" {
			apiKey = strings.TrimSpace(os.Getenv("OPENAI_API_KEY"))
		}
		if apiKey == "" {
			return nil, "", fmt.Errorf("openai api_key required for memory embedding (set in config or OPENAI_API_KEY)")
		}
		if cfg == nil {
			cfg = &config.Config{}
		}
		c := DefaultOpenAIConfig(apiKey)
		if cfg.Providers.OpenAI.BaseURL != "" {
			c.BaseURL = strings.TrimSuffix(cfg.Providers.OpenAI.BaseURL, "/")
		}
		c.Timeout = 30 * time.Second
		if cfg.Providers.OpenAI.Timeout > 0 {
			c.Timeout = time.Duration(cfg.Providers.OpenAI.Timeout) * time.Second
		}
		p, err := NewOpenAIProvider(c)
		if err != nil {
			return nil, "", err
		}
		return p, "openai", nil
	default:
		return nil, "", fmt.Errorf("unsupported memory embedding provider: %q", name)
	}
}
