package config

import (
	"context"
	"fmt"
	"path/filepath"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/smallnest/goclaw/internal/logger"
	"go.uber.org/zap"
)

// ChangeHandler 配置变更处理函数
type ChangeHandler func(oldCfg, newCfg *Config) error

// Watcher 配置文件监听器
type Watcher struct {
	configPath    string
	watcher       *fsnotify.Watcher
	handlers      []ChangeHandler
	mu            sync.RWMutex
	lastModTime   time.Time
	debounceDelay time.Duration
	ctx           context.Context
	cancel        context.CancelFunc
}

// NewWatcher 创建配置监听器
func NewWatcher(configPath string) (*Watcher, error) {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, fmt.Errorf("failed to create file watcher: %w", err)
	}

	// 监听配置文件所在目录（因为某些编辑器会重命名文件）
	configDir := filepath.Dir(configPath)
	if err := watcher.Add(configDir); err != nil {
		watcher.Close()
		return nil, fmt.Errorf("failed to watch config directory: %w", err)
	}

	ctx, cancel := context.WithCancel(context.Background())

	w := &Watcher{
		configPath:    configPath,
		watcher:       watcher,
		handlers:      make([]ChangeHandler, 0),
		debounceDelay: 500 * time.Millisecond, // 防抖延迟
		ctx:           ctx,
		cancel:        cancel,
	}

	logger.Info("Config watcher created",
		zap.String("config_path", configPath),
		zap.String("watch_dir", configDir))

	return w, nil
}

// OnChange 注册配置变更处理函数
func (w *Watcher) OnChange(handler ChangeHandler) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.handlers = append(w.handlers, handler)
}

// Start 启动监听
func (w *Watcher) Start() {
	go w.watch()
}

// Stop 停止监听
func (w *Watcher) Stop() error {
	w.cancel()
	return w.watcher.Close()
}

// watch 监听配置文件变化
func (w *Watcher) watch() {
	logger.Info("Config watcher started")

	// 防抖定时器
	var debounceTimer *time.Timer

	for {
		select {
		case <-w.ctx.Done():
			logger.Info("Config watcher stopped")
			return

		case event, ok := <-w.watcher.Events:
			if !ok {
				return
			}

			// 只处理配置文件的写入和创建事件
			if event.Name != w.configPath {
				continue
			}

			if event.Op&fsnotify.Write == fsnotify.Write || event.Op&fsnotify.Create == fsnotify.Create {
				logger.Debug("Config file changed",
					zap.String("event", event.Op.String()),
					zap.String("file", event.Name))

				// 防抖：重置定时器
				if debounceTimer != nil {
					debounceTimer.Stop()
				}

				debounceTimer = time.AfterFunc(w.debounceDelay, func() {
					w.handleConfigChange()
				})
			}

		case err, ok := <-w.watcher.Errors:
			if !ok {
				return
			}
			logger.Error("Config watcher error", zap.Error(err))
		}
	}
}

// handleConfigChange 处理配置变更
func (w *Watcher) handleConfigChange() {
	logger.Info("Reloading configuration", zap.String("path", w.configPath))

	// 保存旧配置
	oldCfg := Get()
	if oldCfg == nil {
		logger.Warn("No previous config to compare")
		return
	}

	// 加载新配置
	newCfg, err := Load(w.configPath)
	if err != nil {
		logger.Error("Failed to reload config", zap.Error(err))
		// 记录失败的变更
		if configHistory != nil {
			configHistory.Record(oldCfg, nil, false, err, "auto")
		}
		return
	}

	// 验证新配置
	if err := Validate(newCfg); err != nil {
		logger.Error("Invalid config after reload", zap.Error(err))
		// 记录失败的变更
		if configHistory != nil {
			configHistory.Record(oldCfg, newCfg, false, err, "auto")
		}
		return
	}

	// 调用所有处理函数
	w.mu.RLock()
	handlers := make([]ChangeHandler, len(w.handlers))
	copy(handlers, w.handlers)
	w.mu.RUnlock()

	var handlerErr error
	for i, handler := range handlers {
		if err := handler(oldCfg, newCfg); err != nil {
			logger.Error("Config change handler failed",
				zap.Int("handler_index", i),
				zap.Error(err))
			handlerErr = err
			// 继续执行其他处理函数
		}
	}

	// 记录配置变更
	if configHistory != nil {
		if err := configHistory.Record(oldCfg, newCfg, handlerErr == nil, handlerErr, "auto"); err != nil {
			logger.Error("Failed to record config change", zap.Error(err))
		}
	}

	logger.Info("Configuration reloaded successfully")
}
