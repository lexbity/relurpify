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
	euclopolicy "github.com/lexcodex/relurpify/named/euclo/runtime/policy"
	eucloreporting "github.com/lexcodex/relurpify/named/euclo/runtime/reporting"
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
	contextRuntime   *euclocontext.ContextRuntime
	prep             executionPreparation
}

func (a *Agent) initializeManagedExecution(ctx context.Context, task *core.Task, state *core.Context, workflowExecutor graph.WorkflowExecutor) (*managedExecutionFlow, *core.Result, error) {
	if state == nil {
		state = core.NewContext()
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
		return nil, result, restoreErr
	}

	envelope, classification, mode, profile, work = a.runtimeState(task, state)
	a.seedRuntimeState(state, envelope, classification, mode, profile, work)
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
		return nil, result, contractErr
	}

	return &managedExecutionFlow{
		task:             task,
		state:            state,
		workflowExecutor: workflowExecutor,
		envelope:         envelope,
		classification:   classification,
		mode:             mode,
		profile:          profile,
		work:             work,
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
	a.seedRuntimeState(flow.state, flow.envelope, flow.classification, flow.mode, flow.profile, flow.work)

	if prep.activeStep == nil && shouldShortCircuitExecution(prep, flow.state) {
		flow.work.Status = eucloruntime.UnitOfWorkStatusCompleted
		flow.work.ResultClass = euclowork.ResultClassForOutcome(euclowork.ExecutionStatusCompleted, flow.work.DeferredIssueIDs, nil)
		flow.state.Set("euclo.unit_of_work", flow.work)
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
			flow.state.Set("euclo.deferral_plan", a.DeferralPlan)
		}
		if flow.contextRuntime != nil {
			flow.contextRuntime.HandleResult(flow.state, a.Environment.Model, sessionOutput.Result)
			flow.state.Set("euclo.shared_context_runtime", euclopolicy.BuildSharedContextRuntimeState(flow.contextRuntime.Shared, flow.work))
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
	routing := eucloruntime.RouteCapabilityFamilies(flow.mode, flow.profile)
	flow.state.Set("euclo.capability_family_routing", routing)
	flow.work.Status = eucloruntime.UnitOfWorkStatusExecuting
	flow.state.Set("euclo.unit_of_work", flow.work)
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
		flow.state.Set("euclo.shared_context_runtime", euclopolicy.BuildSharedContextRuntimeState(flow.contextRuntime.Shared, flow.work))
	}
	a.phaseDriver().EnterVerification(ctx, flow.task, flow.state, prep.activeStep, err)
	a.executionFinalizer().FinalizeLivingPlan(ctx, flow.task, flow.state, prep.livingPlan, prep.activeStep, result, err)
	a.phaseDriver().EnterSurfacing(ctx, flow.task, flow.state, prep.activeStep, err)
	if a.DeferralPlan != nil && !a.DeferralPlan.IsEmpty() {
		flow.state.Set("euclo.deferral_plan", a.DeferralPlan)
	}
	a.phaseDriver().Complete(ctx, flow.task, flow.state, prep.activeStep, err)
	postContractRuntime, contractErr := eucloruntime.EvaluatePostExecutionCapabilityContracts(flow.work, flow.state, time.Now().UTC())
	flow.state.Set("euclo.capability_contract_runtime", postContractRuntime)
	flow.state.Set("euclo.archaeology_capability_runtime", eucloarchaeomem.BuildArchaeologyCapabilityRuntimeState(flow.work, flow.state, time.Now().UTC()))
	flow.state.Set("euclo.debug_capability_runtime", eucloreporting.BuildDebugCapabilityRuntimeState(flow.work, flow.state, time.Now().UTC()))
	flow.state.Set("euclo.chat_capability_runtime", eucloreporting.BuildChatCapabilityRuntimeState(flow.work, flow.state, time.Now().UTC()))
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
