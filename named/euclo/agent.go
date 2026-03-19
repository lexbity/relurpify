package euclo

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	reactpkg "github.com/lexcodex/relurpify/agents/react"
	"github.com/lexcodex/relurpify/framework/agentenv"
	"github.com/lexcodex/relurpify/framework/capability"
	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/framework/graph"
	"github.com/lexcodex/relurpify/framework/memory"
	"github.com/lexcodex/relurpify/named/euclo/capabilities"
	"github.com/lexcodex/relurpify/named/euclo/euclotypes"
	"github.com/lexcodex/relurpify/named/euclo/gate"
	"github.com/lexcodex/relurpify/named/euclo/interaction"
	"github.com/lexcodex/relurpify/named/euclo/orchestrate"
	eucloruntime "github.com/lexcodex/relurpify/named/euclo/runtime"
)

// Agent is the named coding-runtime boundary. The initial implementation keeps
// the public surface narrow while delegating execution to generic agent
// machinery underneath.
type Agent struct {
	Config         *core.Config
	Delegate       *reactpkg.ReActAgent
	CheckpointPath string
	Memory         memory.MemoryStore
	Environment    agentenv.AgentEnvironment

	ModeRegistry        *euclotypes.ModeRegistry
	ProfileRegistry     *euclotypes.ExecutionProfileRegistry
	InteractionRegistry *interaction.ModeMachineRegistry
	CodingCapabilities  *capabilities.EucloCapabilityRegistry
	ProfileCtrl         *orchestrate.ProfileController
	RecoveryCtrl        *orchestrate.RecoveryController
}

func New(env agentenv.AgentEnvironment) *Agent {
	agent := &Agent{}
	_ = agent.InitializeEnvironment(env)
	return agent
}

func (a *Agent) InitializeEnvironment(env agentenv.AgentEnvironment) error {
	a.Config = env.Config
	a.Memory = env.Memory
	a.Environment = env
	if a.Delegate == nil {
		a.Delegate = reactpkg.New(env)
	} else if err := a.Delegate.InitializeEnvironment(env); err != nil {
		return err
	}
	if a.ModeRegistry == nil {
		a.ModeRegistry = euclotypes.DefaultModeRegistry()
	}
	if a.ProfileRegistry == nil {
		a.ProfileRegistry = euclotypes.DefaultExecutionProfileRegistry()
	}
	if a.InteractionRegistry == nil {
		a.InteractionRegistry = defaultInteractionRegistry()
	}
	if a.CodingCapabilities == nil {
		a.CodingCapabilities = capabilities.NewDefaultCapabilityRegistry(env)
	}

	// Wire the snapshot function for orchestrate package.
	orchestrate.SetDefaultSnapshotFunc(func(reg interface{}) euclotypes.CapabilitySnapshot {
		if registry, ok := reg.(*capability.Registry); ok {
			return eucloruntime.SnapshotCapabilities(registry)
		}
		return euclotypes.CapabilitySnapshot{}
	})

	if a.RecoveryCtrl == nil {
		a.RecoveryCtrl = orchestrate.NewRecoveryController(
			orchestrate.AdaptCapabilityRegistry(a.CodingCapabilities),
			a.ProfileRegistry,
			a.ModeRegistry,
			env,
		)
	}
	if a.ProfileCtrl == nil {
		a.ProfileCtrl = orchestrate.NewProfileController(
			orchestrate.AdaptCapabilityRegistry(a.CodingCapabilities),
			gate.DefaultPhaseGates(),
			env,
			a.ProfileRegistry,
			a.RecoveryCtrl,
		)
	}
	return a.Initialize(env.Config)
}

func (a *Agent) Initialize(cfg *core.Config) error {
	a.Config = cfg
	if a.Delegate == nil {
		a.Delegate = &reactpkg.ReActAgent{}
	}
	if a.ModeRegistry == nil {
		a.ModeRegistry = euclotypes.DefaultModeRegistry()
	}
	if a.ProfileRegistry == nil {
		a.ProfileRegistry = euclotypes.DefaultExecutionProfileRegistry()
	}
	if a.CheckpointPath != "" {
		a.Delegate.CheckpointPath = a.CheckpointPath
	}
	return a.Delegate.Initialize(cfg)
}

func (a *Agent) Capabilities() []core.Capability {
	if a == nil || a.Delegate == nil {
		return nil
	}
	return a.Delegate.Capabilities()
}

func (a *Agent) CapabilityRegistry() *capability.Registry {
	if a == nil || a.Delegate == nil {
		return nil
	}
	return a.Delegate.Tools
}

func (a *Agent) BuildGraph(task *core.Task) (*graph.Graph, error) {
	if a.Delegate == nil {
		if err := a.Initialize(a.Config); err != nil {
			return nil, err
		}
	}
	env, classification, mode, profile := a.runtimeState(task, nil)
	return a.Delegate.BuildGraph(a.eucloTask(task, env, classification, mode, profile))
}

func (a *Agent) Execute(ctx context.Context, task *core.Task, state *core.Context) (*core.Result, error) {
	if a.Delegate == nil {
		if err := a.Initialize(a.Config); err != nil {
			return nil, err
		}
	}
	if state == nil {
		state = core.NewContext()
	}
	seedPersistedInteractionState(task, state)
	// Session scoping: prevent recursive Euclo invocations.
	sessionID := generateSessionID()
	if scopeErr := enforceSessionScoping(state, sessionID); scopeErr != nil {
		return &core.Result{Success: false, Error: scopeErr}, scopeErr
	}
	envelope, classification, mode, profile := a.runtimeState(task, state)
	state.Set("euclo.envelope", envelope)
	state.Set("euclo.classification", classification)
	state.Set("euclo.mode_resolution", mode)
	state.Set("euclo.execution_profile_selection", profile)
	state.Set("euclo.mode", mode.ModeID)
	state.Set("euclo.execution_profile", profile.ProfileID)
	a.hydratePersistedArtifacts(ctx, task, state)
	var err error
	retrievalPolicy := eucloruntime.ResolveRetrievalPolicy(mode, profile)
	state.Set("euclo.retrieval_policy", retrievalPolicy)
	routing := eucloruntime.RouteCapabilityFamilies(mode, profile)
	state.Set("euclo.capability_family_routing", routing)
	executionTask := a.eucloTask(task, envelope, classification, mode, profile)
	if surfaces := eucloruntime.ResolveRuntimeSurfaces(a.Memory); surfaces.Workflow != nil {
		workflowID := state.GetString("euclo.workflow_id")
		if workflowID == "" && task != nil && task.Context != nil {
			if value, ok := task.Context["workflow_id"]; ok {
				workflowID = stringValue(value)
			}
		}
		if expansion, expandErr := eucloruntime.ExpandContext(ctx, surfaces.Workflow, workflowID, executionTask, state, retrievalPolicy); expandErr == nil {
			executionTask = eucloruntime.ApplyContextExpansion(state, executionTask, expansion)
		} else {
			err = expandErr
		}
	}
	var result *core.Result
	var execErr error
	execEnvelope := eucloruntime.BuildExecutionEnvelope(
		executionTask, state, mode, profile, a.Environment,
		nil, "", "", a.ConfigTelemetry(),
	)
	seedInteractionPrepass(state, executionTask, classification, mode)
	if a.InteractionRegistry != nil {
		emitter, withTransitions := interactionEmitterForTask(executionTask)
		var interactionErr error
		if withTransitions {
			_, _, interactionErr = a.ProfileCtrl.ExecuteInteractiveWithTransitions(
				ctx,
				a.InteractionRegistry,
				mode,
				execEnvelope,
				emitter,
				interactionMaxTransitions(executionTask),
			)
		} else {
			_, _, interactionErr = a.ProfileCtrl.ExecuteInteractive(
				ctx,
				a.InteractionRegistry,
				mode,
				execEnvelope,
				emitter,
			)
		}
		if interactionErr != nil && err == nil {
			err = interactionErr
		}
	}
	result, _, execErr = a.ProfileCtrl.ExecuteProfile(ctx, profile, mode, execEnvelope)
	if err == nil {
		err = execErr
	}
	policy := eucloruntime.ResolveVerificationPolicy(mode, profile)
	state.Set("euclo.verification_policy", policy)
	if err == nil && profile.MutationAllowed {
		if _, applyErr := eucloruntime.ApplyEditIntentArtifacts(ctx, a.CapabilityRegistry(), state); applyErr != nil {
			err = applyErr
		}
	}
	evidence := eucloruntime.NormalizeVerificationEvidence(state)
	state.Set("euclo.verification", evidence)
	var editRecord *eucloruntime.EditExecutionRecord
	if raw, ok := state.Get("euclo.edit_execution"); ok && raw != nil {
		if typed, ok := raw.(eucloruntime.EditExecutionRecord); ok {
			editRecord = &typed
		}
	}
	successGate := eucloruntime.EvaluateSuccessGate(policy, evidence, editRecord)
	state.Set("euclo.success_gate", successGate)
	if result != nil {
		if result.Data == nil {
			result.Data = map[string]any{}
		}
		result.Data["verification"] = evidence
		result.Data["success_gate"] = successGate
	}
	if err == nil && !successGate.Allowed {
		err = fmt.Errorf("euclo success gate blocked completion: %s", successGate.Reason)
	}
	if result != nil {
		result.Success = err == nil && successGate.Allowed && result.Success
		if err != nil {
			result.Error = err
		}
	}
	artifacts := euclotypes.CollectArtifactsFromState(state)
	actionLog := eucloruntime.BuildActionLog(state, artifacts)
	state.Set("euclo.action_log", actionLog)
	proofSurface := eucloruntime.BuildProofSurface(state, artifacts)
	state.Set("euclo.proof_surface", proofSurface)
	artifacts = euclotypes.CollectArtifactsFromState(state)
	state.Set("euclo.artifacts", artifacts)
	if persistErr := a.persistArtifacts(ctx, task, state, artifacts); persistErr != nil && err == nil {
		err = persistErr
		if result != nil {
			result.Success = false
			result.Error = err
		}
	}
	finalReport := euclotypes.AssembleFinalReport(artifacts)
	state.Set("euclo.final_report", finalReport)
	eucloruntime.EmitObservabilityTelemetry(a.ConfigTelemetry(), task, actionLog, proofSurface)
	if result != nil {
		if result.Data == nil {
			result.Data = map[string]any{}
		}
		result.Data["final_report"] = finalReport
		result.Data["action_log"] = actionLog
		result.Data["proof_surface"] = proofSurface
	}
	return result, err
}

func seedPersistedInteractionState(task *core.Task, state *core.Context) {
	if task == nil || task.Context == nil || state == nil {
		return
	}
	if _, ok := state.Get("euclo.interaction_state"); !ok {
		if raw, ok := task.Context["euclo.interaction_state"]; ok && raw != nil {
			state.Set("euclo.interaction_state", raw)
		}
	}
}

func (a *Agent) runtimeState(task *core.Task, state *core.Context) (eucloruntime.TaskEnvelope, eucloruntime.TaskClassification, euclotypes.ModeResolution, euclotypes.ExecutionProfileSelection) {
	envelope := eucloruntime.NormalizeTaskEnvelope(task, state, a.CapabilityRegistry())
	classification := eucloruntime.ClassifyTask(envelope)
	mode := eucloruntime.ResolveMode(envelope, classification, a.ModeRegistry)
	profile := eucloruntime.SelectExecutionProfile(envelope, classification, mode, a.ProfileRegistry)
	envelope.ResolvedMode = mode.ModeID
	envelope.ExecutionProfile = profile.ProfileID
	return envelope, classification, mode, profile
}

func (a *Agent) eucloTask(task *core.Task, envelope eucloruntime.TaskEnvelope, classification eucloruntime.TaskClassification, mode euclotypes.ModeResolution, profile euclotypes.ExecutionProfileSelection) *core.Task {
	cloned := core.CloneTask(task)
	if cloned == nil {
		cloned = &core.Task{}
	}
	if cloned.Context == nil {
		cloned.Context = map[string]any{}
	}
	cloned.Context["mode"] = mode.ModeID
	cloned.Context["euclo.mode"] = mode.ModeID
	cloned.Context["euclo.execution_profile"] = profile.ProfileID
	cloned.Context["euclo.envelope"] = envelope
	cloned.Context["euclo.classification"] = eucloruntime.ClassificationContextPayload(classification)
	return cloned
}

func seedInteractionPrepass(state *core.Context, task *core.Task, classification eucloruntime.TaskClassification, mode euclotypes.ModeResolution) {
	if state == nil {
		return
	}
	instruction := ""
	if task != nil {
		instruction = strings.ToLower(strings.TrimSpace(task.Instruction))
	}
	state.Set("requires_evidence_before_mutation", classification.RequiresEvidenceBeforeMutation)
	switch mode.ModeID {
	case "debug":
		if hasInstructionEvidence(instruction, classification.ReasonCodes) {
			state.Set("has_evidence", true)
			state.Set("evidence_in_instruction", true)
		}
	case "code":
		if strings.Contains(instruction, "just do it") {
			state.Set("just_do_it", true)
		}
	case "planning":
		if strings.Contains(instruction, "just plan it") || strings.Contains(instruction, "skip to plan") {
			state.Set("just_plan_it", true)
		}
	}
}

func hasInstructionEvidence(instruction string, reasonCodes []string) bool {
	for _, reason := range reasonCodes {
		if strings.HasPrefix(reason, "error_text:") {
			return true
		}
	}
	for _, token := range []string{"panic:", "stacktrace", "stack trace", "goroutine ", ".go:", "failing test", "runtime error"} {
		if strings.Contains(instruction, token) {
			return true
		}
	}
	return false
}

func interactionEmitterForTask(task *core.Task) (interaction.FrameEmitter, bool) {
	script := interactionScriptFromTask(task)
	if len(script) == 0 {
		return &interaction.NoopEmitter{}, false
	}
	return interaction.NewTestFrameEmitter(script...), true
}

func interactionScriptFromTask(task *core.Task) []interaction.ScriptedResponse {
	if task == nil || task.Context == nil {
		return nil
	}
	raw, ok := task.Context["euclo.interaction_script"]
	if !ok || raw == nil {
		return nil
	}
	rows, ok := raw.([]any)
	if !ok {
		if typed, ok := raw.([]map[string]any); ok {
			rows = make([]any, 0, len(typed))
			for _, item := range typed {
				rows = append(rows, item)
			}
		} else {
			return nil
		}
	}
	script := make([]interaction.ScriptedResponse, 0, len(rows))
	for _, row := range rows {
		entry, ok := row.(map[string]any)
		if !ok {
			continue
		}
		action := stringValue(entry["action"])
		if action == "" {
			continue
		}
		script = append(script, interaction.ScriptedResponse{
			Phase:    stringValue(entry["phase"]),
			Kind:     stringValue(entry["kind"]),
			ActionID: action,
			Text:     stringValue(entry["text"]),
		})
	}
	return script
}

func interactionMaxTransitions(task *core.Task) int {
	if task == nil || task.Context == nil {
		return 5
	}
	raw, ok := task.Context["euclo.max_interactive_transitions"]
	if !ok || raw == nil {
		return 5
	}
	switch typed := raw.(type) {
	case int:
		if typed > 0 {
			return typed
		}
	case int64:
		if typed > 0 {
			return int(typed)
		}
	case float64:
		if typed > 0 {
			return int(typed)
		}
	case string:
		if parsed, err := strconv.Atoi(strings.TrimSpace(typed)); err == nil && parsed > 0 {
			return parsed
		}
	}
	return 5
}

func (a *Agent) hydratePersistedArtifacts(ctx context.Context, task *core.Task, state *core.Context) {
	if state == nil {
		return
	}
	if raw, ok := state.Get("euclo.artifacts"); ok && raw != nil {
		return
	}
	surfaces := eucloruntime.ResolveRuntimeSurfaces(a.Memory)
	if surfaces.Workflow == nil {
		return
	}
	workflowID := state.GetString("euclo.workflow_id")
	if workflowID == "" && task != nil && task.Context != nil {
		if value, ok := task.Context["workflow_id"]; ok {
			workflowID = stringValue(value)
		}
	}
	if workflowID == "" {
		return
	}
	runID := state.GetString("euclo.run_id")
	if runID == "" && task != nil && task.Context != nil {
		if value, ok := task.Context["run_id"]; ok {
			runID = stringValue(value)
		}
	}
	artifacts, err := euclotypes.LoadPersistedArtifacts(ctx, surfaces.Workflow, workflowID, runID)
	if err != nil || len(artifacts) == 0 {
		return
	}
	state.Set("euclo.artifacts", artifacts)
	euclotypes.RestoreStateFromArtifacts(state, artifacts)
}

func (a *Agent) persistArtifacts(ctx context.Context, task *core.Task, state *core.Context, artifacts []euclotypes.Artifact) error {
	surfaces := eucloruntime.ResolveRuntimeSurfaces(a.Memory)
	if surfaces.Workflow == nil || len(artifacts) == 0 {
		return nil
	}
	workflowID, runID, err := eucloruntime.EnsureWorkflowRun(ctx, surfaces.Workflow, task, state)
	if err != nil {
		return err
	}
	if workflowID == "" {
		return nil
	}
	return euclotypes.PersistWorkflowArtifacts(ctx, surfaces.Workflow, workflowID, runID, artifacts)
}

func (a *Agent) ConfigTelemetry() core.Telemetry {
	if a == nil || a.Config == nil {
		return nil
	}
	return a.Config.Telemetry
}

// stringValue extracts a string from an interface value.
func stringValue(raw any) string {
	if raw == nil {
		return ""
	}
	if s, ok := raw.(string); ok {
		return s
	}
	return ""
}
