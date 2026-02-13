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

// PendingPairRequest 待批准的配对请求
type PendingPairRequest struct {
	RequestID string `json:"requestId"`
	CreatedAt int64  `json:"createdAt"`
}

// PairedDevice 已配对设备（不存明文 token）
type PairedDevice struct {
	DeviceID  string `json:"deviceId"`
	Role      string `json:"role"`
	CreatedAt int64  `json:"createdAt"`
}

type devicesStore struct {
	path string
	mu   sync.RWMutex
}

type devicesFile struct {
	Pending []PendingPairRequest `json:"pending"`
	Paired  []PairedDevice       `json:"paired"`
}

func defaultDevicesStorePath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".goclaw", "data", "devices.json")
}

func newDevicesStore(path string) *devicesStore {
	if path == "" {
		path = defaultDevicesStorePath()
	}
	return &devicesStore{path: path}
}

func (d *devicesStore) Load() (devicesFile, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	data, err := os.ReadFile(d.path)
	if err != nil {
		if os.IsNotExist(err) {
			return devicesFile{Pending: []PendingPairRequest{}, Paired: []PairedDevice{}}, nil
		}
		return devicesFile{}, err
	}
	var f devicesFile
	if err := json.Unmarshal(data, &f); err != nil {
		return devicesFile{}, err
	}
	if f.Pending == nil {
		f.Pending = []PendingPairRequest{}
	}
	if f.Paired == nil {
		f.Paired = []PairedDevice{}
	}
	return f, nil
}

func (d *devicesStore) Save(f devicesFile) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	dir := filepath.Dir(d.path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(f, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(d.path, data, 0600)
}

func (d *devicesStore) AddPending(requestID string) error {
	f, err := d.Load()
	if err != nil {
		return err
	}
	f.Pending = append(f.Pending, PendingPairRequest{
		RequestID: requestID,
		CreatedAt: time.Now().UnixMilli(),
	})
	return d.Save(f)
}

func (d *devicesStore) Approve(requestID, deviceID, role string) (token string, err error) {
	token, err = generateToken()
	if err != nil {
		return "", err
	}
	f, err := d.Load()
	if err != nil {
		return "", err
	}
	newPending := make([]PendingPairRequest, 0, len(f.Pending))
	for _, p := range f.Pending {
		if p.RequestID != requestID {
			newPending = append(newPending, p)
		}
	}
	f.Pending = newPending
	f.Paired = append(f.Paired, PairedDevice{
		DeviceID:  deviceID,
		Role:      role,
		CreatedAt: time.Now().UnixMilli(),
	})
	if err := d.Save(f); err != nil {
		return "", err
	}
	return token, nil
}

func (d *devicesStore) Reject(requestID string) error {
	f, err := d.Load()
	if err != nil {
		return err
	}
	newPending := make([]PendingPairRequest, 0, len(f.Pending))
	for _, p := range f.Pending {
		if p.RequestID != requestID {
			newPending = append(newPending, p)
		}
	}
	f.Pending = newPending
	return d.Save(f)
}

func (d *devicesStore) Revoke(deviceID, role string) error {
	f, err := d.Load()
	if err != nil {
		return err
	}
	newPaired := make([]PairedDevice, 0, len(f.Paired))
	for _, p := range f.Paired {
		if !(p.DeviceID == deviceID && p.Role == role) {
			newPaired = append(newPaired, p)
		}
	}
	f.Paired = newPaired
	return d.Save(f)
}

func (d *devicesStore) Rotate(deviceID, role string) (token string, err error) {
	token, err = generateToken()
	if err != nil {
		return "", err
	}
	// 不更新文件：配对记录保留，仅返回新 token 给调用方
	return token, nil
}

func generateToken() (string, error) {
	b := make([]byte, 24)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return "goclaw-" + hex.EncodeToString(b), nil
}
