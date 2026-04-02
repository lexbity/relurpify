package server

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/framework/event"
	rexcontrolplane "github.com/lexcodex/relurpify/named/rex/controlplane"
	rexevents "github.com/lexcodex/relurpify/named/rex/events"
	rexgateway "github.com/lexcodex/relurpify/named/rex/gateway"
	rexruntime "github.com/lexcodex/relurpify/named/rex/runtime"
)

const rexEventBridgeConsumerID = "rex_event_bridge.v1"

type rexEventCursorStore interface {
	Load(context.Context) (uint64, error)
	Save(context.Context, uint64) error
}

type rexEventBridgeGateway interface {
	Resolve(context.Context, rexevents.CanonicalEvent) (rexgateway.Decision, error)
}

type RexEventBridge struct {
	Log        event.Log
	Partition  string
	Cursor     rexEventCursorStore
	Gateway    rexEventBridgeGateway
	Control    func(context.Context, core.FrameworkEvent) error
	Handle     func(context.Context, rexgateway.Decision, rexevents.CanonicalEvent) error
	PollPeriod time.Duration
	// Phase 7.1: Admission control for gateway routing
	Admission      rexcontrolplane.AdmissionController
	AdmissionAudit *rexcontrolplane.AuditLog
}

func NewRexEventBridge(log event.Log, partition string, runtime *RexRuntimeProvider) (*RexEventBridge, error) {
	if log == nil {
		return nil, fmt.Errorf("event log required")
	}
	if runtime == nil || runtime.Agent == nil || runtime.WorkflowStore == nil {
		return nil, fmt.Errorf("rex runtime with workflow store required")
	}
	return &RexEventBridge{
		Log:       log,
		Partition: partitionOrDefault(partition),
		Cursor:    newSQLiteRexEventCursorStore(runtime.WorkflowStore.DB(), partitionOrDefault(partition), rexEventBridgeConsumerID),
		Gateway:   rexgateway.DefaultGateway{Store: runtime.WorkflowStore},
		Handle:    runtime.handleEventDecision,
		// Phase 7.1: Wire admission controller from runtime
		Admission:      runtime.Admission,
		AdmissionAudit: runtime.AdmissionAudit,
	}, nil
}

func (b *RexEventBridge) Start(ctx context.Context) error {
	if b == nil || b.Log == nil || b.Cursor == nil || b.Gateway == nil || b.Handle == nil {
		return nil
	}
	afterSeq, err := b.Cursor.Load(ctx)
	if err != nil {
		return err
	}
	go b.loop(ctx, afterSeq)
	return nil
}

func (b *RexEventBridge) loop(ctx context.Context, afterSeq uint64) {
	for {
		events, err := b.Log.Read(ctx, partitionOrDefault(b.Partition), afterSeq, 256, true)
		if err != nil {
			return
		}
		for _, frameworkEvent := range events {
			if err := b.processEvent(ctx, frameworkEvent); err != nil {
				continue
			}
			afterSeq = frameworkEvent.Seq
			// Persist successful progress even if shutdown has already canceled the loop context.
			_ = b.Cursor.Save(context.WithoutCancel(ctx), afterSeq)
		}
	}
}

func (b *RexEventBridge) processEvent(ctx context.Context, frameworkEvent core.FrameworkEvent) error {
	if b.Control != nil {
		if handled := isRexControlPlaneEvent(frameworkEvent.Type); handled {
			if err := b.Control(ctx, frameworkEvent); err != nil {
				return err
			}
			return nil
		}
	}
	canonicalEvent, ok, err := mapFrameworkEventToRex(frameworkEvent)
	if err != nil || !ok {
		return err
	}

	// Phase 7.1: Check admission control before routing task to Rex
	if b.Admission != nil && shouldCheckAdmission(canonicalEvent) {
		tenantID := extractTenantID(frameworkEvent)
		workloadClass := extractWorkloadClass(frameworkEvent)
		admissionReq := rexcontrolplane.AdmissionRequest{
			TenantID: tenantID,
			Class:    workloadClass,
		}
		admissionDecision := b.Admission.Decide(admissionReq)
		recordAdmissionDecision(b.AdmissionAudit, admissionReq, admissionDecision)
		if !admissionDecision.Allowed {
			emitAdmissionRejection(ctx, b.Log, frameworkEvent, admissionDecision)
			return nil
		}
		annotateAdmissionContext(&canonicalEvent, admissionReq)
	}

	decision, err := b.Gateway.Resolve(ctx, canonicalEvent)
	if err != nil {
		return err
	}
	if decision.Decision == rexgateway.SignalDecisionReject {
		return nil
	}
	return b.Handle(ctx, decision, canonicalEvent)
}

func isRexControlPlaneEvent(eventType string) bool {
	switch strings.TrimSpace(eventType) {
	case core.FrameworkEventFMPHandoffOffered, core.FrameworkEventFMPHandoffAccepted, core.FrameworkEventFMPResumeCommitted, core.FrameworkEventFMPFenceIssued:
		return true
	default:
		return false
	}
}

func mapFrameworkEventToRex(frameworkEvent core.FrameworkEvent) (rexevents.CanonicalEvent, bool, error) {
	switch strings.TrimSpace(frameworkEvent.Type) {
	case rexevents.TypeTaskRequested, rexevents.TypeWorkflowResume, rexevents.TypeWorkflowSignal, rexevents.TypeCallbackReceived:
		ev, err := rexevents.FromFrameworkEvent(frameworkEvent)
		return ev, err == nil, err
	case core.FrameworkEventSessionMessage:
		ev, err := mapSessionMessageToRex(frameworkEvent)
		return ev, err == nil, err
	default:
		return rexevents.CanonicalEvent{}, false, nil
	}
}

func mapSessionMessageToRex(frameworkEvent core.FrameworkEvent) (rexevents.CanonicalEvent, error) {
	var payload struct {
		SessionKey     string `json:"session_key"`
		Channel        string `json:"channel"`
		ConversationID string `json:"conversation_id"`
		ThreadID       string `json:"thread_id"`
		SenderID       string `json:"sender_id"`
		Content        string `json:"content"`
	}
	if err := json.Unmarshal(frameworkEvent.Payload, &payload); err != nil {
		return rexevents.CanonicalEvent{}, err
	}
	instruction := strings.TrimSpace(payload.Content)
	if instruction == "" {
		return rexevents.CanonicalEvent{}, fmt.Errorf("session message content required")
	}
	canonicalPayload := map[string]any{
		"task_id":         fmt.Sprintf("session:%s:%d", firstNonEmpty(payload.SessionKey, frameworkEvent.Actor.ID), frameworkEvent.Seq),
		"instruction":     instruction,
		"session_key":     payload.SessionKey,
		"channel":         payload.Channel,
		"conversation_id": payload.ConversationID,
		"thread_id":       payload.ThreadID,
		"sender_id":       payload.SenderID,
	}
	if payload.SessionKey != "" {
		canonicalPayload["workflow_id"] = "rex-session:" + payload.SessionKey
	}
	return rexevents.DefaultNormalizer{}.Normalize(rexevents.CanonicalEvent{
		ID:             fmt.Sprintf("%d", frameworkEvent.Seq),
		Type:           rexevents.TypeTaskRequested,
		Timestamp:      frameworkEvent.Timestamp,
		ActorID:        firstNonEmpty(payload.SessionKey, frameworkEvent.Actor.ID),
		Partition:      frameworkEvent.Partition,
		IdempotencyKey: firstNonEmpty(frameworkEvent.IdempotencyKey, fmt.Sprintf("%d", frameworkEvent.Seq)),
		Payload:        canonicalPayload,
		TrustClass:     rexevents.TrustInternal,
		Source:         "framework.session",
	})
}

type sqliteRexEventCursorStore struct {
	db        *sql.DB
	partition string
	consumer  string
}

func newSQLiteRexEventCursorStore(db *sql.DB, partition, consumer string) *sqliteRexEventCursorStore {
	store := &sqliteRexEventCursorStore{
		db:        db,
		partition: partitionOrDefault(partition),
		consumer:  strings.TrimSpace(consumer),
	}
	store.ensureSchema()
	return store
}

func (s *sqliteRexEventCursorStore) ensureSchema() {
	if s == nil || s.db == nil {
		return
	}
	_, _ = s.db.Exec(`CREATE TABLE IF NOT EXISTS nexus_runtime_cursors (
		partition TEXT NOT NULL,
		consumer_id TEXT NOT NULL,
		last_seq INTEGER NOT NULL DEFAULT 0,
		updated_at TEXT NOT NULL,
		PRIMARY KEY (partition, consumer_id)
	);`)
}

func (s *sqliteRexEventCursorStore) Load(ctx context.Context) (uint64, error) {
	if s == nil || s.db == nil {
		return 0, nil
	}
	var seq uint64
	err := s.db.QueryRowContext(ctx, `SELECT last_seq FROM nexus_runtime_cursors WHERE partition = ? AND consumer_id = ?`, s.partition, s.consumer).Scan(&seq)
	if err == sql.ErrNoRows {
		return 0, nil
	}
	return seq, err
}

func (s *sqliteRexEventCursorStore) Save(ctx context.Context, seq uint64) error {
	if s == nil || s.db == nil {
		return nil
	}
	_, err := s.db.ExecContext(ctx, `INSERT INTO nexus_runtime_cursors (partition, consumer_id, last_seq, updated_at)
		VALUES (?, ?, ?, ?)
		ON CONFLICT(partition, consumer_id) DO UPDATE SET last_seq = excluded.last_seq, updated_at = excluded.updated_at`,
		s.partition, s.consumer, seq, time.Now().UTC().Format(time.RFC3339Nano))
	return err
}

func partitionOrDefault(partition string) string {
	if strings.TrimSpace(partition) == "" {
		return "local"
	}
	return strings.TrimSpace(partition)
}

// Phase 7.1: Admission control helper functions

func shouldCheckAdmission(event rexevents.CanonicalEvent) bool {
	// Check admission for task submission events, not internal signals
	return event.Type == rexevents.TypeTaskRequested || event.Type == rexevents.TypeWorkflowResume
}

func extractTenantID(frameworkEvent core.FrameworkEvent) string {
	if frameworkEvent.Actor.TenantID != "" {
		return frameworkEvent.Actor.TenantID
	}
	return "default"
}

func extractWorkloadClass(frameworkEvent core.FrameworkEvent) rexcontrolplane.WorkloadClass {
	// Parse workload class from event payload or default to best_effort
	if len(frameworkEvent.Payload) == 0 {
		return rexcontrolplane.WorkloadBestEffort
	}
	var payload map[string]any
	if err := json.Unmarshal(frameworkEvent.Payload, &payload); err != nil {
		return rexcontrolplane.WorkloadBestEffort
	}
	if classStr, ok := payload["workload_class"].(string); ok {
		switch strings.TrimSpace(classStr) {
		case string(rexcontrolplane.WorkloadCritical):
			return rexcontrolplane.WorkloadCritical
		case string(rexcontrolplane.WorkloadImportant):
			return rexcontrolplane.WorkloadImportant
		}
	}
	return rexcontrolplane.WorkloadBestEffort
}

func emitAdmissionRejection(ctx context.Context, log event.Log, frameworkEvent core.FrameworkEvent, decision rexcontrolplane.AdmissionDecision) {
	if log == nil {
		return
	}
	payload, err := json.Marshal(map[string]any{
		"tenant_id":      firstNonEmpty(frameworkEvent.Actor.TenantID, "default"),
		"event_type":     frameworkEvent.Type,
		"event_seq":      frameworkEvent.Seq,
		"workload_class": string(extractWorkloadClass(frameworkEvent)),
		"reason":         decision.Reason,
	})
	if err != nil {
		return
	}
	_, _ = log.Append(ctx, partitionOrDefault(frameworkEvent.Partition), []core.FrameworkEvent{{
		Type:      "rex.admission.rejected.v1",
		Timestamp: time.Now().UTC(),
		Payload:   payload,
		Partition: partitionOrDefault(frameworkEvent.Partition),
		Actor: core.EventActor{
			Kind:     frameworkEvent.Actor.Kind,
			ID:       frameworkEvent.Actor.ID,
			TenantID: frameworkEvent.Actor.TenantID,
		},
	}})
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func annotateAdmissionContext(event *rexevents.CanonicalEvent, req rexcontrolplane.AdmissionRequest) {
	if event == nil {
		return
	}
	if event.Payload == nil {
		event.Payload = map[string]any{}
	}
	event.Payload["rex.admission_tenant_id"] = strings.TrimSpace(req.TenantID)
	event.Payload["rex.workload_class"] = string(req.Class)
}

func recordAdmissionDecision(audit *rexcontrolplane.AuditLog, req rexcontrolplane.AdmissionRequest, decision rexcontrolplane.AdmissionDecision) {
	if audit == nil {
		return
	}
	audit.Append(rexcontrolplane.AuditRecord{
		Action:    "gateway_route_admission",
		Role:      "gateway",
		TenantID:  strings.TrimSpace(req.TenantID),
		Allowed:   decision.Allowed,
		Reason:    decision.Reason,
		Timestamp: time.Now().UTC(),
	})
}

func admissionRequestFromContext(ctx map[string]any) (rexcontrolplane.AdmissionRequest, bool) {
	if len(ctx) == 0 {
		return rexcontrolplane.AdmissionRequest{}, false
	}
	tenantID := strings.TrimSpace(stringValue(ctx["rex.admission_tenant_id"]))
	if tenantID == "" {
		tenantID = strings.TrimSpace(stringValue(ctx["gateway.tenant_id"]))
	}
	if tenantID == "" {
		return rexcontrolplane.AdmissionRequest{}, false
	}
	class := rexcontrolplane.WorkloadBestEffort
	switch strings.TrimSpace(stringValue(ctx["rex.workload_class"])) {
	case string(rexcontrolplane.WorkloadCritical):
		class = rexcontrolplane.WorkloadCritical
	case string(rexcontrolplane.WorkloadImportant):
		class = rexcontrolplane.WorkloadImportant
	}
	return rexcontrolplane.AdmissionRequest{TenantID: tenantID, Class: class}, true
}

func (p *RexRuntimeProvider) handleEventDecision(ctx context.Context, decision rexgateway.Decision, event rexevents.CanonicalEvent) error {
	if p == nil || p.Agent == nil || p.Agent.Runtime == nil {
		return fmt.Errorf("rex runtime unavailable")
	}
	task := rexevents.ToTask(event)
	state := core.NewContext()
	for key, value := range task.Context {
		state.Set(key, value)
	}
	admissionReq, releaseAdmission := admissionRequestFromContext(task.Context)
	if decision.WorkflowID != "" {
		task.Context["workflow_id"] = decision.WorkflowID
		state.Set("workflow_id", decision.WorkflowID)
		state.Set("rex.workflow_id", decision.WorkflowID)
	}
	if decision.RunID != "" {
		task.Context["run_id"] = decision.RunID
		state.Set("run_id", decision.RunID)
		state.Set("rex.run_id", decision.RunID)
	}
	state.Set("rex.event_type", event.Type)
	state.Set("rex.event_id", event.ID)
	state.Set("rex.event_partition", event.Partition)
	state.Set("rex.event_trust_class", event.TrustClass)
	item := rexruntime.WorkItem{
		WorkflowID: decision.WorkflowID,
		RunID:      decision.RunID,
		Task:       task,
		State:      state,
		Execute: func(ctx context.Context, item rexruntime.WorkItem) error {
			if releaseAdmission && p.Admission != nil {
				defer p.Admission.Release(admissionReq)
			}
			_, err := p.Agent.Execute(ctx, item.Task, item.State)
			return err
		},
	}
	if !p.Agent.Runtime.Enqueue(item) {
		if releaseAdmission && p.Admission != nil {
			p.Admission.Release(admissionReq)
		}
		return fmt.Errorf("rex runtime queue full")
	}
	return nil
}
