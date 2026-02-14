package process

import (
	"context"
	"sync"
	"time"

	"github.com/smallnest/goclaw/internal/logger"
	"go.uber.org/zap"
)

// CommandLane defines execution lanes for parallel processing
type CommandLane string

const (
	LaneMain       CommandLane = "main"
	LaneCron       CommandLane = "cron"
	LaneSubagent   CommandLane = "subagent"   // 与 OpenClaw CommandLane.Subagent 一致，子 agent 共用一个 lane，并发数由配置控制
	LaneAuthProbe  CommandLane = "auth-probe"
	LaneBackground CommandLane = "background"
)

// QueueEntry represents a queued task
type QueueEntry struct {
	Task        func(context.Context) (interface{}, error)
	EnqueuedAt  time.Time
	WarnAfterMs int64
	OnWait      func(waitMs int64, queuedAhead int)
}

// LaneState tracks the state of an execution lane
type LaneState struct {
	Lane           string
	Queue          []*QueueEntry
	Active         int
	ActiveTaskIDs  map[int]struct{}
	MaxConcurrent  int
	Draining       bool
	mu             sync.Mutex
}

var (
	lanes      = make(map[string]*LaneState)
	lanesMu    sync.RWMutex
	nextTaskID = 1
	taskIDMu   sync.Mutex
)

// getLaneState gets or creates a lane state
func getLaneState(lane string) *LaneState {
	lanesMu.RLock()
	existing, ok := lanes[lane]
	lanesMu.RUnlock()

	if ok {
		return existing
	}

	lanesMu.Lock()
	defer lanesMu.Unlock()

	// Double-check after acquiring write lock
	if existing, ok := lanes[lane]; ok {
		return existing
	}

	created := &LaneState{
		Lane:          lane,
		Queue:         make([]*QueueEntry, 0),
		Active:        0,
		ActiveTaskIDs: make(map[int]struct{}),
		MaxConcurrent: 1,
		Draining:      false,
	}
	lanes[lane] = created
	return created
}

// getNextTaskID generates a unique task ID
func getNextTaskID() int {
	taskIDMu.Lock()
	defer taskIDMu.Unlock()
	id := nextTaskID
	nextTaskID++
	return id
}

// drainLane processes queued tasks in a lane
func drainLane(ctx context.Context, lane string) {
	state := getLaneState(lane)

	state.mu.Lock()
	if state.Draining {
		state.mu.Unlock()
		return
	}
	state.Draining = true
	state.mu.Unlock()

	pump := func() {
		for {
			state.mu.Lock()

			// Check if we can process more tasks
			if state.Active >= state.MaxConcurrent || len(state.Queue) == 0 {
				state.Draining = false
				state.mu.Unlock()
				return
			}

			// Dequeue task
			entry := state.Queue[0]
			state.Queue = state.Queue[1:]

			waitedMs := time.Since(entry.EnqueuedAt).Milliseconds()
			if waitedMs >= entry.WarnAfterMs {
				if entry.OnWait != nil {
					entry.OnWait(waitedMs, len(state.Queue))
				}
				logger.Warn("Lane wait exceeded",
					zap.String("lane", lane),
					zap.Int64("waited_ms", waitedMs),
					zap.Int("queue_ahead", len(state.Queue)))
			}

			taskID := getNextTaskID()
			state.Active++
			state.ActiveTaskIDs[taskID] = struct{}{}
			queueLen := len(state.Queue)

			state.mu.Unlock()

			// Execute task in goroutine
			go func(entry *QueueEntry, taskID int) {
				startTime := time.Now()

				_, err := entry.Task(ctx)

				state.mu.Lock()
				state.Active--
				delete(state.ActiveTaskIDs, taskID)
				active := state.Active
				queued := len(state.Queue)
				state.mu.Unlock()

				durationMs := time.Since(startTime).Milliseconds()

				if err != nil {
					// Skip logging for probe lanes
					isProbeLane := lane == string(LaneAuthProbe) ||
						(len(lane) > 13 && lane[:13] == "session:probe")
					if !isProbeLane {
						logger.Error("Lane task error",
							zap.String("lane", lane),
							zap.Int64("duration_ms", durationMs),
							zap.Error(err))
					}
				} else {
					logger.Debug("Lane task done",
						zap.String("lane", lane),
						zap.Int64("duration_ms", durationMs),
						zap.Int("active", active),
						zap.Int("queued", queued))
				}

				// Continue draining
				drainLane(ctx, lane)
			}(entry, taskID)

			logger.Debug("Lane dequeue",
				zap.String("lane", lane),
				zap.Int64("waited_ms", waitedMs),
				zap.Int("queue_ahead", queueLen))
		}
	}

	pump()
}

// SetCommandLaneConcurrency sets the max concurrent tasks for a lane
func SetCommandLaneConcurrency(lane string, maxConcurrent int) {
	cleaned := lane
	if cleaned == "" {
		cleaned = string(LaneMain)
	}

	state := getLaneState(cleaned)
	state.mu.Lock()
	state.MaxConcurrent = max(1, maxConcurrent)
	state.mu.Unlock()

	drainLane(context.Background(), cleaned)
}

// EnqueueCommandInLane enqueues a task in a specific lane
func EnqueueCommandInLane(ctx context.Context, lane string, task func(context.Context) (interface{}, error), opts *EnqueueOptions) (interface{}, error) {
	cleaned := lane
	if cleaned == "" {
		cleaned = string(LaneMain)
	}

	warnAfterMs := int64(2000)
	if opts != nil && opts.WarnAfterMs > 0 {
		warnAfterMs = opts.WarnAfterMs
	}

	var onWait func(int64, int)
	if opts != nil {
		onWait = opts.OnWait
	}

	state := getLaneState(cleaned)

	// Create result channel
	resultChan := make(chan taskResult, 1)

	entry := &QueueEntry{
		Task: func(ctx context.Context) (interface{}, error) {
			result, err := task(ctx)
			resultChan <- taskResult{Result: result, Err: err}
			return result, err
		},
		EnqueuedAt:  time.Now(),
		WarnAfterMs: warnAfterMs,
		OnWait:      onWait,
	}

	state.mu.Lock()
	state.Queue = append(state.Queue, entry)
	queueSize := len(state.Queue) + state.Active
	state.mu.Unlock()

	logger.Debug("Lane enqueue",
		zap.String("lane", cleaned),
		zap.Int("queue_size", queueSize))

	drainLane(ctx, cleaned)

	// Wait for result
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case result := <-resultChan:
		return result.Result, result.Err
	}
}

// EnqueueCommand enqueues a task in the main lane
func EnqueueCommand(ctx context.Context, task func(context.Context) (interface{}, error), opts *EnqueueOptions) (interface{}, error) {
	return EnqueueCommandInLane(ctx, string(LaneMain), task, opts)
}

// EnqueueOptions configures task enqueueing
type EnqueueOptions struct {
	WarnAfterMs int64
	OnWait      func(waitMs int64, queuedAhead int)
}

type taskResult struct {
	Result interface{}
	Err    error
}

// GetQueueSize returns the queue size for a lane
func GetQueueSize(lane string) int {
	resolved := lane
	if resolved == "" {
		resolved = string(LaneMain)
	}

	lanesMu.RLock()
	state, ok := lanes[resolved]
	lanesMu.RUnlock()

	if !ok {
		return 0
	}

	state.mu.Lock()
	defer state.mu.Unlock()
	return len(state.Queue) + state.Active
}

// GetTotalQueueSize returns total queue size across all lanes
func GetTotalQueueSize() int {
	lanesMu.RLock()
	defer lanesMu.RUnlock()

	total := 0
	for _, state := range lanes {
		state.mu.Lock()
		total += len(state.Queue) + state.Active
		state.mu.Unlock()
	}
	return total
}

// ClearCommandLane clears all queued tasks in a lane
func ClearCommandLane(lane string) int {
	cleaned := lane
	if cleaned == "" {
		cleaned = string(LaneMain)
	}

	lanesMu.RLock()
	state, ok := lanes[cleaned]
	lanesMu.RUnlock()

	if !ok {
		return 0
	}

	state.mu.Lock()
	defer state.mu.Unlock()

	removed := len(state.Queue)
	state.Queue = make([]*QueueEntry, 0)
	return removed
}

// GetActiveTaskCount returns the number of actively executing tasks
func GetActiveTaskCount() int {
	lanesMu.RLock()
	defer lanesMu.RUnlock()

	total := 0
	for _, state := range lanes {
		state.mu.Lock()
		total += state.Active
		state.mu.Unlock()
	}
	return total
}

// WaitForActiveTasks waits for all active tasks to complete
func WaitForActiveTasks(ctx context.Context, timeoutMs int64) (bool, error) {
	const pollIntervalMs = 250
	deadline := time.Now().Add(time.Duration(timeoutMs) * time.Millisecond)

	// Collect active task IDs at start
	activeAtStart := make(map[int]struct{})
	lanesMu.RLock()
	for _, state := range lanes {
		state.mu.Lock()
		for taskID := range state.ActiveTaskIDs {
			activeAtStart[taskID] = struct{}{}
		}
		state.mu.Unlock()
	}
	lanesMu.RUnlock()

	ticker := time.NewTicker(time.Duration(pollIntervalMs) * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return false, ctx.Err()
		case <-ticker.C:
			if len(activeAtStart) == 0 {
				return true, nil
			}

			// Check if any tasks from the start are still active
			hasPending := false
			lanesMu.RLock()
			for _, state := range lanes {
				state.mu.Lock()
				for taskID := range state.ActiveTaskIDs {
					if _, exists := activeAtStart[taskID]; exists {
						hasPending = true
						state.mu.Unlock()
						break
					}
				}
				state.mu.Unlock()
				if hasPending {
					break
				}
			}
			lanesMu.RUnlock()

			if !hasPending {
				return true, nil
			}

			if time.Now().After(deadline) {
				return false, nil
			}
		}
	}
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
