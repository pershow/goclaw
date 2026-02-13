package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/smallnest/goclaw/session"
)

// SessionsListTool 列出会话 key 列表（与 OpenClaw sessions-list 对齐）
type SessionsListTool struct {
	sessionMgr *session.Manager
}

// NewSessionsListTool 创建 sessions_list 工具
func NewSessionsListTool(sessionMgr *session.Manager) *SessionsListTool {
	return &SessionsListTool{sessionMgr: sessionMgr}
}

// Name 返回工具名
func (t *SessionsListTool) Name() string {
	return "sessions_list"
}

// Description 返回描述
func (t *SessionsListTool) Description() string {
	return "List session keys (identifiers) for the current agent. Returns a list of session keys; use sessions_history with a key to read messages."
}

// Parameters 返回参数 schema
func (t *SessionsListTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"limit": map[string]interface{}{
				"type":        "integer",
				"description": "Maximum number of session keys to return (default 50)",
			},
		},
	}
}

// Execute 执行
func (t *SessionsListTool) Execute(ctx context.Context, params map[string]interface{}) (string, error) {
	keys, err := t.sessionMgr.List()
	if err != nil {
		return "", fmt.Errorf("list sessions: %w", err)
	}
	limit := 50
	if l, ok := params["limit"].(float64); ok && l > 0 {
		limit = int(l)
	}
	if limit < len(keys) {
		keys = keys[:limit]
	}
	out, _ := json.Marshal(keys)
	return string(out), nil
}

// SessionsHistoryTool 获取指定会话的历史消息（只读）
type SessionsHistoryTool struct {
	sessionMgr *session.Manager
}

// NewSessionsHistoryTool 创建 sessions_history 工具
func NewSessionsHistoryTool(sessionMgr *session.Manager) *SessionsHistoryTool {
	return &SessionsHistoryTool{sessionMgr: sessionMgr}
}

// Name 返回工具名
func (t *SessionsHistoryTool) Name() string {
	return "sessions_history"
}

// Description 返回描述
func (t *SessionsHistoryTool) Description() string {
	return "Get recent messages for a session by session_key. Returns message list (role, content, timestamp) for the given session."
}

// Parameters 返回参数 schema
func (t *SessionsHistoryTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"session_key": map[string]interface{}{
				"type":        "string",
				"description": "Session key (from sessions_list)",
			},
			"limit": map[string]interface{}{
				"type":        "integer",
				"description": "Max messages to return (default 20)",
			},
		},
		"required": []interface{}{"session_key"},
	}
}

// Execute 执行
func (t *SessionsHistoryTool) Execute(ctx context.Context, params map[string]interface{}) (string, error) {
	key, _ := params["session_key"].(string)
	if key == "" {
		return "", fmt.Errorf("session_key is required")
	}
	sess, err := t.sessionMgr.GetOrCreate(key)
	if err != nil {
		return "", fmt.Errorf("get session: %w", err)
	}
	limit := 20
	if l, ok := params["limit"].(float64); ok && l > 0 {
		limit = int(l)
	}
	history := sess.GetHistory(limit)
	type msgRow struct {
		Role      string `json:"role"`
		Content   string `json:"content"`
		Timestamp string `json:"timestamp"`
	}
	rows := make([]msgRow, 0, len(history))
	for _, m := range history {
		rows = append(rows, msgRow{
			Role:      m.Role,
			Content:   m.Content,
			Timestamp: m.Timestamp.Format(time.RFC3339),
		})
	}
	out, _ := json.Marshal(rows)
	return string(out), nil
}

// SessionsSendTool 向指定会话发送一条消息（需注入发送回调）
type SessionsSendTool struct {
	sessionMgr *session.Manager
	sendFunc   func(ctx context.Context, sessionKey, content string) error
}

// NewSessionsSendTool 创建 sessions_send 工具；sendFunc 为向会话发送消息的实现（如通过 bus）
func NewSessionsSendTool(sessionMgr *session.Manager, sendFunc func(ctx context.Context, sessionKey, content string) error) *SessionsSendTool {
	return &SessionsSendTool{sessionMgr: sessionMgr, sendFunc: sendFunc}
}

// Name 返回工具名
func (t *SessionsSendTool) Name() string {
	return "sessions_send"
}

// Description 返回描述
func (t *SessionsSendTool) Description() string {
	return "Send a message to a session by session_key. The message will be delivered to that session's channel/chat."
}

// Parameters 返回参数 schema
func (t *SessionsSendTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"session_key": map[string]interface{}{
				"type":        "string",
				"description": "Target session key",
			},
			"content": map[string]interface{}{
				"type":        "string",
				"description": "Message content to send",
			},
		},
		"required": []interface{}{"session_key", "content"},
	}
}

// Execute 执行
func (t *SessionsSendTool) Execute(ctx context.Context, params map[string]interface{}) (string, error) {
	if t.sendFunc == nil {
		return "", fmt.Errorf("sessions_send not configured (no send callback)")
	}
	key, _ := params["session_key"].(string)
	content, _ := params["content"].(string)
	if key == "" || content == "" {
		return "", fmt.Errorf("session_key and content are required")
	}
	if err := t.sendFunc(ctx, key, content); err != nil {
		return "", fmt.Errorf("send: %w", err)
	}
	return fmt.Sprintf("Message sent to session %s", key), nil
}

// SessionStatusTool 返回当前会话状态（key、消息数、最后活动时间）
type SessionStatusTool struct {
	sessionMgr *session.Manager
	currentKey func() string // 当前会话 key，由调用方注入
}

// NewSessionStatusTool 创建 session_status 工具
func NewSessionStatusTool(sessionMgr *session.Manager, currentKey func() string) *SessionStatusTool {
	return &SessionStatusTool{sessionMgr: sessionMgr, currentKey: currentKey}
}

// Name 返回工具名
func (t *SessionStatusTool) Name() string {
	return "session_status"
}

// Description 返回描述
func (t *SessionStatusTool) Description() string {
	return "Get status of the current session: session_key, message count, last updated. Optionally pass session_key to query another session."
}

// Parameters 返回参数 schema
func (t *SessionStatusTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"session_key": map[string]interface{}{
				"type":        "string",
				"description": "Session key to query; omit for current session",
			},
		},
	}
}

// Execute 执行
func (t *SessionStatusTool) Execute(ctx context.Context, params map[string]interface{}) (string, error) {
	key, _ := params["session_key"].(string)
	if key == "" && t.currentKey != nil {
		key = t.currentKey()
	}
	if key == "" {
		return "", fmt.Errorf("session_key required or set current session")
	}
	sess, err := t.sessionMgr.GetOrCreate(key)
	if err != nil {
		return "", fmt.Errorf("get session: %w", err)
	}
	history := sess.GetHistory(0)
	msgCount := len(history)
	updatedAt := sess.UpdatedAt
	out := map[string]interface{}{
		"session_key":   key,
		"message_count": msgCount,
		"updated_at":    updatedAt.Format(time.RFC3339),
	}
	enc, _ := json.Marshal(out)
	return string(enc), nil
}
