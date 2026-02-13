package agent

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// SummarizeFunc 用于调用 LLM 对给定对话内容做摘要（与 OpenClaw generateSummary / summarizeWithFallback 对齐）
type SummarizeFunc func(ctx context.Context, prompt string) (summary string, err error)

const (
	// DefaultSummaryHeadKeep 压缩时保留的开头消息数（保留首轮上下文）
	DefaultSummaryHeadKeep = 1
	// DefaultSummaryTailKeep 压缩时保留的末尾消息数（最近几轮）
	DefaultSummaryTailKeep = 12
	// SummaryMessagePrefix 替换为摘要时使用的单条消息前缀
	SummaryMessagePrefix = "[Previous conversation summary]: "
)

// CompactWithSummary 当消息总 token 可能超窗时，对“中间段”做 LLM 摘要并替换为一条 user 消息，再返回新列表（与 OpenClaw 方案 B 对齐）
// 若 summarize 为 nil 或估算未超限，则返回原 messages（不修改）。
func CompactWithSummary(
	ctx context.Context,
	messages []AgentMessage,
	contextWindowTokens, reserveTokens int,
	summarize SummarizeFunc,
) ([]AgentMessage, error) {
	if summarize == nil || len(messages) == 0 {
		return messages, nil
	}
	limit := contextWindowTokens - reserveTokens
	if limit <= 0 {
		return messages, nil
	}
	if EstimateMessagesTokens(messages) <= limit {
		return messages, nil
	}

	headKeep := DefaultSummaryHeadKeep
	tailKeep := DefaultSummaryTailKeep
	if headKeep+tailKeep >= len(messages) {
		// 消息太少，只做截断不做摘要
		return LimitHistoryTurns(messages, 3), nil
	}

	// 中间段： [headKeep : len(messages)-tailKeep]
	midStart := headKeep
	midEnd := len(messages) - tailKeep
	if midEnd <= midStart {
		return messages, nil
	}

	mid := messages[midStart:midEnd]
	prompt := messagesToSummaryPrompt(mid)
	summary, err := summarize(ctx, prompt)
	if err != nil {
		return nil, fmt.Errorf("compaction summarization failed: %w", err)
	}
	summary = strings.TrimSpace(summary)
	if summary == "" {
		summary = "(No summary generated.)"
	}

	// 新列表： head + 一条摘要 user 消息 + tail
	out := make([]AgentMessage, 0, headKeep+1+tailKeep)
	for i := 0; i < headKeep; i++ {
		out = append(out, messages[i])
	}
	out = append(out, AgentMessage{
		Role:      RoleUser,
		Content:   []ContentBlock{TextContent{Text: SummaryMessagePrefix + summary}},
		Timestamp: time.Now().UnixMilli(),
	})
	for i := len(messages) - tailKeep; i < len(messages); i++ {
		out = append(out, messages[i])
	}
	return out, nil
}

func messagesToSummaryPrompt(messages []AgentMessage) string {
	var b strings.Builder
	for _, m := range messages {
		b.WriteString(string(m.Role))
		b.WriteString(": ")
		b.WriteString(contentBlocksToText(m.Content))
		b.WriteString("\n\n")
	}
	return strings.TrimSpace(b.String())
}
