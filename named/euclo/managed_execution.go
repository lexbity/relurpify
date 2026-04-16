package euclo

import (
	"context"
	"fmt"
	"time"

	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/framework/graph"
	"github.com/lexcodex/relurpify/named/euclo/euclotypes"
	eucloruntime "github.com/lexcodex/relurpify/named/euclo/runtime"
	eucloarchaeomem "github.com/lexcodex/relurpify/named/euclo/runtime/archaeomem"
	eucloassurance "github.com/lexcodex/relurpify/named/euclo/runtime/assurance"
	euclocontext "github.com/lexcodex/relurpify/named/euclo/runtime/context"
	euclointake "github.com/lexcodex/relurpify/named/euclo/runtime/intake"
	euclopolicy "github.com/lexcodex/relurpify/named/euclo/runtime/policy"
	eucloreporting "github.com/lexcodex/relurpify/named/euclo/runtime/reporting"
	euclostate "github.com/lexcodex/relurpify/named/euclo/runtime/state"
	euclowork "github.com/lexcodex/relurpify/named/euclo/runtime/work"
)

type managedExecutionFlow struct {
	task             *core.Task
	state            *core.Context
	workflowExecutor graph.WorkflowExecutor
	envelope         eucloruntime.TaskEnvelope
	classification   eucloruntime.TaskClassification
	mode             euclotypes.ModeResolution
	profile          euclotypes.ExecutionProfileSelection
	work             eucloruntime.UnitOfWork
	classified       euclointake.ClassifiedEnvelope // Keep the full envelope for rebuilds
	contextRuntime   *euclocontext.ContextRuntime
	prep             executionPreparation
}

func (a *Agent) initializeManagedExecution(ctx context.Context, task *core.Task, state *core.Context, workflowExecutor graph.WorkflowExecutor) (*managedExecutionFlow, *core.Result, error) {
	if state == nil {
		state = core.NewContext()
	}

	// Apply session resume context if present before enrichment.
	if err := a.applySessionResumeContext(ctx, task, state); err != nil {
		return nil, &core.Result{Success: false, Error: err}, err
	}

	if workflowExecutor == nil {
		if err := a.ensureReactDelegate(); err != nil {
			return nil, nil, err
		}
	} else {
		if err := workflowExecutor.Initialize(a.Config); err != nil {
			return nil, &core.Result{Success: false, Error: err}, err
		}
	}
	a.ensureGuidanceWiring()
	seedPersistedInteractionState(task, state)
	sessionID := generateSessionID()
	if scopeErr := enforceSessionScoping(state, sessionID); scopeErr != nil {
		return nil, &core.Result{Success: false, Error: scopeErr}, scopeErr
	}

	// Single-pass enrichment builds the classified envelope and unit of work together.
	semanticInputs := a.semanticInputBundle(task, state, euclotypes.ModeResolution{})
	skillPolicy := eucloruntime.BuildResolvedExecutionPolicy(task, a.Config, a.CapabilityRegistry(), euclotypes.ModeResolution{}, euclotypes.ExecutionProfileSelection{})
	executorDescriptor := eucloruntime.WorkUnitExecutorDescriptor{}

	// Create capability classifier
	classifier := a.newCapabilityClassifier()

	classified, err := euclointake.RunEnrichment(ctx, task, state, a.Environment, a.ModeRegistry, a.ProfileRegistry, classifier, semanticInputs, skillPolicy, executorDescriptor)
	if err != nil {
		return nil, &core.Result{Success: false, Error: err}, err
	}

	// Persist the classified envelope to state.
	euclointake.SeedClassifiedEnvelope(state, classified)

	a.ensureDeferralPlan(task, state)
	a.ensureWorkflowRun(ctx, task, state)

	// Restore execution continuity; if it fails, rebuild work with updated state
	if restoreErr := a.restoreExecutionContinuity(ctx, task, state, classified.Envelope, classified.Work); restoreErr != nil {
		// Rebuild work after restore failure - only UnitOfWork, no re-classification
		classified.Work = euclointake.RebuildUnitOfWork(task, state, classified, a.ModeRegistry, classified.Work.SemanticInputs, classified.Work.ResolvedPolicy, classified.Work.ExecutorDescriptor)
		a.refreshRuntimeExecutionArtifacts(ctx, task, state, classified.Work, eucloruntime.ExecutionStatusRestoreFailed, restoreErr)
		result := &core.Result{Success: false, Error: restoreErr}
		result.Metadata = map[string]any{"result_class": string(eucloruntime.ExecutionResultClassRestoreFailed)}
		a.applyRuntimeResultMetadata(result, state)
		return nil, result, restoreErr
	}
	if err := a.applyLearningResolution(ctx, task, state); err != nil {
		return nil, &core.Result{Success: false, Error: err}, err
	}

	contextRuntime := euclocontext.BuildContextRuntime(task, state, euclocontext.ContextRuntimeConfig{
		Config:            a.Config,
		Model:             a.Environment.Model,
		MemoryStore:       a.Memory,
		IndexManager:      a.Environment.IndexManager,
		SearchEngine:      a.Environment.SearchEngine,
		BKCBootstrapReady: a.WorkspaceEnv.BKCEvents == nil || a.WorkspaceEnv.BKCEvents.BootstrapReady(),
	}, classified.Mode, classified.Work)
	if contextRuntime != nil {
		contextRuntime.Activate(task, state, a.Environment.Model)
		euclostate.SetSharedContextRuntime(state, euclopolicy.BuildSharedContextRuntimeState(contextRuntime.Shared, classified.Work))
	}
	securityRuntime := euclopolicy.BuildSecurityRuntimeState(a.Config, a.CapabilityRegistry(), a.runtimeProviders(state), state, classified.Work)
	euclostate.SetSecurityRuntime(state, securityRuntime)
	contractRuntime := eucloruntime.BuildCapabilityContractRuntimeState(classified.Work, state, time.Now().UTC())
	euclostate.SetCapabilityContractRuntime(state, contractRuntime)
	euclostate.SetArchaeologyCapabilityRuntime(state, eucloarchaeomem.BuildArchaeologyCapabilityRuntimeState(classified.Work, state, time.Now().UTC()))
	euclostate.SetDebugCapabilityRuntime(state, eucloreporting.BuildDebugCapabilityRuntimeState(classified.Work, state, time.Now().UTC()))
	euclostate.SetChatCapabilityRuntime(state, eucloreporting.BuildChatCapabilityRuntimeState(classified.Work, state, time.Now().UTC()))
	if contractErr := eucloruntime.EnforcePreExecutionCapabilityContracts(classified.Work); contractErr != nil {
		classified.Work.Status = eucloruntime.UnitOfWorkStatusBlocked
		classified.Work.ResultClass = eucloruntime.ExecutionResultClassBlocked
		a.refreshRuntimeExecutionArtifacts(ctx, task, state, classified.Work, eucloruntime.ExecutionStatusBlocked, contractErr)
		result := &core.Result{Success: false, Error: contractErr}
		result.Metadata = map[string]any{"result_class": string(eucloruntime.ExecutionResultClassBlocked)}
		a.applyRuntimeResultMetadata(result, state)
		return nil, result, contractErr
	}

	return &managedExecutionFlow{
		task:             task,
		state:            state,
		workflowExecutor: workflowExecutor,
		envelope:         classified.Envelope,
		classification:   classified.Classification,
		mode:             classified.Mode,
		profile:          classified.Profile,
		work:             classified.Work,
		classified:       classified,
		contextRuntime:   contextRuntime,
		prep:             executionPreparation{},
	}, nil, nil
}

func (a *Agent) executeManagedExecution(ctx context.Context, flow *managedExecutionFlow) (*core.Result, error) {
	if flow == nil {
		return nil, fmt.Errorf("managed execution flow unavailable")
	}
	prep := a.prepareExecution(ctx, flow.task, flow.state, flow.classification, flow.profile)
	if handledResult, handledErr, handled := a.phaseDriver().HandlePreparationOutcome(ctx, flow.task, flow.state, prep.preflightResult, prep.err, a.DeferralPlan); handled {
		return handledResult, handledErr
	}

	flow.prep = prep
	flow.work = euclowork.BuildUnitOfWork(flow.task, flow.state, flow.envelope, flow.classification, flow.mode, flow.profile, a.ModeRegistry, flow.work.SemanticInputs, flow.work.ResolvedPolicy, flow.work.ExecutorDescriptor)
	// Update classified envelope with rebuilt work and persist to state
	flow.classified.Work = flow.work
	euclointake.SeedClassifiedEnvelope(flow.state, flow.classified)

	if prep.activeStep == nil && (hasTerminalExecutionPreparation(prep) || shouldShortCircuitExecution(flow.state)) {
		flow.work.Status = eucloruntime.UnitOfWorkStatusCompleted
		flow.work.ResultClass = euclowork.ResultClassForOutcome(euclowork.ExecutionStatusCompleted, flow.work.DeferredIssueIDs, nil)
		euclostate.SetUnitOfWork(flow.state, flow.work)
		euclowork.SeedCompiledExecutionState(flow.state, flow.work, euclowork.BuildRuntimeExecutionStatus(flow.work, euclowork.ExecutionStatusCompleted, flow.work.ResultClass, time.Now().UTC()))
		short := a.shortCircuitResult(flow.state, prep)
		sessionOutput := eucloassurance.ShortCircuit(a.assuranceRuntime(), ctx, eucloassurance.ShortCircuitInput{
			Task:            flow.task,
			State:           flow.state,
			Mode:            flow.mode,
			Profile:         flow.profile,
			Telemetry:       a.ConfigTelemetry(),
			Result:          short,
			SkipSuccessGate: true,
		})
		if a.DeferralPlan != nil && !a.DeferralPlan.IsEmpty() {
			euclostate.SetDeferralPlan(flow.state, a.DeferralPlan)
		}
		if flow.contextRuntime != nil {
			flow.contextRuntime.HandleResult(flow.state, a.Environment.Model, sessionOutput.Result)
			euclostate.SetSharedContextRuntime(flow.state, euclopolicy.BuildSharedContextRuntimeState(flow.contextRuntime.Shared, flow.work))
		}
		a.phaseDriver().EnterSurfacing(ctx, flow.task, flow.state, nil, sessionOutput.Err)
		a.phaseDriver().Complete(ctx, flow.task, flow.state, nil, sessionOutput.Err)
		a.refreshRuntimeExecutionArtifacts(ctx, flow.task, flow.state, flow.work, eucloruntime.ExecutionStatusCompleted, sessionOutput.Err)
		a.applyRuntimeResultMetadata(sessionOutput.Result, flow.state)
		return sessionOutput.Result, sessionOutput.Err
	}

	if prep.activeStep != nil {
		a.phaseDriver().EnterExecution(ctx, flow.task, flow.state, prep.activeStep)
	}
	if !prep.summaryFastPath && shouldHydratePersistedArtifacts(flow.task, flow.state, flow.envelope) {
		a.hydratePersistedArtifacts(ctx, flow.task, flow.state)
	}
	flow.work.Status = eucloruntime.UnitOfWorkStatusExecuting
	euclostate.SetUnitOfWork(flow.state, flow.work)
	euclowork.SeedCompiledExecutionState(flow.state, flow.work, euclowork.BuildRuntimeExecutionStatus(flow.work, euclowork.ExecutionStatusExecuting, "", time.Now().UTC()))
	executionTask := a.eucloTask(flow.task, flow.envelope, flow.classification, flow.mode, flow.profile, flow.work)
	sessionOutput := eucloassurance.Execute(a.assuranceRuntime(), ctx, eucloassurance.Input{
		Task:             flow.task,
		ExecutionTask:    executionTask,
		WorkflowExecutor: flow.workflowExecutor,
		State:            flow.state,
		Classification:   flow.classification,
		Mode:             flow.mode,
		Profile:          flow.profile,
		Telemetry:        a.ConfigTelemetry(),
		Work:             flow.work,
		ServiceBundle:    a.serviceBundle(),
	})

	result := sessionOutput.Result
	err := sessionOutput.Err
	if flow.contextRuntime != nil {
		flow.contextRuntime.HandleResult(flow.state, a.Environment.Model, result)
		euclostate.SetSharedContextRuntime(flow.state, euclopolicy.BuildSharedContextRuntimeState(flow.contextRuntime.Shared, flow.work))
	}
	a.phaseDriver().EnterVerification(ctx, flow.task, flow.state, prep.activeStep, err)
	a.executionFinalizer().FinalizeLivingPlan(ctx, flow.task, flow.state, prep.livingPlan, prep.activeStep, result, err)
	a.phaseDriver().EnterSurfacing(ctx, flow.task, flow.state, prep.activeStep, err)
	if a.DeferralPlan != nil && !a.DeferralPlan.IsEmpty() {
		euclostate.SetDeferralPlan(flow.state, a.DeferralPlan)
	}
	a.phaseDriver().Complete(ctx, flow.task, flow.state, prep.activeStep, err)
	postContractRuntime, contractErr := eucloruntime.EvaluatePostExecutionCapabilityContracts(flow.work, flow.state, time.Now().UTC())
	euclostate.SetCapabilityContractRuntime(flow.state, postContractRuntime)
	euclostate.SetArchaeologyCapabilityRuntime(flow.state, eucloarchaeomem.BuildArchaeologyCapabilityRuntimeState(flow.work, flow.state, time.Now().UTC()))
	euclostate.SetDebugCapabilityRuntime(flow.state, eucloreporting.BuildDebugCapabilityRuntimeState(flow.work, flow.state, time.Now().UTC()))
	euclostate.SetChatCapabilityRuntime(flow.state, eucloreporting.BuildChatCapabilityRuntimeState(flow.work, flow.state, time.Now().UTC()))
	if err == nil && contractErr != nil {
		err = contractErr
	}
	finalStatus := eucloruntime.ExecutionStatusCompleted
	if err != nil {
		finalStatus = eucloruntime.ExecutionStatusFailed
	}
	a.refreshRuntimeExecutionArtifacts(ctx, flow.task, flow.state, flow.work, finalStatus, err)
	a.applyRuntimeResultMetadata(result, flow.state)
	return result, err
}

// applySessionResumeContext injects SessionResumeContext into the task context
// and state before enrichment. This enables resumed sessions to warm up with
// their previously anchored BKC context and semantic state.
func (a *Agent) applySessionResumeContext(ctx context.Context, task *core.Task, state *core.Context) error {
	raw, ok := state.Get("euclo.session_resume_context")
	if !ok || raw == nil {
		return nil // no resume context; new session path
	}
	resumeCtx, ok := raw.(session.SessionResumeContext)
	if !ok || resumeCtx.IsEmpty() {
		return nil
	}

	// Apply workflow and run IDs so EnsureWorkflowRun uses the resumed workflow.
	if task.Context == nil {
		task.Context = map[string]any{}
	}
	task.Context["workflow_id"] = resumeCtx.WorkflowID
	task.Context["run_id"] = resumeCtx.RunID

	// Apply mode hint so mode resolution resolves to the prior mode.
	if resumeCtx.Mode != "" && task.Context["mode_hint"] == nil {
		task.Context["mode_hint"] = resumeCtx.Mode
	}

	// Apply BKC root chunk IDs so wrapBKCStrategy receives them via
	// bkcSeedChunks -> work.PlanBinding.RootChunkIDs.
	if len(resumeCtx.RootChunkIDs) > 0 {
		task.Context["root_chunk_ids"] = resumeCtx.RootChunkIDs
		task.Context["active_plan_root_chunk_ids"] = resumeCtx.RootChunkIDs
	}

	// Apply phase state so shouldShortCircuitExecution and phase-gated
	// behaviors pick up where the prior session left off.
	if resumeCtx.PhaseState != nil {
		euclostate.SetArchaeoPhaseState(state, resumeCtx.PhaseState)
	}

	// Apply code revision for BKC staleness checking.
	if resumeCtx.CodeRevision != "" {
		euclostate.SetCodeRevision(state, resumeCtx.CodeRevision)
		state.Set("euclo.last_session_revision", resumeCtx.CodeRevision)
	}

	// Apply session start time so pretask learning delta can compare
	// resolved interactions against the prior session boundary.
	if !resumeCtx.SessionStartTime.IsZero() {
		state.Set("euclo.session_start_time", resumeCtx.SessionStartTime)
	}

	// Seed executor semantic context from resume's semantic summary.
	if !resumeCtx.SemanticSummary.IsEmpty() {
		semCtx := resumeCtx.SemanticSummary.ToExecutorSemanticContext(resumeCtx.ActivePlanSummary)
		euclostate.SetResumeSemanticContext(state, semCtx)
	}

	return nil
}
