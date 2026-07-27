package state

import (
	"sync"
	"time"
)

type TaskUpdate struct {
	Status   string `json:"status"`
	Progress int    `json:"progress"`
	Error    string `json:"error,omitempty"`
}

type TaskItem struct {
	ID          string
	Status      string
	Progress    int
	AudioWAV    []byte
	AudioMP3    []byte
	Cancel      bool
	CreatedAt   time.Time
	Error       string
	subscribers map[chan TaskUpdate]struct{}
	mu          sync.Mutex
}

func (t *TaskItem) Subscribe() chan TaskUpdate {
	t.mu.Lock()
	defer t.mu.Unlock()

	ch := make(chan TaskUpdate, 10)
	if t.subscribers == nil {
		t.subscribers = make(map[chan TaskUpdate]struct{})
	}
	t.subscribers[ch] = struct{}{}
	return ch
}

func (t *TaskItem) Unsubscribe(ch chan TaskUpdate) {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.subscribers != nil {
		delete(t.subscribers, ch)
		close(ch)
	}
}

func (t *TaskItem) Notify(update TaskUpdate) {
	t.mu.Lock()
	defer t.mu.Unlock()

	t.Status = update.Status
	t.Progress = update.Progress
	if update.Error != "" {
		t.Error = update.Error
	}

	for ch := range t.subscribers {
		select {
		case ch <- update:
		default:
		}
	}
}

type TaskManager struct {
	tasks map[string]*TaskItem
	mu    sync.RWMutex
}

var GlobalTaskManager = NewTaskManager()

func NewTaskManager() *TaskManager {
	tm := &TaskManager{
		tasks: make(map[string]*TaskItem),
	}
	go tm.startCleanupRoutine()
	return tm
}

func (tm *TaskManager) GetOrCreate(taskID string) *TaskItem {
	tm.mu.Lock()
	defer tm.mu.Unlock()

	if item, ok := tm.tasks[taskID]; ok {
		return item
	}

	item := &TaskItem{
		ID:          taskID,
		Status:      "processing",
		Progress:    0,
		CreatedAt:   time.Now(),
		subscribers: make(map[chan TaskUpdate]struct{}),
	}
	tm.tasks[taskID] = item
	return item
}

func (tm *TaskManager) Get(taskID string) (*TaskItem, bool) {
	tm.mu.RLock()
	defer tm.mu.RUnlock()

	item, ok := tm.tasks[taskID]
	return item, ok
}

func (tm *TaskManager) Cancel(taskID string) bool {
	tm.mu.Lock()
	defer tm.mu.Unlock()

	if item, ok := tm.tasks[taskID]; ok {
		item.Cancel = true
		item.Notify(TaskUpdate{
			Status:   "cancelled",
			Progress: item.Progress,
		})
		return true
	}
	return false
}

func (tm *TaskManager) Cleanup() {
	tm.mu.Lock()
	defer tm.mu.Unlock()

	now := time.Now()
	for id, t := range tm.tasks {
		if now.Sub(t.CreatedAt) > 10*time.Minute {
			delete(tm.tasks, id)
		}
	}
}

func (tm *TaskManager) startCleanupRoutine() {
	ticker := time.NewTicker(5 * time.Minute)
	for range ticker.C {
		tm.Cleanup()
	}
}
