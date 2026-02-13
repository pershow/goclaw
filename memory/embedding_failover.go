package memory

import (
	"fmt"
	"sync"
)

// FailoverProvider 按顺序尝试多个 EmbeddingProvider，主提供商失败时自动切换备用（与 OpenClaw createEmbeddingProvider fallback 对齐）
type FailoverProvider struct {
	providers []EmbeddingProvider
	names     []string // 用于日志，与 providers 一一对应
	mu        sync.RWMutex
	active    int // 当前使用的 provider 下标
}

// FailoverProviderOption 备用 provider 选项
type FailoverProviderOption struct {
	Provider EmbeddingProvider
	Name     string
}

// NewFailoverProvider 创建故障转移 Provider，第一个为主，其余为备用
func NewFailoverProvider(primary EmbeddingProvider, primaryName string, fallbacks ...FailoverProviderOption) *FailoverProvider {
	providers := make([]EmbeddingProvider, 0, 1+len(fallbacks))
	names := make([]string, 0, 1+len(fallbacks))
	providers = append(providers, primary)
	names = append(names, primaryName)
	for _, f := range fallbacks {
		if f.Provider != nil {
			providers = append(providers, f.Provider)
			names = append(names, f.Name)
		}
	}
	return &FailoverProvider{
		providers: providers,
		names:     names,
		active:    0,
	}
}

// Embed 依次尝试各 Provider 的 Embed，直到成功或全部失败
func (f *FailoverProvider) Embed(text string) ([]float32, error) {
	f.mu.RLock()
	start := f.active
	providers := f.providers
	f.mu.RUnlock()

	var lastErr error
	for i := 0; i < len(providers); i++ {
		idx := (start + i) % len(providers)
		emb, err := providers[idx].Embed(text)
		if err == nil {
			if idx != start {
				f.mu.Lock()
				f.active = idx
				f.mu.Unlock()
			}
			return emb, nil
		}
		lastErr = err
	}
	return nil, fmt.Errorf("all embedding providers failed (last: %w)", lastErr)
}

// EmbedBatch 依次尝试各 Provider 的 EmbedBatch，直到成功或全部失败
func (f *FailoverProvider) EmbedBatch(texts []string) ([][]float32, error) {
	if len(texts) == 0 {
		return nil, nil
	}
	f.mu.RLock()
	start := f.active
	providers := f.providers
	f.mu.RUnlock()

	var lastErr error
	for i := 0; i < len(providers); i++ {
		idx := (start + i) % len(providers)
		embs, err := providers[idx].EmbedBatch(texts)
		if err == nil {
			if idx != start {
				f.mu.Lock()
				f.active = idx
				f.mu.Unlock()
			}
			return embs, nil
		}
		lastErr = err
	}
	return nil, fmt.Errorf("all embedding providers failed on batch (last: %w)", lastErr)
}

// Dimension 返回当前活跃 Provider 的维度
func (f *FailoverProvider) Dimension() int {
	f.mu.RLock()
	defer f.mu.RUnlock()
	if f.active < len(f.providers) {
		return f.providers[f.active].Dimension()
	}
	return f.providers[0].Dimension()
}

// MaxBatchSize 返回当前活跃 Provider 的最大批大小
func (f *FailoverProvider) MaxBatchSize() int {
	f.mu.RLock()
	defer f.mu.RUnlock()
	if f.active < len(f.providers) {
		return f.providers[f.active].MaxBatchSize()
	}
	return f.providers[0].MaxBatchSize()
}
