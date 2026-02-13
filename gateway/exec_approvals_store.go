package gateway

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
)

// ExecApprovalsFile 与前端约定：path, exists, hash, file (content)
type ExecApprovalsFile struct {
	Path   string                 `json:"path"`
	Exists bool                   `json:"exists"`
	Hash   string                 `json:"hash"`
	File   map[string]interface{} `json:"file"`
}

type execApprovalsStore struct {
	path string
	mu   sync.RWMutex
}

func defaultExecApprovalsPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".goclaw", "exec-approvals.json")
}

func newExecApprovalsStore(path string) *execApprovalsStore {
	if path == "" {
		path = defaultExecApprovalsPath()
	}
	return &execApprovalsStore{path: path}
}

func (e *execApprovalsStore) Load() (ExecApprovalsFile, error) {
	e.mu.RLock()
	defer e.mu.RUnlock()

	data, err := os.ReadFile(e.path)
	if err != nil {
		if os.IsNotExist(err) {
			return ExecApprovalsFile{
				Path:   e.path,
				Exists: false,
				Hash:   "",
				File:   map[string]interface{}{},
			}, nil
		}
		return ExecApprovalsFile{}, err
	}
	var f ExecApprovalsFile
	if err := json.Unmarshal(data, &f); err != nil {
		return ExecApprovalsFile{}, err
	}
	f.Path = e.path
	f.Exists = true
	if f.File == nil {
		f.File = map[string]interface{}{}
	}
	return f, nil
}

func (e *execApprovalsStore) Save(f ExecApprovalsFile) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	dir := filepath.Dir(e.path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(f, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(e.path, data, 0600)
}
