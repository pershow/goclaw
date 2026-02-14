package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/smallnest/goclaw/internal/logger"
	"go.uber.org/zap"
)

// ConfigChange 配置变更记录
type ConfigChange struct {
	Timestamp   time.Time              `json:"timestamp"`
	Changes     map[string]interface{} `json:"changes"`
	OldConfig   *Config                `json:"old_config,omitempty"`
	NewConfig   *Config                `json:"new_config,omitempty"`
	Success     bool                   `json:"success"`
	Error       string                 `json:"error,omitempty"`
	TriggeredBy string                 `json:"triggered_by"` // "auto" | "manual"
}

// ConfigHistory 配置变更历史管理器
type ConfigHistory struct {
	historyFile string
	maxEntries  int
	entries     []ConfigChange
	mu          sync.RWMutex
}

// NewConfigHistory 创建配置历史管理器
func NewConfigHistory(historyFile string, maxEntries int) (*ConfigHistory, error) {
	if maxEntries <= 0 {
		maxEntries = 100 // 默认保留 100 条记录
	}

	h := &ConfigHistory{
		historyFile: historyFile,
		maxEntries:  maxEntries,
		entries:     make([]ConfigChange, 0),
	}

	// 加载历史记录
	if err := h.load(); err != nil {
		logger.Warn("Failed to load config history", zap.Error(err))
	}

	return h, nil
}

// Record 记录配置变更
func (h *ConfigHistory) Record(oldCfg, newCfg *Config, success bool, err error, triggeredBy string) error {
	h.mu.Lock()
	defer h.mu.Unlock()

	change := ConfigChange{
		Timestamp:   time.Now(),
		Changes:     h.detectChanges(oldCfg, newCfg),
		OldConfig:   oldCfg,
		NewConfig:   newCfg,
		Success:     success,
		TriggeredBy: triggeredBy,
	}

	if err != nil {
		change.Error = err.Error()
	}

	// 添加到历史记录
	h.entries = append(h.entries, change)

	// 限制历史记录数量
	if len(h.entries) > h.maxEntries {
		h.entries = h.entries[len(h.entries)-h.maxEntries:]
	}

	// 保存到文件
	return h.save()
}

// detectChanges 检测配置变更
func (h *ConfigHistory) detectChanges(oldCfg, newCfg *Config) map[string]interface{} {
	changes := make(map[string]interface{})

	if oldCfg == nil || newCfg == nil {
		return changes
	}

	// 检测 Gateway 配置变更
	if oldCfg.Gateway.Port != newCfg.Gateway.Port {
		changes["gateway.port"] = map[string]interface{}{
			"old": oldCfg.Gateway.Port,
			"new": newCfg.Gateway.Port,
		}
	}

	if oldCfg.Gateway.Host != newCfg.Gateway.Host {
		changes["gateway.host"] = map[string]interface{}{
			"old": oldCfg.Gateway.Host,
			"new": newCfg.Gateway.Host,
		}
	}

	// 检测 WebSocket 配置变更
	if oldCfg.Gateway.WebSocket.Port != newCfg.Gateway.WebSocket.Port {
		changes["gateway.websocket.port"] = map[string]interface{}{
			"old": oldCfg.Gateway.WebSocket.Port,
			"new": newCfg.Gateway.WebSocket.Port,
		}
	}

	// 检测 Agent 配置变更
	if oldCfg.Agents.Defaults.Model != newCfg.Agents.Defaults.Model {
		changes["agents.defaults.model"] = map[string]interface{}{
			"old": oldCfg.Agents.Defaults.Model,
			"new": newCfg.Agents.Defaults.Model,
		}
	}

	if oldCfg.Agents.Defaults.Temperature != newCfg.Agents.Defaults.Temperature {
		changes["agents.defaults.temperature"] = map[string]interface{}{
			"old": oldCfg.Agents.Defaults.Temperature,
			"new": newCfg.Agents.Defaults.Temperature,
		}
	}

	if oldCfg.Agents.Defaults.MaxTokens != newCfg.Agents.Defaults.MaxTokens {
		changes["agents.defaults.max_tokens"] = map[string]interface{}{
			"old": oldCfg.Agents.Defaults.MaxTokens,
			"new": newCfg.Agents.Defaults.MaxTokens,
		}
	}

	return changes
}

// GetHistory 获取历史记录
func (h *ConfigHistory) GetHistory(limit int) []ConfigChange {
	h.mu.RLock()
	defer h.mu.RUnlock()

	if limit <= 0 || limit > len(h.entries) {
		limit = len(h.entries)
	}

	// 返回最近的 N 条记录
	start := len(h.entries) - limit
	result := make([]ConfigChange, limit)
	copy(result, h.entries[start:])

	return result
}

// GetLatest 获取最新的配置变更
func (h *ConfigHistory) GetLatest() *ConfigChange {
	h.mu.RLock()
	defer h.mu.RUnlock()

	if len(h.entries) == 0 {
		return nil
	}

	return &h.entries[len(h.entries)-1]
}

// Clear 清空历史记录
func (h *ConfigHistory) Clear() error {
	h.mu.Lock()
	defer h.mu.Unlock()

	h.entries = make([]ConfigChange, 0)
	return h.save()
}

// load 从文件加载历史记录
func (h *ConfigHistory) load() error {
	if h.historyFile == "" {
		return nil
	}

	// 检查文件是否存在
	if _, err := os.Stat(h.historyFile); os.IsNotExist(err) {
		return nil
	}

	// 读取文件
	data, err := os.ReadFile(h.historyFile)
	if err != nil {
		return fmt.Errorf("failed to read history file: %w", err)
	}

	// 解析 JSON
	if err := json.Unmarshal(data, &h.entries); err != nil {
		return fmt.Errorf("failed to unmarshal history: %w", err)
	}

	logger.Info("Config history loaded",
		zap.String("file", h.historyFile),
		zap.Int("entries", len(h.entries)))

	return nil
}

// save 保存历史记录到文件
func (h *ConfigHistory) save() error {
	if h.historyFile == "" {
		return nil
	}

	// 确保目录存在
	dir := filepath.Dir(h.historyFile)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create history directory: %w", err)
	}

	// 序列化为 JSON
	data, err := json.MarshalIndent(h.entries, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal history: %w", err)
	}

	// 写入文件
	if err := os.WriteFile(h.historyFile, data, 0600); err != nil {
		return fmt.Errorf("failed to write history file: %w", err)
	}

	return nil
}

// Rollback 回滚到指定的配置
func (h *ConfigHistory) Rollback(index int) (*Config, error) {
	h.mu.RLock()
	defer h.mu.RUnlock()

	if index < 0 || index >= len(h.entries) {
		return nil, fmt.Errorf("invalid history index: %d", index)
	}

	entry := h.entries[index]
	if entry.OldConfig == nil {
		return nil, fmt.Errorf("no old config available for rollback")
	}

	return entry.OldConfig, nil
}

// RollbackToLatest 回滚到最近一次成功的配置
func (h *ConfigHistory) RollbackToLatest() (*Config, error) {
	h.mu.RLock()
	defer h.mu.RUnlock()

	// 从后往前查找最近一次成功的配置变更
	for i := len(h.entries) - 1; i >= 0; i-- {
		if h.entries[i].Success && h.entries[i].OldConfig != nil {
			return h.entries[i].OldConfig, nil
		}
	}

	return nil, fmt.Errorf("no successful config change found in history")
}
