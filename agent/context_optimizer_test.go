package agent

import (
	"testing"
)

func TestContextOptimizer_OptimizeContext(t *testing.T) {
	optimizer := NewContextOptimizer(1000, 200)

	tests := []struct {
		name     string
		messages []AgentMessage
		wantLen  int
	}{
		{
			name: "no optimization needed",
			messages: []AgentMessage{
				{Role: RoleUser, Content: []ContentBlock{TextContent{Text: "Hello"}}},
				{Role: RoleAssistant, Content: []ContentBlock{TextContent{Text: "Hi"}}},
			},
			wantLen: 2,
		},
		{
			name: "compress low priority messages",
			messages: []AgentMessage{
				{Role: RoleUser, Content: []ContentBlock{TextContent{Text: generateLongText(1000)}}},
				{Role: RoleAssistant, Content: []ContentBlock{TextContent{Text: generateLongText(1000)}}},
				{Role: RoleUser, Content: []ContentBlock{TextContent{Text: "Recent message"}}},
			},
			wantLen: 3,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := optimizer.OptimizeContext(tt.messages)
			if len(result) != tt.wantLen {
				t.Errorf("OptimizeContext() len = %d, want %d", len(result), tt.wantLen)
			}

			// 验证 token 数在限制内
			tokens := EstimateMessagesTokens(result)
			maxAllowed := optimizer.maxTokens - optimizer.reserveTokens
			if tokens > maxAllowed {
				t.Errorf("OptimizeContext() tokens = %d, max allowed = %d", tokens, maxAllowed)
			}
		})
	}
}

func TestCalculatePriority(t *testing.T) {
	optimizer := NewContextOptimizer(1000, 200)

	tests := []struct {
		name     string
		msg      AgentMessage
		index    int
		total    int
		expected MessagePriority
	}{
		{
			name:     "system message",
			msg:      AgentMessage{Role: RoleSystem},
			index:    0,
			total:    10,
			expected: PriorityCritical,
		},
		{
			name:     "recent message",
			msg:      AgentMessage{Role: RoleUser},
			index:    9,
			total:    10,
			expected: PriorityHigh,
		},
		{
			name: "tool call message",
			msg: AgentMessage{
				Role:    RoleAssistant,
				Content: []ContentBlock{ToolCallContent{Name: "test"}},
			},
			index:    5,
			total:    10,
			expected: PriorityHigh,
		},
		{
			name:     "user message",
			msg:      AgentMessage{Role: RoleUser},
			index:    3,
			total:    10,
			expected: PriorityMedium,
		},
		{
			name:     "assistant message",
			msg:      AgentMessage{Role: RoleAssistant},
			index:    3,
			total:    10,
			expected: PriorityLow,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			priority := optimizer.calculatePriority(tt.msg, tt.index, tt.total)
			if priority != tt.expected {
				t.Errorf("calculatePriority() = %v, want %v", priority, tt.expected)
			}
		})
	}
}

func TestProgressiveCompression(t *testing.T) {
	pc := NewProgressiveCompression(1000, 200)

	messages := []AgentMessage{
		{Role: RoleUser, Content: []ContentBlock{TextContent{Text: generateLongText(500)}}},
		{Role: RoleAssistant, Content: []ContentBlock{
			TextContent{Text: generateLongText(500)},
			ThinkingContent{Thinking: "Some thinking"},
		}},
		{Role: RoleUser, Content: []ContentBlock{TextContent{Text: "Recent"}}},
	}

	result := pc.Compress(messages)

	// 验证结果不为空
	if len(result) == 0 {
		t.Error("Compress() returned empty result")
	}

	// 验证 token 数减少
	originalTokens := EstimateMessagesTokens(messages)
	compressedTokens := EstimateMessagesTokens(result)
	if compressedTokens >= originalTokens {
		t.Errorf("Compress() did not reduce tokens: %d >= %d", compressedTokens, originalTokens)
	}
}

func TestRemoveThinking(t *testing.T) {
	messages := []AgentMessage{
		{
			Role: RoleAssistant,
			Content: []ContentBlock{
				TextContent{Text: "Hello"},
				ThinkingContent{Thinking: "Thinking..."},
			},
		},
	}

	result := removeThinking(messages)

	if len(result) != 1 {
		t.Errorf("removeThinking() len = %d, want 1", len(result))
	}

	if len(result[0].Content) != 1 {
		t.Errorf("removeThinking() content len = %d, want 1", len(result[0].Content))
	}

	if _, ok := result[0].Content[0].(TextContent); !ok {
		t.Error("removeThinking() did not preserve TextContent")
	}
}

func TestCompressLongText(t *testing.T) {
	longText := generateLongText(2000)
	messages := []AgentMessage{
		{
			Role:    RoleUser,
			Content: []ContentBlock{TextContent{Text: longText}},
		},
	}

	result := compressLongText(messages, 1000)

	if len(result) != 1 {
		t.Errorf("compressLongText() len = %d, want 1", len(result))
	}

	textBlock := result[0].Content[0].(TextContent)
	if len(textBlock.Text) > 1020 { // 1000 + "... [truncated]"
		t.Errorf("compressLongText() text len = %d, want <= 1020", len(textBlock.Text))
	}
}

// generateLongText 生成指定长度的文本
func generateLongText(length int) string {
	text := ""
	for len(text) < length {
		text += "Lorem ipsum dolor sit amet, consectetur adipiscing elit. "
	}
	return text[:length]
}
