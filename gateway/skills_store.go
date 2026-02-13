package gateway

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
)

// SkillOverlay 单个技能的覆盖配置（enabled / apiKey）
type SkillOverlay struct {
	Enabled bool   `json:"enabled,omitempty"`
	APIKey  string `json:"apiKey,omitempty"`
}

type skillsStore struct {
	path string
	mu   sync.RWMutex
}

func defaultSkillsOverlayPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".goclaw", "skills-overlay.json")
}

func newSkillsStore(path string) *skillsStore {
	if path == "" {
		path = defaultSkillsOverlayPath()
	}
	return &skillsStore{path: path}
}

func (s *skillsStore) Load() (map[string]SkillOverlay, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	data, err := os.ReadFile(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			return map[string]SkillOverlay{}, nil
		}
		return nil, err
	}
	var out map[string]SkillOverlay
	if err := json.Unmarshal(data, &out); err != nil {
		return nil, err
	}
	if out == nil {
		out = map[string]SkillOverlay{}
	}
	return out, nil
}

func (s *skillsStore) Save(overlays map[string]SkillOverlay) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	dir := filepath.Dir(s.path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(overlays, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.path, data, 0600)
}

func (s *skillsStore) UpdateSkill(key string, enabled *bool, apiKey *string) error {
	overlays, err := s.Load()
	if err != nil {
		return err
	}
	if overlays == nil {
		overlays = map[string]SkillOverlay{}
	}
	cur := overlays[key]
	if enabled != nil {
		cur.Enabled = *enabled
	}
	if apiKey != nil {
		cur.APIKey = *apiKey
	}
	overlays[key] = cur
	return s.Save(overlays)
}
