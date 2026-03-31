package euclo

import (
	"context"
	"fmt"
	"time"

	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/framework/graph"
	eucloexec "github.com/lexcodex/relurpify/named/euclo/execution"
	eucloarchaeomem "github.com/lexcodex/relurpify/named/euclo/runtime/archaeomem"
	euclocontext "github.com/lexcodex/relurpify/named/euclo/runtime/context"
	euclopolicy "github.com/lexcodex/relurpify/named/euclo/runtime/policy"
	eucloreporting "github.com/lexcodex/relurpify/named/euclo/runtime/reporting"
	eucloruntime "github.com/lexcodex/relurpify/named/euclo/runtime"
	eucloses "github.com/lexcodex/relurpify/named/euclo/runtime/session"
	euclowork "github.com/lexcodex/relurpify/named/euclo/runtime/work"
)

func (a *Agent) BuildGraph(task *core.Task) (*graph.Graph, error) {
	env, classification, mode, profile, work := a.runtimeState(task, nil)
	executor := a.selectExecutor(work)
	return executor.BuildGraph(&eucloexec.ExecutorContext{
		Task:           task,
		Envelope:       env,
		Classification: classification,
		Mode:           mode,
		Profile:        profile,
		Work:           work,
	})
}

func (a *Agent) Execute(ctx context.Context, task *core.Task, state *core.Context) (*core.Result, error) {
	if state == nil {
		state = core.NewContext()
	}
	envelope, classification, mode, profile, work := a.runtimeState(task, state)
	a.seedRuntimeState(state, envelope, classification, mode, profile, work)
	executor := a.selectExecutor(work)
	return executor.Execute(ctx, &eucloexec.ExecutorContext{
		Task:           task,
		State:          state,
		Envelope:       envelope,
		Classification: classification,
		Mode:           mode,
		Profile:        profile,
		Work:           work,
	})
}

func (a *Agent) executeWithWorkflowExecutor(ctx context.Context, exec *eucloexec.ExecutorContext, executor graph.WorkflowExecutor) (*core.Result, error) {
	if exec == nil || executor == nil {
		return &core.Result{Success: false, Error: fmt.Errorf("executor context unavailable")}, fmt.Errorf("executor context unavailable")
	}
	task := exec.Task
	state := exec.State
	if state == nil {
		state = core.NewContext()
	}
	return a.executeManagedFlow(ctx, task, state, executor)
}

func (a *Agent) executeManagedFlow(ctx context.Context, task *core.Task, state *core.Context, workflowExecutor graph.WorkflowExecutor) (*core.Result, error) {
	// ── Setup ────────────────────────────────────────────────────────────────
	if state == nil {
		state = core.NewContext()
	}
	if workflowExecutor == nil {
		if err := a.ensureReactDelegate(); err != nil {
			return nil, err
		}
	} else {
		if err := workflowExecutor.Initialize(a.Config); err != nil {
			return &core.Result{Success: false, Error: err}, err
		}
	}
	a.ensureGuidanceWiring()
	seedPersistedInteractionState(task, state)
	sessionID := generateSessionID()
	if scopeErr := enforceSessionScoping(state, sessionID); scopeErr != nil {
		return &core.Result{Success: false, Error: scopeErr}, scopeErr
	}
	// First runtimeState: build initial mode/profile/work from raw task + state
	// before any restore or learning resolution has run.
	envelope, classification, mode, profile, work := a.runtimeState(task, state)
	a.seedRuntimeState(state, envelope, classification, mode, profile, work)
	a.ensureDeferralPlan(task, state)
	a.ensureWorkflowRun(ctx, task, state)
	if restoreErr := a.restoreExecutionContinuity(ctx, task, state, envelope, work); restoreErr != nil {
		work = euclowork.BuildUnitOfWork(task, state, envelope, classification, mode, profile, a.ModeRegistry, work.SemanticInputs, work.ResolvedPolicy, work.ExecutorDescriptor)
		a.refreshRuntimeExecutionArtifacts(ctx, task, state, work, eucloruntime.ExecutionStatusRestoreFailed, restoreErr)
		result := &core.Result{Success: false, Error: restoreErr}
		result.Metadata = map[string]any{"result_class": string(eucloruntime.ExecutionResultClassRestoreFailed)}
		a.applyRuntimeResultMetadata(result, state)
		return result, restoreErr
	}
	// Second runtimeState: re-derive mode/profile/work after restoreExecutionContinuity
	// has potentially written restored workflow state (step position, prior artifacts,
	// continuation context) into state — those changes must be reflected in UnitOfWork.
	envelope, classification, mode, profile, work = a.runtimeState(task, state)
	a.seedRuntimeState(state, envelope, classification, mode, profile, work)
	// ── Pre-execution ────────────────────────────────────────────────────────
	if err := a.applyLearningResolution(ctx, task, state); err != nil {
		return &core.Result{Success: false, Error: err}, err
	}
	contextRuntime := euclocontext.BuildContextRuntime(task, euclocontext.ContextRuntimeConfig{
		Config:       a.Config,
		Model:        a.Environment.Model,
		MemoryStore:  a.Memory,
		IndexManager: a.Environment.IndexManager,
		SearchEngine: a.Environment.SearchEngine,
	}, mode, work)
	if contextRuntime != nil {
		contextRuntime.Activate(task, state, a.Environment.Model)
		state.Set("euclo.shared_context_runtime", euclopolicy.BuildSharedContextRuntimeState(contextRuntime.Shared, work))
	}
	securityRuntime := euclopolicy.BuildSecurityRuntimeState(a.Config, a.CapabilityRegistry(), a.runtimeProviders(state), state, work)
	state.Set("euclo.security_runtime", securityRuntime)
	contractRuntime := eucloruntime.BuildCapabilityContractRuntimeState(work, state, time.Now().UTC())
	state.Set("euclo.capability_contract_runtime", contractRuntime)
	state.Set("euclo.archaeology_capability_runtime", eucloarchaeomem.BuildArchaeologyCapabilityRuntimeState(work, state, time.Now().UTC()))
	state.Set("euclo.debug_capability_runtime", eucloreporting.BuildDebugCapabilityRuntimeState(work, state, time.Now().UTC()))
	state.Set("euclo.chat_capability_runtime", eucloreporting.BuildChatCapabilityRuntimeState(work, state, time.Now().UTC()))
	if contractErr := eucloruntime.EnforcePreExecutionCapabilityContracts(work); contractErr != nil {
		work.Status = eucloruntime.UnitOfWorkStatusBlocked
		work.ResultClass = eucloruntime.ExecutionResultClassBlocked
		a.refreshRuntimeExecutionArtifacts(ctx, task, state, work, eucloruntime.ExecutionStatusBlocked, contractErr)
		result := &core.Result{Success: false, Error: contractErr}
		result.Metadata = map[string]any{"result_class": string(eucloruntime.ExecutionResultClassBlocked)}
		a.applyRuntimeResultMetadata(result, state)
		return result, contractErr
	}
	prep := a.prepareExecution(ctx, task, state, classification, profile)
	if handledResult, handledErr, handled := a.phaseDriver().HandlePreparationOutcome(ctx, task, state, prep.preflightResult, prep.err, a.DeferralPlan); handled {
		return handledResult, handledErr
	}
	// Third work rebuild: re-assemble UnitOfWork after preflight has resolved the active
	// plan step, learning anchors, and deferral decisions — those outputs (activeStep,
	// livingPlan, resolvedPolicy) are written into state by prepareExecution and must be
	// reflected in UnitOfWork before dispatch.
	work = euclowork.BuildUnitOfWork(task, state, envelope, classification, mode, profile, a.ModeRegistry, work.SemanticInputs, work.ResolvedPolicy, work.ExecutorDescriptor)
	a.seedRuntimeState(state, envelope, classification, mode, profile, work)
	if prep.activeStep == nil && shouldShortCircuitExecution(prep, state) {
		work.Status = eucloruntime.UnitOfWorkStatusCompleted
		work.ResultClass = euclowork.ResultClassForOutcome(euclowork.ExecutionStatusCompleted, work.DeferredIssueIDs, nil)
		state.Set("euclo.unit_of_work", work)
		euclowork.SeedCompiledExecutionState(state, work, euclowork.BuildRuntimeExecutionStatus(work, euclowork.ExecutionStatusCompleted, work.ResultClass, time.Now().UTC()))
		short := a.shortCircuitResult(state, prep)
		sessionOutput := a.executionSession().ShortCircuit(ctx, eucloses.ShortCircuitInput{
			Task:            task,
			State:           state,
			Mode:            mode,
			Profile:         profile,
			Telemetry:       a.ConfigTelemetry(),
			Result:          short,
			SkipSuccessGate: true,
		})
		if a.DeferralPlan != nil && !a.DeferralPlan.IsEmpty() {
			state.Set("euclo.deferral_plan", a.DeferralPlan)
		}
		if contextRuntime != nil {
			contextRuntime.HandleResult(state, a.Environment.Model, sessionOutput.Result)
			state.Set("euclo.shared_context_runtime", euclopolicy.BuildSharedContextRuntimeState(contextRuntime.Shared, work))
		}
		a.phaseDriver().EnterSurfacing(ctx, task, state, nil, sessionOutput.Err)
		a.phaseDriver().Complete(ctx, task, state, nil, sessionOutput.Err)
		a.refreshRuntimeExecutionArtifacts(ctx, task, state, work, eucloruntime.ExecutionStatusCompleted, sessionOutput.Err)
		a.applyRuntimeResultMetadata(sessionOutput.Result, state)
		return sessionOutput.Result, sessionOutput.Err
	}
	// ── Dispatch ─────────────────────────────────────────────────────────────
	if prep.activeStep != nil {
		a.phaseDriver().EnterExecution(ctx, task, state, prep.activeStep)
	}
	if !prep.summaryFastPath && shouldHydratePersistedArtifacts(task, state, envelope) {
		a.hydratePersistedArtifacts(ctx, task, state)
	}
	routing := eucloruntime.RouteCapabilityFamilies(mode, profile)
	state.Set("euclo.capability_family_routing", routing)
	work.Status = eucloruntime.UnitOfWorkStatusExecuting
	state.Set("euclo.unit_of_work", work)
	euclowork.SeedCompiledExecutionState(state, work, euclowork.BuildRuntimeExecutionStatus(work, euclowork.ExecutionStatusExecuting, "", time.Now().UTC()))
	executionTask := a.eucloTask(task, envelope, classification, mode, profile, work)
	sessionOutput := a.executionSession().Execute(ctx, eucloses.SessionInput{
		Task:             task,
		ExecutionTask:    executionTask,
		WorkflowExecutor: workflowExecutor,
		State:            state,
		Classification:   classification,
		Mode:             mode,
		Profile:          profile,
		Telemetry:        a.ConfigTelemetry(),
		Work:             work,
		ServiceBundle:    a.serviceBundle(),
	})
	// ── Post-execution ───────────────────────────────────────────────────────
	result := sessionOutput.Result
	err := sessionOutput.Err
	if contextRuntime != nil {
		contextRuntime.HandleResult(state, a.Environment.Model, result)
		state.Set("euclo.shared_context_runtime", euclopolicy.BuildSharedContextRuntimeState(contextRuntime.Shared, work))
	}
	a.phaseDriver().EnterVerification(ctx, task, state, prep.activeStep, err)
	a.executionFinalizer().FinalizeLivingPlan(ctx, task, state, prep.livingPlan, prep.activeStep, result, err)
	a.phaseDriver().EnterSurfacing(ctx, task, state, prep.activeStep, err)
	if a.DeferralPlan != nil && !a.DeferralPlan.IsEmpty() {
		state.Set("euclo.deferral_plan", a.DeferralPlan)
	}
	a.phaseDriver().Complete(ctx, task, state, prep.activeStep, err)
	postContractRuntime, contractErr := eucloruntime.EvaluatePostExecutionCapabilityContracts(work, state, time.Now().UTC())
	state.Set("euclo.capability_contract_runtime", postContractRuntime)
	state.Set("euclo.archaeology_capability_runtime", eucloarchaeomem.BuildArchaeologyCapabilityRuntimeState(work, state, time.Now().UTC()))
	state.Set("euclo.debug_capability_runtime", eucloreporting.BuildDebugCapabilityRuntimeState(work, state, time.Now().UTC()))
	state.Set("euclo.chat_capability_runtime", eucloreporting.BuildChatCapabilityRuntimeState(work, state, time.Now().UTC()))
	if err == nil && contractErr != nil {
		err = contractErr
	}
	finalStatus := eucloruntime.ExecutionStatusCompleted
	if err != nil {
		finalStatus = eucloruntime.ExecutionStatusFailed
	}
	a.refreshRuntimeExecutionArtifacts(ctx, task, state, work, finalStatus, err)
	a.applyRuntimeResultMetadata(result, state)
	return result, err
}

