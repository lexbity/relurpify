package graph

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/lexcodex/relurpify/framework/core"
)

type RuntimePersistenceStore interface {
	PutDeclarative(ctx context.Context, record DeclarativeRecord) error
	SearchDeclarative(ctx context.Context, query DeclarativeQuery) ([]DeclarativeRecord, error)
	PutProcedural(ctx context.Context, record ProceduralRecord) error
	SearchProcedural(ctx context.Context, query ProceduralQuery) ([]ProceduralRecord, error)
}

type DeclarativeKind string

const (
	DeclarativeKindFact             DeclarativeKind = "fact"
	DeclarativeKindDecision         DeclarativeKind = "decision"
	DeclarativeKindConstraint       DeclarativeKind = "constraint"
	DeclarativeKindPreference       DeclarativeKind = "preference"
	DeclarativeKindProjectKnowledge DeclarativeKind = "project-knowledge"
)

type ProceduralKind string

const (
	ProceduralKindRoutine               ProceduralKind = "routine"
	ProceduralKindCapabilityComposition ProceduralKind = "capability-composition"
	ProceduralKindRecoveryRoutine       ProceduralKind = "recovery-routine"
)

type DeclarativeRecord struct {
	RecordID    string
	Scope       string
	Kind        DeclarativeKind
	Title       string
	Content     string
	Summary     string
	TaskID      string
	WorkflowID  string
	ArtifactRef string
	Tags        []string
	Metadata    map[string]any
	Verified    bool
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

type ProceduralRecord struct {
	RoutineID              string
	Scope                  string
	Kind                   ProceduralKind
	Name                   string
	Description            string
	Summary                string
	TaskID                 string
	WorkflowID             string
	BodyRef                string
	InlineBody             string
	CapabilityDependencies []core.CapabilitySelector
	VerificationMetadata   map[string]any
	PolicySnapshotID       string
	Verified               bool
	Version                int
	ReuseCount             int
	CreatedAt              time.Time
	UpdatedAt              time.Time
}

type DeclarativeQuery struct {
	Query  string
	Scope  string
	Kinds  []DeclarativeKind
	TaskID string
	Limit  int
}

type ProceduralQuery struct {
	Query          string
	Scope          string
	Kinds          []ProceduralKind
	TaskID         string
	CapabilityName string
	Limit          int
}

type RuntimePersistencePolicy struct {
	Name                          string
	MaxDeclarativeSummaryBytes    int
	MaxDeclarativeContentBytes    int
	MaxProceduralSummaryBytes     int
	DeduplicateNearIdentical      bool
	AllowArtifactPersistence      bool
	RequireProceduralVerification bool
}

func DefaultRuntimePersistencePolicy() RuntimePersistencePolicy {
	return RuntimePersistencePolicy{
		Name:                          "runtime-default",
		MaxDeclarativeSummaryBytes:    1024,
		MaxDeclarativeContentBytes:    2048,
		MaxProceduralSummaryBytes:     1024,
		DeduplicateNearIdentical:      true,
		AllowArtifactPersistence:      true,
		RequireProceduralVerification: true,
	}
}

type PersistenceAction string

const (
	PersistenceActionCreated      PersistenceAction = "created"
	PersistenceActionUpdated      PersistenceAction = "updated"
	PersistenceActionSkipped      PersistenceAction = "skipped"
	PersistenceActionDeduplicated PersistenceAction = "deduplicated"
)

type PersistenceAuditRecord struct {
	EntryID      string
	Action       PersistenceAction
	Reason       string
	Summary      string
	MemoryClass  core.MemoryClass
	Scope        string
	SubjectID    string
	SubjectType  string
	TaskID       string
	WorkflowID   string
	OriginNodeID string
	PolicyName   string
	Metadata     map[string]any
	CreatedAt    time.Time
}

type PersistenceAuditSink interface {
	RecordPersistence(ctx context.Context, record PersistenceAuditRecord) error
}

type DeclarativePersistenceRequest struct {
	StateKey            string
	Scope               string
	Kind                DeclarativeKind
	Title               string
	TitleStateKey       string
	TitleField          string
	SummaryStateKey     string
	SummaryField        string
	ContentField        string
	ArtifactRefStateKey string
	Tags                []string
	Reason              string
}

type ProceduralPersistenceRequest struct {
	StateKey                    string
	Scope                       string
	Kind                        ProceduralKind
	Name                        string
	NameStateKey                string
	NameField                   string
	SummaryStateKey             string
	SummaryField                string
	DescriptionField            string
	BodyRefField                string
	InlineBodyField             string
	PolicySnapshotIDStateKey    string
	CapabilityDependenciesField string
	VerifiedField               string
	Reason                      string
}

type ArtifactPersistenceRequest struct {
	ArtifactRefStateKey string
	SummaryStateKey     string
	Reason              string
}

type PersistenceWriterNode struct {
	id           string
	Store        RuntimePersistenceStore
	ArtifactSink ArtifactSink
	AuditSink    PersistenceAuditSink
	Policy       RuntimePersistencePolicy
	TaskID       string
	WorkflowID   string
	StateKey     string
	Declarative  []DeclarativePersistenceRequest
	Procedural   []ProceduralPersistenceRequest
	Artifacts    []ArtifactPersistenceRequest
	Telemetry    core.Telemetry
}

func NewPersistenceWriterNode(id string, store RuntimePersistenceStore) *PersistenceWriterNode {
	if id == "" {
		id = "persistence_writer"
	}
	return &PersistenceWriterNode{
		id:       id,
		Store:    store,
		Policy:   DefaultRuntimePersistencePolicy(),
		StateKey: "graph.persistence",
	}
}

func (n *PersistenceWriterNode) ID() string     { return n.id }
func (n *PersistenceWriterNode) Type() NodeType { return NodeTypeSystem }
func (n *PersistenceWriterNode) Contract() NodeContract {
	return NodeContract{
		SideEffectClass: SideEffectLocal,
		Idempotency:     IdempotencyReplaySafe,
		ContextPolicy: core.StateBoundaryPolicy{
			ReadKeys:                 []string{"task.*", "workflow.*", "react.*", "planner.*", "graph.*"},
			WriteKeys:                []string{"graph.persistence"},
			AllowedMemoryClasses:     []core.MemoryClass{core.MemoryClassDeclarative, core.MemoryClassProcedural},
			AllowedDataClasses:       []core.StateDataClass{core.StateDataClassTaskMetadata, core.StateDataClassArtifactRef, core.StateDataClassStructuredState},
			MaxStateEntryBytes:       4096,
			MaxInlineCollectionItems: 16,
			PreferArtifactReferences: true,
		},
	}
}

func (n *PersistenceWriterNode) Execute(ctx context.Context, state *Context) (*Result, error) {
	taskID := n.resolveTaskID(state)
	workflowID := n.resolveWorkflowID(state)
	audits := make([]PersistenceAuditRecord, 0, len(n.Declarative)+len(n.Procedural)+len(n.Artifacts))

	for _, req := range n.Declarative {
		if audit := n.persistDeclarative(ctx, state, taskID, workflowID, req); audit.EntryID != "" {
			audits = append(audits, audit)
		}
	}
	for _, req := range n.Procedural {
		if audit := n.persistProcedural(ctx, state, taskID, workflowID, req); audit.EntryID != "" {
			audits = append(audits, audit)
		}
	}
	for _, req := range n.Artifacts {
		if audit := n.persistArtifact(ctx, state, taskID, workflowID, req); audit.EntryID != "" {
			audits = append(audits, audit)
		}
	}

	if state != nil {
		state.Set(n.StateKey, map[string]any{"policy": n.Policy.Name, "records": audits})
	}
	emitSystemNodeEvent(n.Telemetry, taskID, "persistence evaluated", map[string]any{"node": n.id, "records": len(audits), "policy": n.Policy.Name})
	return &Result{NodeID: n.id, Success: true, Data: map[string]any{"records": audits}}, nil
}

func (n *PersistenceWriterNode) resolveTaskID(state *Context) string {
	if strings.TrimSpace(n.TaskID) != "" {
		return strings.TrimSpace(n.TaskID)
	}
	if state == nil {
		return ""
	}
	return strings.TrimSpace(state.GetString("task.id"))
}

func (n *PersistenceWriterNode) resolveWorkflowID(state *Context) string {
	if strings.TrimSpace(n.WorkflowID) != "" {
		return strings.TrimSpace(n.WorkflowID)
	}
	if state == nil {
		return ""
	}
	return strings.TrimSpace(state.GetString("workflow.id"))
}

func (n *PersistenceWriterNode) persistDeclarative(ctx context.Context, state *Context, taskID, workflowID string, req DeclarativePersistenceRequest) PersistenceAuditRecord {
	audit := newPersistenceAuditRecord(n.Policy.Name, n.id, taskID, workflowID, req.Reason, core.MemoryClassDeclarative, req.Scope, "declarative")
	record, ok := buildDeclarativeRecord(state, taskID, workflowID, req, n.Policy)
	if !ok {
		audit.Action = PersistenceActionSkipped
		audit.Summary = "declarative candidate missing"
		return n.recordAudit(ctx, audit)
	}
	if reason, allowed := validateDeclarativeCandidate(n.Policy, record); !allowed {
		audit.Action = PersistenceActionSkipped
		audit.SubjectID = record.RecordID
		audit.Summary = reason
		audit.Metadata["kind"] = string(record.Kind)
		return n.recordAudit(ctx, audit)
	}
	if n.Store != nil && n.Policy.DeduplicateNearIdentical {
		existing, action := n.findDeclarativeDuplicate(ctx, record)
		if existing != nil {
			record.RecordID = existing.RecordID
			record.CreatedAt = existing.CreatedAt
			audit.Action = action
			audit.SubjectID = record.RecordID
			audit.Summary = record.Summary
			audit.Metadata["kind"] = string(record.Kind)
			audit.Metadata["deduplicated"] = true
			if err := n.Store.PutDeclarative(ctx, record); err != nil {
				audit.Action = PersistenceActionSkipped
				audit.Summary = err.Error()
			}
			return n.recordAudit(ctx, audit)
		}
	}
	audit.Action = PersistenceActionCreated
	audit.SubjectID = record.RecordID
	audit.Summary = record.Summary
	audit.Metadata["kind"] = string(record.Kind)
	if n.Store != nil {
		if err := n.Store.PutDeclarative(ctx, record); err != nil {
			audit.Action = PersistenceActionSkipped
			audit.Summary = err.Error()
		}
	}
	return n.recordAudit(ctx, audit)
}

func (n *PersistenceWriterNode) persistProcedural(ctx context.Context, state *Context, taskID, workflowID string, req ProceduralPersistenceRequest) PersistenceAuditRecord {
	audit := newPersistenceAuditRecord(n.Policy.Name, n.id, taskID, workflowID, req.Reason, core.MemoryClassProcedural, req.Scope, "procedural")
	record, ok := buildProceduralRecord(state, taskID, workflowID, req)
	if !ok {
		audit.Action = PersistenceActionSkipped
		audit.Summary = "procedural candidate missing"
		return n.recordAudit(ctx, audit)
	}
	if reason, allowed := validateProceduralCandidate(n.Policy, record); !allowed {
		audit.Action = PersistenceActionSkipped
		audit.SubjectID = record.RoutineID
		audit.Summary = reason
		audit.Metadata["kind"] = string(record.Kind)
		return n.recordAudit(ctx, audit)
	}
	if n.Store != nil && n.Policy.DeduplicateNearIdentical {
		if existing, action := n.findProceduralDuplicate(ctx, record); existing != nil {
			record.RoutineID = existing.RoutineID
			record.CreatedAt = existing.CreatedAt
			record.Version = existing.Version
			if action == PersistenceActionUpdated {
				record.Version++
			}
			record.ReuseCount = existing.ReuseCount
			audit.Action = action
			audit.SubjectID = record.RoutineID
			audit.Summary = record.Summary
			audit.Metadata["kind"] = string(record.Kind)
			if err := n.Store.PutProcedural(ctx, record); err != nil {
				audit.Action = PersistenceActionSkipped
				audit.Summary = err.Error()
			}
			return n.recordAudit(ctx, audit)
		}
	}
	audit.Action = PersistenceActionCreated
	audit.SubjectID = record.RoutineID
	audit.Summary = record.Summary
	audit.Metadata["kind"] = string(record.Kind)
	if n.Store != nil {
		if err := n.Store.PutProcedural(ctx, record); err != nil {
			audit.Action = PersistenceActionSkipped
			audit.Summary = err.Error()
		}
	}
	return n.recordAudit(ctx, audit)
}

func (n *PersistenceWriterNode) persistArtifact(ctx context.Context, state *Context, taskID, workflowID string, req ArtifactPersistenceRequest) PersistenceAuditRecord {
	audit := newPersistenceAuditRecord(n.Policy.Name, n.id, taskID, workflowID, req.Reason, "", "", "artifact")
	if !n.Policy.AllowArtifactPersistence || state == nil {
		audit.Action = PersistenceActionSkipped
		audit.Summary = "artifact persistence disabled"
		return n.recordAudit(ctx, audit)
	}
	raw, ok := state.Get(req.ArtifactRefStateKey)
	if !ok || raw == nil {
		audit.Action = PersistenceActionSkipped
		audit.Summary = "artifact reference missing"
		return n.recordAudit(ctx, audit)
	}
	ref, ok := raw.(core.ArtifactReference)
	if !ok {
		audit.Action = PersistenceActionSkipped
		audit.Summary = "artifact reference invalid"
		return n.recordAudit(ctx, audit)
	}
	audit.Action = PersistenceActionCreated
	audit.SubjectID = ref.ArtifactID
	audit.Summary = strings.TrimSpace(ref.Summary)
	audit.Metadata["artifact_kind"] = ref.Kind
	audit.Metadata["uri"] = ref.URI
	if n.ArtifactSink != nil {
		_ = n.ArtifactSink.SaveArtifact(ctx, ArtifactRecord{
			ArtifactID:   ref.ArtifactID,
			Kind:         ref.Kind,
			ContentType:  ref.ContentType,
			StorageKind:  ref.StorageKind,
			Summary:      ref.Summary,
			RawSizeBytes: ref.RawSizeBytes,
			Metadata: map[string]any{
				"reason":      req.Reason,
				"task_id":     taskID,
				"workflow_id": workflowID,
				"origin_node": n.id,
			},
			CreatedAt: time.Now().UTC(),
		})
	}
	return n.recordAudit(ctx, audit)
}

func (n *PersistenceWriterNode) findDeclarativeDuplicate(ctx context.Context, record DeclarativeRecord) (*DeclarativeRecord, PersistenceAction) {
	if n.Store == nil || strings.TrimSpace(record.Summary) == "" {
		return nil, ""
	}
	results, err := n.Store.SearchDeclarative(ctx, DeclarativeQuery{
		Query:  record.Summary,
		Scope:  record.Scope,
		Kinds:  []DeclarativeKind{record.Kind},
		TaskID: record.TaskID,
		Limit:  8,
	})
	if err != nil {
		return nil, ""
	}
	for _, existing := range results {
		if normalizeComparableText(existing.Summary) == normalizeComparableText(record.Summary) &&
			normalizeComparableText(existing.Title) == normalizeComparableText(record.Title) {
			return &existing, PersistenceActionDeduplicated
		}
	}
	return nil, ""
}

func (n *PersistenceWriterNode) findProceduralDuplicate(ctx context.Context, record ProceduralRecord) (*ProceduralRecord, PersistenceAction) {
	if n.Store == nil || strings.TrimSpace(record.Name) == "" {
		return nil, ""
	}
	results, err := n.Store.SearchProcedural(ctx, ProceduralQuery{
		Query:  record.Name,
		Scope:  record.Scope,
		Kinds:  []ProceduralKind{record.Kind},
		TaskID: record.TaskID,
		Limit:  8,
	})
	if err != nil {
		return nil, ""
	}
	for _, existing := range results {
		if normalizeComparableText(existing.Name) != normalizeComparableText(record.Name) {
			continue
		}
		if normalizeComparableText(existing.Summary) == normalizeComparableText(record.Summary) &&
			strings.TrimSpace(existing.BodyRef) == strings.TrimSpace(record.BodyRef) &&
			strings.TrimSpace(existing.InlineBody) == strings.TrimSpace(record.InlineBody) {
			return &existing, PersistenceActionDeduplicated
		}
		return &existing, PersistenceActionUpdated
	}
	return nil, ""
}

func (n *PersistenceWriterNode) recordAudit(ctx context.Context, audit PersistenceAuditRecord) PersistenceAuditRecord {
	if audit.EntryID != "" && n.AuditSink != nil {
		_ = n.AuditSink.RecordPersistence(ctx, audit)
	}
	return audit
}

func newPersistenceAuditRecord(policyName, nodeID, taskID, workflowID, reason string, class core.MemoryClass, scope, subjectType string) PersistenceAuditRecord {
	now := time.Now().UTC()
	return PersistenceAuditRecord{
		EntryID:      fmt.Sprintf("persist_%d", now.UnixNano()),
		Reason:       strings.TrimSpace(reason),
		MemoryClass:  class,
		Scope:        scope,
		SubjectType:  subjectType,
		TaskID:       strings.TrimSpace(taskID),
		WorkflowID:   strings.TrimSpace(workflowID),
		OriginNodeID: nodeID,
		PolicyName:   strings.TrimSpace(policyName),
		Metadata:     map[string]any{},
		CreatedAt:    now,
	}
}

func buildDeclarativeRecord(state *Context, taskID, workflowID string, req DeclarativePersistenceRequest, policy RuntimePersistencePolicy) (DeclarativeRecord, bool) {
	record := DeclarativeRecord{
		Scope:      req.Scope,
		Kind:       req.Kind,
		TaskID:     taskID,
		WorkflowID: workflowID,
		Tags:       append([]string{}, req.Tags...),
		Metadata:   map[string]any{"state_key": req.StateKey, "reason": req.Reason},
	}
	raw, ok := stateValue(state, req.StateKey)
	if !ok {
		return record, false
	}
	title := firstNonEmpty(req.Title, stateString(state, req.TitleStateKey), fieldString(raw, req.TitleField))
	summary := firstNonEmpty(stateString(state, req.SummaryStateKey), fieldString(raw, req.SummaryField))
	if summary == "" {
		summary = compactSummaryFromValue(raw)
	}
	record.Title = title
	record.Summary = summary
	if req.ContentField != "" {
		record.Content = compactContentJSON(fieldValue(raw, req.ContentField), policy.MaxDeclarativeContentBytes)
	}
	if record.Content == "" {
		record.Content = compactContentJSON(raw, policy.MaxDeclarativeContentBytes)
	}
	if ref, ok := stateArtifactReference(state, req.ArtifactRefStateKey); ok {
		record.ArtifactRef = ref.URI
		if record.ArtifactRef == "" {
			record.ArtifactRef = ref.ArtifactID
		}
	}
	record.RecordID = stablePersistenceID("decl", record.Scope, string(record.Kind), taskID, title, summary)
	return record, true
}

func buildProceduralRecord(state *Context, taskID, workflowID string, req ProceduralPersistenceRequest) (ProceduralRecord, bool) {
	record := ProceduralRecord{
		Scope:      req.Scope,
		Kind:       req.Kind,
		TaskID:     taskID,
		WorkflowID: workflowID,
		VerificationMetadata: map[string]any{
			"state_key": req.StateKey,
			"reason":    req.Reason,
		},
		Version: 1,
	}
	raw, ok := stateValue(state, req.StateKey)
	if !ok {
		return record, false
	}
	record.Name = firstNonEmpty(req.Name, stateString(state, req.NameStateKey), fieldString(raw, req.NameField))
	record.Summary = firstNonEmpty(stateString(state, req.SummaryStateKey), fieldString(raw, req.SummaryField))
	record.Description = fieldString(raw, req.DescriptionField)
	record.BodyRef = fieldString(raw, req.BodyRefField)
	record.InlineBody = fieldString(raw, req.InlineBodyField)
	record.PolicySnapshotID = firstNonEmpty(stateString(state, req.PolicySnapshotIDStateKey))
	record.Verified = fieldBool(raw, req.VerifiedField)
	if deps, ok := fieldCapabilitySelectors(raw, req.CapabilityDependenciesField); ok {
		record.CapabilityDependencies = deps
	}
	record.RoutineID = stablePersistenceID("proc", record.Scope, string(record.Kind), taskID, record.Name, record.Summary)
	return record, true
}

func validateDeclarativeCandidate(policy RuntimePersistencePolicy, record DeclarativeRecord) (string, bool) {
	if strings.TrimSpace(record.Summary) == "" {
		return "missing declarative summary", false
	}
	if len(record.Summary) > policy.MaxDeclarativeSummaryBytes {
		return "declarative summary too large", false
	}
	if looksLikeTranscript(record.Summary) || looksLikeTranscript(record.Content) {
		return "raw transcript persistence blocked", false
	}
	return "", true
}

func validateProceduralCandidate(policy RuntimePersistencePolicy, record ProceduralRecord) (string, bool) {
	if strings.TrimSpace(record.Name) == "" || strings.TrimSpace(record.Summary) == "" {
		return "procedural routine requires name and summary", false
	}
	if len(record.Summary) > policy.MaxProceduralSummaryBytes {
		return "procedural summary too large", false
	}
	if policy.RequireProceduralVerification && !record.Verified {
		return "procedural routine must be verified", false
	}
	if strings.TrimSpace(record.BodyRef) == "" && strings.TrimSpace(record.InlineBody) == "" {
		return "procedural routine requires body or body reference", false
	}
	return "", true
}

func normalizeComparableText(value string) string {
	return strings.Join(strings.Fields(strings.ToLower(strings.TrimSpace(value))), " ")
}

func stablePersistenceID(prefix string, values ...string) string {
	h := sha1.New()
	for _, value := range values {
		_, _ = h.Write([]byte("\x00"))
		_, _ = h.Write([]byte(normalizeComparableText(value)))
	}
	return fmt.Sprintf("%s_%s", prefix, hex.EncodeToString(h.Sum(nil))[:16])
}

func compactContentJSON(value any, maxBytes int) string {
	if value == nil || maxBytes <= 0 {
		return ""
	}
	data, err := json.Marshal(value)
	if err != nil || len(data) > maxBytes {
		return ""
	}
	return string(data)
}

func looksLikeTranscript(value string) bool {
	lower := strings.ToLower(value)
	return (strings.Contains(lower, "user:") && strings.Contains(lower, "assistant:")) ||
		strings.Contains(lower, "\"history\"") ||
		strings.Contains(lower, "\"interactions\"")
}

func compactSummaryFromValue(value any) string {
	switch typed := value.(type) {
	case string:
		return strings.TrimSpace(typed)
	case map[string]any:
		for _, key := range []string{"summary", "title", "description", "task", "decision"} {
			if raw, ok := typed[key]; ok {
				if text := strings.TrimSpace(fmt.Sprint(raw)); text != "" {
					return text
				}
			}
		}
	}
	return strings.TrimSpace(fmt.Sprint(value))
}

func stateValue(state *Context, key string) (any, bool) {
	if state == nil || strings.TrimSpace(key) == "" {
		return nil, false
	}
	return state.Get(key)
}

func stateString(state *Context, key string) string {
	if state == nil || strings.TrimSpace(key) == "" {
		return ""
	}
	return strings.TrimSpace(state.GetString(key))
}

func stateArtifactReference(state *Context, key string) (core.ArtifactReference, bool) {
	if state == nil || strings.TrimSpace(key) == "" {
		return core.ArtifactReference{}, false
	}
	raw, ok := state.Get(key)
	if !ok || raw == nil {
		return core.ArtifactReference{}, false
	}
	ref, ok := raw.(core.ArtifactReference)
	return ref, ok
}

func fieldValue(value any, field string) any {
	if strings.TrimSpace(field) == "" {
		return nil
	}
	values, ok := value.(map[string]any)
	if !ok {
		return nil
	}
	return values[field]
}

func fieldString(value any, field string) string {
	if raw := fieldValue(value, field); raw != nil {
		return strings.TrimSpace(fmt.Sprint(raw))
	}
	return ""
}

func fieldBool(value any, field string) bool {
	switch typed := fieldValue(value, field).(type) {
	case bool:
		return typed
	case string:
		return strings.EqualFold(strings.TrimSpace(typed), "true")
	default:
		return false
	}
}

func fieldCapabilitySelectors(value any, field string) ([]core.CapabilitySelector, bool) {
	selectors, ok := fieldValue(value, field).([]core.CapabilitySelector)
	return selectors, ok
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
