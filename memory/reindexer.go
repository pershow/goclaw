package memory

import (
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/smallnest/goclaw/internal/logger"
	"go.uber.org/zap"
)

// AtomicReindexer 原子重索引器
type AtomicReindexer struct {
	store         *SQLiteStore // 使用具体类型
	isReindexing  atomic.Bool
	lastReindex   atomic.Int64 // Unix timestamp in milliseconds
	reindexCount  atomic.Int64
	mu            sync.Mutex
	minInterval   time.Duration // 最小重索引间隔
}

// NewAtomicReindexer 创建原子重索引器
func NewAtomicReindexer(store *SQLiteStore) *AtomicReindexer {
	return &AtomicReindexer{
		store:       store,
		minInterval: 5 * time.Minute, // 默认最小间隔 5 分钟
	}
}

// SetMinInterval 设置最小重索引间隔
func (r *AtomicReindexer) SetMinInterval(interval time.Duration) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.minInterval = interval
}

// IsReindexing 检查是否正在重索引
func (r *AtomicReindexer) IsReindexing() bool {
	return r.isReindexing.Load()
}

// GetLastReindexTime 获取最后重索引时间
func (r *AtomicReindexer) GetLastReindexTime() time.Time {
	ms := r.lastReindex.Load()
	if ms == 0 {
		return time.Time{}
	}
	return time.UnixMilli(ms)
}

// GetReindexCount 获取重索引次数
func (r *AtomicReindexer) GetReindexCount() int64 {
	return r.reindexCount.Load()
}

// Reindex 执行原子重索引
func (r *AtomicReindexer) Reindex() error {
	// 检查是否正在重索引
	if !r.isReindexing.CompareAndSwap(false, true) {
		return fmt.Errorf("reindexing already in progress")
	}
	defer r.isReindexing.Store(false)

	// 检查最小间隔
	lastReindex := r.GetLastReindexTime()
	if !lastReindex.IsZero() && time.Since(lastReindex) < r.minInterval {
		return fmt.Errorf("reindex too frequent, minimum interval is %v", r.minInterval)
	}

	logger.Info("Starting atomic reindex")
	startTime := time.Now()

	// 执行重索引
	if err := r.performReindex(); err != nil {
		logger.Error("Atomic reindex failed", zap.Error(err))
		return fmt.Errorf("reindex failed: %w", err)
	}

	// 更新统计信息
	r.lastReindex.Store(time.Now().UnixMilli())
	r.reindexCount.Add(1)

	duration := time.Since(startTime)
	logger.Info("Atomic reindex completed",
		zap.Duration("duration", duration),
		zap.Int64("count", r.GetReindexCount()))

	return nil
}

// performReindex 执行实际的重索引操作
func (r *AtomicReindexer) performReindex() error {
	// 创建临时表
	tempTableName := fmt.Sprintf("memories_temp_%d", time.Now().UnixNano())

	// 1. 创建临时表结构
	createTableSQL := fmt.Sprintf(`
		CREATE TABLE IF NOT EXISTS %s (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			content TEXT NOT NULL,
			embedding BLOB,
			metadata TEXT,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)
	`, tempTableName)

	if _, err := r.store.db.Exec(createTableSQL); err != nil {
		return fmt.Errorf("failed to create temp table: %w", err)
	}

	// 确保清理临时表
	defer func() {
		dropSQL := fmt.Sprintf("DROP TABLE IF EXISTS %s", tempTableName)
		if _, err := r.store.db.Exec(dropSQL); err != nil {
			logger.Error("Failed to drop temp table", zap.Error(err))
		}
	}()

	// 2. 从原表读取所有数据
	rows, err := r.store.db.Query(`
		SELECT id, content, embedding, metadata, created_at, updated_at
		FROM memories
		ORDER BY id
	`)
	if err != nil {
		return fmt.Errorf("failed to read from original table: %w", err)
	}
	defer rows.Close()

	// 3. 重新生成 embedding 并写入临时表
	var processed, failed int
	for rows.Next() {
		var id int64
		var content string
		var embedding []byte
		var metadata string
		var createdAt, updatedAt string

		if err := rows.Scan(&id, &content, &embedding, &metadata, &createdAt, &updatedAt); err != nil {
			logger.Error("Failed to scan row", zap.Error(err))
			failed++
			continue
		}

		// 重新生成 embedding（如果需要）
		// 这里可以调用 embedding 服务重新生成
		// newEmbedding, err := r.store.generateEmbedding(content)

		// 插入到临时表
		insertSQL := fmt.Sprintf(`
			INSERT INTO %s (content, embedding, metadata, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?)
		`, tempTableName)

		if _, err := r.store.db.Exec(insertSQL, content, embedding, metadata, createdAt, updatedAt); err != nil {
			logger.Error("Failed to insert into temp table", zap.Error(err))
			failed++
			continue
		}

		processed++
	}

	if err := rows.Err(); err != nil {
		return fmt.Errorf("error iterating rows: %w", err)
	}

	logger.Info("Reindex data processing completed",
		zap.Int("processed", processed),
		zap.Int("failed", failed))

	// 4. 原子性地替换表
	// 使用事务确保原子性
	tx, err := r.store.db.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	// 删除旧表
	if _, err := tx.Exec("DROP TABLE IF EXISTS memories_old"); err != nil {
		return fmt.Errorf("failed to drop old backup table: %w", err)
	}

	// 重命名当前表为备份
	if _, err := tx.Exec("ALTER TABLE memories RENAME TO memories_old"); err != nil {
		return fmt.Errorf("failed to rename current table: %w", err)
	}

	// 重命名临时表为当前表
	renameSQL := fmt.Sprintf("ALTER TABLE %s RENAME TO memories", tempTableName)
	if _, err := tx.Exec(renameSQL); err != nil {
		// 回滚：恢复原表名
		tx.Exec("ALTER TABLE memories_old RENAME TO memories")
		return fmt.Errorf("failed to rename temp table: %w", err)
	}

	// 删除备份表
	if _, err := tx.Exec("DROP TABLE IF EXISTS memories_old"); err != nil {
		logger.Warn("Failed to drop old backup table", zap.Error(err))
	}

	// 提交事务
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	// 5. 重建索引
	if err := r.rebuildIndexes(); err != nil {
		logger.Warn("Failed to rebuild indexes", zap.Error(err))
	}

	return nil
}

// rebuildIndexes 重建索引
func (r *AtomicReindexer) rebuildIndexes() error {
	// 创建全文搜索索引
	if _, err := r.store.db.Exec(`
		CREATE VIRTUAL TABLE IF NOT EXISTS memories_fts USING fts5(
			content,
			content='memories',
			content_rowid='id'
		)
	`); err != nil {
		return fmt.Errorf("failed to create FTS index: %w", err)
	}

	// 重建 FTS 索引
	if _, err := r.store.db.Exec(`
		INSERT INTO memories_fts(memories_fts) VALUES('rebuild')
	`); err != nil {
		logger.Warn("Failed to rebuild FTS index", zap.Error(err))
	}

	return nil
}

// ReindexAsync 异步执行重索引
func (r *AtomicReindexer) ReindexAsync() error {
	// 检查是否正在重索引
	if r.IsReindexing() {
		return fmt.Errorf("reindexing already in progress")
	}

	go func() {
		if err := r.Reindex(); err != nil {
			logger.Error("Async reindex failed", zap.Error(err))
		}
	}()

	return nil
}

// GetStatus 获取重索引状态
func (r *AtomicReindexer) GetStatus() map[string]interface{} {
	return map[string]interface{}{
		"is_reindexing":   r.IsReindexing(),
		"last_reindex":    r.GetLastReindexTime(),
		"reindex_count":   r.GetReindexCount(),
		"min_interval_ms": r.minInterval.Milliseconds(),
	}
}
