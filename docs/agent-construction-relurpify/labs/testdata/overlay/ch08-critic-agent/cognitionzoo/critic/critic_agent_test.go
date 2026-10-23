package critic_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"testing/fstest"

	// Blank-import react so the critic's default delegate constructor has
	// what it needs and the test binary mirrors the runtime's linking.
	_ "codeburg.org/lexbit/relurpify/cognitionzoo/react"

	"codeburg.org/lexbit/relurpify/cognitionzoo/critic"
	"codeburg.org/lexbit/relurpify/cognitionzoo/paradigm"
	"codeburg.org/lexbit/relurpify/context/contextdata"
	execution "codeburg.org/lexbit/relurpify/execution"
	"codeburg.org/lexbit/relurpify/execution/agentgraph"
	"codeburg.org/lexbit/relurpify/execution/prompt"
	"codeburg.org/lexbit/relurpify/governance/permissions"
	"codeburg.org/lexbit/relurpify/telemetry"
)

// allowAllChecker mirrors the rewoo governance tests: a one-line adapter
// over the narrow permissions.CapabilityChecker port.
func allowAllChecker() permissions.CapabilityChecker {
	return checkerFunc(func(ctx context.Context, agentID, capability string) error { return nil })
}

type checkerFunc func(ctx context.Context, agentID, capability string) error

func (f checkerFunc) CheckCapability(ctx context.Context, agentID, capability string) error {
	return f(ctx, agentID, capability)
}

// recordingDelegate records each work execution and succeeds — the draft
// phase's double, so the tests count delegate runs separately from
// critique calls.
type recordingDelegate struct {
	calls int
}

func (d *recordingDelegate) Initialize(*execution.Config) error { return nil }
func (d *recordingDelegate) Capabilities() []string             { return nil }
func (d *recordingDelegate) BuildGraph(context.Context, *execution.Task) (*agentgraph.Graph, error) {
	g := agentgraph.NewGraph()
	done := agentgraph.NewTerminalNode("critic_delegate_done")
	_ = g.AddNode(done)
	_ = g.SetStart(done.ID())
	return g, nil
}

func (d *recordingDelegate) Execute(_ context.Context, _ *execution.Task, _ *contextdata.Envelope) (*execution.Result, error) {
	d.calls++
	return &execution.Result{Success: true, Data: execution.NewToolResultPayload(map[string]any{"summary": "draft done"})}, nil
}

// recordingSink captures paradigm-level telemetry events.
type recordingSink struct {
	events []telemetry.Event
}

func (s *recordingSink) Emit(event telemetry.Event) { s.events = append(s.events, event) }

func (s *recordingSink) has(eventType telemetry.EventType) bool {
	for _, event := range s.events {
		if event.Type == eventType {
			return true
		}
	}
	return false
}

func newCriticAgent(t *testing.T, model *critic.FakeModel, delegate *recordingDelegate, sink telemetry.Telemetry, opts ...critic.Option) *critic.CriticAgent {
	t.Helper()
	cfg := &execution.Config{Model: "test-model", Telemetry: sink}
	deps := &paradigm.Deps{Model: model, Config: cfg, PermissionChecker: allowAllChecker()}
	return critic.New(deps, delegate, opts...)
}

func scratchCriticVerdict(env *contextdata.Envelope) string {
	verdict, _ := contextdata.GetTyped[string](env, "scratch.critic.verdict")
	return verdict
}

// TestCriticFailsClosedWithoutPermissionChecker is the governance bar
// (D14): no permission checker, no run — an error, not a silent downgrade.
func TestCriticFailsClosedWithoutPermissionChecker(t *testing.T) {
	cfg := &execution.Config{Model: "test-model"}
	deps := &paradigm.Deps{Model: critic.NewFakeModel(critic.FakeTurn{Chunks: []string{"x"}}), Config: cfg}
	agent := critic.New(deps, &recordingDelegate{}, critic.WithCritiqueCriteria("check it"))

	_, err := agent.Execute(context.Background(), &execution.Task{ID: "t", Instruction: "work"}, nil)
	if !errors.Is(err, critic.ErrNoPermissionChecker) {
		t.Fatalf("err = %v, want ErrNoPermissionChecker", err)
	}
}

// TestCritiqueWritesScratch proves the contract: the critique verdict is
// written to the scratch namespace and the result fields, the delegate ran
// once (the draft), and the critic model saw the criteria.
func TestCritiqueWritesScratch(t *testing.T) {
	model := critic.NewFakeModel(critic.FakeTurn{Chunks: []string{`{"verdict":"issues","issues":["a"]}`}})
	delegate := &recordingDelegate{}
	agent := newCriticAgent(t, model, delegate, nil, critic.WithCritiqueCriteria("Check error handling."))

	env := contextdata.NewEnvelope("task", "session")
	result, err := agent.Execute(context.Background(), &execution.Task{ID: "task", Instruction: "work"}, env)
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if got := scratchCriticVerdict(env); got != "issues" {
		t.Fatalf("scratch.critic.verdict = %q, want issues", got)
	}
	issues, ok := contextdata.GetTyped[any](env, "scratch.critic.issues")
	if !ok || strings.Join(issues.([]string), ",") != "a" {
		t.Fatalf("scratch.critic.issues = %v, want [a]", issues)
	}
	fields := execution.ResultFields(result.Data)
	if fields["critique"] != "issues" {
		t.Fatalf("result critique = %v, want issues", fields["critique"])
	}
	if model.Calls() != 1 {
		t.Fatalf("critique calls = %d, want 1", model.Calls())
	}
	if delegate.calls != 1 {
		t.Fatalf("delegate (draft) calls = %d, want 1", delegate.calls)
	}
	if !strings.Contains(model.LastInput(), "Check error handling.") {
		t.Fatalf("critique prompt = %q, want the authored criteria", model.LastInput())
	}
}

// TestReviseFiresOnPredicateAndRecritiques proves the revise body runs
// exactly once when scratch.critic.verdict contains issues and the loop
// re-critiques to a pass.
func TestReviseFiresOnPredicateAndRecritiques(t *testing.T) {
	model := critic.NewFakeModel(critic.FakeTurn{
		Chunks: []string{`{"verdict":"issues","issues":["a"]}`},
	}, critic.FakeTurn{
		Chunks: []string{`{"verdict":"pass","issues":[]}`},
	})
	delegate := &recordingDelegate{}
	bodyRuns := 0
	body := func(context.Context, *contextdata.Envelope) (*execution.Result, error) {
		bodyRuns++
		return &execution.Result{Success: true, Data: execution.NewToolResultPayload(map[string]any{"revised": true})}, nil
	}
	predicate := func(env *contextdata.Envelope) bool {
		return strings.Contains(scratchCriticVerdict(env), "issues")
	}
	agent := newCriticAgent(t, model, delegate, nil,
		critic.WithCritiqueCriteria("Check error handling."),
		critic.WithReviseCycle(body, predicate))

	env := contextdata.NewEnvelope("task", "session")
	if _, err := agent.Execute(context.Background(), &execution.Task{ID: "task", Instruction: "work"}, env); err != nil {
		t.Fatalf("execute: %v", err)
	}
	if bodyRuns != 1 {
		t.Fatalf("revise body ran %d times, want 1", bodyRuns)
	}
	if got := scratchCriticVerdict(env); got != "pass" {
		t.Fatalf("scratch.critic.verdict after re-critique = %q, want pass", got)
	}
	if model.Calls() != 2 {
		t.Fatalf("critique calls = %d, want 2 (draft→critique→body→critique)", model.Calls())
	}
}

// TestRevisionCapEndsLoopNormally proves the bound is a contract constant
// with a telemetry receipt: persistent issues run at most MaxRevisionCycles
// bodies, critic.revision_capped is emitted, and the loop ends with the
// last verdict standing — success, not error.
func TestRevisionCapEndsLoopNormally(t *testing.T) {
	model := critic.NewFakeModel(critic.FakeTurn{Chunks: []string{`{"verdict":"issues","issues":["persistent"]}`}})
	delegate := &recordingDelegate{}
	sink := &recordingSink{}
	bodyRuns := 0
	body := func(context.Context, *contextdata.Envelope) (*execution.Result, error) {
		bodyRuns++
		return &execution.Result{Success: true}, nil
	}
	agent := newCriticAgent(t, model, delegate, sink,
		critic.WithCritiqueCriteria("Check error handling."),
		critic.WithReviseCycle(body, func(env *contextdata.Envelope) bool {
			return strings.Contains(scratchCriticVerdict(env), "issues")
		}))

	env := contextdata.NewEnvelope("task", "session")
	result, err := agent.Execute(context.Background(), &execution.Task{ID: "task", Instruction: "work"}, env)
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if !result.Success {
		t.Fatalf("result = %+v, want success (a cap is a bound, not a malfunction)", result)
	}
	if bodyRuns != critic.MaxRevisionCycles {
		t.Fatalf("revise body ran %d times, want %d", bodyRuns, critic.MaxRevisionCycles)
	}
	fields := execution.ResultFields(result.Data)
	if fields["revisions"] != critic.MaxRevisionCycles {
		t.Fatalf("result revisions = %v, want %d", fields["revisions"], critic.MaxRevisionCycles)
	}
	if !sink.has(telemetry.EventType("critic.revision_capped")) {
		t.Fatalf("telemetry = %+v, want critic.revision_capped", sink.events)
	}
}

// TestLibraryModeReportsWithoutRevision proves the zero value keeps the
// library critique report (FR-9): the graph runs, one critique, no revise.
func TestLibraryModeReportsWithoutRevision(t *testing.T) {
	model := critic.NewFakeModel(critic.FakeTurn{Chunks: []string{`{"verdict":"pass","issues":[]}`}})
	delegate := &recordingDelegate{}
	agent := newCriticAgent(t, model, delegate, nil)

	env := contextdata.NewEnvelope("task", "session")
	result, err := agent.Execute(context.Background(), &execution.Task{ID: "task", Instruction: "work"}, env)
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if !result.Success {
		t.Fatalf("result = %+v, want success", result)
	}
	if delegate.calls != 1 || model.Calls() != 1 {
		t.Fatalf("delegate=%d model=%d, want 1/1", delegate.calls, model.Calls())
	}
	if agent.Capabilities()[0] != "critic" {
		t.Fatalf("capabilities = %v", agent.Capabilities())
	}
}

// TestCriticPromptFileLoadsAndValidates is the book-1 payoff: the workspace
// prompt file follows the agent.<agent-name>.default convention, loads
// through the prompt registry, and validates clean.
func TestCriticPromptFileLoadsAndValidates(t *testing.T) {
	reg := prompt.NewRegistry()
	err := reg.LoadFS(fstest.MapFS{
		"agent.critic.default.prompt": {Data: []byte(criticPromptFile)},
	}, "prompts")
	if err != nil {
		t.Fatalf("LoadFS: %v", err)
	}
	if _, ok := reg.Get("agent.critic.default"); !ok {
		t.Fatal("expected agent.critic.default to be resolvable")
	}
	if issues := reg.Validate("agent.critic.default"); len(issues) != 0 {
		t.Fatalf("validate issues = %v, want none", issues)
	}
}

// criticPromptFile is the workspace artifact shipped in the overlay at
// relurpify_cfg/prompts/agents/agent.critic.default.prompt — kept in sync
// by the test (the file must load byte-for-byte as shipped).
const criticPromptFile = `---
schema framework.prompt/v2
id agent.critic.default
tag "system"
tag "agent"
tag "critic"
---

You are an independent critique agent. You did not write the work under
review, and you never edit it: you judge it.

Judge the material against the criterion. Be specific: name file, symbol,
or line where an issue lives. Reply with ONLY a JSON object, no prose:

{"verdict":"pass"|"fail"|"issues","issues":["..."]}

"issues" means the work needs a revision; name every issue that a revision
must resolve.
`
