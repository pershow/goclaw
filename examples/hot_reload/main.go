package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/smallnest/goclaw/config"
	"github.com/smallnest/goclaw/internal/logger"
	"go.uber.org/zap"
)

// 这是一个简单的示例，演示如何使用配置热重载功能
func main() {
	// 初始化日志
	if err := logger.Init("info", false); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to initialize logger: %v\n", err)
		os.Exit(1)
	}

	// 创建临时配置文件
	tmpDir := os.TempDir()
	configPath := filepath.Join(tmpDir, "goclaw-example-config.json")

	// 写入初始配置
	initialConfig := `{
  "agents": {
    "defaults": {
      "model": "openrouter:anthropic/claude-opus-4-5",
      "max_iterations": 15,
      "temperature": 1.0,
      "max_tokens": 8192
    }
  },
  "gateway": {
    "host": "localhost",
    "port": 8080,
    "websocket": {
      "host": "0.0.0.0",
      "port": 28789,
      "path": "/ws"
    }
  }
}`

	if err := os.WriteFile(configPath, []byte(initialConfig), 0644); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to write config: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Created config file: %s\n", configPath)
	fmt.Println("You can edit this file to test hot reload")
	fmt.Println()

	// 加载配置
	cfg, err := config.Load(configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to load config: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Initial config loaded:\n")
	fmt.Printf("  Gateway Port: %d\n", cfg.Gateway.Port)
	fmt.Printf("  WebSocket Port: %d\n", cfg.Gateway.WebSocket.Port)
	fmt.Printf("  Model: %s\n", cfg.Agents.Defaults.Model)
	fmt.Println()

	// 启用热重载
	if err := config.EnableHotReload(configPath); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to enable hot reload: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("✅ Hot reload enabled")
	fmt.Println()

	// 注册配置变更处理函数
	if err := config.OnConfigChange(func(oldCfg, newCfg *config.Config) error {
		fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
		fmt.Println("🔄 Configuration changed!")
		fmt.Println()

		// 检查 Gateway 端口变化
		if oldCfg.Gateway.Port != newCfg.Gateway.Port {
			fmt.Printf("  Gateway Port: %d → %d\n", oldCfg.Gateway.Port, newCfg.Gateway.Port)
		}

		// 检查 WebSocket 端口变化
		if oldCfg.Gateway.WebSocket.Port != newCfg.Gateway.WebSocket.Port {
			fmt.Printf("  WebSocket Port: %d → %d\n",
				oldCfg.Gateway.WebSocket.Port,
				newCfg.Gateway.WebSocket.Port)
		}

		// 检查模型变化
		if oldCfg.Agents.Defaults.Model != newCfg.Agents.Defaults.Model {
			fmt.Printf("  Model: %s → %s\n",
				oldCfg.Agents.Defaults.Model,
				newCfg.Agents.Defaults.Model)
		}

		// 检查温度变化
		if oldCfg.Agents.Defaults.Temperature != newCfg.Agents.Defaults.Temperature {
			fmt.Printf("  Temperature: %.1f → %.1f\n",
				oldCfg.Agents.Defaults.Temperature,
				newCfg.Agents.Defaults.Temperature)
		}

		fmt.Println()
		fmt.Println("✅ Configuration reloaded successfully")
		fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
		fmt.Println()

		return nil
	}); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to register config change handler: %v\n", err)
		os.Exit(1)
	}

	// 创建上下文用于优雅关闭
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// 处理信号
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	fmt.Println("📝 Try editing the config file to see hot reload in action:")
	fmt.Printf("   %s\n", configPath)
	fmt.Println()
	fmt.Println("Example changes:")
	fmt.Println("  - Change gateway.port from 8080 to 9090")
	fmt.Println("  - Change gateway.websocket.port from 28789 to 28790")
	fmt.Println("  - Change agents.defaults.temperature from 1.0 to 0.7")
	fmt.Println()
	fmt.Println("Press Ctrl+C to exit")
	fmt.Println()

	// 定期显示当前配置
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			fmt.Println("\nShutting down...")
			if err := config.DisableHotReload(); err != nil {
				logger.Error("Failed to disable hot reload", zap.Error(err))
			}
			// 清理临时配置文件
			os.Remove(configPath)
			return

		case <-sigChan:
			cancel()

		case <-ticker.C:
			currentCfg := config.Get()
			fmt.Printf("⏰ Current config (as of %s):\n", time.Now().Format("15:04:05"))
			fmt.Printf("   Gateway Port: %d\n", currentCfg.Gateway.Port)
			fmt.Printf("   WebSocket Port: %d\n", currentCfg.Gateway.WebSocket.Port)
			fmt.Printf("   Model: %s\n", currentCfg.Agents.Defaults.Model)
			fmt.Printf("   Temperature: %.1f\n", currentCfg.Agents.Defaults.Temperature)
			fmt.Println()
		}
	}
}
