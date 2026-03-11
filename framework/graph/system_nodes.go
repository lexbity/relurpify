package graph

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/lexcodex/relurpify/framework/core"
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

// SummarizeContextNode summarizes selected context and emits a summary artifact.
type SummarizeContextNode struct {
	id               string
	Summarizer       core.Summarizer
	Level            core.SummaryLevel
	IncludeHistory   bool
	StateKeys        []string
	StateKey         string
	ArtifactStateKey string
	ArtifactSink     ArtifactSink
	Telemetry        core.Telemetry
}

func NewSummarizeContextNode(id string, summarizer core.Summarizer) *SummarizeContextNode {
	if id == "" {
		id = "summarize_context"
	}
	if summarizer == nil {
		summarizer = &core.SimpleSummarizer{}
	}
	return &SummarizeContextNode{
		id:               id,
		Summarizer:       summarizer,
		Level:            core.SummaryConcise,
		IncludeHistory:   true,
		StateKey:         "graph.summary",
		ArtifactStateKey: "graph.summary_ref",
	}
}

func (n *SummarizeContextNode) ID() string     { return n.id }
func (n *SummarizeContextNode) Type() NodeType { return NodeTypeSystem }
func (n *SummarizeContextNode) Contract() NodeContract {
	return NodeContract{
		SideEffectClass: SideEffectContext,
		Idempotency:     IdempotencyReplaySafe,
		ContextPolicy: core.StateBoundaryPolicy{
			ReadKeys:                 []string{"task.*", "react.*", "planner.*", "architect.*", "graph.*"},
			WriteKeys:                []string{"graph.summary*", "artifact.*"},
			AllowHistoryAccess:       true,
			AllowedMemoryClasses:     []core.MemoryClass{core.MemoryClassWorking},
			AllowedDataClasses:       []core.StateDataClass{core.StateDataClassTaskMetadata, core.StateDataClassArtifactRef, core.StateDataClassStructuredState},
			MaxStateEntryBytes:       4096,
			MaxInlineCollectionItems: 16,
			PreferArtifactReferences: true,
		},
	}
}

func (n *SummarizeContextNode) Execute(ctx context.Context, state *Context) (*Result, error) {
	if n.Summarizer == nil {
		n.Summarizer = &core.SimpleSummarizer{}
	}
	payload := summarizeNodePayload(state, n.StateKeys, n.IncludeHistory)
	summary, err := n.Summarizer.Summarize(payload, n.Level)
	if err != nil {
		return nil, err
	}
	artifactID := generateCheckpointID()
	ref := core.ArtifactReference{
		ArtifactID:   artifactID,
		Kind:         "summary",
		ContentType:  "text/plain",
		StorageKind:  "summary",
		URI:          fmt.Sprintf("workflow://artifact/%s", artifactID),
		Summary:      summary,
		RawSizeBytes: int64(len(payload)),
	}
	if n.ArtifactSink != nil {
		_ = n.ArtifactSink.SaveArtifact(ctx, ArtifactRecord{
			ArtifactID:   artifactID,
			Kind:         "summary",
			ContentType:  "text/plain",
			StorageKind:  "inline",
			Summary:      summary,
			RawText:      payload,
			RawSizeBytes: int64(len(payload)),
			Metadata: map[string]any{
				"node":            n.id,
				"include_history": n.IncludeHistory,
				"state_keys":      append([]string{}, n.StateKeys...),
			},
			CreatedAt: time.Now().UTC(),
		})
	}
	if state != nil {
		state.Set(n.StateKey, map[string]any{
			"summary":         summary,
			"artifact_id":     artifactID,
			"include_history": n.IncludeHistory,
			"state_keys":      append([]string{}, n.StateKeys...),
		})
		state.Set(n.ArtifactStateKey, ref)
	}
	emitSystemNodeEvent(n.Telemetry, state.GetString("task.id"), "context summarized", map[string]any{
		"node":        n.id,
		"artifact_id": artifactID,
	})
	return &Result{NodeID: n.id, Success: true, Data: map[string]any{
		"summary":      summary,
		"artifact_ref": ref,
	}}, nil
}

// RetrieveDeclarativeMemoryNode fetches bounded declarative memory into state.
type RetrieveDeclarativeMemoryNode struct {
	id        string
	Retriever MemoryRetriever
	Query     string
	Limit     int
	StateKey  string
}

func NewRetrieveDeclarativeMemoryNode(id string, retriever MemoryRetriever) *RetrieveDeclarativeMemoryNode {
	if id == "" {
		id = "retrieve_declarative_memory"
	}
	return &RetrieveDeclarativeMemoryNode{id: id, Retriever: retriever, Limit: 3, StateKey: "graph.declarative_memory"}
}

func (n *RetrieveDeclarativeMemoryNode) ID() string     { return n.id }
func (n *RetrieveDeclarativeMemoryNode) Type() NodeType { return NodeTypeSystem }
func (n *RetrieveDeclarativeMemoryNode) Contract() NodeContract {
	return NodeContract{
		SideEffectClass: SideEffectNone,
		Idempotency:     IdempotencyReplaySafe,
		ContextPolicy: core.StateBoundaryPolicy{
			ReadKeys:                 []string{"task.*"},
			WriteKeys:                []string{"graph.declarative_memory"},
			AllowRetrieval:           true,
			AllowedMemoryClasses:     []core.MemoryClass{core.MemoryClassDeclarative},
			AllowedDataClasses:       []core.StateDataClass{core.StateDataClassTaskMetadata, core.StateDataClassMemoryRef, core.StateDataClassStructuredState},
			MaxStateEntryBytes:       4096,
			MaxInlineCollectionItems: 8,
			PreferArtifactReferences: true,
		},
	}
}

func (n *RetrieveDeclarativeMemoryNode) Execute(ctx context.Context, state *Context) (*Result, error) {
	return executeMemoryRetrievalNode(ctx, state, n.id, n.Retriever, n.Query, n.Limit, n.StateKey, core.MemoryClassDeclarative)
}

// RetrieveProceduralMemoryNode fetches bounded procedural memory into state.
type RetrieveProceduralMemoryNode struct {
	id        string
	Retriever MemoryRetriever
	Query     string
	Limit     int
	StateKey  string
}

func NewRetrieveProceduralMemoryNode(id string, retriever MemoryRetriever) *RetrieveProceduralMemoryNode {
	if id == "" {
		id = "retrieve_procedural_memory"
	}
	return &RetrieveProceduralMemoryNode{id: id, Retriever: retriever, Limit: 3, StateKey: "graph.procedural_memory"}
}

func (n *RetrieveProceduralMemoryNode) ID() string     { return n.id }
func (n *RetrieveProceduralMemoryNode) Type() NodeType { return NodeTypeSystem }
func (n *RetrieveProceduralMemoryNode) Contract() NodeContract {
	return NodeContract{
		SideEffectClass: SideEffectNone,
		Idempotency:     IdempotencyReplaySafe,
		ContextPolicy: core.StateBoundaryPolicy{
			ReadKeys:                 []string{"task.*"},
			WriteKeys:                []string{"graph.procedural_memory"},
			AllowRetrieval:           true,
			AllowedMemoryClasses:     []core.MemoryClass{core.MemoryClassProcedural},
			AllowedDataClasses:       []core.StateDataClass{core.StateDataClassTaskMetadata, core.StateDataClassMemoryRef, core.StateDataClassStructuredState},
			MaxStateEntryBytes:       4096,
			MaxInlineCollectionItems: 8,
			PreferArtifactReferences: true,
		},
	}
}

func (n *RetrieveProceduralMemoryNode) Execute(ctx context.Context, state *Context) (*Result, error) {
	return executeMemoryRetrievalNode(ctx, state, n.id, n.Retriever, n.Query, n.Limit, n.StateKey, core.MemoryClassProcedural)
}

// HydrateContextNode restores selected references into active graph state.
type HydrateContextNode struct {
	id       string
	Hydrator StateHydrator
	Refs     []string
	StateKey string
}

func NewHydrateContextNode(id string, hydrator StateHydrator) *HydrateContextNode {
	if id == "" {
		id = "hydrate_context"
	}
	return &HydrateContextNode{id: id, Hydrator: hydrator, StateKey: "graph.hydrated"}
}

func (n *HydrateContextNode) ID() string     { return n.id }
func (n *HydrateContextNode) Type() NodeType { return NodeTypeSystem }
func (n *HydrateContextNode) Contract() NodeContract {
	return NodeContract{
		SideEffectClass: SideEffectContext,
		Idempotency:     IdempotencyReplaySafe,
		ContextPolicy: core.StateBoundaryPolicy{
			ReadKeys:                 []string{"graph.*", "artifact.*"},
			WriteKeys:                []string{"graph.hydrated"},
			AllowedMemoryClasses:     []core.MemoryClass{core.MemoryClassWorking, core.MemoryClassDeclarative, core.MemoryClassProcedural},
			AllowedDataClasses:       []core.StateDataClass{core.StateDataClassArtifactRef, core.StateDataClassMemoryRef, core.StateDataClassStructuredState},
			MaxStateEntryBytes:       4096,
			MaxInlineCollectionItems: 16,
			PreferArtifactReferences: true,
		},
	}
}

func (n *HydrateContextNode) Execute(ctx context.Context, state *Context) (*Result, error) {
	if n.Hydrator == nil {
		return &Result{NodeID: n.id, Success: true, Data: map[string]any{}}, nil
	}
	values, err := n.Hydrator.Hydrate(ctx, append([]string{}, n.Refs...))
	if err != nil {
		return nil, err
	}
	if state != nil {
		state.Set(n.StateKey, values)
	}
	return &Result{NodeID: n.id, Success: true, Data: map[string]any{"hydrated": values}}, nil
}

func executeMemoryRetrievalNode(ctx context.Context, state *Context, nodeID string, retriever MemoryRetriever, query string, limit int, stateKey string, expectedClass core.MemoryClass) (*Result, error) {
	if retriever == nil {
		if state != nil {
			state.Set(stateKey, []core.MemoryRecordEnvelope{})
		}
		return &Result{NodeID: nodeID, Success: true, Data: map[string]any{"results": []core.MemoryRecordEnvelope{}}}, nil
	}
	query = strings.TrimSpace(query)
	if query == "" && state != nil {
		query = strings.TrimSpace(state.GetString("task.instruction"))
	}
	if query == "" {
		query = "current task"
	}
	if limit <= 0 {
		limit = 3
	}
	results, err := retriever.Retrieve(ctx, query, limit)
	if err != nil {
		return nil, err
	}
	if len(results) > limit {
		results = results[:limit]
	}
	references := make([]core.MemoryReference, 0, len(results))
	for _, record := range results {
		if record.MemoryClass == "" {
			record.MemoryClass = expectedClass
		}
		references = append(references, core.MemoryReference{
			MemoryClass: record.MemoryClass,
			Scope:       record.Scope,
			RecordKey:   record.Key,
			Summary:     record.Summary,
		})
	}
	if state != nil {
		state.Set(stateKey, map[string]any{
			"query":      query,
			"results":    results,
			"references": references,
		})
	}
	return &Result{NodeID: nodeID, Success: true, Data: map[string]any{
		"query":      query,
		"results":    results,
		"references": references,
	}}, nil
}

func summarizeNodePayload(state *Context, keys []string, includeHistory bool) string {
	if state == nil {
		return ""
	}
	var parts []string
	for _, key := range keys {
		if raw, ok := state.Get(key); ok && raw != nil {
			parts = append(parts, fmt.Sprintf("%s: %v", key, raw))
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
