package thoughtrecipe

import (
	"strings"
	"testing"

	blackboardagent "codeburg.org/lexbit/relurpify/cognitionzoo/blackboard"
	rewooagent "codeburg.org/lexbit/relurpify/cognitionzoo/rewoo"
	"codeburg.org/lexbit/relurpify/context/contextdata"
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

// TestHTNMethodCardinality pins the htn directive lowering: a method without
// tasks or a name is a load error, a task outside a method is a load error, and
// a valid authored method lowers to exactly one option.
func TestHTNMethodCardinality(t *testing.T) {
	methodWithoutTasks := ExecutionStep{
		Paradigm:   "htn",
		Directives: []TypedDirective{{Name: "method", TextArgs: []string{`"m"`}}},
	}
	if _, err := htnOptions(methodWithoutTasks); err == nil || !strings.Contains(err.Error(), "at least one task") {
		t.Fatalf("method without tasks error = %v, want at-least-one-task", err)
	}

	methodWithoutName := ExecutionStep{
		Paradigm: "htn",
		Directives: []TypedDirective{
			{Name: "method", Body: []TypedDirective{{Name: "task", TextArgs: []string{`"x"`}}}},
		},
	}
	if _, err := htnOptions(methodWithoutName); err == nil || !strings.Contains(err.Error(), "requires a name") {
		t.Fatalf("method without name error = %v, want requires-a-name", err)
	}

	taskWithoutMethod := ExecutionStep{
		Paradigm:   "htn",
		Directives: []TypedDirective{{Name: "task", TextArgs: []string{`"x"`}}},
	}
	if _, err := htnOptions(taskWithoutMethod); err == nil || !strings.Contains(err.Error(), "requires a method") {
		t.Fatalf("task outside method error = %v, want requires-a-method", err)
	}

	valid := ExecutionStep{
		Paradigm: "htn",
		Directives: []TypedDirective{
			{Name: "method", TextArgs: []string{`"full_analysis"`}, Body: []TypedDirective{
				{Name: "task", TextArgs: []string{`"Explore"`}, Body: []TypedDirective{{Name: "do", TextArgs: []string{"relurpic:layer_check"}}}},
			}},
		},
	}
	opts, err := htnOptions(valid)
	if err != nil {
		t.Fatalf("htnOptions(valid): %v", err)
	}
	if len(opts) != 1 {
		t.Fatalf("htnOptions(valid) = %d options, want 1", len(opts))
	}
}

func applyBlackboardOptions(t *testing.T, step ExecutionStep) (*blackboardagent.BlackboardAgent, error) {
	t.Helper()
	opts, err := blackboardOptions(step)
	if err != nil {
		return nil, err
	}
	return blackboardagent.New(nil, opts...), nil
}

// TestBlackboardOptionsLowering proves the source-block lowering: names, the
// compiled when predicate, read keys, the pinned capability, and the state
// write target all cross the boundary as typed values.
func TestBlackboardOptionsLowering(t *testing.T) {
	step := ExecutionStep{
		ID:       "run.board",
		Paradigm: "blackboard",
		Directives: []TypedDirective{
			{Name: "source", TextArgs: []string{`"architecture"`}, Body: []TypedDirective{
				{Name: "when", Predicate: &PredicateExpr{Raw: "state.phase is initial", Kind: "is", Subject: PathExpr{Raw: "state.phase"}, Value: StringLiteral{Value: "initial"}}},
				{Name: "read", TextArgs: []string{"state.workspace", "state.notes"}},
				{Name: "do", TextArgs: []string{"relurpic:layer_check"}},
				{Name: "write", TextArgs: []string{"state.architecture_state"}},
			}},
			{Name: "source", TextArgs: []string{`"security"`}, Body: []TypedDirective{
				{Name: "write", TextArgs: []string{"state.security_state"}},
			}},
		},
	}
	agent, err := applyBlackboardOptions(t, step)
	if err != nil {
		t.Fatalf("blackboardOptions: %v", err)
	}
	sources := agent.AuthoredSources()
	if len(sources) == 0 {
		t.Fatal("authored sources not installed")
	}
	if len(sources) != 2 {
		t.Fatalf("authored sources = %d, want 2", len(sources))
	}
	first := sources[0]
	if first.Name != "architecture" || first.Capability != "euclo:cap.layer_check" {
		t.Fatalf("source[0] = %+v", first)
	}
	if len(first.Read) != 2 || first.Read[0] != "state.workspace" {
		t.Fatalf("source[0].Read = %v", first.Read)
	}
	if first.Write != "state.architecture_state" {
		t.Fatalf("source[0].Write = %q", first.Write)
	}
	if first.When == nil {
		t.Fatal("source[0].When predicate missing")
	}
	// The predicate must actually evaluate (scratch-reach semantics included).
	env := contextdata.NewEnvelope("t", "s")
	if first.When(env) {
		t.Fatal("predicate held on an empty envelope")
	}
	env.SetWorkingValueWithClass("state.phase", "initial", contextdata.MemoryClassTask)
	if !first.When(env) {
		t.Fatal("predicate did not hold on a matching envelope")
	}
	if sources[1].Capability != "" {
		t.Fatalf("source[1].Capability = %q, want empty", sources[1].Capability)
	}
}

// TestBlackboardSourceLoweringErrors pins the per-source load errors: missing
// name, missing write, scratch write, when-after-write, and unsupported clauses.
func TestBlackboardSourceLoweringErrors(t *testing.T) {
	cases := []struct {
		name    string
		source  TypedDirective
		message string
	}{
		{
			name:    "missing name",
			source:  TypedDirective{Name: "source", Body: []TypedDirective{{Name: "write", TextArgs: []string{"state.x"}}}},
			message: "requires a name",
		},
		{
			name:    "missing write",
			source:  TypedDirective{Name: "source", TextArgs: []string{`"a"`}},
			message: "requires a write clause",
		},
		{
			name: "scratch write",
			source: TypedDirective{Name: "source", TextArgs: []string{`"a"`}, Body: []TypedDirective{
				{Name: "write", TextArgs: []string{"scratch.x"}},
			}},
			message: "must be a state.* key",
		},
		{
			name: "when after write",
			source: TypedDirective{Name: "source", TextArgs: []string{`"a"`}, Body: []TypedDirective{
				{Name: "write", TextArgs: []string{"state.x"}},
				{Name: "when", Predicate: &PredicateExpr{Raw: "state.g is ready", Kind: "is", Subject: PathExpr{Raw: "state.g"}, Value: StringLiteral{Value: "ready"}}},
			}},
			message: "when clause must precede write",
		},
		{
			name: "unsupported clause",
			source: TypedDirective{Name: "source", TextArgs: []string{`"a"`}, Body: []TypedDirective{
				{Name: "stream", TextArgs: []string{"q"}},
				{Name: "write", TextArgs: []string{"state.x"}},
			}},
			message: "unsupported clause",
		},
		{
			name: "when without predicate",
			source: TypedDirective{Name: "source", TextArgs: []string{`"a"`}, Body: []TypedDirective{
				{Name: "when"},
				{Name: "write", TextArgs: []string{"state.x"}},
			}},
			message: "when clause requires a predicate",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			step := ExecutionStep{ID: "run.board", Paradigm: "blackboard", Directives: []TypedDirective{tc.source}}
			_, err := applyBlackboardOptions(t, step)
			if err == nil {
				t.Fatalf("expected load error containing %q, got none", tc.message)
			}
			if !strings.Contains(err.Error(), tc.message) {
				t.Fatalf("error %q does not contain %q", err.Error(), tc.message)
			}
		})
	}
}

// TestBlackboardOptionsAbsentDirectivesNoOp pins FR-9 for blackboard: a step
// without source blocks lowers to no options.
func TestBlackboardOptionsAbsentDirectivesNoOp(t *testing.T) {
	opts, err := blackboardOptions(ExecutionStep{ID: "run.board", Paradigm: "blackboard"})
	if err != nil {
		t.Fatalf("blackboardOptions: %v", err)
	}
	if len(opts) != 0 {
		t.Fatalf("expected no options for absent directives, got %d", len(opts))
	}
}
