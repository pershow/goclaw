package gateway

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"github.com/smallnest/goclaw/bus"
	"github.com/smallnest/goclaw/channels"
	"github.com/smallnest/goclaw/config"
	"github.com/smallnest/goclaw/internal/logger"
	"github.com/smallnest/goclaw/session"
	"go.uber.org/zap"
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		// 在生产环境中应该检查 Origin
		return true
	},
}

// Server HTTP 网关服务器
type Server struct {
	config        *config.GatewayConfig
	wsConfig      *WebSocketConfig
	bus           *bus.MessageBus
	channelMgr    *channels.Manager
	sessionMgr    *session.Manager
	server        *http.Server
	wsServer      *http.Server
	handler       *Handler
	mu            sync.RWMutex
	running       bool
	connections     map[string]*Connection
	connectionsMu   sync.RWMutex
	enableAuth      bool
	authToken       string
	broadcastSeq    atomic.Uint64
	lastHeartbeatMs atomic.Int64
}

// WebSocketConfig WebSocket 配置
type WebSocketConfig struct {
	Host           string
	Port           int
	Path           string
	EnableAuth     bool
	AuthToken      string
	PingInterval   time.Duration
	PongTimeout    time.Duration
	ReadTimeout    time.Duration
	WriteTimeout   time.Duration
	MaxMessageSize int64
	// TLS 配置
	EnableTLS bool
	CertFile  string
	KeyFile   string
}

// NewServer 创建网关服务器
func NewServer(cfg *config.GatewayConfig, messageBus *bus.MessageBus, channelMgr *channels.Manager, sessionMgr *session.Manager) *Server {
	// 从配置文件获取 WebSocket 设置，如果未配置则使用默认值
	wsPort := cfg.WebSocket.Port
	if wsPort == 0 {
		wsPort = 28789 // 默认端口
	}
	wsHost := cfg.WebSocket.Host
	if wsHost == "" {
		wsHost = "0.0.0.0" // 默认监听地址
	}
	wsPath := cfg.WebSocket.Path
	if wsPath == "" {
		wsPath = "/ws" // 默认路径
	}
	pingInterval := cfg.WebSocket.PingInterval
	if pingInterval == 0 {
		pingInterval = 30 * time.Second
	}
	pongTimeout := cfg.WebSocket.PongTimeout
	if pongTimeout == 0 {
		pongTimeout = 60 * time.Second
	}
	readTimeout := cfg.WebSocket.ReadTimeout
	if readTimeout == 0 {
		readTimeout = 60 * time.Second
	}
	writeTimeout := cfg.WebSocket.WriteTimeout
	if writeTimeout == 0 {
		writeTimeout = 10 * time.Second
	}

	return &Server{
		config: cfg,
		wsConfig: &WebSocketConfig{
			Host:           wsHost,
			Port:           wsPort,
			Path:           wsPath,
			EnableAuth:     cfg.WebSocket.EnableAuth,
			AuthToken:      cfg.WebSocket.AuthToken,
			PingInterval:   pingInterval,
			PongTimeout:    pongTimeout,
			ReadTimeout:    readTimeout,
			WriteTimeout:   writeTimeout,
			MaxMessageSize: 10 * 1024 * 1024, // 10MB
		},
		bus:         messageBus,
		channelMgr:  channelMgr,
		sessionMgr:  sessionMgr,
		handler:     NewHandler(messageBus, sessionMgr, channelMgr),
		connections: make(map[string]*Connection),
	}
}

// SetWebSocketConfig 设置 WebSocket 配置
func (s *Server) SetWebSocketConfig(cfg *WebSocketConfig) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.wsConfig = cfg
	s.enableAuth = cfg.EnableAuth
	s.authToken = cfg.AuthToken
}

// SetSessionResetPolicy 设置会话重置策略（与 OpenClaw 对齐）；由调用方根据 config.session.reset 注入
func (s *Server) SetSessionResetPolicy(policy *session.ResetPolicy) {
	s.handler.SetSessionResetPolicy(policy)
}

// Start 启动服务器
func (s *Server) Start(ctx context.Context) error {
	s.mu.Lock()
	if s.running {
		s.mu.Unlock()
		return fmt.Errorf("server already running")
	}
	s.running = true
	s.mu.Unlock()

	s.lastHeartbeatMs.Store(time.Now().UnixMilli())
	// 注入 presence 与 lastHeartbeat 供 RPC 使用
	s.handler.SetPresenceProvider(s)
	s.handler.SetLastHeartbeat(func() int64 { return s.lastHeartbeatMs.Load() })

	// 启动 HTTP 服务器
	if err := s.startHTTPServer(ctx); err != nil {
		return err
	}

	// 启动 WebSocket 服务器
	if err := s.startWebSocketServer(ctx); err != nil {
		return err
	}

	// 启动出站消息广播（使用新的订阅机制）
	go s.broadcastOutbound(ctx)

	// 监听上下文取消
	go func() {
		<-ctx.Done()
		_ = s.Stop()
	}()

	return nil
}

// startHTTPServer 启动 HTTP 服务器
func (s *Server) startHTTPServer(ctx context.Context) error {
	// 创建 HTTP 路由
	mux := http.NewServeMux()

	// 健康检查端点
	mux.HandleFunc("/health", s.handleHealth)

	// Channels API 端点
	mux.HandleFunc("/api/channels", s.handleChannelsAPI)

	// 飞书 webhook 端点
	mux.HandleFunc("/webhook/feishu", s.handleFeishuWebhook)

	// 通用 webhook 端点
	mux.HandleFunc("/webhook/", s.handleGenericWebhook)

	// WebSocket 端点（如果使用同一端口）
	mux.HandleFunc(s.wsConfig.Path, s.handleWebSocket)

	// 提供 Control UI
	if err := s.ServeControlUI(mux); err != nil {
		logger.Warn("Failed to serve Control UI", zap.Error(err))
	}

	// 创建 HTTP 服务器
	s.server = &http.Server{
		Addr:         fmt.Sprintf("%s:%d", s.config.Host, s.config.Port),
		Handler:      mux,
		ReadTimeout:  time.Duration(s.config.ReadTimeout) * time.Second,
		WriteTimeout: time.Duration(s.config.WriteTimeout) * time.Second,
	}

	// 启动服务器
	go func() {
		logger.Info("HTTP gateway server started",
			zap.String("addr", s.server.Addr),
		)

		if err := s.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error("HTTP gateway server error", zap.Error(err))
		}
	}()

	return nil
}

// startWebSocketServer 启动 WebSocket 服务器
func (s *Server) startWebSocketServer(ctx context.Context) error {
	// 如果 WebSocket 端口与 HTTP 端口相同，则复用 HTTP 服务器
	if s.wsConfig.Port == s.config.Port && s.wsConfig.Host == s.config.Host {
		logger.Info("WebSocket using same port as HTTP, skipping separate server")
		return nil
	}

	// 创建 WebSocket 路由
	mux := http.NewServeMux()

	// WebSocket 端点
	mux.HandleFunc(s.wsConfig.Path, s.handleWebSocket)

	// 健康检查端点
	mux.HandleFunc("/health", s.handleHealth)

	// Channels API 端点
	mux.HandleFunc("/api/channels", s.handleChannelsAPI)

	// 创建 WebSocket 服务器
	s.wsServer = &http.Server{
		Addr:         fmt.Sprintf("%s:%d", s.wsConfig.Host, s.wsConfig.Port),
		Handler:      mux,
		ReadTimeout:  s.wsConfig.ReadTimeout,
		WriteTimeout: s.wsConfig.WriteTimeout,
	}

	// 启动服务器
	go func() {
		logger.Info("WebSocket gateway server started",
			zap.String("addr", s.wsServer.Addr),
			zap.String("path", s.wsConfig.Path),
		)

		if err := s.wsServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error("WebSocket gateway server error", zap.Error(err))
		}
	}()

	return nil
}

// Stop 停止服务器
func (s *Server) Stop() error {
	s.mu.Lock()
	if !s.running {
		s.mu.Unlock()
		return nil
	}
	s.running = false
	s.mu.Unlock()

	// 关闭所有 WebSocket 连接
	s.closeAllConnections()

	// 停止 HTTP 服务器
	if s.server != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		if err := s.server.Shutdown(ctx); err != nil {
			logger.Error("Failed to shutdown HTTP gateway server", zap.Error(err))
		}
	}

	// 停止 WebSocket 服务器
	if s.wsServer != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		if err := s.wsServer.Shutdown(ctx); err != nil {
			logger.Error("Failed to shutdown WebSocket gateway server", zap.Error(err))
		}
	}

	logger.Info("Gateway server stopped")
	return nil
}

// closeAllConnections 关闭所有 WebSocket 连接
func (s *Server) closeAllConnections() {
	s.connectionsMu.Lock()
	defer s.connectionsMu.Unlock()

	for id, conn := range s.connections {
		conn.Close()
		delete(s.connections, id)
	}
}

// addConnection 添加连接
func (s *Server) addConnection(conn *Connection) {
	s.connectionsMu.Lock()
	defer s.connectionsMu.Unlock()
	s.connections[conn.ID] = conn
}

// removeConnection 移除连接
func (s *Server) removeConnection(id string) {
	s.connectionsMu.Lock()
	defer s.connectionsMu.Unlock()
	delete(s.connections, id)
}

// _getConnection 获取连接 (未使用，保留供将来使用)
// nolint:unused
func (s *Server) _getConnection(id string) (*Connection, bool) {
	s.connectionsMu.RLock()
	defer s.connectionsMu.RUnlock()
	conn, ok := s.connections[id]
	return conn, ok
}

// IsRunning 检查是否运行中
func (s *Server) IsRunning() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.running
}

// Handler 返回网关处理器（用于注入 presence/heartbeat）
func (s *Server) Handler() *Handler {
	return s.handler
}

// GetPresenceEntries 返回当前连接的 presence 列表，供 system-presence RPC 使用
func (s *Server) GetPresenceEntries() []map[string]interface{} {
	s.connectionsMu.RLock()
	defer s.connectionsMu.RUnlock()
	entries := make([]map[string]interface{}, 0, len(s.connections))
	for _, c := range s.connections {
		entries = append(entries, map[string]interface{}{
			"id":           c.ID,
			"connectedAtMs": c.CreatedAt.UnixMilli(),
		})
	}
	return entries
}

// handleHealth 健康检查处理器
func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)

	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"status": "ok",
		"time":   time.Now().Unix(),
	})
}

// handleFeishuWebhook 飞书 webhook 处理器
func (s *Server) handleFeishuWebhook(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// 获取飞书通道
	_, ok := s.channelMgr.Get("feishu")
	if !ok {
		http.Error(w, "Feishu channel not found", http.StatusServiceUnavailable)
		return
	}

	// 读取请求体
	body, err := io.ReadAll(r.Body)
	if err != nil {
		logger.Error("Failed to read webhook body", zap.Error(err))
		http.Error(w, "Failed to read request body", http.StatusBadRequest)
		return
	}

	// 验证签名（由通道处理）
	// 这里我们需要将请求传递给飞书通道处理
	// 由于接口限制，我们暂时记录日志

	logger.Info("Received Feishu webhook",
		zap.Int("content_length", len(body)),
	)

	// 将事件发布到消息总线（由飞书通道解析）
	// 这里简化处理，实际应该由飞书通道解析并发布

	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok"))
}

// handleGenericWebhook 通用 webhook 处理器
func (s *Server) handleGenericWebhook(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// 从 URL 路径提取通道名称
	// /webhook/{channel}
	channelName := r.URL.Path[len("/webhook/"):]
	if channelName == "" {
		http.Error(w, "Channel not specified", http.StatusBadRequest)
		return
	}

	// 获取通道
	_, ok := s.channelMgr.Get(channelName)
	if !ok {
		http.Error(w, fmt.Sprintf("Channel %s not found", channelName), http.StatusServiceUnavailable)
		return
	}

	// 读取请求体
	body, err := io.ReadAll(r.Body)
	if err != nil {
		logger.Error("Failed to read webhook body",
			zap.String("channel", channelName),
			zap.Error(err),
		)
		http.Error(w, "Failed to read request body", http.StatusBadRequest)
		return
	}

	logger.Info("Received webhook",
		zap.String("channel", channelName),
		zap.Int("content_length", len(body)),
	)

	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok"))
}

// handleWebSocket WebSocket 连接处理器
func (s *Server) handleWebSocket(w http.ResponseWriter, r *http.Request) {
	// 检查认证
	if s.wsConfig.EnableAuth && !s.authenticateWebSocket(r) {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	// 升级到 WebSocket
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		logger.Error("Failed to upgrade to WebSocket", zap.Error(err))
		return
	}

	// 创建连接对象（仅生成连接 ID，不创建聊天会话；聊天会话由前端 sessionKey + chat.send/chat.history 触发 GetOrCreate）
	connection := NewConnection(conn, s.wsConfig)
	connectionID := connection.ID

	// 添加到连接管理
	s.addConnection(connection)

	logger.Info("WebSocket connection established",
		zap.String("connection_id", connectionID),
		zap.String("remote_addr", r.RemoteAddr),
	)

	// 发送欢迎消息（Params 中 session_id 为连接标识，与聊天 sessionKey 无关，保留字段名兼容前端）
	welcome := JSONRPCRequest{
		JSONRPC: "2.0",
		Method:  "connected",
		Params: map[string]interface{}{
			"session_id": connectionID,
			"version":    ProtocolVersion,
		},
	}
	_ = connection.SendJSON(welcome)

	// 启动心跳
	go connection.heartbeat()

	// 处理消息
	go s.handleWebSocketMessages(connection)
}

// authenticateWebSocket 验证 WebSocket 连接
func (s *Server) authenticateWebSocket(r *http.Request) bool {
	// 从查询参数获取 token
	token := r.URL.Query().Get("token")
	if token == "" {
		// 从 Authorization header 获取
		auth := r.Header.Get("Authorization")
		if auth != "" {
			// 支持 "Bearer <token>" 格式
			if len(auth) > 7 && auth[:7] == "Bearer " {
				token = auth[7:]
			}
		}
	}

	if token == "" {
		return false
	}

	// 使用恒定时间比较防止时序攻击
	return subtle.ConstantTimeCompare([]byte(token), []byte(s.authToken)) == 1
}

// handleWebSocketMessages 处理 WebSocket 消息（conn.ID 为连接 ID，与聊天 sessionKey 无关）
func (s *Server) handleWebSocketMessages(conn *Connection) {
	defer func() {
		conn.Close()
		s.removeConnection(conn.ID)
		logger.Info("WebSocket connection closed",
			zap.String("connection_id", conn.ID),
		)
	}()

	for {
		messageType, data, err := conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseNormalClosure) {
				logger.Error("WebSocket error",
					zap.String("connection_id", conn.ID),
					zap.Error(err))
			}
			break
		}

		// 只处理文本消息
		if messageType != websocket.TextMessage {
			continue
		}

		// 解析请求（支持前端 type:"req" 与 JSON-RPC 2.0）
		req, err := ParseGatewayRequest(data)
		if err != nil {
			logger.Error("Failed to parse WebSocket message",
				zap.String("connection_id", conn.ID),
				zap.Error(err))
			errorResp := NewGatewayErrorFrame("", "PARSE_ERROR", "Parse error", nil)
			_ = conn.SendJSON(errorResp)
			continue
		}

		logger.Debug("WebSocket request",
			zap.String("connection_id", conn.ID),
			zap.String("method", req.Method),
		)

		s.lastHeartbeatMs.Store(time.Now().UnixMilli())
		// 处理请求
		resp := s.handler.HandleRequest(conn.ID, req)

		// 转为前端期望的 type:"res" 帧并发送
		var frame *GatewayResponseFrame
		if resp.Error != nil {
			code := "INTERNAL_ERROR"
			switch resp.Error.Code {
			case ErrorMethodNotFound:
				code = "METHOD_NOT_FOUND"
			case ErrorInvalidParams:
				code = "INVALID_PARAMS"
			case ErrorInvalidRequest:
				code = "INVALID_REQUEST"
			}
			frame = NewGatewayErrorFrame(req.ID, code, resp.Error.Message, nil)
		} else {
			frame = NewGatewaySuccess(req.ID, resp.Result)
		}
		if err := conn.SendJSON(frame); err != nil {
			logger.Error("Failed to send WebSocket response",
				zap.String("connection_id", conn.ID),
				zap.Error(err))
		}
	}
}

// canonicalSessionKeyForBroadcast 将 ChatID 转为与 connect snapshot 一致的 sessionKey（agent_*_* -> agent:*:*），便于前端 sessionKeyMatch
func canonicalSessionKeyForBroadcast(chatID string) string {
	s := strings.TrimSpace(chatID)
	if s == "" {
		return s
	}
	// agent_<id>_<mainKey>（磁盘 safeKey）-> agent:id:mainKey，与 buildConnectSnapshot 的 mainSessionKey 一致
	if ok, _ := regexp.MatchString(`^agent_[^_]+_[^_]+$`, s); ok {
		parts := strings.SplitN(s, "_", 3) // ["agent", "main", "main"]
		if len(parts) == 3 {
			return parts[0] + ":" + parts[1] + ":" + parts[2]
		}
	}
	return s
}

// broadcastOutbound 广播出站消息到所有 WebSocket 连接
func (s *Server) broadcastOutbound(ctx context.Context) {
	logger.Info("Starting WebSocket outbound broadcaster")

	// 订阅出站消息
	subscription := s.bus.SubscribeOutbound()
	defer subscription.Unsubscribe()

	logger.Info("WebSocket broadcaster subscribed",
		zap.String("subscription_id", subscription.ID))

	busChan := subscription.Channel

	for {
		select {
		case <-ctx.Done():
			logger.Info("WebSocket outbound broadcaster stopped")
			return
		case msg, ok := <-busChan:
			if !ok {
				logger.Info("Outbound channel closed, exiting broadcaster")
				return
			}
			if msg == nil {
				continue
			}

			// 仅对 websocket 通道广播到 Control UI；其他通道由 channels.Manager 投递
			if msg.Channel != "websocket" {
				continue
			}

			s.connectionsMu.RLock()
			connCount := len(s.connections)
			s.connectionsMu.RUnlock()

			seq := s.broadcastSeq.Add(1)
			tsMs := msg.Timestamp.UnixMilli()
			if tsMs == 0 {
				tsMs = time.Now().UnixMilli()
			}
			// 与 OpenClaw 一致：runId、sessionKey、seq、state、message（含 timestamp）；sessionKey 使用规范形式 agent:id:main 便于前端匹配
			sessionKey := canonicalSessionKeyForBroadcast(msg.ChatID)

			// 根据是否为流式消息设置 state
			state := "final"
			if msg.IsStream {
				state = "delta" // 前端期望 "delta" 表示流式增量
			}

			logger.Info("Broadcasting chat event to WebSocket",
				zap.String("chat_id", msg.ChatID),
				zap.String("state", state),
				zap.Int("content_length", len(msg.Content)),
				zap.Int("connections", connCount))

			payload := map[string]interface{}{
				"runId":      msg.ID,
				"sessionKey": sessionKey,
				"seq":        seq,
				"state":      state,
				"message": map[string]interface{}{
					"role": "assistant",
					"content": []map[string]interface{}{
						{"type": "text", "text": msg.Content},
					},
					"timestamp": tsMs,
				},
			}
			eventFrame := map[string]interface{}{
				"type":    "event",
				"event":  "chat",
				"payload": payload,
				"seq":     seq,
			}
			notif, err := json.Marshal(eventFrame)
			if err != nil {
				logger.Error("Failed to marshal chat event", zap.Error(err))
				continue
			}

			s.connectionsMu.RLock()
			for _, conn := range s.connections {
				if err := conn.SendMessage(websocket.TextMessage, notif); err != nil {
					logger.Error("Failed to broadcast chat event",
						zap.String("connection_id", conn.ID),
						zap.Error(err))
				}
			}
			s.connectionsMu.RUnlock()
		}
	}
}

// Connection WebSocket 连接
type Connection struct {
	*websocket.Conn
	ID        string
	CreatedAt time.Time
	// nolint:unused
	_sessionID   string // 保留供将来使用
	pingInterval time.Duration
	pongTimeout  time.Duration
	mu           sync.Mutex
}

// NewConnection 创建连接
func NewConnection(ws *websocket.Conn, cfg *WebSocketConfig) *Connection {
	return &Connection{
		Conn:         ws,
		ID:           uuid.New().String(),
		CreatedAt:    time.Now(),
		pingInterval: cfg.PingInterval,
		pongTimeout:  cfg.PongTimeout,
	}
}

// SendJSON 发送 JSON 消息
func (c *Connection) SendJSON(v interface{}) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	// 每次写入前重新设置写入超时
	if err := c.SetWriteDeadline(time.Now().Add(10 * time.Second)); err != nil {
		return err
	}
	return c.WriteJSON(v)
}

// SendMessage 发送消息
func (c *Connection) SendMessage(messageType int, data []byte) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	// 每次写入前重新设置写入超时，避免之前的 deadline 导致超时
	if err := c.SetWriteDeadline(time.Now().Add(10 * time.Second)); err != nil {
		return err
	}
	return c.WriteMessage(messageType, data)
}

// heartbeat 心跳
func (c *Connection) heartbeat() {
	ticker := time.NewTicker(c.pingInterval)
	defer ticker.Stop()

	c.SetPongHandler(func(string) error {
		c.mu.Lock()
		defer c.mu.Unlock()
		return c.SetReadDeadline(time.Now().Add(c.pongTimeout))
	})

	for range ticker.C {
		c.mu.Lock()
		if err := c.SetWriteDeadline(time.Now().Add(10 * time.Second)); err != nil {
			c.mu.Unlock()
			return
		}
		if err := c.WriteMessage(websocket.PingMessage, nil); err != nil {
			c.mu.Unlock()
			return
		}
		c.mu.Unlock()
	}
}

// handleChannelsAPI 处理 channels API 请求
func (s *Server) handleChannelsAPI(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// 获取查询参数
	channelName := r.URL.Query().Get("channel")

	w.Header().Set("Content-Type", "application/json")

	if channelName != "" {
		// 获取特定 channel 的状态
		status, err := s.channelMgr.Status(channelName)
		if err != nil {
			w.WriteHeader(http.StatusNotFound)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"error": err.Error(),
			})
			return
		}
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(status)
	} else {
		// 获取所有 channels 列表和状态
		channelNames := s.channelMgr.List()
		channels := make([]map[string]interface{}, 0, len(channelNames))

		for _, name := range channelNames {
			status, _ := s.channelMgr.Status(name)
			channels = append(channels, status)
		}

		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"channels": channels,
			"count":    len(channels),
		})
	}
}

// Close 关闭连接
func (c *Connection) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	// 发送关闭帧
	message := websocket.FormatCloseMessage(websocket.CloseNormalClosure, "")
	_ = c.WriteMessage(websocket.CloseMessage, message)

	// 关闭连接
	return c.Conn.Close()
}
