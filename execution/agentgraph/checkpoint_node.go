package agentgraph

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	relurpctx "codeburg.org/lexbit/relurpify/context"
	"codeburg.org/lexbit/relurpify/context/contextdata"
	"codeburg.org/lexbit/relurpify/context/contextstream"
	"codeburg.org/lexbit/relurpify/context/knowledge"
	"codeburg.org/lexbit/relurpify/context/persistence"
	contextports "codeburg.org/lexbit/relurpify/context/ports"
	execution "codeburg.org/lexbit/relurpify/execution"
	"codeburg.org/lexbit/relurpify/execution/agentlifecycle"
	"codeburg.org/lexbit/relurpify/governance/identity"
	telemetry "codeburg.org/lexbit/relurpify/telemetry"
)

// CheckpointNode materializes envelope checkpoint requests into persisted artifacts.
type CheckpointNode struct {
	id                string
	repository        agentlifecycle.Repository
	writer            *persistence.Writer
	snapshotHook      CheckpointSnapshotHook
	principalResolver CheckpointPrincipalResolver
	workflowResolver  CheckpointWorkflowResolver
	runResolver       CheckpointRunResolver
	telemetry         telemetry.Telemetry
	artifactKind      string
	// memoryEvictor releases the task's working memory once its checkpoint
	// is durably saved (working memory expires at the checkpoint boundary).
	memoryEvictor interface{ Evict(taskID string) }
}

// CheckpointSnapshotHook can override how checkpoint payloads are built.
type CheckpointSnapshotHook func(context.Context, *contextdata.Envelope) (persistence.CheckpointSnapshot, bool, error)

// CheckpointPrincipalResolver selects the source principal used for optional persistence writes.
type CheckpointPrincipalResolver func(*contextdata.Envelope) (identity.SubjectRef, bool)

// CheckpointWorkflowResolver resolves the workflow identifier for checkpoint artifacts.
type CheckpointWorkflowResolver func(*contextdata.Envelope) string

// CheckpointRunResolver resolves the run identifier for checkpoint artifacts.
type CheckpointRunResolver func(*contextdata.Envelope) string

// NewCheckpointNode creates a new checkpoint node.
func NewCheckpointNode(id string) *CheckpointNode {
	return &CheckpointNode{
		id:           id,
		artifactKind: "checkpoint",
		workflowResolver: func(env *contextdata.Envelope) string {
			if env == nil {
				return ""
			}
			return strings.TrimSpace(env.TaskIDSnapshot())
		},
		runResolver: func(env *contextdata.Envelope) string {
			if env == nil {
				return ""
			}
			return strings.TrimSpace(env.SessionIDSnapshot())
		},
		principalResolver: func(env *contextdata.Envelope) (identity.SubjectRef, bool) {
			if env == nil || strings.TrimSpace(env.TaskIDSnapshot()) == "" || strings.TrimSpace(env.SessionIDSnapshot()) == "" {
				return identity.SubjectRef{}, false
			}
			return identity.SubjectRef{
				TenantID: strings.TrimSpace(env.SessionIDSnapshot()),
				Kind:     identity.SubjectKindSystem,
				ID:       strings.TrimSpace(env.TaskIDSnapshot()),
			}, true
		},
	}
}

// WithRepository wires the lifecycle repository used to persist checkpoint artifacts.
func (n *CheckpointNode) WithRepository(repo agentlifecycle.Repository) *CheckpointNode {
	if n != nil && repo != nil {
		n.repository = repo
	}
	return n
}

// WithWriter wires the generic persistence writer for optional mirrored writes.
func (n *CheckpointNode) WithWriter(writer *persistence.Writer) *CheckpointNode {
	if n != nil {
		n.writer = writer
	}
	return n
}

// WithSnapshotHook wires a custom checkpoint snapshot builder.
func (n *CheckpointNode) WithSnapshotHook(hook CheckpointSnapshotHook) *CheckpointNode {
	if n != nil && hook != nil {
		n.snapshotHook = hook
	}
	return n
}

// WithPrincipalResolver wires the principal resolver used for optional writer writes.
func (n *CheckpointNode) WithPrincipalResolver(resolver CheckpointPrincipalResolver) *CheckpointNode {
	if n != nil && resolver != nil {
		n.principalResolver = resolver
	}
	return n
}

// WithWorkflowResolver wires the workflow ID resolver.
func (n *CheckpointNode) WithWorkflowResolver(resolver CheckpointWorkflowResolver) *CheckpointNode {
	if n != nil && resolver != nil {
		n.workflowResolver = resolver
	}
	return n
}

// WithRunResolver wires the run ID resolver.
func (n *CheckpointNode) WithRunResolver(resolver CheckpointRunResolver) *CheckpointNode {
	if n != nil && resolver != nil {
		n.runResolver = resolver
	}
	return n
}

// WithWorkingMemoryEvictor wires the working-memory release invoked after a
// checkpoint is durably saved. Nil (the default) disables eviction.
func (n *CheckpointNode) WithWorkingMemoryEvictor(evictor interface{ Evict(taskID string) }) *CheckpointNode {
	n.memoryEvictor = evictor
	return n
}

// WithTelemetry wires checkpoint lifecycle telemetry.
func (n *CheckpointNode) WithTelemetry(t telemetry.Telemetry) *CheckpointNode {
	if n != nil {
		n.telemetry = t
	}
	return n
}

// ID implements agentgraph.Node.
func (n *CheckpointNode) ID() string { return n.id }

// Type implements agentgraph.Node.
func (n *CheckpointNode) Type() NodeType { return NodeTypeSystem }

// Contract implements agentgraph.ContractNode.
func (n *CheckpointNode) Contract() NodeContract {
	return NodeContract{
		SideEffectClass:  SideEffectContext,
		Idempotency:      IdempotencyReplaySafe,
		CheckpointPolicy: CheckpointPolicyPreferred,
		ContextPolicy: relurpctx.StateBoundaryPolicy{
			ReadKeys:                 []string{"task.*", "contextstream.*", "euclo.*"},
			WriteKeys:                []string{"checkpoint.*", "contextstream.*"},
			AllowedMemoryClasses:     []relurpctx.MemoryClass{relurpctx.MemoryClassWorking},
			AllowedDataClasses:       []relurpctx.StateDataClass{relurpctx.StateDataClassTaskMetadata, relurpctx.StateDataClassStructuredState, relurpctx.StateDataClassArtifactRef},
			MaxStateEntryBytes:       8192,
			MaxInlineCollectionItems: 64,
		},
	}
}

// Execute materializes a checkpoint artifact if the envelope has requested one.
func (n *CheckpointNode) Execute(ctx context.Context, env *contextdata.Envelope) (*execution.Result, error) {
	if env == nil {
		return nil, fmt.Errorf("checkpoint node %q missing envelope", n.id)
	}
	snapshot, ok, err := n.buildSnapshot(ctx, env)
	if err != nil {
		return nil, err
	}
	if !ok {
		return &execution.Result{
			NodeID:  n.id,
			Success: true,
			Data:    execution.NewToolResultPayload(map[string]any{"checkpoint_created": false}),
		}, nil
	}
	if n.repository == nil {
		return nil, fmt.Errorf("checkpoint node %q missing repository", n.id)
	}

	ref, err := persistence.SaveCheckpointArtifact(ctx, env, func(artifact contextports.WorkflowArtifactRecord) error {
		inlineRaw, _ := artifact.Metadata["inline_raw"].(string)
		return n.repository.UpsertArtifact(ctx, agentlifecycle.WorkflowArtifactRecord{
			ArtifactID:      artifact.ArtifactID,
			WorkflowID:      artifact.WorkflowID,
			RunID:           artifact.RunID,
			ContentType:     artifact.ContentType,
			StorageKind:     agentlifecycle.ArtifactStorageKind(artifact.StorageKind),
			SummaryText:     artifact.Summary,
			SummaryMetadata: artifact.Metadata,
			InlineRawText:   inlineRaw,
			RawSizeBytes:    int64(len(inlineRaw)),
			CreatedAt:       artifact.CreatedAt,
		})
	}, snapshot)
	if err != nil {
		return nil, err
	}
	if ref == nil {
		return nil, fmt.Errorf("checkpoint node %q did not produce a checkpoint reference", n.id)
	}

	checkpointRef := contextdata.CheckpointReference{
		CheckpointID:      ref.ArtifactID,
		SequenceNum:       env.AssemblyMetadataSnapshot().EventLogSeq,
		RequestedBy:       checkpointRequester(env),
		CreatedAt:         time.Now().UTC(),
		WorkingMemoryKeys: env.WorkingMemoryKeys(),
	}
	env.AddCheckpointReference(checkpointRef)
	env.SetWorkingValueWithClass("checkpoint.id", ref.ArtifactID, contextdata.MemoryClassTask)
	env.SetWorkingValueWithClass("checkpoint.artifact_ref", ref, contextdata.MemoryClassTask)
	env.SetWorkingValueWithClass("checkpoint.materialized", true, contextdata.MemoryClassTask)
	env.SetWorkingValueWithClass("checkpoint.snapshot", snapshot, contextdata.MemoryClassTask)
	env.ClearCheckpointRequest()

	// The checkpoint is durable: the task's working memory expires here.
	// Idempotent; a missing task is a no-op.
	if n.memoryEvictor != nil {
		n.memoryEvictor.Evict(env.TaskIDSnapshot())
	}

	resultData := map[string]any{
		"checkpoint_created": true,
		"checkpoint_id":      ref.ArtifactID,
		"workflow_id":        snapshot.WorkflowID,
		"run_id":             snapshot.RunID,
	}
	if n.writer != nil {
		if err := n.persistMirroredCheckpoint(ctx, env, snapshot); err != nil {
			// Honesty: the primary checkpoint is durable, but the durable
			// mirror failed — surface the failure instead of reporting a
			// fully-clean write.
			n.emitMirrorFailed(ctx, env.TaskIDSnapshot(), err)
			resultData["checkpoint_mirror_failed"] = err.Error()
		}
	}
	if tel, ok := telemetry.TelemetryFromContext(ctx).(telemetry.CheckpointTelemetry); ok {
		tel.OnCheckpointCreated(env.TaskIDSnapshot(), ref.ArtifactID, n.id)
	}
	if n.telemetry != nil {
		ev := telemetry.Event{
			Type:      telemetry.EventStateChange,
			NodeID:    n.id,
			TaskID:    env.TaskIDSnapshot(),
			RunID:     snapshot.RunID, // first-class correlation, not metadata
			Timestamp: time.Now().UTC(),
			Metadata: map[string]any{
				"checkpoint_id": ref.ArtifactID,
				"workflow_id":   snapshot.WorkflowID,
			},
		}
		telemetry.StampCorrelation(ctx, &ev)
		n.telemetry.Emit(ev)
	}

	return &execution.Result{
		NodeID:  n.id,
		Success: true,
		Data:    execution.NewToolResultPayload(resultData),
	}, nil
}

func (n *CheckpointNode) buildSnapshot(ctx context.Context, env *contextdata.Envelope) (persistence.CheckpointSnapshot, bool, error) {
	if n.snapshotHook != nil {
		return n.snapshotHook(ctx, env)
	}
	req := env.CheckpointRequestSnapshot()
	if req == nil {
		return persistence.CheckpointSnapshot{}, false, nil
	}
	streamResult, _ := contextdata.GetTyped[any](env, "contextstream.result")
	if streamResult == nil {
		streamResult, _ = contextdata.GetTyped[any](env, "euclo.stream_result")
	}
	workflowID := ""
	runID := ""
	if n.workflowResolver != nil {
		workflowID = strings.TrimSpace(n.workflowResolver(env))
	}
	if n.runResolver != nil {
		runID = strings.TrimSpace(n.runResolver(env))
	}
	if workflowID == "" {
		workflowID = strings.TrimSpace(env.TaskIDSnapshot())
	}
	if runID == "" {
		runID = strings.TrimSpace(env.SessionIDSnapshot())
	}
	snapshot := persistence.CheckpointSnapshot{
		CheckpointID: n.checkpointID(env, req),
		WorkflowID:   workflowID,
		RunID:        runID,
		Kind:         n.artifactKind,
		Summary:      "checkpoint materialized",
		Metadata:     map[string]any{},
	}
	if req != nil {
		snapshot.Metadata["requested_by"] = req.RequestedBy
		snapshot.Metadata["reason"] = reqReason(req)
		snapshot.Metadata["priority"] = reqPriority(req)
		snapshot.Metadata["evict_working_memory"] = req.EvictWorkingMemory
	}
	if sr, ok := streamResult.(*contextstream.Result); ok && sr != nil {
		snapshot.Metadata["has_stream_result"] = true
		snapshot.Metadata["stream_request_id"] = sr.Request.ID
		snapshot.Metadata["stream_mode"] = string(sr.Request.Mode)
		snapshot.Metadata["shortfall_tokens"] = sr.Trim.ShortfallTokens
		snapshot.Metadata["trimmed"] = sr.Trim.ShortfallTokens > 0 || len(sr.Trim.Substitutions) > 0
	}
	inline, err := json.Marshal(map[string]any{
		"checkpoint_request": req,
		"stream_result":      streamResult,
		"working_data":       env.WorkingDataSnapshot(),
		"references":         env.ReferencesSnapshot(),
	})
	if err != nil {
		return persistence.CheckpointSnapshot{}, false, err
	}
	snapshot.InlineRaw = string(inline)
	return snapshot, true, nil
}

func (n *CheckpointNode) checkpointID(env *contextdata.Envelope, req *contextdata.CheckpointRequest) string {
	if req != nil && strings.TrimSpace(req.RequestedBy) != "" {
		return strings.TrimSpace(req.RequestedBy) + ":" + strings.TrimSpace(env.TaskIDSnapshot()) + ":" + strings.TrimSpace(env.SessionIDSnapshot())
	}
	if env == nil {
		return "checkpoint"
	}
	return strings.TrimSpace(env.TaskIDSnapshot()) + ":" + strings.TrimSpace(env.SessionIDSnapshot()) + ":" + n.id
}

func reqReason(req *contextdata.CheckpointRequest) string {
	if req == nil {
		return ""
	}
	return strings.TrimSpace(req.Reason)
}

func reqPriority(req *contextdata.CheckpointRequest) int {
	if req == nil {
		return 0
	}
	return req.Priority
}

func (n *CheckpointNode) persistMirroredCheckpoint(ctx context.Context, env *contextdata.Envelope, snapshot persistence.CheckpointSnapshot) error {
	if n == nil || n.writer == nil || env == nil {
		return nil
	}
	principal := identity.SubjectRef{}
	ok := false
	if n.principalResolver != nil {
		principal, ok = n.principalResolver(env)
	}
	if !ok || strings.TrimSpace(principal.ID) == "" {
		return nil
	}
	payload, err := json.Marshal(snapshot)
	if err != nil {
		return fmt.Errorf("marshal mirrored checkpoint: %w", err)
	}
	_, err = n.writer.Persist(ctx, persistence.PersistenceRequest{
		Content:         payload,
		Kind:            knowledge.ChunkKindDerivation,
		ContentType:     "application/json",
		SourcePrincipal: principal,
		SourceOrigin:    knowledge.SourceOriginDerivation,
		Reason:          "checkpoint materialization",
		Tags:            []string{"checkpoint", "graph"},
	})
	if err != nil {
		return fmt.Errorf("persist mirrored checkpoint: %w", err)
	}
	return nil
}

// emitMirrorFailed surfaces a checkpoint mirror failure on the durable trail.
func (n *CheckpointNode) emitMirrorFailed(ctx context.Context, taskID string, err error) {
	if n == nil || n.telemetry == nil || err == nil {
		return
	}
	ev := telemetry.Event{
		Type:      telemetry.EventCheckpointMirrorFailed,
		Message:   "checkpoint mirror failed",
		NodeID:    n.id,
		TaskID:    taskID,
		Timestamp: time.Now().UTC(),
		Metadata:  map[string]any{"error": err.Error()},
	}
	telemetry.StampCorrelation(ctx, &ev)
	n.telemetry.Emit(ev)
}

func checkpointRequester(env *contextdata.Envelope) string {
	if env == nil {
		return ""
	}
	req := env.CheckpointRequestSnapshot()
	if req == nil {
		return ""
	}
	return strings.TrimSpace(req.RequestedBy)
}
