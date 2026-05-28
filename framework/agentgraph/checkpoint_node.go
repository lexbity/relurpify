package agentgraph

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"codeburg.org/lexbit/relurpify/framework/agentlifecycle"
	"codeburg.org/lexbit/relurpify/framework/contextdata"
	"codeburg.org/lexbit/relurpify/framework/contextstream"
	"codeburg.org/lexbit/relurpify/framework/core"
	"codeburg.org/lexbit/relurpify/framework/knowledge"
	"codeburg.org/lexbit/relurpify/framework/persistence"
	"codeburg.org/lexbit/relurpify/relurpnet/identity"
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
	telemetry         core.Telemetry
	artifactKind      string
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
			return strings.TrimSpace(env.TaskID)
		},
		runResolver: func(env *contextdata.Envelope) string {
			if env == nil {
				return ""
			}
			return strings.TrimSpace(env.SessionID)
		},
		principalResolver: func(env *contextdata.Envelope) (identity.SubjectRef, bool) {
			if env == nil || strings.TrimSpace(env.TaskID) == "" || strings.TrimSpace(env.SessionID) == "" {
				return identity.SubjectRef{}, false
			}
			return identity.SubjectRef{
				TenantID: strings.TrimSpace(env.SessionID),
				Kind:     identity.SubjectKindSystem,
				ID:       strings.TrimSpace(env.TaskID),
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

// WithTelemetry wires checkpoint lifecycle telemetry.
func (n *CheckpointNode) WithTelemetry(t core.Telemetry) *CheckpointNode {
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
		ContextPolicy: core.StateBoundaryPolicy{
			ReadKeys:                 []string{"task.*", "contextstream.*", "euclo.*"},
			WriteKeys:                []string{"checkpoint.*", "contextstream.*"},
			AllowedMemoryClasses:     []core.MemoryClass{core.MemoryClassWorking},
			AllowedDataClasses:       []core.StateDataClass{core.StateDataClassTaskMetadata, core.StateDataClassStructuredState, core.StateDataClassArtifactRef},
			MaxStateEntryBytes:       8192,
			MaxInlineCollectionItems: 64,
		},
	}
}

// Execute materializes a checkpoint artifact if the envelope has requested one.
func (n *CheckpointNode) Execute(ctx context.Context, env *contextdata.Envelope) (*core.Result, error) {
	if env == nil {
		return nil, fmt.Errorf("checkpoint node %q missing envelope", n.id)
	}
	snapshot, ok, err := n.buildSnapshot(ctx, env)
	if err != nil {
		return nil, err
	}
	if !ok {
		return &core.Result{
			NodeID:  n.id,
			Success: true,
			Data:    core.NewToolResultPayload(map[string]any{"checkpoint_created": false}),
		}, nil
	}
	if n.repository == nil {
		return nil, fmt.Errorf("checkpoint node %q missing repository", n.id)
	}

	ref, err := persistence.SaveCheckpointArtifact(ctx, env, n.repository, snapshot)
	if err != nil {
		return nil, err
	}
	if ref == nil {
		return nil, fmt.Errorf("checkpoint node %q did not produce a checkpoint reference", n.id)
	}

	checkpointRef := contextdata.CheckpointReference{
		CheckpointID:      ref.ArtifactID,
		SequenceNum:       env.AssemblyMetadata.EventLogSeq,
		RequestedBy:       checkpointRequester(env),
		CreatedAt:         time.Now().UTC(),
		WorkingMemoryKeys: env.WorkingMemoryKeys(),
	}
	env.AddCheckpointReference(checkpointRef)
	env.SetWorkingValue("checkpoint.id", ref.ArtifactID, contextdata.MemoryClassTask)
	env.SetWorkingValue("checkpoint.artifact_ref", ref, contextdata.MemoryClassTask)
	env.SetWorkingValue("checkpoint.materialized", true, contextdata.MemoryClassTask)
	env.SetWorkingValue("checkpoint.snapshot", snapshot, contextdata.MemoryClassTask)
	env.ClearCheckpointRequest()

	if n.writer != nil {
		n.persistMirroredCheckpoint(ctx, env, snapshot)
	}
	if tel, ok := core.TelemetryFromContext(ctx).(core.CheckpointTelemetry); ok {
		tel.OnCheckpointCreated(env.TaskID, ref.ArtifactID, n.id)
	}
	if n.telemetry != nil {
		n.telemetry.Emit(core.Event{
			Type:      core.EventStateChange,
			NodeID:    n.id,
			TaskID:    env.TaskID,
			Timestamp: time.Now().UTC(),
			Metadata: map[string]any{
				"checkpoint_id": ref.ArtifactID,
				"workflow_id":   snapshot.WorkflowID,
				"run_id":        snapshot.RunID,
			},
		})
	}

	return &core.Result{
		NodeID:  n.id,
		Success: true,
		Data: core.NewToolResultPayload(map[string]any{
			"checkpoint_created": true,
			"checkpoint_id":      ref.ArtifactID,
			"workflow_id":        snapshot.WorkflowID,
			"run_id":             snapshot.RunID,
		}),
	}, nil
}

func (n *CheckpointNode) buildSnapshot(ctx context.Context, env *contextdata.Envelope) (persistence.CheckpointSnapshot, bool, error) {
	if n.snapshotHook != nil {
		return n.snapshotHook(ctx, env)
	}
	req := env.CheckpointRequest
	if req == nil {
		return persistence.CheckpointSnapshot{}, false, nil
	}
	streamResult, _ := env.GetWorkingValue("contextstream.result")
	if streamResult == nil {
		streamResult, _ = env.GetWorkingValue("euclo.stream_result")
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
		workflowID = strings.TrimSpace(env.TaskID)
	}
	if runID == "" {
		runID = strings.TrimSpace(env.SessionID)
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
		snapshot.Metadata["trimmed"] = sr.Trim.Truncated
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
		return strings.TrimSpace(req.RequestedBy) + ":" + strings.TrimSpace(env.TaskID) + ":" + strings.TrimSpace(env.SessionID)
	}
	if env == nil {
		return "checkpoint"
	}
	return strings.TrimSpace(env.TaskID) + ":" + strings.TrimSpace(env.SessionID) + ":" + n.id
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

func (n *CheckpointNode) persistMirroredCheckpoint(ctx context.Context, env *contextdata.Envelope, snapshot persistence.CheckpointSnapshot) {
	if n == nil || n.writer == nil || env == nil {
		return
	}
	principal := identity.SubjectRef{}
	ok := false
	if n.principalResolver != nil {
		principal, ok = n.principalResolver(env)
	}
	if !ok || strings.TrimSpace(principal.ID) == "" {
		return
	}
	payload, err := json.Marshal(snapshot)
	if err != nil {
		return
	}
	_, _ = n.writer.Persist(ctx, persistence.PersistenceRequest{
		Content:         payload,
		ContentType:     "application/json",
		SourcePrincipal: principal,
		SourceOrigin:    knowledge.SourceOriginDerivation,
		Reason:          "checkpoint materialization",
		Tags:            []string{"checkpoint", "graph"},
	})
}

func checkpointRequester(env *contextdata.Envelope) string {
	if env == nil || env.CheckpointRequest == nil {
		return ""
	}
	return strings.TrimSpace(env.CheckpointRequest.RequestedBy)
}
