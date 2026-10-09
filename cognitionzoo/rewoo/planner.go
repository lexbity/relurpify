package rewoo

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"codeburg.org/lexbit/relurpify/capability/ports"
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

	planOutputSchema = `{"goal":"short restatement","steps":[{"id":"s1","description":"what this step does","tool":"exact tool name","params":{},"depends_on":[]}]}`
)

// PlanWithModel issues exactly ONE LLM call that produces a strict-JSON
// ReWOO plan. It runs only when the task context carries no plan; a decode
// failure fails the turn with ErrRewooPlanInvalid — there is no silent
// fallback to single-step execution.
func (a *RewooAgent) PlanWithModel(ctx context.Context, task *execution.Task, env *contextdata.Envelope) (*RewooPlan, error) {
	if a == nil || a.Model == nil {
		return nil, fmt.Errorf("rewoo: language model unavailable for planning")
	}
	a.emitLLMPhase(ctx, env, "plan", "llm")
	resp, err := a.Model.Chat(ctx, []model.Message{
		{Role: "system", Content: a.resolvePhasePrompt(ctx, planPromptID, task, env)},
		{Role: "user", Content: planUserPrompt(task, a.modelCallableToolNames(ctx))},
	}, &model.LLMOptions{
		Model:       a.modelID(),
		Temperature: 0,
		MaxTokens:   1024,
	})
	if err != nil {
		return nil, err
	}
	plan, err := decodeRewooPlan(resp)
	if err != nil {
		a.emitRewooEvent(ctx, env, "rewoo_plan_invalid", map[string]any{"err": err.Error()})
		return nil, fmt.Errorf("%w: %v", ErrRewooPlanInvalid, err)
	}
	return plan, nil
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

// planUserPrompt renders the user turn: the instruction, the callable tool
// names (registry-derived, the same source react uses for its tool list) and
// the required output schema.
func planUserPrompt(task *execution.Task, toolNames []string) string {
	instruction := ""
	if task != nil {
		instruction = task.Instruction
	}
	return fmt.Sprintf(`Task: %s

Available tools:
%s

Produce a plan of mechanical tool steps that accomplishes the task.
Return ONLY JSON matching this schema, no prose outside the JSON object:
%s`, instruction, strings.Join(toolNames, "\n"), planOutputSchema)
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
