package thoughtrecipe

import (
	"context"
	"fmt"
	"strings"

	htnagent "codeburg.org/lexbit/relurpify/cognitionzoo/htn"
	"codeburg.org/lexbit/relurpify/cognitionzoo/paradigm"
	pl "codeburg.org/lexbit/relurpify/cognitionzoo/plan"
	planneragent "codeburg.org/lexbit/relurpify/cognitionzoo/planner"
	reflectionagent "codeburg.org/lexbit/relurpify/cognitionzoo/reflection"
	rewooagent "codeburg.org/lexbit/relurpify/cognitionzoo/rewoo"
	"codeburg.org/lexbit/relurpify/context/contextdata"
	execution "codeburg.org/lexbit/relurpify/execution"
)

// paradigm_options.go lowers a step's typed directive payloads into typed
// cognitionzoo Option values. It is the paradigm-boundary crossing point (D2):
// directive strings never reach the runners, and every error here is a
// load-time validation failure (the option builders run during lowering/graph
// build, before execution).

// rewooOptions lowers the rewoo directive vocabulary (plan/step/synthesize)
// into the runner's option surface. Authored steps are authoritative: the
// runner executes exactly those, deterministically, with zero planning model
// calls. A bare `plan` (no steps) refines the generated planner's objective.
// `synthesize` guidance replaces the default synthesizer instruction. Absent
// directives yield no options and the runner keeps its library behavior (FR-9).
func rewooOptions(step ExecutionStep) ([]rewooagent.Option, error) {
	directives := step.Directives
	steps := StepItems(directives, "step")
	if !Has(directives, "plan") && len(steps) == 0 && !Has(directives, "synthesize") {
		return nil, nil
	}

	planDirective, err := ExactlyOne(directives, "plan")
	if err != nil {
		return nil, err
	}
	synthesizeDirective, hasSynthesize, err := AtMostOne(directives, "synthesize")
	if err != nil {
		return nil, err
	}

	opts := make([]rewooagent.Option, 0, 2)
	if len(steps) > 0 {
		authored, err := rewooAuthoredSteps(steps)
		if err != nil {
			return nil, err
		}
		opts = append(opts, rewooagent.WithAuthoredPlan(directiveFirstText(planDirective), authored))
	} else {
		opts = append(opts, rewooagent.WithPlanObjective(directiveFirstText(planDirective)))
	}
	if hasSynthesize {
		opts = append(opts, rewooagent.WithSynthesizeGuidance(directiveFirstText(synthesizeDirective)))
	}
	return opts, nil
}

// rewooAuthoredSteps lowers ordered `step` blocks into ReWOO executor steps.
// Step IDs are stable by declaration order (s1…sn) so completed-step resume is
// deterministic; each step depends on its predecessor so the executor's
// ready-step order is exactly the declaration order. Every step MUST carry a
// `do` clause: ReWOO steps are tool steps by definition.
func rewooAuthoredSteps(steps []TypedDirective) ([]rewooagent.RewooStep, error) {
	authored := make([]rewooagent.RewooStep, 0, len(steps))
	previousID := ""
	for i, directive := range steps {
		id := fmt.Sprintf("s%d", i+1)
		tool, err := rewooStepCapability(directive)
		if err != nil {
			return nil, err
		}
		step := rewooagent.RewooStep{
			ID:          id,
			Description: directiveFirstText(directive),
			Tool:        tool,
			Params:      map[string]any{},
		}
		if previousID != "" {
			step.DependsOn = []string{previousID}
		}
		authored = append(authored, step)
		previousID = id
	}
	return authored, nil
}

// rewooStepCapability resolves a rewoo step's `do` clause to a canonical
// capability ID. ReWOO steps are tool steps by definition, so a step without a
// `do` clause is a load error.
func rewooStepCapability(step TypedDirective) (string, error) {
	tool, err := doCapabilityID(step, "rewoo step")
	if err != nil {
		return "", err
	}
	if tool == "" {
		return "", fmt.Errorf("rewoo step %q requires a do clause", directiveFirstText(step))
	}
	return tool, nil
}

// htnOptions lowers the htn directive vocabulary (method/task) into the
// runner's option surface. Authored methods are authoritative: exactly the
// authored tasks run, in declaration order, with zero LLM decomposition calls
// (D4). Each task's text is the sub-goal and an optional `do` clause pins the
// capability it dispatches to. Absent directives yield no options and the
// runner keeps its library method-lookup behavior (FR-9).
func htnOptions(step ExecutionStep) ([]htnagent.Option, error) {
	directives := step.Directives
	methodDirective, hasMethod, err := AtMostOne(directives, "method")
	if err != nil {
		return nil, err
	}
	if !hasMethod {
		if len(StepItems(directives, "task")) > 0 {
			return nil, fmt.Errorf("htn task requires a method block")
		}
		return nil, nil
	}
	methodName := directiveFirstText(methodDirective)
	if methodName == "" {
		return nil, fmt.Errorf("htn method requires a name")
	}
	tasks := StepItems(methodDirective.Body, "task")
	if len(tasks) == 0 {
		return nil, fmt.Errorf("htn method %q requires at least one task", methodName)
	}
	// The contract admits nested directive blocks under `method`; the runner
	// honours exactly the `task` clause (belt to the load-time shape check).
	for _, body := range methodDirective.Body {
		if strings.TrimSpace(body.Name) != "" && body.Name != "task" {
			return nil, fmt.Errorf("htn method %q contains unsupported nested clause %q", methodName, body.Name)
		}
	}
	authored := make([]htnagent.AuthoredTask, 0, len(tasks))
	for i, task := range tasks {
		text := directiveFirstText(task)
		if text == "" {
			return nil, fmt.Errorf("htn task %d requires text", i+1)
		}
		capability, err := doCapabilityID(task, "htn task")
		if err != nil {
			return nil, err
		}
		authored = append(authored, htnagent.AuthoredTask{Text: text, Capability: capability})
	}
	methodOption, err := htnagent.WithAuthoredMethod(methodName, authored)
	if err != nil {
		return nil, err
	}
	return []htnagent.Option{methodOption}, nil
}

// reflectionOptions lowers the reflection directive vocabulary (review/revise)
// into the runner's option surface (D5). The review criterion drives the
// directive-mode review phase; the revise predicate (compiled by the same
// compiler route predicates use) and the lowered body steps form the bounded
// in-process revise loop. Absent directives yield no options and the runner
// keeps its library review loop (FR-9).
func reflectionOptions(step ExecutionStep, deps *paradigm.Deps) ([]reflectionagent.Option, error) {
	directives := step.Directives
	reviewDirective, hasReview, err := AtMostOne(directives, "review")
	if err != nil {
		return nil, err
	}
	reviseDirective, hasRevise, err := AtMostOne(directives, "revise")
	if err != nil {
		return nil, err
	}
	if !hasReview && !hasRevise {
		return nil, nil
	}
	opts := make([]reflectionagent.Option, 0, 2)
	if hasReview {
		criterion := directiveFirstText(reviewDirective)
		if criterion == "" {
			return nil, fmt.Errorf("reflection review requires a criterion")
		}
		opts = append(opts, reflectionagent.WithReviewCriteria(criterion))
	}
	if hasRevise {
		if reviseDirective.Predicate == nil {
			return nil, fmt.Errorf("reflection revise requires a when predicate")
		}
		predicate, err := NormalizeRoutePredicate(*reviseDirective.Predicate)
		if err != nil {
			return nil, err
		}
		condition := compilePredicate(*predicate)
		if len(step.ReviseBody) == 0 {
			return nil, fmt.Errorf("reflection revise requires at least one body step")
		}
		bodySteps := append([]ExecutionStep(nil), step.ReviseBody...)
		opts = append(opts, reflectionagent.WithReviseCycle(
			newReflectionReviseBody(bodySteps, deps),
			func(env *contextdata.Envelope) bool { return condition(nil, env) },
		))
	}
	return opts, nil
}

// newReflectionReviseBody wraps the lowered revise-body steps into the
// in-process ReviseBodyFunc: each step runs sequentially through the shared
// delegate execution core, so scoped registries, permission checks, and
// Wave-1 grounding apply to body products automatically.
func newReflectionReviseBody(steps []ExecutionStep, deps *paradigm.Deps) reflectionagent.ReviseBodyFunc {
	return func(ctx context.Context, env *contextdata.Envelope) (*execution.Result, error) {
		var last *execution.Result
		for _, bodyStep := range steps {
			result, err := ExecuteDelegateCore(ctx, deps, bodyStep, env)
			if err != nil {
				return result, err
			}
			last = result
		}
		return last, nil
	}
}

// plannerOptions lowers the planner directive vocabulary
// (plan/step/verify/summarize) into the runner's option surface. Authored
// `step` blocks make the plan structure authoritative (zero planning model
// calls); a bare `plan` selects bounded generated mode; `verify`/`summarize`
// add the quality phases. Absent directives yield no options and the runner
// keeps its library behavior (FR-9). The D1 mixing rule is also enforced at
// load (step Requires plan); the check here is the builder-level belt.
func plannerOptions(step ExecutionStep) ([]planneragent.Option, error) {
	directives := step.Directives
	hasPlan := Has(directives, "plan")
	steps := StepItems(directives, "step")
	verifyDirective, hasVerify, err := AtMostOne(directives, "verify")
	if err != nil {
		return nil, err
	}
	summarizeDirective, hasSummarize, err := AtMostOne(directives, "summarize")
	if err != nil {
		return nil, err
	}
	if !hasPlan && len(steps) == 0 && !hasVerify && !hasSummarize {
		return nil, nil
	}

	objective := ""
	if hasPlan {
		planDirective, err := ExactlyOne(directives, "plan")
		if err != nil {
			return nil, err
		}
		objective = directiveFirstText(planDirective)
	}

	opts := make([]planneragent.Option, 0, 3)
	if len(steps) > 0 {
		if !hasPlan {
			return nil, fmt.Errorf("planner step requires a plan directive")
		}
		authored, err := plannerAuthoredSteps(steps)
		if err != nil {
			return nil, err
		}
		opts = append(opts, planneragent.WithAuthoredPlan(objective, authored))
	} else {
		opts = append(opts, planneragent.WithGeneratedPlan(objective, planneragent.DefaultGeneratedPlanBound))
	}
	if hasVerify {
		opts = append(opts, planneragent.WithVerify(directiveFirstText(verifyDirective)))
	}
	if hasSummarize {
		opts = append(opts, planneragent.WithSummarize(directiveFirstText(summarizeDirective)))
	}
	return opts, nil
}

// plannerAuthoredSteps lowers ordered `step` blocks into plan steps. Step IDs
// are stable by declaration order (s1…sn) so completed-step resume is
// deterministic. A step's `do` clause pins its capability (Tool); a step with
// no `do` is reasoning-only and is skipped at execution.
func plannerAuthoredSteps(steps []TypedDirective) ([]pl.PlanStep, error) {
	authored := make([]pl.PlanStep, 0, len(steps))
	for i, directive := range steps {
		text := directiveFirstText(directive)
		if text == "" {
			return nil, fmt.Errorf("planner step %d requires text", i+1)
		}
		tool, err := doCapabilityID(directive, "planner step")
		if err != nil {
			return nil, err
		}
		authored = append(authored, pl.PlanStep{
			ID:          fmt.Sprintf("s%d", i+1),
			Description: text,
			Tool:        tool,
		})
	}
	return authored, nil
}

// doCapabilityID resolves a step's single `do relurpic:<cap>` child into the
// canonical capability ID. It returns ("", nil) when the step carries no `do`
// clause. More than one `do`, or an empty capability, is a load error. label
// names the paradigm in diagnostics.
func doCapabilityID(step TypedDirective, label string) (string, error) {
	dos := StepItems(step.Body, "do")
	if len(dos) == 0 {
		return "", nil
	}
	if len(dos) > 1 {
		return "", fmt.Errorf("%s %q declares multiple do clauses; exactly one is required", label, directiveFirstText(step))
	}
	reference := ""
	if len(dos[0].TextArgs) > 0 {
		reference = strings.TrimSpace(dos[0].TextArgs[0])
	}
	if reference == "" {
		return "", fmt.Errorf("%s %q requires a non-empty do capability", label, directiveFirstText(step))
	}
	// The DSL namespace prefix (e.g. `relurpic:`) is not part of the canonical
	// capability ID; strip it and normalize the remainder.
	if _, after, ok := strings.Cut(reference, ":"); ok {
		reference = strings.TrimSpace(after)
	}
	id := NormalizeCapabilityReference(reference)
	if id == "" {
		return "", fmt.Errorf("%s %q requires a valid do capability", label, directiveFirstText(step))
	}
	return id, nil
}

// directiveFirstText returns a directive's first text argument, unquoted, with
// whitespace trimmed.
func directiveFirstText(directive TypedDirective) string {
	if len(directive.TextArgs) == 0 {
		return ""
	}
	return strings.TrimSpace(unquoteString(directive.TextArgs[0]))
}
