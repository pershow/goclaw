package bus

import (
	"time"
)

// InboundMessage 入站消息
type InboundMessage struct {
	ID        string                 `json:"id"`
	Channel   string                 `json:"channel"`    // telegram, whatsapp, feishu, cli, system
	AccountID string                 `json:"account_id"` // 账号ID（用于多账号场景）
	SenderID  string                 `json:"sender_id"`  // 发送者ID
	ChatID    string                 `json:"chat_id"`    // 聊天ID
	Content   string                 `json:"content"`    // 消息内容
	Media     []Media                `json:"media"`      // 媒体文件
	Metadata  map[string]interface{} `json:"metadata"`   // 元数据
	Timestamp time.Time              `json:"timestamp"`
}

// Media 媒体文件
type Media struct {
	Type     string `json:"type"`     // image, video, audio, document
	URL      string `json:"url"`      // 文件URL
	Base64   string `json:"base64"`   // Base64编码内容
	MimeType string `json:"mimetype"` // MIME类型
}

// SessionKey 返回会话键
func (m *InboundMessage) SessionKey() string {
	return m.Channel + ":" + m.ChatID
}

// OutboundMessage 出站消息
type OutboundMessage struct {
	ID        string                 `json:"id"`
	Channel   string                 `json:"channel"`    // telegram, whatsapp, feishu, cli
	ChatID    string                 `json:"chat_id"`    // 聊天ID
	Content   string                 `json:"content"`    // 消息内容
	Media     []Media                `json:"media"`     // 媒体文件
	ReplyTo   string                 `json:"reply_to"`    // 回复的消息ID
	Metadata  map[string]interface{} `json:"metadata"`   // 元数据
	IsStream  bool                   `json:"is_stream"`  // 是否为流式增量内容
	ChatState string                 `json:"chat_state"` // 可选：chat 事件 state，如 "error"、"aborted"；空则按 IsStream 推导 delta/final
	Timestamp time.Time              `json:"timestamp"`
}

// SystemMessage 系统消息（用于子代理结果通知）
type SystemMessage struct {
	InboundMessage
	TaskID    string `json:"task_id"`    // 任务ID
	TaskLabel string `json:"task_label"` // 任务标签
	Status    string `json:"status"`     // completed, failed
}

// IsSystemMessage 判断是否为系统消息
func (m *InboundMessage) IsSystemMessage() bool {
	return m.Channel == "system"
}

// AgentEventStream 与 OpenClaw 对齐：lifecycle | tool | assistant | error
type AgentEventStream string

const (
	AgentStreamLifecycle AgentEventStream = "lifecycle"
	AgentStreamTool      AgentEventStream = "tool"
	AgentStreamAssistant AgentEventStream = "assistant"
	AgentStreamError     AgentEventStream = "error"
)

// AgentEventPayload 与 OpenClaw infra/agent-events.ts 一致，供 Control UI 显示进度与工具执行
type AgentEventPayload struct {
	RunId      string                 `json:"runId"`
	Seq        int                    `json:"seq"`
	Stream     AgentEventStream       `json:"stream"`
	Ts         int64                  `json:"ts"`
	Data       map[string]interface{} `json:"data"`
	SessionKey string                 `json:"sessionKey,omitempty"`
}
