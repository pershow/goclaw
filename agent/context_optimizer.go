package agent

import (
	"sort"
)

// MessagePriority 消息优先级
type MessagePriority int

const (
	PriorityLow      MessagePriority = 1
	PriorityMedium   MessagePriority = 2
	PriorityHigh     MessagePriority = 3
	PriorityCritical MessagePriority = 4
)

// PrioritizedMessage 带优先级的消息
type PrioritizedMessage struct {
	Message  AgentMessage
	Priority MessagePriority
	Index    int // 原始索引，用于保持相对顺序
}

// ContextOptimizer 上下文优化器
type ContextOptimizer struct {
	maxTokens     int
	reserveTokens int
}

// NewContextOptimizer 创建上下文优化器
func NewContextOptimizer(maxTokens, reserveTokens int) *ContextOptimizer {
	return &ContextOptimizer{
		maxTokens:     maxTokens,
		reserveTokens: reserveTokens,
	}
}

// OptimizeContext 优化上下文（智能压缩和截断）
func (o *ContextOptimizer) OptimizeContext(messages []AgentMessage) []AgentMessage {
	if len(messages) == 0 {
		return messages
	}

	// 估算当前 token 数
	currentTokens := EstimateMessagesTokens(messages)
	availableTokens := o.maxTokens - o.reserveTokens

	// 如果未超限，直接返回
	if currentTokens <= availableTokens {
		return messages
	}

	// 1. 标记关键消息
	prioritized := o.prioritizeMessages(messages)

	// 2. 压缩非关键消息
	compressed := o.compressMessages(prioritized, availableTokens)

	// 3. 如果仍超限，截断旧消息
	if EstimateMessagesTokens(compressed) > availableTokens {
		compressed = o.truncateOldMessages(compressed, availableTokens)
	}

	return compressed
}

// prioritizeMessages 为消息分配优先级
func (o *ContextOptimizer) prioritizeMessages(messages []AgentMessage) []PrioritizedMessage {
	prioritized := make([]PrioritizedMessage, len(messages))

	for i, msg := range messages {
		priority := o.calculatePriority(msg, i, len(messages))
		prioritized[i] = PrioritizedMessage{
			Message:  msg,
			Priority: priority,
			Index:    i,
		}
	}

	return prioritized
}

// calculatePriority 计算消息优先级
func (o *ContextOptimizer) calculatePriority(msg AgentMessage, index, total int) MessagePriority {
	// 1. 系统消息：最高优先级
	if msg.Role == RoleSystem {
		return PriorityCritical
	}

	// 2. 最近的消息：高优先级（最后 3 条）
	if index >= total-3 {
		return PriorityHigh
	}

	// 3. 包含工具调用的消息：高优先级
	hasToolCall := false
	for _, block := range msg.Content {
		if _, ok := block.(ToolCallContent); ok {
			hasToolCall = true
			break
		}
	}
	if hasToolCall || msg.Role == RoleToolResult {
		return PriorityHigh
	}

	// 4. 用户消息：中等优先级
	if msg.Role == RoleUser {
		return PriorityMedium
	}

	// 5. 其他消息：低优先级
	return PriorityLow
}

// compressMessages 压缩非关键消息
func (o *ContextOptimizer) compressMessages(prioritized []PrioritizedMessage, targetTokens int) []AgentMessage {
	result := make([]AgentMessage, 0, len(prioritized))
	currentTokens := 0

	// 按优先级排序（保持原始顺序作为次要排序）
	sorted := make([]PrioritizedMessage, len(prioritized))
	copy(sorted, prioritized)
	sort.Slice(sorted, func(i, j int) bool {
		if sorted[i].Priority != sorted[j].Priority {
			return sorted[i].Priority > sorted[j].Priority
		}
		return sorted[i].Index < sorted[j].Index
	})

	// 收集消息，压缩低优先级消息
	for _, pm := range sorted {
		msg := pm.Message
		msgTokens := EstimateMessageTokens(msg)

		// 如果是低优先级且会超限，压缩消息
		if pm.Priority == PriorityLow && currentTokens+msgTokens > targetTokens {
			compressed := o.compressMessage(msg)
			compressedTokens := EstimateMessageTokens(compressed)
			if currentTokens+compressedTokens <= targetTokens {
				result = append(result, compressed)
				currentTokens += compressedTokens
			}
		} else {
			result = append(result, msg)
			currentTokens += msgTokens
		}

		// 如果已经超限，停止添加
		if currentTokens > targetTokens {
			break
		}
	}

	// 恢复原始顺序
	sort.Slice(result, func(i, j int) bool {
		return o.findOriginalIndex(result[i], prioritized) < o.findOriginalIndex(result[j], prioritized)
	})

	return result
}

// compressMessage 压缩单条消息
func (o *ContextOptimizer) compressMessage(msg AgentMessage) AgentMessage {
	compressed := msg

	// 压缩文本内容
	newContent := make([]ContentBlock, 0, len(msg.Content))
	for _, block := range msg.Content {
		switch b := block.(type) {
		case TextContent:
			// 截断长文本
			text := b.Text
			if len(text) > 500 {
				text = text[:500] + "... [truncated]"
			}
			newContent = append(newContent, TextContent{Text: text})
		case ThinkingContent:
			// 移除 thinking 内容
			continue
		default:
			// 保留其他内容
			newContent = append(newContent, block)
		}
	}

	compressed.Content = newContent
	return compressed
}

// truncateOldMessages 截断旧消息
func (o *ContextOptimizer) truncateOldMessages(messages []AgentMessage, targetTokens int) []AgentMessage {
	if len(messages) == 0 {
		return messages
	}

	// 从后往前保留消息，直到达到 token 限制
	currentTokens := 0
	startIndex := len(messages)

	for i := len(messages) - 1; i >= 0; i-- {
		msgTokens := EstimateMessageTokens(messages[i])
		if currentTokens+msgTokens > targetTokens {
			break
		}
		currentTokens += msgTokens
		startIndex = i
	}

	// 确保至少保留最后一条用户消息
	if startIndex >= len(messages) {
		// 找到最后一条用户消息
		for i := len(messages) - 1; i >= 0; i-- {
			if messages[i].Role == RoleUser {
				startIndex = i
				break
			}
		}
	}

	return messages[startIndex:]
}

// findOriginalIndex 查找消息的原始索引
func (o *ContextOptimizer) findOriginalIndex(msg AgentMessage, prioritized []PrioritizedMessage) int {
	for _, pm := range prioritized {
		if o.messagesEqual(msg, pm.Message) {
			return pm.Index
		}
	}
	return 0
}

// messagesEqual 比较两条消息是否相等
func (o *ContextOptimizer) messagesEqual(a, b AgentMessage) bool {
	if a.Role != b.Role || a.Timestamp != b.Timestamp {
		return false
	}
	if len(a.Content) != len(b.Content) {
		return false
	}
	return true
}

// ProgressiveCompression 渐进式压缩
type ProgressiveCompression struct {
	optimizer *ContextOptimizer
	levels    []CompressionLevel
}

// CompressionLevel 压缩级别
type CompressionLevel struct {
	Name        string
	MaxTokens   int
	Compression func([]AgentMessage) []AgentMessage
}

// NewProgressiveCompression 创建渐进式压缩器
func NewProgressiveCompression(maxTokens, reserveTokens int) *ProgressiveCompression {
	optimizer := NewContextOptimizer(maxTokens, reserveTokens)

	return &ProgressiveCompression{
		optimizer: optimizer,
		levels: []CompressionLevel{
			{
				Name:      "none",
				MaxTokens: maxTokens,
				Compression: func(msgs []AgentMessage) []AgentMessage {
					return msgs
				},
			},
			{
				Name:      "light",
				MaxTokens: int(float64(maxTokens) * 0.8),
				Compression: func(msgs []AgentMessage) []AgentMessage {
					// 移除 thinking 内容
					return removeThinking(msgs)
				},
			},
			{
				Name:      "medium",
				MaxTokens: int(float64(maxTokens) * 0.6),
				Compression: func(msgs []AgentMessage) []AgentMessage {
					// 压缩长文本
					return compressLongText(msgs, 1000)
				},
			},
			{
				Name:      "heavy",
				MaxTokens: int(float64(maxTokens) * 0.4),
				Compression: func(msgs []AgentMessage) []AgentMessage {
					// 使用优化器
					return optimizer.OptimizeContext(msgs)
				},
			},
		},
	}
}

// Compress 执行渐进式压缩
func (p *ProgressiveCompression) Compress(messages []AgentMessage) []AgentMessage {
	currentTokens := EstimateMessagesTokens(messages)

	// 尝试每个压缩级别
	for _, level := range p.levels {
		if currentTokens <= level.MaxTokens {
			return level.Compression(messages)
		}
	}

	// 如果所有级别都不够，使用最高级别
	return p.levels[len(p.levels)-1].Compression(messages)
}

// removeThinking 移除 thinking 内容
func removeThinking(messages []AgentMessage) []AgentMessage {
	result := make([]AgentMessage, len(messages))
	for i, msg := range messages {
		newContent := make([]ContentBlock, 0, len(msg.Content))
		for _, block := range msg.Content {
			if _, ok := block.(ThinkingContent); !ok {
				newContent = append(newContent, block)
			}
		}
		result[i] = msg
		result[i].Content = newContent
	}
	return result
}

// compressLongText 压缩长文本
func compressLongText(messages []AgentMessage, maxLength int) []AgentMessage {
	result := make([]AgentMessage, len(messages))
	for i, msg := range messages {
		newContent := make([]ContentBlock, 0, len(msg.Content))
		for _, block := range msg.Content {
			switch b := block.(type) {
			case TextContent:
				text := b.Text
				if len(text) > maxLength {
					text = text[:maxLength] + "... [truncated]"
				}
				newContent = append(newContent, TextContent{Text: text})
			default:
				newContent = append(newContent, block)
			}
		}
		result[i] = msg
		result[i].Content = newContent
	}
	return result
}
