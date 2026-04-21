package execution

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"codeburg.org/lexbit/relurpify/framework/agentenv"
	capabilitypkg "codeburg.org/lexbit/relurpify/framework/capability"
	"codeburg.org/lexbit/relurpify/framework/core"
	"codeburg.org/lexbit/relurpify/framework/graph"
	"codeburg.org/lexbit/relurpify/named/euclo/euclotypes"
	architectexec "codeburg.org/lexbit/relurpify/named/euclo/execution/architect"
	htnexec "codeburg.org/lexbit/relurpify/named/euclo/execution/htn"
	plannerexec "codeburg.org/lexbit/relurpify/named/euclo/execution/planner"
	reactexec "codeburg.org/lexbit/relurpify/named/euclo/execution/react"
	reflectionexec "codeburg.org/lexbit/relurpify/named/euclo/execution/reflection"
	rewoo "codeburg.org/lexbit/relurpify/named/euclo/execution/rewoo"
	eucloruntime "codeburg.org/lexbit/relurpify/named/euclo/runtime"
	euclostate "codeburg.org/lexbit/relurpify/named/euclo/runtime/state"
	"codeburg.org/lexbit/relurpify/named/euclo/runtime/statebus"
)

type ExecuteInput struct {
	Task             *core.Task
	ExecutionTask    *core.Task
	State            *core.Context
	Mode             euclotypes.ModeResolution
	Profile          euclotypes.ExecutionProfileSelection
	Work             eucloruntime.UnitOfWork
	Environment      agentenv.AgentEnvironment
	ServiceBundle    ServiceBundle
	WorkflowExecutor graph.WorkflowExecutor
	Telemetry        core.Telemetry
	InvokeSupporting func(context.Context, string, InvokeInput) ([]euclotypes.Artifact, error)
}

type Trace struct {
	PrimaryCapabilityID      string   `json:"primary_capability_id,omitempty"`
	SupportingRoutines       []string `json:"supporting_routines,omitempty"`
	RecipeIDs                []string `json:"recipe_ids,omitempty"`
	SpecializedCapabilityIDs []string `json:"specialized_capability_ids,omitempty"`
	ExecutorFamily           string   `json:"executor_family,omitempty"`
	Path                     string   `json:"path,omitempty"`
}

type RecipeID string

type RecipeRunner func(context.Context, ExecuteInput, RecipeSpec, string, string) (*core.Result, *core.Context, error)

type RecipeSpec struct {
	TaskType core.TaskType
	Run      RecipeRunner
}

const (
	RecipeChatAskInquiry                  RecipeID = "chat.ask.inquiry"
	RecipeChatAskOptions                  RecipeID = "chat.ask.options"
	RecipeChatAskReview                   RecipeID = "chat.ask.review"
	RecipeChatInspectCollect              RecipeID = "chat.inspect.collect"
	RecipeChatInspectReview               RecipeID = "chat.inspect.review"
	RecipeChatImplementArchitect          RecipeID = "chat.implement.architect"
	RecipeChatImplementExplore            RecipeID = "chat.implement.explore"
	RecipeChatImplementEdit               RecipeID = "chat.implement.edit"
	RecipeChatImplementVerify             RecipeID = "chat.implement.verify"
	RecipeDebugInvestigateRepairReproduce RecipeID = "debug.investigate-repair.reproduce"
	RecipeDebugInvestigateRepairLocalize  RecipeID = "debug.investigate-repair.localize"
	RecipeDebugInvestigateRepairPatch     RecipeID = "debug.investigate-repair.patch"
	RecipeDebugInvestigateRepairReview    RecipeID = "debug.investigate-repair.review"
	RecipeArchaeologyExploreShape         RecipeID = "archaeology.explore.shape"
	RecipeArchaeologyExploreReview        RecipeID = "archaeology.explore.review"
	RecipeArchaeologyCompileReconcile     RecipeID = "archaeology.compile.reconcile"
	RecipeArchaeologyCompileShape         RecipeID = "archaeology.compile.shape"
	RecipeArchaeologyCompileReview        RecipeID = "archaeology.compile.review"
	RecipeArchaeologyImplementStep        RecipeID = "archaeology.implement.step"
	RecipeArchaeologyImplementCheckpoint  RecipeID = "archaeology.implement.checkpoint"
	RecipeArchaeologyImplementGapDetect   RecipeID = "archaeology.implement.gap-detect"
	RecipeDebugRepairSimpleRead           RecipeID = "debug.repair-simple.read"
	RecipeDebugRepairSimpleEdit           RecipeID = "debug.repair-simple.edit"
	RecipeDebugRepairSimpleVerify         RecipeID = "debug.repair-simple.verify"
)

var recipeSpecs = map[RecipeID]RecipeSpec{
	RecipeChatAskInquiry:                  reactRecipe(core.TaskTypeAnalysis),
	RecipeChatAskOptions:                  plannerRecipe(),
	RecipeChatAskReview:                   reflectionRecipe(),
	RecipeChatInspectCollect:              reactRecipe(core.TaskTypeAnalysis),
	RecipeChatInspectReview:               reflectionRecipe(),
	RecipeChatImplementArchitect:          architectRecipe(core.TaskTypeCodeModification),
	RecipeChatImplementExplore:            reactRecipe(core.TaskTypeAnalysis),
	RecipeChatImplementEdit:               reactRecipe(core.TaskTypeCodeModification),
	RecipeChatImplementVerify:             reactRecipe(core.TaskTypeAnalysis),
	RecipeDebugInvestigateRepairReproduce: reactRecipe(core.TaskTypeAnalysis),
	RecipeDebugInvestigateRepairLocalize:  blackboardRecipe(core.TaskTypeAnalysis),
	RecipeDebugInvestigateRepairPatch:     reactRecipe(core.TaskTypeCodeModification),
	RecipeDebugInvestigateRepairReview:    reflectionRecipe(),
	RecipeArchaeologyExploreShape:         plannerRecipe(),
	RecipeArchaeologyExploreReview:        reflectionRecipe(),
	RecipeArchaeologyCompileReconcile:     plannerRecipe(),
	RecipeArchaeologyCompileShape:         plannerRecipe(),
	RecipeArchaeologyCompileReview:        reflectionRecipe(),
	RecipeArchaeologyImplementStep:        rewooRecipe(core.TaskTypeCodeModification),
	RecipeArchaeologyImplementCheckpoint:  reflectionRecipe(),
	RecipeArchaeologyImplementGapDetect:   reflectionRecipe(),
	RecipeDebugRepairSimpleRead:           reactRecipe(core.TaskTypeAnalysis),
	RecipeDebugRepairSimpleEdit:           reactRecipe(core.TaskTypeCodeModification),
	RecipeDebugRepairSimpleVerify:         reactRecipe(core.TaskTypeAnalysis),
}

func reactRecipe(taskType core.TaskType) RecipeSpec {
	return RecipeSpec{
		TaskType: taskType,
		Run: func(ctx context.Context, in ExecuteInput, spec RecipeSpec, taskID, instruction string) (*core.Result, *core.Context, error) {
			return ExecuteReactTask(ctx, in, taskID, instruction, spec.TaskType)
		},
	}
}

func htnRecipe(taskType core.TaskType) RecipeSpec {
	return RecipeSpec{
		TaskType: taskType,
		Run: func(ctx context.Context, in ExecuteInput, spec RecipeSpec, taskID, instruction string) (*core.Result, *core.Context, error) {
			return ExecuteHTNTask(ctx, in, taskID, instruction, spec.TaskType)
		},
	}
}

func blackboardRecipe(taskType core.TaskType) RecipeSpec {
	return RecipeSpec{
		TaskType: taskType,
		Run: func(ctx context.Context, in ExecuteInput, spec RecipeSpec, taskID, instruction string) (*core.Result, *core.Context, error) {
			return ExecuteBlackboardTask(ctx, in, taskID, instruction, spec.TaskType)
		},
	}
}

func plannerRecipe() RecipeSpec {
	return RecipeSpec{
		TaskType: core.TaskTypeAnalysis,
		Run: func(ctx context.Context, in ExecuteInput, _ RecipeSpec, taskID, instruction string) (*core.Result, *core.Context, error) {
			return ExecutePlannerTask(ctx, in, taskID, instruction)
		},
	}
}

func reflectionRecipe() RecipeSpec {
	return RecipeSpec{
		TaskType: core.TaskTypeAnalysis,
		Run: func(ctx context.Context, in ExecuteInput, _ RecipeSpec, taskID, instruction string) (*core.Result, *core.Context, error) {
			return ExecuteReflectionTask(ctx, in, taskID, instruction)
		},
	}
}

func architectRecipe(taskType core.TaskType) RecipeSpec {
	return RecipeSpec{
		TaskType: taskType,
		Run: func(ctx context.Context, in ExecuteInput, spec RecipeSpec, taskID, instruction string) (*core.Result, *core.Context, error) {
			return ExecuteArchitectTask(ctx, in, taskID, instruction, spec.TaskType)
		},
	}
}

func rewooRecipe(taskType core.TaskType) RecipeSpec {
	return RecipeSpec{
		TaskType: taskType,
		Run: func(ctx context.Context, in ExecuteInput, spec RecipeSpec, taskID, instruction string) (*core.Result, *core.Context, error) {
			return ExecuteReWOOTask(ctx, in, taskID, instruction, spec.TaskType)
		},
	}
}

func SetBehaviorTrace(state *core.Context, work eucloruntime.UnitOfWork, routines []string) {
	setBehaviorTrace(state, work, routines, true)
}

// SetBehaviorTraceWithoutRecipeID records a behavior trace without attaching the
// executor descriptor's recipe ID. This is used for specialized paths where the
// trace should reflect executed routines only.
func SetBehaviorTraceWithoutRecipeID(state *core.Context, work eucloruntime.UnitOfWork, routines []string) {
	setBehaviorTrace(state, work, routines, false)
}

func setBehaviorTrace(state *core.Context, work eucloruntime.UnitOfWork, routines []string, includeRecipeID bool) {
	if state == nil {
		return
	}
	trace := readBehaviorTrace(state)
	trace.PrimaryCapabilityID = work.PrimaryRelurpicCapabilityID
	trace.SupportingRoutines = append([]string(nil), routines...)
	trace.ExecutorFamily = string(work.ExecutorDescriptor.Family)
	trace.Path = "unit_of_work_behavior"
	if includeRecipeID && strings.TrimSpace(work.ExecutorDescriptor.RecipeID) != "" {
		trace.RecipeIDs = UniqueStrings(append(trace.RecipeIDs, strings.TrimSpace(work.ExecutorDescriptor.RecipeID)))
	}
	storeBehaviorTrace(state, trace)
}

func AddSpecializedCapabilityTrace(state *core.Context, capabilityID string) {
	if state == nil || strings.TrimSpace(capabilityID) == "" {
		return
	}
	trace := readBehaviorTrace(state)
	trace.SpecializedCapabilityIDs = UniqueStrings(append(trace.SpecializedCapabilityIDs, strings.TrimSpace(capabilityID)))
	storeBehaviorTrace(state, trace)
}

func readBehaviorTrace(state *core.Context) euclostate.Trace {
	if state == nil {
		return euclostate.Trace{}
	}
	if trace, ok := euclostate.GetBehaviorTrace(state); ok {
		return trace
	}
	return euclostate.Trace{}
}

func storeBehaviorTrace(state *core.Context, trace euclostate.Trace) {
	if state == nil {
		return
	}
	statebus.SetAny(state, euclostate.KeyBehaviorTrace, Trace{
		PrimaryCapabilityID:      trace.PrimaryCapabilityID,
		SupportingRoutines:       append([]string(nil), trace.SupportingRoutines...),
		RecipeIDs:                append([]string(nil), trace.RecipeIDs...),
		SpecializedCapabilityIDs: append([]string(nil), trace.SpecializedCapabilityIDs...),
		ExecutorFamily:           trace.ExecutorFamily,
		Path:                     trace.Path,
	})
}

func ExecuteWorkflow(ctx context.Context, in ExecuteInput) (*core.Result, error) {
	if in.WorkflowExecutor == nil {
		err := fmt.Errorf("workflow executor unavailable")
		return &core.Result{Success: false, Error: err}, err
	}
	task := in.ExecutionTask
	if task == nil {
		task = in.Task
	}
	if task == nil {
		task = &core.Task{}
	}
	if in.State == nil {
		in.State = core.NewContext()
	}
	return in.WorkflowExecutor.Execute(ctx, task, in.State)
}

func SupportingIDs(work eucloruntime.UnitOfWork, prefix string) []string {
	out := make([]string, 0, len(work.SupportingRelurpicCapabilityIDs))
	for _, id := range work.SupportingRelurpicCapabilityIDs {
		if strings.HasPrefix(strings.TrimSpace(id), prefix) {
			out = append(out, id)
		}
	}
	return out
}

func AppendDiagnostic(state *core.Context, key, message string) {
	if state == nil || strings.TrimSpace(key) == "" || strings.TrimSpace(message) == "" {
		return
	}
	raw, _ := statebus.GetAny(state, key)
	payload, _ := raw.(map[string]any)
	if payload == nil {
		payload = map[string]any{}
	}
	diags := []string{}
	switch typed := payload["diagnostics"].(type) {
	case []string:
		diags = append(diags, typed...)
	case []any:
		for _, item := range typed {
			if text, ok := item.(string); ok {
				diags = append(diags, text)
			}
		}
	}
	diags = append(diags, message)
	payload["diagnostics"] = UniqueStrings(diags)
	statebus.SetAny(state, key, payload)
}

func EnsureRoutineArtifacts(state *core.Context, routineID string, work eucloruntime.UnitOfWork) {
	if state == nil {
		return
	}
	switch routineID {
	case "euclo:chat.local-review":
		if !statebus.Has(state, euclostate.KeyReviewFindings) {
			statebus.SetAny(state, euclostate.KeyReviewFindings, map[string]any{
				"review_source":         routineID,
				"primary_capability_id": work.PrimaryRelurpicCapabilityID,
				"summary":               "local review routine prepared inspection context",
				"findings":              []map[string]any{},
			})
		}
	case "euclo:chat.targeted-verification-repair":
		if !statebus.Has(state, euclostate.KeyVerificationSummary) {
			statebus.SetAny(state, euclostate.KeyVerificationSummary, map[string]any{
				"source":     routineID,
				"summary":    "targeted verification repair routine activated",
				"provenance": "absent",
			})
		}
	case "euclo:debug.root-cause":
		if !statebus.Has(state, euclostate.KeyRootCauseCandidates) {
			statebus.SetAny(state, euclostate.KeyRootCauseCandidates, map[string]any{
				"source":       routineID,
				"tension_refs": append([]string(nil), work.SemanticInputs.TensionRefs...),
				"summary":      "root cause candidates derived from debug investigation context",
			})
		}
	case "euclo:debug.localization":
		if !statebus.Has(state, euclostate.KeyRootCause) {
			statebus.SetAny(state, euclostate.KeyRootCause, map[string]any{
				"source":          routineID,
				"pattern_refs":    append([]string(nil), work.SemanticInputs.PatternRefs...),
				"touched_symbols": work.SemanticInputs.RequestProvenanceRefs,
				"summary":         "localization routine prepared candidate fault scope",
			})
		}
	case "euclo:debug.verification-repair":
		if !statebus.Has(state, euclostate.KeyRegressionAnalysis) {
			statebus.SetAny(state, euclostate.KeyRegressionAnalysis, map[string]any{
				"source":  routineID,
				"summary": "verification repair routine activated for bounded debug repair",
			})
		}
	case "euclo:archaeology.pattern-surface":
		if !statebus.Has(state, euclostate.KeyPipelineExplore) {
			statebus.SetAny(state, euclostate.KeyPipelineExplore, map[string]any{
				"source":       routineID,
				"pattern_refs": append([]string(nil), work.SemanticInputs.PatternRefs...),
				"summary":      "pattern surface routine grounded archaeology execution",
			})
		}
	case "euclo:archaeology.prospective-assess", "euclo:archaeology.convergence-guard":
		raw, _ := statebus.GetAny(state, euclostate.KeyPlanCandidates)
		payload, _ := raw.(map[string]any)
		if payload == nil {
			payload = map[string]any{"source": "euclo.relurpic.archaeology"}
		}
		ops := []string{}
		switch typed := payload["operations"].(type) {
		case []string:
			ops = append(ops, typed...)
		case []any:
			for _, item := range typed {
				if text, ok := item.(string); ok {
					ops = append(ops, text)
				}
			}
		}
		payload["operations"] = UniqueStrings(append(ops, routineID))
		statebus.SetAny(state, euclostate.KeyPlanCandidates, payload)
	}
}

func ExecuteSupportingRoutines(ctx context.Context, in ExecuteInput, routineIDs []string) ([]euclotypes.Artifact, []string, error) {
	if len(routineIDs) == 0 {
		return nil, nil, nil
	}
	var artifacts []euclotypes.Artifact
	var executed []string
	for _, routineID := range routineIDs {
		routineID = strings.TrimSpace(routineID)
		if routineID == "" {
			continue
		}
		if in.InvokeSupporting == nil {
			EnsureRoutineArtifacts(in.State, routineID, in.Work)
			executed = append(executed, routineID)
			continue
		}
		routineArtifacts, err := in.InvokeSupporting(ctx, routineID, InvokeInput{
			Task:             in.Task,
			ExecutionTask:    in.ExecutionTask,
			State:            in.State,
			Mode:             in.Mode,
			Profile:          in.Profile,
			Work:             in.Work,
			Environment:      in.Environment,
			ServiceBundle:    in.ServiceBundle,
			WorkflowExecutor: in.WorkflowExecutor,
			Telemetry:        in.Telemetry,
		})
		if err != nil {
			return artifacts, executed, err
		}
		if len(routineArtifacts) == 0 {
			EnsureRoutineArtifacts(in.State, routineID, in.Work)
		} else {
			MergeStateArtifactsToContext(in.State, routineArtifacts)
			artifacts = append(artifacts, routineArtifacts...)
		}
		executed = append(executed, routineID)
	}
	return artifacts, UniqueStrings(executed), nil
}

func CapabilityTaskInstruction(task *core.Task) string {
	if task == nil || strings.TrimSpace(task.Instruction) == "" {
		return "the requested change"
	}
	return strings.TrimSpace(task.Instruction)
}

func MergeStateArtifactsToContext(state *core.Context, artifacts []euclotypes.Artifact) {
	if state == nil || len(artifacts) == 0 {
		return
	}
	existing := euclotypes.ArtifactStateFromContext(state).All()
	merged := append(existing, artifacts...)
	euclostate.SetArtifacts(state, merged)
	for _, artifact := range artifacts {
		if key := euclotypes.StateKeyForArtifactKind(artifact.Kind); key != "" && artifact.Payload != nil {
			state.Set(key, artifact.Payload)
		}
	}
}

func ExecuteReactTask(ctx context.Context, in ExecuteInput, taskID, instruction string, taskType core.TaskType) (*core.Result, *core.Context, error) {
	state := in.State.Clone()
	agent := reactexec.New(in.Environment)
	task := &core.Task{ID: taskID, Instruction: instruction, Type: taskType, Context: taskContextFromInput(in)}
	result, err := agent.Execute(ctx, task, state)
	if err == nil {
		mergeStateArtifacts(in.State, state)
	}
	return result, state, err
}

func ExecuteHTNTask(ctx context.Context, in ExecuteInput, taskID, instruction string, taskType core.TaskType) (*core.Result, *core.Context, error) {
	state := in.State.Clone()
	agent := htnexec.New(in.Environment)
	task := &core.Task{ID: taskID, Instruction: instruction, Type: taskType, Context: taskContextFromInput(in)}
	result, err := agent.Execute(ctx, task, state)
	return result, state, err
}

func ExecuteBlackboardTask(ctx context.Context, in ExecuteInput, taskID, instruction string, taskType core.TaskType) (*core.Result, *core.Context, error) {
	state := in.State.Clone()
	factory := ExecutorFactory{
		Model:        in.Environment.Model,
		Registry:     in.Environment.Registry,
		Memory:       in.Environment.Memory,
		Config:       in.Environment.Config,
		IndexManager: in.Environment.IndexManager,
		SearchEngine: in.Environment.SearchEngine,
	}
	executor := newBlackboardExecutor(factory)
	task := &core.Task{ID: taskID, Instruction: instruction, Type: taskType, Context: taskContextFromInput(in)}
	result, err := executor.Execute(ctx, task, state)
	return result, state, err
}

func ExecutePlannerTask(ctx context.Context, in ExecuteInput, taskID, instruction string) (*core.Result, *core.Context, error) {
	state := in.State.Clone()
	agent := plannerexec.New(in.Environment)
	task := &core.Task{ID: taskID, Instruction: instruction, Type: core.TaskTypeAnalysis, Context: taskContextFromInput(in)}
	result, err := agent.Execute(ctx, task, state)
	if err == nil {
		mergeStateArtifacts(in.State, state)
	}
	return result, state, err
}

func ExecuteReflectionTask(ctx context.Context, in ExecuteInput, taskID, instruction string) (*core.Result, *core.Context, error) {
	state := in.State.Clone()
	agent := reflectionexec.New(in.Environment)
	task := &core.Task{ID: taskID, Instruction: instruction, Type: core.TaskTypeAnalysis, Context: taskContextFromInput(in)}
	result, err := agent.Execute(ctx, task, state)
	if err == nil {
		mergeStateArtifacts(in.State, state)
	}
	return result, state, err
}

func ExecuteArchitectTask(ctx context.Context, in ExecuteInput, taskID, instruction string, taskType core.TaskType) (*core.Result, *core.Context, error) {
	state := in.State.Clone()
	task := &core.Task{ID: taskID, Instruction: instruction, Type: taskType, Context: taskContextFromInput(in)}
	result, err := architectexec.ExecuteArchitect(ctx, in.Environment, task, state)
	if err == nil {
		mergeStateArtifacts(in.State, state)
	}
	return result, state, err
}

func ExecuteReWOOTask(ctx context.Context, in ExecuteInput, taskID, instruction string, taskType core.TaskType) (*core.Result, *core.Context, error) {
	state := in.State.Clone()
	task := &core.Task{ID: taskID, Instruction: instruction, Type: taskType, Context: taskContextFromInput(in)}
	result, err := rewoo.Execute(ctx, in.Environment, task, state)
	if err == nil {
		mergeStateArtifacts(in.State, state)
	}
	return result, state, err
}

func ExecuteRecipe(ctx context.Context, in ExecuteInput, recipeID RecipeID, taskID, instruction string) (*core.Result, *core.Context, error) {
	spec, ok := recipeSpecs[recipeID]
	if !ok {
		err := fmt.Errorf("unknown execution recipe %s", string(recipeID))
		return &core.Result{Success: false, Error: err}, nil, err
	}
	appendRecipeTrace(in.State, recipeID)
	if spec.Run == nil {
		err := fmt.Errorf("recipe %s has no runner", string(recipeID))
		return &core.Result{Success: false, Error: err}, nil, err
	}
	return spec.Run(ctx, in, spec, taskID, instruction)
}

func ExecuteEnvelopeRecipe(ctx context.Context, env euclotypes.ExecutionEnvelope, recipeID RecipeID, taskID, instruction string) (*core.Result, *core.Context, error) {
	in := ExecuteInput{
		Task:        env.Task,
		State:       env.State,
		Mode:        env.Mode,
		Profile:     env.Profile,
		Environment: env.Environment,
		Telemetry:   env.Telemetry,
	}
	if in.State == nil {
		in.State = core.NewContext()
	}
	if in.Task == nil {
		in.Task = &core.Task{}
	}
	if in.Task.Context == nil {
		in.Task.Context = map[string]any{}
	}
	if _, ok := in.Task.Context["workspace"]; !ok && env.Task != nil && env.Task.Context != nil {
		in.Task.Context["workspace"] = env.Task.Context["workspace"]
	}
	return ExecuteRecipe(ctx, in, recipeID, taskID, instruction)
}

func PropagateBehaviorTrace(dst, src *core.Context) {
	if dst == nil || src == nil {
		return
	}
	if trace, ok := euclostate.GetBehaviorTrace(src); ok {
		storeBehaviorTrace(dst, trace)
	}
}

func appendRecipeTrace(state *core.Context, recipeID RecipeID) {
	if state == nil || strings.TrimSpace(string(recipeID)) == "" {
		return
	}
	trace := readBehaviorTrace(state)
	trace.RecipeIDs = UniqueStrings(append(trace.RecipeIDs, strings.TrimSpace(string(recipeID))))
	storeBehaviorTrace(state, trace)
}

func ResultSummary(result *core.Result) string {
	if result == nil {
		return ""
	}
	if result.Data != nil {
		if summary, ok := result.Data["summary"].(string); ok && strings.TrimSpace(summary) != "" {
			return strings.TrimSpace(summary)
		}
	}
	if result.Error != nil {
		return result.Error.Error()
	}
	return "completed"
}

func ErrorMessage(err error, result *core.Result) string {
	if err != nil {
		return err.Error()
	}
	if result != nil && result.Error != nil {
		return result.Error.Error()
	}
	return "unknown error"
}

func VerificationFallbackPayload(ctx context.Context, in ExecuteInput) (map[string]any, bool) {
	if ctx.Err() != nil || in.Environment.Registry == nil || in.State == nil {
		return nil, false
	}
	workspace := taskWorkspace(in.Task)
	changedPaths := changedPathsFromPipelineCode(in.State)
	if workspace == "" || len(changedPaths) == 0 {
		return nil, false
	}
	primary := changedPaths[0]
	relPath, err := filepath.Rel(workspace, primary)
	if err != nil {
		return nil, false
	}
	relPath = filepath.ToSlash(filepath.Clean(relPath))
	ext := strings.ToLower(filepath.Ext(primary))
	if ext == ".go" {
		if payload, ok := invokeVerificationTool(ctx, in.Environment.Registry, in.State, "go_test", map[string]any{
			"working_directory": workspace,
			"package":           "./" + filepath.ToSlash(filepath.Dir(relPath)),
		}); ok {
			return payload, true
		}
	}
	if payload, ok := invokeVerificationTool(ctx, in.Environment.Registry, in.State, "exec_run_tests", map[string]any{
		"pattern": filepath.ToSlash(filepath.Dir(relPath)),
	}); ok {
		return payload, true
	}
	return nil, false
}

func VerificationToolAllowed(work eucloruntime.UnitOfWork) bool {
	for _, binding := range work.ToolBindings {
		if binding.ToolID == "verification" && binding.Allowed {
			return true
		}
	}
	return false
}

func CompilePlanFallback(work eucloruntime.UnitOfWork) map[string]any {
	steps := make([]map[string]any, 0, len(work.SemanticInputs.PatternProposals)+len(work.SemanticInputs.CoherenceSuggestions))
	for i, proposal := range work.SemanticInputs.PatternProposals {
		steps = append(steps, map[string]any{
			"id":           fmt.Sprintf("pattern-step-%d", i+1),
			"title":        proposal.Title,
			"summary":      proposal.Summary,
			"pattern_refs": append([]string(nil), proposal.PatternRefs...),
		})
	}
	for i, suggestion := range work.SemanticInputs.CoherenceSuggestions {
		steps = append(steps, map[string]any{
			"id":               fmt.Sprintf("coherence-step-%d", i+1),
			"title":            suggestion.Title,
			"summary":          suggestion.Summary,
			"suggested_action": suggestion.SuggestedAction,
		})
	}
	if len(steps) == 0 {
		return nil
	}
	return map[string]any{
		"source":                "euclo.relurpic.archaeology.compile-plan",
		"primary_capability_id": work.PrimaryRelurpicCapabilityID,
		"steps":                 steps,
		"summary":               "compiled plan synthesized from archaeology semantic inputs",
	}
}

func SuccessResult(summary string, artifacts []euclotypes.Artifact) (*core.Result, error) {
	return &core.Result{
		Success: true,
		Data: map[string]any{
			"summary":   summary,
			"artifacts": artifacts,
		},
	}, nil
}

func UniqueStrings(input []string) []string {
	if len(input) == 0 {
		return nil
	}
	seen := map[string]struct{}{}
	out := make([]string, 0, len(input))
	for _, item := range input {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		if _, ok := seen[item]; ok {
			continue
		}
		seen[item] = struct{}{}
		out = append(out, item)
	}
	return out
}

func StringValue(v any) string {
	if s, ok := v.(string); ok {
		return strings.TrimSpace(s)
	}
	return ""
}

func ContainsString(items []string, target string) bool {
	target = strings.TrimSpace(target)
	for _, item := range items {
		if strings.TrimSpace(item) == target {
			return true
		}
	}
	return false
}

func taskContextFromInput(in ExecuteInput) map[string]any {
	ctx := map[string]any{"mode": in.Mode.ModeID, "profile": in.Profile.ProfileID}
	if in.Task != nil && in.Task.Context != nil {
		for k, v := range in.Task.Context {
			ctx[k] = v
		}
	}
	return ctx
}

func mergeStateArtifacts(parent, child *core.Context) {
	if parent == nil || child == nil {
		return
	}

	// Use typed accessors for pipeline keys where available
	if v, ok := euclostate.GetPipelineExplore(child); ok {
		euclostate.SetPipelineExplore(parent, v)
	}
	if v, ok := euclostate.GetPipelineAnalyze(child); ok {
		euclostate.SetPipelineAnalyze(parent, v)
	}
	if v, ok := euclostate.GetPipelinePlan(child); ok {
		euclostate.SetPipelinePlan(parent, v)
	}
	if v, ok := euclostate.GetPipelineCode(child); ok {
		euclostate.SetPipelineCode(parent, v)
	}
	if v, ok := euclostate.GetPipelineVerify(child); ok {
		euclostate.SetPipelineVerify(parent, v)
	}
	if v, ok := euclostate.GetPipelineFinalOutput(child); ok {
		euclostate.SetPipelineFinalOutput(parent, v)
	}

	// Direct keys that don't have typed accessors yet
	for _, key := range []string{
		"euclo.review_findings", "euclo.compatibility_assessment",
		"euclo.root_cause", "euclo.root_cause_candidates", "euclo.regression_analysis",
	} {
		if raw, ok := child.Get(key); ok && raw != nil {
			parent.Set(key, raw)
		}
	}
}

func invokeVerificationTool(ctx context.Context, registry *capabilitypkg.CapabilityRegistry, state *core.Context, toolName string, args map[string]any) (map[string]any, bool) {
	if registry == nil {
		return nil, false
	}
	if _, ok := registry.Get(toolName); !ok {
		return nil, false
	}
	result, err := registry.InvokeCapability(ctx, state, toolName, args)
	if err != nil || result == nil || !result.Success {
		return nil, false
	}
	summary := strings.TrimSpace(fmt.Sprint(result.Data["summary"]))
	if summary == "" || summary == "<nil>" {
		summary = strings.TrimSpace(fmt.Sprint(result.Data["stdout"]))
	}
	if summary == "" || summary == "<nil>" {
		summary = toolName + " passed"
	}
	return map[string]any{
		"status":  "pass",
		"summary": summary,
		"checks": []map[string]any{{
			"name":    toolName,
			"command": toolName,
			"status":  "pass",
			"details": strings.TrimSpace(fmt.Sprint(result.Data["stderr"])),
		}},
	}, true
}

func changedPathsFromPipelineCode(state *core.Context) []string {
	if state == nil {
		return nil
	}
	// Try typed accessor first
	payload, ok := euclostate.GetPipelineCode(state)
	if !ok || payload == nil {
		return nil
	}
	finalOutput, ok := payload["final_output"].(map[string]any)
	if !ok {
		return nil
	}
	result, ok := finalOutput["result"].(map[string]any)
	if !ok {
		return nil
	}
	var paths []string
	for _, item := range result {
		entry, ok := item.(map[string]any)
		if !ok {
			continue
		}
		data, ok := entry["data"].(map[string]any)
		if !ok {
			continue
		}
		path := strings.TrimSpace(fmt.Sprint(data["path"]))
		if path == "" || path == "<nil>" {
			continue
		}
		paths = append(paths, path)
	}
	return paths
}

func taskWorkspace(task *core.Task) string {
	if task == nil || task.Context == nil {
		return ""
	}
	return strings.TrimSpace(fmt.Sprint(task.Context["workspace"]))
}
