package rex

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"codeburg.org/lexbit/relurpify/ayenitd"
	"codeburg.org/lexbit/relurpify/framework/agentenv"
	frameworkconfig "codeburg.org/lexbit/relurpify/framework/config"
	"codeburg.org/lexbit/relurpify/framework/core"
	"codeburg.org/lexbit/relurpify/framework/graph"
	"codeburg.org/lexbit/relurpify/framework/memory"
	"codeburg.org/lexbit/relurpify/named/rex/classify"
	rexconfig "codeburg.org/lexbit/relurpify/named/rex/config"
	"codeburg.org/lexbit/relurpify/named/rex/delegates"
	"codeburg.org/lexbit/relurpify/named/rex/envelope"
	"codeburg.org/lexbit/relurpify/named/rex/nexus"
	"codeburg.org/lexbit/relurpify/named/rex/proof"
	"codeburg.org/lexbit/relurpify/named/rex/reconcile"
	"codeburg.org/lexbit/relurpify/named/rex/retrieval"
	"codeburg.org/lexbit/relurpify/named/rex/rexkeys"
	"codeburg.org/lexbit/relurpify/named/rex/route"
	rexruntime "codeburg.org/lexbit/relurpify/named/rex/runtime"
	"codeburg.org/lexbit/relurpify/named/rex/state"
)

// Agent is the Nexus-managed named runtime for rex.
type Agent struct {
	Config      *core.Config
	Environment ayenitd.WorkspaceEnvironment
	Workspace   string
	RexConfig   rexconfig.Config
	Delegates   *delegates.Registry
	Runtime     *rexruntime.Manager
	Reconciler  reconcile.Reconciler
	Observer    state.ExecutionObserver
	LastProof   proof.ProofSurface
}

func New(env ayenitd.WorkspaceEnvironment) *Agent {
	return NewWithWorkspace(env, "")
}

func NewWithWorkspace(env ayenitd.WorkspaceEnvironment, workspace string) *Agent {
	agent := &Agent{}
	_ = agent.InitializeEnvironment(env, workspace)
	return agent
}

func (a *Agent) InitializeEnvironment(env ayenitd.WorkspaceEnvironment, workspace string) error {
	a.Environment = env
	a.Config = env.Config
	a.RexConfig = rexconfig.Default()
	a.Workspace = resolveWorkspaceRoot(workspace)
	a.Delegates = delegates.NewRegistry(agentenv.AgentEnvironment{
		Config:       env.Config,
		Model:        env.Model,
		Registry:     env.Registry,
		IndexManager: env.IndexManager,
		SearchEngine: env.SearchEngine,
		Memory:       env.Memory,
	}, a.Workspace)
	a.Runtime = rexruntime.New(a.RexConfig, env.Memory)
	a.Reconciler = &reconcile.InMemoryReconciler{}
	return a.Initialize(env.Config)
}

func (a *Agent) Initialize(cfg *core.Config) error {
	a.Config = cfg
	if a.Runtime == nil {
		a.Runtime = rexruntime.New(a.RexConfig, a.Environment.Memory)
	}
	return nil
}

func (a *Agent) Capabilities() []core.Capability {
	return []core.Capability{
		core.CapabilityPlan,
		core.CapabilityExecute,
		core.CapabilityCode,
		core.CapabilityExplain,
		core.CapabilityHumanInLoop,
	}
}

func (a *Agent) BuildGraph(task *core.Task) (*graph.Graph, error) {
	env := envelope.Normalize(task, nil)
	class := classify.Classify(env)
	decision := route.Decide(env, class)
	plan := route.BuildExecutionPlan(decision)
	delegate, err := a.Delegates.Resolve(plan)
	if err != nil {
		return nil, err
	}
	return delegate.BuildGraph(task)
}

func (a *Agent) Execute(ctx context.Context, task *core.Task, stateCtx *core.Context) (*core.Result, error) {
	var execErr error
	var result *core.Result
	if stateCtx == nil {
		stateCtx = core.NewContext()
	}
	env := envelope.Normalize(task, stateCtx)
	class := classify.Classify(env)
	decision := route.Decide(env, class)
	execPlan := route.BuildExecutionPlan(decision)
	identity := state.ComputeIdentity(env)
	stateCtx.Set(rexkeys.RexWorkflowID, identity.WorkflowID)
	stateCtx.Set(rexkeys.RexRunID, identity.RunID)
	stateCtx.Set("rex.route", decision.Family)
	if a.Observer != nil {
		if err := a.Observer.BeforeExecute(ctx, identity.WorkflowID, identity.RunID, task, stateCtx); err != nil {
			execErr = err
			return nil, err
		}
		defer func() {
			_ = a.Observer.AfterExecute(ctx, identity.WorkflowID, identity.RunID, task, stateCtx, result, execErr)
		}()
	}
	finishRuntime := a.Runtime.BeginExecution(identity.WorkflowID, identity.RunID)
	defer func() {
		finishRuntime(execErr)
	}()

	surfaces := state.ResolveRuntimeSurfaces(a.Environment.Memory)
	eventSuffix := stateCtx.GetString(rexkeys.RexEventID)
	if eventSuffix == "" {
		eventSuffix = "runtime"
	}
	if surfaces.Workflow != nil {
		if err := state.EnsureWorkflowRun(ctx, surfaces.Workflow, identity, task, decision.Mode); err != nil {
			execErr = err
			return nil, err
		}
		_ = surfaces.Workflow.AppendEvent(ctx, memory.WorkflowEventRecord{
			EventID:    identity.RunID + ":" + eventSuffix + ":start",
			WorkflowID: identity.WorkflowID,
			RunID:      identity.RunID,
			EventType:  "rex.run.started",
			Message:    "rex execution started",
			Metadata:   map[string]any{"route": decision.Family, "mode": decision.Mode, "profile": decision.Profile},
			CreatedAt:  time.Now().UTC(),
		})
	}
	executionTask := task
	if execPlan.RequireRetrieval && surfaces.Workflow != nil {
		expansion, err := retrieval.ExpandWithWorkflowStore(ctx, surfaces.Workflow, identity.WorkflowID, task, stateCtx, decision)
		if err == nil {
			executionTask = retrieval.Apply(stateCtx, task, expansion)
			artifactKinds := []string{"rex.proof_surface", "rex.action_log", "rex.completion"}
			if len(expansion.LocalPaths) > 0 {
				artifactKinds = append(artifactKinds, "rex.context_expansion")
			}
			if len(expansion.WorkflowRetrieval) > 0 {
				artifactKinds = append(artifactKinds, "rex.workflow_retrieval")
			}
			stateCtx.Set("rex.artifact_kinds", artifactKinds)
			if surfaces.Workflow != nil {
				_ = persistContextExpansion(ctx, surfaces.Workflow, identity, expansion)
			}
		}
	}
	if err := enforceCapabilityProjection(stateCtx, decision, task); err != nil {
		execErr = err
		return nil, err
	}
	delegate, err := a.Delegates.Resolve(execPlan)
	if err != nil {
		execErr = err
		return nil, err
	}
	result, err = delegate.Execute(ctx, executionTask, stateCtx)
	completion := proof.EvaluateCompletion(decision, class, stateCtx)
	artifactKinds := []string{"rex.proof_surface", "rex.action_log", "rex.completion", "rex.verification_policy", "rex.success_gate"}
	if verification := proof.VerificationEvidence(stateCtx); verification.EvidencePresent {
		artifactKinds = append(artifactKinds, "rex.verification")
	}
	if raw, ok := stateCtx.Get("rex.artifact_kinds"); ok {
		if existing, ok := raw.([]string); ok {
			artifactKinds = append(existing, artifactKinds...)
		}
	}
	stateCtx.Set("rex.artifact_kinds", uniqueStrings(artifactKinds))
	actionLog := proof.BuildActionLog(decision, class, stateCtx)
	if result == nil {
		result = &core.Result{Success: err == nil, Data: map[string]any{}}
	}
	if result.Data == nil {
		result.Data = map[string]any{}
	}
	result.Data["rex.action_log"] = actionLog
	a.LastProof = proof.BuildProofSurface(decision, result, stateCtx)
	result.Data["rex.proof_surface"] = a.LastProof
	result.Data["rex.completion"] = completion
	result.Data[rexkeys.RexWorkflowID] = identity.WorkflowID
	result.Data[rexkeys.RexRunID] = identity.RunID
	result.Data["rex.route"] = decision.Family
	if surfaces.Workflow != nil {
		_ = persistProof(ctx, surfaces.Workflow, identity, decision, a.LastProof, actionLog, completion, stateCtx)
		status := memory.WorkflowRunStatusCompleted
		var finishedAt *time.Time
		now := time.Now().UTC()
		finishedAt = &now
		if err != nil || !completion.Allowed {
			status = memory.WorkflowRunStatusFailed
		}
		_ = surfaces.Workflow.UpdateRunStatus(ctx, identity.RunID, status, finishedAt)
		_, _ = surfaces.Workflow.UpdateWorkflowStatus(ctx, identity.WorkflowID, 0, status, "")
		_ = surfaces.Workflow.AppendEvent(ctx, memory.WorkflowEventRecord{
			EventID:    identity.RunID + ":" + eventSuffix + ":finish",
			WorkflowID: identity.WorkflowID,
			RunID:      identity.RunID,
			EventType:  "rex.run.finished",
			Message:    "rex execution finished",
			Metadata:   map[string]any{"route": decision.Family, "allowed": completion.Allowed, "success": result.Success},
			CreatedAt:  now,
		})
	}
	if !completion.Allowed {
		result.Success = false
		blockErr := fmt.Errorf("rex completion blocked: %s", completion.Reason)
		result.Error = blockErr
		execErr = blockErr
		return result, blockErr
	}
	if err != nil {
		result.Error = err
	}
	execErr = err
	return result, err
}

func (a *Agent) RuntimeProjection() nexus.Projection {
	return nexus.BuildProjection(a.Runtime, a.LastProof)
}

func (a *Agent) ManagedAdapter() *nexus.Adapter {
	surfaces := state.ResolveRuntimeSurfaces(a.Environment.Memory)
	return nexus.NewAdapter("rex", a, surfaces.Workflow)
}

func (a *Agent) RecordAmbiguity(workflowID, runID, reason string) reconcile.Record {
	if a == nil {
		return reconcile.Record{}
	}
	if a.Reconciler == nil {
		a.Reconciler = &reconcile.InMemoryReconciler{}
	}
	return a.Reconciler.RecordAmbiguity(workflowID, runID, reason)
}

func (a *Agent) ResolveAmbiguity(record reconcile.Record, outcome reconcile.Outcome, notes string) reconcile.Record {
	if a == nil {
		return record
	}
	if a.Reconciler == nil {
		a.Reconciler = &reconcile.InMemoryReconciler{}
	}
	return a.Reconciler.Resolve(record, outcome, notes)
}

func (a *Agent) ShouldRetryAmbiguity(record reconcile.Record) bool {
	if a == nil {
		return false
	}
	if a.Reconciler == nil {
		a.Reconciler = &reconcile.InMemoryReconciler{}
	}
	return a.Reconciler.ShouldRetry(record)
}

func persistProof(ctx context.Context, store interface {
	UpsertWorkflowArtifact(context.Context, memory.WorkflowArtifactRecord) error
}, identity state.Identity, decision route.RouteDecision, surface proof.ProofSurface, actionLog []proof.ActionLogEntry, completion proof.CompletionDecision, stateCtx *core.Context) error {
	if store == nil {
		return nil
	}
	proofJSON, err := json.Marshal(surface)
	if err != nil {
		return err
	}
	actionLogJSON, err := json.Marshal(actionLog)
	if err != nil {
		return err
	}
	completionJSON, err := json.Marshal(completion)
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	if err := store.UpsertWorkflowArtifact(ctx, memory.WorkflowArtifactRecord{
		ArtifactID:      identity.RunID + ":proof",
		WorkflowID:      identity.WorkflowID,
		RunID:           identity.RunID,
		Kind:            "rex.proof_surface",
		ContentType:     "application/json",
		StorageKind:     memory.ArtifactStorageInline,
		SummaryText:     "rex proof surface",
		InlineRawText:   string(proofJSON),
		SummaryMetadata: map[string]any{"route": decision.Family},
		CreatedAt:       now,
	}); err != nil {
		return err
	}
	if err := store.UpsertWorkflowArtifact(ctx, memory.WorkflowArtifactRecord{
		ArtifactID:      identity.RunID + ":action-log",
		WorkflowID:      identity.WorkflowID,
		RunID:           identity.RunID,
		Kind:            "rex.action_log",
		ContentType:     "application/json",
		StorageKind:     memory.ArtifactStorageInline,
		SummaryText:     "rex action log",
		InlineRawText:   string(actionLogJSON),
		SummaryMetadata: map[string]any{"route": decision.Family},
		CreatedAt:       now,
	}); err != nil {
		return err
	}
	if raw, ok := stateCtx.Get("rex.verification_policy"); ok && raw != nil {
		payload, err := json.Marshal(raw)
		if err != nil {
			return err
		}
		if err := store.UpsertWorkflowArtifact(ctx, memory.WorkflowArtifactRecord{
			ArtifactID:      identity.RunID + ":verification-policy",
			WorkflowID:      identity.WorkflowID,
			RunID:           identity.RunID,
			Kind:            "rex.verification_policy",
			ContentType:     "application/json",
			StorageKind:     memory.ArtifactStorageInline,
			SummaryText:     "rex verification policy",
			InlineRawText:   string(payload),
			SummaryMetadata: map[string]any{"route": decision.Family},
			CreatedAt:       now,
		}); err != nil {
			return err
		}
	}
	if raw, ok := stateCtx.Get("rex.verification"); ok && raw != nil {
		payload, err := json.Marshal(raw)
		if err != nil {
			return err
		}
		if err := store.UpsertWorkflowArtifact(ctx, memory.WorkflowArtifactRecord{
			ArtifactID:      identity.RunID + ":verification",
			WorkflowID:      identity.WorkflowID,
			RunID:           identity.RunID,
			Kind:            "rex.verification",
			ContentType:     "application/json",
			StorageKind:     memory.ArtifactStorageInline,
			SummaryText:     "rex verification evidence",
			InlineRawText:   string(payload),
			SummaryMetadata: map[string]any{"route": decision.Family},
			CreatedAt:       now,
		}); err != nil {
			return err
		}
	}
	if raw, ok := stateCtx.Get("rex.success_gate"); ok && raw != nil {
		payload, err := json.Marshal(raw)
		if err != nil {
			return err
		}
		if err := store.UpsertWorkflowArtifact(ctx, memory.WorkflowArtifactRecord{
			ArtifactID:      identity.RunID + ":success-gate",
			WorkflowID:      identity.WorkflowID,
			RunID:           identity.RunID,
			Kind:            "rex.success_gate",
			ContentType:     "application/json",
			StorageKind:     memory.ArtifactStorageInline,
			SummaryText:     "rex success gate",
			InlineRawText:   string(payload),
			SummaryMetadata: map[string]any{"route": decision.Family},
			CreatedAt:       now,
		}); err != nil {
			return err
		}
	}
	return store.UpsertWorkflowArtifact(ctx, memory.WorkflowArtifactRecord{
		ArtifactID:      identity.RunID + ":completion",
		WorkflowID:      identity.WorkflowID,
		RunID:           identity.RunID,
		Kind:            "rex.completion",
		ContentType:     "application/json",
		StorageKind:     memory.ArtifactStorageInline,
		SummaryText:     "rex completion decision",
		InlineRawText:   string(completionJSON),
		SummaryMetadata: map[string]any{"route": decision.Family},
		CreatedAt:       now,
	})
}

func persistContextExpansion(ctx context.Context, store interface {
	UpsertWorkflowArtifact(context.Context, memory.WorkflowArtifactRecord) error
}, identity state.Identity, expansion retrieval.Expansion) error {
	providerKinds := map[string]any{
		"local_paths":         append([]string{}, expansion.LocalPaths...),
		"widened_to_workflow": expansion.WidenedToWorkflow,
		"summary":             expansion.Summary,
		"strategy":            expansion.ExpansionStrategy,
		"workflow_retrieval":  expansion.WorkflowRetrieval,
	}
	raw, err := json.Marshal(providerKinds)
	if err != nil {
		return err
	}
	return store.UpsertWorkflowArtifact(ctx, memory.WorkflowArtifactRecord{
		ArtifactID:      identity.RunID + ":context-expansion",
		WorkflowID:      identity.WorkflowID,
		RunID:           identity.RunID,
		Kind:            "rex.context_expansion",
		ContentType:     "application/json",
		StorageKind:     memory.ArtifactStorageInline,
		SummaryText:     expansion.Summary,
		InlineRawText:   string(raw),
		SummaryMetadata: map[string]any{"strategy": expansion.ExpansionStrategy, "widened_to_workflow": expansion.WidenedToWorkflow},
		CreatedAt:       time.Now().UTC(),
	})
}

func uniqueStrings(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	out := make([]string, 0, len(values))
	for _, value := range values {
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
}

func resolveWorkspaceRoot(workspace string) string {
	if trimmed := filepath.Clean(workspace); trimmed != "" && trimmed != "." {
		return trimmed
	}
	cwd, err := os.Getwd()
	if err != nil {
		return "."
	}
	current := cwd
	for {
		if _, err := os.Stat(filepath.Join(current, frameworkconfig.DirName)); err == nil {
			return current
		}
		parent := filepath.Dir(current)
		if parent == current {
			break
		}
		current = parent
	}
	return cwd
}
