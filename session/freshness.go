package session

import (
	"strings"
	"time"
)

// ResetMode 重置模式：daily 按日重置，idle 按空闲时长
type ResetMode string

const (
	ResetModeDaily ResetMode = "daily"
	ResetModeIdle  ResetMode = "idle"
)

// ResetPolicy 会话重置策略（与 config.SessionResetConfig 对应）
type ResetPolicy struct {
	Mode        ResetMode
	AtHour      int // 0-23，daily 时在当日该小时重置
	IdleMinutes int // idle 时多少分钟无活动视为不新鲜
}

// EvaluateSessionFreshness 根据策略判断会话是否仍为“新鲜”。
// updatedAt 为会话最后更新时间，now 为当前时间。
// 返回 true 表示新鲜（可复用该会话），false 表示不新鲜（应创建新会话）。
func EvaluateSessionFreshness(updatedAt, now time.Time, policy ResetPolicy) bool {
	if policy.Mode == ResetModeIdle && policy.IdleMinutes > 0 {
		elapsed := now.Sub(updatedAt)
		return elapsed < time.Duration(policy.IdleMinutes)*time.Minute
	}
	if policy.Mode == ResetModeDaily && policy.AtHour >= 0 && policy.AtHour <= 23 {
		// 计算“上次重置点”：今天或昨天 policy.AtHour:00
		lastReset := lastDailyResetAt(now, policy.AtHour)
		return updatedAt.After(lastReset) || !updatedAt.Before(lastReset)
	}
	// 无有效策略时默认视为新鲜
	return true
}

// lastDailyResetAt 返回在 now 之前最近一次 atHour 时刻（0:00 为该小时）
func lastDailyResetAt(now time.Time, atHour int) time.Time {
	y, m, d := now.Date()
	reset := time.Date(y, m, d, atHour, 0, 0, 0, now.Location())
	if now.Before(reset) {
		reset = reset.AddDate(0, 0, -1)
	}
	return reset
}

// PolicyFromConfig 从 config 的 SessionResetConfig 转为 ResetPolicy（避免 session 包依赖 config）
type SessionResetConfigLike struct {
	Mode        string
	AtHour      int
	IdleMinutes int
}

// ToResetPolicy 将配置转为 ResetPolicy
func ToResetPolicy(c *SessionResetConfigLike) ResetPolicy {
	if c == nil {
		return ResetPolicy{}
	}
	mode := ResetModeDaily
	switch strings.ToLower(strings.TrimSpace(c.Mode)) {
	case "idle":
		mode = ResetModeIdle
	case "daily":
		mode = ResetModeDaily
	}
	atHour := c.AtHour
	if atHour < 0 || atHour > 23 {
		atHour = 4
	}
	return ResetPolicy{Mode: mode, AtHour: atHour, IdleMinutes: c.IdleMinutes}
}
