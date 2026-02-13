package agent

import (
	"strings"
)

// NormalizeDeliveryContext 规范化 DeliveryContext：去空白、统一格式（与 OpenClaw normalizeDeliveryContext 对齐）
func NormalizeDeliveryContext(dc *DeliveryContext) *DeliveryContext {
	if dc == nil {
		return nil
	}
	channel := strings.TrimSpace(dc.Channel)
	to := strings.TrimSpace(dc.To)
	accountID := strings.TrimSpace(dc.AccountID)
	threadID := strings.TrimSpace(dc.ThreadID)
	if channel == "" && to == "" && accountID == "" && threadID == "" {
		return nil
	}
	return &DeliveryContext{
		Channel:   channel,
		To:        to,
		AccountID: accountID,
		ThreadID:  threadID,
	}
}

// MergeDeliveryContext 合并 primary 与 fallback，primary 优先（与 OpenClaw mergeDeliveryContext 对齐）
func MergeDeliveryContext(primary, fallback *DeliveryContext) *DeliveryContext {
	p := NormalizeDeliveryContext(primary)
	f := NormalizeDeliveryContext(fallback)
	if p == nil && f == nil {
		return nil
	}
	if p == nil {
		return f
	}
	if f == nil {
		return p
	}
	out := &DeliveryContext{
		Channel:   p.Channel,
		To:        p.To,
		AccountID: p.AccountID,
		ThreadID:  p.ThreadID,
	}
	if out.Channel == "" {
		out.Channel = f.Channel
	}
	if out.To == "" {
		out.To = f.To
	}
	if out.AccountID == "" {
		out.AccountID = f.AccountID
	}
	if out.ThreadID == "" {
		out.ThreadID = f.ThreadID
	}
	return out
}

// DeliveryContextKey 返回用于去重或路由的键（channel|to|accountID|threadID）
func DeliveryContextKey(dc *DeliveryContext) string {
	dc = NormalizeDeliveryContext(dc)
	if dc == nil || (dc.Channel == "" && dc.To == "") {
		return ""
	}
	threadID := dc.ThreadID
	if threadID == "" {
		threadID = ""
	}
	return strings.Join([]string{dc.Channel, dc.To, dc.AccountID, threadID}, "|")
}
