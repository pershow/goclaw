package providers

import (
	"context"
)

// LimitConcurrencyProvider 包装 Provider，限制全局并发 LLM 调用数（多 agent 共享同一 provider 时防卡死）
type LimitConcurrencyProvider struct {
	inner Provider
	sem   chan struct{} // 信号量，cap = maxConcurrent
}

// NewLimitConcurrencyProvider 创建限流包装。maxConcurrent 必须 >= 1。
func NewLimitConcurrencyProvider(inner Provider, maxConcurrent int) *LimitConcurrencyProvider {
	if maxConcurrent < 1 {
		maxConcurrent = 1
	}
	return &LimitConcurrencyProvider{
		inner: inner,
		sem:   make(chan struct{}, maxConcurrent),
	}
}

func (p *LimitConcurrencyProvider) acquire(ctx context.Context) error {
	select {
	case p.sem <- struct{}{}:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (p *LimitConcurrencyProvider) release() {
	<-p.sem
}

// Chat 实现 Provider
func (p *LimitConcurrencyProvider) Chat(ctx context.Context, messages []Message, tools []ToolDefinition, options ...ChatOption) (*Response, error) {
	if err := p.acquire(ctx); err != nil {
		return nil, err
	}
	defer p.release()
	return p.inner.Chat(ctx, messages, tools, options...)
}

// ChatWithTools 实现 Provider
func (p *LimitConcurrencyProvider) ChatWithTools(ctx context.Context, messages []Message, tools []ToolDefinition, options ...ChatOption) (*Response, error) {
	if err := p.acquire(ctx); err != nil {
		return nil, err
	}
	defer p.release()
	return p.inner.ChatWithTools(ctx, messages, tools, options...)
}

// Close 转发到内层
func (p *LimitConcurrencyProvider) Close() error {
	return p.inner.Close()
}

// SupportsStreaming 转发到内层
func (p *LimitConcurrencyProvider) SupportsStreaming() bool {
	return p.inner.SupportsStreaming()
}

// limitStreamingProvider 在限流内实现 StreamingProvider（仅当 inner 为 StreamingProvider 时使用）
type limitStreamingProvider struct {
	*LimitConcurrencyProvider
}

var _ StreamingProvider = (*limitStreamingProvider)(nil)

// ChatStream 限流后调用内层 ChatStream
func (p *limitStreamingProvider) ChatStream(ctx context.Context, messages []Message, tools []ToolDefinition, callback StreamCallback, options ...ChatOption) error {
	if err := p.acquire(ctx); err != nil {
		return err
	}
	defer p.release()
	sp := p.inner.(StreamingProvider)
	return sp.ChatStream(ctx, messages, tools, callback, options...)
}

// WrapProviderWithConcurrencyLimit 若 maxConcurrent > 0 则用限流包装；否则返回原 Provider。若 inner 实现 StreamingProvider 则返回也实现 StreamingProvider。
func WrapProviderWithConcurrencyLimit(inner Provider, maxConcurrent int) Provider {
	if maxConcurrent <= 0 {
		return inner
	}
	limited := NewLimitConcurrencyProvider(inner, maxConcurrent)
	if _, ok := inner.(StreamingProvider); ok {
		return &limitStreamingProvider{LimitConcurrencyProvider: limited}
	}
	return limited
}
