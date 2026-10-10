package reflection

import (
	"context"
	"strings"
	"sync"
	"testing"

	"codeburg.org/lexbit/relurpify/cognitionzoo/paradigm"
	"codeburg.org/lexbit/relurpify/context/contextdata"
	execution "codeburg.org/lexbit/relurpify/execution"
	"codeburg.org/lexbit/relurpify/execution/agentgraph"
	"codeburg.org/lexbit/relurpify/model"
	"codeburg.org/lexbit/relurpify/telemetry"
)

// reflectionScriptedReviewer returns scripted responses in order for every call
// and counts Chat (review) vs Generate (library) calls.
type reflectionScriptedReviewer struct {
	mu        sync.Mutex
	responses []string
	idx       int
	chatCalls int
}

func (m *reflectionScriptedReviewer) next() string {
	m.mu.Lock()
	defer m.mu.Unlock()
	text := ""
	if len(m.responses) > 0 {
		idx := m.idx
		if idx >= len(m.responses) {
			idx = len(m.responses) - 1
		}
		text = m.responses[idx]
		m.idx++
	}
	return text
}

func (m *reflectionScriptedReviewer) Generate(_ context.Context, _ string, _ *model.LLMOptions) (*model.LLMResponse, error) {
	return &model.LLMResponse{Text: m.next()}, nil
}

func (m *reflectionScriptedReviewer) GenerateStream(_ context.Context, _ string, _ *model.LLMOptions) (<-chan string, error) {
	ch := make(chan string)
	close(ch)
	return ch, nil
}

func (m *reflectionScriptedReviewer) Chat(_ context.Context, _ []model.Message, _ *model.LLMOptions) (*model.LLMResponse, error) {
	m.mu.Lock()
	m.chatCalls++
	m.mu.Unlock()
	return &model.LLMResponse{Text: m.next()}, nil
}

func (m *reflectionScriptedReviewer) ChatWithTools(ctx context.Context, messages []model.Message, _ []model.LLMToolSpec, options *model.LLMOptions) (*model.LLMResponse, error) {
	return m.Chat(ctx, messages, options)
}

func (m *reflectionScriptedReviewer) chats() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.chatCalls
}

// recordingReflectionDelegate records each work execution and succeeds.
type recordingReflectionDelegate struct {
	mu     sync.Mutex
	calls  int
	result *execution.Result
}

func (d *recordingReflectionDelegate) Initialize(*execution.Config) error { return nil }
func (d *recordingReflectionDelegate) Capabilities() []string             { return nil }
func (d *recordingReflectionDelegate) BuildGraph(context.Context, *execution.Task) (*agentgraph.Graph, error) {
	g := agentgraph.NewGraph()
	done := agentgraph.NewTerminalNode("reflection_delegate_done")
	_ = g.AddNode(done)
	_ = g.SetStart(done.ID())
	return g, nil
}

func (d *recordingReflectionDelegate) Execute(_ context.Context, _ *execution.Task, _ *contextdata.Envelope) (*execution.Result, error) {
	d.mu.Lock()
	d.calls++
	d.mu.Unlock()
	if d.result != nil {
		return d.result, nil
	}
	return &execution.Result{Success: true, Data: execution.NewToolResultPayload(map[string]any{"summary": "work done"})}, nil
}

func (d *recordingReflectionDelegate) workCalls() int {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.calls
}

func newAuthoredReflectionAgent(t *testing.T, reviewer model.LanguageModel, delegate agentgraph.WorkflowExecutor, sink telemetry.Telemetry, opts ...Option) *ReflectionAgent {
	t.Helper()
	cfg := &execution.Config{Model: "test-model", Telemetry: sink}
	deps := &paradigm.Deps{Model: reviewer, Config: cfg}
	return New(deps, delegate, opts...)
}

func scratchVerdict(env *contextdata.Envelope) string {
	verdict, _ := contextdata.GetTyped[string](env, "scratch.review")
	return verdict
}

// TestReviewWritesScratch proves the D5 contract: the review verdict is written
// to the ephemeral scratch namespace and the result fields.
func TestReviewWritesScratch(t *testing.T) {
	reviewer := &reflectionScriptedReviewer{responses: []string{`{"verdict":"issues","issues":["a"]}`}}
	delegate := &recordingReflectionDelegate{}
	agent := newAuthoredReflectionAgent(t, reviewer, delegate, nil, WithReviewCriteria("Check correctness."))

	env := contextdata.NewEnvelope("task", "session")
	result, err := agent.Execute(context.Background(), &execution.Task{ID: "task", Instruction: "work"}, env)
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if got := scratchVerdict(env); got != "issues" {
		t.Fatalf("scratch.review = %q, want issues", got)
	}
	issues, ok := contextdata.GetTyped[any](env, "scratch.review_issues")
	if !ok || strings.Join(issues.([]string), ",") != "a" {
		t.Fatalf("scratch.review_issues = %v, want [a]", issues)
	}
	fields := execution.ResultFields(result.Data)
	if fields["review"] != "issues" {
		t.Fatalf("result review = %v, want issues", fields["review"])
	}
	if reviewer.chats() != 1 {
		t.Fatalf("review calls = %d, want 1", reviewer.chats())
	}
	if delegate.workCalls() != 1 {
		t.Fatalf("work calls = %d, want 1", delegate.workCalls())
	}
}

// TestReviseFiresOnPredicateAndRereviews proves the revise body runs exactly
// once when scratch.review contains issues and the loop re-reviews to a pass.
func TestReviseFiresOnPredicateAndRereviews(t *testing.T) {
	reviewer := &reflectionScriptedReviewer{responses: []string{
		`{"verdict":"issues","issues":["a"]}`,
		`{"verdict":"pass","issues":[]}`,
	}}
	delegate := &recordingReflectionDelegate{}
	bodyRuns := 0
	body := func(context.Context, *contextdata.Envelope) (*execution.Result, error) {
		bodyRuns++
		return &execution.Result{Success: true, Data: execution.NewToolResultPayload(map[string]any{"revised": true})}, nil
	}
	predicate := func(env *contextdata.Envelope) bool {
		return strings.Contains(scratchVerdict(env), "issues")
	}
	agent := newAuthoredReflectionAgent(t, reviewer, delegate, nil,
		WithReviewCriteria("Check correctness."),
		WithReviseCycle(body, predicate))

	env := contextdata.NewEnvelope("task", "session")
	if _, err := agent.Execute(context.Background(), &execution.Task{ID: "task", Instruction: "work"}, env); err != nil {
		t.Fatalf("execute: %v", err)
	}
	if bodyRuns != 1 {
		t.Fatalf("revise body ran %d times, want 1", bodyRuns)
	}
	if got := scratchVerdict(env); got != "pass" {
		t.Fatalf("scratch.review after re-review = %q, want pass", got)
	}
	if reviewer.chats() != 2 {
		t.Fatalf("review calls = %d, want 2 (work→review→body→review)", reviewer.chats())
	}
	if delegate.workCalls() != 1 {
		t.Fatalf("work calls = %d, want 1", delegate.workCalls())
	}
}

// TestRevisionCapBounded proves persistent issues run at most MaxRevisionCycles
// bodies and emit reflection.revision_capped exactly once, ending normally.
func TestRevisionCapBounded(t *testing.T) {
	sink := &reflectionSink{}
	reviewer := &reflectionScriptedReviewer{responses: []string{`{"verdict":"issues","issues":["a"]}`}}
	delegate := &recordingReflectionDelegate{}
	bodyRuns := 0
	body := func(context.Context, *contextdata.Envelope) (*execution.Result, error) {
		bodyRuns++
		return &execution.Result{Success: true, Data: execution.NewToolResultPayload(map[string]any{"revised": true})}, nil
	}
	predicate := func(*contextdata.Envelope) bool { return true }
	agent := newAuthoredReflectionAgent(t, reviewer, delegate, sink,
		WithReviewCriteria("Check correctness."),
		WithReviseCycle(body, predicate))

	if _, err := agent.Execute(context.Background(), &execution.Task{ID: "task", Instruction: "work"}, contextdata.NewEnvelope("task", "session")); err != nil {
		t.Fatalf("execute: %v", err)
	}
	if bodyRuns != MaxRevisionCycles {
		t.Fatalf("revise body ran %d times, want %d (the cap)", bodyRuns, MaxRevisionCycles)
	}
	if got := sink.count("reflection.revision_capped"); got != 1 {
		t.Fatalf("revision_capped events = %d, want 1", got)
	}
}

// TestReviewPassEndsLoopWithoutBody proves a pass verdict on the first review
// ends the loop without executing the revise body.
func TestReviewPassEndsLoopWithoutBody(t *testing.T) {
	reviewer := &reflectionScriptedReviewer{responses: []string{`{"verdict":"pass","issues":[]}`}}
	delegate := &recordingReflectionDelegate{}
	bodyRuns := 0
	body := func(context.Context, *contextdata.Envelope) (*execution.Result, error) {
		bodyRuns++
		return &execution.Result{Success: true}, nil
	}
	agent := newAuthoredReflectionAgent(t, reviewer, delegate, nil,
		WithReviewCriteria("Check correctness."),
		WithReviseCycle(body, func(env *contextdata.Envelope) bool {
			return strings.Contains(scratchVerdict(env), "issues")
		}))

	if _, err := agent.Execute(context.Background(), &execution.Task{ID: "task", Instruction: "work"}, contextdata.NewEnvelope("task", "session")); err != nil {
		t.Fatalf("execute: %v", err)
	}
	if bodyRuns != 0 {
		t.Fatalf("revise body ran %d times, want 0 on a pass verdict", bodyRuns)
	}
}

// TestReviewInvalidVerdictClassifies probes the strict verdict grammar: an
// unrecognised verdict is ErrInvalidReview (classified model_invalid_output).
func TestReviewInvalidVerdictClassifies(t *testing.T) {
	reviewer := &reflectionScriptedReviewer{responses: []string{`{"verdict":"maybe"}`}}
	delegate := &recordingReflectionDelegate{}
	agent := newAuthoredReflectionAgent(t, reviewer, delegate, nil, WithReviewCriteria("c"))
	_, err := agent.Execute(context.Background(), &execution.Task{ID: "task"}, contextdata.NewEnvelope("task", "session"))
	if err == nil || !strings.Contains(err.Error(), ErrInvalidReview.Error()) {
		t.Fatalf("err = %v, want invalid review output", err)
	}
}

// TestReflectionZeroOptionsUnchanged pins FR-9: an agent with no directive
// options keeps the library review graph loop and writes no scratch verdict.
func TestReflectionZeroOptionsUnchanged(t *testing.T) {
	reviewer := approvingReviewer{}
	delegate := delegateAgent{}
	agent := &ReflectionAgent{
		Reviewer: reviewer,
		Delegate: delegate,
		Config:   &execution.Config{Telemetry: &reflectionSink{}, MaxIterations: 2},
	}
	env := contextdata.NewEnvelope("task", "session")
	if _, err := agent.Execute(context.Background(), &execution.Task{ID: "task", Instruction: "work"}, env); err != nil {
		t.Fatalf("execute: %v", err)
	}
	if _, ok := contextdata.GetTyped[string](env, "scratch.review"); ok {
		t.Fatal("library mode must not write the scratch review verdict")
	}
	if revise, _ := contextdata.GetTyped[bool](env, "reflection.revise"); revise {
		t.Fatal("library mode with an approving reviewer must not revise")
	}
}
