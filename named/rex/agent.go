package rex

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"codeburg.org/lexbit/relurpify/framework/agentenv"
	"codeburg.org/lexbit/relurpify/framework/agentgraph"
	"codeburg.org/lexbit/relurpify/framework/cfgload"
	"codeburg.org/lexbit/relurpify/framework/contextdata"
	"codeburg.org/lexbit/relurpify/framework/core"
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
	"codeburg.org/lexbit/relurpify/named/rex/store"
)

// Agent is the Nexus-managed named runtime for rex.
type Agent struct {
	Environment *agentenv.WorkspaceEnvironment
	Runtime     *rexruntime.Manager
	Observer    state.ExecutionObserver

	config      *core.Config
	workspace   string
	rexConfig   rexconfig.Config
	delegates   *delegates.Registry
	reconciler  reconcile.Reconciler
	lastProof   proof.ProofSurface
}

type executionSurfaces struct {
	Runtime  *rexruntime.Manager
	Workflow *store.SQLiteWorkflowStore
}

type executionPlan struct {
	Identity       state.Identity
	Classification classify.Classification
	Decision       route.RouteDecision
	RoutePlan      route.ExecutionPlan
	Delegate       delegates.Delegate
	EventSuffix    string
	ExecutionTask  *core.Task
}

func New(env *agentenv.WorkspaceEnvironment) *Agent {
	return NewWithWorkspace(env, "")
}

func NewWithWorkspace(env *agentenv.WorkspaceEnvironment, workspace string) *Agent {
	if env == nil {
		panic("rex workspace environment required")
	}
	agent := &Agent{}
	_ = agent.InitializeEnvironment(env, workspace)
	return agent
}

func (a *Agent) InitializeEnvironment(env *agentenv.WorkspaceEnvironment, workspace string) error {
	a.Environment = env
	a.config = env.Config
	a.rexConfig = rexconfig.Default()
	a.workspace = resolveWorkspaceRoot(workspace)
	a.delegates = delegates.NewRegistry(env, a.workspace)
	a.Runtime = rexruntime.New(a.rexConfig, env.WorkingMemory)
	a.reconciler = &reconcile.InMemoryReconciler{}
	return a.Initialize(env.Config)
}

func (a *Agent) Initialize(cfg *core.Config) error {
	a.config = cfg
	if a.Runtime == nil {
		a.Runtime = rexruntime.New(a.rexConfig, a.Environment.WorkingMemory)
	}
	return nil
}

func (a *Agent) Capabilities() []string {
	return []string{
		"plan",
		"execute",
		"code",
		"explain",
		"human-in-loop",
	}
}

func (a *Agent) BuildGraph(task *core.Task) (*agentgraph.Graph, error) {
	env := envelope.Normalize(task, nil)
	class := classify.Classify(env)
	decision := route.Decide(env, class)
	plan := route.BuildExecutionPlan(decision)
	delegate, err := a.delegates.Resolve(plan)
	if err != nil {
		return nil, err
	}
	return delegate.BuildGraph(task)
}

func (a *Agent) Execute(ctx context.Context, task *core.Task, env *contextdata.Envelope) (*core.Result, error) {
	surfaces := a.openSurfaces(ctx, task)
	plan, err := a.planExecution(ctx, task, env, surfaces)
	if err != nil {
		return nil, err
	}
	return a.runExecution(ctx, task, env, plan, surfaces)
}

func (a *Agent) RuntimeProjection() nexus.Projection {
	return nexus.BuildProjection(a.Runtime, a.lastProof)
}

func (a *Agent) openSurfaces(ctx context.Context, task *core.Task) executionSurfaces {
	_ = ctx
	_ = task
	if a.Runtime == nil {
		return executionSurfaces{}
	}
	return executionSurfaces{
		Runtime:  a.Runtime,
		Workflow: a.Runtime.WorkflowStore(),
	}
}

func (a *Agent) planExecution(ctx context.Context, task *core.Task, env *contextdata.Envelope, surfaces executionSurfaces) (*executionPlan, error) {
	_ = ctx
	_ = surfaces
	if a.delegates == nil {
		return nil, fmt.Errorf("rex delegate registry unavailable")
	}
	rexEnv := envelope.Normalize(task, env)
	class := classify.Classify(rexEnv)
	decision := route.Decide(rexEnv, class)
	routePlan := route.BuildExecutionPlan(decision)
	identity := state.ComputeIdentity(rexEnv)
	state.SetEnvelopeWorkflowID(env, identity.WorkflowID)
	state.SetEnvelopeRunID(env, identity.RunID)
	state.SetResumedRoute(env, decision.Family)
	if err := enforceCapabilityProjection(env, decision, task); err != nil {
		return nil, err
	}
	delegate, err := a.delegates.Resolve(routePlan)
	if err != nil {
		return nil, err
	}
	return &executionPlan{
		Identity:       identity,
		Classification: class,
		Decision:       decision,
		RoutePlan:      routePlan,
		Delegate:       delegate,
		EventSuffix:    executionEventSuffix(env),
		ExecutionTask:  task,
	}, nil
}

func (a *Agent) runExecution(ctx context.Context, task *core.Task, env *contextdata.Envelope, plan *executionPlan, surfaces executionSurfaces) (*core.Result, error) {
	var result *core.Result
	var execErr error
	if a.Observer != nil {
		if err := a.Observer.BeforeExecute(ctx, plan.Identity.WorkflowID, plan.Identity.RunID, task, env); err != nil {
			return nil, err
		}
		defer func() {
			_ = a.Observer.AfterExecute(ctx, plan.Identity.WorkflowID, plan.Identity.RunID, task, env, result, execErr)
		}()
	}
	if surfaces.Runtime != nil {
		finishRuntime := surfaces.Runtime.BeginExecution(plan.Identity.WorkflowID, plan.Identity.RunID)
		defer func() {
			finishRuntime(execErr)
		}()
	}
	result, execErr = a.runDelegate(ctx, task, env, plan, surfaces)
	completion := proof.EvaluateCompletion(plan.Decision, plan.Classification, env)
	if result != nil && !completion.Allowed {
		result.Success = false
		blockErr := fmt.Errorf("rex completion blocked: %s", completion.Reason)
		result.Error = blockErr.Error()
		execErr = blockErr
	}
	a.persistOutcome(ctx, task, env, plan, result, surfaces)
	if !completion.Allowed {
		return result, execErr
	}
	if execErr != nil {
		return result, execErr
	}
	return result, nil
}

func (a *Agent) runDelegate(ctx context.Context, task *core.Task, env *contextdata.Envelope, plan *executionPlan, surfaces executionSurfaces) (*core.Result, error) {
	if plan == nil {
		return nil, fmt.Errorf("execution plan required")
	}
	executionTask := plan.ExecutionTask
	if surfaces.Workflow != nil {
		if err := state.EnsureWorkflowRun(ctx, surfaces.Workflow, plan.Identity, task, plan.Decision.Mode); err != nil {
			return nil, err
		}
		if err := surfaces.Workflow.AppendEvent(ctx, store.WorkflowEventRecord{
			EventID:    plan.Identity.RunID + ":" + plan.EventSuffix + ":start",
			WorkflowID: plan.Identity.WorkflowID,
			RunID:      plan.Identity.RunID,
			EventType:  "rex.run.started",
			Message:    "rex execution started",
			Metadata:   map[string]any{"route": plan.Decision.Family, "mode": plan.Decision.Mode, "profile": plan.Decision.Profile},
			CreatedAt:  time.Now().UTC(),
		}); err != nil {
			logOutcomeError("append workflow start event", err)
		}
	}
	if plan.RoutePlan.RequireRetrieval && surfaces.Workflow != nil {
		expansion, err := retrieval.ExpandWithWorkflowStore(ctx, surfaces.Workflow, plan.Identity.WorkflowID, task, env, plan.Decision)
		if err == nil {
			executionTask = retrieval.Apply(env, task, expansion)
			artifactKinds := []string{"rex.proof_surface", "rex.action_log", "rex.completion"}
			if len(expansion.LocalPaths) > 0 {
				artifactKinds = append(artifactKinds, "rex.context_expansion")
			}
			if len(expansion.WorkflowRetrieval) > 0 {
				artifactKinds = append(artifactKinds, "rex.workflow_retrieval")
			}
			state.SetArtifactKinds(env, artifactKinds)
			if err := persistContextExpansion(ctx, surfaces.Workflow, plan.Identity, expansion); err != nil {
				logOutcomeError("persist context expansion", err)
			}
		}
	}
	if plan.Delegate == nil {
		return nil, fmt.Errorf("rex delegate unavailable")
	}
	result, err := plan.Delegate.Execute(ctx, executionTask, env)
	if result == nil {
		result = &core.Result{Success: err == nil}
	}
	if err != nil {
		result.Success = false
		result.Error = err.Error()
	}
	plan.ExecutionTask = executionTask
	return result, err
}

func (a *Agent) persistOutcome(ctx context.Context, task *core.Task, env *contextdata.Envelope, plan *executionPlan, result *core.Result, surfaces executionSurfaces) {
	_ = task
	if plan == nil {
		return
	}
	if result == nil {
		result = &core.Result{}
	}
	fields := core.ResultFields(result.Data)
	if fields == nil {
		fields = map[string]any{}
		result.Data = core.NewToolResultPayload(fields)
	}
	completion := proof.EvaluateCompletion(plan.Decision, plan.Classification, env)
	artifactKinds := []string{"rex.proof_surface", "rex.action_log", "rex.completion", "rex.verification_policy", "rex.success_gate"}
	if verification := proof.VerificationEvidence(env); verification.EvidencePresent {
		artifactKinds = append(artifactKinds, "rex.verification")
	}
	if existing := state.ArtifactKinds(env); len(existing) > 0 {
		artifactKinds = append(existing, artifactKinds...)
	}
	state.SetArtifactKinds(env, uniqueStrings(artifactKinds))
	actionLog := proof.BuildActionLog(plan.Decision, plan.Classification, env)
	fields["rex.action_log"] = actionLog
	a.lastProof = proof.BuildProofSurface(plan.Decision, result, env)
	fields["rex.proof_surface"] = a.lastProof
	fields["rex.completion"] = completion
	fields[rexkeys.RexWorkflowID] = plan.Identity.WorkflowID
	fields[rexkeys.RexRunID] = plan.Identity.RunID
	fields["rex.route"] = plan.Decision.Family
	if surfaces.Workflow == nil {
		return
	}
	if err := persistProof(ctx, surfaces.Workflow, plan.Identity, plan.Decision, a.lastProof, actionLog, completion, env); err != nil {
		logOutcomeError("persist proof outcome", err)
	}
	status := memory.WorkflowRunStatusCompleted
	now := time.Now().UTC()
	finishedAt := &now
	if result.Error != "" || !completion.Allowed {
		status = memory.WorkflowRunStatusFailed
		result.Success = false
	}
	if err := surfaces.Workflow.UpdateRunStatus(ctx, plan.Identity.RunID, status, finishedAt); err != nil {
		logOutcomeError("update run status", err)
	}
	if _, err := surfaces.Workflow.UpdateWorkflowStatus(ctx, plan.Identity.WorkflowID, 0, status, ""); err != nil {
		logOutcomeError("update workflow status", err)
	}
	// best-effort: workflow event loss is acceptable
	_ = surfaces.Workflow.AppendEvent(ctx, store.WorkflowEventRecord{
		EventID:    plan.Identity.RunID + ":" + plan.EventSuffix + ":finish",
		WorkflowID: plan.Identity.WorkflowID,
		RunID:      plan.Identity.RunID,
		EventType:  "rex.run.finished",
		Message:    "rex execution finished",
		Metadata:   map[string]any{"route": plan.Decision.Family, "allowed": completion.Allowed, "success": result.Success},
		CreatedAt:  now,
	})
}

func executionEventSuffix(env *contextdata.Envelope) string {
	if id := state.EventID(env); id != "" {
		return id
	}
	return "runtime"
}

func logOutcomeError(step string, err error) {
	if err == nil {
		return
	}
	_, _ = fmt.Fprintf(os.Stderr, "rex %s: %v\n", step, err)
}

func (a *Agent) ManagedAdapter() *nexus.Adapter {
	surfaces := state.ResolveRuntimeSurfaces(a.Environment.WorkingMemory)
	return nexus.NewAdapter("rex", a, surfaces.Workflow)
}

func (a *Agent) SetReconciler(r reconcile.Reconciler) {
	a.reconciler = r
}

func (a *Agent) RecordAmbiguity(workflowID, runID, reason string) reconcile.Record {
	if a.reconciler == nil {
		a.reconciler = &reconcile.InMemoryReconciler{}
	}
	return a.reconciler.RecordAmbiguity(workflowID, runID, reason)
}

func (a *Agent) ResolveAmbiguity(record reconcile.Record, outcome reconcile.Outcome, notes string) reconcile.Record {
	if a.reconciler == nil {
		a.reconciler = &reconcile.InMemoryReconciler{}
	}
	return a.reconciler.Resolve(record, outcome, notes)
}

func (a *Agent) ShouldRetryAmbiguity(record reconcile.Record) bool {
	if a.reconciler == nil {
		a.reconciler = &reconcile.InMemoryReconciler{}
	}
	return a.reconciler.ShouldRetry(record)
}

func persistProof(ctx context.Context, store interface {
	UpsertWorkflowArtifact(context.Context, memory.WorkflowArtifactRecord) error
}, identity state.Identity, decision route.RouteDecision, surface proof.ProofSurface, actionLog []proof.ActionLogEntry, completion proof.CompletionDecision, env *contextdata.Envelope) error {
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
	if policy, ok := contextdata.GetTyped[proof.VerificationPolicy](env, rexkeys.RexVerificationPolicy); ok {
		payload, err := json.Marshal(policy)
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
	if evidence, ok := contextdata.GetTyped[proof.VerificationEvidenceRecord](env, rexkeys.RexVerification); ok {
		payload, err := json.Marshal(evidence)
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
	if gate, ok := contextdata.GetTyped[proof.SuccessGateResult](env, rexkeys.RexSuccessGate); ok {
		payload, err := json.Marshal(gate)
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
		if _, err := os.Stat(filepath.Join(current, cfgload.DirName)); err == nil {
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
