package events

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	archaeodomain "codeburg.org/lexbit/relurpify/archaeo/domain"
	"codeburg.org/lexbit/relurpify/framework/core"
	frameworkevent "codeburg.org/lexbit/relurpify/framework/event"
	"codeburg.org/lexbit/relurpify/framework/memory"
	memorydb "codeburg.org/lexbit/relurpify/framework/memory/db"
)

type workflowEventPayload struct {
	EventID    string         `json:"event_id,omitempty"`
	WorkflowID string         `json:"workflow_id,omitempty"`
	RunID      string         `json:"run_id,omitempty"`
	StepID     string         `json:"step_id,omitempty"`
	Message    string         `json:"message,omitempty"`
	Metadata   map[string]any `json:"metadata,omitempty"`
}

const (
	EventWorkflowPhaseTransitioned    = "archaeo.workflow_phase_transitioned"
	EventExplorationSessionUpserted   = "archaeo.exploration_session_upserted"
	EventExplorationSnapshotUpserted  = "archaeo.exploration_snapshot_upserted"
	EventLearningInteractionRequested = "archaeo.learning_interaction_requested"
	EventLearningInteractionResolved  = "archaeo.learning_interaction_resolved"
	EventLearningInteractionExpired   = "archaeo.learning_interaction_expired"
	EventTensionUpserted              = "archaeo.tension_upserted"
	EventPlanVersionUpserted          = "archaeo.plan_version_upserted"
	EventPlanVersionActivated         = "archaeo.plan_version_activated"
	EventPlanVersionArchived          = "archaeo.plan_version_archived"
	EventConvergenceVerified          = "archaeo.convergence_verified"
	EventConvergenceFailed            = "archaeo.convergence_failed"
	EventExecutionHandoffRecorded     = "archaeo.execution_handoff_recorded"
	EventMutationRecorded             = "archaeo.mutation_recorded"
	EventRequestCreated               = "archaeo.request_created"
	EventRequestDispatched            = "archaeo.request_dispatched"
	EventRequestStarted               = "archaeo.request_started"
	EventRequestCompleted             = "archaeo.request_completed"
	EventRequestFailed                = "archaeo.request_failed"
	EventRequestCanceled              = "archaeo.request_canceled"
	EventDeferredDraftUpserted        = "archaeo.deferred_draft_upserted"
	EventConvergenceRecordUpserted    = "archaeo.convergence_record_upserted"
	EventDecisionRecordUpserted       = "archaeo.decision_record_upserted"

	projectionSnapshotArtifactKind = "archaeo_projection_snapshot"
	projectionSnapshotArtifactID   = "archaeo-projection-snapshot"
	snapshotPartitionSeparator     = "::snapshot:"
)

func AppendWorkflowEvent(ctx context.Context, store memory.WorkflowStateStore, workflowID, eventType, message string, metadata map[string]any, now time.Time) error {
	if store == nil || strings.TrimSpace(workflowID) == "" || strings.TrimSpace(eventType) == "" {
		return nil
	}
	seq, err := nextSeq(ctx, store, workflowID)
	if err != nil {
		return err
	}
	payload := cloneMetadata(metadata)
	payload["archaeo_seq"] = seq
	return store.AppendEvent(ctx, memory.WorkflowEventRecord{
		EventID:    fmt.Sprintf("archaeo-%s-%d", strings.TrimSpace(workflowID), seq),
		WorkflowID: strings.TrimSpace(workflowID),
		EventType:  strings.TrimSpace(eventType),
		Message:    strings.TrimSpace(message),
		Metadata:   payload,
		CreatedAt:  ensureTime(now),
	})
}

func AppendMutationEvent(ctx context.Context, store memory.WorkflowStateStore, mutation archaeodomain.MutationEvent) error {
	workflowID := strings.TrimSpace(mutation.WorkflowID)
	if store == nil || workflowID == "" {
		return nil
	}
	now := ensureTime(mutation.CreatedAt)
	if strings.TrimSpace(mutation.ID) == "" {
		seq, err := nextSeq(ctx, store, workflowID)
		if err != nil {
			return err
		}
		mutation.ID = fmt.Sprintf("mutation-%s-%d", workflowID, seq)
	}
	metadata := cloneMetadata(mutation.Metadata)
	metadata["mutation_id"] = mutation.ID
	metadata["category"] = string(mutation.Category)
	metadata["source_kind"] = strings.TrimSpace(mutation.SourceKind)
	metadata["source_ref"] = strings.TrimSpace(mutation.SourceRef)
	metadata["impact"] = string(mutation.Impact)
	metadata["disposition"] = string(mutation.Disposition)
	metadata["blocking"] = mutation.Blocking
	metadata["plan_id"] = strings.TrimSpace(mutation.PlanID)
	metadata["plan_version"] = mutation.PlanVersion
	metadata["exploration_id"] = strings.TrimSpace(mutation.ExplorationID)
	metadata["step_id"] = strings.TrimSpace(mutation.StepID)
	metadata["based_on_revision"] = strings.TrimSpace(mutation.BasedOnRevision)
	metadata["semantic_snapshot_ref"] = strings.TrimSpace(mutation.SemanticSnapshotRef)
	metadata["blast_radius_scope"] = string(mutation.BlastRadius.Scope)
	metadata["affected_step_ids"] = append([]string(nil), mutation.BlastRadius.AffectedStepIDs...)
	metadata["affected_symbol_ids"] = append([]string(nil), mutation.BlastRadius.AffectedSymbolIDs...)
	metadata["affected_pattern_ids"] = append([]string(nil), mutation.BlastRadius.AffectedPatternIDs...)
	metadata["affected_anchor_refs"] = append([]string(nil), mutation.BlastRadius.AffectedAnchorRefs...)
	metadata["affected_node_ids"] = append([]string(nil), mutation.BlastRadius.AffectedNodeIDs...)
	metadata["estimated_count"] = mutation.BlastRadius.EstimatedCount
	return AppendWorkflowEvent(ctx, store, workflowID, EventMutationRecorded, strings.TrimSpace(mutation.Description), metadata, now)
}

func ReadMutationEvents(ctx context.Context, store memory.WorkflowStateStore, workflowID string) ([]archaeodomain.MutationEvent, error) {
	log := &WorkflowLog{Store: store}
	records, err := log.ReadRecords(ctx, workflowID)
	if err != nil {
		return nil, err
	}
	out := make([]archaeodomain.MutationEvent, 0)
	for _, record := range records {
		if record.EventType != EventMutationRecorded {
			continue
		}
		out = append(out, mutationFromRecord(record))
	}
	return out, nil
}

func AppendRequestEvent(ctx context.Context, store memory.WorkflowStateStore, request archaeodomain.RequestRecord, eventType, message string, metadata map[string]any, now time.Time) error {
	workflowID := strings.TrimSpace(request.WorkflowID)
	if store == nil || workflowID == "" || strings.TrimSpace(eventType) == "" {
		return nil
	}
	payload := cloneMetadata(metadata)
	payload["request_id"] = strings.TrimSpace(request.ID)
	payload["request_kind"] = string(request.Kind)
	payload["request_status"] = string(request.Status)
	payload["exploration_id"] = strings.TrimSpace(request.ExplorationID)
	payload["snapshot_id"] = strings.TrimSpace(request.SnapshotID)
	payload["plan_id"] = strings.TrimSpace(request.PlanID)
	payload["plan_version"] = request.PlanVersion
	payload["requested_by"] = strings.TrimSpace(request.RequestedBy)
	payload["correlation_id"] = strings.TrimSpace(request.CorrelationID)
	payload["idempotency_key"] = strings.TrimSpace(request.IdempotencyKey)
	payload["claimed_by"] = strings.TrimSpace(request.ClaimedBy)
	payload["supersedes_request_id"] = strings.TrimSpace(request.SupersedesRequestID)
	payload["invalidation_reason"] = strings.TrimSpace(request.InvalidationReason)
	payload["fulfillment_ref"] = strings.TrimSpace(request.FulfillmentRef)
	payload["subject_refs"] = append([]string(nil), request.SubjectRefs...)
	payload["based_on_revision"] = strings.TrimSpace(request.BasedOnRevision)
	payload["attempt"] = request.Attempt
	payload["retry_count"] = request.RetryCount
	if request.Result != nil {
		payload["result_kind"] = strings.TrimSpace(request.Result.Kind)
		payload["result_ref_id"] = strings.TrimSpace(request.Result.RefID)
		payload["result_summary"] = strings.TrimSpace(request.Result.Summary)
	}
	if request.Fulfillment != nil {
		payload["fulfillment_kind"] = strings.TrimSpace(request.Fulfillment.Kind)
		payload["fulfillment_validity"] = string(request.Fulfillment.Validity)
		payload["fulfillment_applied"] = request.Fulfillment.Applied
		payload["fulfillment_rejected_reason"] = strings.TrimSpace(request.Fulfillment.RejectedReason)
		payload["fulfillment_executor"] = strings.TrimSpace(request.Fulfillment.ExecutorRef)
		payload["fulfillment_session"] = strings.TrimSpace(request.Fulfillment.SessionRef)
	}
	return AppendWorkflowEvent(ctx, store, workflowID, eventType, strings.TrimSpace(message), payload, now)
}

type WorkflowLog struct {
	Store memory.WorkflowStateStore
	Now   func() time.Time
}

var _ frameworkevent.Log = (*WorkflowLog)(nil)

func (l *WorkflowLog) Append(ctx context.Context, partition string, events []core.FrameworkEvent) ([]uint64, error) {
	workflowID, _ := normalizePartition(partition)
	if l == nil || l.Store == nil || workflowID == "" || len(events) == 0 {
		return nil, nil
	}
	next, err := nextSeq(ctx, l.Store, workflowID)
	if err != nil {
		return nil, err
	}
	seqs := make([]uint64, 0, len(events))
	for _, ev := range events {
		record := memory.WorkflowEventRecord{
			EventID:    fmt.Sprintf("archaeo-%s-%d", workflowID, next),
			WorkflowID: workflowID,
			EventType:  strings.TrimSpace(ev.Type),
			Message:    strings.TrimSpace(string(ev.Payload)),
			Metadata: map[string]any{
				"archaeo_seq":           next,
				"framework_payload_raw": string(ev.Payload),
				"framework_actor_kind":  ev.Actor.Kind,
				"framework_actor_id":    ev.Actor.ID,
				"framework_partition":   workflowID,
			},
			CreatedAt: ensureTime(ev.Timestamp),
		}
		if err := l.Store.AppendEvent(ctx, record); err != nil {
			return nil, err
		}
		seqs = append(seqs, next)
		next++
	}
	return seqs, nil
}

func (l *WorkflowLog) Read(ctx context.Context, partition string, afterSeq uint64, limit int, _ bool) ([]core.FrameworkEvent, error) {
	workflowID, _ := normalizePartition(partition)
	records, err := l.listAscending(ctx, workflowID)
	if err != nil {
		return nil, err
	}
	out := make([]core.FrameworkEvent, 0, len(records))
	for _, record := range records {
		ev, ok := toFrameworkEvent(record)
		if !ok || ev.Seq <= afterSeq {
			continue
		}
		out = append(out, ev)
		if limit > 0 && len(out) >= limit {
			break
		}
	}
	return out, nil
}

func (l *WorkflowLog) ReadRecords(ctx context.Context, partition string) ([]memory.WorkflowEventRecord, error) {
	workflowID, _ := normalizePartition(partition)
	return l.listAscending(ctx, workflowID)
}

func (l *WorkflowLog) LatestRecordByTypes(ctx context.Context, partition string, eventTypes ...string) (*memory.WorkflowEventRecord, bool, error) {
	workflowID, _ := normalizePartition(partition)
	if l == nil || l.Store == nil || workflowID == "" || len(eventTypes) == 0 {
		return nil, false, nil
	}
	if typed, ok := l.Store.(*memorydb.SQLiteWorkflowStateStore); ok {
		return typed.LatestEventByTypes(ctx, workflowID, eventTypes...)
	}
	records, err := l.listAscending(ctx, workflowID)
	if err != nil {
		return nil, false, err
	}
	allowed := make(map[string]struct{}, len(eventTypes))
	for _, eventType := range eventTypes {
		if strings.TrimSpace(eventType) != "" {
			allowed[strings.TrimSpace(eventType)] = struct{}{}
		}
	}
	for i := len(records) - 1; i >= 0; i-- {
		if _, ok := allowed[records[i].EventType]; ok {
			record := records[i]
			return &record, true, nil
		}
	}
	return nil, false, nil
}

func (l *WorkflowLog) ReadByType(ctx context.Context, partition string, typePrefix string, afterSeq uint64, limit int) ([]core.FrameworkEvent, error) {
	events, err := l.Read(ctx, partition, afterSeq, 0, false)
	if err != nil {
		return nil, err
	}
	out := make([]core.FrameworkEvent, 0, len(events))
	for _, ev := range events {
		if !strings.HasPrefix(ev.Type, typePrefix) {
			continue
		}
		out = append(out, ev)
		if limit > 0 && len(out) >= limit {
			break
		}
	}
	return out, nil
}

func (l *WorkflowLog) LastSeq(ctx context.Context, partition string) (uint64, error) {
	workflowID, _ := normalizePartition(partition)
	records, err := l.listAscending(ctx, workflowID)
	if err != nil || len(records) == 0 {
		return 0, err
	}
	last, ok := toFrameworkEvent(records[len(records)-1])
	if !ok {
		return uint64(len(records)), nil
	}
	return last.Seq, nil
}

func (l *WorkflowLog) TakeSnapshot(ctx context.Context, partition string, seq uint64, data []byte) error {
	workflowID, snapshotKey := normalizePartition(partition)
	if l == nil || l.Store == nil || workflowID == "" {
		return nil
	}
	return l.Store.UpsertWorkflowArtifact(ctx, memory.WorkflowArtifactRecord{
		ArtifactID:      fmt.Sprintf("%s:%s:%s", projectionSnapshotArtifactID, workflowID, snapshotKey),
		WorkflowID:      workflowID,
		Kind:            projectionSnapshotArtifactKind,
		ContentType:     "application/json",
		StorageKind:     memory.ArtifactStorageInline,
		SummaryText:     fmt.Sprintf("archaeo projection snapshot %s @%d", snapshotKey, seq),
		SummaryMetadata: map[string]any{"seq": seq, "snapshot_key": snapshotKey},
		InlineRawText:   string(data),
		CreatedAt:       l.now(),
	})
}

func (l *WorkflowLog) LoadSnapshot(ctx context.Context, partition string) (uint64, []byte, error) {
	workflowID, snapshotKey := normalizePartition(partition)
	if l == nil || l.Store == nil || workflowID == "" {
		return 0, nil, nil
	}
	artifactID := fmt.Sprintf("%s:%s:%s", projectionSnapshotArtifactID, workflowID, snapshotKey)
	if typed, ok := l.Store.(*memorydb.SQLiteWorkflowStateStore); ok {
		artifact, found, err := typed.WorkflowArtifactByID(ctx, artifactID)
		if err != nil {
			return 0, nil, err
		}
		if found && artifact != nil {
			return uint64Value(artifact.SummaryMetadata["seq"]), []byte(artifact.InlineRawText), nil
		}
	}
	artifacts, err := l.Store.ListWorkflowArtifacts(ctx, workflowID, "")
	if err != nil {
		return 0, nil, err
	}
	for i := len(artifacts) - 1; i >= 0; i-- {
		if artifacts[i].Kind != projectionSnapshotArtifactKind {
			continue
		}
		if stringValue(metadataValue(artifacts[i].SummaryMetadata, "snapshot_key")) != snapshotKey {
			continue
		}
		seq := uint64(0)
		if raw, ok := artifacts[i].SummaryMetadata["seq"]; ok {
			seq = uint64Value(raw)
		}
		return seq, []byte(artifacts[i].InlineRawText), nil
	}
	return 0, nil, nil
}

func (l *WorkflowLog) Close() error { return nil }

func (l *WorkflowLog) now() time.Time {
	if l != nil && l.Now != nil {
		return l.Now().UTC()
	}
	return time.Now().UTC()
}

func (l *WorkflowLog) listAscending(ctx context.Context, workflowID string) ([]memory.WorkflowEventRecord, error) {
	workflowID, _ = normalizePartition(workflowID)
	if l == nil || l.Store == nil || strings.TrimSpace(workflowID) == "" {
		return nil, nil
	}
	records, err := l.Store.ListEvents(ctx, strings.TrimSpace(workflowID), 0)
	if err != nil {
		return nil, err
	}
	sort.SliceStable(records, func(i, j int) bool {
		if records[i].CreatedAt.Equal(records[j].CreatedAt) {
			return uint64Value(metadataValue(records[i].Metadata, "archaeo_seq")) < uint64Value(metadataValue(records[j].Metadata, "archaeo_seq"))
		}
		return records[i].CreatedAt.Before(records[j].CreatedAt)
	})
	return records, nil
}

func normalizePartition(partition string) (workflowID string, snapshotKey string) {
	trimmed := strings.TrimSpace(partition)
	if trimmed == "" {
		return "", "default"
	}
	parts := strings.SplitN(trimmed, snapshotPartitionSeparator, 2)
	workflowID = strings.TrimSpace(parts[0])
	snapshotKey = "default"
	if len(parts) == 2 && strings.TrimSpace(parts[1]) != "" {
		snapshotKey = strings.TrimSpace(parts[1])
	}
	return workflowID, snapshotKey
}

func toFrameworkEvent(record memory.WorkflowEventRecord) (core.FrameworkEvent, bool) {
	seq := uint64Value(metadataValue(record.Metadata, "archaeo_seq"))
	payload, err := json.Marshal(workflowEventPayload{
		EventID:    record.EventID,
		WorkflowID: record.WorkflowID,
		RunID:      record.RunID,
		StepID:     record.StepID,
		Message:    record.Message,
		Metadata:   record.Metadata,
	})
	if err != nil {
		return core.FrameworkEvent{}, false
	}
	return core.FrameworkEvent{
		Seq:       seq,
		Timestamp: record.CreatedAt,
		Type:      record.EventType,
		Payload:   payload,
		Actor:     core.EventActor{Kind: "archaeo", ID: record.WorkflowID},
		Partition: record.WorkflowID,
	}, true
}

func nextSeq(ctx context.Context, store memory.WorkflowStateStore, workflowID string) (uint64, error) {
	if typed, ok := store.(*memorydb.SQLiteWorkflowStateStore); ok && typed != nil {
		record, found, err := typed.LatestEvent(ctx, workflowID)
		if err != nil {
			return 0, err
		}
		if !found || record == nil {
			return 1, nil
		}
		if seq := uint64Value(metadataValue(record.Metadata, "archaeo_seq")); seq > 0 {
			return seq + 1, nil
		}
		return 1, nil
	}
	records, err := store.ListEvents(ctx, workflowID, 0)
	if err != nil {
		return 0, err
	}
	var maxSeq uint64
	for _, record := range records {
		if seq := uint64Value(metadataValue(record.Metadata, "archaeo_seq")); seq > maxSeq {
			maxSeq = seq
		}
	}
	return maxSeq + 1, nil
}

func cloneMetadata(in map[string]any) map[string]any {
	if len(in) == 0 {
		return map[string]any{}
	}
	out := make(map[string]any, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func metadataValue(metadata map[string]any, key string) any {
	if metadata == nil {
		return nil
	}
	return metadata[key]
}

func uint64Value(raw any) uint64 {
	switch typed := raw.(type) {
	case uint64:
		return typed
	case uint32:
		return uint64(typed)
	case uint:
		return uint64(typed)
	case int64:
		if typed > 0 {
			return uint64(typed)
		}
	case int:
		if typed > 0 {
			return uint64(typed)
		}
	case float64:
		if typed > 0 {
			return uint64(typed)
		}
	case json.Number:
		if value, err := typed.Int64(); err == nil && value > 0 {
			return uint64(value)
		}
	case string:
		if value, err := strconv.ParseUint(strings.TrimSpace(typed), 10, 64); err == nil {
			return value
		}
	}
	return 0
}

func stringValue(raw any) string {
	if typed, ok := raw.(string); ok {
		return strings.TrimSpace(typed)
	}
	return ""
}

func ensureTime(value time.Time) time.Time {
	if value.IsZero() {
		return time.Now().UTC()
	}
	return value.UTC()
}

func NowUTC() time.Time {
	return time.Now().UTC()
}

func mutationFromRecord(record memory.WorkflowEventRecord) archaeodomain.MutationEvent {
	event := archaeodomain.MutationEvent{
		ID:                  strings.TrimSpace(stringValue(metadataValue(record.Metadata, "mutation_id"))),
		WorkflowID:          strings.TrimSpace(record.WorkflowID),
		ExplorationID:       strings.TrimSpace(stringValue(metadataValue(record.Metadata, "exploration_id"))),
		PlanID:              strings.TrimSpace(stringValue(metadataValue(record.Metadata, "plan_id"))),
		PlanVersion:         intPointer(metadataValue(record.Metadata, "plan_version")),
		StepID:              strings.TrimSpace(stringValue(metadataValue(record.Metadata, "step_id"))),
		Category:            archaeodomain.MutationCategory(strings.TrimSpace(stringValue(metadataValue(record.Metadata, "category")))),
		SourceKind:          strings.TrimSpace(stringValue(metadataValue(record.Metadata, "source_kind"))),
		SourceRef:           strings.TrimSpace(stringValue(metadataValue(record.Metadata, "source_ref"))),
		Description:         strings.TrimSpace(record.Message),
		Impact:              archaeodomain.MutationImpact(strings.TrimSpace(stringValue(metadataValue(record.Metadata, "impact")))),
		Disposition:         archaeodomain.ExecutionDisposition(strings.TrimSpace(stringValue(metadataValue(record.Metadata, "disposition")))),
		Blocking:            boolValue(metadataValue(record.Metadata, "blocking")),
		BasedOnRevision:     strings.TrimSpace(stringValue(metadataValue(record.Metadata, "based_on_revision"))),
		SemanticSnapshotRef: strings.TrimSpace(stringValue(metadataValue(record.Metadata, "semantic_snapshot_ref"))),
		Metadata:            cloneMetadata(record.Metadata),
		CreatedAt:           ensureTime(record.CreatedAt),
	}
	if event.ID == "" {
		event.ID = strings.TrimSpace(record.EventID)
	}
	event.BlastRadius = archaeodomain.BlastRadius{
		Scope:              archaeodomain.BlastRadiusScope(strings.TrimSpace(stringValue(metadataValue(record.Metadata, "blast_radius_scope")))),
		AffectedStepIDs:    stringSliceValue(metadataValue(record.Metadata, "affected_step_ids")),
		AffectedSymbolIDs:  stringSliceValue(metadataValue(record.Metadata, "affected_symbol_ids")),
		AffectedPatternIDs: stringSliceValue(metadataValue(record.Metadata, "affected_pattern_ids")),
		AffectedAnchorRefs: stringSliceValue(metadataValue(record.Metadata, "affected_anchor_refs")),
		AffectedNodeIDs:    stringSliceValue(metadataValue(record.Metadata, "affected_node_ids")),
		EstimatedCount:     intValue(metadataValue(record.Metadata, "estimated_count")),
	}
	return event
}

func intPointer(raw any) *int {
	value := intValue(raw)
	if value <= 0 {
		return nil
	}
	return &value
}

func intValue(raw any) int {
	switch typed := raw.(type) {
	case int:
		return typed
	case int64:
		return int(typed)
	case int32:
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

func boolValue(raw any) bool {
	switch typed := raw.(type) {
	case bool:
		return typed
	case string:
		return strings.EqualFold(strings.TrimSpace(typed), "true")
	}
	return false
}

func stringSliceValue(raw any) []string {
	switch typed := raw.(type) {
	case []string:
		return append([]string(nil), typed...)
	case []any:
		out := make([]string, 0, len(typed))
		for _, item := range typed {
			if value := stringValue(item); value != "" {
				out = append(out, value)
			}
		}
		return out
	}
	return nil
}
