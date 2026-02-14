package agent

import (
	"context"
	"sync"
	"time"

	"github.com/smallnest/goclaw/internal/logger"
	"go.uber.org/zap"
)

// ProgressTracker tracks execution progress for real-time visibility
// Aligned with OpenClaw's progress tracking approach
type ProgressTracker struct {
	mu              sync.RWMutex
	sessionKey      string
	totalSteps      int
	completedSteps  int
	currentStep     string
	startTime       time.Time
	lastUpdateTime  time.Time
	toolsExecuted   int
	toolsTotal      int
	currentToolName string
	status          ProgressStatus
	error           error
	subscribers     []chan *ProgressUpdate
}

// ProgressStatus represents the current status
type ProgressStatus string

const (
	ProgressStatusIdle       ProgressStatus = "idle"
	ProgressStatusStarting   ProgressStatus = "starting"
	ProgressStatusProcessing ProgressStatus = "processing"
	ProgressStatusTooling    ProgressStatus = "tooling"
	ProgressStatusCompleted  ProgressStatus = "completed"
	ProgressStatusError      ProgressStatus = "error"
)

// ProgressUpdate represents a progress update
type ProgressUpdate struct {
	SessionKey      string         `json:"session_key"`
	Status          ProgressStatus `json:"status"`
	CurrentStep     string         `json:"current_step,omitempty"`
	CompletedSteps  int            `json:"completed_steps"`
	TotalSteps      int            `json:"total_steps"`
	PercentComplete float64        `json:"percent_complete"`
	ElapsedMs       int64          `json:"elapsed_ms"`
	ToolsExecuted   int            `json:"tools_executed,omitempty"`
	ToolsTotal      int            `json:"tools_total,omitempty"`
	CurrentToolName string         `json:"current_tool_name,omitempty"`
	Error           string         `json:"error,omitempty"`
	Timestamp       time.Time      `json:"timestamp"`
}

// NewProgressTracker creates a new progress tracker
func NewProgressTracker(sessionKey string) *ProgressTracker {
	return &ProgressTracker{
		sessionKey:     sessionKey,
		status:         ProgressStatusIdle,
		startTime:      time.Now(),
		lastUpdateTime: time.Now(),
		subscribers:    make([]chan *ProgressUpdate, 0),
	}
}

// Start marks the start of execution
func (p *ProgressTracker) Start(totalSteps int) {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.status = ProgressStatusStarting
	p.totalSteps = totalSteps
	p.completedSteps = 0
	p.startTime = time.Now()
	p.lastUpdateTime = time.Now()
	p.error = nil

	p.notifySubscribers()
}

// UpdateStep updates the current step
func (p *ProgressTracker) UpdateStep(step string) {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.currentStep = step
	p.status = ProgressStatusProcessing
	p.lastUpdateTime = time.Now()

	p.notifySubscribers()
}

// CompleteStep marks a step as completed
func (p *ProgressTracker) CompleteStep() {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.completedSteps++
	p.lastUpdateTime = time.Now()

	p.notifySubscribers()
}

// StartTool marks the start of tool execution
func (p *ProgressTracker) StartTool(toolName string, totalTools int) {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.status = ProgressStatusTooling
	p.currentToolName = toolName
	p.toolsTotal = totalTools
	p.lastUpdateTime = time.Now()

	p.notifySubscribers()

	logger.Debug("Tool execution started",
		zap.String("session_key", p.sessionKey),
		zap.String("tool_name", toolName),
		zap.Int("tools_total", totalTools))
}

// CompleteTool marks a tool as completed
func (p *ProgressTracker) CompleteTool(toolName string) {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.toolsExecuted++
	p.currentToolName = ""
	p.lastUpdateTime = time.Now()

	p.notifySubscribers()

	logger.Debug("Tool execution completed",
		zap.String("session_key", p.sessionKey),
		zap.String("tool_name", toolName),
		zap.Int("tools_executed", p.toolsExecuted),
		zap.Int("tools_total", p.toolsTotal))
}

// Complete marks execution as completed
func (p *ProgressTracker) Complete() {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.status = ProgressStatusCompleted
	p.completedSteps = p.totalSteps
	p.lastUpdateTime = time.Now()

	p.notifySubscribers()

	logger.Info("Execution completed",
		zap.String("session_key", p.sessionKey),
		zap.Int64("elapsed_ms", time.Since(p.startTime).Milliseconds()))
}

// Error marks execution as failed
func (p *ProgressTracker) Error(err error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.status = ProgressStatusError
	p.error = err
	p.lastUpdateTime = time.Now()

	p.notifySubscribers()

	logger.Error("Execution failed",
		zap.String("session_key", p.sessionKey),
		zap.Error(err))
}

// GetUpdate returns the current progress update
func (p *ProgressTracker) GetUpdate() *ProgressUpdate {
	p.mu.RLock()
	defer p.mu.RUnlock()

	percentComplete := 0.0
	if p.totalSteps > 0 {
		percentComplete = float64(p.completedSteps) / float64(p.totalSteps) * 100.0
	}

	update := &ProgressUpdate{
		SessionKey:      p.sessionKey,
		Status:          p.status,
		CurrentStep:     p.currentStep,
		CompletedSteps:  p.completedSteps,
		TotalSteps:      p.totalSteps,
		PercentComplete: percentComplete,
		ElapsedMs:       time.Since(p.startTime).Milliseconds(),
		ToolsExecuted:   p.toolsExecuted,
		ToolsTotal:      p.toolsTotal,
		CurrentToolName: p.currentToolName,
		Timestamp:       time.Now(),
	}

	if p.error != nil {
		update.Error = p.error.Error()
	}

	return update
}

// Subscribe subscribes to progress updates
func (p *ProgressTracker) Subscribe() <-chan *ProgressUpdate {
	p.mu.Lock()
	defer p.mu.Unlock()

	ch := make(chan *ProgressUpdate, 10)
	p.subscribers = append(p.subscribers, ch)

	// Send current state immediately
	go func() {
		ch <- p.getUpdateLocked()
	}()

	return ch
}

// Unsubscribe removes a subscriber
func (p *ProgressTracker) Unsubscribe(ch <-chan *ProgressUpdate) {
	p.mu.Lock()
	defer p.mu.Unlock()

	for i, sub := range p.subscribers {
		if sub == ch {
			close(sub)
			p.subscribers = append(p.subscribers[:i], p.subscribers[i+1:]...)
			break
		}
	}
}

// notifySubscribers sends updates to all subscribers
func (p *ProgressTracker) notifySubscribers() {
	update := p.getUpdateLocked()

	for _, ch := range p.subscribers {
		select {
		case ch <- update:
		default:
			// Skip if channel is full
			logger.Debug("Progress update channel full, skipping",
				zap.String("session_key", p.sessionKey))
		}
	}
}

// getUpdateLocked returns the current update (must be called with lock held)
func (p *ProgressTracker) getUpdateLocked() *ProgressUpdate {
	percentComplete := 0.0
	if p.totalSteps > 0 {
		percentComplete = float64(p.completedSteps) / float64(p.totalSteps) * 100.0
	}

	update := &ProgressUpdate{
		SessionKey:      p.sessionKey,
		Status:          p.status,
		CurrentStep:     p.currentStep,
		CompletedSteps:  p.completedSteps,
		TotalSteps:      p.totalSteps,
		PercentComplete: percentComplete,
		ElapsedMs:       time.Since(p.startTime).Milliseconds(),
		ToolsExecuted:   p.toolsExecuted,
		ToolsTotal:      p.toolsTotal,
		CurrentToolName: p.currentToolName,
		Timestamp:       time.Now(),
	}

	if p.error != nil {
		update.Error = p.error.Error()
	}

	return update
}

// Reset resets the progress tracker
func (p *ProgressTracker) Reset() {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.status = ProgressStatusIdle
	p.totalSteps = 0
	p.completedSteps = 0
	p.currentStep = ""
	p.toolsExecuted = 0
	p.toolsTotal = 0
	p.currentToolName = ""
	p.error = nil
	p.startTime = time.Now()
	p.lastUpdateTime = time.Now()
}

// ProgressTrackerManager manages progress trackers for multiple sessions
type ProgressTrackerManager struct {
	mu       sync.RWMutex
	trackers map[string]*ProgressTracker
}

// NewProgressTrackerManager creates a new progress tracker manager
func NewProgressTrackerManager() *ProgressTrackerManager {
	return &ProgressTrackerManager{
		trackers: make(map[string]*ProgressTracker),
	}
}

// GetOrCreate gets or creates a progress tracker for a session
func (m *ProgressTrackerManager) GetOrCreate(sessionKey string) *ProgressTracker {
	m.mu.Lock()
	defer m.mu.Unlock()

	if tracker, ok := m.trackers[sessionKey]; ok {
		return tracker
	}

	tracker := NewProgressTracker(sessionKey)
	m.trackers[sessionKey] = tracker
	return tracker
}

// Get gets a progress tracker for a session
func (m *ProgressTrackerManager) Get(sessionKey string) (*ProgressTracker, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	tracker, ok := m.trackers[sessionKey]
	return tracker, ok
}

// Remove removes a progress tracker
func (m *ProgressTrackerManager) Remove(sessionKey string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	delete(m.trackers, sessionKey)
}

// List lists all active session keys
func (m *ProgressTrackerManager) List() []string {
	m.mu.RLock()
	defer m.mu.RUnlock()

	keys := make([]string, 0, len(m.trackers))
	for key := range m.trackers {
		keys = append(keys, key)
	}
	return keys
}

// GetAllUpdates returns progress updates for all sessions
func (m *ProgressTrackerManager) GetAllUpdates() []*ProgressUpdate {
	m.mu.RLock()
	defer m.mu.RUnlock()

	updates := make([]*ProgressUpdate, 0, len(m.trackers))
	for _, tracker := range m.trackers {
		updates = append(updates, tracker.GetUpdate())
	}
	return updates
}

// CleanupIdle removes idle trackers older than the specified duration
func (m *ProgressTrackerManager) CleanupIdle(maxAge time.Duration) int {
	m.mu.Lock()
	defer m.mu.Unlock()

	now := time.Now()
	removed := 0

	for key, tracker := range m.trackers {
		tracker.mu.RLock()
		isIdle := tracker.status == ProgressStatusIdle ||
			tracker.status == ProgressStatusCompleted ||
			tracker.status == ProgressStatusError
		age := now.Sub(tracker.lastUpdateTime)
		tracker.mu.RUnlock()

		if isIdle && age > maxAge {
			delete(m.trackers, key)
			removed++
		}
	}

	return removed
}

// StartCleanupRoutine starts a background routine to cleanup idle trackers
func (m *ProgressTrackerManager) StartCleanupRoutine(ctx context.Context, interval, maxAge time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			removed := m.CleanupIdle(maxAge)
			if removed > 0 {
				logger.Debug("Cleaned up idle progress trackers",
					zap.Int("removed", removed))
			}
		}
	}
}
