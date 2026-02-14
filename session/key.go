package session

import (
	"strings"
)

// SessionScope 会话作用域：per-sender 按发送者（及可选 channel/thread）区分，global 全局单一会话
type SessionScope string

const (
	ScopePerSender SessionScope = "per-sender"
	ScopeGlobal    SessionScope = "global"
)

// DefaultAgentID 默认 agent ID
const DefaultAgentID = "main"

// DefaultMainKey 默认主会话 key
const DefaultMainKey = "main"

// BuildAgentMainSessionKey 构建 agent 主会话 key，格式：agent:<agentId>:<mainKey>
// 与 OpenClaw 的 buildAgentMainSessionKey 对齐
func BuildAgentMainSessionKey(agentID, mainKey string) string {
	if agentID == "" {
		agentID = DefaultAgentID
	}
	if mainKey == "" {
		mainKey = DefaultMainKey
	}
	return "agent:" + agentID + ":" + mainKey
}

// BuildAgentSessionKey 根据消息上下文构建 session key
// 与 OpenClaw 对齐：所有 session key 都以 agent:<agentId>: 开头
func BuildAgentSessionKey(agentID, channel, accountID, chatID, mainKey string, isGroup bool) string {
	if agentID == "" {
		agentID = DefaultAgentID
	}
	if mainKey == "" {
		mainKey = DefaultMainKey
	}

	// 群组消息保持独立的 session
	if isGroup {
		// 格式：agent:<agentId>:<channel>:group:<chatID>
		if accountID != "" {
			return "agent:" + agentID + ":" + channel + ":" + accountID + ":group:" + chatID
		}
		return "agent:" + agentID + ":" + channel + ":group:" + chatID
	}

	// DM 消息折叠到主会话（与 OpenClaw dmScope="main" 一致）
	return BuildAgentMainSessionKey(agentID, mainKey)
}

// ParseAgentSessionKey 解析 session key，提取 agentId
// 格式：agent:<agentId>:<rest>
func ParseAgentSessionKey(sessionKey string) (agentID string, rest string, ok bool) {
	if !strings.HasPrefix(sessionKey, "agent:") {
		return "", "", false
	}
	parts := strings.SplitN(sessionKey[6:], ":", 2) // 跳过 "agent:" 前缀
	if len(parts) == 0 {
		return "", "", false
	}
	agentID = parts[0]
	if len(parts) > 1 {
		rest = parts[1]
	}
	return agentID, rest, true
}

// IsAgentSessionKey 检查是否为 agent session key 格式
func IsAgentSessionKey(sessionKey string) bool {
	return strings.HasPrefix(sessionKey, "agent:")
}

// IsSubagentSessionKey 检查是否为子 agent 会话 key（与 OpenClaw 一致：agent:<id>:subagent:<uuid>）
func IsSubagentSessionKey(sessionKey string) bool {
	return strings.Contains(sessionKey, ":subagent:")
}

// IsGroupSessionKey 检查是否为群组 session key
func IsGroupSessionKey(sessionKey string) bool {
	return strings.Contains(sessionKey, ":group:")
}

// ResolveParams 解析会话 key 的入参
type ResolveParams struct {
	Scope    SessionScope // 来自配置 session.scope
	Channel  string       // 通道 ID，如 telegram, slack
	From     string       // 发送者 ID（用户/群 ID）
	ThreadID string       // 线程/话题 ID，空表示非线程
	MainKey  string       // 主会话键，用于将非群组直聊统一到一个 key，如 "main"
}

// ResolveSessionKey 根据作用域与上下文解析会话 key，供 gateway 与各 channel 调用。
// 与 OpenClaw 的 deriveSessionKey / resolveSessionKey 对齐语义。
func ResolveSessionKey(p ResolveParams) string {
	scope := SessionScope(strings.TrimSpace(string(p.Scope)))
	if scope == "" {
		scope = ScopePerSender
	}
	if scope == ScopeGlobal {
		return "global"
	}
	// per-sender: 按 channel、发送者、线程派生
	channel := strings.TrimSpace(p.Channel)
	from := strings.TrimSpace(p.From)
	threadID := strings.TrimSpace(p.ThreadID)
	mainKey := strings.TrimSpace(p.MainKey)
	if mainKey == "" {
		mainKey = "main"
	}
	// 群组/频道：key 包含 :group: 或 :channel: 以区分
	isGroup := isGroupLike(from, channel)
	if isGroup {
		return buildGroupKey(channel, from, threadID)
	}
	// 直聊：可折叠到 mainKey（与 openclaw mainKey 一致）
	if threadID != "" {
		return mainKey + ":direct:" + from + ":thread:" + threadID
	}
	return mainKey + ":direct:" + from
}

// isGroupLike 根据 from/channel 启发式判断是否为群组会话（可根据实际渠道扩展）
func isGroupLike(from, channel string) bool {
	// 若 from 已包含群组标记则视为群组
	lower := strings.ToLower(from)
	if strings.Contains(lower, ":group:") || strings.Contains(lower, ":channel:") {
		return true
	}
	// 部分渠道的群 ID 与用户 ID 格式不同，可按渠道扩展
	return false
}

// buildGroupKey 构建群组/频道会话 key
func buildGroupKey(channel, from, threadID string) string {
	if channel == "" {
		channel = "unknown"
	}
	if from == "" {
		from = "unknown"
	}
	key := channel + ":group:" + from
	if threadID != "" {
		key += ":thread:" + threadID
	}
	return key
}

// NormalizeScope 将配置字符串规范为 SessionScope
func NormalizeScope(s string) SessionScope {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "global":
		return ScopeGlobal
	case "per-sender", "persender":
		return ScopePerSender
	default:
		return ScopePerSender
	}
}

// KeyToSafeFilename 将 session key 转换为文件系统安全的文件名
// 例如：agent:main:main -> agent_main_main
func KeyToSafeFilename(key string) string {
	return strings.Map(func(r rune) rune {
		if r == '/' || r == '\\' || r == ':' || r == '*' || r == '?' || r == '"' || r == '<' || r == '>' || r == '|' {
			return '_'
		}
		return r
	}, key)
}

// SafeFilenameToKey 将文件系统安全的文件名转换回 session key
// 例如：agent_main_main -> agent:main:main
// 注意：这是一个启发式转换，只处理 agent_xxx_xxx 格式
func SafeFilenameToKey(filename string) string {
	// 处理 agent_xxx_xxx 格式 -> agent:xxx:xxx
	if strings.HasPrefix(filename, "agent_") {
		// agent_main_main -> agent:main:main
		// agent_main_telegram_123_group_456 -> agent:main:telegram:123:group:456
		rest := filename[6:] // 去掉 "agent_" 前缀
		// 找到第一个下划线位置（agentId 和 rest 的分隔）
		idx := strings.Index(rest, "_")
		if idx > 0 {
			agentID := rest[:idx]
			remaining := rest[idx+1:]
			// 将剩余部分的下划线替换为冒号
			remaining = strings.ReplaceAll(remaining, "_", ":")
			return "agent:" + agentID + ":" + remaining
		}
	}
	// 其他格式：将下划线替换为冒号
	return strings.ReplaceAll(filename, "_", ":")
}
