package session

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// DefaultIdleMinutes 默认空闲分钟数（idle 模式）
const DefaultIdleMinutes = 60

// DefaultResetAtHour 默认每日重置时刻（0-23）
const DefaultResetAtHour = 4

// Media 媒体文件
type Media struct {
	Type     string `json:"type"`             // image, video, audio, document
	URL      string `json:"url"`              // 文件URL
	Base64   string `json:"base64,omitempty"` // Base64编码内容
	MimeType string `json:"mimetype"`         // MIME类型
}

// ToolCall 工具调用
type ToolCall struct {
	ID     string                 `json:"id"`
	Name   string                 `json:"name"`
	Params map[string]interface{} `json:"params"`
}

// Message 消息
type Message struct {
	Role       string                 `json:"role"` // user, assistant, system, tool
	Content    string                 `json:"content"`
	Media      []Media                `json:"media,omitempty"`
	Timestamp  time.Time              `json:"timestamp"`
	Metadata   map[string]interface{} `json:"metadata,omitempty"`
	ToolCallID string                 `json:"tool_call_id,omitempty"` // For tool role
	ToolCalls  []ToolCall             `json:"tool_calls,omitempty"`   // For assistant role
}

// Session 会话
type Session struct {
	Key       string                 `json:"key"`
	Messages  []Message              `json:"messages"`
	CreatedAt time.Time              `json:"created_at"`
	UpdatedAt time.Time              `json:"updated_at"`
	Metadata  map[string]interface{} `json:"metadata,omitempty"`
	mu        sync.RWMutex
}

// AddMessage 添加消息
func (s *Session) AddMessage(msg Message) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.Messages = append(s.Messages, msg)
	s.UpdatedAt = time.Now()
}

// GetHistory 获取历史消息
func (s *Session) GetHistory(maxMessages int) []Message {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if maxMessages <= 0 || maxMessages >= len(s.Messages) {
		// 返回所有消息的副本
		result := make([]Message, len(s.Messages))
		copy(result, s.Messages)
		return result
	}

	// 返回最近的消息
	start := len(s.Messages) - maxMessages
	result := make([]Message, maxMessages)
	copy(result, s.Messages[start:])
	return result
}

// Clear 清空消息
func (s *Session) Clear() {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.Messages = []Message{}
	s.UpdatedAt = time.Now()
}

// PatchMetadata 更新会话元数据字段（如 label, thinkingLevel, verboseLevel, reasoningLevel）
func (s *Session) PatchMetadata(updates map[string]interface{}) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.Metadata == nil {
		s.Metadata = make(map[string]interface{})
	}
	for k, v := range updates {
		if v == nil {
			delete(s.Metadata, k)
		} else {
			s.Metadata[k] = v
		}
	}
	s.UpdatedAt = time.Now()
}

// Manager 会话管理器
type Manager struct {
	sessions    map[string]*Session
	mu          sync.RWMutex
	baseDir     string
	resetPolicy *ResetPolicy // 可选：与 OpenClaw 对齐，不新鲜会话自动重置
}

// NewManager 创建会话管理器
func NewManager(baseDir string) (*Manager, error) {
	// 确保目录存在
	if err := os.MkdirAll(baseDir, 0755); err != nil {
		return nil, err
	}

	return &Manager{
		sessions: make(map[string]*Session),
		baseDir:  baseDir,
	}, nil
}

// SetResetPolicy 设置会话重置策略；之后 GetOrCreate 将按策略判定是否重置（TUI/agent 与 Gateway 同一套策略）
func (m *Manager) SetResetPolicy(policy *ResetPolicy) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.resetPolicy = policy
}

// GetOrCreate 获取或创建会话；若已通过 SetResetPolicy 设置策略则按策略判定是否重置
func (m *Manager) GetOrCreate(key string) (*Session, error) {
	m.mu.RLock()
	policy := m.resetPolicy
	m.mu.RUnlock()
	return m.GetOrCreateWithPolicy(key, policy)
}

// GetOrCreateWithPolicy 获取或创建会话；当 policy 非空且已存在会话时，若不新鲜则重置为空白会话。
func (m *Manager) GetOrCreateWithPolicy(key string, policy *ResetPolicy) (*Session, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// 检查内存缓存
	if sess, ok := m.sessions[key]; ok {
		if policy != nil && !EvaluateSessionFreshness(sess.UpdatedAt, time.Now(), *policy) {
			// 不新鲜：重置为空白会话，保留 key
			sess.Messages = []Message{}
			sess.CreatedAt = time.Now()
			sess.UpdatedAt = time.Now()
			if sess.Metadata == nil {
				sess.Metadata = make(map[string]interface{})
			}
		}
		return sess, nil
	}

	// 尝试从磁盘加载
	sess, err := m.load(key)
	if err != nil {
		if !os.IsNotExist(err) {
			return nil, err
		}
		// 文件不存在，创建新会话
		sess = &Session{
			Key:       key,
			Messages:  []Message{},
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
			Metadata:  make(map[string]interface{}),
		}
	} else if policy != nil && !EvaluateSessionFreshness(sess.UpdatedAt, time.Now(), *policy) {
		// 已加载但不新鲜：重置为空白会话
		sess.Messages = []Message{}
		sess.CreatedAt = time.Now()
		sess.UpdatedAt = time.Now()
		if sess.Metadata == nil {
			sess.Metadata = make(map[string]interface{})
		}
	}

	// 添加到缓存
	m.sessions[key] = sess
	return sess, nil
}

// Save 保存会话
func (m *Manager) Save(session *Session) error {
	session.mu.RLock()
	defer session.mu.RUnlock()

	// 确定文件路径
	filePath := m.sessionPath(session.Key)

	// 创建临时文件
	tmpPath := filePath + ".tmp"
	file, err := os.Create(tmpPath)
	if err != nil {
		return err
	}
	closeAndRename := func() error {
		if err := file.Sync(); err != nil {
			_ = file.Close()
			return err
		}
		if err := file.Close(); err != nil {
			return err
		}
		// Windows 上句柄可能延迟释放，重试几次
		var lastErr error
		for attempt := 0; attempt < 4; attempt++ {
			if attempt > 0 {
				time.Sleep(25 * time.Millisecond)
			}
			lastErr = os.Rename(tmpPath, filePath)
			if lastErr == nil {
				return nil
			}
		}
		return lastErr
	}

	// 写入元数据行
	encoder := json.NewEncoder(file)
	metadata := map[string]interface{}{
		"_type":      "metadata",
		"created_at": session.CreatedAt,
		"updated_at": session.UpdatedAt,
		"metadata":   session.Metadata,
	}
	if err := encoder.Encode(metadata); err != nil {
		_ = file.Close()
		_ = os.Remove(tmpPath)
		return err
	}

	// 写入消息
	for _, msg := range session.Messages {
		if err := encoder.Encode(msg); err != nil {
			_ = file.Close()
			_ = os.Remove(tmpPath)
			return err
		}
	}

	// 先关闭文件再重命名，避免 Windows 上“文件被占用”导致 Rename 失败
	return closeAndRename()
}

// Delete 删除会话
func (m *Manager) Delete(key string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// 从缓存中删除
	delete(m.sessions, key)

	// 删除文件
	filePath := m.sessionPath(key)
	if err := os.Remove(filePath); err != nil && !os.IsNotExist(err) {
		return err
	}

	return nil
}

// Path 返回会话存储根目录路径
func (m *Manager) Path() string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.baseDir
}

// List 列出所有会话
func (m *Manager) List() ([]string, error) {
	m.mu.RLock()
	baseDir := m.baseDir
	m.mu.RUnlock()

	entries, err := os.ReadDir(baseDir)
	if err != nil {
		return nil, err
	}

	// 恢复未完成的 .jsonl.tmp：若存在 xxx.jsonl.tmp 且不存在 xxx.jsonl，则重命名为 xxx.jsonl（进程中断或 Rename 失败时会留下 .tmp）
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !strings.HasSuffix(strings.ToLower(name), ".jsonl.tmp") {
			continue
		}
		key := name[:len(name)-len(".jsonl.tmp")]
		finalPath := filepath.Join(baseDir, key+".jsonl")
		tmpPath := filepath.Join(baseDir, name)
		if _, err := os.Stat(finalPath); err != nil && os.IsNotExist(err) {
			_ = os.Rename(tmpPath, finalPath)
		}
	}

	entries, err = os.ReadDir(baseDir)
	if err != nil {
		return nil, err
	}

	// 提取会话键（扩展名不区分大小写；只认 .jsonl）
	keys := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		ext := filepath.Ext(entry.Name())
		if !strings.EqualFold(ext, ".jsonl") {
			continue
		}
		filename := strings.TrimSuffix(entry.Name(), ext)
		// 将文件名转换回规范的 session key（agent_main_main -> agent:main:main）
		key := SafeFilenameToKey(filename)
		keys = append(keys, key)
	}

	return keys, nil
}

// load 从磁盘加载会话
func (m *Manager) load(key string) (*Session, error) {
	filePath := m.sessionPath(key)

	// 打开文件
	file, err := os.Open(filePath)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	// 创建会话
	session := &Session{
		Key:       key,
		Messages:  []Message{},
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
		Metadata:  make(map[string]interface{}),
	}

	// 解析文件
	decoder := json.NewDecoder(file)
	for decoder.More() {
		var raw map[string]interface{}
		if err := decoder.Decode(&raw); err != nil {
			return nil, err
		}

		// 检查是否为元数据行
		if msgType, ok := raw["_type"].(string); ok && msgType == "metadata" {
			if createdAt, ok := raw["created_at"].(string); ok {
				session.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
			}
			if updatedAt, ok := raw["updated_at"].(string); ok {
				session.UpdatedAt, _ = time.Parse(time.RFC3339, updatedAt)
			}
			if metadata, ok := raw["metadata"].(map[string]interface{}); ok {
				session.Metadata = metadata
			}
		} else {
			// 消息行
			data, _ := json.Marshal(raw)
			var msg Message
			if err := json.Unmarshal(data, &msg); err != nil {
				return nil, err
			}
			session.Messages = append(session.Messages, msg)
		}
	}

	return session, nil
}

// sessionPath 获取会话文件路径
func (m *Manager) sessionPath(key string) string {
	// 将 key 转换为文件系统安全的文件名
	safeKey := KeyToSafeFilename(key)
	return filepath.Join(m.baseDir, safeKey+".jsonl")
}
