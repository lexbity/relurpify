package projections

import (
	"context"
	"encoding/json"
	"strconv"
	"strings"
	"sync"
	"time"

	archaeoarch "github.com/lexcodex/relurpify/archaeo/archaeology"
	archaeodomain "github.com/lexcodex/relurpify/archaeo/domain"
	archaeoevents "github.com/lexcodex/relurpify/archaeo/events"
	archaeolearning "github.com/lexcodex/relurpify/archaeo/learning"
	archaeophases "github.com/lexcodex/relurpify/archaeo/phases"
	archaeoplans "github.com/lexcodex/relurpify/archaeo/plans"
	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/framework/event"
	"github.com/lexcodex/relurpify/framework/memory"
)

type WorkflowReadModel struct {
	WorkflowID           string                              `json:"workflow_id"`
	PhaseState           *archaeodomain.WorkflowPhaseState   `json:"phase_state,omitempty"`
	ActiveExploration    *archaeodomain.ExplorationSession   `json:"active_exploration,omitempty"`
	ExplorationSnapshots []archaeodomain.ExplorationSnapshot `json:"exploration_snapshots,omitempty"`
	PendingLearning      []archaeolearning.Interaction       `json:"pending_learning,omitempty"`
	PendingGuidanceIDs   []string                            `json:"pending_guidance_ids,omitempty"`
	ActivePlanVersion    *archaeodomain.VersionedLivingPlan  `json:"active_plan_version,omitempty"`
	ConvergenceState     *archaeodomain.ConvergenceState     `json:"convergence_state,omitempty"`
	TensionSummary       *archaeodomain.TensionSummary       `json:"tension_summary,omitempty"`
	Timeline             []archaeodomain.TimelineEvent       `json:"timeline,omitempty"`
	LastEventSeq         uint64                              `json:"last_event_seq,omitempty"`
}

type workflowEventPayload struct {
	EventID    string         `json:"event_id,omitempty"`
	WorkflowID string         `json:"workflow_id,omitempty"`
	RunID      string         `json:"run_id,omitempty"`
	StepID     string         `json:"step_id,omitempty"`
	Message    string         `json:"message,omitempty"`
	Metadata   map[string]any `json:"metadata,omitempty"`
}

type ProjectionEventType string

const (
	ProjectionEventSnapshot ProjectionEventType = "snapshot"
)

type ProjectionEvent struct {
	Type          ProjectionEventType
	WorkflowID    string
	Workflow      *WorkflowReadModel
	Exploration   *ExplorationProjection
	LearningQueue *LearningQueueProjection
	ActivePlan    *ActivePlanProjection
	Timeline      *TimelineProjection
}

type Service struct {
	Store        memory.WorkflowStateStore
	Now          func() time.Time
	PollInterval time.Duration

	mu   sync.Mutex
	subs map[int]projectionSubscription
	seq  int
}

type projectionSubscription struct {
	workflowID string
	ch         chan ProjectionEvent
	cancel     context.CancelFunc
}

func (s *Service) Workflow(ctx context.Context, workflowID string) (*WorkflowReadModel, error) {
	workflowID = strings.TrimSpace(workflowID)
	if s == nil || s.Store == nil || workflowID == "" {
		return nil, nil
	}
	mat := &CompositeMaterializer{WorkflowID: workflowID, Service: s}
	if err := s.runMaterializer(ctx, workflowID, projectionSnapshotKey("workflow"), mat); err != nil {
		return nil, err
	}
	model := mat.Model
	if strings.TrimSpace(model.WorkflowID) == "" {
		model.WorkflowID = workflowID
	}
	return &model, nil
}

func (s *Service) buildWorkflow(ctx context.Context, workflowID string) (*WorkflowReadModel, error) {
	workflowID = strings.TrimSpace(workflowID)
	if s == nil || s.Store == nil || workflowID == "" {
		return nil, nil
	}
	phaseMat := &PhaseStateMaterializer{Store: s.Store, WorkflowID: workflowID}
	if err := s.runMaterializer(ctx, workflowID, projectionSnapshotKey("phase"), phaseMat); err != nil {
		return nil, err
	}
	explorationView, err := (archaeoarch.Service{
		Store:    s.Store,
		Learning: archaeolearning.Service{Store: s.Store},
	}).LoadExplorationViewByWorkflow(ctx, workflowID)
	if err != nil {
		return nil, err
	}
	pendingLearning, err := (archaeolearning.Service{Store: s.Store}).Pending(ctx, workflowID)
	if err != nil {
		return nil, err
	}
	var pendingGuidance []string
	if phase, ok, err := (archaeophases.Service{Store: s.Store}).Load(ctx, workflowID); err != nil {
		return nil, err
	} else if ok {
		if phaseMat.Projection == nil {
			phaseMat.Projection = phase
		}
		pendingGuidance = append([]string(nil), phase.PendingGuidance...)
	}
	activePlan, err := (archaeoplans.Service{WorkflowStore: s.Store}).LoadActiveVersion(ctx, workflowID)
	if err != nil {
		return nil, err
	}
	timelineMat := &TimelineMaterializer{Store: s.Store, WorkflowID: workflowID}
	timeline, lastSeq, err := timelineMat.build(ctx)
	if err != nil {
		return nil, err
	}
	model := &WorkflowReadModel{WorkflowID: workflowID}
	if phaseMat.Projection != nil {
		model.PhaseState = phaseMat.Projection
	}
	if explorationView != nil {
		model.ActiveExploration = explorationView.Session
		model.ExplorationSnapshots = append([]archaeodomain.ExplorationSnapshot(nil), explorationView.Snapshots...)
		model.TensionSummary = explorationView.TensionSummary
	}
	model.PendingLearning = append([]archaeolearning.Interaction(nil), pendingLearning...)
	model.PendingGuidanceIDs = pendingGuidance
	model.ActivePlanVersion = activePlan
	model.ConvergenceState = convergenceFromTimeline(timeline)
	model.Timeline = append([]archaeodomain.TimelineEvent(nil), timeline...)
	model.LastEventSeq = lastSeq
	return model, nil
}

func projectionSnapshotKey(name string) string {
	return "projection-" + strings.TrimSpace(name)
}

func projectionPartition(workflowID, snapshotKey string) string {
	workflowID = strings.TrimSpace(workflowID)
	snapshotKey = strings.TrimSpace(snapshotKey)
	if snapshotKey == "" {
		return workflowID
	}
	return workflowID + "::snapshot:" + snapshotKey
}

func (s *Service) Exploration(ctx context.Context, workflowID string) (*ExplorationProjection, error) {
	workflowID = strings.TrimSpace(workflowID)
	mat := &ExplorationMaterializer{Store: s.Store, WorkflowID: workflowID}
	if err := s.runMaterializer(ctx, workflowID, projectionSnapshotKey("exploration"), mat); err != nil {
		return nil, err
	}
	return mat.Projection, nil
}

func (s *Service) LearningQueue(ctx context.Context, workflowID string) (*LearningQueueProjection, error) {
	workflowID = strings.TrimSpace(workflowID)
	mat := &LearningQueueMaterializer{Store: s.Store, WorkflowID: workflowID}
	if err := s.runMaterializer(ctx, workflowID, projectionSnapshotKey("learning-queue"), mat); err != nil {
		return nil, err
	}
	return mat.Projection, nil
}

func (s *Service) ActivePlan(ctx context.Context, workflowID string) (*ActivePlanProjection, error) {
	workflowID = strings.TrimSpace(workflowID)
	mat := &ActivePlanMaterializer{Store: s.Store, WorkflowID: workflowID}
	if err := s.runMaterializer(ctx, workflowID, projectionSnapshotKey("active-plan"), mat); err != nil {
		return nil, err
	}
	return mat.Projection, nil
}

func (s *Service) Timeline(ctx context.Context, workflowID string) ([]archaeodomain.TimelineEvent, error) {
	proj, err := s.TimelineProjection(ctx, workflowID)
	if err != nil || proj == nil {
		return nil, err
	}
	return append([]archaeodomain.TimelineEvent(nil), proj.Timeline...), nil
}

func (s *Service) TimelineProjection(ctx context.Context, workflowID string) (*TimelineProjection, error) {
	workflowID = strings.TrimSpace(workflowID)
	mat := &TimelineMaterializer{Store: s.Store, WorkflowID: workflowID}
	if err := s.runMaterializer(ctx, workflowID, projectionSnapshotKey("timeline"), mat); err != nil {
		return nil, err
	}
	return mat.Projection, nil
}

func (s *Service) SubscribeWorkflow(workflowID string, buffer int) (<-chan ProjectionEvent, func()) {
	if s == nil || s.Store == nil || strings.TrimSpace(workflowID) == "" {
		ch := make(chan ProjectionEvent)
		close(ch)
		return ch, func() {}
	}
	if buffer <= 0 {
		buffer = 16
	}
	ctx, cancel := context.WithCancel(context.Background())
	ch := make(chan ProjectionEvent, buffer)
	id := s.addSub(strings.TrimSpace(workflowID), ch, cancel)
	go s.runSubscription(ctx, strings.TrimSpace(workflowID), ch)
	unsub := func() {
		s.removeSub(id)
		cancel()
	}
	return ch, unsub
}

func (s *Service) runSubscription(ctx context.Context, workflowID string, ch chan ProjectionEvent) {
	defer close(ch)
	interval := s.pollInterval()
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	var lastSeq uint64
	for {
		model, err := s.Workflow(ctx, workflowID)
		if err == nil && model != nil && model.LastEventSeq != lastSeq {
			lastSeq = model.LastEventSeq
			event := ProjectionEvent{
				Type:       ProjectionEventSnapshot,
				WorkflowID: workflowID,
				Workflow:   model,
				Exploration: &ExplorationProjection{
					WorkflowID:           workflowID,
					ActiveExploration:    model.ActiveExploration,
					ExplorationSnapshots: append([]archaeodomain.ExplorationSnapshot(nil), model.ExplorationSnapshots...),
					TensionSummary:       model.TensionSummary,
				},
				LearningQueue: &LearningQueueProjection{
					WorkflowID:         workflowID,
					PendingLearning:    append([]archaeolearning.Interaction(nil), model.PendingLearning...),
					PendingGuidanceIDs: append([]string(nil), model.PendingGuidanceIDs...),
				},
				ActivePlan: &ActivePlanProjection{
					WorkflowID:        workflowID,
					PhaseState:        model.PhaseState,
					ActivePlanVersion: model.ActivePlanVersion,
					ConvergenceState:  model.ConvergenceState,
				},
				Timeline: &TimelineProjection{
					WorkflowID:   workflowID,
					Timeline:     append([]archaeodomain.TimelineEvent(nil), model.Timeline...),
					LastEventSeq: model.LastEventSeq,
					UpdatedAt:    s.now(),
				},
			}
			select {
			case ch <- event:
			default:
			}
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func (s *Service) runMaterializer(ctx context.Context, workflowID, snapshotKey string, materializer event.Materializer) error {
	workflowID = strings.TrimSpace(workflowID)
	if s == nil || s.Store == nil || workflowID == "" {
		return nil
	}
	runner := &event.Runner{
		Log:           &archaeoevents.WorkflowLog{Store: s.Store, Now: s.Now},
		Materializers: []event.Materializer{materializer},
		Partition:     projectionPartition(workflowID, snapshotKey),
	}
	return runner.RestoreAndRunOnce(ctx)
}

func (s *Service) addSub(workflowID string, ch chan ProjectionEvent, cancel context.CancelFunc) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.subs == nil {
		s.subs = make(map[int]projectionSubscription)
	}
	id := s.seq
	s.seq++
	s.subs[id] = projectionSubscription{workflowID: workflowID, ch: ch, cancel: cancel}
	return id
}

func (s *Service) removeSub(id int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.subs, id)
}

func (s *Service) pollInterval() time.Duration {
	if s != nil && s.PollInterval > 0 {
		return s.PollInterval
	}
	return 100 * time.Millisecond
}

func (s *Service) now() time.Time {
	if s != nil && s.Now != nil {
		return s.Now().UTC()
	}
	return time.Now().UTC()
}

type PhaseStateMaterializer struct {
	Store      memory.WorkflowStateStore
	WorkflowID string
	Projection *archaeodomain.WorkflowPhaseState
}

func (m *PhaseStateMaterializer) Name() string { return "archaeo-phase-state-projection" }
func (m *PhaseStateMaterializer) Apply(ctx context.Context, events []core.FrameworkEvent) error {
	if m.Projection != nil && !hasRelevantEvent(events, archaeoevents.EventWorkflowPhaseTransitioned) {
		return nil
	}
	if m.Store == nil || strings.TrimSpace(m.WorkflowID) == "" {
		return nil
	}
	proj, ok, err := (archaeophases.Service{Store: m.Store}).Load(ctx, m.WorkflowID)
	if err != nil {
		return err
	}
	if ok {
		m.Projection = proj
	}
	return nil
}
func (m *PhaseStateMaterializer) Snapshot(_ context.Context) ([]byte, error) {
	return json.Marshal(m.Projection)
}
func (m *PhaseStateMaterializer) Restore(_ context.Context, data []byte) error {
	if len(data) == 0 {
		return nil
	}
	var value archaeodomain.WorkflowPhaseState
	if err := json.Unmarshal(data, &value); err != nil {
		return err
	}
	m.Projection = &value
	return nil
}

type ExplorationMaterializer struct {
	Store      memory.WorkflowStateStore
	WorkflowID string
	Projection *ExplorationProjection
}

func (m *ExplorationMaterializer) Name() string { return "archaeo-exploration-projection" }
func (m *ExplorationMaterializer) Apply(ctx context.Context, events []core.FrameworkEvent) error {
	if m.Projection != nil && !hasRelevantEvent(events,
		archaeoevents.EventExplorationSessionUpserted,
		archaeoevents.EventExplorationSnapshotUpserted,
		archaeoevents.EventTensionUpserted,
	) {
		return nil
	}
	if m.Store == nil || strings.TrimSpace(m.WorkflowID) == "" {
		return nil
	}
	svc := archaeoarch.Service{Store: m.Store, Learning: archaeolearning.Service{Store: m.Store}}
	view, err := svc.LoadExplorationViewByWorkflow(ctx, m.WorkflowID)
	if err != nil {
		return err
	}
	if view == nil {
		m.Projection = &ExplorationProjection{WorkflowID: m.WorkflowID}
		return nil
	}
	m.Projection = &ExplorationProjection{
		WorkflowID:           m.WorkflowID,
		ActiveExploration:    view.Session,
		ExplorationSnapshots: append([]archaeodomain.ExplorationSnapshot(nil), view.Snapshots...),
		TensionSummary:       view.TensionSummary,
	}
	return nil
}
func (m *ExplorationMaterializer) Snapshot(_ context.Context) ([]byte, error) {
	return json.Marshal(m.Projection)
}
func (m *ExplorationMaterializer) Restore(_ context.Context, data []byte) error {
	if len(data) == 0 {
		return nil
	}
	var value ExplorationProjection
	if err := json.Unmarshal(data, &value); err != nil {
		return err
	}
	m.Projection = &value
	return nil
}

type LearningQueueMaterializer struct {
	Store      memory.WorkflowStateStore
	WorkflowID string
	Projection *LearningQueueProjection
}

func (m *LearningQueueMaterializer) Name() string { return "archaeo-learning-queue-projection" }
func (m *LearningQueueMaterializer) Apply(ctx context.Context, events []core.FrameworkEvent) error {
	if m.Projection != nil && !hasRelevantEvent(events,
		archaeoevents.EventLearningInteractionRequested,
		archaeoevents.EventLearningInteractionResolved,
		archaeoevents.EventLearningInteractionExpired,
		archaeoevents.EventWorkflowPhaseTransitioned,
	) {
		return nil
	}
	if m.Store == nil || strings.TrimSpace(m.WorkflowID) == "" {
		return nil
	}
	learningSvc := archaeolearning.Service{Store: m.Store}
	pending, err := learningSvc.Pending(ctx, m.WorkflowID)
	if err != nil {
		return err
	}
	blockingIDs := make([]string, 0)
	for _, interaction := range pending {
		if interaction.Blocking {
			blockingIDs = append(blockingIDs, interaction.ID)
		}
	}
	var pendingGuidance []string
	if phase, ok, err := (archaeophases.Service{Store: m.Store}).Load(ctx, m.WorkflowID); err != nil {
		return err
	} else if ok {
		pendingGuidance = append([]string(nil), phase.PendingGuidance...)
	}
	m.Projection = &LearningQueueProjection{
		WorkflowID:         m.WorkflowID,
		PendingLearning:    append([]archaeolearning.Interaction(nil), pending...),
		PendingGuidanceIDs: pendingGuidance,
		BlockingLearning:   blockingIDs,
	}
	return nil
}
func (m *LearningQueueMaterializer) Snapshot(_ context.Context) ([]byte, error) {
	return json.Marshal(m.Projection)
}
func (m *LearningQueueMaterializer) Restore(_ context.Context, data []byte) error {
	if len(data) == 0 {
		return nil
	}
	var value LearningQueueProjection
	if err := json.Unmarshal(data, &value); err != nil {
		return err
	}
	m.Projection = &value
	return nil
}

type ActivePlanMaterializer struct {
	Store      memory.WorkflowStateStore
	WorkflowID string
	Projection *ActivePlanProjection
}

func (m *ActivePlanMaterializer) Name() string { return "archaeo-active-plan-projection" }
func (m *ActivePlanMaterializer) Apply(ctx context.Context, events []core.FrameworkEvent) error {
	if m.Projection != nil && !hasRelevantEvent(events,
		archaeoevents.EventWorkflowPhaseTransitioned,
		archaeoevents.EventPlanVersionUpserted,
		archaeoevents.EventPlanVersionActivated,
		archaeoevents.EventPlanVersionArchived,
		archaeoevents.EventConvergenceVerified,
		archaeoevents.EventConvergenceFailed,
	) {
		return nil
	}
	if m.Store == nil || strings.TrimSpace(m.WorkflowID) == "" {
		return nil
	}
	activePlan, err := (archaeoplans.Service{WorkflowStore: m.Store}).LoadActiveVersion(ctx, m.WorkflowID)
	if err != nil {
		return err
	}
	var phase *archaeodomain.WorkflowPhaseState
	if value, ok, err := (archaeophases.Service{Store: m.Store}).Load(ctx, m.WorkflowID); err != nil {
		return err
	} else if ok {
		phase = value
	}
	m.Projection = &ActivePlanProjection{
		WorkflowID:        m.WorkflowID,
		PhaseState:        phase,
		ActivePlanVersion: activePlan,
		ConvergenceState:  latestConvergenceState(ctx, m.Store, m.WorkflowID),
	}
	return nil
}
func (m *ActivePlanMaterializer) Snapshot(_ context.Context) ([]byte, error) {
	return json.Marshal(m.Projection)
}
func (m *ActivePlanMaterializer) Restore(_ context.Context, data []byte) error {
	if len(data) == 0 {
		return nil
	}
	var value ActivePlanProjection
	if err := json.Unmarshal(data, &value); err != nil {
		return err
	}
	m.Projection = &value
	return nil
}

type TimelineMaterializer struct {
	Store      memory.WorkflowStateStore
	WorkflowID string
	Projection *TimelineProjection
}

func (m *TimelineMaterializer) Name() string { return "archaeo-workflow-timeline-projection" }
func (m *TimelineMaterializer) Apply(ctx context.Context, events []core.FrameworkEvent) error {
	if m.Projection != nil {
		if len(events) == 0 {
			return nil
		}
		next := timelineFromFrameworkEvents(events)
		if len(next) == 0 {
			return nil
		}
		timeline := append([]archaeodomain.TimelineEvent(nil), m.Projection.Timeline...)
		for _, entry := range next {
			if entry.Seq <= m.Projection.LastEventSeq {
				continue
			}
			timeline = append(timeline, entry)
			m.Projection.LastEventSeq = entry.Seq
		}
		m.Projection.Timeline = timeline
		m.Projection.UpdatedAt = time.Now().UTC()
		return nil
	}
	timeline, seq, err := m.build(ctx)
	if err != nil {
		return err
	}
	m.Projection = &TimelineProjection{
		WorkflowID:   m.WorkflowID,
		Timeline:     timeline,
		LastEventSeq: seq,
		UpdatedAt:    time.Now().UTC(),
	}
	return nil
}
func (m *TimelineMaterializer) Snapshot(_ context.Context) ([]byte, error) {
	return json.Marshal(m.Projection)
}
func (m *TimelineMaterializer) Restore(_ context.Context, data []byte) error {
	if len(data) == 0 {
		return nil
	}
	var value TimelineProjection
	if err := json.Unmarshal(data, &value); err != nil {
		return err
	}
	m.Projection = &value
	return nil
}

func (m *TimelineMaterializer) build(ctx context.Context) ([]archaeodomain.TimelineEvent, uint64, error) {
	recordsAsc, err := (&archaeoevents.WorkflowLog{Store: m.Store}).ReadRecords(ctx, m.WorkflowID)
	if err != nil {
		return nil, 0, err
	}
	timeline := timelineFromWorkflowRecords(recordsAsc)
	var seq uint64
	if len(recordsAsc) > 0 {
		seq = uint64Value(recordsAsc[len(recordsAsc)-1].Metadata["archaeo_seq"])
	}
	return timeline, seq, nil
}

func (m *TimelineMaterializer) buildTimeline(ctx context.Context) ([]archaeodomain.TimelineEvent, error) {
	timeline, _, err := m.build(ctx)
	return timeline, err
}

type CompositeMaterializer struct {
	WorkflowID string
	Service    *Service
	Model      WorkflowReadModel
}

func (m *CompositeMaterializer) Name() string { return "archaeo-workflow-projection" }
func (m *CompositeMaterializer) Apply(ctx context.Context, events []core.FrameworkEvent) error {
	if len(events) == 0 && m.Model.WorkflowID != "" {
		return nil
	}
	if m.Service == nil {
		return nil
	}
	model, err := m.Service.buildWorkflow(ctx, m.WorkflowID)
	if err != nil {
		return err
	}
	if model != nil {
		m.Model = *model
	}
	return nil
}
func (m *CompositeMaterializer) Snapshot(_ context.Context) ([]byte, error) {
	return json.Marshal(m.Model)
}
func (m *CompositeMaterializer) Restore(_ context.Context, data []byte) error {
	if len(data) == 0 {
		return nil
	}
	var value WorkflowReadModel
	if err := json.Unmarshal(data, &value); err != nil {
		return err
	}
	m.Model = value
	return nil
}

type MutationHistoryMaterializer struct {
	Store      memory.WorkflowStateStore
	WorkflowID string
	Projection *MutationHistoryProjection
}

func (m *MutationHistoryMaterializer) Name() string { return "archaeo-mutation-history-projection" }
func (m *MutationHistoryMaterializer) Apply(ctx context.Context, events []core.FrameworkEvent) error {
	if m.Projection != nil && !hasRelevantEvent(events, archaeoevents.EventMutationRecorded) {
		return nil
	}
	proj, err := buildMutationHistoryProjection(ctx, m.Store, m.WorkflowID)
	if err != nil {
		return err
	}
	m.Projection = proj
	return nil
}
func (m *MutationHistoryMaterializer) Snapshot(_ context.Context) ([]byte, error) {
	return json.Marshal(m.Projection)
}
func (m *MutationHistoryMaterializer) Restore(_ context.Context, data []byte) error {
	if len(data) == 0 {
		return nil
	}
	var value MutationHistoryProjection
	if err := json.Unmarshal(data, &value); err != nil {
		return err
	}
	m.Projection = &value
	return nil
}

type RequestHistoryMaterializer struct {
	Store      memory.WorkflowStateStore
	WorkflowID string
	Projection *RequestHistoryProjection
}

func (m *RequestHistoryMaterializer) Name() string { return "archaeo-request-history-projection" }
func (m *RequestHistoryMaterializer) Apply(ctx context.Context, events []core.FrameworkEvent) error {
	if m.Projection != nil && !hasRelevantEvent(events,
		archaeoevents.EventRequestCreated,
		archaeoevents.EventRequestDispatched,
		archaeoevents.EventRequestStarted,
		archaeoevents.EventRequestCompleted,
		archaeoevents.EventRequestFailed,
		archaeoevents.EventRequestCanceled,
	) {
		return nil
	}
	proj, err := buildRequestHistoryProjection(ctx, m.Store, m.WorkflowID)
	if err != nil {
		return err
	}
	m.Projection = proj
	return nil
}
func (m *RequestHistoryMaterializer) Snapshot(_ context.Context) ([]byte, error) {
	return json.Marshal(m.Projection)
}
func (m *RequestHistoryMaterializer) Restore(_ context.Context, data []byte) error {
	if len(data) == 0 {
		return nil
	}
	var value RequestHistoryProjection
	if err := json.Unmarshal(data, &value); err != nil {
		return err
	}
	m.Projection = &value
	return nil
}

type PlanLineageMaterializer struct {
	Store      memory.WorkflowStateStore
	WorkflowID string
	Projection *PlanLineageProjection
}

func (m *PlanLineageMaterializer) Name() string { return "archaeo-plan-lineage-projection" }
func (m *PlanLineageMaterializer) Apply(ctx context.Context, events []core.FrameworkEvent) error {
	if m.Projection != nil && !hasRelevantEvent(events,
		archaeoevents.EventPlanVersionUpserted,
		archaeoevents.EventPlanVersionActivated,
		archaeoevents.EventPlanVersionArchived,
	) {
		return nil
	}
	proj, err := buildPlanLineageProjection(ctx, m.Store, m.WorkflowID)
	if err != nil {
		return err
	}
	m.Projection = proj
	return nil
}
func (m *PlanLineageMaterializer) Snapshot(_ context.Context) ([]byte, error) {
	return json.Marshal(m.Projection)
}
func (m *PlanLineageMaterializer) Restore(_ context.Context, data []byte) error {
	if len(data) == 0 {
		return nil
	}
	var value PlanLineageProjection
	if err := json.Unmarshal(data, &value); err != nil {
		return err
	}
	m.Projection = &value
	return nil
}

type ExplorationActivityMaterializer struct {
	Store      memory.WorkflowStateStore
	WorkflowID string
	Service    *Service
	Projection *ExplorationActivityProjection
}

func (m *ExplorationActivityMaterializer) Name() string { return "archaeo-exploration-activity-projection" }
func (m *ExplorationActivityMaterializer) Apply(ctx context.Context, events []core.FrameworkEvent) error {
	if m.Projection != nil && !hasRelevantEvent(events,
		archaeoevents.EventExplorationSessionUpserted,
		archaeoevents.EventExplorationSnapshotUpserted,
		archaeoevents.EventLearningInteractionRequested,
		archaeoevents.EventLearningInteractionResolved,
		archaeoevents.EventLearningInteractionExpired,
		archaeoevents.EventTensionUpserted,
		archaeoevents.EventMutationRecorded,
		archaeoevents.EventRequestCreated,
		archaeoevents.EventRequestDispatched,
		archaeoevents.EventRequestStarted,
		archaeoevents.EventRequestCompleted,
		archaeoevents.EventRequestFailed,
		archaeoevents.EventRequestCanceled,
	) {
		return nil
	}
	proj, err := buildExplorationActivityProjection(ctx, m.Store, m.WorkflowID)
	if err != nil {
		return err
	}
	m.Projection = proj
	return nil
}
func (m *ExplorationActivityMaterializer) Snapshot(_ context.Context) ([]byte, error) {
	return json.Marshal(m.Projection)
}
func (m *ExplorationActivityMaterializer) Restore(_ context.Context, data []byte) error {
	if len(data) == 0 {
		return nil
	}
	var value ExplorationActivityProjection
	if err := json.Unmarshal(data, &value); err != nil {
		return err
	}
	m.Projection = &value
	return nil
}

type ProvenanceMaterializer struct {
	Store      memory.WorkflowStateStore
	WorkflowID string
	Service    *Service
	Projection *ProvenanceProjection
}

func (m *ProvenanceMaterializer) Name() string { return "archaeo-provenance-projection" }
func (m *ProvenanceMaterializer) Apply(ctx context.Context, events []core.FrameworkEvent) error {
	if m.Projection != nil && !hasRelevantEvent(events,
		archaeoevents.EventLearningInteractionRequested,
		archaeoevents.EventLearningInteractionResolved,
		archaeoevents.EventLearningInteractionExpired,
		archaeoevents.EventTensionUpserted,
		archaeoevents.EventPlanVersionUpserted,
		archaeoevents.EventPlanVersionActivated,
		archaeoevents.EventPlanVersionArchived,
		archaeoevents.EventMutationRecorded,
	) {
		return nil
	}
	proj, err := buildProvenanceProjection(ctx, m.Store, m.WorkflowID)
	if err != nil {
		return err
	}
	m.Projection = proj
	return nil
}
func (m *ProvenanceMaterializer) Snapshot(_ context.Context) ([]byte, error) {
	return json.Marshal(m.Projection)
}
func (m *ProvenanceMaterializer) Restore(_ context.Context, data []byte) error {
	if len(data) == 0 {
		return nil
	}
	var value ProvenanceProjection
	if err := json.Unmarshal(data, &value); err != nil {
		return err
	}
	m.Projection = &value
	return nil
}

type CoherenceMaterializer struct {
	Store      memory.WorkflowStateStore
	WorkflowID string
	Service    *Service
	Projection *CoherenceProjection
}

func (m *CoherenceMaterializer) Name() string { return "archaeo-coherence-projection" }
func (m *CoherenceMaterializer) Apply(ctx context.Context, events []core.FrameworkEvent) error {
	if m.Projection != nil && !hasRelevantEvent(events,
		archaeoevents.EventTensionUpserted,
		archaeoevents.EventLearningInteractionRequested,
		archaeoevents.EventLearningInteractionResolved,
		archaeoevents.EventLearningInteractionExpired,
		archaeoevents.EventPlanVersionUpserted,
		archaeoevents.EventPlanVersionActivated,
		archaeoevents.EventPlanVersionArchived,
		archaeoevents.EventConvergenceVerified,
		archaeoevents.EventConvergenceFailed,
		archaeoevents.EventMutationRecorded,
	) {
		return nil
	}
	proj, err := buildCoherenceProjection(ctx, m.Store, m.WorkflowID)
	if err != nil {
		return err
	}
	m.Projection = proj
	return nil
}
func (m *CoherenceMaterializer) Snapshot(_ context.Context) ([]byte, error) {
	return json.Marshal(m.Projection)
}
func (m *CoherenceMaterializer) Restore(_ context.Context, data []byte) error {
	if len(data) == 0 {
		return nil
	}
	var value CoherenceProjection
	if err := json.Unmarshal(data, &value); err != nil {
		return err
	}
	m.Projection = &value
	return nil
}

func hasRelevantEvent(events []core.FrameworkEvent, types ...string) bool {
	if len(events) == 0 {
		return false
	}
	if len(types) == 0 {
		return true
	}
	allowed := make(map[string]struct{}, len(types))
	for _, value := range types {
		allowed[value] = struct{}{}
	}
	for _, ev := range events {
		if _, ok := allowed[ev.Type]; ok {
			return true
		}
	}
	return false
}

func timelineFromFrameworkEvents(events []core.FrameworkEvent) []archaeodomain.TimelineEvent {
	out := make([]archaeodomain.TimelineEvent, 0, len(events))
	for _, ev := range events {
		entry := archaeodomain.TimelineEvent{
			Seq:       ev.Seq,
			EventType: ev.Type,
			CreatedAt: ev.Timestamp,
			Metadata:  map[string]any{},
		}
		var payload workflowEventPayload
		if err := json.Unmarshal(ev.Payload, &payload); err == nil {
			entry.EventID = payload.EventID
			entry.WorkflowID = payload.WorkflowID
			entry.RunID = payload.RunID
			entry.StepID = payload.StepID
			entry.Message = payload.Message
			if payload.Metadata != nil {
				entry.Metadata = payload.Metadata
			}
		}
		out = append(out, entry)
	}
	return out
}

func timelineFromWorkflowRecords(records []memory.WorkflowEventRecord) []archaeodomain.TimelineEvent {
	out := make([]archaeodomain.TimelineEvent, 0, len(records))
	for _, record := range records {
		out = append(out, archaeodomain.TimelineEvent{
			Seq:        uint64Value(record.Metadata["archaeo_seq"]),
			EventID:    record.EventID,
			WorkflowID: record.WorkflowID,
			RunID:      record.RunID,
			StepID:     record.StepID,
			EventType:  record.EventType,
			Message:    record.Message,
			Metadata:   cloneMap(record.Metadata),
			CreatedAt:  record.CreatedAt,
		})
	}
	return out
}

func convergenceFromTimeline(timeline []archaeodomain.TimelineEvent) *archaeodomain.ConvergenceState {
	for i := len(timeline) - 1; i >= 0; i-- {
		entry := timeline[i]
		switch entry.EventType {
		case archaeoevents.EventConvergenceVerified:
			return &archaeodomain.ConvergenceState{
				WorkflowID:          entry.WorkflowID,
				PlanID:              stringValue(entry.Metadata["plan_id"]),
				PlanVersion:         intPointer(entry.Metadata["plan_version"]),
				Status:              archaeodomain.ConvergenceStatusVerified,
				BasedOnRevision:     stringValue(entry.Metadata["based_on_revision"]),
				SemanticSnapshotRef: stringValue(entry.Metadata["semantic_snapshot_ref"]),
				UpdatedAt:           entry.CreatedAt,
			}
		case archaeoevents.EventConvergenceFailed:
			return &archaeodomain.ConvergenceState{
				WorkflowID:           entry.WorkflowID,
				PlanID:               stringValue(entry.Metadata["plan_id"]),
				PlanVersion:          intPointer(entry.Metadata["plan_version"]),
				Status:               archaeodomain.ConvergenceStatusFailed,
				Description:          stringValue(entry.Metadata["description"]),
				UnresolvedTensionIDs: stringSlice(entry.Metadata["unresolved_tension_ids"]),
				BasedOnRevision:      stringValue(entry.Metadata["based_on_revision"]),
				SemanticSnapshotRef:  stringValue(entry.Metadata["semantic_snapshot_ref"]),
				UpdatedAt:            entry.CreatedAt,
			}
		}
	}
	return nil
}

func latestConvergenceState(ctx context.Context, store memory.WorkflowStateStore, workflowID string) *archaeodomain.ConvergenceState {
	record, ok, err := (&archaeoevents.WorkflowLog{Store: store}).LatestRecordByTypes(ctx, workflowID,
		archaeoevents.EventConvergenceVerified,
		archaeoevents.EventConvergenceFailed,
	)
	if err != nil || !ok || record == nil {
		return nil
	}
	entry := archaeodomain.TimelineEvent{
		EventID:    record.EventID,
		WorkflowID: record.WorkflowID,
		RunID:      record.RunID,
		StepID:     record.StepID,
		EventType:  record.EventType,
		Message:    record.Message,
		Metadata:   cloneMap(record.Metadata),
		CreatedAt:  record.CreatedAt,
	}
	return convergenceFromTimeline([]archaeodomain.TimelineEvent{entry})
}

func stringValue(raw any) string {
	if typed, ok := raw.(string); ok {
		return typed
	}
	return ""
}

func intValue(raw any) int {
	switch typed := raw.(type) {
	case int:
		return typed
	case int64:
		return int(typed)
	case float64:
		return int(typed)
	case json.Number:
		if value, err := typed.Int64(); err == nil {
			return int(value)
		}
	case string:
		if value, err := strconv.Atoi(strings.TrimSpace(typed)); err == nil {
			return value
		}
	}
	return 0
}

func intPointerClone(value *int) *int {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}

func intPointer(raw any) *int {
	switch typed := raw.(type) {
	case int:
		value := typed
		return &value
	case int64:
		value := int(typed)
		return &value
	case float64:
		value := int(typed)
		return &value
	}
	return nil
}

func stringSlice(raw any) []string {
	values, ok := raw.([]any)
	if !ok {
		return nil
	}
	out := make([]string, 0, len(values))
	for _, value := range values {
		if text, ok := value.(string); ok && strings.TrimSpace(text) != "" {
			out = append(out, text)
		}
	}
	return out
}

func cloneMap(values map[string]any) map[string]any {
	if len(values) == 0 {
		return map[string]any{}
	}
	out := make(map[string]any, len(values))
	for key, value := range values {
		out[key] = value
	}
	return out
}

func uint64Value(raw any) uint64 {
	switch typed := raw.(type) {
	case uint64:
		return typed
	case uint32:
		return uint64(typed)
	case uint:
		return uint64(typed)
	case int:
		if typed > 0 {
			return uint64(typed)
		}
	case int64:
		if typed > 0 {
			return uint64(typed)
		}
	case float64:
		if typed > 0 {
			return uint64(typed)
		}
	case json.Number:
		value, err := typed.Int64()
		if err == nil && value > 0 {
			return uint64(value)
		}
	case string:
		value, err := strconv.ParseUint(strings.TrimSpace(typed), 10, 64)
		if err == nil {
			return value
		}
	}
	return 0
}
