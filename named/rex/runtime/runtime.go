package runtime

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/framework/memory"
	"github.com/lexcodex/relurpify/named/rex/config"
	"github.com/lexcodex/relurpify/named/rex/state"
)

type Health string

const (
	HealthHealthy    Health = "healthy"
	HealthRecovering Health = "recovering"
	HealthDegraded   Health = "degraded"
)

type WorkItem struct {
	WorkflowID string
	RunID      string
	Task       *core.Task
	State      *core.Context
	Execute    func(context.Context, WorkItem) error
}

type Details struct {
	Health         Health                     `json:"health"`
	ActiveWork     int                        `json:"active_work"`
	QueueDepth     int                        `json:"queue_depth"`
	RecoveryCount  int                        `json:"recovery_count"`
	LastWorkflowID string                     `json:"last_workflow_id,omitempty"`
	LastRunID      string                     `json:"last_run_id,omitempty"`
	LastError      string                     `json:"last_error,omitempty"`
	Recoveries     []state.RecoveryCandidate  `json:"recoveries,omitempty"`
}

// Manager coordinates long-running Nexus-managed rex work.
type Manager struct {
	cfg         config.Config
	mem         memory.MemoryStore
	queue       chan WorkItem
	mu          sync.RWMutex
	health      Health
	active      int
	queueDepth  int
	cancel      context.CancelFunc
	recoveries  []state.RecoveryCandidate
	loopStarted bool
	lastWorkflowID string
	lastRunID      string
	lastError      string
	worker         func(context.Context, WorkItem) error
}

func New(cfg config.Config, mem memory.MemoryStore) *Manager {
	return &Manager{cfg: cfg, mem: mem, queue: make(chan WorkItem, max(1, cfg.QueueCapacity)), health: HealthHealthy}
}

func (m *Manager) SetWorker(worker func(context.Context, WorkItem) error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.worker = worker
}

func (m *Manager) Start(ctx context.Context) {
	m.mu.Lock()
	if m.loopStarted {
		m.mu.Unlock()
		return
	}
	runCtx, cancel := context.WithCancel(ctx)
	m.cancel = cancel
	m.loopStarted = true
	m.mu.Unlock()
	go m.loop(runCtx)
}

func (m *Manager) Stop() {
	m.mu.Lock()
	cancel := m.cancel
	m.loopStarted = false
	m.mu.Unlock()
	if cancel != nil {
		cancel()
	}
}

func (m *Manager) Enqueue(item WorkItem) bool {
	select {
	case m.queue <- item:
		m.mu.Lock()
		m.queueDepth++
		m.mu.Unlock()
		return true
	default:
		m.setHealth(HealthDegraded)
		m.recordError(fmt.Errorf("runtime queue full"))
		return false
	}
}

func (m *Manager) Snapshot() (Health, int, []state.RecoveryCandidate) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.health, m.active, append([]state.RecoveryCandidate{}, m.recoveries...)
}

func (m *Manager) Details() Details {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return Details{
		Health:         m.health,
		ActiveWork:     m.active,
		QueueDepth:     m.queueDepth,
		RecoveryCount:  len(m.recoveries),
		LastWorkflowID: m.lastWorkflowID,
		LastRunID:      m.lastRunID,
		LastError:      m.lastError,
		Recoveries:     append([]state.RecoveryCandidate{}, m.recoveries...),
	}
}

func (m *Manager) BeginExecution(workflowID, runID string) func(error) {
	m.mu.Lock()
	m.active++
	m.lastWorkflowID = workflowID
	m.lastRunID = runID
	m.health = HealthHealthy
	m.mu.Unlock()
	return func(err error) {
		m.mu.Lock()
		defer m.mu.Unlock()
		if err != nil {
			m.lastError = err.Error()
			m.health = HealthDegraded
		} else if m.health != HealthRecovering {
			m.health = HealthHealthy
		}
		m.active--
		if m.active < 0 {
			m.active = 0
		}
	}
}

func (m *Manager) loop(ctx context.Context) {
	ticker := time.NewTicker(m.cfg.RecoveryScanPeriod)
	defer ticker.Stop()
	m.scanRecoveries(ctx)
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			m.scanRecoveries(ctx)
		case item := <-m.queue:
			m.mu.Lock()
			if m.queueDepth > 0 {
				m.queueDepth--
			}
			worker := m.worker
			m.mu.Unlock()
			finish := m.BeginExecution(item.WorkflowID, item.RunID)
			err := m.executeItem(ctx, worker, item)
			finish(err)
		}
	}
}

func (m *Manager) scanRecoveries(ctx context.Context) {
	m.setHealth(HealthRecovering)
	candidates, err := state.RecoveryBoot(ctx, m.mem)
	if err != nil {
		m.setHealth(HealthDegraded)
		return
	}
	m.mu.Lock()
	m.recoveries = candidates
	m.mu.Unlock()
	m.setHealth(HealthHealthy)
}

func (m *Manager) executeItem(ctx context.Context, worker func(context.Context, WorkItem) error, item WorkItem) error {
	if worker != nil {
		if err := worker(ctx, item); err != nil {
			m.recordError(err)
			return err
		}
		return nil
	}
	if item.Execute != nil {
		if err := item.Execute(ctx, item); err != nil {
			m.recordError(err)
			return err
		}
	}
	return nil
}

func (m *Manager) setHealth(health Health) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.health = health
}

func (m *Manager) recordError(err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if err == nil {
		return
	}
	m.lastError = err.Error()
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
