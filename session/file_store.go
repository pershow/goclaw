package session

import (
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

// FileStore implements session storage using JSONL files
// Aligned with OpenClaw's session file format
type FileStore struct {
	dataDir   string
	agentID   string
	mu        sync.RWMutex
	sessions  map[string]*Session
	indexFile string
}

// SessionIndex tracks session metadata
type SessionIndex struct {
	Sessions map[string]*SessionMetadata `json:"sessions"`
}

// SessionMetadata contains session metadata
type SessionMetadata struct {
	SessionKey  string    `json:"session_key"`
	AgentID     string    `json:"agent_id"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
	MessageCount int      `json:"message_count"`
	TokenCount   int      `json:"token_count,omitempty"`
}

// NewFileStore creates a new file-based session store
func NewFileStore(dataDir, agentID string) (*FileStore, error) {
	sessionsDir := filepath.Join(dataDir, "agents", agentID, "sessions")
	if err := os.MkdirAll(sessionsDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create sessions directory: %w", err)
	}

	indexFile := filepath.Join(sessionsDir, "sessions.json")

	store := &FileStore{
		dataDir:   sessionsDir,
		agentID:   agentID,
		sessions:  make(map[string]*Session),
		indexFile: indexFile,
	}

	// Load index
	if err := store.loadIndex(); err != nil {
		logger.Warn("Failed to load session index, starting fresh",
			zap.String("agent_id", agentID),
			zap.Error(err))
	}

	return store, nil
}

// loadIndex loads the session index from disk
func (s *FileStore) loadIndex() error {
	data, err := os.ReadFile(s.indexFile)
	if err != nil {
		if os.IsNotExist(err) {
			return nil // Index doesn't exist yet
		}
		return err
	}

	var index SessionIndex
	if err := json.Unmarshal(data, &index); err != nil {
		return err
	}

	// Load sessions from index
	for key := range index.Sessions {
		// Sessions are loaded lazily when accessed
		s.sessions[key] = nil
	}

	return nil
}

// saveIndex saves the session index to disk
func (s *FileStore) saveIndex() error {
	s.mu.RLock()
	defer s.mu.RUnlock()

	index := SessionIndex{
		Sessions: make(map[string]*SessionMetadata),
	}

	for key, sess := range s.sessions {
		if sess != nil {
			index.Sessions[key] = &SessionMetadata{
				SessionKey:   key,
				AgentID:      s.agentID,
				CreatedAt:    sess.CreatedAt,
				UpdatedAt:    sess.UpdatedAt,
				MessageCount: len(sess.Messages),
			}
		}
	}

	data, err := json.MarshalIndent(index, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(s.indexFile, data, 0644)
}

// getSessionFilePath returns the file path for a session
func (s *FileStore) getSessionFilePath(sessionKey string) string {
	// Sanitize session key for filename
	filename := strings.ReplaceAll(sessionKey, ":", "_")
	filename = strings.ReplaceAll(filename, "/", "_")
	return filepath.Join(s.dataDir, filename+".jsonl")
}

// Save saves a session to disk
func (s *FileStore) Save(sess *Session) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	sess.UpdatedAt = time.Now()
	s.sessions[sess.Key] = sess

	// Write session to JSONL file
	filePath := s.getSessionFilePath(sess.Key)
	file, err := os.Create(filePath)
	if err != nil {
		return fmt.Errorf("failed to create session file: %w", err)
	}
	defer file.Close()

	encoder := json.NewEncoder(file)
	for _, msg := range sess.Messages {
		if err := encoder.Encode(msg); err != nil {
			return fmt.Errorf("failed to write message: %w", err)
		}
	}

	// Update index
	if err := s.saveIndex(); err != nil {
		logger.Warn("Failed to save session index",
			zap.String("session_key", sess.Key),
			zap.Error(err))
	}

	return nil
}

// Load loads a session from disk
func (s *FileStore) Load(sessionKey string) (*Session, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Check cache
	if sess, ok := s.sessions[sessionKey]; ok && sess != nil {
		return sess, nil
	}

	// Load from file
	filePath := s.getSessionFilePath(sessionKey)
	file, err := os.Open(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("session not found: %s", sessionKey)
		}
		return nil, fmt.Errorf("failed to open session file: %w", err)
	}
	defer file.Close()

	sess := &Session{
		Key:       sessionKey,
		Messages:  make([]Message, 0),
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	decoder := json.NewDecoder(file)
	for decoder.More() {
		var msg Message
		if err := decoder.Decode(&msg); err != nil {
			return nil, fmt.Errorf("failed to decode message: %w", err)
		}
		sess.Messages = append(sess.Messages, msg)
	}

	// Update timestamps from file info
	if info, err := file.Stat(); err == nil {
		sess.UpdatedAt = info.ModTime()
	}

	s.sessions[sessionKey] = sess
	return sess, nil
}

// Delete deletes a session
func (s *FileStore) Delete(sessionKey string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	delete(s.sessions, sessionKey)

	// Delete file
	filePath := s.getSessionFilePath(sessionKey)
	if err := os.Remove(filePath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to delete session file: %w", err)
	}

	// Update index
	if err := s.saveIndex(); err != nil {
		logger.Warn("Failed to save session index after delete",
			zap.String("session_key", sessionKey),
			zap.Error(err))
	}

	return nil
}

// List lists all session keys
func (s *FileStore) List() ([]string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	keys := make([]string, 0, len(s.sessions))
	for key := range s.sessions {
		keys = append(keys, key)
	}

	return keys, nil
}

// Exists checks if a session exists
func (s *FileStore) Exists(sessionKey string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if _, ok := s.sessions[sessionKey]; ok {
		return true
	}

	// Check file
	filePath := s.getSessionFilePath(sessionKey)
	_, err := os.Stat(filePath)
	return err == nil
}
