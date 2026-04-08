package runtime

import (
	"context"
	"fmt"
	"strings"
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

const maxFailCount = 3

type WorkItem struct {
	WorkflowID string
	RunID      string
	Attempts   int
	Task       *core.Task
	State      *core.Context
	Execute    func(context.Context, WorkItem) error
}

type Details struct {
	Health         Health                    `json:"health"`
	ActiveWork     int                       `json:"active_work"`
	QueueDepth     int                       `json:"queue_depth"`
	RecoveryCount  int                       `json:"recovery_count"`
	Partitioned    bool                      `json:"partitioned,omitempty"`
	LastWorkflowID string                    `json:"last_workflow_id,omitempty"`
	LastRunID      string                    `json:"last_run_id,omitempty"`
	LastError      string                    `json:"last_error,omitempty"`
	DeadLetter     []WorkItem                `json:"dead_letter,omitempty"`
	Recoveries     []state.RecoveryCandidate `json:"recoveries,omitempty"`
}

type PartitionDetector interface {
	IsPartitioned() bool
}

// Manager coordinates long-running Nexus-managed rex work.
type Manager struct {
	cfg            config.Config
	mem            memory.MemoryStore
	queue          chan WorkItem
	mu             sync.RWMutex
	health         Health
	active         int
	queueDepth     int
	cancel         context.CancelFunc
	loopDone       chan struct{}
	workerWG       sync.WaitGroup
	recoveries     []state.RecoveryCandidate
	loopStarted    bool
	lastWorkflowID string
	lastRunID      string
	lastError      string
	worker         func(context.Context, WorkItem) error
	partition      PartitionDetector

	inflightMu sync.Mutex
	inflight   map[string]struct{}
	pending    map[string][]WorkItem
	failCount  map[string]int
	deadLetter []WorkItem
}

func New(cfg config.Config, mem memory.MemoryStore) *Manager {
	cfg = normalizeConfig(cfg)
	return &Manager{
		cfg:       cfg,
		mem:       mem,
		queue:     make(chan WorkItem, max(1, cfg.QueueCapacity)),
		health:    HealthHealthy,
		inflight:  map[string]struct{}{},
		pending:   map[string][]WorkItem{},
		failCount: map[string]int{},
	}
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
	m.loopDone = make(chan struct{})
	m.loopStarted = true
	m.mu.Unlock()
	go m.loop(runCtx)
}

func (m *Manager) Stop() {
	m.mu.Lock()
	cancel := m.cancel
	done := m.loopDone
	m.loopStarted = false
	m.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	if done != nil {
		<-done
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
	health := m.health
	if m.partition != nil && m.partition.IsPartitioned() {
		health = HealthDegraded
	}
	return health, m.active, append([]state.RecoveryCandidate{}, m.recoveries...)
}

func (m *Manager) Details() Details {
	m.mu.RLock()
	defer m.mu.RUnlock()
	health := m.health
	partitioned := m.partition != nil && m.partition.IsPartitioned()
	lastError := m.lastError
	if partitioned {
		health = HealthDegraded
		if lastError == "" {
			lastError = "ownership store partitioned"
		}
	}
	return Details{
		Health:         health,
		ActiveWork:     m.active,
		QueueDepth:     m.queueDepth,
		RecoveryCount:  len(m.recoveries),
		Partitioned:    partitioned,
		LastWorkflowID: m.lastWorkflowID,
		LastRunID:      m.lastRunID,
		LastError:      lastError,
		DeadLetter:     append([]WorkItem{}, m.deadLetter...),
		Recoveries:     append([]state.RecoveryCandidate{}, m.recoveries...),
	}
}

func (m *Manager) SetPartitionDetector(detector PartitionDetector) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.partition = detector
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
	defer close(m.loopDone)
	for i := 0; i < max(1, m.cfg.WorkerCount); i++ {
		m.workerWG.Add(1)
		go m.runWorker(ctx)
	}
	ticker := time.NewTicker(m.cfg.RecoveryScanPeriod)
	defer ticker.Stop()
	m.scanRecoveries(ctx)
	defer m.workerWG.Wait()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			m.scanRecoveries(ctx)
		}
	}
}

func (m *Manager) runWorker(ctx context.Context) {
	defer m.workerWG.Done()
	for {
		select {
		case <-ctx.Done():
			return
		case item := <-m.queue:
			m.mu.Lock()
			if m.queueDepth > 0 {
				m.queueDepth--
			}
			worker := m.worker
			m.mu.Unlock()

			if !m.tryAcquireWorkflow(item.WorkflowID) {
				m.addPending(item)
				continue
			}

			finish := m.BeginExecution(item.WorkflowID, item.RunID)
			err := m.executeItem(ctx, worker, item)
			finish(err)
			if err != nil {
				m.handleFailedItem(ctx, item)
			} else {
				m.clearFailureCount(item)
			}

			pending := m.releaseWorkflow(item.WorkflowID)
			m.requeuePending(ctx, pending)
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
	_ = m.DrainDeadLetter()
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

func (m *Manager) tryAcquireWorkflow(workflowID string) bool {
	if strings.TrimSpace(workflowID) == "" {
		return true
	}
	m.inflightMu.Lock()
	defer m.inflightMu.Unlock()
	if _, ok := m.inflight[workflowID]; ok {
		return false
	}
	m.inflight[workflowID] = struct{}{}
	return true
}

func (m *Manager) releaseWorkflow(workflowID string) []WorkItem {
	if strings.TrimSpace(workflowID) == "" {
		return nil
	}
	m.inflightMu.Lock()
	defer m.inflightMu.Unlock()
	delete(m.inflight, workflowID)
	items := append([]WorkItem(nil), m.pending[workflowID]...)
	delete(m.pending, workflowID)
	return items
}

func (m *Manager) addPending(item WorkItem) {
	if strings.TrimSpace(item.WorkflowID) == "" {
		return
	}
	m.inflightMu.Lock()
	defer m.inflightMu.Unlock()
	m.pending[item.WorkflowID] = append(m.pending[item.WorkflowID], item)
}

func (m *Manager) requeuePending(ctx context.Context, items []WorkItem) {
	if len(items) == 0 {
		return
	}
	items = append([]WorkItem(nil), items...)
	go func() {
		for _, item := range items {
			if !m.enqueueBlocking(ctx, item) {
				return
			}
		}
	}()
}

func (m *Manager) enqueueBlocking(ctx context.Context, item WorkItem) bool {
	select {
	case <-ctx.Done():
		return false
	case m.queue <- item:
		m.mu.Lock()
		m.queueDepth++
		m.mu.Unlock()
		return true
	}
}

func (m *Manager) handleFailedItem(ctx context.Context, item WorkItem) {
	key := item.workflowKey()
	if key == "" {
		return
	}
	item.Attempts = m.incrementFailureCount(key)
	if item.Attempts >= maxFailCount {
		m.addDeadLetter(item)
		return
	}
	delay := retryDelay(item.Attempts)
	go func() {
		timer := time.NewTimer(delay)
		defer timer.Stop()
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
			_ = m.enqueueBlocking(ctx, item)
		}
	}()
}

func (m *Manager) incrementFailureCount(key string) int {
	m.inflightMu.Lock()
	defer m.inflightMu.Unlock()
	m.failCount[key]++
	return m.failCount[key]
}

func (m *Manager) clearFailureCount(item WorkItem) {
	key := item.workflowKey()
	if key == "" {
		return
	}
	m.inflightMu.Lock()
	defer m.inflightMu.Unlock()
	delete(m.failCount, key)
}

func (m *Manager) addDeadLetter(item WorkItem) {
	m.inflightMu.Lock()
	defer m.inflightMu.Unlock()
	m.deadLetter = append(m.deadLetter, item)
}

func (m *Manager) DrainDeadLetter() []WorkItem {
	m.inflightMu.Lock()
	defer m.inflightMu.Unlock()
	return append([]WorkItem(nil), m.deadLetter...)
}

func (item WorkItem) workflowKey() string {
	workflowID := strings.TrimSpace(item.WorkflowID)
	runID := strings.TrimSpace(item.RunID)
	if workflowID == "" {
		return ""
	}
	if runID == "" {
		return workflowID
	}
	return workflowID + ":" + runID
}

func retryDelay(attempts int) time.Duration {
	if attempts < 1 {
		attempts = 1
	}
	return time.Duration(20+attempts*10) * time.Millisecond
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

func normalizeConfig(cfg config.Config) config.Config {
	if strings.TrimSpace(string(cfg.RuntimeMode)) == "" {
		cfg.RuntimeMode = config.RuntimeModeNexusManaged
	}
	if cfg.QueueCapacity <= 0 {
		cfg.QueueCapacity = 32
	}
	if cfg.WorkerCount <= 0 {
		if cfg.RuntimeMode == config.RuntimeModeEmbedded {
			cfg.WorkerCount = 1
		} else {
			cfg.WorkerCount = 4
		}
	}
	if cfg.RecoveryScanPeriod <= 0 {
		cfg.RecoveryScanPeriod = 30 * time.Second
	}
	if cfg.IdlePollPeriod <= 0 {
		cfg.IdlePollPeriod = 200 * time.Millisecond
	}
	return cfg
}
