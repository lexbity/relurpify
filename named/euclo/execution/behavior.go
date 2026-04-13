package execution

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/lexcodex/relurpify/framework/agentenv"
	capabilitypkg "github.com/lexcodex/relurpify/framework/capability"
	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/framework/graph"
	"github.com/lexcodex/relurpify/named/euclo/euclotypes"
	architectexec "github.com/lexcodex/relurpify/named/euclo/execution/architect"
	htnexec "github.com/lexcodex/relurpify/named/euclo/execution/htn"
	plannerexec "github.com/lexcodex/relurpify/named/euclo/execution/planner"
	reactexec "github.com/lexcodex/relurpify/named/euclo/execution/react"
	reflectionexec "github.com/lexcodex/relurpify/named/euclo/execution/reflection"
	eucloruntime "github.com/lexcodex/relurpify/named/euclo/runtime"
)

type ExecuteInput struct {
	Task                 *core.Task
	ExecutionTask        *core.Task
	State                *core.Context
	Mode                 euclotypes.ModeResolution
	Profile              euclotypes.ExecutionProfileSelection
	Work                 eucloruntime.UnitOfWork
	Environment          agentenv.AgentEnvironment
	ServiceBundle        ServiceBundle
	WorkflowExecutor     graph.WorkflowExecutor
	Telemetry            core.Telemetry
	RunSupportingRoutine func(context.Context, string, *core.Task, *core.Context, eucloruntime.UnitOfWork, agentenv.AgentEnvironment, ServiceBundle) ([]euclotypes.Artifact, error)
}

type Behavior interface {
	ID() string
	Execute(context.Context, ExecuteInput) (*core.Result, error)
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
	RecipeChatAskInquiry                 RecipeID = "chat.ask.inquiry"
	RecipeChatAskOptions                 RecipeID = "chat.ask.options"
	RecipeChatAskReview                  RecipeID = "chat.ask.review"
	RecipeChatInspectCollect             RecipeID = "chat.inspect.collect"
	RecipeChatInspectReview              RecipeID = "chat.inspect.review"
	RecipeChatImplementArchitect         RecipeID = "chat.implement.architect"
	RecipeChatImplementExplore           RecipeID = "chat.implement.explore"
	RecipeChatImplementEdit              RecipeID = "chat.implement.edit"
	RecipeChatImplementVerify            RecipeID = "chat.implement.verify"
	RecipeDebugInvestigateReproduce      RecipeID = "debug.investigate.reproduce"
	RecipeDebugInvestigateLocalize       RecipeID = "debug.investigate.localize"
	RecipeDebugInvestigatePatch          RecipeID = "debug.investigate.patch"
	RecipeDebugInvestigateReview         RecipeID = "debug.investigate.review"
	RecipeArchaeologyExploreShape        RecipeID = "archaeology.explore.shape"
	RecipeArchaeologyExploreReview       RecipeID = "archaeology.explore.review"
	RecipeArchaeologyCompileReconcile    RecipeID = "archaeology.compile.reconcile"
	RecipeArchaeologyCompileShape        RecipeID = "archaeology.compile.shape"
	RecipeArchaeologyCompileReview       RecipeID = "archaeology.compile.review"
	RecipeArchaeologyImplementStep       RecipeID = "archaeology.implement.step"
	RecipeArchaeologyImplementCheckpoint RecipeID = "archaeology.implement.checkpoint"
	RecipeArchaeologyImplementGapDetect  RecipeID = "archaeology.implement.gap-detect"
)

var recipeSpecs = map[RecipeID]RecipeSpec{
	RecipeChatAskInquiry:                 reactRecipe(core.TaskTypeAnalysis),
	RecipeChatAskOptions:                 plannerRecipe(),
	RecipeChatAskReview:                  reflectionRecipe(),
	RecipeChatInspectCollect:             reactRecipe(core.TaskTypeAnalysis),
	RecipeChatInspectReview:              reflectionRecipe(),
	RecipeChatImplementArchitect:         architectRecipe(core.TaskTypeCodeModification),
	RecipeChatImplementExplore:           reactRecipe(core.TaskTypeAnalysis),
	RecipeChatImplementEdit:              reactRecipe(core.TaskTypeCodeModification),
	RecipeChatImplementVerify:            reactRecipe(core.TaskTypeAnalysis),
	RecipeDebugInvestigateReproduce:      reactRecipe(core.TaskTypeAnalysis),
	RecipeDebugInvestigateLocalize:       blackboardRecipe(core.TaskTypeAnalysis),
	RecipeDebugInvestigatePatch:          reactRecipe(core.TaskTypeCodeModification),
	RecipeDebugInvestigateReview:         reflectionRecipe(),
	RecipeArchaeologyExploreShape:        plannerRecipe(),
	RecipeArchaeologyExploreReview:       reflectionRecipe(),
	RecipeArchaeologyCompileReconcile:    plannerRecipe(),
	RecipeArchaeologyCompileShape:        plannerRecipe(),
	RecipeArchaeologyCompileReview:       reflectionRecipe(),
	RecipeArchaeologyImplementStep:       reactRecipe(core.TaskTypeCodeModification),
	RecipeArchaeologyImplementCheckpoint: reflectionRecipe(),
	RecipeArchaeologyImplementGapDetect:  reflectionRecipe(),
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

func SetBehaviorTrace(state *core.Context, work eucloruntime.UnitOfWork, routines []string) {
	if state == nil {
		return
	}
	trace := readBehaviorTrace(state)
	trace.PrimaryCapabilityID = work.PrimaryRelurpicCapabilityID
	trace.SupportingRoutines = append([]string(nil), routines...)
	trace.ExecutorFamily = string(work.ExecutorDescriptor.Family)
	trace.Path = "unit_of_work_behavior"
	if strings.TrimSpace(work.ExecutorDescriptor.RecipeID) != "" {
		trace.RecipeIDs = UniqueStrings(append(trace.RecipeIDs, strings.TrimSpace(work.ExecutorDescriptor.RecipeID)))
	}
	state.Set("euclo.relurpic_behavior_trace", trace)
}

func AddSpecializedCapabilityTrace(state *core.Context, capabilityID string) {
	if state == nil || strings.TrimSpace(capabilityID) == "" {
		return
	}
	trace := readBehaviorTrace(state)
	trace.SpecializedCapabilityIDs = UniqueStrings(append(trace.SpecializedCapabilityIDs, strings.TrimSpace(capabilityID)))
	state.Set("euclo.relurpic_behavior_trace", trace)
}

func readBehaviorTrace(state *core.Context) Trace {
	if state == nil {
		return Trace{}
	}
	if raw, ok := state.Get("euclo.relurpic_behavior_trace"); ok && raw != nil {
		if trace, ok := raw.(Trace); ok {
			return trace
		}
	}
	return Trace{}
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
	raw, _ := state.Get(key)
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
	state.Set(key, payload)
}

func EnsureRoutineArtifacts(state *core.Context, routineID string, work eucloruntime.UnitOfWork) {
	if state == nil {
		return
	}
	switch routineID {
	case "euclo:chat.local-review":
		if _, ok := state.Get("euclo.review_findings"); !ok {
			state.Set("euclo.review_findings", map[string]any{
				"review_source":         routineID,
				"primary_capability_id": work.PrimaryRelurpicCapabilityID,
				"summary":               "local review routine prepared inspection context",
				"findings":              []map[string]any{},
			})
		}
	case "euclo:chat.targeted-verification-repair":
		if _, ok := state.Get("euclo.verification_summary"); !ok {
			state.Set("euclo.verification_summary", map[string]any{
				"source":     routineID,
				"summary":    "targeted verification repair routine activated",
				"provenance": "absent",
			})
		}
	case "euclo:debug.root-cause":
		if _, ok := state.Get("euclo.root_cause_candidates"); !ok {
			state.Set("euclo.root_cause_candidates", map[string]any{
				"source":       routineID,
				"tension_refs": append([]string(nil), work.SemanticInputs.TensionRefs...),
				"summary":      "root cause candidates derived from debug investigation context",
			})
		}
	case "euclo:debug.localization":
		if _, ok := state.Get("euclo.root_cause"); !ok {
			state.Set("euclo.root_cause", map[string]any{
				"source":          routineID,
				"pattern_refs":    append([]string(nil), work.SemanticInputs.PatternRefs...),
				"touched_symbols": work.SemanticInputs.RequestProvenanceRefs,
				"summary":         "localization routine prepared candidate fault scope",
			})
		}
	case "euclo:debug.verification-repair":
		if _, ok := state.Get("euclo.regression_analysis"); !ok {
			state.Set("euclo.regression_analysis", map[string]any{
				"source":  routineID,
				"summary": "verification repair routine activated for bounded debug repair",
			})
		}
	case "euclo:archaeology.pattern-surface":
		if _, ok := state.Get("pipeline.explore"); !ok {
			state.Set("pipeline.explore", map[string]any{
				"source":       routineID,
				"pattern_refs": append([]string(nil), work.SemanticInputs.PatternRefs...),
				"summary":      "pattern surface routine grounded archaeology execution",
			})
		}
	case "euclo:archaeology.prospective-assess", "euclo:archaeology.convergence-guard":
		raw, _ := state.Get("euclo.plan_candidates")
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
		state.Set("euclo.plan_candidates", payload)
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
		if in.RunSupportingRoutine == nil {
			EnsureRoutineArtifacts(in.State, routineID, in.Work)
			executed = append(executed, routineID)
			continue
		}
		routineArtifacts, err := in.RunSupportingRoutine(ctx, routineID, in.Task, in.State, in.Work, in.Environment, in.ServiceBundle)
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
	state.Set("euclo.artifacts", merged)
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
	if raw, ok := src.Get("euclo.relurpic_behavior_trace"); ok && raw != nil {
		dst.Set("euclo.relurpic_behavior_trace", raw)
	}
}

func appendRecipeTrace(state *core.Context, recipeID RecipeID) {
	if state == nil || strings.TrimSpace(string(recipeID)) == "" {
		return
	}
	trace := readBehaviorTrace(state)
	trace.RecipeIDs = UniqueStrings(append(trace.RecipeIDs, strings.TrimSpace(string(recipeID))))
	state.Set("euclo.relurpic_behavior_trace", trace)
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
	for _, key := range []string{
		"pipeline.explore", "pipeline.analyze", "pipeline.plan",
		"pipeline.code", "pipeline.verify", "pipeline.final_output",
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
	raw, ok := state.Get("pipeline.code")
	if !ok || raw == nil {
		return nil
	}
	payload, ok := raw.(map[string]any)
	if !ok {
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
