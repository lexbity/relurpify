package thoughtrecipe

import (
	"strings"
	"testing"

	rewooagent "codeburg.org/lexbit/relurpify/cognitionzoo/rewoo"
)

func applyRewooOptions(t *testing.T, step ExecutionStep) (*rewooagent.RewooAgent, error) {
	t.Helper()
	opts, err := rewooOptions(step)
	if err != nil {
		return nil, err
	}
	agent := &rewooagent.RewooAgent{}
	for _, opt := range opts {
		opt(agent)
	}
	return agent, nil
}

// TestRewooOptionsBuildsAuthoredPlan proves the directive-to-option lowering:
// ordered step blocks become an authored plan with stable IDs, a linear
// dependency chain, and canonical capability IDs; synthesize guidance lands.
func TestRewooOptionsBuildsAuthoredPlan(t *testing.T) {
	step := ExecutionStep{
		ID:       "run.executor",
		Paradigm: "rewoo",
		Directives: []TypedDirective{
			{Name: "plan", TextArgs: []string{`"Identify the checks needed."`}},
			{Name: "step", TextArgs: []string{`"Check architecture"`}, Body: []TypedDirective{
				{Name: "do", TextArgs: []string{"relurpic:layer_check"}},
			}},
			{Name: "step", TextArgs: []string{`"Suggest fixes"`}, Body: []TypedDirective{
				{Name: "do", TextArgs: []string{"relurpic:targeted_refactor"}},
			}},
			{Name: "synthesize", TextArgs: []string{`"Combine the results."`}},
		},
	}
	agent, err := applyRewooOptions(t, step)
	if err != nil {
		t.Fatalf("rewooOptions: %v", err)
	}
	if agent.Options.PlanObjective != "Identify the checks needed." {
		t.Fatalf("PlanObjective = %q", agent.Options.PlanObjective)
	}
	if agent.Options.SynthesizeGuidance != "Combine the results." {
		t.Fatalf("SynthesizeGuidance = %q", agent.Options.SynthesizeGuidance)
	}
	steps := agent.Options.AuthoredSteps
	if len(steps) != 2 {
		t.Fatalf("authored steps = %d, want 2", len(steps))
	}
	if steps[0].ID != "s1" || steps[1].ID != "s2" {
		t.Fatalf("step IDs = %q,%q, want s1,s2", steps[0].ID, steps[1].ID)
	}
	if steps[0].Tool != "euclo:cap.layer_check" || steps[1].Tool != "euclo:cap.targeted_refactor" {
		t.Fatalf("step tools = %q,%q", steps[0].Tool, steps[1].Tool)
	}
	if len(steps[0].DependsOn) != 0 {
		t.Fatalf("step 1 dependencies = %v, want none", steps[0].DependsOn)
	}
	if len(steps[1].DependsOn) != 1 || steps[1].DependsOn[0] != "s1" {
		t.Fatalf("step 2 dependencies = %v, want [s1]", steps[1].DependsOn)
	}
}

// TestRewooStepRequiresDo proves a step without a `do` clause is a load error.
func TestRewooStepRequiresDo(t *testing.T) {
	step := ExecutionStep{
		ID:       "run.executor",
		Paradigm: "rewoo",
		Directives: []TypedDirective{
			{Name: "plan", TextArgs: []string{`"p"`}},
			{Name: "step", TextArgs: []string{`"no tool"`}},
		},
	}
	_, err := applyRewooOptions(t, step)
	if err == nil {
		t.Fatal("expected a load error for a step without a do clause")
	}
	if !strings.Contains(err.Error(), "requires a do clause") {
		t.Fatalf("error %q missing the do-clause diagnostic", err)
	}
}

// TestRewooOptionsAbsentDirectivesNoOp pins FR-9: a rewoo step with no
// directive vocabulary yields no options (library behavior is unchanged).
func TestRewooOptionsAbsentDirectivesNoOp(t *testing.T) {
	opts, err := rewooOptions(ExecutionStep{ID: "run.executor", Paradigm: "rewoo"})
	if err != nil {
		t.Fatalf("rewooOptions: %v", err)
	}
	if len(opts) != 0 {
		t.Fatalf("options = %d, want 0 for a directive-free step", len(opts))
	}
}

// TestRewooMissingPlanIsLoadError proves `step` without `plan` is a load error.
func TestRewooMissingPlanIsLoadError(t *testing.T) {
	step := ExecutionStep{
		Paradigm: "rewoo",
		Directives: []TypedDirective{
			{Name: "step", Body: []TypedDirective{{Name: "do", TextArgs: []string{"relurpic:x"}}}},
		},
	}
	_, err := applyRewooOptions(t, step)
	if err == nil || !strings.Contains(err.Error(), "missing required directive") {
		t.Fatalf("error = %v, want missing required directive", err)
	}
}

// TestPlannerOptionsLowering pins the planner directive lowering: absent
// directives yield no options, authored and generated plan modes both lower to
// options, and `step` without `plan` is a builder-level load error (belt).
func TestPlannerOptionsLowering(t *testing.T) {
	absent, err := plannerOptions(ExecutionStep{Paradigm: "planner"})
	if err != nil {
		t.Fatalf("plannerOptions(absent): %v", err)
	}
	if len(absent) != 0 {
		t.Fatalf("plannerOptions(absent) = %d options, want 0", len(absent))
	}

	authored, err := plannerOptions(ExecutionStep{
		Paradigm: "planner",
		Directives: []TypedDirective{
			{Name: "plan", TextArgs: []string{`"Identify the checks."`}},
			{Name: "step", TextArgs: []string{`"Check architecture"`}, Body: []TypedDirective{{Name: "do", TextArgs: []string{"relurpic:layer_check"}}}},
			{Name: "verify", TextArgs: []string{`"The checks are complete."`}},
			{Name: "summarize", TextArgs: []string{`"Produce a report."`}},
		},
	})
	if err != nil {
		t.Fatalf("plannerOptions(authored): %v", err)
	}
	if len(authored) != 3 {
		t.Fatalf("plannerOptions(authored) = %d options, want 3 (plan+verify+summarize)", len(authored))
	}

	generated, err := plannerOptions(ExecutionStep{
		Paradigm:   "planner",
		Directives: []TypedDirective{{Name: "plan", TextArgs: []string{`"List the checks."`}}},
	})
	if err != nil {
		t.Fatalf("plannerOptions(generated): %v", err)
	}
	if len(generated) != 1 {
		t.Fatalf("plannerOptions(generated) = %d options, want 1", len(generated))
	}

	if _, err := plannerOptions(ExecutionStep{
		Paradigm:   "planner",
		Directives: []TypedDirective{{Name: "step", TextArgs: []string{`"x"`}, Body: []TypedDirective{{Name: "do", TextArgs: []string{"relurpic:y"}}}}},
	}); err == nil || !strings.Contains(err.Error(), "requires a plan") {
		t.Fatalf("plannerOptions(step without plan) error = %v, want requires-plan", err)
	}
}
