package memory

import (
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
)

// Watcher 监听目录变更，去抖后触发同步回调（与 OpenClaw 的 dirty + runSync 逻辑对齐）
type Watcher struct {
	watcher    *fsnotify.Watcher
	dir        string
	debounce   time.Duration
	onSync     func()
	timer      *time.Timer
	mu         sync.Mutex
	closed     bool
}

// NewWatcher 创建目录监听器；debounce 为事件后等待时间（如 5s），onSync 为同步回调（如增量索引）
func NewWatcher(dir string, debounce time.Duration, onSync func()) (*Watcher, error) {
	w, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}
	abs, err := filepath.Abs(dir)
	if err != nil {
		w.Close()
		return nil, err
	}
	if err := w.Add(abs); err != nil {
		w.Close()
		return nil, err
	}
	// 添加一层子目录
	entries, _ := os.ReadDir(abs)
	for _, e := range entries {
		if e.IsDir() {
			_ = w.Add(filepath.Join(abs, e.Name()))
		}
	}
	timer := time.NewTimer(0)
	if !timer.Stop() {
		<-timer.C
	}
	watcher := &Watcher{watcher: w, dir: abs, debounce: debounce, onSync: onSync, timer: timer}
	go watcher.run()
	return watcher, nil
}

func (w *Watcher) run() {
	var pending bool
	for {
		if !pending {
			select {
			case _, ok := <-w.watcher.Events:
				if !ok {
					return
				}
				pending = true
				w.timer.Reset(w.debounce)
			case _, ok := <-w.watcher.Errors:
				if !ok {
					return
				}
			}
			continue
		}
		select {
		case _, ok := <-w.watcher.Events:
			if !ok {
				return
			}
			if !w.timer.Stop() {
				<-w.timer.C
			}
			w.timer.Reset(w.debounce)
		case <-w.timer.C:
			pending = false
			w.mu.Lock()
			if w.closed {
				w.mu.Unlock()
				return
			}
			if w.onSync != nil {
				w.onSync()
			}
			w.mu.Unlock()
		case _, ok := <-w.watcher.Errors:
			if !ok {
				return
			}
		}
	}
}

// Close 停止监听
func (w *Watcher) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.closed {
		return nil
	}
	w.closed = true
	w.timer.Stop()
	return w.watcher.Close()
}
