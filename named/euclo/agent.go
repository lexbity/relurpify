package euclo

import (
	"context"
	"fmt"

	plannerpkg "github.com/lexcodex/relurpify/agents/planner"
	reactpkg "github.com/lexcodex/relurpify/agents/react"
	reflectionpkg "github.com/lexcodex/relurpify/agents/reflection"
	"github.com/lexcodex/relurpify/framework/agentenv"
	"github.com/lexcodex/relurpify/framework/capability"
	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/framework/graph"
	"github.com/lexcodex/relurpify/framework/memory"
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

	ModeRegistry    *ModeRegistry
	ProfileRegistry *ExecutionProfileRegistry
	FamilyRegistry  *CapabilityFamilyRegistry
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
		a.ModeRegistry = DefaultModeRegistry()
	}
	if a.ProfileRegistry == nil {
		a.ProfileRegistry = DefaultExecutionProfileRegistry()
	}
	if a.FamilyRegistry == nil {
		a.FamilyRegistry = DefaultCapabilityFamilyRegistry()
	}
	return a.Initialize(env.Config)
}

func (a *Agent) Initialize(cfg *core.Config) error {
	a.Config = cfg
	if a.Delegate == nil {
		a.Delegate = &reactpkg.ReActAgent{}
	}
	if a.ModeRegistry == nil {
		a.ModeRegistry = DefaultModeRegistry()
	}
	if a.ProfileRegistry == nil {
		a.ProfileRegistry = DefaultExecutionProfileRegistry()
	}
	if a.FamilyRegistry == nil {
		a.FamilyRegistry = DefaultCapabilityFamilyRegistry()
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
	envelope, classification, mode, profile := a.runtimeState(task, state)
	state.Set("euclo.envelope", envelope)
	state.Set("euclo.classification", classification)
	state.Set("euclo.mode_resolution", mode)
	state.Set("euclo.execution_profile_selection", profile)
	state.Set("euclo.mode", mode.ModeID)
	state.Set("euclo.execution_profile", profile.ProfileID)
	a.hydratePersistedArtifacts(ctx, task, state)
	var err error
	retrievalPolicy := ResolveRetrievalPolicy(mode, profile)
	state.Set("euclo.retrieval_policy", retrievalPolicy)
	routing := RouteCapabilityFamilies(mode, profile, a.FamilyRegistry)
	state.Set("euclo.capability_family_routing", routing)
	executionTask := a.eucloTask(task, envelope, classification, mode, profile)
	if surfaces := resolveRuntimeSurfaces(a.Memory); surfaces.Workflow != nil {
		workflowID := state.GetString("euclo.workflow_id")
		if workflowID == "" && task != nil && task.Context != nil {
			if value, ok := task.Context["workflow_id"]; ok {
				workflowID = stringValue(value)
			}
		}
		if expansion, expandErr := ExpandContext(ctx, surfaces.Workflow, workflowID, executionTask, state, retrievalPolicy); expandErr == nil {
			executionTask = applyContextExpansion(state, executionTask, expansion)
		} else if err == nil {
			err = expandErr
		}
	}
	executor, buildErr := a.buildExecutorForRouting(routing)
	if buildErr != nil && err == nil {
		err = buildErr
	}
	result, execErr := executor.Execute(ctx, executionTask, state)
	if err == nil {
		err = execErr
	}
	policy := ResolveVerificationPolicy(mode, profile)
	state.Set("euclo.verification_policy", policy)
	if err == nil && profile.MutationAllowed {
		if _, applyErr := ApplyEditIntentArtifacts(ctx, a.CapabilityRegistry(), state); applyErr != nil {
			err = applyErr
		}
	}
	evidence := NormalizeVerificationEvidence(state)
	state.Set("euclo.verification", evidence)
	var editRecord *EditExecutionRecord
	if raw, ok := state.Get("euclo.edit_execution"); ok && raw != nil {
		if typed, ok := raw.(EditExecutionRecord); ok {
			editRecord = &typed
		}
	}
	successGate := EvaluateSuccessGate(policy, evidence, editRecord)
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
	artifacts := CollectArtifactsFromState(state)
	actionLog := BuildActionLog(state, artifacts)
	state.Set("euclo.action_log", actionLog)
	proofSurface := BuildProofSurface(state, artifacts)
	state.Set("euclo.proof_surface", proofSurface)
	artifacts = CollectArtifactsFromState(state)
	state.Set("euclo.artifacts", artifacts)
	if persistErr := a.persistArtifacts(ctx, task, state, artifacts); persistErr != nil && err == nil {
		err = persistErr
		if result != nil {
			result.Success = false
			result.Error = err
		}
	}
	finalReport := AssembleFinalReport(artifacts)
	state.Set("euclo.final_report", finalReport)
	EmitObservabilityTelemetry(a.ConfigTelemetry(), task, actionLog, proofSurface)
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

func (a *Agent) runtimeState(task *core.Task, state *core.Context) (TaskEnvelope, TaskClassification, ModeResolution, ExecutionProfileSelection) {
	envelope := NormalizeTaskEnvelope(task, state, a.CapabilityRegistry())
	classification := ClassifyTask(envelope)
	mode := ResolveMode(envelope, classification, a.ModeRegistry)
	profile := SelectExecutionProfile(envelope, classification, mode, a.ProfileRegistry)
	envelope.ResolvedMode = mode.ModeID
	envelope.ExecutionProfile = profile.ProfileID
	return envelope, classification, mode, profile
}

func (a *Agent) eucloTask(task *core.Task, envelope TaskEnvelope, classification TaskClassification, mode ModeResolution, profile ExecutionProfileSelection) *core.Task {
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
	cloned.Context["euclo.classification"] = classificationContextPayload(classification)
	return cloned
}

func (a *Agent) buildExecutorForRouting(routing CapabilityFamilyRouting) (graph.Agent, error) {
	env := a.Environment
	switch routing.PrimaryFamilyID {
	case "planning":
		agent := plannerpkg.New(env)
		return agent, nil
	case "review":
		agent := reflectionpkg.New(env, reactpkg.New(env))
		return agent, nil
	case "implementation", "debugging", "verification":
		agent := reactpkg.New(env)
		agent.Mode = routing.ModeID
		if a.CheckpointPath != "" {
			agent.CheckpointPath = a.CheckpointPath
		}
		return agent, nil
	default:
		if a.Delegate != nil {
			a.Delegate.Mode = routing.ModeID
			return a.Delegate, nil
		}
		return reactpkg.New(env), nil
	}
}

func (a *Agent) hydratePersistedArtifacts(ctx context.Context, task *core.Task, state *core.Context) {
	if state == nil {
		return
	}
	if raw, ok := state.Get("euclo.artifacts"); ok && raw != nil {
		return
	}
	surfaces := resolveRuntimeSurfaces(a.Memory)
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
	artifacts, err := LoadPersistedArtifacts(ctx, surfaces.Workflow, workflowID, runID)
	if err != nil || len(artifacts) == 0 {
		return
	}
	state.Set("euclo.artifacts", artifacts)
	RestoreStateFromArtifacts(state, artifacts)
}

func (a *Agent) persistArtifacts(ctx context.Context, task *core.Task, state *core.Context, artifacts []Artifact) error {
	surfaces := resolveRuntimeSurfaces(a.Memory)
	if surfaces.Workflow == nil || len(artifacts) == 0 {
		return nil
	}
	workflowID, runID, err := ensureWorkflowRun(ctx, surfaces.Workflow, task, state)
	if err != nil {
		return err
	}
	if workflowID == "" {
		return nil
	}
	return PersistWorkflowArtifacts(ctx, surfaces.Workflow, workflowID, runID, artifacts)
}

func (a *Agent) ConfigTelemetry() core.Telemetry {
	if a == nil || a.Config == nil {
		return nil
	}
	return a.Config.Telemetry
}
