package thoughtrecipe

import (
	"fmt"
	"strings"

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
		opts = append(opts, rewooagent.WithAuthoredPlan(rewooDirectiveText(planDirective), authored))
	} else {
		opts = append(opts, rewooagent.WithPlanObjective(rewooDirectiveText(planDirective)))
	}
	if hasSynthesize {
		opts = append(opts, rewooagent.WithSynthesizeGuidance(rewooDirectiveText(synthesizeDirective)))
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
			Description: rewooDirectiveText(directive),
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

// rewooStepCapability resolves a step's `do relurpic:<cap>` child into the
// canonical capability ID. A step without exactly one non-empty `do` clause is
// a load error.
func rewooStepCapability(step TypedDirective) (string, error) {
	dos := StepItems(step.Body, "do")
	if len(dos) == 0 {
		return "", fmt.Errorf("rewoo step %q requires a do clause", rewooDirectiveText(step))
	}
	if len(dos) > 1 {
		return "", fmt.Errorf("rewoo step %q declares multiple do clauses; exactly one is required", rewooDirectiveText(step))
	}
	reference := ""
	if len(dos[0].TextArgs) > 0 {
		reference = strings.TrimSpace(dos[0].TextArgs[0])
	}
	if reference == "" {
		return "", fmt.Errorf("rewoo step %q requires a non-empty do capability", rewooDirectiveText(step))
	}
	// The DSL namespace prefix (e.g. `relurpic:`) is not part of the canonical
	// capability ID; strip it and normalize the remainder.
	if _, after, ok := strings.Cut(reference, ":"); ok {
		reference = strings.TrimSpace(after)
	}
	id := NormalizeCapabilityReference(reference)
	if id == "" {
		return "", fmt.Errorf("rewoo step %q requires a valid do capability", rewooDirectiveText(step))
	}
	return id, nil
}

// rewooDirectiveText returns a directive's first text argument, unquoted, with
// whitespace trimmed.
func rewooDirectiveText(directive TypedDirective) string {
	if len(directive.TextArgs) == 0 {
		return ""
	}
	return strings.TrimSpace(unquoteString(directive.TextArgs[0]))
}
