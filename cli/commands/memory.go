package commands

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/smallnest/goclaw/config"
	"github.com/smallnest/goclaw/internal"
	"github.com/smallnest/goclaw/memory"
	"github.com/smallnest/goclaw/memory/qmd"
	"github.com/spf13/cobra"
)

// MemoryCmd 记忆管理命令
var MemoryCmd = &cobra.Command{
	Use:   "memory",
	Short: "Manage goclaw memory",
	Long:  `View status, index, and search memory stores. Supports builtin and QMD backends.`,
}

// memoryStatusCmd 显示记忆状态
var memoryStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show memory index statistics",
	Long:  `Display statistics about the memory store including backend type, collections, and documents.`,
	Run:   runMemoryStatus,
}

// memoryIndexCmd 重新索引记忆文件
var memoryIndexCmd = &cobra.Command{
	Use:   "index",
	Short: "Reindex memory files",
	Long:  `Rebuild the memory index from configured sources.`,
	Run:   runMemoryIndex,
}

// memorySearchCmd 语义搜索记忆
var memorySearchCmd = &cobra.Command{
	Use:   "search <query>",
	Short: "Semantic search over memory",
	Long:  `Perform semantic search over stored memories using the configured backend.`,
	Args:  cobra.ExactArgs(1),
	Run:   runMemorySearch,
}

// memoryBackendCmd 查看当前后端
var memoryBackendCmd = &cobra.Command{
	Use:   "backend",
	Short: "Show current memory backend",
	Long:  `Display the current memory backend configuration.`,
	Run:   runMemoryBackend,
}

var (
	memorySearchLimit        int
	memorySearchMinScore     float64
	memorySearchJSON         bool
	memoryForceBuiltin       bool
	memoryIndexAtomic        bool
	memoryIndexWatch         bool
	memoryWatchDebounceSec   int
)

func init() {
	MemoryCmd.AddCommand(memoryStatusCmd)
	MemoryCmd.AddCommand(memoryIndexCmd)
	MemoryCmd.AddCommand(memorySearchCmd)
	MemoryCmd.AddCommand(memoryBackendCmd)

	memorySearchCmd.Flags().IntVarP(&memorySearchLimit, "limit", "n", 10, "Maximum number of results")
	memorySearchCmd.Flags().Float64Var(&memorySearchMinScore, "min-score", 0.7, "Minimum similarity score (0-1)")
	memorySearchCmd.Flags().BoolVar(&memorySearchJSON, "json", false, "Output in JSON format")

	memoryIndexCmd.Flags().BoolVar(&memoryForceBuiltin, "builtin", false, "Force using builtin backend")
	memoryIndexCmd.Flags().BoolVar(&memoryIndexAtomic, "atomic", false, "Atomic rebuild: write to temp DB then swap (like OpenClaw)")
	memoryIndexCmd.Flags().BoolVar(&memoryIndexWatch, "watch", false, "Watch workspace/memory and reindex on file changes (builtin only)")
	memoryIndexCmd.Flags().IntVar(&memoryWatchDebounceSec, "watch-debounce", 5, "Seconds to wait after last change before reindex (with --watch)")
}

// getWorkspace 获取工作区路径
func getWorkspace() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}

	cfg, err := config.Load("")
	if err != nil {
		// 使用默认工作区
		return filepath.Join(home, ".goclaw", "workspace"), nil
	}

	if cfg.Workspace.Path != "" {
		return cfg.Workspace.Path, nil
	}

	return filepath.Join(home, ".goclaw", "workspace"), nil
}

// getSearchManager 获取搜索管理器
func getSearchManager() (memory.MemorySearchManager, error) {
	workspace, err := getWorkspace()
	if err != nil {
		return nil, err
	}

	cfg, err := config.Load("")
	if err != nil {
		// 使用默认配置；启用 builtin 嵌入（API Key 可从 OPENAI_API_KEY 或配置文件读取）
		cfg = &config.Config{
			Memory: config.MemoryConfig{
				Backend: "builtin",
				Builtin: config.BuiltinMemoryConfig{
					Enabled:   true,
					Embedding: &config.BuiltinEmbeddingConfig{Provider: "openai"},
				},
			},
		}
	}

	// 如果强制使用 builtin
	if memoryForceBuiltin {
		cfg.Memory.Backend = "builtin"
	}

	return memory.GetMemorySearchManager(cfg, workspace)
}

// runMemoryStatus 执行记忆状态命令
func runMemoryStatus(cmd *cobra.Command, args []string) {
	mgr, err := getSearchManager()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to create search manager: %v\n", err)
		os.Exit(1)
	}
	defer mgr.Close()

	status := mgr.GetStatus()

	// Display status
	fmt.Println("Memory Status")
	fmt.Println("=============")

	// Backend
	if backend, ok := status["backend"].(string); ok {
		fmt.Printf("\nBackend: %s\n", backend)
	}

	// QMD specific
	if available, ok := status["available"].(bool); ok {
		if available {
			fmt.Println("  Status: Available")
		} else {
			fmt.Println("  Status: Unavailable")
		}
	}

	if collections, ok := status["collections"].([]string); ok {
		fmt.Printf("  Collections: %v\n", collections)
	}

	if indexedFiles, ok := status["indexed_files"].(int); ok {
		fmt.Printf("  Indexed Files: %d\n", indexedFiles)
	}

	if totalDocs, ok := status["total_documents"].(int); ok {
		fmt.Printf("  Total Documents: %d\n", totalDocs)
	}

	if lastUpdated, ok := status["last_updated"].(time.Time); ok && !lastUpdated.IsZero() {
		fmt.Printf("  Last Updated: %s\n", lastUpdated.Format(time.RFC3339))
	}

	if lastEmbed, ok := status["last_embed"].(time.Time); ok && !lastEmbed.IsZero() {
		fmt.Printf("  Last Embed: %s\n", lastEmbed.Format(time.RFC3339))
	}

	// Builtin specific
	if dbPath, ok := status["database_path"].(string); ok {
		fmt.Printf("\nDatabase: %s\n", dbPath)
	}

	if totalCount, ok := status["total_count"].(int); ok {
		fmt.Printf("Total Entries: %d\n", totalCount)
	}

	if sourceCounts, ok := status["source_counts"].(map[memory.MemorySource]int); ok {
		fmt.Println("\nBy Source:")
		for source, count := range sourceCounts {
			fmt.Printf("  %s: %d\n", source, count)
		}
	}

	if typeCounts, ok := status["type_counts"].(map[memory.MemoryType]int); ok {
		fmt.Println("\nBy Type:")
		for memType, count := range typeCounts {
			fmt.Printf("  %s: %d\n", memType, count)
		}
	}

	// Fallback status
	if fallbackEnabled, ok := status["fallback_enabled"].(bool); ok && fallbackEnabled {
		fmt.Println("\nNote: Running in fallback mode (builtin)")
		if fallbackStatus, ok := status["fallback_status"].(map[string]interface{}); ok {
			fmt.Println("Fallback Status:")
			for k, v := range fallbackStatus {
				fmt.Printf("  %s: %v\n", k, v)
			}
		}
	}

	// Error message
	if errMsg, ok := status["error"].(string); ok && errMsg != "" {
		fmt.Printf("\nError: %s\n", errMsg)
	}
}

// runMemoryBackend 显示当前后端
func runMemoryBackend(cmd *cobra.Command, args []string) {
	cfg, err := config.Load("")
	if err != nil {
		fmt.Printf("Backend: builtin (default)\n")
		return
	}

	backend := cfg.Memory.Backend
	if backend == "" {
		backend = "builtin"
	}

	fmt.Printf("Backend: %s\n", backend)

	if backend == "builtin" || backend == "" {
		if cfg.Memory.Builtin.Embedding != nil && cfg.Memory.Builtin.Embedding.Provider != "" {
			fmt.Printf("  Embedding: provider=%s", cfg.Memory.Builtin.Embedding.Provider)
			if cfg.Memory.Builtin.Embedding.Fallback != "" {
				fmt.Printf(", fallback=%s", cfg.Memory.Builtin.Embedding.Fallback)
			}
			fmt.Println()
		}
	}

	if backend == "qmd" {
		fmt.Printf("  QMD Command: %s\n", cfg.Memory.QMD.Command)
		fmt.Printf("  Enabled: %v\n", cfg.Memory.QMD.Enabled)
		if len(cfg.Memory.QMD.Paths) > 0 {
			fmt.Println("  Paths:")
			for _, p := range cfg.Memory.QMD.Paths {
				fmt.Printf("    - %s: %s (%s)\n", p.Name, p.Path, p.Pattern)
			}
		}
		if cfg.Memory.QMD.Sessions.Enabled {
			fmt.Printf("  Sessions Export: %s\n", cfg.Memory.QMD.Sessions.ExportDir)
		}
	}
}

// runMemoryIndex 执行记忆索引命令
func runMemoryIndex(cmd *cobra.Command, args []string) {
	workspace, err := getWorkspace()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to get workspace: %v\n", err)
		os.Exit(1)
	}

	cfg, err := config.Load("")
	if err != nil {
		cfg = &config.Config{}
	}

	// 如果强制使用 builtin 或配置为 builtin
	if memoryForceBuiltin || cfg.Memory.Backend == "builtin" || cfg.Memory.Backend == "" {
		if memoryIndexWatch {
			runBuiltinIndexWatch(workspace, cfg)
			return
		}
		runBuiltinIndex(workspace, cfg)
		return
	}

	// QMD 模式
	if cfg.Memory.Backend == "qmd" {
		runQMDIndex(workspace, cfg)
		return
	}

	fmt.Fprintf(os.Stderr, "Unknown backend: %s\n", cfg.Memory.Backend)
	os.Exit(1)
}

// buildEmbeddingProvider 从 config 创建嵌入 Provider（与 OpenClaw 一致：含 failover）；无配置时用默认 OpenAI
func buildEmbeddingProvider(cfg *config.Config) (memory.EmbeddingProvider, error) {
	if cfg != nil && cfg.Memory.Builtin.Embedding != nil && cfg.Memory.Builtin.Embedding.Provider != "" {
		return memory.NewEmbeddingProviderFromConfig(cfg, cfg.Memory.Builtin.Embedding)
	}
	apiKey := ""
	if cfg != nil {
		apiKey = cfg.Providers.OpenAI.APIKey
		if apiKey == "" {
			apiKey = cfg.Providers.OpenRouter.APIKey
		}
	}
	if apiKey == "" {
		apiKey = os.Getenv("OPENAI_API_KEY")
	}
	if apiKey == "" {
		return nil, fmt.Errorf("no embedding API key (set memory.builtin.embedding in config or OPENAI_API_KEY)")
	}
	providerCfg := memory.DefaultOpenAIConfig(apiKey)
	return memory.NewOpenAIProvider(providerCfg)
}

// populateStore 向给定 store 写入 workspace/memory 下的 MEMORY.md 与日更文件（用于原子重建或直接写入）
func populateStore(ctx context.Context, store memory.Store, provider memory.EmbeddingProvider, memoryDir string) error {
	return memory.IndexWorkspaceToStore(ctx, store, provider, memoryDir)
}

// runBuiltinIndexOnce 执行一次 builtin 索引；quiet 为 true 时仅输出简要信息（供 watch 回调使用）
func runBuiltinIndexOnce(workspace string, cfg *config.Config, quiet bool) error {
	memoryDir := filepath.Join(workspace, "memory")
	dbPath := filepath.Join(internal.GetMemoryDir(), "store.db")
	if cfg != nil && cfg.Memory.Builtin.DatabasePath != "" {
		dbPath = cfg.Memory.Builtin.DatabasePath
	}

	provider, err := buildEmbeddingProvider(cfg)
	if err != nil {
		return err
	}

	storeConfig := memory.DefaultStoreConfig(dbPath, provider)
	store, err := memory.NewSQLiteStore(storeConfig)
	if err != nil {
		return fmt.Errorf("open memory store: %w", err)
	}
	defer store.Close()

	if !quiet {
		fmt.Println("Indexing memory files (builtin backend)...")
		fmt.Printf("Workspace: %s\n", workspace)
		fmt.Printf("Database: %s\n", dbPath)
		if memoryIndexAtomic {
			fmt.Println("Using atomic rebuild (temp DB + swap)")
		}
		fmt.Println()
	}

	ctx := context.Background()

	if memoryIndexAtomic {
		if err := store.RebuildAtomic(func(tmp memory.Store) error {
			return populateStore(ctx, tmp, provider, memoryDir)
		}); err != nil {
			return fmt.Errorf("atomic rebuild: %w", err)
		}
	} else {
		managerConfig := memory.DefaultManagerConfig(store, provider)
		manager, err := memory.NewMemoryManager(managerConfig)
		if err != nil {
			return fmt.Errorf("create memory manager: %w", err)
		}
		defer manager.Close()

		longTermPath := filepath.Join(memoryDir, "MEMORY.md")
		if _, err := os.Stat(longTermPath); err == nil {
			if !quiet {
				fmt.Printf("Indexing %s...\n", longTermPath)
			}
			if err := memory.IndexFileToManager(ctx, manager, longTermPath, memory.MemorySourceLongTerm, memory.MemoryTypeFact); err != nil {
				if quiet {
					return fmt.Errorf("index %s: %w", longTermPath, err)
				}
				fmt.Fprintf(os.Stderr, "Warning: Failed to index %s: %v\n", longTermPath, err)
			} else if !quiet {
				fmt.Println("  OK")
			}
		} else if !quiet {
			fmt.Printf("No long-term memory file found (%s)\n", longTermPath)
		}

		dailyFiles, err := filepath.Glob(filepath.Join(memoryDir, "????-??-??.md"))
		if err != nil {
			if !quiet {
				fmt.Fprintf(os.Stderr, "Warning: Failed to find daily notes: %v\n", err)
			}
			return fmt.Errorf("glob daily notes: %w", err)
		}
		for _, dailyFile := range dailyFiles {
			if !quiet {
				fmt.Printf("  %s...", filepath.Base(dailyFile))
			}
			if err := memory.IndexFileToManager(ctx, manager, dailyFile, memory.MemorySourceDaily, memory.MemoryTypeContext); err != nil {
				if quiet {
					return fmt.Errorf("index %s: %w", dailyFile, err)
				}
				fmt.Fprintf(os.Stderr, "Failed: %v\n", err)
			} else if !quiet {
				fmt.Println(" OK")
			}
		}
	}

	if quiet {
		fmt.Fprintf(os.Stderr, "[memory] Reindex complete\n")
	} else {
		fmt.Println("\nIndexing complete!")
	}
	return nil
}

// runBuiltinIndex 执行 builtin 索引（失败时退出进程）
func runBuiltinIndex(workspace string, cfg *config.Config) {
	if err := runBuiltinIndexOnce(workspace, cfg, false); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		fmt.Fprintf(os.Stderr, "Set memory.builtin.embedding in config or OPENAI_API_KEY env.\n")
		os.Exit(1)
	}
}

// runBuiltinIndexWatch 首次索引后监听 workspace/memory，变更时去抖重索引（与 OpenClaw 对齐）
func runBuiltinIndexWatch(workspace string, cfg *config.Config) {
	memoryDir := filepath.Join(workspace, "memory")

	if err := runBuiltinIndexOnce(workspace, cfg, false); err != nil {
		fmt.Fprintf(os.Stderr, "Initial index failed: %v\n", err)
		os.Exit(1)
	}

	if err := os.MkdirAll(memoryDir, 0755); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to create memory dir: %v\n", err)
		os.Exit(1)
	}

	debounce := time.Duration(memoryWatchDebounceSec) * time.Second
	if debounce < time.Second {
		debounce = time.Second
	}
	watcher, err := memory.NewWatcher(memoryDir, debounce, func() {
		if err := runBuiltinIndexOnce(workspace, cfg, true); err != nil {
			fmt.Fprintf(os.Stderr, "[memory watch] reindex failed: %v\n", err)
		}
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to start memory watcher: %v\n", err)
		os.Exit(1)
	}
	defer watcher.Close()

	fmt.Printf("\nWatching %s (debounce %s). Ctrl+C to stop.\n", memoryDir, debounce)
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	<-sigChan
	fmt.Println("\nStopping memory watcher...")
}

// runQMDIndex 执行 QMD 索引
func runQMDIndex(workspace string, cfg *config.Config) {
	fmt.Println("Indexing memory files (QMD backend)...")

	// Create QMD config
	qmdCfg := cfg.Memory.QMD
	qmdMgrConfig := qmd.QMDConfig{
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
		qmdMgrConfig.Paths[i] = qmd.QMDPathConfig{
			Name:    p.Name,
			Path:    p.Path,
			Pattern: p.Pattern,
		}
	}

	qmdMgr := qmd.NewQMDManager(qmdMgrConfig, workspace, "")

	// Initialize
	ctx, cancel := context.WithTimeout(context.Background(), qmdCfg.Update.UpdateTimeout)
	defer cancel()

	if err := qmdMgr.Initialize(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to initialize QMD manager: %v\n", err)
		os.Exit(1)
	}
	defer qmdMgr.Close()

	// Update
	fmt.Println("Updating QMD collections...")
	if err := qmdMgr.Update(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: Failed to update collections: %v\n", err)
	}

	// Embed
	fmt.Println("Generating embeddings...")
	if err := qmdMgr.Embed(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: Failed to generate embeddings: %v\n", err)
	}

	// Show status
	status := qmdMgr.GetStatus()
	fmt.Println("\nIndexing complete!")
	fmt.Printf("Collections: %v\n", status.Collections)
	fmt.Printf("Indexed Files: %d\n", status.IndexedFiles)
	fmt.Printf("Total Documents: %d\n", status.TotalDocuments)
}

// runMemorySearch 执行记忆搜索命令
func runMemorySearch(cmd *cobra.Command, args []string) {
	query := args[0]

	mgr, err := getSearchManager()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to create search manager: %v\n", err)
		os.Exit(1)
	}
	defer mgr.Close()

	// Perform search
	ctx := context.Background()
	opts := memory.DefaultSearchOptions()
	opts.Limit = memorySearchLimit
	opts.MinScore = memorySearchMinScore

	results, err := mgr.Search(ctx, query, opts)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Search failed: %v\n", err)
		os.Exit(1)
	}

	if memorySearchJSON {
		outputSearchResultsJSON(query, results)
		return
	}

	outputSearchResults(query, results)
}

// outputSearchResultsJSON 输出搜索结果为 JSON
func outputSearchResultsJSON(query string, results []*memory.SearchResult) {
	data := struct {
		Query   string                 `json:"query"`
		Count   int                    `json:"count"`
		Results []*memory.SearchResult `json:"results"`
	}{
		Query:   query,
		Count:   len(results),
		Results: results,
	}

	jsonData, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to marshal JSON: %v\n", err)
		os.Exit(1)
	}

	fmt.Println(string(jsonData))
}

// outputSearchResults 输出搜索结果
func outputSearchResults(query string, results []*memory.SearchResult) {
	if len(results) == 0 {
		fmt.Printf("No results found for: %s\n", query)
		return
	}

	fmt.Printf("Search Results for: %s\n", query)
	fmt.Printf("Found %d result(s)\n\n", len(results))

	for i, result := range results {
		fmt.Printf("[%d] Score: %.2f\n", i+1, result.Score)
		if result.Source != "" {
			fmt.Printf("    Source: %s\n", result.Source)
		}
		if result.Type != "" {
			fmt.Printf("    Type: %s\n", result.Type)
		}

		if result.Metadata.FilePath != "" {
			fmt.Printf("    File: %s", result.Metadata.FilePath)
			if result.Metadata.LineNumber > 0 {
				fmt.Printf(":%d", result.Metadata.LineNumber)
			}
			fmt.Println()
		}

		if !result.CreatedAt.IsZero() {
			fmt.Printf("    Created: %s\n", result.CreatedAt.Format(time.RFC3339))
		}

		// Truncate text for display
		text := result.Text
		maxLen := 200
		if len(text) > maxLen {
			text = text[:maxLen] + "..."
		}
		fmt.Printf("    Text: %s\n\n", text)
	}
}

// Helper: builtin 索引使用 memory.IndexFileToManager / memory.IndexWorkspaceToStore（与 memory 包共用）
