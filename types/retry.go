package types

import (
	"context"
	"math"
	"time"
)

// RetryStrategy 重试策略
type RetryStrategy struct {
	MaxRetries      int           // 最大重试次数
	InitialDelay    time.Duration // 初始延迟
	MaxDelay        time.Duration // 最大延迟
	BackoffFactor   float64       // 退避因子
	RetryableErrors []FailoverReason
	classifier      ErrorClassifier
}

// NewRetryStrategy 创建重试策略
func NewRetryStrategy(maxRetries int, initialDelay, maxDelay time.Duration, backoffFactor float64, classifier ErrorClassifier) *RetryStrategy {
	return &RetryStrategy{
		MaxRetries:    maxRetries,
		InitialDelay:  initialDelay,
		MaxDelay:      maxDelay,
		BackoffFactor: backoffFactor,
		RetryableErrors: []FailoverReason{
			FailoverReasonTimeout,
			FailoverReasonRateLimit,
			FailoverReasonContextOverflow,
		},
		classifier: classifier,
	}
}

// NewDefaultRetryStrategy 创建默认重试策略
func NewDefaultRetryStrategy(classifier ErrorClassifier) *RetryStrategy {
	return NewRetryStrategy(
		3,                // 最大重试 3 次
		1*time.Second,    // 初始延迟 1 秒
		30*time.Second,   // 最大延迟 30 秒
		2.0,              // 指数退避因子 2.0
		classifier,
	)
}

// ShouldRetry 判断是否应该重试
func (s *RetryStrategy) ShouldRetry(err error, attempt int) bool {
	if err == nil {
		return false
	}

	if attempt >= s.MaxRetries {
		return false
	}

	reason := s.classifier.ClassifyError(err)
	return s.isRetryable(reason)
}

// isRetryable 判断错误类型是否可重试
func (s *RetryStrategy) isRetryable(reason FailoverReason) bool {
	for _, r := range s.RetryableErrors {
		if r == reason {
			return true
		}
	}
	return false
}

// GetDelay 获取重试延迟（指数退避）
func (s *RetryStrategy) GetDelay(attempt int) time.Duration {
	if attempt < 0 {
		attempt = 0
	}

	// 指数退避：delay = initialDelay * (backoffFactor ^ attempt)
	delay := float64(s.InitialDelay) * math.Pow(s.BackoffFactor, float64(attempt))

	// 限制最大延迟
	if time.Duration(delay) > s.MaxDelay {
		return s.MaxDelay
	}

	return time.Duration(delay)
}

// Wait 等待指定的重试延迟
func (s *RetryStrategy) Wait(ctx context.Context, attempt int) error {
	delay := s.GetDelay(attempt)

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(delay):
		return nil
	}
}

// Retry 执行带重试的操作
func (s *RetryStrategy) Retry(ctx context.Context, fn func() error) error {
	var lastErr error

	for attempt := 0; attempt <= s.MaxRetries; attempt++ {
		// 第一次尝试不需要等待
		if attempt > 0 {
			if err := s.Wait(ctx, attempt-1); err != nil {
				return err
			}
		}

		// 执行操作
		err := fn()
		if err == nil {
			return nil
		}

		lastErr = err

		// 检查是否应该重试
		if !s.ShouldRetry(err, attempt) {
			return err
		}
	}

	return lastErr
}

// RetryWithResult 执行带重试的操作（带返回值）
func RetryWithResult[T any](ctx context.Context, strategy *RetryStrategy, fn func() (T, error)) (T, error) {
	var result T
	var lastErr error

	for attempt := 0; attempt <= strategy.MaxRetries; attempt++ {
		// 第一次尝试不需要等待
		if attempt > 0 {
			if err := strategy.Wait(ctx, attempt-1); err != nil {
				return result, err
			}
		}

		// 执行操作
		res, err := fn()
		if err == nil {
			return res, nil
		}

		lastErr = err

		// 检查是否应该重试
		if !strategy.ShouldRetry(err, attempt) {
			return result, err
		}
	}

	return result, lastErr
}

// RetryConfig 重试配置（用于配置文件）
type RetryConfig struct {
	Enabled       bool          `mapstructure:"enabled" json:"enabled"`
	MaxRetries    int           `mapstructure:"max_retries" json:"max_retries"`
	InitialDelay  time.Duration `mapstructure:"initial_delay" json:"initial_delay"`
	MaxDelay      time.Duration `mapstructure:"max_delay" json:"max_delay"`
	BackoffFactor float64       `mapstructure:"backoff_factor" json:"backoff_factor"`
}

// ToRetryStrategy 转换为重试策略
func (c *RetryConfig) ToRetryStrategy(classifier ErrorClassifier) *RetryStrategy {
	maxRetries := c.MaxRetries
	if maxRetries == 0 {
		maxRetries = 3
	}

	initialDelay := c.InitialDelay
	if initialDelay == 0 {
		initialDelay = 1 * time.Second
	}

	maxDelay := c.MaxDelay
	if maxDelay == 0 {
		maxDelay = 30 * time.Second
	}

	backoffFactor := c.BackoffFactor
	if backoffFactor == 0 {
		backoffFactor = 2.0
	}

	return NewRetryStrategy(maxRetries, initialDelay, maxDelay, backoffFactor, classifier)
}
