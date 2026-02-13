package gateway

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// CronJob 与前端约定：id, schedule, sessionKey, enabled, label 等
type CronJob struct {
	ID         string `json:"id"`
	Schedule   string `json:"schedule"`
	SessionKey string `json:"sessionKey,omitempty"`
	Enabled    bool   `json:"enabled"`
	Label      string `json:"label,omitempty"`
	CreatedAt  int64  `json:"createdAt,omitempty"`
}

type cronStore struct {
	path string
	mu   sync.RWMutex
}

func defaultCronStorePath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".goclaw", "cron.json")
}

func newCronStore(path string) *cronStore {
	if path == "" {
		path = defaultCronStorePath()
	}
	return &cronStore{path: path}
}

func (c *cronStore) Load() ([]CronJob, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	data, err := os.ReadFile(c.path)
	if err != nil {
		if os.IsNotExist(err) {
			return []CronJob{}, nil
		}
		return nil, err
	}
	var out struct {
		Jobs []CronJob `json:"jobs"`
	}
	if err := json.Unmarshal(data, &out); err != nil {
		return nil, err
	}
	if out.Jobs == nil {
		out.Jobs = []CronJob{}
	}
	return out.Jobs, nil
}

func (c *cronStore) Save(jobs []CronJob) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	dir := filepath.Dir(c.path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(struct {
		Jobs []CronJob `json:"jobs"`
	}{Jobs: jobs}, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(c.path, data, 0600)
}

func (c *cronStore) Add(job CronJob) (CronJob, error) {
	jobs, err := c.Load()
	if err != nil {
		return CronJob{}, err
	}
	if job.ID == "" {
		job.ID = time.Now().Format("20060102150405") + "-" + randomShortID()
	}
	if job.CreatedAt == 0 {
		job.CreatedAt = time.Now().UnixMilli()
	}
	jobs = append(jobs, job)
	if err := c.Save(jobs); err != nil {
		return CronJob{}, err
	}
	return job, nil
}

func (c *cronStore) Update(id string, patch map[string]interface{}) error {
	jobs, err := c.Load()
	if err != nil {
		return err
	}
	for i := range jobs {
		if jobs[i].ID == id {
			if v, ok := patch["enabled"].(bool); ok {
				jobs[i].Enabled = v
			}
			if v, ok := patch["schedule"].(string); ok {
				jobs[i].Schedule = v
			}
			if v, ok := patch["sessionKey"].(string); ok {
				jobs[i].SessionKey = v
			}
			if v, ok := patch["label"].(string); ok {
				jobs[i].Label = v
			}
			return c.Save(jobs)
		}
	}
	return os.ErrNotExist
}

func (c *cronStore) Remove(id string) error {
	jobs, err := c.Load()
	if err != nil {
		return err
	}
	newJobs := make([]CronJob, 0, len(jobs))
	for _, j := range jobs {
		if j.ID != id {
			newJobs = append(newJobs, j)
		}
	}
	return c.Save(newJobs)
}

func randomShortID() string {
	b := make([]byte, 4)
	if _, err := rand.Read(b); err != nil {
		return "id"
	}
	return hex.EncodeToString(b)
}
