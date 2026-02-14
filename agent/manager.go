package agent

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/smallnest/goclaw/agent/tools"
	"github.com/smallnest/goclaw/bus"
	"github.com/smallnest/goclaw/config"
	"github.com/smallnest/goclaw/internal/logger"
	"github.com/smallnest/goclaw/process"
	"github.com/smallnest/goclaw/providers"
	"github.com/smallnest/goclaw/session"
	"github.com/smallnest/goclaw/types"
	"go.uber.org/zap"
)

// AgentManager 管理多个 Agent 实例
type AgentManager struct {
	agents         map[string]*Agent        // agentID -> Agent
	bindings       map[string]*BindingEntry // channel:accountID -> BindingEntry
	defaultAgent   *Agent                   // 默认 Agent
	bus            *bus.MessageBus
	sessionMgr     *session.Manager
	provider       providers.Provider
	tools          *ToolRegistry
	mu             sync.RWMutex
	cfg            *config.Config
	contextBuilder *ContextBuilder
	skillsLoader   *SkillsLoader
	// 分身支持
	subagentRegistry  *SubagentRegistry
	subagentAnnouncer *SubagentAnnouncer
	dataDir           string
}

// BindingEntry Agent 绑定条目
type BindingEntry struct {
	AgentID   string
	Channel   string
	AccountID string
	Agent     *Agent
}

// NewAgentManagerConfig AgentManager 配置
type NewAgentManagerConfig struct {
	Bus            *bus.MessageBus
	Provider       providers.Provider
	SessionMgr     *session.Manager
	Tools          *ToolRegistry
	DataDir        string          // 数据目录，用于存储分身注册表
	ContextBuilder *ContextBuilder // 上下文构建器
	SkillsLoader   *SkillsLoader   // 技能加载器
}

// NewAgentManager 创建 Agent 管理器
func NewAgentManager(cfg *NewAgentManagerConfig) *AgentManager {
	// 创建分身注册表
	subagentRegistry := NewSubagentRegistry(cfg.DataDir)

	// 创建分身宣告器
	subagentAnnouncer := NewSubagentAnnouncer(nil) // 回调在 Start 中设置

	return &AgentManager{
		agents:            make(map[string]*Agent),
		bindings:          make(map[string]*BindingEntry),
		bus:               cfg.Bus,
		sessionMgr:        cfg.SessionMgr,
		provider:          cfg.Provider,
		tools:             cfg.Tools,
		subagentRegistry:  subagentRegistry,
		subagentAnnouncer: subagentAnnouncer,
		dataDir:           cfg.DataDir,
		contextBuilder:    cfg.ContextBuilder,
		skillsLoader:      cfg.SkillsLoader,
	}
}

// readLatestAssistantReply 读取会话最后一条 assistant 消息内容（与 OpenClaw readLatestAssistantReply 对齐）
func (m *AgentManager) readLatestAssistantReply(sessionKey string) string {
	sess, err := m.sessionMgr.GetOrCreate(sessionKey)
	if err != nil {
		return ""
	}
	history := sess.GetHistory(-1)
	for i := len(history) - 1; i >= 0; i-- {
		if strings.ToLower(history[i].Role) == "assistant" {
			return strings.TrimSpace(history[i].Content)
		}
	}
	return ""
}

// handleSubagentCompletion 处理分身完成事件（与 OpenClaw 一致：读子会话最后回复作 Findings，cleanup=delete 时删除子会话）
func (m *AgentManager) handleSubagentCompletion(runID string, record *SubagentRunRecord) {
	logger.Info("Subagent completed",
		zap.String("run_id", runID),
		zap.String("task", record.Task))

	// 启动宣告流程
	if record.Outcome != nil {
		latestReply := m.readLatestAssistantReply(record.ChildSessionKey)
		announceParams := &SubagentAnnounceParams{
			ChildSessionKey:     record.ChildSessionKey,
			ChildRunID:          record.RunID,
			RequesterSessionKey: record.RequesterSessionKey,
			RequesterOrigin:     record.RequesterOrigin,
			RequesterDisplayKey: record.RequesterDisplayKey,
			Task:                record.Task,
			Label:               record.Label,
			LatestReply:         latestReply,
			StartedAt:           record.StartedAt,
			EndedAt:             record.EndedAt,
			Outcome:             record.Outcome,
			Cleanup:             record.Cleanup,
			AnnounceType:        SubagentAnnounceTypeTask,
		}

		if err := m.subagentAnnouncer.RunAnnounceFlow(announceParams); err != nil {
			logger.Error("Failed to announce subagent result",
				zap.String("run_id", runID),
				zap.Error(err))
		} else if record.Cleanup == "delete" {
			// 与 OpenClaw 一致：宣告成功后若 cleanup=delete 则删除子会话（含 transcript）
			if err := m.sessionMgr.Delete(record.ChildSessionKey); err != nil {
				logger.Warn("Failed to delete subagent session after announce",
					zap.String("child_session_key", record.ChildSessionKey),
					zap.Error(err))
			} else {
				logger.Info("Subagent session deleted after announce",
					zap.String("child_session_key", record.ChildSessionKey))
			}
		}

		// 标记清理完成
		m.subagentRegistry.Cleanup(runID, record.Cleanup, true)
	}
}

// SetupFromConfig 从配置设置 Agent 和绑定
func (m *AgentManager) SetupFromConfig(cfg *config.Config, contextBuilder *ContextBuilder) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.cfg = cfg
	m.contextBuilder = contextBuilder

	logger.Info("Setting up agents from config")

	// 1. 先注册所有工具（含 sessions_spawn、sessions_list 等），再创建 Agent，这样 createAgent 时 ListExisting() 已包含全部工具，主 agent 才能看到子 agent/会话工具
	m.setupSubagentSupport(cfg, contextBuilder)
	m.setupSessionTools()

	// 与 OpenClaw 一致：子 agent 使用全局 lane "subagent"，并发数由配置控制
	subagentConcurrent := 8
	if cfg.Agents.Defaults.Subagents != nil && cfg.Agents.Defaults.Subagents.MaxConcurrent > 0 {
		subagentConcurrent = cfg.Agents.Defaults.Subagents.MaxConcurrent
	}
	process.SetCommandLaneConcurrency(string(process.LaneSubagent), subagentConcurrent)

	// 2. 创建 Agent 实例（此时 state.Tools 会包含上面已注册的 sessions_spawn、sessions_list 等）
	for _, agentCfg := range cfg.Agents.List {
		if err := m.createAgent(agentCfg, contextBuilder, cfg); err != nil {
			logger.Error("Failed to create agent",
				zap.String("agent_id", agentCfg.ID),
				zap.Error(err))
			continue
		}
	}

	// 3. 如果没有配置 Agent，创建默认 Agent
	if len(m.agents) == 0 {
		logger.Info("No agents configured, creating default agent")
		defaultAgentCfg := config.AgentConfig{
			ID:        "main", // 使用 "main" 作为默认 Agent ID，与 gateway 返回的 mainSessionKey 一致
			Name:      "Default Agent",
			Default:   true,
			Model:     cfg.Agents.Defaults.Model,
			Workspace: cfg.Workspace.Path,
		}
		if err := m.createAgent(defaultAgentCfg, contextBuilder, cfg); err != nil {
			return fmt.Errorf("failed to create default agent: %w", err)
		}
	}

	// 4. 设置绑定
	for _, binding := range cfg.Bindings {
		if err := m.setupBinding(binding); err != nil {
			logger.Error("Failed to setup binding",
				zap.String("agent_id", binding.AgentID),
				zap.String("channel", binding.Match.Channel),
				zap.String("account_id", binding.Match.AccountID),
				zap.Error(err))
		}
	}

	logger.Info("Agent manager setup complete",
		zap.Int("agents", len(m.agents)),
		zap.Int("bindings", len(m.bindings)))

	return nil
}

// setupSubagentSupport 设置分身支持
func (m *AgentManager) setupSubagentSupport(cfg *config.Config, contextBuilder *ContextBuilder) {
	// 加载分身注册表
	if err := m.subagentRegistry.LoadFromDisk(); err != nil {
		logger.Warn("Failed to load subagent registry", zap.Error(err))
	}

	// 设置分身运行完成回调
	m.subagentRegistry.SetOnRunComplete(func(runID string, record *SubagentRunRecord) {
		m.handleSubagentCompletion(runID, record)
	})

	// 启动时恢复未收尾的子 agent（异常退出前已完成或未完成的 Run），与 OpenClaw initSubagentRegistry + restoreSubagentRunsOnce 对齐
	m.subagentRegistry.RecoverAfterRestart()

	// 更新宣告器回调
	m.subagentAnnouncer = NewSubagentAnnouncer(func(sessionKey, message string) error {
		// 发送宣告消息到指定会话
		return m.sendToSession(sessionKey, message)
	})

	// 创建分身注册表适配器
	registryAdapter := &subagentRegistryAdapter{registry: m.subagentRegistry}

	// 注册 sessions_spawn 工具
	spawnTool := tools.NewSubagentSpawnTool(registryAdapter)
	spawnTool.SetAgentConfigGetter(func(agentID string) *config.AgentConfig {
		for _, agentCfg := range cfg.Agents.List {
			if agentCfg.ID == agentID {
				return &agentCfg
			}
		}
		return nil
	})
	spawnTool.SetDefaultConfigGetter(func() *config.AgentDefaults {
		return &cfg.Agents.Defaults
	})
	spawnTool.SetAgentIDGetter(func(sessionKey string) string {
		// 从会话密钥中解析 agent ID
		agentID, _, _ := ParseAgentSessionKey(sessionKey)
		if agentID == "" {
			// 尝试从绑定中查找
			for _, entry := range m.bindings {
				if entry.Agent != nil {
					return entry.AgentID
				}
			}
		}
		return agentID
	})
	spawnTool.SetOnSpawn(func(result *tools.SubagentSpawnResult) error {
		return m.handleSubagentSpawn(result)
	})

	// 注册工具
	if err := m.tools.RegisterExisting(spawnTool); err != nil {
		logger.Error("Failed to register sessions_spawn tool", zap.Error(err))
	}

	logger.Info("Subagent support configured")
}

// setupSessionTools 注册会话类工具（sessions_list, sessions_history, sessions_send, session_status）
func (m *AgentManager) setupSessionTools() {
	if err := m.tools.RegisterExisting(tools.NewSessionsListTool(m.sessionMgr)); err != nil {
		logger.Warn("Failed to register sessions_list tool", zap.Error(err))
	}
	if err := m.tools.RegisterExisting(tools.NewSessionsHistoryTool(m.sessionMgr)); err != nil {
		logger.Warn("Failed to register sessions_history tool", zap.Error(err))
	}
	sendTool := tools.NewSessionsSendTool(m.sessionMgr, func(ctx context.Context, sessionKey, content string) error {
		return m.sendToSession(sessionKey, content)
	})
	if err := m.tools.RegisterExisting(sendTool); err != nil {
		logger.Warn("Failed to register sessions_send tool", zap.Error(err))
	}
	// session_status 不注入 currentKey，调用方需传 session_key 参数
	if err := m.tools.RegisterExisting(tools.NewSessionStatusTool(m.sessionMgr, nil)); err != nil {
		logger.Warn("Failed to register session_status tool", zap.Error(err))
	}
}

// subagentRegistryAdapter 分身注册表适配器
type subagentRegistryAdapter struct {
	registry *SubagentRegistry
}

// RegisterRun 注册分身运行
func (a *subagentRegistryAdapter) RegisterRun(params *tools.SubagentRunParams) error {
	// 转换并规范化 RequesterOrigin（与 OpenClaw DeliveryContext 对齐）
	var requesterOrigin *DeliveryContext
	if params.RequesterOrigin != nil {
		requesterOrigin = NormalizeDeliveryContext(&DeliveryContext{
			Channel:   params.RequesterOrigin.Channel,
			AccountID: params.RequesterOrigin.AccountID,
			To:        params.RequesterOrigin.To,
			ThreadID:  params.RequesterOrigin.ThreadID,
		})
	}

	return a.registry.RegisterRun(&SubagentRunParams{
		RunID:               params.RunID,
		ChildSessionKey:     params.ChildSessionKey,
		RequesterSessionKey: params.RequesterSessionKey,
		RequesterOrigin:     requesterOrigin,
		RequesterDisplayKey: params.RequesterDisplayKey,
		Task:                params.Task,
		Cleanup:             params.Cleanup,
		Label:               params.Label,
		ArchiveAfterMinutes: params.ArchiveAfterMinutes,
	})
}

// handleSubagentSpawn 处理分身生成（与 OpenClaw 一致：通过 internal 入站走主 agent 同一套 session + lane + 执行路径）
func (m *AgentManager) handleSubagentSpawn(result *tools.SubagentSpawnResult) error {
	if !session.IsSubagentSessionKey(result.ChildSessionKey) {
		return fmt.Errorf("invalid subagent session key: %s", result.ChildSessionKey)
	}

	record, ok := m.subagentRegistry.GetRun(result.RunID)
	if !ok {
		return fmt.Errorf("subagent run not found: %s", result.RunID)
	}

	logger.Info("Subagent spawn: publishing internal run",
		zap.String("run_id", result.RunID),
		zap.String("child_session_key", result.ChildSessionKey),
		zap.String("task", record.Task))

	// 与 OpenClaw 一致：先创建子会话并写入 label/spawnedBy，便于 sessions.list 前端展示
	sess, err := m.sessionMgr.GetOrCreate(result.ChildSessionKey)
	if err != nil {
		return fmt.Errorf("get or create subagent session: %w", err)
	}
	updates := map[string]interface{}{"spawnedBy": record.RequesterSessionKey}
	if record.Label != "" {
		updates["label"] = record.Label
	}
	sess.PatchMetadata(updates)
	if err := m.sessionMgr.Save(sess); err != nil {
		return fmt.Errorf("save subagent session metadata: %w", err)
	}

	// 与 OpenClaw 一致：子 agent 通过 gateway "agent" 等价路径执行（sessionKey=childSessionKey, lane=subagent）
	// 这里发布一条 internal 入站消息，由同一套 HandleInbound → GetOrCreate(session) → processMessageAsync(lane=subagent) → executeAgentRun 处理
	internalMsg := &bus.InboundMessage{
		ID:        result.RunID,
		Channel:   "internal",
		ChatID:    result.ChildSessionKey,
		Content:   record.Task,
		Timestamp: time.Now(),
	}
	if err := m.bus.PublishInbound(context.Background(), internalMsg); err != nil {
		return fmt.Errorf("failed to publish subagent run: %w", err)
	}
	return nil
}

// sendToSession 发送消息到指定会话
func (m *AgentManager) sendToSession(sessionKey, message string) error {
	// 解析会话密钥获取 agent ID
	agentID, _, _ := ParseAgentSessionKey(sessionKey)

	// 获取对应的 Agent
	agent, ok := m.GetAgent(agentID)
	if !ok {
		// 尝试使用默认 Agent
		agent = m.defaultAgent
	}

	if agent == nil {
		return fmt.Errorf("no agent found for session: %s", sessionKey)
	}

	// 构建 AgentMessage
	agentMsg := AgentMessage{
		Role: RoleUser,
		Content: []ContentBlock{
			TextContent{Text: message},
		},
	}

	// 注入为 steering 消息（中断当前运行）
	// 如果 agent 正在运行，这会立即中断并处理新消息
	agent.state.Steer(agentMsg)

	logger.Info("Message sent to session",
		zap.String("session_key", sessionKey),
		zap.String("agent_id", agentID),
		zap.Int("message_length", len(message)))

	return nil
}

// createAgent 创建 Agent 实例
func (m *AgentManager) createAgent(cfg config.AgentConfig, contextBuilder *ContextBuilder, globalCfg *config.Config) error {
	// 获取 workspace 路径
	workspace := cfg.Workspace
	if workspace == "" {
		workspace = globalCfg.Workspace.Path
	}

	// 获取模型
	model := cfg.Model
	if model == "" {
		model = globalCfg.Agents.Defaults.Model
	}

	// 获取最大迭代次数
	maxIterations := globalCfg.Agents.Defaults.MaxIterations
	if maxIterations == 0 {
		maxIterations = 15
	}

	temperature := globalCfg.Agents.Defaults.Temperature
	maxTokens := globalCfg.Agents.Defaults.MaxTokens

	// 上下文窗口与压缩：从 agent 默认与 profile 解析（profile 暂不传）
	ctxTokens, _ := ResolveContextWindow(globalCfg.Agents.Defaults.ContextTokens, 0)
	reserveTokens := EffectiveReserveTokens(0) // 4096
	maxHistoryTurns := globalCfg.Agents.Defaults.LimitHistoryTurns // 0 表示不限制轮次（与 OpenClaw 对齐）

	// 创建 Agent
	agent, err := NewAgent(&NewAgentConfig{
		ID:                          cfg.ID, // 传递 agent ID
		Bus:                         m.bus,
		Provider:                    m.provider,
		SessionMgr:                  m.sessionMgr,
		Tools:                       m.tools,
		Context:                     contextBuilder,
		Model:                       model,
		Workspace:                   workspace,
		MaxIteration:                maxIterations,
		Temperature:                 temperature,
		MaxTokens:                   maxTokens,
		ContextWindowTokens:         ctxTokens,
		ReserveTokens:                reserveTokens,
		MaxHistoryTurns:             maxHistoryTurns,
		ModelRequestIntervalSeconds: globalCfg.Agents.Defaults.ModelRequestIntervalSeconds,
		SkillsLoader:                m.skillsLoader,
	})
	if err != nil {
		return fmt.Errorf("failed to create agent %s: %w", cfg.ID, err)
	}

	// 设置系统提示词
	if cfg.SystemPrompt != "" {
		agent.SetSystemPrompt(cfg.SystemPrompt)
	}

	// 存储到管理器
	m.agents[cfg.ID] = agent

	// 如果是默认 Agent，设置默认
	if cfg.Default {
		m.defaultAgent = agent
	}

	logger.Info("Agent created",
		zap.String("agent_id", cfg.ID),
		zap.String("name", cfg.Name),
		zap.String("workspace", workspace),
		zap.String("model", model),
		zap.Bool("is_default", cfg.Default))

	return nil
}

// setupBinding 设置 Agent 绑定
func (m *AgentManager) setupBinding(binding config.BindingConfig) error {
	// 获取 Agent
	agent, ok := m.agents[binding.AgentID]
	if !ok {
		return fmt.Errorf("agent not found: %s", binding.AgentID)
	}

	// 构建绑定键
	bindingKey := fmt.Sprintf("%s:%s", binding.Match.Channel, binding.Match.AccountID)

	// 存储绑定
	m.bindings[bindingKey] = &BindingEntry{
		AgentID:   binding.AgentID,
		Channel:   binding.Match.Channel,
		AccountID: binding.Match.AccountID,
		Agent:     agent,
	}

	logger.Info("Binding setup",
		zap.String("binding_key", bindingKey),
		zap.String("agent_id", binding.AgentID))

	return nil
}

// RouteInbound 路由入站消息到对应的 Agent
func (m *AgentManager) RouteInbound(ctx context.Context, msg *bus.InboundMessage) error {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var agent *Agent

	// 与 OpenClaw 一致：internal channel 为子 agent 触发，sessionKey=ChatID，agent 从 sessionKey 解析
	if msg.Channel == "internal" {
		agentID, _, ok := ParseAgentSessionKey(msg.ChatID)
		if !ok {
			return fmt.Errorf("invalid internal session key: %s", msg.ChatID)
		}
		if a, ok := m.GetAgent(agentID); ok {
			agent = a
		} else {
			agent = m.defaultAgent
		}
		if agent == nil {
			return fmt.Errorf("no agent for internal run: %s", agentID)
		}
		logger.Debug("Internal (subagent) message routed by session key",
			zap.String("chat_id", msg.ChatID),
			zap.String("agent_id", agentID))
		return m.handleInboundMessage(ctx, msg, agent)
	}

	// 构建绑定键
	bindingKey := fmt.Sprintf("%s:%s", msg.Channel, msg.AccountID)

	// 查找绑定的 Agent
	entry, ok := m.bindings[bindingKey]
	if ok {
		agent = entry.Agent
		logger.Debug("Message routed by binding",
			zap.String("binding_key", bindingKey),
			zap.String("agent_id", entry.AgentID))
	} else if m.defaultAgent != nil {
		agent = m.defaultAgent
		logger.Debug("Message routed to default agent",
			zap.String("channel", msg.Channel),
			zap.String("account_id", msg.AccountID))
	} else {
		return fmt.Errorf("no agent found for message: %s", bindingKey)
	}

	return m.handleInboundMessage(ctx, msg, agent)
}

// handleInboundMessage 处理入站消息
func (m *AgentManager) handleInboundMessage(ctx context.Context, msg *bus.InboundMessage, agent *Agent) error {
	// 调用 Agent 处理消息（内部逻辑和 agent.go 中的 handleInboundMessage 类似）
	logger.Info("Processing inbound message",
		zap.String("channel", msg.Channel),
		zap.String("account_id", msg.AccountID),
		zap.String("chat_id", msg.ChatID))

	// 获取 agent ID（从 agent 实例或使用默认值）
	agentID := session.DefaultAgentID
	if agent != nil && agent.GetID() != "" {
		agentID = agent.GetID()
	}

	// 获取配置中的 mainKey
	mainKey := session.DefaultMainKey
	if m.cfg != nil && m.cfg.Session.MainKey != "" {
		mainKey = m.cfg.Session.MainKey
	}

	// 生成会话键（与 OpenClaw 对齐：所有 session key 都以 agent:<agentId>: 开头）
	var sessionKey string
	if msg.Channel == "internal" {
		// 子 agent：ChatID 即为 childSessionKey（agent:<id>:subagent:<uuid>）
		sessionKey = msg.ChatID
		logger.Info("Using child session for subagent", zap.String("session_key", sessionKey))
	} else if msg.Channel == "websocket" {
		// Web 控制台：
		// - 如果 chatID 已经是 agent:xxx:xxx 格式，直接使用
		// - 否则使用主会话 key
		if session.IsAgentSessionKey(msg.ChatID) {
			sessionKey = msg.ChatID
		} else {
			sessionKey = session.BuildAgentMainSessionKey(agentID, mainKey)
		}
	} else if msg.ChatID == "default" || msg.ChatID == "" {
		// CLI/default 场景：使用主会话
		sessionKey = session.BuildAgentMainSessionKey(agentID, mainKey)
		logger.Info("Using main session for default chat", zap.String("session_key", sessionKey))
	} else {
		// 其他渠道：根据是否为群组决定 session key
		isGroup := session.IsGroupSessionKey(msg.ChatID) || strings.Contains(strings.ToLower(msg.ChatID), "group")
		sessionKey = session.BuildAgentSessionKey(agentID, msg.Channel, msg.AccountID, msg.ChatID, mainKey, isGroup)
	}

	logger.Info("Resolved session key",
		zap.String("original_chat_id", msg.ChatID),
		zap.String("session_key", sessionKey),
		zap.String("agent_id", agentID))

	// 获取或创建会话
	sess, err := m.sessionMgr.GetOrCreate(sessionKey)
	if err != nil {
		logger.Error("Failed to get session", zap.Error(err))
		return err
	}

	// 转换为 Agent 消息
	agentMsg := AgentMessage{
		Role:      RoleUser,
		Content:   []ContentBlock{TextContent{Text: msg.Content}},
		Timestamp: msg.Timestamp.UnixMilli(),
	}

	// 添加媒体内容
	for _, media := range msg.Media {
		if media.Type == "image" {
			agentMsg.Content = append(agentMsg.Content, ImageContent{
				URL:      media.URL,
				Data:     media.Base64,
				MimeType: media.MimeType,
			})
		}
	}

	// 为本次 Run 创建独立 Orchestrator，避免多 agent/多会话共用同一 eventChan 导致流式事件串台
	orchestrator := agent.CreateOrchestratorForRun(sessionKey)

	// 加载历史消息并构造发给 orchestrator 的列表（与 OpenClaw 一致）
	// internal（子 agent）：先写入当前用户消息再读历史，与主 agent 同一套 session 流程
	if msg.Channel == "internal" {
		sessMsg := session.Message{
			Role:      "user",
			Content:   msg.Content,
			Timestamp: time.Now(),
		}
		sess.AddMessage(sessMsg)
		if err := m.sessionMgr.Save(sess); err != nil {
			logger.Error("Failed to save subagent session", zap.Error(err))
			return err
		}
	}
	history := sess.GetHistory(-1) // -1 表示加载所有历史消息
	historyAgentMsgs := sessionMessagesToAgentMessages(history)
	var allMessages []AgentMessage
	if msg.Channel == "websocket" || msg.Channel == "internal" {
		allMessages = historyAgentMsgs
	} else {
		allMessages = append(historyAgentMsgs, agentMsg)
	}

	logger.Info("About to call orchestrator.Run",
		zap.String("session_key", sessionKey),
		zap.Int("history_count", len(history)),
		zap.Int("all_messages_count", len(allMessages)))

	// 异步处理消息，避免阻塞主 agent 接收新消息
	go m.processMessageAsync(ctx, msg, agent, orchestrator, allMessages, sessionKey, agentMsg, sess, len(history))

	return nil
}

// processMessageAsync 异步处理消息，避免阻塞主 agent
// 使用 lane-based 队列：子 agent 用全局 lane "subagent"（与 OpenClaw CommandLane.Subagent 一致），主 agent 用 session lane
func (m *AgentManager) processMessageAsync(ctx context.Context, msg *bus.InboundMessage, agent *Agent, orchestrator *Orchestrator, allMessages []AgentMessage, sessionKey string, agentMsg AgentMessage, sess *session.Session, historyLen int) {
	lane := fmt.Sprintf("session:%s", sessionKey)
	if session.IsSubagentSessionKey(sessionKey) {
		lane = string(process.LaneSubagent)
	}

	// 单次 Run 超时：模型 API 断开或不可达时不会无限卡住，超时后 ctx 取消、返回错误给用户（continueExecuteAgentRun 会发 phase: error）
	runCtx := ctx
	if cfg := config.Get(); cfg != nil && cfg.Agents.Defaults.RunTimeoutSeconds > 0 {
		runCtx, _ = context.WithTimeout(ctx, time.Duration(cfg.Agents.Defaults.RunTimeoutSeconds)*time.Second)
	}

	go func() {
		_, err := process.EnqueueCommandInLane(ctx, lane, func(laneCtx context.Context) (interface{}, error) {
			return m.executeAgentRun(runCtx, msg, agent, orchestrator, allMessages, sessionKey, agentMsg, sess, historyLen)
		}, nil)
		if err != nil {
			logger.Error("Failed to execute agent run in lane",
				zap.String("lane", lane),
				zap.String("session_key", sessionKey),
				zap.Error(err))
		}
	}()
}

// buildRunOptionsForSession 子 agent 会话时返回 agents.defaults.subagents 的 model/max_iterations 覆盖，主会话返回 nil。
// 子 agent 使用的 model 与配置 agents.defaults.subagents.model 完全一致（仅 TrimSpace），不修改格式。
func (m *AgentManager) buildRunOptionsForSession(sessionKey string) *RunOptions {
	if !session.IsSubagentSessionKey(sessionKey) {
		return nil
	}
	cfg := config.Get()
	if cfg == nil || cfg.Agents.Defaults.Subagents == nil {
		logger.Debug("Subagent run: no subagents config, using agent default model")
		return nil
	}
	s := cfg.Agents.Defaults.Subagents
	model := strings.TrimSpace(s.Model) // 与配置一致，仅去首尾空格
	maxIter := 15
	// 子 agent 可用独立 model；若配置了 timeout_seconds 可适当提高迭代上限（按每轮约 30s 粗算）
	if s.TimeoutSeconds > 0 && s.TimeoutSeconds > 60 {
		if n := s.TimeoutSeconds / 30; n > maxIter {
			maxIter = n
		}
	}
	if model == "" {
		logger.Info("Subagent run options: no model override (subagents.model empty), using agent default",
			zap.String("session_key", sessionKey),
			zap.Int("max_iterations", maxIter))
		return &RunOptions{MaxIterations: maxIter}
	}
	logger.Info("Subagent run options: using model from agents.defaults.subagents.model",
		zap.String("session_key", sessionKey),
		zap.String("model", model),
		zap.Int("max_iterations", maxIter))
	return &RunOptions{Model: model, MaxIterations: maxIter}
}

// emitAgentEvent 向总线发送 Agent 事件（与 OpenClaw emitAgentEvent 对齐），供 Control UI 显示进度
func (m *AgentManager) emitAgentEvent(ctx context.Context, runId, sessionKey string, seq *int, stream bus.AgentEventStream, data map[string]interface{}) {
	*seq++
	payload := &bus.AgentEventPayload{
		RunId:      runId,
		Seq:        *seq,
		Stream:     stream,
		Ts:         time.Now().UnixMilli(),
		Data:       data,
		SessionKey: sessionKey,
	}
	_ = m.bus.PublishAgentEvent(ctx, payload)
}

// executeAgentRun 执行 agent 运行（在 lane 中串行执行）
func (m *AgentManager) executeAgentRun(ctx context.Context, msg *bus.InboundMessage, agent *Agent, orchestrator *Orchestrator, allMessages []AgentMessage, sessionKey string, agentMsg AgentMessage, sess *session.Session, historyLen int) (interface{}, error) {
	runId := msg.ID
	seq := 0

	// 与 OpenClaw 一致：先发送 lifecycle start，UI 可显示“运行中”
	m.emitAgentEvent(ctx, runId, sessionKey, &seq, bus.AgentStreamLifecycle, map[string]interface{}{
		"phase": "start",
	})

	eventChan := orchestrator.Subscribe()
	eventCtx, eventCancel := context.WithCancel(ctx)
	streamDone := make(chan struct{})
	var accumulated strings.Builder

	go func() {
		defer close(streamDone)
		for {
			select {
			case <-eventCtx.Done():
				return
			case event, ok := <-eventChan:
				if !ok {
					return
				}
				if event.Type == EventMessageDelta && event.Content != "" {
					accumulated.WriteString(event.Content)
					m.publishStreamDelta(ctx, msg.Channel, msg.ChatID, msg.ID, accumulated.String())
					// 与 OpenClaw 一致：assistant 流式增量也发 agent 事件
					m.emitAgentEvent(ctx, runId, sessionKey, &seq, bus.AgentStreamAssistant, map[string]interface{}{
						"text": accumulated.String(),
					})
				}
				if event.Type == EventToolExecutionStart {
					// 与 Control UI app-tool-stream 对齐：toolCallId, name, phase, args
					m.emitAgentEvent(ctx, runId, sessionKey, &seq, bus.AgentStreamTool, map[string]interface{}{
						"toolCallId": event.ToolID,
						"name":       event.ToolName,
						"phase":      "start",
						"args":       event.ToolArgs,
					})
				}
				if event.Type == EventToolExecutionEnd {
					resultText := ""
					if event.ToolResult != nil {
						resultText = extractToolResultContent(event.ToolResult.Content)
					}
					// UI 用 phase "result" 显示工具输出
					m.emitAgentEvent(ctx, runId, sessionKey, &seq, bus.AgentStreamTool, map[string]interface{}{
						"toolCallId": event.ToolID,
						"name":       event.ToolName,
						"phase":      "result",
						"error":      event.ToolError,
						"result":     resultText,
					})
				}
			}
		}
	}()

	runOpts := m.buildRunOptionsForSession(sessionKey)
	finalMessages, err := orchestrator.Run(ctx, allMessages, runOpts)

	eventCancel()
	<-streamDone

	// 与 OpenClaw 一致：发送 lifecycle end 或 error，UI 可显示完成/错误
	if err != nil {
		m.emitAgentEvent(ctx, runId, sessionKey, &seq, bus.AgentStreamLifecycle, map[string]interface{}{
			"phase": "error",
			"error": err.Error(),
		})
	} else {
		m.emitAgentEvent(ctx, runId, sessionKey, &seq, bus.AgentStreamLifecycle, map[string]interface{}{
			"phase": "end",
		})
	}

	m.continueExecuteAgentRun(ctx, msg, sessionKey, sess, historyLen, finalMessages, agentMsg, err)

	if err != nil {
		return nil, err
	}
	return finalMessages, nil
}

// continueExecuteAgentRun 继续执行 agent 运行的后续处理
func (m *AgentManager) continueExecuteAgentRun(ctx context.Context, msg *bus.InboundMessage, sessionKey string, sess *session.Session, historyLen int, finalMessages []AgentMessage, agentMsg AgentMessage, err error) {
	logger.Info("orchestrator.Run returned",
		zap.String("session_key", sessionKey),
		zap.Int("final_messages_count", len(finalMessages)),
		zap.Error(err))

	if err != nil {
		// 子 agent 失败时也标记完成，由回调统一 announcer + cleanup
		if session.IsSubagentSessionKey(sessionKey) {
			if _, ok := m.subagentRegistry.GetRun(msg.ID); ok {
				endedAt := time.Now().UnixMilli()
				_ = m.subagentRegistry.MarkCompleted(msg.ID, &SubagentRunOutcome{Status: "error", Error: err.Error()}, &endedAt)
			}
			return
		}
		// Check if error is related to historical message incompatibility (old session format)
		errStr := err.Error()
		hasToolCallIDMismatch := strings.Contains(errStr, "tool_call_id") && strings.Contains(errStr, "mismatch")
		hasMissingReasoning := strings.Contains(errStr, "reasoning_content") && strings.Contains(errStr, "assistant tool call")
		if hasToolCallIDMismatch || hasMissingReasoning {
			logger.Warn("Detected old session format, clearing session",
				zap.String("session_key", sessionKey),
				zap.Error(err))
			// Clear old session and retry
			if delErr := m.sessionMgr.Delete(sessionKey); delErr != nil {
				logger.Error("Failed to clear old session", zap.Error(delErr))
				return
			}
			logger.Info("Cleared old session, retrying with fresh session")
			// Get fresh session
			freshSess, getErr := m.sessionMgr.GetOrCreate(sessionKey)
			if getErr != nil {
				logger.Error("Failed to create fresh session", zap.Error(getErr))
				return
			}

			// Get agent from session key
			agentID, _, _ := ParseAgentSessionKey(sessionKey)
			agent, ok := m.GetAgent(agentID)
			if !ok {
				agent = m.defaultAgent
			}
			if agent == nil {
				logger.Error("No agent found for retry")
				return
			}

			// Retry with fresh session (no history)
			finalMessages, retryErr := agent.GetOrchestrator().Run(ctx, []AgentMessage{agentMsg}, nil)
			if retryErr != nil {
				logger.Error("Agent execution failed on retry", zap.Error(retryErr))
				m.publishRunErrorToBus(ctx, msg.Channel, msg.ChatID, msg.ID, retryErr)
				return
			}
			// Update session with new messages
			m.updateSession(freshSess, finalMessages, 0)
			// Publish response
			if len(finalMessages) > 0 {
				lastMsg := finalMessages[len(finalMessages)-1]
				if lastMsg.Role == RoleAssistant {
					m.publishToBus(ctx, msg.Channel, msg.ChatID, msg.ID, lastMsg)
				}
			}
			return
		}
		logger.Error("Agent execution failed", zap.Error(err))
		m.publishRunErrorToBus(ctx, msg.Channel, msg.ChatID, msg.ID, err)
		return
	}

	// 更新会话（只保存新产生的消息）并在发布前完成 Save，保证 chat.history 能读到助手回复
	m.updateSession(sess, finalMessages, historyLen)

	// 发布响应（Save 已在 updateSession 内完成）；子 agent（internal）不推 bus，结果通过 announcer 回主会话
	if msg.Channel != "internal" && len(finalMessages) > 0 {
		lastMsg := finalMessages[len(finalMessages)-1]
		if lastMsg.Role == RoleAssistant {
			content := extractTextContent(lastMsg)
			if strings.TrimSpace(content) != "" {
				m.publishToBus(ctx, msg.Channel, msg.ChatID, msg.ID, lastMsg)
			} else {
				// LLM 返回空回复时也要发 state: "final"，否则前端收不到结束事件会一直转圈
				m.publishRunFinalToBus(ctx, msg.Channel, msg.ChatID, msg.ID, "")
			}
		}
	}

	// 子 agent 完成后标记完成，由 SetOnRunComplete 回调（handleSubagentCompletion）统一做 announcer + cleanup
	if session.IsSubagentSessionKey(sessionKey) {
		if _, ok := m.subagentRegistry.GetRun(msg.ID); ok {
			outcome := &SubagentRunOutcome{Status: "success"}
			if len(finalMessages) > 0 {
				for i := len(finalMessages) - 1; i >= 0; i-- {
					if finalMessages[i].Role == RoleAssistant {
						t := extractTextContent(finalMessages[i])
						if t != "" {
							outcome.Artifacts = []Artifact{{Type: "text", Payload: t}}
						}
						break
					}
				}
			}
			endedAt := time.Now().UnixMilli()
			_ = m.subagentRegistry.MarkCompleted(msg.ID, outcome, &endedAt)
		}
	}
}

// updateSession 更新会话
func (m *AgentManager) updateSession(sess *session.Session, messages []AgentMessage, historyLen int) {
	// 只保存新产生的消息（不包括历史消息）
	newMessages := messages
	if historyLen >= 0 && len(messages) > historyLen {
		newMessages = messages[historyLen:]
	}

	for _, msg := range newMessages {
		sessMsg := session.Message{
			Role:      string(msg.Role),
			Content:   extractTextContent(msg),
			Timestamp: time.Unix(msg.Timestamp/1000, 0),
		}

		if msg.Role == RoleAssistant {
			for _, block := range msg.Content {
				if tc, ok := block.(ToolCallContent); ok {
					sessMsg.ToolCalls = append(sessMsg.ToolCalls, session.ToolCall{
						ID:     tc.ID,
						Name:   tc.Name,
						Params: tc.Arguments,
					})
				}
			}
			if reasoning, ok := msg.Metadata["reasoning_content"].(string); ok && strings.TrimSpace(reasoning) != "" {
				if sessMsg.Metadata == nil {
					sessMsg.Metadata = make(map[string]interface{})
				}
				sessMsg.Metadata["reasoning_content"] = reasoning
			}
		}

		if msg.Role == RoleToolResult {
			if id, ok := msg.Metadata["tool_call_id"].(string); ok {
				sessMsg.ToolCallID = id
			}
			// Preserve tool_name in metadata for validation
			if toolName, ok := msg.Metadata["tool_name"].(string); ok {
				if sessMsg.Metadata == nil {
					sessMsg.Metadata = make(map[string]interface{})
				}
				sessMsg.Metadata["tool_name"] = toolName
			}
		}

		sess.AddMessage(sessMsg)
	}

	if err := m.sessionMgr.Save(sess); err != nil {
		logger.Error("Failed to save session", zap.Error(err))
	}
}

// friendlyRunErrorMessage 将限流/406 等错误转为对用户友好的中文提示，其余错误返回原始文案
func friendlyRunErrorMessage(runErr error) string {
	if runErr == nil {
		return ""
	}
	classifier := types.NewSimpleErrorClassifier()
	if classifier.ClassifyError(runErr) == types.FailoverReasonRateLimit {
		delaySec := types.ExtractRateLimitDelay(runErr, 30, 60)
		if delaySec > 0 {
			return fmt.Sprintf("请求过于频繁或模型暂时限流，请 %d 秒后再试。", delaySec)
		}
		return "请求过于频繁或模型暂时限流，请稍后再试。"
	}
	return runErr.Error()
}

// publishRunErrorToBus 在 Run 报错（如超时、模型 API 断开）时发布一条 chat 事件 state: "error"，便于前端按 chat 结束统一收尾（与 OpenClaw 的 aborted 一致）
func (m *AgentManager) publishRunErrorToBus(ctx context.Context, channel, chatID, runID string, runErr error) {
	if runErr == nil {
		return
	}
	content := friendlyRunErrorMessage(runErr)
	outbound := &bus.OutboundMessage{
		ID:        runID,
		Channel:   channel,
		ChatID:    chatID,
		Content:   content,
		ChatState: "error",
		Timestamp: time.Now(),
	}
	if err := m.bus.PublishOutbound(ctx, outbound); err != nil {
		logger.Error("Failed to publish run error to outbound", zap.Error(err))
	}
}

// publishRunFinalToBus 在 Run 正常结束但内容为空时发送 state: "final"，让前端能结束当前 run
func (m *AgentManager) publishRunFinalToBus(ctx context.Context, channel, chatID, runID, content string) {
	outbound := &bus.OutboundMessage{
		ID:        runID,
		Channel:   channel,
		ChatID:    chatID,
		Content:   content,
		ChatState: "final",
		Timestamp: time.Now(),
	}
	if err := m.bus.PublishOutbound(ctx, outbound); err != nil {
		logger.Error("Failed to publish run final to outbound", zap.Error(err))
	}
}

// publishToBus 发布消息到总线
func (m *AgentManager) publishToBus(ctx context.Context, channel, chatID, runID string, msg AgentMessage) {
	content := extractTextContent(msg)

	// 如果内容为空（只有工具调用，没有文本），不发布消息
	if strings.TrimSpace(content) == "" {
		logger.Debug("Skipping empty message publish (tool-only response)",
			zap.String("run_id", runID),
			zap.String("role", string(msg.Role)))
		return
	}

	outbound := &bus.OutboundMessage{
		ID:        runID, // 保留原始 runId，确保前端能匹配
		Channel:   channel,
		ChatID:    chatID,
		Content:   content,
		Timestamp: time.Unix(msg.Timestamp/1000, 0),
	}

	if err := m.bus.PublishOutbound(ctx, outbound); err != nil {
		logger.Error("Failed to publish outbound", zap.Error(err))
	}
}

// channelsThatSupportStreaming 仅这些通道会收到流式 delta；其他通道（如飞书、Telegram）只收到最终完整消息，避免刷屏
var channelsThatSupportStreaming = map[string]bool{
	"websocket": true, // Control UI 需要逐字展示
}

// publishStreamDelta 发布流式增量内容到总线（仅对支持流式的通道发送，飞书等只收最终消息）
func (m *AgentManager) publishStreamDelta(ctx context.Context, channel, chatID, runID, delta string) {
	if !channelsThatSupportStreaming[channel] {
		return
	}
	outbound := &bus.OutboundMessage{
		ID:        runID,
		Channel:   channel,
		ChatID:    chatID,
		Content:   delta,
		IsStream:  true, // 标记为流式增量
		Timestamp: time.Now(),
	}

	if err := m.bus.PublishOutbound(ctx, outbound); err != nil {
		logger.Error("Failed to publish stream delta", zap.Error(err))
	}
}

// sessionMessagesToAgentMessages 将 session 消息转换为 Agent 消息
func sessionMessagesToAgentMessages(sessMsgs []session.Message) []AgentMessage {
	result := make([]AgentMessage, 0, len(sessMsgs))
	for _, sessMsg := range sessMsgs {
		agentMsg := AgentMessage{
			Role:      MessageRole(sessMsg.Role),
			Content:   []ContentBlock{TextContent{Text: sessMsg.Content}},
			Timestamp: sessMsg.Timestamp.UnixMilli(),
		}

		// Handle tool calls in assistant messages
		if sessMsg.Role == "assistant" && len(sessMsg.ToolCalls) > 0 {
			// Clear the text content if there are tool calls
			agentMsg.Content = []ContentBlock{}
			for _, tc := range sessMsg.ToolCalls {
				agentMsg.Content = append(agentMsg.Content, ToolCallContent{
					ID:        tc.ID,
					Name:      tc.Name,
					Arguments: tc.Params,
				})
			}
		}

		// Handle tool result messages
		if sessMsg.Role == "tool" {
			agentMsg.Role = RoleToolResult
			// Set tool_call_id in metadata
			if sessMsg.ToolCallID != "" {
				if agentMsg.Metadata == nil {
					agentMsg.Metadata = make(map[string]any)
				}
				agentMsg.Metadata["tool_call_id"] = sessMsg.ToolCallID
			}
		}
		if reasoning, ok := sessMsg.Metadata["reasoning_content"].(string); ok && strings.TrimSpace(reasoning) != "" {
			if agentMsg.Metadata == nil {
				agentMsg.Metadata = make(map[string]any)
			}
			agentMsg.Metadata["reasoning_content"] = reasoning
		}

		result = append(result, agentMsg)
	}
	return result
}

// GetAgent 获取 Agent
func (m *AgentManager) GetAgent(agentID string) (*Agent, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	agent, ok := m.agents[agentID]
	return agent, ok
}

// ListAgents 列出所有 Agent ID
func (m *AgentManager) ListAgents() []string {
	m.mu.RLock()
	defer m.mu.RUnlock()

	ids := make([]string, 0, len(m.agents))
	for id := range m.agents {
		ids = append(ids, id)
	}
	return ids
}

// Start 启动所有 Agent
func (m *AgentManager) Start(ctx context.Context) error {
	m.mu.RLock()
	defer m.mu.RUnlock()

	for id, agent := range m.agents {
		if err := agent.Start(ctx); err != nil {
			logger.Error("Failed to start agent",
				zap.String("agent_id", id),
				zap.Error(err))
		} else {
			logger.Info("Agent started", zap.String("agent_id", id))
		}
	}

	// 启动消息处理器
	go m.processMessages(ctx)

	return nil
}

// Stop 停止所有 Agent
func (m *AgentManager) Stop() error {
	m.mu.RLock()
	defer m.mu.RUnlock()

	for id, agent := range m.agents {
		if err := agent.Stop(); err != nil {
			logger.Error("Failed to stop agent",
				zap.String("agent_id", id),
				zap.Error(err))
		}
	}

	return nil
}

// processMessages 处理入站消息
func (m *AgentManager) processMessages(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			logger.Info("Agent manager message processor stopped")
			return
		default:
			msg, err := m.bus.ConsumeInbound(ctx)
			if err != nil {
				if err == context.DeadlineExceeded || err == context.Canceled {
					continue
				}
				logger.Error("Failed to consume inbound", zap.Error(err))
				continue
			}

			if err := m.RouteInbound(ctx, msg); err != nil {
				logger.Error("Failed to route message",
					zap.String("channel", msg.Channel),
					zap.String("account_id", msg.AccountID),
					zap.Error(err))
			}
		}
	}
}

// GetDefaultAgent 获取默认 Agent
func (m *AgentManager) GetDefaultAgent() *Agent {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.defaultAgent
}

// GetToolsInfo 获取工具信息
func (m *AgentManager) GetToolsInfo() (map[string]interface{}, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	// 从 tool registry 获取工具列表
	existingTools := m.tools.ListExisting()
	result := make(map[string]interface{})

	for _, tool := range existingTools {
		result[tool.Name()] = map[string]interface{}{
			"name":        tool.Name(),
			"description": tool.Description(),
			"parameters":  tool.Parameters(),
		}
	}

	return result, nil
}

// getOrCreateSubagent 获取或创建子 agent
func (m *AgentManager) getOrCreateSubagent(parentAgentID, subagentID string, parentAgent *Agent) (*Agent, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// 构建子 agent ID
	fullSubagentID := fmt.Sprintf("%s:subagent:%s", parentAgentID, subagentID)

	// 检查是否已存在
	if subagent, ok := m.agents[fullSubagentID]; ok {
		return subagent, nil
	}

	// 创建新的子 agent（复用父 agent 的配置）
	parentState := parentAgent.GetState()

	subagent, err := NewAgent(&NewAgentConfig{
		ID:                          fullSubagentID,
		Bus:                         m.bus,
		Provider:                    m.provider,
		SessionMgr:                  m.sessionMgr,
		Tools:                       m.tools,
		Context:                     m.contextBuilder,
		Model:                       parentState.Model,
		Workspace:                   parentAgent.workspace,
		MaxIteration:                15, // 子 agent 使用默认迭代次数
		Temperature:                 0,  // 使用 provider 默认
		MaxTokens:                   0,  // 使用 provider 默认
		ContextWindowTokens:        0,  // 使用默认
		ReserveTokens:               0,  // 使用默认
		MaxHistoryTurns:             0,  // 不限制
		ModelRequestIntervalSeconds: m.cfg.Agents.Defaults.ModelRequestIntervalSeconds,
		SkillsLoader:                m.skillsLoader,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create subagent: %w", err)
	}

	// 存储子 agent
	m.agents[fullSubagentID] = subagent

	logger.Info("Subagent created",
		zap.String("subagent_id", fullSubagentID),
		zap.String("parent_agent_id", parentAgentID))

	return subagent, nil
}

// runSubagent 在后台运行子 agent
func (m *AgentManager) runSubagent(subagent *Agent, runID, sessionKey, task string) {
	ctx := context.Background()

	logger.Info("Starting subagent execution",
		zap.String("run_id", runID),
		zap.String("session_key", sessionKey),
		zap.String("task", task))

	// 构建任务消息
	taskMsg := AgentMessage{
		Role: RoleUser,
		Content: []ContentBlock{
			TextContent{Text: task},
		},
	}

	// 更新 agent 的 session key
	subagent.state.SessionKey = sessionKey

	// 运行 orchestrator
	startTime := time.Now()
	finalMessages, err := subagent.GetOrchestrator().Run(ctx, []AgentMessage{taskMsg}, nil)

	endTime := time.Now()
	endedAt := endTime.UnixMilli()

	// 构建结果
	outcome := &SubagentRunOutcome{
		Status: "success",
	}

	if err != nil {
		outcome.Status = "error"
		outcome.Error = err.Error()
		logger.Error("Subagent execution failed",
			zap.String("run_id", runID),
			zap.Error(err))
	} else {
		// 提取最后的 assistant 消息作为结果
		for i := len(finalMessages) - 1; i >= 0; i-- {
			if finalMessages[i].Role == RoleAssistant {
				// 提取文本内容
				var textParts []string
				for _, content := range finalMessages[i].Content {
					if tc, ok := content.(TextContent); ok {
						textParts = append(textParts, tc.Text)
					}
				}
				// 将结果存储为 artifact
				resultText := strings.Join(textParts, "\n")
				if resultText != "" {
					outcome.Artifacts = []Artifact{
						{
							Type:    "text",
							Payload: resultText,
						},
					}
				}
				break
			}
		}

		logger.Info("Subagent execution completed",
			zap.String("run_id", runID),
			zap.Duration("duration", endTime.Sub(startTime)))
	}

	// 标记完成
	if err := m.subagentRegistry.MarkCompleted(runID, outcome, &endedAt); err != nil {
		logger.Error("Failed to mark subagent completed",
			zap.String("run_id", runID),
			zap.Error(err))
	}

	// 获取子 agent 运行记录
	record, ok := m.subagentRegistry.GetRun(runID)
	if !ok {
		logger.Error("Failed to get subagent run record for announcement",
			zap.String("run_id", runID))
		return
	}

	// 发送 announcement 到主 agent
	announcer := NewSubagentAnnouncer(m.sendToSession)
	startedAtMs := startTime.UnixMilli()
	announceParams := &SubagentAnnounceParams{
		ChildSessionKey:     sessionKey,
		ChildRunID:          runID,
		RequesterSessionKey: record.RequesterSessionKey,
		RequesterOrigin:     record.RequesterOrigin,
		RequesterDisplayKey: record.RequesterDisplayKey,
		Task:                record.Task,
		Label:               record.Label,
		StartedAt:           &startedAtMs,
		EndedAt:             &endedAt,
		Outcome:             outcome,
		Cleanup:             record.Cleanup,
		AnnounceType:        SubagentAnnounceTypeTask,
	}

	if err := announcer.RunAnnounceFlow(announceParams); err != nil {
		logger.Error("Failed to announce subagent result",
			zap.String("run_id", runID),
			zap.Error(err))
	}
}
