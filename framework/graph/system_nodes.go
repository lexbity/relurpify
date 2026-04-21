package graph

import (
	"context"
	"fmt"
	"reflect"
	"sort"
	"strings"
	"time"

	"codeburg.org/lexbit/relurpify/framework/core"
)

// ArtifactRecord captures a durable graph artifact without binding graph to a
// concrete persistence implementation.
type ArtifactRecord struct {
	ArtifactID   string
	Kind         string
	ContentType  string
	StorageKind  string
	Summary      string
	RawText      string
	RawSizeBytes int64
	Metadata     map[string]any
	CreatedAt    time.Time
}

// ArtifactSink persists graph-produced artifacts.
type ArtifactSink interface {
	SaveArtifact(ctx context.Context, artifact ArtifactRecord) error
}

// CheckpointPersister persists graph-native checkpoints.
type CheckpointPersister interface {
	Save(checkpoint *GraphCheckpoint) error
}

// MemoryRetriever returns bounded, compact memory retrieval results.
type MemoryRetriever interface {
	Retrieve(ctx context.Context, query string, limit int) ([]core.MemoryRecordEnvelope, error)
}

// PublishedMemoryRetriever can return the richer graph publication shape
// directly once callers are ready for it.
type PublishedMemoryRetriever interface {
	RetrievePublication(ctx context.Context, query string, limit int) (*MemoryRetrievalPublication, error)
}

// MemoryRetrievalPublication is the richer retrieval publication contract used
// by graph memory nodes once mixed-evidence consumers exist.
type MemoryRetrievalPublication struct {
	Query      string
	Results    []core.MemoryRecordEnvelope
	References []core.MemoryReference
	Payload    map[string]any
	Refs       []core.ContextReference
}

// StateHydrator restores selected durable references into active state.
type StateHydrator interface {
	Hydrate(ctx context.Context, refs []string) (map[string]any, error)
}

// CheckpointNode persists a checkpoint artifact and records a reference in state.
type CheckpointNode struct {
	id               string
	TaskID           string
	NextNodeID       string
	Persister        CheckpointPersister
	ArtifactSink     ArtifactSink
	StateKey         string
	ArtifactStateKey string
	Telemetry        core.Telemetry
	Metadata         map[string]any
}

func NewCheckpointNode(id string, nextNodeID string, persister CheckpointPersister) *CheckpointNode {
	if id == "" {
		id = "checkpoint"
	}
	return &CheckpointNode{
		id:               id,
		NextNodeID:       nextNodeID,
		Persister:        persister,
		StateKey:         "graph.checkpoint",
		ArtifactStateKey: "graph.checkpoint_ref",
	}
}

func (n *CheckpointNode) ID() string     { return n.id }
func (n *CheckpointNode) Type() NodeType { return NodeTypeSystem }
func (n *CheckpointNode) Contract() NodeContract {
	return NodeContract{
		SideEffectClass: SideEffectLocal,
		Idempotency:     IdempotencyReplaySafe,
		ContextPolicy: core.StateBoundaryPolicy{
			ReadKeys:                 []string{"task.*", "workflow.*", "graph.*"},
			WriteKeys:                []string{"graph.checkpoint*", "artifact.*"},
			AllowedMemoryClasses:     []core.MemoryClass{core.MemoryClassWorking},
			AllowedDataClasses:       []core.StateDataClass{core.StateDataClassTaskMetadata, core.StateDataClassArtifactRef, core.StateDataClassStructuredState},
			MaxStateEntryBytes:       4096,
			MaxInlineCollectionItems: 8,
			PreferArtifactReferences: true,
		},
	}
}

func (n *CheckpointNode) Execute(ctx context.Context, state *Context) (*Result, error) {
	taskID := strings.TrimSpace(n.TaskID)
	if taskID == "" && state != nil {
		taskID = strings.TrimSpace(state.GetString("task.id"))
	}
	checkpoint := &GraphCheckpoint{
		CheckpointID:    generateCheckpointID(),
		TaskID:          taskID,
		CreatedAt:       time.Now().UTC(),
		CurrentNodeID:   n.id,
		CompletedNodeID: n.id,
		NextNodeID:      n.NextNodeID,
		LastTransition: &NodeTransitionRecord{
			CompletedNodeID:  n.id,
			NextNodeID:       n.NextNodeID,
			TransitionReason: "explicit-checkpoint-node",
			CompletedAt:      time.Now().UTC(),
		},
		Context:   state.Clone(),
		GraphHash: "",
		Metadata:  cloneMapAny(n.Metadata),
	}
	if n.Persister != nil {
		if err := n.Persister.Save(checkpoint); err != nil {
			return nil, err
		}
	}
	ref := core.ArtifactReference{
		ArtifactID:   checkpoint.CheckpointID,
		Kind:         "checkpoint",
		ContentType:  "application/json",
		StorageKind:  "checkpoint",
		URI:          fmt.Sprintf("workflow://checkpoint/%s", checkpoint.CheckpointID),
		Summary:      fmt.Sprintf("checkpoint at %s -> %s", n.id, n.NextNodeID),
		RawSizeBytes: int64(len(fmt.Sprintf("%v", checkpoint.Metadata))),
	}
	if n.ArtifactSink != nil {
		_ = n.ArtifactSink.SaveArtifact(ctx, ArtifactRecord{
			ArtifactID:   checkpoint.CheckpointID,
			Kind:         "checkpoint",
			ContentType:  "application/json",
			StorageKind:  "checkpoint",
			Summary:      ref.Summary,
			RawText:      "",
			RawSizeBytes: ref.RawSizeBytes,
			Metadata: map[string]any{
				"task_id":           checkpoint.TaskID,
				"completed_node_id": checkpoint.CompletedNodeID,
				"next_node_id":      checkpoint.NextNodeID,
				"transition_reason": checkpoint.LastTransition.TransitionReason,
				"checkpoint_id":     checkpoint.CheckpointID,
			},
			CreatedAt: checkpoint.CreatedAt,
		})
	}
	if state != nil {
		state.Set(n.StateKey, map[string]any{
			"checkpoint_id":     checkpoint.CheckpointID,
			"task_id":           checkpoint.TaskID,
			"completed_node_id": checkpoint.CompletedNodeID,
			"next_node_id":      checkpoint.NextNodeID,
			"transition_reason": checkpoint.LastTransition.TransitionReason,
			"created_at":        checkpoint.CreatedAt,
		})
		state.Set(n.ArtifactStateKey, ref)
	}
	emitSystemNodeEvent(n.Telemetry, taskID, "checkpoint persisted", map[string]any{
		"node":          n.id,
		"checkpoint_id": checkpoint.CheckpointID,
		"next_node_id":  checkpoint.NextNodeID,
	})
	return &Result{
		NodeID:  n.id,
		Success: true,
		Data: map[string]any{
			"checkpoint_id": checkpoint.CheckpointID,
			"artifact_ref":  ref,
		},
	}, nil
}

// BuildMemoryRetrievalPublication derives the richer graph publication shape
// from legacy envelope results.
func BuildMemoryRetrievalPublication(query string, results []core.MemoryRecordEnvelope, expectedClass core.MemoryClass) *MemoryRetrievalPublication {
	if len(results) == 0 {
		return &MemoryRetrievalPublication{
			Query:      query,
			Results:    []core.MemoryRecordEnvelope{},
			References: []core.MemoryReference{},
			Payload:    nil,
			Refs:       nil,
		}
	}
	normalized := results
	needsNormalization := false
	for _, record := range results {
		if record.MemoryClass == "" {
			needsNormalization = true
			break
		}
	}
	if needsNormalization {
		normalized = append([]core.MemoryRecordEnvelope(nil), results...)
		for i := range normalized {
			if normalized[i].MemoryClass == "" {
				normalized[i].MemoryClass = expectedClass
			}
		}
	}
	references := make([]core.MemoryReference, 0, len(results))
	for _, record := range normalized {
		references = append(references, core.MemoryReference{
			MemoryClass: record.MemoryClass,
			Scope:       record.Scope,
			RecordKey:   record.Key,
			Summary:     record.Summary,
		})
	}
	return &MemoryRetrievalPublication{
		Query:      query,
		Results:    normalized,
		References: references,
		Payload:    mixedEvidencePayloadFromEnvelopes(query, normalized),
		Refs:       contextReferencesFromEnvelopes(normalized, expectedClass),
	}
}

func mixedEvidencePayloadFromEnvelopes(query string, results []core.MemoryRecordEnvelope) map[string]any {
	if len(results) == 0 {
		return nil
	}
	texts := make([]string, 0, len(results))
	entries := make([]map[string]any, 0, len(results))
	citationCount := 0
	for _, result := range results {
		summary := strings.TrimSpace(result.Summary)
		text := strings.TrimSpace(result.Text)
		if summary == "" && text == "" {
			continue
		}
		if summary == "" {
			summary = text
		}
		texts = append(texts, summary)
		entry := map[string]any{
			"summary": summary,
		}
		if text != "" {
			entry["text"] = text
		}
		if source := strings.TrimSpace(result.Source); source != "" {
			entry["source"] = source
		}
		if recordID := strings.TrimSpace(result.RecordID); recordID != "" {
			entry["record_id"] = recordID
		} else if key := strings.TrimSpace(result.Key); key != "" {
			entry["record_id"] = key
		}
		if kind := strings.TrimSpace(result.Kind); kind != "" {
			entry["kind"] = kind
		}
		if result.Reference != nil {
			entry["reference"] = result.Reference
		}
		switch typed := result.Citations.(type) {
		case []map[string]any:
			if len(typed) > 0 {
				entry["citations"] = typed
				citationCount += len(typed)
			}
		case []any:
			if len(typed) > 0 {
				entry["citations"] = typed
				citationCount += len(typed)
			}
		}
		entries = append(entries, entry)
	}
	if len(entries) == 0 {
		return nil
	}
	return map[string]any{
		"query":          query,
		"texts":          texts,
		"results":        entries,
		"summary":        strings.Join(texts, "\n\n"),
		"result_size":    len(entries),
		"citation_count": citationCount,
	}
}

func contextReferencesFromEnvelopes(results []core.MemoryRecordEnvelope, expectedClass core.MemoryClass) []core.ContextReference {
	if len(results) == 0 {
		return nil
	}
	refs := make([]core.ContextReference, 0, len(results))
	seen := make(map[contextReferenceKey]struct{}, len(results))
	for _, result := range results {
		ref := contextReferenceFromEnvelope(result, expectedClass)
		if ref == nil {
			continue
		}
		key := contextReferenceKey{kind: ref.Kind, id: ref.ID, uri: ref.URI}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		refs = append(refs, *ref)
	}
	if len(refs) == 0 {
		return nil
	}
	return refs
}

type contextReferenceKey struct {
	kind core.ContextReferenceKind
	id   string
	uri  string
}

func contextReferenceFromEnvelope(result core.MemoryRecordEnvelope, expectedClass core.MemoryClass) *core.ContextReference {
	values, ok := result.Reference.(map[string]any)
	if ok && len(values) > 0 {
		ref := &core.ContextReference{
			Kind:    core.ContextReferenceKind(trimmedAnyString(values["kind"])),
			ID:      trimmedAnyString(values["id"]),
			URI:     trimmedAnyString(values["uri"]),
			Version: trimmedAnyString(values["version"]),
			Detail:  trimmedAnyString(values["detail"]),
		}
		if ref.Kind == "" {
			ref.Kind = defaultContextReferenceKind(result, expectedClass)
		}
		recordID := trimmedAnyString(values["record_id"])
		source := trimmedAnyString(values["source"])
		kind := trimmedAnyString(values["kind"])
		if recordID != "" || source != "" || kind != "" {
			metadata := make(map[string]string, 3)
			if recordID != "" {
				metadata["record_id"] = recordID
			}
			if source != "" {
				metadata["source"] = source
			}
			if kind != "" {
				metadata["kind"] = kind
			}
			ref.Metadata = metadata
		}
		if ref.ID != "" || ref.URI != "" {
			return ref
		}
	}
	key := strings.TrimSpace(result.RecordID)
	if key == "" {
		key = strings.TrimSpace(result.Key)
	}
	if key == "" {
		return nil
	}
	return &core.ContextReference{
		Kind:   defaultContextReferenceKind(result, expectedClass),
		ID:     key,
		Detail: strings.TrimSpace(result.Kind),
		Metadata: map[string]string{
			"memory_class": string(nonEmptyMemoryClass(result.MemoryClass, expectedClass)),
			"source":       strings.TrimSpace(result.Source),
		},
	}
}

func defaultContextReferenceKind(result core.MemoryRecordEnvelope, expectedClass core.MemoryClass) core.ContextReferenceKind {
	if strings.TrimSpace(result.Source) == "retrieval" {
		return core.ContextReferenceRetrievalEvidence
	}
	return core.ContextReferenceRuntimeMemory
}

func nonEmptyMemoryClass(class core.MemoryClass, fallback core.MemoryClass) core.MemoryClass {
	if strings.TrimSpace(string(class)) != "" {
		return class
	}
	return fallback
}

func trimmedAnyString(value any) string {
	switch typed := value.(type) {
	case string:
		return strings.TrimSpace(typed)
	case fmt.Stringer:
		return strings.TrimSpace(typed.String())
	case nil:
		return ""
	default:
		formatted := strings.TrimSpace(fmt.Sprint(typed))
		if formatted == "<nil>" {
			return ""
		}
		return formatted
	}
}

func summarizeNodePayload(state *Context, keys []string, includeHistory bool) string {
	if state == nil {
		return ""
	}
	var parts []string
	for _, key := range keys {
		if raw, ok := state.Get(key); ok && raw != nil {
			parts = append(parts, fmt.Sprintf("%s: %v", key, boundedSummaryValue(raw, 0)))
		}
	}
	if includeHistory {
		history := state.History()
		start := 0
		if len(history) > 8 {
			start = len(history) - 8
		}
		for _, item := range history[start:] {
			parts = append(parts, fmt.Sprintf("%s: %s", item.Role, item.Content))
		}
	}
	return strings.Join(parts, "\n")
}

const (
	summaryMaxDepth           = 3
	summaryMaxMapItems        = 12
	summaryMaxCollectionItems = 8
	summaryMaxStringLen       = 512
)

func boundedSummaryValue(value any, depth int) any {
	if depth >= summaryMaxDepth {
		return summarizeLeafValue(value)
	}
	switch typed := value.(type) {
	case nil:
		return nil
	case string:
		return truncateSummaryString(typed)
	case []string:
		limit := minInt(len(typed), summaryMaxCollectionItems)
		out := make([]any, 0, limit+1)
		for i := 0; i < limit; i++ {
			out = append(out, truncateSummaryString(typed[i]))
		}
		if len(typed) > limit {
			out = append(out, fmt.Sprintf("... (%d more)", len(typed)-limit))
		}
		return out
	case []any:
		limit := minInt(len(typed), summaryMaxCollectionItems)
		out := make([]any, 0, limit+1)
		for i := 0; i < limit; i++ {
			out = append(out, boundedSummaryValue(typed[i], depth+1))
		}
		if len(typed) > limit {
			out = append(out, fmt.Sprintf("... (%d more)", len(typed)-limit))
		}
		return out
	case map[string]any:
		return boundedSummaryMap(typed, depth)
	case core.ArtifactReference:
		return map[string]any{
			"artifact_id": typed.ArtifactID,
			"kind":        typed.Kind,
			"summary":     truncateSummaryString(typed.Summary),
			"storage":     typed.StorageKind,
		}
	default:
		return boundedSummaryReflectValue(reflect.ValueOf(value), depth)
	}
}

func boundedSummaryReflectValue(value reflect.Value, depth int) any {
	if !value.IsValid() {
		return nil
	}
	if depth >= summaryMaxDepth {
		return summarizeLeafValue(reflectValueInterface(value))
	}
	for value.Kind() == reflect.Pointer || value.Kind() == reflect.Interface {
		if value.IsNil() {
			return nil
		}
		value = value.Elem()
	}
	switch value.Kind() {
	case reflect.Slice, reflect.Array:
		limit := minInt(value.Len(), summaryMaxCollectionItems)
		out := make([]any, 0, limit+1)
		for i := 0; i < limit; i++ {
			out = append(out, boundedSummaryReflectValue(value.Index(i), depth+1))
		}
		if value.Len() > limit {
			out = append(out, fmt.Sprintf("... (%d more)", value.Len()-limit))
		}
		return out
	case reflect.Map:
		if value.Type().Key().Kind() != reflect.String {
			return summarizeLeafValue(reflectValueInterface(value))
		}
		keys := value.MapKeys()
		keyNames := make([]string, 0, len(keys))
		for _, key := range keys {
			keyNames = append(keyNames, key.String())
		}
		sort.Strings(keyNames)
		limit := minInt(len(keyNames), summaryMaxMapItems)
		out := make(map[string]any, limit+1)
		for _, key := range keyNames[:limit] {
			out[key] = boundedSummaryReflectValue(value.MapIndex(reflect.ValueOf(key)), depth+1)
		}
		if len(keyNames) > limit {
			out["_truncated_keys"] = len(keyNames) - limit
		}
		return out
	case reflect.Struct:
		typ := value.Type()
		fieldNames := make([]string, 0, typ.NumField())
		fields := make(map[string]reflect.Value, typ.NumField())
		for i := 0; i < typ.NumField(); i++ {
			field := typ.Field(i)
			if field.PkgPath != "" {
				continue
			}
			fieldNames = append(fieldNames, field.Name)
			fields[field.Name] = value.Field(i)
		}
		sort.Strings(fieldNames)
		limit := minInt(len(fieldNames), summaryMaxMapItems)
		out := make(map[string]any, limit+1)
		for _, name := range fieldNames[:limit] {
			out[name] = boundedSummaryReflectValue(fields[name], depth+1)
		}
		if len(fieldNames) > limit {
			out["_truncated_fields"] = len(fieldNames) - limit
		}
		return out
	default:
		return summarizeLeafValue(reflectValueInterface(value))
	}
}

func reflectValueInterface(value reflect.Value) any {
	if !value.IsValid() {
		return nil
	}
	if value.CanInterface() {
		return value.Interface()
	}
	return fmt.Sprintf("<%s>", value.Type().String())
}

func boundedSummaryMap(values map[string]any, depth int) map[string]any {
	if len(values) == 0 {
		return map[string]any{}
	}
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	limit := minInt(len(keys), summaryMaxMapItems)
	out := make(map[string]any, limit+1)
	for _, key := range keys[:limit] {
		out[key] = boundedSummaryValue(values[key], depth+1)
	}
	if len(keys) > limit {
		out["_truncated_keys"] = len(keys) - limit
	}
	return out
}

func summarizeLeafValue(value any) string {
	return truncateSummaryString(fmt.Sprint(value))
}

func truncateSummaryString(value string) string {
	value = strings.TrimSpace(value)
	if len(value) <= summaryMaxStringLen {
		return value
	}
	return value[:summaryMaxStringLen] + "...(truncated)"
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func emitSystemNodeEvent(telemetry core.Telemetry, taskID, message string, metadata map[string]any) {
	if telemetry == nil {
		return
	}
	telemetry.Emit(core.Event{
		Type:      core.EventStateChange,
		TaskID:    strings.TrimSpace(taskID),
		Message:   message,
		Timestamp: time.Now().UTC(),
		Metadata:  metadata,
	})
}

func cloneMapAny(src map[string]any) map[string]any {
	if len(src) == 0 {
		return nil
	}
	out := make(map[string]any, len(src))
	for k, v := range src {
		out[k] = v
	}
	return out
}
