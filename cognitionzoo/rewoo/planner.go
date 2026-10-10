package rewoo

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"codeburg.org/lexbit/relurpify/capability/ports"
	"codeburg.org/lexbit/relurpify/cognitionzoo/paradigm"
	"codeburg.org/lexbit/relurpify/cognitionzoo/react"
	"codeburg.org/lexbit/relurpify/context/contextdata"
	execution "codeburg.org/lexbit/relurpify/execution"
	"codeburg.org/lexbit/relurpify/execution/prompt"
	"codeburg.org/lexbit/relurpify/model"
	telemetry "codeburg.org/lexbit/relurpify/telemetry"
)

const (
	// planPromptID is the registry prompt for the planner phase; when the
	// workspace has not authored it, an equivalent inline prompt is used.
	planPromptID = "rewoo.plan.v1"
	// synthPromptID is the registry prompt for the synthesis phase.
	synthPromptID = "rewoo.synth.v1"

	// planSourceContext marks a plan that came from the task context
	// (rewoo.plan / plan keys); planSourceLLM marks a planner LLM product.
	planSourceContext = "context"
	planSourceLLM     = "llm"

	// planOriginAuthored / planOriginGenerated / planOriginContext classify how
	// the executed plan came to be (D1/D7 provenance). `authored` means the
	// recipe declared the structure and no planning LLM call ran; `generated`
	// means one bounded planner call produced it; `context` means it was
	// supplied on the task context.
	planOriginAuthored  = "authored"
	planOriginGenerated = "generated"
	planOriginContext   = "context"

	planOutputSchema = `{"goal":"short restatement","steps":[{"id":"s1","description":"what this step does","tool":"exact tool name","params":{},"depends_on":[]}]}`
)

// PlanWithModel resolves the plan for a run and records its origin on the
// envelope under rewoo.plan_origin. An authored plan (Options.AuthoredSteps)
// is returned verbatim with no model call; otherwise the single bounded
// planner LLM call runs.
func (a *RewooAgent) PlanWithModel(ctx context.Context, task *execution.Task, env *contextdata.Envelope) (*RewooPlan, error) {
	plan, origin, err := a.resolvePlan(ctx, task, env)
	if err != nil {
		return nil, err
	}
	if env != nil {
		env.SetWorkingValueWithClass("rewoo.plan_origin", origin, contextdata.MemoryClassTask)
	}
	return plan, nil
}

// resolvePlan is the model-free authored fast path plus the generated plan
// path. Authored structures are authoritative: when the recipe authored steps,
// they are the plan, deterministically, and no planner call occurs (D1).
func (a *RewooAgent) resolvePlan(ctx context.Context, task *execution.Task, env *contextdata.Envelope) (*RewooPlan, string, error) {
	if a == nil {
		return nil, "", fmt.Errorf("rewoo: language model unavailable for planning")
	}
	if len(a.Options.AuthoredSteps) > 0 {
		a.emitRewooEvent(ctx, env, "rewoo.plan_authored", map[string]any{"steps": len(a.Options.AuthoredSteps)})
		return a.authoredPlan(task), planOriginAuthored, nil
	}
	if a.Model == nil {
		return nil, "", fmt.Errorf("rewoo: language model unavailable for planning")
	}
	a.emitLLMPhase(ctx, env, "plan", "llm")
	streamed, err := paradigm.StreamedSection(ctx, env, "rewoo")
	if err != nil {
		return nil, "", err
	}
	planMessages := []model.Message{
		{Role: "system", Content: a.resolvePhasePrompt(ctx, planPromptID, task, env)},
	}
	if streamed != "" {
		planMessages = append(planMessages, model.Message{Role: "system", Content: streamed})
	}
	planMessages = append(planMessages, model.Message{Role: "user", Content: planUserPrompt(task, a.planObjective(task), a.modelCallableToolNames(ctx))})
	resp, err := a.Model.Chat(ctx, planMessages, &model.LLMOptions{
		Model:       a.modelID(),
		Temperature: 0,
		MaxTokens:   1024,
	})
	if err != nil {
		return nil, "", err
	}
	plan, err := decodeRewooPlan(resp)
	if err != nil {
		a.emitRewooEvent(ctx, env, "rewoo_plan_invalid", map[string]any{"err": err.Error()})
		return nil, "", fmt.Errorf("%w: %v", ErrRewooPlanInvalid, err)
	}
	return plan, planOriginGenerated, nil
}

// authoredPlan lowers the recipe-authored steps into a RewooPlan. The authored
// plan objective refines the goal; an empty objective falls back to the task
// instruction so generated downstream behavior stays identical.
func (a *RewooAgent) authoredPlan(task *execution.Task) *RewooPlan {
	goal := a.planObjective(task)
	steps := make([]RewooStep, len(a.Options.AuthoredSteps))
	copy(steps, a.Options.AuthoredSteps)
	return &RewooPlan{Goal: goal, Steps: steps}
}

// decodeRewooPlan strictly decodes the model response into a plan and
// validates its structure: at least one step, every step identified and
// bound to a tool.
func decodeRewooPlan(resp *model.LLMResponse) (*RewooPlan, error) {
	if resp == nil {
		return nil, fmt.Errorf("empty model response")
	}
	var plan RewooPlan
	if err := json.Unmarshal([]byte(react.ExtractJSON(resp.Text)), &plan); err != nil {
		return nil, fmt.Errorf("plan JSON decode: %w", err)
	}
	if len(plan.Steps) == 0 {
		return nil, fmt.Errorf("plan has no steps")
	}
	seen := make(map[string]bool, len(plan.Steps))
	for i, step := range plan.Steps {
		if strings.TrimSpace(step.ID) == "" {
			return nil, fmt.Errorf("step %d has empty id", i)
		}
		if seen[step.ID] {
			return nil, fmt.Errorf("duplicate step id %q", step.ID)
		}
		seen[step.ID] = true
		if strings.TrimSpace(step.Tool) == "" {
			return nil, fmt.Errorf("step %s has empty tool", step.ID)
		}
	}
	return &plan, nil
}

// planUserPrompt renders the user turn: the objective, the callable tool names
// (registry-derived, the same source react uses for its tool list) and the
// required output schema. The objective is the authored `plan` guidance when
// present, else the task instruction.
func planUserPrompt(task *execution.Task, objective string, toolNames []string) string {
	instruction := strings.TrimSpace(objective)
	if instruction == "" && task != nil {
		instruction = task.Instruction
	}
	return fmt.Sprintf(`Task: %s

Available tools:
%s

Produce a plan of mechanical tool steps that accomplishes the task.
Return ONLY JSON matching this schema, no prose outside the JSON object:
%s`, instruction, strings.Join(toolNames, "\n"), planOutputSchema)
}

// planObjective returns the authored `plan` guidance, falling back to the task
// instruction. It is the single source for both the authored-plan goal and the
// generated planner prompt.
func (a *RewooAgent) planObjective(task *execution.Task) string {
	if a != nil {
		if objective := strings.TrimSpace(a.Options.PlanObjective); objective != "" {
			return objective
		}
	}
	return taskInstructionText(task)
}

// callableToolsForPrompt returns the registry's callable tools for prompt
// assembly.
func (a *RewooAgent) callableToolsForPrompt(ctx context.Context) []ports.Tool {
	if a == nil || a.Tools == nil {
		return nil
	}
	return a.Tools.ModelCallableTools(ctx)
}

// modelCallableToolNames lists the registry's callable tools, sorted.
func (a *RewooAgent) modelCallableToolNames(ctx context.Context) []string {
	if a == nil || a.Tools == nil {
		return nil
	}
	tools := a.Tools.ModelCallableTools(ctx)
	names := make([]string, 0, len(tools))
	for _, tool := range tools {
		names = append(names, tool.Name())
	}
	sort.Strings(names)
	return names
}

// resolvePhasePrompt resolves a phase prompt through the workspace prompt
// registry, falling back to the equivalent inline prompt so the paradigm
// stays functional before prompt files are authored (the react pattern).
func (a *RewooAgent) resolvePhasePrompt(ctx context.Context, id string, task *execution.Task, env *contextdata.Envelope) string {
	if a.PromptRegistry != nil {
		rctx := prompt.RuntimeContext{
			Variables: map[string]string{"instruction": taskInstructionText(task)},
			Task:      task,
			Envelope:  env,
			Paradigm:  "rewoo",
			Tools:     a.callableToolsForPrompt(ctx),
		}
		if a.Config != nil {
			rctx.ConsumerID = a.Config.Name
		}
		if text, err := a.PromptRegistry.Resolve(id, rctx); err == nil && strings.TrimSpace(text) != "" {
			return text
		}
	}
	if id == synthPromptID {
		return synthPrompt
	}
	return planPrompt
}

const planPrompt = `You are a ReWOO planner. Decompose the task into a short sequence of mechanical tool steps with explicit dependencies.
Return ONLY JSON matching the schema given in the user message. No prose outside the JSON object.`

const synthPrompt = `You are a ReWOO synthesizer. Compose a concise final answer to the task from the executed step results.
State the outcome directly; mention failed steps explicitly. No tool calls, no questions.`

func taskInstructionText(task *execution.Task) string {
	if task == nil {
		return ""
	}
	return task.Instruction
}

// emitLLMPhase records that an LLM phase ran (rewoo.llm_phase{phase,mode}).
func (a *RewooAgent) emitLLMPhase(ctx context.Context, env *contextdata.Envelope, phase, mode string) {
	a.emitRewooEvent(ctx, env, "rewoo.llm_phase", map[string]any{"phase": phase, "mode": mode})
}

func (a *RewooAgent) emitRewooEvent(ctx context.Context, env *contextdata.Envelope, kind string, metadata map[string]any) {
	if a == nil || a.Config == nil || a.Config.Telemetry == nil {
		return
	}
	ev := telemetry.Event{
		Type:      telemetry.EventType(kind),
		Message:   kind,
		Timestamp: time.Now().UTC(),
		TaskID:    taskIDForEnvelope(env),
		Metadata:  metadata,
	}
	telemetry.StampCorrelation(ctx, &ev)
	a.Config.Telemetry.Emit(ev)
}

func taskIDForEnvelope(env *contextdata.Envelope) string {
	if env == nil {
		return ""
	}
	return env.TaskIDSnapshot()
}
