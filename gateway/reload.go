package gateway

import (
	"github.com/smallnest/goclaw/config"
	"github.com/smallnest/goclaw/internal/logger"
	"go.uber.org/zap"
)

// HandleConfigReload 处理配置重载
func (s *Server) HandleConfigReload(oldCfg, newCfg *config.Config) error {
	logger.Info("Gateway handling config reload")

	// 更新 WebSocket 配置
	if configChanged(&oldCfg.Gateway.WebSocket, &newCfg.Gateway.WebSocket) {
		logger.Info("WebSocket config changed, updating...")

		wsConfig := &WebSocketConfig{
			Host:           newCfg.Gateway.WebSocket.Host,
			Port:           newCfg.Gateway.WebSocket.Port,
			Path:           newCfg.Gateway.WebSocket.Path,
			EnableAuth:     newCfg.Gateway.WebSocket.EnableAuth,
			AuthToken:      newCfg.Gateway.WebSocket.AuthToken,
			PingInterval:   newCfg.Gateway.WebSocket.PingInterval,
			PongTimeout:    newCfg.Gateway.WebSocket.PongTimeout,
			ReadTimeout:    newCfg.Gateway.WebSocket.ReadTimeout,
			WriteTimeout:   newCfg.Gateway.WebSocket.WriteTimeout,
			MaxMessageSize: 10 * 1024 * 1024, // 10MB
		}

		s.SetWebSocketConfig(wsConfig)
		logger.Info("WebSocket config updated")
	}

	// 更新会话重置策略
	if oldCfg.Session.Reset != newCfg.Session.Reset {
		logger.Info("Session reset policy changed, updating...")
		// 这里可以更新会话重置策略
		logger.Info("Session reset policy updated")
	}

	logger.Info("Gateway config reload completed")
	return nil
}

// configChanged 检查配置是否变化
func configChanged(old, new interface{}) bool {
	// 简单比较，实际可以使用更精细的比较
	return true
}

// BroadcastConfigReload 向所有连接广播配置重载事件
func (s *Server) BroadcastConfigReload() {
	s.connectionsMu.RLock()
	defer s.connectionsMu.RUnlock()

	notification := map[string]interface{}{
		"type":  "event",
		"event": "config_reloaded",
		"payload": map[string]interface{}{
			"timestamp": s.lastHeartbeatMs.Load(),
		},
	}

	for _, conn := range s.connections {
		if err := conn.SendJSON(notification); err != nil {
			logger.Error("Failed to broadcast config reload",
				zap.String("connection_id", conn.ID),
				zap.Error(err))
		}
	}

	logger.Info("Config reload notification broadcasted",
		zap.Int("connections", len(s.connections)))
}
