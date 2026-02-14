package types

import (
	"fmt"
	"regexp"
	"strings"
)

// FailoverReason 失败原因类型
type FailoverReason string

const (
	// FailoverReasonAuth 认证错误
	FailoverReasonAuth FailoverReason = "auth"
	// FailoverReasonRateLimit 速率限制
	FailoverReasonRateLimit FailoverReason = "rate_limit"
	// FailoverReasonTimeout 超时
	FailoverReasonTimeout FailoverReason = "timeout"
	// FailoverReasonBilling 计费错误
	FailoverReasonBilling FailoverReason = "billing"
	// FailoverReasonContextOverflow 上下文溢出
	FailoverReasonContextOverflow FailoverReason = "context_overflow"
	// FailoverReasonServerError 服务器错误（5xx）
	FailoverReasonServerError FailoverReason = "server_error"
	// FailoverReasonNetworkError 网络错误
	FailoverReasonNetworkError FailoverReason = "network_error"
	// FailoverReasonUnknown 未知错误
	FailoverReasonUnknown FailoverReason = "unknown"
)

// IsRetryable 判断错误类型是否可重试
func (r FailoverReason) IsRetryable() bool {
	switch r {
	case FailoverReasonTimeout, FailoverReasonRateLimit,
		FailoverReasonServerError, FailoverReasonNetworkError,
		FailoverReasonContextOverflow:
		return true
	case FailoverReasonAuth, FailoverReasonBilling:
		return false
	default:
		return false
	}
}

// ErrorClassifier 错误分类器接口
type ErrorClassifier interface {
	ClassifyError(err error) FailoverReason
	IsFailoverError(err error) bool
}

// SimpleErrorClassifier 简单的错误分类器实现
type SimpleErrorClassifier struct {
	authPatterns         []string
	rateLimitPatterns    []string
	timeoutPatterns      []string
	billingPatterns      []string
	serverErrorPatterns  []string
	networkErrorPatterns []string
	contextOverflowPatterns []string
}

// NewSimpleErrorClassifier 创建简单错误分类器
func NewSimpleErrorClassifier() *SimpleErrorClassifier {
	return &SimpleErrorClassifier{
		authPatterns: []string{
			"invalid api key", "incorrect api key", "invalid token",
			"authentication", "re-authenticate", "unauthorized",
			"forbidden", "access denied", "expired", "401", "403",
		},
		rateLimitPatterns: []string{
			"rate limit", "too many requests", "429", "406", "not acceptable",
			"quota exceeded", "resource_exhausted", "usage limit", "overloaded",
			"reset after", "model not supported",
		},
		timeoutPatterns: []string{
			"timeout", "timed out", "deadline exceeded", "context deadline exceeded",
			"request timeout", "gateway timeout", "504",
		},
		billingPatterns: []string{
			"402", "payment required", "insufficient credits", "billing",
			"insufficient_quota", "quota_exceeded",
		},
		serverErrorPatterns: []string{
			"500", "501", "502", "503", "505",
			"internal server error", "bad gateway", "service unavailable",
			"server error", "upstream error",
		},
		networkErrorPatterns: []string{
			"connection refused", "connection reset", "network error",
			"no such host", "dns", "connection timeout",
			"eof", "broken pipe", "connection closed",
		},
		contextOverflowPatterns: []string{
			"context length", "maximum context", "token limit",
			"context_length_exceeded", "tokens exceed",
		},
	}
}

// ClassifyError 分类错误
func (c *SimpleErrorClassifier) ClassifyError(err error) FailoverReason {
	if err == nil {
		return FailoverReasonUnknown
	}

	errMsg := strings.ToLower(err.Error())

	// 按优先级检查（从最具体到最一般）
	if c.matchesAny(errMsg, c.contextOverflowPatterns) {
		return FailoverReasonContextOverflow
	}
	if c.matchesAny(errMsg, c.authPatterns) {
		return FailoverReasonAuth
	}
	if c.matchesAny(errMsg, c.rateLimitPatterns) {
		return FailoverReasonRateLimit
	}
	if c.matchesAny(errMsg, c.billingPatterns) {
		return FailoverReasonBilling
	}
	if c.matchesAny(errMsg, c.timeoutPatterns) {
		return FailoverReasonTimeout
	}
	if c.matchesAny(errMsg, c.serverErrorPatterns) {
		return FailoverReasonServerError
	}
	if c.matchesAny(errMsg, c.networkErrorPatterns) {
		return FailoverReasonNetworkError
	}

	return FailoverReasonUnknown
}

// IsFailoverError 检查是否为可回退的错误
func (c *SimpleErrorClassifier) IsFailoverError(err error) bool {
	if err == nil {
		return false
	}
	reason := c.ClassifyError(err)
	return reason != FailoverReasonUnknown
}

// matchesAny 检查错误消息是否匹配任何模式
func (c *SimpleErrorClassifier) matchesAny(errMsg string, patterns []string) bool {
	for _, pattern := range patterns {
		if strings.Contains(errMsg, pattern) {
			return true
		}
	}
	return false
}

// resetAfterSecondsRe 解析上游返回的 "reset after 30s" 中的秒数
var resetAfterSecondsRe = regexp.MustCompile(`(?i)reset\s+after\s+(\d+)\s*s`)

// ExtractRateLimitDelay 从错误信息中解析建议等待秒数（如 "reset after 30s"），未匹配或为 0 时返回默认值 defaultSec，最大不超过 maxSec
func ExtractRateLimitDelay(err error, defaultSec, maxSec int) int {
	if err == nil {
		return 0
	}
	matches := resetAfterSecondsRe.FindStringSubmatch(err.Error())
	if len(matches) < 2 {
		return defaultSec
	}
	var sec int
	if _, err := fmt.Sscanf(matches[1], "%d", &sec); err != nil || sec <= 0 {
		return defaultSec
	}
	if sec > maxSec {
		return maxSec
	}
	return sec
}
