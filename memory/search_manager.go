package memory

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/smallnest/goclaw/config"
	"github.com/smallnest/goclaw/internal"
	"github.com/smallnest/goclaw/memory/qmd"
)

// MemorySearchManager 统一的记忆搜索接口
type MemorySearchManager interface {
	// Search 执行语义搜索
	Search(ctx context.Context, query string, opts SearchOptions) ([]*SearchResult, error)
	// Add 添加记忆（仅 builtin 支持）
	Add(ctx context.Context, text string, source MemorySource, memType MemoryType, metadata MemoryMetadata) error
	// GetStatus 获取状态
	GetStatus() map[string]interface{}
	// Close 关闭
	Close() error
}

// BuiltinSearchManager builtin 后端实现
type BuiltinSearchManager struct {
	manager        *MemoryManager
	dbPath         string
	watcher        *Watcher
	watchStore     *SQLiteStore
	watchProvider  EmbeddingProvider
	watchMemoryDir string
}

// QMDSearchManager QMD 后端实现
type QMDSearchManager struct {
	qmdMgr      *qmd.QMDManager
	fallbackMgr MemorySearchManager // 回退到 builtin
	useFallback bool
	config      config.QMDConfig
	workspace   string
}

// NewBuiltinSearchManager 创建 builtin 搜索管理器（无嵌入 provider，仅 FTS/元数据或由调用方注入）
func NewBuiltinSearchManager(cfg config.MemoryConfig, workspace string) (MemorySearchManager, error) {
	return newBuiltinSearchManagerWithProvider(cfg, nil, workspace)
}

// NewBuiltinSearchManagerFromConfig 根据完整配置创建 builtin 搜索管理器；若 memory.builtin.embedding 已配置则创建带故障转移的嵌入 provider（与 OpenClaw 对齐）
func NewBuiltinSearchManagerFromConfig(cfg *config.Config, workspace string) (MemorySearchManager, error) {
	if cfg == nil {
		return NewBuiltinSearchManager(config.MemoryConfig{}, workspace)
	}
	mem := cfg.Memory
	var provider EmbeddingProvider
	if mem.Builtin.Embedding != nil {
		var err error
		provider, err = NewEmbeddingProviderFromConfig(cfg, mem.Builtin.Embedding)
		if err != nil {
			return nil, fmt.Errorf("memory embedding provider: %w", err)
		}
	}
	return newBuiltinSearchManagerWithProvider(mem, provider, workspace)
}

func newBuiltinSearchManagerWithProvider(cfg config.MemoryConfig, provider EmbeddingProvider, workspace string) (MemorySearchManager, error) {
	dbPath := cfg.Builtin.DatabasePath
	if dbPath == "" {
		dbPath = filepath.Join(internal.GetMemoryDir(), "store.db")
	}

	if err := os.MkdirAll(filepath.Dir(dbPath), 0755); err != nil {
		return nil, fmt.Errorf("failed to create database directory: %w", err)
	}

	storeConfig := DefaultStoreConfig(dbPath, provider)
	store, err := NewSQLiteStore(storeConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to open memory store: %w", err)
	}

	managerConfig := ManagerConfig{
		Store:        store,
		Provider:     provider,
		CacheMaxSize: 1000,
	}
	manager, err := NewMemoryManager(managerConfig)
	if err != nil {
		store.Close()
		return nil, fmt.Errorf("failed to create memory manager: %w", err)
	}

	bsm := &BuiltinSearchManager{
		manager: manager,
		dbPath:  dbPath,
	}

	// 与 OpenClaw 一致：sync.watch 默认 true，自动监听 workspace/memory，变更后去抖重索引
	syncCfg := cfg.Builtin.Sync
	watchEnabled := workspace != "" && provider != nil && (syncCfg == nil || syncCfg.Watch)
	if watchEnabled {
		memoryDir := filepath.Join(workspace, "memory")
		if err := os.MkdirAll(memoryDir, 0755); err != nil {
			_ = manager.Close()
			return nil, fmt.Errorf("create memory dir for watch: %w", err)
		}
		debounceMs := 1500
		if syncCfg != nil && syncCfg.WatchDebounceMs > 0 {
			debounceMs = syncCfg.WatchDebounceMs
		}
		debounce := time.Duration(debounceMs) * time.Millisecond
		if debounce < time.Second {
			debounce = time.Second
		}
		onSync := func() {
			if bsm.watchStore != nil && bsm.watchProvider != nil {
				_ = bsm.watchStore.RebuildAtomic(func(tmp Store) error {
					return IndexWorkspaceToStore(context.Background(), tmp, bsm.watchProvider, bsm.watchMemoryDir)
				})
			}
		}
		watcher, err := NewWatcher(memoryDir, debounce, onSync)
		if err != nil {
			_ = manager.Close()
			return nil, fmt.Errorf("memory watcher: %w", err)
		}
		bsm.watcher = watcher
		bsm.watchStore = store
		bsm.watchProvider = provider
		bsm.watchMemoryDir = memoryDir
	}

	return bsm, nil
}

// Search 执行搜索
func (m *BuiltinSearchManager) Search(ctx context.Context, query string, opts SearchOptions) ([]*SearchResult, error) {
	return m.manager.Search(ctx, query, opts)
}

// Add 添加记忆
func (m *BuiltinSearchManager) Add(ctx context.Context, text string, source MemorySource, memType MemoryType, metadata MemoryMetadata) error {
	_, err := m.manager.AddMemory(ctx, text, source, memType, metadata)
	return err
}

// GetStatus 获取状态
func (m *BuiltinSearchManager) GetStatus() map[string]interface{} {
	status := make(map[string]interface{})
	status["backend"] = "builtin"
	status["database_path"] = m.dbPath

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if stats, err := m.manager.GetStats(ctx); err == nil {
		status["total_count"] = stats.TotalCount
		status["source_counts"] = stats.SourceCounts
		status["type_counts"] = stats.TypeCounts
		status["cache_size"] = stats.CacheSize
	}

	return status
}

// Close 关闭管理器（含 watcher）
func (m *BuiltinSearchManager) Close() error {
	if m.watcher != nil {
		_ = m.watcher.Close()
		m.watcher = nil
	}
	return m.manager.Close()
}

// NewQMDSearchManager 创建 QMD 搜索管理器
func NewQMDSearchManager(qmdCfg config.QMDConfig, workspace string) (MemorySearchManager, error) {
	// 转换配置
	cfg := qmd.QMDConfig{
		Command:        qmdCfg.Command,
		Enabled:        qmdCfg.Enabled,
		IncludeDefault: qmdCfg.IncludeDefault,
		Paths:          make([]qmd.QMDPathConfig, len(qmdCfg.Paths)),
		Sessions: qmd.QMDSessionsConfig{
			Enabled:       qmdCfg.Sessions.Enabled,
			ExportDir:     qmdCfg.Sessions.ExportDir,
			RetentionDays: qmdCfg.Sessions.RetentionDays,
		},
		Update: qmd.QMDUpdateConfig{
			Interval:       qmdCfg.Update.Interval,
			OnBoot:         qmdCfg.Update.OnBoot,
			EmbedInterval:  qmdCfg.Update.EmbedInterval,
			CommandTimeout: qmdCfg.Update.CommandTimeout,
			UpdateTimeout:  qmdCfg.Update.UpdateTimeout,
		},
		Limits: qmd.QMDLimitsConfig{
			MaxResults:      qmdCfg.Limits.MaxResults,
			MaxSnippetChars: qmdCfg.Limits.MaxSnippetChars,
			TimeoutMs:       qmdCfg.Limits.TimeoutMs,
		},
	}

	for i, p := range qmdCfg.Paths {
		cfg.Paths[i] = qmd.QMDPathConfig{
			Name:    p.Name,
			Path:    p.Path,
			Pattern: p.Pattern,
		}
	}

	qmdMgr := qmd.NewQMDManager(cfg, workspace, "")

	// 尝试初始化
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := qmdMgr.Initialize(ctx); err != nil {
		// QMD 不可用，使用 fallback（无嵌入 provider）
		mgr, err := NewBuiltinSearchManager(config.MemoryConfig{
			Backend: "builtin",
			Builtin: config.BuiltinMemoryConfig{Enabled: true},
		}, workspace)
		if err != nil {
			return nil, err
		}

		return &QMDSearchManager{
			qmdMgr:      qmdMgr,
			fallbackMgr: mgr,
			useFallback: true,
			config:      qmdCfg,
			workspace:   workspace,
		}, nil
	}

	return &QMDSearchManager{
		qmdMgr:      qmdMgr,
		fallbackMgr: nil,
		useFallback: false,
		config:      qmdCfg,
		workspace:   workspace,
	}, nil
}

// Search 执行搜索
func (m *QMDSearchManager) Search(ctx context.Context, query string, opts SearchOptions) ([]*SearchResult, error) {
	if m.useFallback && m.fallbackMgr != nil {
		return m.fallbackMgr.Search(ctx, query, opts)
	}

	// 使用 QMD 搜索
	qmdResults, err := m.qmdMgr.Query(ctx, query)
	if err != nil {
		// 切换到 fallback
		if m.fallbackMgr == nil {
			m.fallbackMgr, _ = NewBuiltinSearchManager(config.MemoryConfig{
				Backend: "builtin",
				Builtin: config.BuiltinMemoryConfig{Enabled: true},
			}, m.workspace)
		}
		m.useFallback = true
		return m.fallbackMgr.Search(ctx, query, opts)
	}

	// 转换 QMD 结果为 SearchResult
	results := make([]*SearchResult, 0, len(qmdResults))
	for _, r := range qmdResults {
		result := &SearchResult{
			VectorEmbedding: VectorEmbedding{
				Text: r.Snippet,
				Metadata: MemoryMetadata{
					FilePath:   r.Path,
					LineNumber: r.Line,
				},
			},
			Score: r.Score,
		}
		results = append(results, result)
	}

	return results, nil
}

// Add 添加记忆（QMD 不支持）
func (m *QMDSearchManager) Add(ctx context.Context, text string, source MemorySource, memType MemoryType, metadata MemoryMetadata) error {
	if m.useFallback && m.fallbackMgr != nil {
		return m.fallbackMgr.Add(ctx, text, source, memType, metadata)
	}
	return fmt.Errorf("QMD backend does not support adding memories directly")
}

// GetStatus 获取状态
func (m *QMDSearchManager) GetStatus() map[string]interface{} {
	status := make(map[string]interface{})
	status["backend"] = "qmd"
	status["fallback_enabled"] = m.useFallback

	if !m.useFallback {
		qmdStatus := m.qmdMgr.GetStatus()
		status["available"] = qmdStatus.Available
		status["collections"] = qmdStatus.Collections
		status["last_updated"] = qmdStatus.LastUpdated
		status["last_embed"] = qmdStatus.LastEmbed
		status["indexed_files"] = qmdStatus.IndexedFiles
		status["total_documents"] = qmdStatus.TotalDocuments
		if qmdStatus.Error != "" {
			status["error"] = qmdStatus.Error
		}
	} else if m.fallbackMgr != nil {
		status["fallback_status"] = m.fallbackMgr.GetStatus()
	}

	return status
}

// Close 关闭管理器
func (m *QMDSearchManager) Close() error {
	var err1, err2 error
	if m.qmdMgr != nil {
		err1 = m.qmdMgr.Close()
	}
	if m.fallbackMgr != nil {
		err2 = m.fallbackMgr.Close()
	}
	if err1 != nil {
		return err1
	}
	return err2
}

// GetMemorySearchManager 根据配置创建搜索管理器。传入完整 Config 时若 backend=builtin 且配置了 embedding 则使用带故障转移的 provider。
func GetMemorySearchManager(cfg *config.Config, workspace string) (MemorySearchManager, error) {
	if cfg == nil {
		return nil, fmt.Errorf("config is required")
	}
	mem := cfg.Memory
	switch mem.Backend {
	case "qmd":
		if mem.QMD.Enabled {
			return NewQMDSearchManager(mem.QMD, workspace)
		}
		return GetBuiltinSearchManager(cfg, workspace)
	case "builtin", "":
		return GetBuiltinSearchManager(cfg, workspace)
	default:
		return nil, fmt.Errorf("unknown memory backend: %s", mem.Backend)
	}
}

// GetBuiltinSearchManager 获取 builtin 搜索管理器。若 cfg 非 nil 且 memory.builtin.embedding 已配置则创建带嵌入与故障转移的 manager。
func GetBuiltinSearchManager(cfg *config.Config, workspace string) (MemorySearchManager, error) {
	if cfg != nil && cfg.Memory.Builtin.Embedding != nil {
		return NewBuiltinSearchManagerFromConfig(cfg, workspace)
	}
	memCfg := config.MemoryConfig{}
	if cfg != nil {
		memCfg = cfg.Memory
	}
	return NewBuiltinSearchManager(memCfg, workspace)
}
