package agent

import (
	"errors"
	"strings"
)

// CharsPerTokenEstimate 简单 token 估算：每 token 约 4 字符（英文偏多时更准）
const CharsPerTokenEstimate = 4

// ToolResultMaxFraction 单条 tool 结果最多占上下文窗口的比例（与 openclaw 一致）
const ToolResultMaxFraction = 0.3

// ErrContextOverflow 表示请求超出模型上下文窗口
var ErrContextOverflow = errors.New("context overflow: request exceeds model context window")

// contextOverflowSubstrings 用于匹配提供商返回的“上下文溢出”类错误文案
var contextOverflowSubstrings = []string{
	"context overflow", "context length exceeded", "request too large",
	"exceeds model context window", "maximum context length", "prompt is too long",
}

// IsContextOverflowError 判断 err 是否为“上下文溢出”类错误（便于上层重试时做截断）
func IsContextOverflowError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, ErrContextOverflow) {
		return true
	}
	lower := strings.ToLower(err.Error())
	for _, sub := range contextOverflowSubstrings {
		if strings.Contains(lower, sub) {
			return true
		}
	}
	return false
}

// EstimateTokens 用字符数粗略估算 token 数
func EstimateTokens(s string) int {
	if s == "" {
		return 0
	}
	n := len(s) / CharsPerTokenEstimate
	if n < 1 && len(s) > 0 {
		return 1
	}
	return n
}

// EstimateMessageTokens 估算单条 AgentMessage 的 token 数
func EstimateMessageTokens(msg AgentMessage) int {
	var n int
	for _, block := range msg.Content {
		switch b := block.(type) {
		case TextContent:
			n += EstimateTokens(b.Text)
		case ThinkingContent:
			n += EstimateTokens(b.Thinking)
		default:
			// ToolCallContent 等按名字和参数估算
			n += 50
		}
	}
	if n == 0 {
		n = 1
	}
	return n
}

// EstimateMessagesTokens 估算多条消息的总 token 数
func EstimateMessagesTokens(messages []AgentMessage) int {
	var total int
	for i := range messages {
		total += EstimateMessageTokens(messages[i])
	}
	return total
}

// LimitHistoryTurns 保留最近 maxTurns 个 user 轮次及其对应的 assistant/tool 消息。
// 与 openclaw 的 limitHistoryTurns 对齐：从后往前数 user，保留最后 maxTurns 个 user 及其之后的消息。
// maxTurns <= 0 时返回原切片（不修改）。
func LimitHistoryTurns(messages []AgentMessage, maxTurns int) []AgentMessage {
	if maxTurns <= 0 || len(messages) == 0 {
		return messages
	}
	userCount := 0
	lastUserIndex := len(messages)
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role == RoleUser {
			userCount++
			if userCount > maxTurns {
				return messages[lastUserIndex:]
			}
			lastUserIndex = i
		}
	}
	return messages
}

// TruncateToolResult 将单条 tool 结果文本截断到不超过 maxChars 字符，并在末尾追加截断说明。
// 若 content 已不超过 maxChars，返回原 content。
func TruncateToolResult(content string, maxChars int) string {
	if maxChars <= 0 || len(content) <= maxChars {
		return content
	}
	suffix := "\n\n[Content truncated — original was too large for the model's context window.]"
	keep := maxChars - len(suffix)
	if keep < 0 {
		keep = 0
	}
	return content[:keep] + suffix
}

// TruncateToolResultByContext 按上下文窗口的 ToolResultMaxFraction 计算最大字符数并截断。
// contextWindowTokens 为模型上下文窗口 token 数。
func TruncateToolResultByContext(content string, contextWindowTokens int) string {
	if contextWindowTokens <= 0 {
		return content
	}
	maxTokens := int(float64(contextWindowTokens) * ToolResultMaxFraction)
	if maxTokens < 500 {
		maxTokens = 500
	}
	maxChars := maxTokens * CharsPerTokenEstimate
	return TruncateToolResult(content, maxChars)
}

// contentBlocksToText 从 ContentBlock 切片提取纯文本（用于估算和截断）
func contentBlocksToText(content []ContentBlock) string {
	var b strings.Builder
	for _, block := range content {
		switch x := block.(type) {
		case TextContent:
			b.WriteString(x.Text)
		case ThinkingContent:
			b.WriteString(x.Thinking)
		}
	}
	return b.String()
}

// CopyMessagesWithTruncatedToolResults 复制 messages 并对其中 role=tool 的消息按上下文窗口截断，返回新切片（不修改原切片）
func CopyMessagesWithTruncatedToolResults(messages []AgentMessage, contextWindowTokens int) []AgentMessage {
	if contextWindowTokens <= 0 {
		out := make([]AgentMessage, len(messages))
		copy(out, messages)
		return out
	}
	out := make([]AgentMessage, len(messages))
	for i := range messages {
		out[i] = messages[i]
		if messages[i].Role != RoleToolResult {
			continue
		}
		text := contentBlocksToText(messages[i].Content)
		if text == "" {
			continue
		}
		truncated := TruncateToolResultByContext(text, contextWindowTokens)
		if truncated != text {
			out[i].Content = []ContentBlock{TextContent{Text: truncated}}
		}
	}
	return out
}
