package agent

// ContextWindowSource 表示上下文窗口值的来源
type ContextWindowSource string

const (
	ContextWindowSourceDefault  ContextWindowSource = "default"
	ContextWindowSourceAgent   ContextWindowSource = "agent"
	ContextWindowSourceProfile ContextWindowSource = "profile"
	ContextWindowSourceModels  ContextWindowSource = "models"
)

// DefaultContextWindowTokens 无配置时的默认上下文窗口（约 128k）
const DefaultContextWindowTokens = 128000

// ContextWindowHardMinTokens 允许的最小上下文窗口
const ContextWindowHardMinTokens = 16000

// ContextWindowWarnBelowTokens 低于此值时告警
const ContextWindowWarnBelowTokens = 32000

// ResolveContextWindow 解析模型可用的上下文窗口 token 数。
// agentContextTokens: 来自 agents.defaults.context_tokens，0 表示不限制
// profileContextWindow: 来自 provider profile 的 context_window，0 表示用默认
// 返回 (tokens, source)
func ResolveContextWindow(agentContextTokens, profileContextWindow int) (tokens int, source ContextWindowSource) {
	// 优先 agent 配置的上限（cap）
	if agentContextTokens > 0 {
		if profileContextWindow > 0 && profileContextWindow < agentContextTokens {
			return profileContextWindow, ContextWindowSourceProfile
		}
		return agentContextTokens, ContextWindowSourceAgent
	}
	if profileContextWindow > 0 {
		return profileContextWindow, ContextWindowSourceProfile
	}
	return DefaultContextWindowTokens, ContextWindowSourceDefault
}

// EffectiveReserveTokens 压缩/截断时保留的 token 数（给系统提示与回复）
func EffectiveReserveTokens(reserve int) int {
	if reserve > 0 {
		return reserve
	}
	return 4096
}
