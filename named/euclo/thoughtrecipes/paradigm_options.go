package thoughtrecipe

import (
	"fmt"
	"strings"

	pl "codeburg.org/lexbit/relurpify/cognitionzoo/plan"
	planneragent "codeburg.org/lexbit/relurpify/cognitionzoo/planner"
	rewooagent "codeburg.org/lexbit/relurpify/cognitionzoo/rewoo"
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
