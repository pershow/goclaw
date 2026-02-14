package gateway

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/smallnest/goclaw/bus"
	"github.com/smallnest/goclaw/channels"
	"github.com/smallnest/goclaw/config"
	"github.com/smallnest/goclaw/internal/logger"
	"github.com/smallnest/goclaw/session"
	"go.uber.org/zap"
)

// PresenceProvider 提供当前连接的 presence 列表（由 Server 实现）
type PresenceProvider interface {
	GetPresenceEntries() []map[string]interface{}
}

// Handler WebSocket 消息处理器
type Handler struct {
	registry          *MethodRegistry
	bus               *bus.MessageBus
	sessionMgr        *session.Manager
	channelMgr        *channels.Manager
	sessionPolicy     *session.ResetPolicy // 可选：与 OpenClaw 对齐，不新鲜会话自动重置
	cronStore         *cronStore
	devicesStore      *devicesStore
	execApprovalsStore *execApprovalsStore
	skillsStore       *skillsStore
	presenceProvider  PresenceProvider
	lastHeartbeatGetter func() int64
}

// SetSessionResetPolicy 设置会话重置策略（由 Server 在启动时根据 config.session.reset 注入）
func (h *Handler) SetSessionResetPolicy(policy *session.ResetPolicy) {
	h.sessionPolicy = policy
}

// SetPresenceProvider 设置 presence 数据源（由 Server 在启动后注入）
func (h *Handler) SetPresenceProvider(p PresenceProvider) {
	h.presenceProvider = p
}

// SetLastHeartbeat 设置最后心跳时间获取函数（由 Server 在启动后注入）
func (h *Handler) SetLastHeartbeat(getter func() int64) {
	h.lastHeartbeatGetter = getter
}

// classifySessionKeyForList 与 OpenClaw GatewaySessionRow.kind 一致：direct | group | global | unknown
func classifySessionKeyForList(key string) string {
	if key == "global" {
		return "global"
	}
	if key == "unknown" {
		return "unknown"
	}
	if session.IsGroupSessionKey(key) {
		return "group"
	}
	return "direct"
}

// resolveGatewaySessionKey 将前端传入的 key（如 "main"）解析为规范 sessionKey，与 OpenClaw 一致
func resolveGatewaySessionKey(key string) string {
	k := strings.TrimSpace(key)
	if k == "" {
		return ""
	}
	if k == "global" || k == "unknown" {
		return k
	}
	cfg := config.Get()
	if cfg == nil {
		return k
	}
	mainKey := strings.TrimSpace(cfg.Session.MainKey)
	if mainKey == "" {
		mainKey = "main"
	}
	scope := strings.TrimSpace(cfg.Session.Scope)
	if scope == "" {
		scope = "per-sender"
	}
	if k == "main" || k == mainKey {
		if scope == "global" {
			return "global"
		}
		defaultAgentId := "main"
		if len(cfg.Agents.List) > 0 {
			if id := strings.TrimSpace(cfg.Agents.List[0].ID); id != "" {
				defaultAgentId = id
			} else if name := strings.TrimSpace(cfg.Agents.List[0].Name); name != "" {
				defaultAgentId = name
			}
		}
		return session.BuildAgentMainSessionKey(defaultAgentId, mainKey)
	}
	if session.IsAgentSessionKey(k) {
		return k
	}
	return k
}

// getSession 获取或创建会话；key 会先解析为规范 sessionKey（如 "main" -> agent:main:main）；若已设置 sessionPolicy 则按策略判定是否重置
func (h *Handler) getSession(key string) (*session.Session, error) {
	canonical := resolveGatewaySessionKey(key)
	if canonical == "" {
		return nil, fmt.Errorf("session key is required")
	}
	if h.sessionPolicy != nil {
		return h.sessionMgr.GetOrCreateWithPolicy(canonical, h.sessionPolicy)
	}
	return h.sessionMgr.GetOrCreate(canonical)
}

// buildConnectSnapshot 构造 connect 返回的 snapshot，与 OpenClaw 对齐：含 sessionDefaults，
// 且按 OpenClaw 规则「一个 agent 一个主会话」：scope=global 时为 "global"，否则为 agent:<agentId>:<mainKey>。
func buildConnectSnapshot() map[string]interface{} {
	snapshot := make(map[string]interface{})
	cfg := config.Get()
	if cfg == nil {
		return snapshot
	}
	mainKey := strings.TrimSpace(cfg.Session.MainKey)
	if mainKey == "" {
		mainKey = "main"
	}
	scope := strings.TrimSpace(cfg.Session.Scope)
	if scope == "" {
		scope = "per-sender"
	}
	defaultAgentId := "main"
	if len(cfg.Agents.List) > 0 {
		id := strings.TrimSpace(cfg.Agents.List[0].ID)
		name := strings.TrimSpace(cfg.Agents.List[0].Name)
		if id != "" {
			defaultAgentId = id
		} else if name != "" {
			defaultAgentId = name
		}
	}
	// 与 OpenClaw 一致：global 时共用一个会话 "global"；per-sender 时每个 agent 一个主会话 agent:<id>:<mainKey>
	mainSessionKey := mainKey
	if scope == "global" {
		mainSessionKey = "global"
	} else {
		mainSessionKey = "agent:" + defaultAgentId + ":" + mainKey
	}
	snapshot["sessionDefaults"] = map[string]interface{}{
		"mainKey":         mainKey,
		"mainSessionKey":  mainSessionKey,
		"defaultAgentId": defaultAgentId,
		"scope":           scope,
	}
	return snapshot
}

// NewHandler 创建处理器
func NewHandler(messageBus *bus.MessageBus, sessionMgr *session.Manager, channelMgr *channels.Manager) *Handler {
	h := &Handler{
		registry:           NewMethodRegistry(),
		bus:                messageBus,
		sessionMgr:         sessionMgr,
		channelMgr:         channelMgr,
		cronStore:          newCronStore(""),
		devicesStore:       newDevicesStore(""),
		execApprovalsStore: newExecApprovalsStore(""),
		skillsStore:        newSkillsStore(""),
	}

	// 注册系统方法
	h.registerSystemMethods()

	// 注册 Agent 方法
	h.registerAgentMethods()

	// 注册 Channel 方法
	h.registerChannelMethods()

	// 注册 Browser 方法
	h.registerBrowserMethods()

	return h
}

// HandleRequest 处理请求。sessionID 为 WebSocket 连接 ID（与聊天 sessionKey 无关），仅用于日志与追踪。
func (h *Handler) HandleRequest(sessionID string, req *JSONRPCRequest) *JSONRPCResponse {
	result, err := h.registry.Call(req.Method, sessionID, req.Params)
	if err != nil {
		logger.Error("Method execution failed",
			zap.String("method", req.Method),
			zap.String("connection_id", sessionID),
			zap.Error(err))
		code := ErrorInternalError
		if strings.Contains(err.Error(), "method not found") {
			code = ErrorMethodNotFound
		}
		return NewErrorResponse(req.ID, code, err.Error())
	}

	return NewSuccessResponse(req.ID, result)
}

// registerSystemMethods 注册系统方法
func (h *Handler) registerSystemMethods() {
	// connect - 前端握手，返回 hello-ok（protocol、features、snapshot）
	// 与 OpenClaw 对齐：snapshot 含 sessionDefaults，前端据此复用同一主会话（main/mainSessionKey），避免每次建新会话
	h.registry.Register("connect", func(sessionID string, params map[string]interface{}) (interface{}, error) {
		// 已实现的 method 列表，供前端 features.methods 能力检测
		methods := []string{
			"connect", "config.get", "config.set", "config.schema", "config.apply", "update.run",
			"health", "status", "last-heartbeat", "models.list",
			"sessions.list", "sessions.patch", "sessions.delete", "sessions.get", "sessions.clear",
			"sessions.usage", "sessions.usage.timeseries", "sessions.usage.logs", "usage.cost",
			"chat.send", "chat.history", "chat.abort",
			"channels.status", "channels.list", "channels.logout",
			"web.login.start", "web.login.wait",
			"agents.list", "agent.identity.get", "skills.status", "skills.update", "skills.install",
			"agents.files.list", "agents.files.get", "agents.files.set",
			"logs.get", "logs.tail",
			"cron.list", "cron.status", "cron.add", "cron.update", "cron.run", "cron.remove",
			"system-presence",
			"device.pair.list", "device.pair.approve", "device.pair.reject", "device.token.revoke", "device.token.rotate",
			"node.list",
			"exec.approvals.get", "exec.approvals.set", "exec.approvals.node.get", "exec.approvals.node.set", "exec.approval.resolve",
			"agent", "agent.wait", "send", "browser.request",
		}
		snapshot := buildConnectSnapshot()
		hello := map[string]interface{}{
			"type":     "hello-ok",
			"protocol": 3,
			"features": map[string]interface{}{
				"methods": methods,
				"events":  []string{},
			},
			"snapshot": snapshot,
		}
		// 可选：若前端传了 auth 且验证通过，可返回 auth.deviceToken 等；当前不签发
		return hello, nil
	})

	// config.get - 获取配置
	h.registry.Register("config.get", func(sessionID string, params map[string]interface{}) (interface{}, error) {
		// 当前仅支持返回完整配置（原始 JSON 字符串）
		// 兼容旧参数：如果 key 存在且不是 "raw"，先忽略，后续可以扩展为按 key 读取
		if key, ok := params["key"].(string); ok && key != "" && key != "raw" {
			logger.Warn("config.get: key parameter is currently ignored, returning full config",
				zap.String("key", key))
		}

		// 优先使用内存中的配置
		cfg := config.Get()
		if cfg == nil {
			// 如果还没有加载过配置，则从默认路径加载一次
			path, err := config.GetDefaultConfigPath()
			if err != nil {
				return nil, fmt.Errorf("failed to get default config path: %w", err)
			}
			loaded, err := config.Load(path)
			if err != nil {
				return nil, fmt.Errorf("failed to load config: %w", err)
			}
			cfg = loaded
		}

		raw, err := json.MarshalIndent(cfg, "", "  ")
		if err != nil {
			return nil, fmt.Errorf("failed to marshal config: %w", err)
		}

		path, err := config.GetDefaultConfigPath()
		if err != nil {
			return nil, fmt.Errorf("failed to get default config path: %w", err)
		}

		hashBytes := sha256.Sum256(raw)
		hash := hex.EncodeToString(hashBytes[:])
		_, statErr := os.Stat(path)
		exists := statErr == nil
		valid := true
		var issues []map[string]interface{}
		if err := config.Validate(cfg); err != nil {
			valid = false
			issues = append(issues, map[string]interface{}{"path": "config", "message": err.Error()})
		}
		var configMap map[string]interface{}
		_ = json.Unmarshal(raw, &configMap)

		return map[string]interface{}{
			"path":   path,
			"raw":    string(raw),
			"hash":   hash,
			"exists": exists,
			"valid":  valid,
			"config": configMap,
			"issues": issues,
		}, nil
	})

	// config.set - 设置配置
	h.registry.Register("config.set", func(sessionID string, params map[string]interface{}) (interface{}, error) {
		raw, ok := params["raw"].(string)
		if !ok || raw == "" {
			return nil, fmt.Errorf("raw parameter (JSON string) is required")
		}

		path, err := config.GetDefaultConfigPath()
		if err != nil {
			return nil, fmt.Errorf("failed to get default config path: %w", err)
		}

		if baseHash, _ := params["baseHash"].(string); baseHash != "" {
			cur := config.Get()
			var curRaw []byte
			if cur != nil {
				curRaw, _ = json.MarshalIndent(cur, "", "  ")
			} else {
				data, err := os.ReadFile(path)
				if err == nil {
					curRaw = data
				}
			}
			if len(curRaw) > 0 {
				sum := sha256.Sum256(curRaw)
				currentHash := hex.EncodeToString(sum[:])
				if currentHash != baseHash {
					return nil, fmt.Errorf("config changed (baseHash mismatch); reload and retry")
				}
			}
		}

		var cfg config.Config
		if err := json.Unmarshal([]byte(raw), &cfg); err != nil {
			return nil, fmt.Errorf("invalid config JSON: %w", err)
		}

		if err := config.Validate(&cfg); err != nil {
			return nil, fmt.Errorf("config validation failed: %w", err)
		}

		if err := config.Save(&cfg, path); err != nil {
			return nil, fmt.Errorf("failed to save config: %w", err)
		}

		if _, err := config.Load(path); err != nil {
			return nil, fmt.Errorf("failed to reload config: %w", err)
		}

		return map[string]interface{}{
			"path": path,
			"ok":   true,
		}, nil
	})

	// config.schema - 返回 JSON Schema + uiHints
	h.registry.Register("config.schema", func(sessionID string, params map[string]interface{}) (interface{}, error) {
		schema, uiHints := configSchemaAndHints()
		return map[string]interface{}{
			"schema":      schema,
			"uiHints":     uiHints,
			"version":     "1.0",
			"generatedAt": time.Now().UTC().Format(time.RFC3339),
		}, nil
	})

	// config.apply - 应用配置并重载（与 config.set 类似，可选 sessionKey 提示）
	h.registry.Register("config.apply", func(sessionID string, params map[string]interface{}) (interface{}, error) {
		raw, ok := params["raw"].(string)
		if !ok || raw == "" {
			return nil, fmt.Errorf("raw parameter (JSON string) is required")
		}
		path, err := config.GetDefaultConfigPath()
		if err != nil {
			return nil, fmt.Errorf("failed to get default config path: %w", err)
		}
		if baseHash, _ := params["baseHash"].(string); baseHash != "" {
			cur := config.Get()
			var curRaw []byte
			if cur != nil {
				curRaw, _ = json.MarshalIndent(cur, "", "  ")
			} else {
				data, err := os.ReadFile(path)
				if err == nil {
					curRaw = data
				}
			}
			if len(curRaw) > 0 {
				sum := sha256.Sum256(curRaw)
				if hex.EncodeToString(sum[:]) != baseHash {
					return nil, fmt.Errorf("config changed (baseHash mismatch); reload and retry")
				}
			}
		}
		var cfg config.Config
		if err := json.Unmarshal([]byte(raw), &cfg); err != nil {
			return nil, fmt.Errorf("invalid config JSON: %w", err)
		}
		if err := config.Validate(&cfg); err != nil {
			return nil, fmt.Errorf("config validation failed: %w", err)
		}
		if err := config.Save(&cfg, path); err != nil {
			return nil, fmt.Errorf("failed to save config: %w", err)
		}
		if _, err := config.Load(path); err != nil {
			return nil, fmt.Errorf("failed to reload config: %w", err)
		}
		return map[string]interface{}{"path": path, "ok": true}, nil
	})

	// update.run - 执行更新/重载（当前与 config 重载一致）
	h.registry.Register("update.run", func(sessionID string, params map[string]interface{}) (interface{}, error) {
		path, err := config.GetDefaultConfigPath()
		if err != nil {
			return nil, fmt.Errorf("failed to get default config path: %w", err)
		}
		if _, err := config.Load(path); err != nil {
			return nil, fmt.Errorf("failed to reload config: %w", err)
		}
		return map[string]interface{}{"ok": true}, nil
	})

	// sessions.usage - 从会话列表汇总用量（消息数、估算 token）
	h.registry.Register("sessions.usage", func(sessionID string, params map[string]interface{}) (interface{}, error) {
		keys, err := h.sessionMgr.List()
		if err != nil {
			return map[string]interface{}{"sessions": []interface{}{}, "ts": time.Now().UnixMilli()}, nil
		}
		startDate, _ := params["startDate"].(string)
		endDate, _ := params["endDate"].(string)
		limit := 100
		if l, ok := params["limit"].(float64); ok && l > 0 {
			limit = int(l)
		}
		sessions := make([]map[string]interface{}, 0, len(keys))
		for _, key := range keys {
			sess, err := h.getSession(key)
			if err != nil {
				continue
			}
			msgCount := len(sess.Messages)
			estTokens := msgCount * 100 // 粗略估算
			row := map[string]interface{}{
				"key":         key,
				"messageCount": msgCount,
				"estimatedTokens": estTokens,
				"updatedAtMs": sess.UpdatedAt.UnixMilli(),
			}
			if startDate != "" || endDate != "" {
				// 简单按日期过滤：用 updatedAt 日期
				row["updatedAt"] = sess.UpdatedAt.Format("2006-01-02")
			}
			sessions = append(sessions, row)
		}
		sort.Slice(sessions, func(i, j int) bool {
			a, _ := sessions[i]["updatedAtMs"].(int64)
			b, _ := sessions[j]["updatedAtMs"].(int64)
			return a > b
		})
		if len(sessions) > limit {
			sessions = sessions[:limit]
		}
		return map[string]interface{}{"sessions": sessions, "ts": time.Now().UnixMilli()}, nil
	})
	h.registry.Register("sessions.usage.timeseries", func(sessionID string, params map[string]interface{}) (interface{}, error) {
		key, _ := params["key"].(string)
		return map[string]interface{}{"key": key, "points": []interface{}{}}, nil
	})
	h.registry.Register("sessions.usage.logs", func(sessionID string, params map[string]interface{}) (interface{}, error) {
		key, _ := params["key"].(string)
		return map[string]interface{}{"key": key, "logs": []interface{}{}}, nil
	})
	h.registry.Register("usage.cost", func(sessionID string, params map[string]interface{}) (interface{}, error) {
		return map[string]interface{}{"total": 0, "byModel": map[string]interface{}{}, "byProvider": map[string]interface{}{}}, nil
	})

	// cron - 使用文件存储
	h.registry.Register("cron.list", func(sessionID string, params map[string]interface{}) (interface{}, error) {
		jobs, err := h.cronStore.Load()
		if err != nil {
			return map[string]interface{}{"jobs": []interface{}{}}, nil
		}
		out := make([]interface{}, 0, len(jobs))
		for _, j := range jobs {
			out = append(out, map[string]interface{}{
				"id": j.ID, "schedule": j.Schedule, "sessionKey": j.SessionKey,
				"enabled": j.Enabled, "label": j.Label, "createdAt": j.CreatedAt,
			})
		}
		return map[string]interface{}{"jobs": out}, nil
	})
	h.registry.Register("cron.status", func(sessionID string, params map[string]interface{}) (interface{}, error) {
		jobs, err := h.cronStore.Load()
		if err != nil {
			return map[string]interface{}{"jobs": []interface{}{}, "nextWakeAtMs": int64(0)}, nil
		}
		out := make([]interface{}, 0, len(jobs))
		for _, j := range jobs {
			out = append(out, map[string]interface{}{
				"id": j.ID, "schedule": j.Schedule, "sessionKey": j.SessionKey,
				"enabled": j.Enabled, "label": j.Label, "createdAt": j.CreatedAt,
			})
		}
		return map[string]interface{}{"jobs": out, "nextWakeAtMs": int64(0)}, nil
	})
	h.registry.Register("cron.add", func(sessionID string, params map[string]interface{}) (interface{}, error) {
		job := CronJob{
			Schedule:   getString(params, "schedule"),
			SessionKey: getString(params, "sessionKey"),
			Enabled:    getBool(params, "enabled", true),
			Label:      getString(params, "label"),
		}
		added, err := h.cronStore.Add(job)
		if err != nil {
			return nil, err
		}
		return map[string]interface{}{"ok": true, "id": added.ID}, nil
	})
	h.registry.Register("cron.update", func(sessionID string, params map[string]interface{}) (interface{}, error) {
		id, _ := params["id"].(string)
		if id == "" {
			return nil, fmt.Errorf("id is required")
		}
		patch, _ := params["patch"].(map[string]interface{})
		if patch == nil {
			patch = map[string]interface{}{}
		}
		if err := h.cronStore.Update(id, patch); err != nil {
			return nil, err
		}
		return map[string]interface{}{"ok": true}, nil
	})
	h.registry.Register("cron.run", func(sessionID string, params map[string]interface{}) (interface{}, error) {
		_, _ = params["id"].(string)
		return map[string]interface{}{"ok": true}, nil
	})
	h.registry.Register("cron.remove", func(sessionID string, params map[string]interface{}) (interface{}, error) {
		id, _ := params["id"].(string)
		if id == "" {
			return nil, fmt.Errorf("id is required")
		}
		if err := h.cronStore.Remove(id); err != nil {
			return nil, err
		}
		return map[string]interface{}{"ok": true}, nil
	})

	// system-presence - 从 Server 获取当前连接列表
	h.registry.Register("system-presence", func(sessionID string, params map[string]interface{}) (interface{}, error) {
		entries := []interface{}{}
		if h.presenceProvider != nil {
			for _, e := range h.presenceProvider.GetPresenceEntries() {
				entries = append(entries, e)
			}
		}
		return map[string]interface{}{"entries": entries}, nil
	})

	// device - 使用文件存储
	h.registry.Register("device.pair.list", func(sessionID string, params map[string]interface{}) (interface{}, error) {
		f, err := h.devicesStore.Load()
		if err != nil {
			return map[string]interface{}{"pending": []interface{}{}, "paired": []interface{}{}}, nil
		}
		pending := make([]interface{}, 0, len(f.Pending))
		for _, p := range f.Pending {
			pending = append(pending, map[string]interface{}{"requestId": p.RequestID, "createdAt": p.CreatedAt})
		}
		paired := make([]interface{}, 0, len(f.Paired))
		for _, p := range f.Paired {
			paired = append(paired, map[string]interface{}{"deviceId": p.DeviceID, "role": p.Role, "createdAt": p.CreatedAt})
		}
		return map[string]interface{}{"pending": pending, "paired": paired}, nil
	})
	h.registry.Register("device.pair.approve", func(sessionID string, params map[string]interface{}) (interface{}, error) {
		requestID, _ := params["requestId"].(string)
		if requestID == "" {
			return nil, fmt.Errorf("requestId is required")
		}
		deviceID := requestID
		role := "control"
		token, err := h.devicesStore.Approve(requestID, deviceID, role)
		if err != nil {
			return nil, err
		}
		return map[string]interface{}{"ok": true, "token": token}, nil
	})
	h.registry.Register("device.pair.reject", func(sessionID string, params map[string]interface{}) (interface{}, error) {
		requestID, _ := params["requestId"].(string)
		if requestID == "" {
			return nil, fmt.Errorf("requestId is required")
		}
		if err := h.devicesStore.Reject(requestID); err != nil {
			return nil, err
		}
		return map[string]interface{}{"ok": true}, nil
	})
	h.registry.Register("device.token.revoke", func(sessionID string, params map[string]interface{}) (interface{}, error) {
		deviceID, _ := params["deviceId"].(string)
		role, _ := params["role"].(string)
		if deviceID == "" || role == "" {
			return nil, fmt.Errorf("deviceId and role are required")
		}
		if err := h.devicesStore.Revoke(deviceID, role); err != nil {
			return nil, err
		}
		return map[string]interface{}{"ok": true}, nil
	})
	h.registry.Register("device.token.rotate", func(sessionID string, params map[string]interface{}) (interface{}, error) {
		deviceID, _ := params["deviceId"].(string)
		role, _ := params["role"].(string)
		if role == "" {
			role = "control"
		}
		token, err := h.devicesStore.Rotate(deviceID, role)
		if err != nil {
			return nil, err
		}
		return map[string]interface{}{
			"token":    token,
			"role":     role,
			"deviceId": deviceID,
			"scopes":   []interface{}{},
		}, nil
	})

	// exec.approvals - 使用文件存储
	h.registry.Register("exec.approvals.get", func(sessionID string, params map[string]interface{}) (interface{}, error) {
		f, err := h.execApprovalsStore.Load()
		if err != nil {
			return map[string]interface{}{"path": defaultExecApprovalsPath(), "exists": false, "hash": "", "file": map[string]interface{}{}}, nil
		}
		return map[string]interface{}{"path": f.Path, "exists": f.Exists, "hash": f.Hash, "file": f.File}, nil
	})
	h.registry.Register("exec.approvals.set", func(sessionID string, params map[string]interface{}) (interface{}, error) {
		hash, _ := params["hash"].(string)
		file, _ := params["file"].(map[string]interface{})
		if file == nil {
			file = map[string]interface{}{}
		}
		path := defaultExecApprovalsPath()
		if err := h.execApprovalsStore.Save(ExecApprovalsFile{Path: path, Exists: true, Hash: hash, File: file}); err != nil {
			return nil, err
		}
		return map[string]interface{}{"ok": true}, nil
	})
	h.registry.Register("exec.approvals.node.get", func(sessionID string, params map[string]interface{}) (interface{}, error) {
		nodeId, _ := params["nodeId"].(string)
		_ = nodeId
		f, err := h.execApprovalsStore.Load()
		if err != nil {
			return map[string]interface{}{"path": "", "exists": false, "hash": "", "file": map[string]interface{}{}}, nil
		}
		return map[string]interface{}{"path": f.Path, "exists": f.Exists, "hash": f.Hash, "file": f.File}, nil
	})
	h.registry.Register("exec.approvals.node.set", func(sessionID string, params map[string]interface{}) (interface{}, error) {
		hash, _ := params["hash"].(string)
		file, _ := params["file"].(map[string]interface{})
		if file == nil {
			file = map[string]interface{}{}
		}
		path := defaultExecApprovalsPath()
		if err := h.execApprovalsStore.Save(ExecApprovalsFile{Path: path, Exists: true, Hash: hash, File: file}); err != nil {
			return nil, err
		}
		return map[string]interface{}{"ok": true}, nil
	})
	h.registry.Register("exec.approval.resolve", func(sessionID string, params map[string]interface{}) (interface{}, error) {
		return map[string]interface{}{"ok": true}, nil
	})

	// node.list - 返回本网关节点
	h.registry.Register("node.list", func(sessionID string, params map[string]interface{}) (interface{}, error) {
		nodes := map[string]interface{}{
			"local": map[string]interface{}{"id": "local", "capabilities": []interface{}{}},
		}
		return map[string]interface{}{"nodes": nodes}, nil
	})

	// health - 健康检查
	h.registry.Register("health", func(sessionID string, params map[string]interface{}) (interface{}, error) {
		return map[string]interface{}{
			"status":    "ok",
			"timestamp": time.Now().Unix(),
			"version":   ProtocolVersion,
		}, nil
	})

	// status - Debug 用，与 health 类似或更详细
	h.registry.Register("status", func(sessionID string, params map[string]interface{}) (interface{}, error) {
		return map[string]interface{}{
			"status":    "ok",
			"timestamp": time.Now().Unix(),
			"version":   ProtocolVersion,
		}, nil
	})

	// last-heartbeat - 最后心跳时间（由 Server 更新）
	h.registry.Register("last-heartbeat", func(sessionID string, params map[string]interface{}) (interface{}, error) {
		ts := time.Now().UnixMilli()
		if h.lastHeartbeatGetter != nil {
			ts = h.lastHeartbeatGetter()
		}
		return map[string]interface{}{"ts": ts}, nil
	})

	// models.list - 从 config agents 与 ~/.goclaw/agents 收集 model 列表
	h.registry.Register("models.list", func(sessionID string, params map[string]interface{}) (interface{}, error) {
		models := make([]string, 0)
		seen := make(map[string]bool)
		if cfg := config.Get(); cfg != nil {
			if cfg.Agents.Defaults.Model != "" && !seen[cfg.Agents.Defaults.Model] {
				models = append(models, cfg.Agents.Defaults.Model)
				seen[cfg.Agents.Defaults.Model] = true
			}
			for _, a := range cfg.Agents.List {
				if a.Model != "" && !seen[a.Model] {
					models = append(models, a.Model)
					seen[a.Model] = true
				}
			}
		}
		homeDir, _ := os.UserHomeDir()
		if homeDir != "" {
			agentsDir := filepath.Join(homeDir, ".goclaw", "agents")
			entries, _ := os.ReadDir(agentsDir)
			for _, e := range entries {
				if e.IsDir() || filepath.Ext(e.Name()) != ".json" {
					continue
				}
				data, err := os.ReadFile(filepath.Join(agentsDir, e.Name()))
				if err != nil {
					continue
				}
				var agent map[string]interface{}
				if json.Unmarshal(data, &agent) != nil {
					continue
				}
				if m, _ := agent["model"].(string); m != "" && !seen[m] {
					models = append(models, m)
					seen[m] = true
				}
			}
		}
		return map[string]interface{}{"models": models}, nil
	})

	// logs - 获取日志
	h.registry.Register("logs.get", func(sessionID string, params map[string]interface{}) (interface{}, error) {
		lines := 100
		if l, ok := params["lines"].(float64); ok {
			lines = int(l)
		}

		if lines <= 0 {
			lines = 100
		}

		logPath := detectLogPath()
		if logPath == "" {
			logger.Warn("logs.get: no log file detected")
			return map[string]interface{}{
				"lines": lines,
				"path":  "",
				"logs":  []string{},
			}, nil
		}

		file, err := os.Open(logPath)
		if err != nil {
			if os.IsNotExist(err) {
				logger.Warn("logs.get: log file not found", zap.String("path", logPath))
				return map[string]interface{}{
					"lines": lines,
					"path":  logPath,
					"logs":  []string{},
				}, nil
			}
			return nil, fmt.Errorf("failed to open log file: %w", err)
		}
		defer file.Close()

		scanner := bufio.NewScanner(file)
		allLines := make([]string, 0)
		for scanner.Scan() {
			allLines = append(allLines, scanner.Text())
		}
		if err := scanner.Err(); err != nil {
			return nil, fmt.Errorf("error reading log file: %w", err)
		}

		start := 0
		if len(allLines) > lines {
			start = len(allLines) - lines
		}
		resultLines := allLines[start:]

		return map[string]interface{}{
			"lines": lines,
			"path":  logPath,
			"logs":  resultLines,
		}, nil
	})

	// logs.tail - 与前端约定：cursor 为上次返回的 cursor（字节偏移），limit/maxBytes 限制条数与字节
	h.registry.Register("logs.tail", func(sessionID string, params map[string]interface{}) (interface{}, error) {
		limit := 200
		if l, ok := params["limit"].(float64); ok && l > 0 {
			limit = int(l)
		}
		maxBytes := 256 * 1024
		if b, ok := params["maxBytes"].(float64); ok && b > 0 {
			maxBytes = int(b)
		}
		var cursor int64
		if c, ok := params["cursor"].(float64); ok && c >= 0 {
			cursor = int64(c)
		}

		logPath := detectLogPath()
		if logPath == "" {
			return map[string]interface{}{
				"file": logPath, "cursor": 0, "lines": []string{}, "truncated": false, "reset": true,
			}, nil
		}

		file, err := os.Open(logPath)
		if err != nil {
			if os.IsNotExist(err) {
				return map[string]interface{}{
					"file": logPath, "cursor": 0, "lines": []string{}, "truncated": false, "reset": true,
				}, nil
			}
			return nil, fmt.Errorf("failed to open log file: %w", err)
		}
		defer file.Close()

		info, err := file.Stat()
		if err != nil {
			return nil, fmt.Errorf("failed to stat log file: %w", err)
		}
		size := info.Size()
		if cursor > size {
			cursor = size
		}

		// 从 cursor 位置往后读，或从文件末尾往前取 limit 行
		if cursor == 0 {
			// 从末尾取 limit 行
			if size == 0 {
				return map[string]interface{}{
					"file": logPath, "cursor": 0, "lines": []string{}, "truncated": false, "reset": true,
				}, nil
			}
			start := size - int64(maxBytes)
			if start < 0 {
				start = 0
			}
			_, _ = file.Seek(start, 0)
			scanner := bufio.NewScanner(file)
			scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
			var tail []string
			for scanner.Scan() {
				tail = append(tail, scanner.Text())
				if len(tail) > limit {
					tail = tail[1:]
				}
			}
			return map[string]interface{}{
				"file": logPath, "cursor": size, "lines": tail, "truncated": start > 0, "reset": true,
			}, nil
		}

		_, _ = file.Seek(cursor, 0)
		scanner := bufio.NewScanner(file)
		scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
		var lines []string
		readBytes := int64(0)
		for scanner.Scan() && len(lines) < limit && readBytes < int64(maxBytes) {
			line := scanner.Text()
			lines = append(lines, line)
			readBytes += int64(len(line)) + 1
		}
		newCursor := cursor + readBytes
		if newCursor > size {
			newCursor = size
		}
		truncated := scanner.Scan() || readBytes >= int64(maxBytes)
		return map[string]interface{}{
			"file": logPath, "cursor": newCursor, "lines": lines, "truncated": truncated, "reset": false,
		}, nil
	})

	// agents.list - 列出已配置的 Agents（AgentsListResult: defaultId, mainKey, scope, agents）
	h.registry.Register("agents.list", func(sessionID string, params map[string]interface{}) (interface{}, error) {
		homeDir, err := os.UserHomeDir()
		if err != nil {
			return nil, fmt.Errorf("failed to get home directory: %w", err)
		}

		agentsDir := filepath.Join(homeDir, ".goclaw", "agents")
		if err := os.MkdirAll(agentsDir, 0755); err != nil {
			return nil, fmt.Errorf("failed to create agents directory: %w", err)
		}

		entries, err := os.ReadDir(agentsDir)
		if err != nil {
			return nil, fmt.Errorf("failed to read agents directory: %w", err)
		}

		defaultId := ""
		mainKey := ""
		scope := "per-sender"
		if cfg := config.Get(); cfg != nil {
			scope = cfg.Session.Scope
			mainKey = cfg.Session.MainKey
		}

		agents := make([]map[string]interface{}, 0)
		for _, entry := range entries {
			if entry.IsDir() {
				continue
			}
			if filepath.Ext(entry.Name()) != ".json" {
				continue
			}

			agentPath := filepath.Join(agentsDir, entry.Name())
			data, err := os.ReadFile(agentPath)
			if err != nil {
				continue
			}

			var agent map[string]interface{}
			if err := json.Unmarshal(data, &agent); err != nil {
				continue
			}

			name, _ := agent["name"].(string)
			if name == "" {
				name = strings.TrimSuffix(entry.Name(), ".json")
			}
			id := name
			if idFromConfig, ok := agent["id"].(string); ok && idFromConfig != "" {
				id = idFromConfig
			}
			if defaultId == "" {
				defaultId = id
			}

			row := map[string]interface{}{"id": id, "name": name}
			if identity, ok := agent["identity"].(map[string]interface{}); ok {
				row["identity"] = identity
			}
			agents = append(agents, row)
		}

		return map[string]interface{}{
			"defaultId": defaultId,
			"mainKey":   mainKey,
			"scope":     scope,
			"agents":    agents,
		}, nil
	})

	// agent.identity.get - 按 agentId 返回名称/头像/emoji；未传 agentId 时返回默认助手身份（供 Control UI assistant 展示）
	h.registry.Register("agent.identity.get", func(sessionID string, params map[string]interface{}) (interface{}, error) {
		agentId, _ := params["agentId"].(string)
		if agentId == "" {
			// 前端 assistant-identity 可能只传 sessionKey，用默认 agent
			if cfg := config.Get(); cfg != nil && len(cfg.Agents.List) > 0 {
				agentId = cfg.Agents.List[0].ID
				if agentId == "" {
					agentId = cfg.Agents.List[0].Name
				}
			}
			if agentId == "" {
				agentId = "main"
			}
		}

		homeDir, err := os.UserHomeDir()
		if err != nil {
			return nil, fmt.Errorf("failed to get home directory: %w", err)
		}
		agentsDir := filepath.Join(homeDir, ".goclaw", "agents")
		// 尝试 id.json 与 name.json
		for _, base := range []string{agentId, strings.ReplaceAll(agentId, "/", "_")} {
			agentPath := filepath.Join(agentsDir, base+".json")
			data, err := os.ReadFile(agentPath)
			if err != nil {
				continue
			}
			var agent map[string]interface{}
			if err := json.Unmarshal(data, &agent); err != nil {
				continue
			}
			name, _ := agent["name"].(string)
			if name == "" {
				name = agentId
			}
			avatar := ""
			emoji := ""
			if identity, ok := agent["identity"].(map[string]interface{}); ok {
				emoji, _ = identity["emoji"].(string)
				avatar, _ = identity["avatar"].(string)
				if n, _ := identity["name"].(string); n != "" {
					name = n
				}
			}
			return map[string]interface{}{
				"agentId": agentId,
				"name":    name,
				"avatar":  avatar,
				"emoji":   emoji,
			}, nil
		}
		// 从 config agents.list 查找
		if cfg := config.Get(); cfg != nil {
			for _, a := range cfg.Agents.List {
				if a.ID == agentId || a.Name == agentId {
					name := a.Name
					avatar := ""
					emoji := ""
					if a.Identity != nil {
						emoji = a.Identity.Emoji
						name = a.Identity.Name
						if name == "" {
							name = a.Name
						}
					}
					if name == "" {
						name = agentId
					}
					return map[string]interface{}{
						"agentId": agentId,
						"name":    name,
						"avatar":  avatar,
						"emoji":   emoji,
					}, nil
				}
			}
		}
		return map[string]interface{}{
			"agentId": agentId,
			"name":    agentId,
			"avatar":  "",
			"emoji":   "",
		}, nil
	})

	// skills.status - 技能状态（合并目录与 overlay）
	h.registry.Register("skills.status", func(sessionID string, params map[string]interface{}) (interface{}, error) {
		overlays, _ := h.skillsStore.Load()
		homeDir, _ := os.UserHomeDir()
		skillsDir := filepath.Join(homeDir, ".goclaw", "skills")
		entries, _ := os.ReadDir(skillsDir)
		skills := make([]map[string]interface{}, 0)
		for _, e := range entries {
			if !e.IsDir() {
				continue
			}
			key := e.Name()
			enabled := true
			apiKey := ""
			if o, ok := overlays[key]; ok {
				enabled = o.Enabled
				apiKey = o.APIKey
			}
			skills = append(skills, map[string]interface{}{
				"key":     key,
				"enabled": enabled,
				"apiKey":  apiKey,
			})
		}
		return map[string]interface{}{"skills": skills}, nil
	})

	// skills.update - 更新技能 enabled 或 apiKey 并持久化
	h.registry.Register("skills.update", func(sessionID string, params map[string]interface{}) (interface{}, error) {
		skillKey, _ := params["skillKey"].(string)
		if skillKey == "" {
			return nil, fmt.Errorf("skillKey is required")
		}
		var enabled *bool
		if v, ok := params["enabled"]; ok {
			b, _ := v.(bool)
			enabled = &b
		}
		var apiKey *string
		if v, ok := params["apiKey"].(string); ok {
			apiKey = &v
		}
		if enabled == nil && apiKey == nil {
			return map[string]interface{}{"ok": true}, nil
		}
		if err := h.skillsStore.UpdateSkill(skillKey, enabled, apiKey); err != nil {
			return nil, err
		}
		return map[string]interface{}{"ok": true}, nil
	})
	// skills.install - 安装技能（与 openclaw 前端兼容）
	h.registry.Register("skills.install", func(sessionID string, params map[string]interface{}) (interface{}, error) {
		return map[string]interface{}{"message": "Installed"}, nil
	})

	// resolveWorkspace 优先用 params，否则用配置中的 workspace.path（与 agent 使用同一工作区）
	resolveWorkspace := func(params map[string]interface{}) string {
		if w, _ := params["workspace"].(string); w != "" {
			return w
		}
		if cfg := config.Get(); cfg != nil {
			if p, err := config.GetWorkspacePath(cfg); err == nil && p != "" {
				return p
			}
		}
		homeDir, _ := os.UserHomeDir()
		return filepath.Join(homeDir, ".goclaw", "workspace")
	}

	// agents.files.list - Agent 工作区文件列表
	h.registry.Register("agents.files.list", func(sessionID string, params map[string]interface{}) (interface{}, error) {
		agentId, _ := params["agentId"].(string)
		if agentId == "" {
			return nil, fmt.Errorf("agentId is required")
		}
		workspace := resolveWorkspace(params)
		entries, err := os.ReadDir(workspace)
		if err != nil {
			return map[string]interface{}{"agentId": agentId, "workspace": workspace, "files": []interface{}{}}, nil
		}
		files := make([]map[string]interface{}, 0)
		for _, e := range entries {
			info, _ := e.Info()
			size := int64(0)
			if info != nil {
				size = info.Size()
			}
			files = append(files, map[string]interface{}{
				"name": e.Name(), "path": e.Name(), "missing": false, "size": size,
			})
		}
		return map[string]interface{}{"agentId": agentId, "workspace": workspace, "files": files}, nil
	})

	// agents.files.get - 读取 Agent 工作区文件
	h.registry.Register("agents.files.get", func(sessionID string, params map[string]interface{}) (interface{}, error) {
		agentId, _ := params["agentId"].(string)
		path, _ := params["path"].(string)
		if agentId == "" || path == "" {
			return nil, fmt.Errorf("agentId and path are required")
		}
		workspace := resolveWorkspace(params)
		fullPath := filepath.Join(workspace, filepath.Clean(path))
		rel, err := filepath.Rel(workspace, fullPath)
		if err != nil || strings.HasPrefix(rel, "..") {
			return nil, fmt.Errorf("path outside workspace")
		}
		data, err := os.ReadFile(fullPath)
		if err != nil {
			return map[string]interface{}{"agentId": agentId, "workspace": workspace, "file": map[string]interface{}{"name": path, "path": path, "missing": true}}, nil
		}
		return map[string]interface{}{
			"agentId": agentId, "workspace": workspace,
			"file": map[string]interface{}{"name": filepath.Base(path), "path": path, "missing": false, "content": string(data)},
		}, nil
	})

	// agents.files.set - 写入 Agent 工作区文件
	h.registry.Register("agents.files.set", func(sessionID string, params map[string]interface{}) (interface{}, error) {
		agentId, _ := params["agentId"].(string)
		path, _ := params["path"].(string)
		content, _ := params["content"].(string)
		if agentId == "" || path == "" {
			return nil, fmt.Errorf("agentId and path are required")
		}
		workspace := resolveWorkspace(params)
		fullPath := filepath.Join(workspace, filepath.Clean(path))
		rel, err := filepath.Rel(workspace, fullPath)
		if err != nil || strings.HasPrefix(rel, "..") {
			return nil, fmt.Errorf("path outside workspace")
		}
		if err := os.MkdirAll(filepath.Dir(fullPath), 0755); err != nil {
			return nil, fmt.Errorf("failed to create directory: %w", err)
		}
		if err := os.WriteFile(fullPath, []byte(content), 0644); err != nil {
			return nil, fmt.Errorf("failed to write file: %w", err)
		}
		info, _ := os.Stat(fullPath)
		size := int64(0)
		if info != nil {
			size = info.Size()
		}
		return map[string]interface{}{
			"ok": true, "agentId": agentId, "workspace": workspace,
			"file": map[string]interface{}{"name": filepath.Base(path), "path": path, "missing": false, "size": size},
		}, nil
	})

	// web.login.start / web.login.wait - WhatsApp 扫码登录占位
	h.registry.Register("web.login.start", func(sessionID string, params map[string]interface{}) (interface{}, error) {
		return map[string]interface{}{"message": "not implemented"}, nil
	})
	h.registry.Register("web.login.wait", func(sessionID string, params map[string]interface{}) (interface{}, error) {
		return map[string]interface{}{"message": "not implemented", "connected": false}, nil
	})
}

func getString(m map[string]interface{}, key string) string {
	if v, ok := m[key].(string); ok {
		return v
	}
	return ""
}

func getBool(m map[string]interface{}, key string, defaultVal bool) bool {
	if v, ok := m[key].(bool); ok {
		return v
	}
	return defaultVal
}

// detectLogPath 尝试自动检测日志文件路径
// 与 CLI 中的逻辑保持一致，优先常见位置，其次返回默认路径
func detectLogPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}

	candidates := []string{
		filepath.Join(home, ".goclaw", "logs", "goclaw.log"),
		filepath.Join(home, ".goclaw", "goclaw.log"),
		filepath.Join(string(filepath.Separator), "var", "log", "goclaw.log"),
		"goclaw.log",
		filepath.Join("logs", "goclaw.log"),
	}

	for _, candidate := range candidates {
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
	}

	// 默认返回 ~/.goclaw/logs/goclaw.log（即使当前不存在）
	return filepath.Join(home, ".goclaw", "logs", "goclaw.log")
}

// registerAgentMethods 注册 Agent 方法
func (h *Handler) registerAgentMethods() {
	// agent - 发送消息给 Agent
	h.registry.Register("agent", func(sessionID string, params map[string]interface{}) (interface{}, error) {
		content, ok := params["content"].(string)
		if !ok {
			return nil, fmt.Errorf("content parameter is required")
		}

		// 构造入站消息
		msg := &bus.InboundMessage{
			Channel:   "websocket",
			SenderID:  sessionID,
			ChatID:    sessionID,
			Content:   content,
			Timestamp: time.Now(),
		}

		// 发布到消息总线
		if err := h.bus.PublishInbound(context.Background(), msg); err != nil {
			return nil, fmt.Errorf("failed to publish message: %w", err)
		}

		return map[string]interface{}{
			"status": "queued",
			"msg_id": msg.ID,
		}, nil
	})

	// agent.wait - 发送消息并等待响应
	h.registry.Register("agent.wait", func(sessionID string, params map[string]interface{}) (interface{}, error) {
		content, ok := params["content"].(string)
		if !ok {
			return nil, fmt.Errorf("content parameter is required")
		}

		timeout := 30 * time.Second
		if t, ok := params["timeout"].(float64); ok {
			timeout = time.Duration(t) * time.Second
		}

		// 构造入站消息
		msg := &bus.InboundMessage{
			Channel:   "websocket",
			SenderID:  sessionID,
			ChatID:    sessionID,
			Content:   content,
			Timestamp: time.Now(),
		}

		// 发布到消息总线
		if err := h.bus.PublishInbound(context.Background(), msg); err != nil {
			return nil, fmt.Errorf("failed to publish message: %w", err)
		}

		// 等待响应（简化实现）
		// 实际应该通过监听出站消息来获取响应
		ctx, cancel := context.WithTimeout(context.Background(), timeout)
		defer cancel()

		select {
		case <-ctx.Done():
			return nil, fmt.Errorf("timeout waiting for response")
		default:
			// 返回初始响应
			return map[string]interface{}{
				"status":  "waiting",
				"msg_id":  msg.ID,
				"timeout": timeout.String(),
			}, nil
		}
	})

	// chat.history - 按 session_key 返回历史消息（与 OpenClaw 对齐，供 UI/TUI 加载会话）
	h.registry.Register("chat.history", func(sessionID string, params map[string]interface{}) (interface{}, error) {
		sessionKey, _ := params["sessionKey"].(string)
		if sessionKey == "" {
			if k, _ := params["session_key"].(string); k != "" {
				sessionKey = k
			}
		}
		if sessionKey == "" {
			return nil, fmt.Errorf("sessionKey or session_key is required")
		}
		limit := 200
		if l, ok := params["limit"].(float64); ok && l > 0 {
			limit = int(l)
		}
		if limit > 1000 {
			limit = 1000
		}
		sess, err := h.getSession(sessionKey)
		if err != nil {
			return nil, fmt.Errorf("failed to get session: %w", err)
		}
		history := sess.GetHistory(limit)
		messages := make([]map[string]interface{}, 0, len(history))
		for _, m := range history {
			// 跳过只有工具调用而没有文本内容的 assistant 消息
			if m.Role == "assistant" && strings.TrimSpace(m.Content) == "" && len(m.ToolCalls) > 0 {
				continue
			}

			msg := map[string]interface{}{
				"role": m.Role, "content": m.Content, "timestamp": m.Timestamp,
			}
			if len(m.Media) > 0 {
				msg["media"] = m.Media
			}
			if m.ToolCallID != "" {
				msg["tool_call_id"] = m.ToolCallID
			}
			if len(m.ToolCalls) > 0 {
				msg["tool_calls"] = m.ToolCalls
			}
			if len(m.Metadata) > 0 {
				msg["metadata"] = m.Metadata
			}
			messages = append(messages, msg)
		}
		out := map[string]interface{}{"sessionKey": sessionKey, "messages": messages}
		if v, ok := sess.Metadata["thinkingLevel"]; ok && v != nil {
			out["thinkingLevel"] = v
		}
		return out, nil
	})

	// chat.abort - 中止当前会话的聊天运行（与 OpenClaw 对齐）；无 run 追踪时返回 aborted: false
	h.registry.Register("chat.abort", func(sessionID string, params map[string]interface{}) (interface{}, error) {
		sessionKey, _ := params["sessionKey"].(string)
		if sessionKey == "" {
			if k, _ := params["session_key"].(string); k != "" {
				sessionKey = k
			}
		}
		if sessionKey == "" {
			return nil, fmt.Errorf("sessionKey or session_key is required")
		}
		// 当前无 run 追踪：返回与 OpenClaw 一致的响应结构，aborted 恒为 false
		return map[string]interface{}{
			"ok":      true,
			"aborted": false,
			"runIds":  []string{},
		}, nil
	})

	// sessions.list - 列出会话（与 OpenClaw 一致：key/kind/label/displayName/sessionId/updatedAt/spawnedBy、过滤 includeGlobal/includeUnknown/label/spawnedBy/agentId/activeMinutes、按 updatedAt 倒序）
	h.registry.Register("sessions.list", func(sessionID string, params map[string]interface{}) (interface{}, error) {
		keys, err := h.sessionMgr.List()
		if err != nil {
			return nil, fmt.Errorf("failed to list sessions: %w", err)
		}

		defaultModel := ""
		defaultContextTokens := int64(0)
		if cfg := config.Get(); cfg != nil {
			defaultModel = cfg.Agents.Defaults.Model
			defaultContextTokens = int64(cfg.Agents.Defaults.ContextTokens)
		}
		limit := 20
		if v, ok := params["limit"]; ok {
			switch n := v.(type) {
			case float64:
				limit = int(n)
			case int:
				limit = n
			}
		}
		if limit <= 0 {
			limit = 20
		}
		includeGlobal := false
		if v, ok := params["includeGlobal"].(bool); ok {
			includeGlobal = v
		}
		includeUnknown := false
		if v, ok := params["includeUnknown"].(bool); ok {
			includeUnknown = v
		}
		filterLabel := ""
		if v, ok := params["label"].(string); ok {
			filterLabel = strings.TrimSpace(v)
		}
		filterSpawnedBy := ""
		if v, ok := params["spawnedBy"].(string); ok {
			filterSpawnedBy = strings.TrimSpace(v)
		}
		filterAgentId := ""
		if v, ok := params["agentId"].(string); ok {
			filterAgentId = strings.TrimSpace(v)
		}
		activeMinutes := 0
		if v, ok := params["activeMinutes"]; ok {
			switch n := v.(type) {
			case float64:
				activeMinutes = int(n)
			case int:
				activeMinutes = n
			}
		}
		nowMs := time.Now().UnixMilli()
		cutoffMs := int64(0)
		if activeMinutes > 0 {
			cutoffMs = nowMs - int64(activeMinutes)*60*1000
		}

		sessions := make([]map[string]interface{}, 0, len(keys))
		for _, key := range keys {
			if !includeGlobal && key == "global" {
				continue
			}
			if !includeUnknown && key == "unknown" {
				continue
			}
			if filterAgentId != "" && key != "global" && key != "unknown" {
				agentID, _, ok := session.ParseAgentSessionKey(key)
				if !ok || !strings.EqualFold(strings.TrimSpace(agentID), filterAgentId) {
					continue
				}
			}
			sess, err := h.getSession(key)
			if err != nil {
				continue
			}
			if filterLabel != "" {
				lab, _ := sess.Metadata["label"].(string)
				if strings.TrimSpace(lab) != filterLabel {
					continue
				}
			}
			if filterSpawnedBy != "" {
				sb, _ := sess.Metadata["spawnedBy"].(string)
				if strings.TrimSpace(sb) != filterSpawnedBy {
					continue
				}
			}
			updatedAtMs := sess.UpdatedAt.UnixMilli()
			if cutoffMs > 0 && updatedAtMs < cutoffMs {
				continue
			}
			canonicalKey := canonicalSessionKeyForBroadcast(sess.Key)
			kind := classifySessionKeyForList(sess.Key)
			row := map[string]interface{}{
				"key":        canonicalKey,
				"kind":       kind,
				"sessionId":  canonicalKey,
				"updatedAt":  updatedAtMs,
			}
			if v, ok := sess.Metadata["label"]; ok && v != nil {
				row["label"] = v
			}
			if v, ok := sess.Metadata["displayName"]; ok && v != nil {
				row["displayName"] = v
			} else if v, ok := sess.Metadata["label"]; ok && v != nil {
				row["displayName"] = v
			} else {
				row["displayName"] = canonicalKey
			}
			if v, ok := sess.Metadata["spawnedBy"]; ok && v != nil {
				row["spawnedBy"] = v
			}
			if v, ok := sess.Metadata["thinkingLevel"]; ok && v != nil {
				row["thinkingLevel"] = v
			}
			if v, ok := sess.Metadata["verboseLevel"]; ok && v != nil {
				row["verboseLevel"] = v
			}
			if v, ok := sess.Metadata["reasoningLevel"]; ok && v != nil {
				row["reasoningLevel"] = v
			}
			sessions = append(sessions, row)
		}

		sort.Slice(sessions, func(i, j int) bool {
			a, _ := sessions[i]["updatedAt"].(int64)
			b, _ := sessions[j]["updatedAt"].(int64)
			return a > b
		})
		if len(sessions) > limit {
			sessions = sessions[:limit]
		}

		return map[string]interface{}{
			"ts":       time.Now().UnixMilli(),
			"path":     h.sessionMgr.Path(),
			"count":    len(sessions),
			"defaults": map[string]interface{}{"model": defaultModel, "contextTokens": defaultContextTokens},
			"sessions": sessions,
		}, nil
	})

	// sessions.patch - 按 key 更新会话元数据（与 OpenClaw 一致：label, thinkingLevel, verboseLevel, reasoningLevel, model, spawnedBy 仅子会话, deleteTranscript）
	h.registry.Register("sessions.patch", func(sessionID string, params map[string]interface{}) (interface{}, error) {
		key, ok := params["key"].(string)
		if !ok || key == "" {
			return nil, fmt.Errorf("key parameter is required")
		}
		canonicalKey := resolveGatewaySessionKey(key)
		sess, err := h.getSession(key)
		if err != nil {
			return nil, fmt.Errorf("failed to get session: %w", err)
		}
		if v, ok := params["label"]; ok && v != nil {
			newLabel := strings.TrimSpace(fmt.Sprintf("%v", v))
			allKeys, _ := h.sessionMgr.List()
			for _, k := range allKeys {
				if k == canonicalKey {
					continue
				}
				other, _ := h.sessionMgr.GetOrCreate(k)
				if other.Metadata != nil {
					if lab, _ := other.Metadata["label"].(string); strings.TrimSpace(lab) == newLabel {
						return nil, fmt.Errorf("label already in use: %s", newLabel)
					}
				}
			}
		}
		if v, ok := params["spawnedBy"]; ok {
			if v == nil {
				if existing, _ := sess.Metadata["spawnedBy"].(string); existing != "" {
					return nil, fmt.Errorf("spawnedBy cannot be cleared once set")
				}
			} else {
				sv := strings.TrimSpace(fmt.Sprintf("%v", v))
				if sv == "" {
					return nil, fmt.Errorf("invalid spawnedBy: empty")
				}
				if !session.IsSubagentSessionKey(canonicalKey) {
					return nil, fmt.Errorf("spawnedBy is only supported for subagent:* sessions")
				}
				if existing, _ := sess.Metadata["spawnedBy"].(string); existing != "" && existing != sv {
					return nil, fmt.Errorf("spawnedBy cannot be changed once set")
				}
			}
		}
		updates := make(map[string]interface{})
		if v, ok := params["label"]; ok {
			updates["label"] = v
		}
		if v, ok := params["thinkingLevel"]; ok {
			updates["thinkingLevel"] = v
		}
		if v, ok := params["verboseLevel"]; ok {
			updates["verboseLevel"] = v
		}
		if v, ok := params["reasoningLevel"]; ok {
			updates["reasoningLevel"] = v
		}
		if v, ok := params["model"]; ok {
			updates["modelOverride"] = v
		}
		if v, ok := params["spawnedBy"]; ok {
			updates["spawnedBy"] = v
		}
		sess.PatchMetadata(updates)
		if v, ok := params["deleteTranscript"].(bool); ok && v {
			sess.Clear()
		}
		if err := h.sessionMgr.Save(sess); err != nil {
			return nil, fmt.Errorf("failed to save session: %w", err)
		}
		entry := map[string]interface{}{
			"sessionId": canonicalKey,
			"updatedAt": sess.UpdatedAt.UnixMilli(),
		}
		for _, k := range []string{"label", "thinkingLevel", "verboseLevel", "reasoningLevel", "modelOverride", "spawnedBy"} {
			if v, ok := sess.Metadata[k]; ok && v != nil {
				entry[k] = v
			}
		}
		return map[string]interface{}{
			"ok": true, "path": h.sessionMgr.Path(), "key": canonicalKey,
			"entry": entry,
		}, nil
	})

	// sessions.delete - 按 key 删除会话（含 transcript）；key 支持 "main" 等别名，会解析为规范 key
	h.registry.Register("sessions.delete", func(sessionID string, params map[string]interface{}) (interface{}, error) {
		key, ok := params["key"].(string)
		if !ok || key == "" {
			return nil, fmt.Errorf("key parameter is required")
		}
		canonicalKey := resolveGatewaySessionKey(key)
		if err := h.sessionMgr.Delete(canonicalKey); err != nil {
			return nil, fmt.Errorf("failed to delete session: %w", err)
		}
		return map[string]interface{}{"ok": true, "key": canonicalKey}, nil
	})

	// sessions.get - 获取会话详情（与 OpenClaw 一致：key/sessionId、messages、entry 元数据）
	h.registry.Register("sessions.get", func(sessionID string, params map[string]interface{}) (interface{}, error) {
		key, ok := params["key"].(string)
		if !ok {
			return nil, fmt.Errorf("key parameter is required")
		}

		sess, err := h.getSession(key)
		if err != nil {
			return nil, fmt.Errorf("failed to get session: %w", err)
		}

		entry := map[string]interface{}{
			"sessionId": sess.Key,
			"updatedAt": sess.UpdatedAt.UnixMilli(),
		}
		if sess.Metadata != nil {
			for k, v := range sess.Metadata {
				if v != nil {
					entry[k] = v
				}
			}
		}
		return map[string]interface{}{
			"key":        sess.Key,
			"sessionId":  sess.Key,
			"messages":   sess.Messages,
			"created_at": sess.CreatedAt,
			"updated_at": sess.UpdatedAt,
			"metadata":   sess.Metadata,
			"entry":      entry,
		}, nil
	})

	// sessions.resolve - 由 key / sessionId / label 解析为规范 sessionKey（与 OpenClaw 一致）
	h.registry.Register("sessions.resolve", func(sessionID string, params map[string]interface{}) (interface{}, error) {
		key, _ := params["key"].(string)
		key = strings.TrimSpace(key)
		sessionId, _ := params["sessionId"].(string)
		sessionId = strings.TrimSpace(sessionId)
		label, _ := params["label"].(string)
		label = strings.TrimSpace(label)
		hasKey := key != ""
		hasSessionId := sessionId != ""
		hasLabel := label != ""
		n := 0
		if hasKey {
			n++
		}
		if hasSessionId {
			n++
		}
		if hasLabel {
			n++
		}
		if n > 1 {
			return nil, fmt.Errorf("provide either key, sessionId, or label (not multiple)")
		}
		if n == 0 {
			return nil, fmt.Errorf("either key, sessionId, or label is required")
		}
		if hasKey {
			canonical := resolveGatewaySessionKey(key)
			keys, err := h.sessionMgr.List()
			if err != nil {
				return nil, err
			}
			for _, k := range keys {
				if k == canonical {
					return map[string]interface{}{"ok": true, "key": canonical}, nil
				}
			}
			return nil, fmt.Errorf("no session found: %s", key)
		}
		if hasSessionId {
			canonical := resolveGatewaySessionKey(sessionId)
			keys, err := h.sessionMgr.List()
			if err != nil {
				return nil, err
			}
			for _, k := range keys {
				if k == canonical {
					return map[string]interface{}{"ok": true, "key": canonical}, nil
				}
			}
			// sessionId 可能是字面 key，再按列表中的 key 匹配
			for _, k := range keys {
				if k == sessionId {
					return map[string]interface{}{"ok": true, "key": k}, nil
				}
			}
			return nil, fmt.Errorf("no session found: %s", sessionId)
		}
		keys, err := h.sessionMgr.List()
		if err != nil {
			return nil, err
		}
		for _, k := range keys {
			sess, err := h.getSession(k)
			if err != nil {
				continue
			}
			if lab, _ := sess.Metadata["label"].(string); strings.TrimSpace(lab) == label {
				return map[string]interface{}{"ok": true, "key": sess.Key}, nil
			}
		}
		return nil, fmt.Errorf("no session found for label: %s", label)
	})

	// sessions.clear - 清空会话
	h.registry.Register("sessions.clear", func(sessionID string, params map[string]interface{}) (interface{}, error) {
		key, ok := params["key"].(string)
		if !ok {
			return nil, fmt.Errorf("key parameter is required")
		}

		sess, err := h.getSession(key)
		if err != nil {
			return nil, fmt.Errorf("failed to get session: %w", err)
		}

		sess.Clear()

		return map[string]interface{}{
			"status": "cleared",
			"key":    key,
		}, nil
	})
}

// registerChannelMethods 注册 Channel 方法
func (h *Handler) registerChannelMethods() {
	// channels.status - 获取通道状态；未传 channel 时返回全量 ChannelsStatusSnapshot
	h.registry.Register("channels.status", func(sessionID string, params map[string]interface{}) (interface{}, error) {
		channelNames := h.channelMgr.List()
		name, hasName := params["channel"].(string)
		if hasName && name != "" {
			channelNames = []string{name}
		}

		channelsMap := make(map[string]interface{})
		channelAccounts := make(map[string]interface{})
		channelDefaultAccountId := make(map[string]string)
		for _, n := range channelNames {
			status, err := h.channelMgr.Status(n)
			if err != nil {
				channelsMap[n] = map[string]interface{}{"error": err.Error()}
				continue
			}
			channelsMap[n] = status
			channelAccounts[n] = []interface{}{}
			channelDefaultAccountId[n] = ""
		}

		return map[string]interface{}{
			"ts":                      time.Now().UnixMilli(),
			"channelOrder":            channelNames,
			"channelLabels":           map[string]string{},
			"channels":                channelsMap,
			"channelAccounts":        channelAccounts,
			"channelDefaultAccountId": channelDefaultAccountId,
		}, nil
	})

	// channels.list - 列出所有通道
	h.registry.Register("channels.list", func(sessionID string, params map[string]interface{}) (interface{}, error) {
		channels := h.channelMgr.List()
		return map[string]interface{}{
			"channels": channels,
		}, nil
	})

	// channels.logout - 登出通道（如 whatsapp）
	h.registry.Register("channels.logout", func(sessionID string, params map[string]interface{}) (interface{}, error) {
		channel, _ := params["channel"].(string)
		if channel == "" {
			return nil, fmt.Errorf("channel parameter is required")
		}
		ch, ok := h.channelMgr.Get(channel)
		if ok {
			if logoutCh, ok := ch.(interface{ Logout() error }); ok {
				_ = logoutCh.Logout()
			}
		}
		return map[string]interface{}{"ok": true, "channel": channel}, nil
	})

	// send - 发送消息到通道
	h.registry.Register("send", func(sessionID string, params map[string]interface{}) (interface{}, error) {
		channel, ok := params["channel"].(string)
		if !ok {
			return nil, fmt.Errorf("channel parameter is required")
		}

		chatID, ok := params["chat_id"].(string)
		if !ok {
			return nil, fmt.Errorf("chat_id parameter is required")
		}

		content, ok := params["content"].(string)
		if !ok {
			return nil, fmt.Errorf("content parameter is required")
		}

		msg := &bus.OutboundMessage{
			Channel:   channel,
			ChatID:    chatID,
			Content:   content,
			Timestamp: time.Now(),
		}

		if err := h.bus.PublishOutbound(context.Background(), msg); err != nil {
			return nil, fmt.Errorf("failed to send message: %w", err)
		}

		return map[string]interface{}{
			"status":  "sent",
			"msg_id":  msg.ID,
			"channel": channel,
			"chat_id": chatID,
		}, nil
	})

	// chat.send - 按 sessionKey 发送消息，写入会话历史并发布到总线
	h.registry.Register("chat.send", func(sessionID string, params map[string]interface{}) (interface{}, error) {
		sessionKey, _ := params["sessionKey"].(string)
		if sessionKey == "" {
			if k, _ := params["session_key"].(string); k != "" {
				sessionKey = k
			}
		}
		if sessionKey == "" {
			return nil, fmt.Errorf("sessionKey or session_key is required")
		}
		message, _ := params["message"].(string)

		// 使用前端传入的 idempotencyKey 作为 runId（与 openclaw 一致）
		idempotencyKey, _ := params["idempotencyKey"].(string)
		if idempotencyKey == "" {
			idempotencyKey = uuid.New().String()
		}

		sess, err := h.getSession(sessionKey)
		if err != nil {
			return nil, fmt.Errorf("failed to get session: %w", err)
		}

		media := make([]session.Media, 0)
		if atts, ok := params["attachments"].([]interface{}); ok {
			for _, a := range atts {
				att, _ := a.(map[string]interface{})
				if att == nil {
					continue
				}
				mimeType, _ := att["mimeType"].(string)
				content, _ := att["content"].(string)
				if content != "" {
					media = append(media, session.Media{Type: "image", Base64: content, MimeType: mimeType})
				}
			}
		}

		userMsg := session.Message{
			Role:      "user",
			Content:   message,
			Media:     media,
			Timestamp: time.Now(),
		}
		sess.AddMessage(userMsg)
		if err := h.sessionMgr.Save(sess); err != nil {
			return nil, fmt.Errorf("failed to save session: %w", err)
		}

		busMedia := make([]bus.Media, 0, len(media))
		for _, m := range media {
			busMedia = append(busMedia, bus.Media{Type: m.Type, Base64: m.Base64, MimeType: m.MimeType})
		}
		msg := &bus.InboundMessage{
			ID:        idempotencyKey, // 使用 idempotencyKey 作为消息 ID，确保与前端 runId 一致
			Channel:   "websocket",
			SenderID:  sessionID,
			ChatID:    sessionKey,
			Content:   message,
			Media:     busMedia,
			Timestamp: time.Now(),
		}
		if err := h.bus.PublishInbound(context.Background(), msg); err != nil {
			return nil, fmt.Errorf("failed to publish message: %w", err)
		}

		// 返回与 openclaw 一致的格式：{runId, status: "started"}
		return map[string]interface{}{
			"runId":  idempotencyKey,
			"status": "started",
		}, nil
	})
}

// registerBrowserMethods 注册 Browser 方法
func (h *Handler) registerBrowserMethods() {
	// browser.request - 浏览器请求
	h.registry.Register("browser.request", func(sessionID string, params map[string]interface{}) (interface{}, error) {
		action, ok := params["action"].(string)
		if !ok {
			return nil, fmt.Errorf("action parameter is required")
		}

		// 这里应该调用浏览器工具
		// 简化实现：返回模拟响应
		return map[string]interface{}{
			"status": "executed",
			"action": action,
			"result": "browser action executed",
		}, nil
	})
}

// configSchemaAndHints 返回 config 的 JSON Schema 与 uiHints（供 config.schema RPC）
func configSchemaAndHints() (schema map[string]interface{}, uiHints map[string]interface{}) {
	schema = map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"workspace": map[string]interface{}{"type": "object", "properties": map[string]interface{}{"path": map[string]interface{}{"type": "string"}}},
			"agents":    map[string]interface{}{"type": "object"},
			"channels":  map[string]interface{}{"type": "object"},
			"providers": map[string]interface{}{"type": "object"},
			"gateway":   map[string]interface{}{"type": "object"},
			"session":   map[string]interface{}{"type": "object"},
			"tools":     map[string]interface{}{"type": "object"},
			"approvals": map[string]interface{}{"type": "object"},
			"memory":    map[string]interface{}{"type": "object"},
			"skills":    map[string]interface{}{"type": "object"},
		},
	}
	uiHints = map[string]interface{}{
		"gateway": map[string]interface{}{"label": "Gateway", "description": "Gateway server settings"},
		"agents":  map[string]interface{}{"label": "Agents", "description": "Agent defaults and list"},
	}
	return schema, uiHints
}

// BroadcastNotification 广播通知
func (h *Handler) BroadcastNotification(method string, data interface{}) ([]byte, error) {
	notif := JSONRPCRequest{
		JSONRPC: "2.0",
		Method:  method,
		Params: map[string]interface{}{
			"data": data,
		},
	}

	return json.Marshal(notif)
}
