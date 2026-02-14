package memory

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/smallnest/goclaw/internal/logger"
	"go.uber.org/zap"
)

// SessionIndexer 会话文件索引器
type SessionIndexer struct {
	store          *SQLiteStore // 使用具体类型
	sessionDir     string
	indexedFiles   map[string]time.Time // 文件路径 -> 最后索引时间
	mu             sync.RWMutex
	stopCh         chan struct{}
	wg             sync.WaitGroup
	indexInterval  time.Duration
	retentionDays  int
}

// NewSessionIndexer 创建会话索引器
func NewSessionIndexer(store *SQLiteStore, sessionDir string, retentionDays int) *SessionIndexer {
	if retentionDays <= 0 {
		retentionDays = 30 // 默认保留 30 天
	}

	return &SessionIndexer{
		store:         store,
		sessionDir:    sessionDir,
		indexedFiles:  make(map[string]time.Time),
		stopCh:        make(chan struct{}),
		indexInterval: 5 * time.Minute,
		retentionDays: retentionDays,
	}
}

// Start 启动索引器
func (si *SessionIndexer) Start() {
	si.wg.Add(1)
	go si.indexLoop()
	logger.Info("Session indexer started",
		zap.String("session_dir", si.sessionDir),
		zap.Int("retention_days", si.retentionDays))
}

// Stop 停止索引器
func (si *SessionIndexer) Stop() {
	close(si.stopCh)
	si.wg.Wait()
	logger.Info("Session indexer stopped")
}

// indexLoop 索引循环
func (si *SessionIndexer) indexLoop() {
	defer si.wg.Done()

	// 启动时立即索引一次
	if err := si.IndexAll(); err != nil {
		logger.Error("Failed to index sessions on startup", zap.Error(err))
	}

	ticker := time.NewTicker(si.indexInterval)
	defer ticker.Stop()

	for {
		select {
		case <-si.stopCh:
			return
		case <-ticker.C:
			if err := si.IndexAll(); err != nil {
				logger.Error("Failed to index sessions", zap.Error(err))
			}
		}
	}
}

// IndexAll 索引所有会话文件
func (si *SessionIndexer) IndexAll() error {
	logger.Debug("Starting session indexing", zap.String("dir", si.sessionDir))

	// 检查目录是否存在
	if _, err := os.Stat(si.sessionDir); os.IsNotExist(err) {
		logger.Warn("Session directory does not exist", zap.String("dir", si.sessionDir))
		return nil
	}

	// 遍历会话文件
	var indexed, skipped, failed int
	cutoffTime := time.Now().AddDate(0, 0, -si.retentionDays)

	err := filepath.Walk(si.sessionDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// 跳过目录
		if info.IsDir() {
			return nil
		}

		// 只处理 .jsonl 文件
		if !strings.HasSuffix(path, ".jsonl") {
			return nil
		}

		// 检查文件是否在保留期内
		if info.ModTime().Before(cutoffTime) {
			skipped++
			return nil
		}

		// 检查是否需要重新索引
		si.mu.RLock()
		lastIndexed, exists := si.indexedFiles[path]
		si.mu.RUnlock()

		if exists && !info.ModTime().After(lastIndexed) {
			// 文件未修改，跳过
			skipped++
			return nil
		}

		// 索引文件
		if err := si.indexFile(path); err != nil {
			logger.Error("Failed to index session file",
				zap.String("file", path),
				zap.Error(err))
			failed++
			return nil // 继续处理其他文件
		}

		// 更新索引时间
		si.mu.Lock()
		si.indexedFiles[path] = time.Now()
		si.mu.Unlock()

		indexed++
		return nil
	})

	if err != nil {
		return fmt.Errorf("failed to walk session directory: %w", err)
	}

	logger.Info("Session indexing completed",
		zap.Int("indexed", indexed),
		zap.Int("skipped", skipped),
		zap.Int("failed", failed))

	return nil
}

// indexFile 索引单个会话文件
func (si *SessionIndexer) indexFile(filePath string) error {
	file, err := os.Open(filePath)
	if err != nil {
		return fmt.Errorf("failed to open file: %w", err)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	lineNum := 0

	for scanner.Scan() {
		lineNum++
		line := scanner.Text()

		// 跳过空行
		if strings.TrimSpace(line) == "" {
			continue
		}

		// 解析 JSON
		var message map[string]interface{}
		if err := json.Unmarshal([]byte(line), &message); err != nil {
			logger.Warn("Failed to parse session line",
				zap.String("file", filePath),
				zap.Int("line", lineNum),
				zap.Error(err))
			continue
		}

		// 提取内容
		content := si.extractContent(message)
		if content == "" {
			continue
		}

		// 构建元数据
		sessionKey := ""
		if sk, ok := message["session_key"].(string); ok {
			sessionKey = sk
		}

		// 添加到内存存储
		embedding := &VectorEmbedding{
			ID:        fmt.Sprintf("session_%s_%d", filepath.Base(filePath), lineNum),
			Text:      content,
			Source:    MemorySourceSession,
			Type:      MemoryTypeConversation,
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
			Metadata: MemoryMetadata{
				FilePath:   filePath,
				LineNumber: lineNum,
				SessionKey: sessionKey,
			},
		}

		if err := si.store.Add(embedding); err != nil {
			logger.Warn("Failed to add session content to memory",
				zap.String("file", filePath),
				zap.Int("line", lineNum),
				zap.Error(err))
		}
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("failed to scan file: %w", err)
	}

	return nil
}

// extractContent 从消息中提取内容
func (si *SessionIndexer) extractContent(message map[string]interface{}) string {
	// 尝试从 content 字段提取
	if content, ok := message["content"]; ok {
		switch v := content.(type) {
		case string:
			return v
		case []interface{}:
			// 处理内容块数组
			var texts []string
			for _, block := range v {
				if blockMap, ok := block.(map[string]interface{}); ok {
					if blockType, ok := blockMap["type"].(string); ok && blockType == "text" {
						if text, ok := blockMap["text"].(string); ok {
							texts = append(texts, text)
						}
					}
				}
			}
			return strings.Join(texts, "\n")
		}
	}

	return ""
}

// GetIndexedFiles 获取已索引的文件列表
func (si *SessionIndexer) GetIndexedFiles() map[string]time.Time {
	si.mu.RLock()
	defer si.mu.RUnlock()

	result := make(map[string]time.Time, len(si.indexedFiles))
	for k, v := range si.indexedFiles {
		result[k] = v
	}

	return result
}

// ClearIndex 清空索引
func (si *SessionIndexer) ClearIndex() {
	si.mu.Lock()
	defer si.mu.Unlock()

	si.indexedFiles = make(map[string]time.Time)
	logger.Info("Session index cleared")
}
