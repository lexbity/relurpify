package server

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/framework/event"
	rexcontrolplane "github.com/lexcodex/relurpify/named/rex/controlplane"
	rexevents "github.com/lexcodex/relurpify/named/rex/events"
	rexgateway "github.com/lexcodex/relurpify/named/rex/gateway"
	rexctx "github.com/lexcodex/relurpify/named/rex/rexctx"
	"github.com/lexcodex/relurpify/named/rex/rexkeys"
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
	Log             event.Log
	Partition       string
	Cursor          rexEventCursorStore
	Gateway         rexEventBridgeGateway
	Control         func(context.Context, core.FrameworkEvent) error
	Handle          func(context.Context, rexgateway.Decision, rexevents.CanonicalEvent) error
	PollPeriod      time.Duration
	Now             func() time.Time
	TrustedResolver rexctx.TrustedContextResolver
	healthMu        sync.RWMutex
	healthy         bool
	lastError       string
	failedAt        time.Time
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
		Log:             log,
		Partition:       partitionOrDefault(partition),
		Cursor:          newSQLiteRexEventCursorStore(runtime.WorkflowStore.DB(), partitionOrDefault(partition), rexEventBridgeConsumerID),
		Gateway:         rexgateway.DefaultGateway{Store: runtime.WorkflowStore},
		Handle:          runtime.handleEventDecision,
		TrustedResolver: runtime.TrustedResolver,
		healthy:         true,
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
	backoff := time.Second
	const maxBackoff = 30 * time.Second
	for {
		events, err := b.Log.Read(ctx, partitionOrDefault(b.Partition), afterSeq, 256, true)
		if err != nil {
			b.setHealth(false, err)
			select {
			case <-ctx.Done():
				return
			case <-time.After(backoff):
				if backoff < maxBackoff {
					backoff *= 2
					if backoff > maxBackoff {
						backoff = maxBackoff
					}
				}
				continue
			}
		}
		backoff = time.Second
		b.setHealth(true, nil)
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

func (b *RexEventBridge) setHealth(healthy bool, err error) {
	if b == nil {
		return
	}
	b.healthMu.Lock()
	defer b.healthMu.Unlock()
	b.healthy = healthy
	if err != nil {
		b.lastError = err.Error()
		b.failedAt = time.Now().UTC()
		return
	}
	b.lastError = ""
	if healthy {
		b.failedAt = time.Time{}
	}
}

func (b *RexEventBridge) Health() (bool, string) {
	if b == nil {
		return false, "bridge unavailable"
	}
	b.healthMu.RLock()
	defer b.healthMu.RUnlock()
	return b.healthy, b.lastError
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
	trusted, err := b.resolveTrustedExecutionContext(ctx, frameworkEvent)
	if err != nil {
		return err
	}
	trustedCtx := rexctx.WithTrustedExecutionContext(ctx, trusted)

	// Phase 7.1: Check admission control before routing task to Rex
	if b.Admission != nil && shouldCheckAdmission(canonicalEvent) {
		tenantID := trusted.TenantID
		workloadClass := trusted.WorkloadClass
		admissionReq := rexcontrolplane.AdmissionRequest{
			TenantID: tenantID,
			Class:    workloadClass,
		}
		admissionDecision := b.Admission.Decide(admissionReq)
		recordAdmissionDecision(b.AdmissionAudit, admissionReq, admissionDecision, b.nowUTC())
		if !admissionDecision.Allowed {
			emitAdmissionRejection(ctx, b.Log, frameworkEvent, admissionDecision, workloadClass)
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
	return b.Handle(trustedCtx, decision, canonicalEvent)
}

func isRexControlPlaneEvent(eventType string) bool {
	switch strings.TrimSpace(eventType) {
	case core.FrameworkEventFMPHandoffOffered, core.FrameworkEventFMPHandoffAccepted, core.FrameworkEventFMPResumeCommitted, core.FrameworkEventFMPFenceIssued:
		return true
	default:
		return false
	}
}

func (b *RexEventBridge) nowUTC() time.Time {
	if b != nil && b.Now != nil {
		return b.Now().UTC()
	}
	return time.Now().UTC()
}

func (b *RexEventBridge) resolveTrustedExecutionContext(ctx context.Context, frameworkEvent core.FrameworkEvent) (rexctx.TrustedExecutionContext, error) {
	if b != nil && b.TrustedResolver != nil {
		return b.TrustedResolver.Resolve(ctx, frameworkEvent.Actor)
	}
	return rexctx.DefaultTrustedContextResolver{}.Resolve(ctx, frameworkEvent.Actor)
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
		SessionKey     string          `json:"session_key"`
		Channel        string          `json:"channel"`
		ConversationID string          `json:"conversation_id"`
		ThreadID       string          `json:"thread_id"`
		SenderID       string          `json:"sender_id"`
		Content        json.RawMessage `json:"content"`
	}
	if err := json.Unmarshal(frameworkEvent.Payload, &payload); err != nil {
		return rexevents.CanonicalEvent{}, err
	}
	instruction := extractSessionMessageInstruction(payload.Content)
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
		canonicalPayload[rexkeys.WorkflowID] = "rex-session:" + payload.SessionKey
	}
	return rexevents.DefaultNormalizer{}.Normalize(rexevents.CanonicalEvent{
		ID:             fmt.Sprintf("%d", frameworkEvent.Seq),
		Type:           rexevents.TypeTaskRequested,
		Timestamp:      frameworkEvent.Timestamp,
		ActorID:        firstNonEmpty(payload.SessionKey, frameworkEvent.Actor.ID),
		Partition:      frameworkEvent.Partition,
		IdempotencyKey: firstNonEmpty(frameworkEvent.IdempotencyKey, fmt.Sprintf("%d", frameworkEvent.Seq)),
		Payload:        canonicalPayload,
		IngressOrigin:  rexevents.OriginInternal,
		Source:         "framework.session",
	})
}

func extractSessionMessageInstruction(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var text string
	if err := json.Unmarshal(raw, &text); err == nil {
		return strings.TrimSpace(text)
	}
	var structured struct {
		Text string `json:"text"`
	}
	if err := json.Unmarshal(raw, &structured); err == nil {
		return strings.TrimSpace(structured.Text)
	}
	return ""
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

func emitAdmissionRejection(ctx context.Context, log event.Log, frameworkEvent core.FrameworkEvent, decision rexcontrolplane.AdmissionDecision, workloadClass rexcontrolplane.WorkloadClass) {
	if log == nil {
		return
	}
	payload, err := json.Marshal(map[string]any{
		"tenant_id":      firstNonEmpty(frameworkEvent.Actor.TenantID, "default"),
		"event_type":     frameworkEvent.Type,
		"event_seq":      frameworkEvent.Seq,
		"workload_class": string(workloadClass),
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
	event.Payload[rexkeys.RexAdmissionTenantID] = strings.TrimSpace(req.TenantID)
	event.Payload[rexkeys.RexWorkloadClass] = string(req.Class)
}

func recordAdmissionDecision(audit *rexcontrolplane.AuditLog, req rexcontrolplane.AdmissionRequest, decision rexcontrolplane.AdmissionDecision, now time.Time) {
	if audit == nil {
		return
	}
	audit.Append(rexcontrolplane.AuditRecord{
		Action:    "gateway_route_admission",
		Role:      "gateway",
		TenantID:  strings.TrimSpace(req.TenantID),
		Allowed:   decision.Allowed,
		Reason:    decision.Reason,
		Timestamp: now.UTC(),
	})
}

func admissionRequestFromContext(ctx map[string]any) (rexcontrolplane.AdmissionRequest, bool) {
	if len(ctx) == 0 {
		return rexcontrolplane.AdmissionRequest{}, false
	}
	tenantID := strings.TrimSpace(stringValue(ctx[rexkeys.RexAdmissionTenantID]))
	if tenantID == "" {
		tenantID = strings.TrimSpace(stringValue(ctx[rexkeys.GatewayTenantID]))
	}
	if tenantID == "" {
		return rexcontrolplane.AdmissionRequest{}, false
	}
	class := rexcontrolplane.WorkloadBestEffort
	switch strings.TrimSpace(stringValue(ctx[rexkeys.RexWorkloadClass])) {
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
	trusted, _ := rexctx.TrustedExecutionContextFromContext(ctx)
	task := rexevents.ToTask(event)
	state := core.NewContext()
	for key, value := range task.Context {
		state.Set(key, value)
	}
	if strings.TrimSpace(trusted.TenantID) != "" {
		task.Context[rexkeys.RexAdmissionTenantID] = trusted.TenantID
		state.Set(rexkeys.RexAdmissionTenantID, trusted.TenantID)
	}
	if strings.TrimSpace(string(trusted.WorkloadClass)) != "" {
		task.Context[rexkeys.RexWorkloadClass] = string(trusted.WorkloadClass)
		state.Set(rexkeys.RexWorkloadClass, string(trusted.WorkloadClass))
	}
	if strings.TrimSpace(trusted.SessionID) != "" {
		state.Set(rexkeys.GatewaySessionID, trusted.SessionID)
	}
	admissionReq, releaseAdmission := admissionRequestFromContext(task.Context)
	if decision.WorkflowID != "" {
		task.Context[rexkeys.WorkflowID] = decision.WorkflowID
		state.Set(rexkeys.WorkflowID, decision.WorkflowID)
		state.Set(rexkeys.RexWorkflowID, decision.WorkflowID)
	}
	if decision.RunID != "" {
		task.Context[rexkeys.RunID] = decision.RunID
		state.Set(rexkeys.RunID, decision.RunID)
		state.Set(rexkeys.RexRunID, decision.RunID)
	}
	state.Set(rexkeys.RexEventType, event.Type)
	state.Set(rexkeys.RexEventID, event.ID)
	state.Set(rexkeys.RexEventPartition, event.Partition)
	state.Set(rexkeys.RexEventIngressOrigin, event.IngressOrigin)
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
