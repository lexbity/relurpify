package htn

import (
	"context"
	"sync"
	"testing"

	"codeburg.org/lexbit/relurpify/capability/agentspec"
	"codeburg.org/lexbit/relurpify/capability/descriptor"
	"codeburg.org/lexbit/relurpify/capability/handler"
	"codeburg.org/lexbit/relurpify/capability/ports"
	"codeburg.org/lexbit/relurpify/capability/registry"
	"codeburg.org/lexbit/relurpify/cognitionzoo/htn/runtime"
	"codeburg.org/lexbit/relurpify/cognitionzoo/paradigm"
	"codeburg.org/lexbit/relurpify/context/contextdata"
	execution "codeburg.org/lexbit/relurpify/execution"
	graph "codeburg.org/lexbit/relurpify/execution/agentgraph"
)

// htnProbeCapability records the order in which it is invoked. Its descriptor
// ID is the canonical capability ID used as the dispatch target.
type htnProbeCapability struct {
	id    string
	order *[]string
}

func (h *htnProbeCapability) Descriptor(_ context.Context, _ ports.State) descriptor.CapabilityDescriptor {
	return descriptor.CapabilityDescriptor{
		ID:            h.id,
		Name:          h.id,
		Kind:          agentspec.CapabilityKindTool,
		RuntimeFamily: agentspec.CapabilityRuntimeFamilyProvider,
		Availability:  descriptor.AvailabilitySpec{Available: true},
	}
}

func (h *htnProbeCapability) Invoke(_ context.Context, _ ports.State, _ map[string]any) (*ports.ToolResult, error) {
	*h.order = append(*h.order, h.id)
	return &ports.ToolResult{Success: true, Data: map[string]any{"capability_id": h.id}}, nil
}

var _ handler.InvocableCapabilityHandler = (*htnProbeCapability)(nil)

// htnRecordingPrimitive is a primitive executor that records the step IDs it
// was asked to run and immediately succeeds.
type htnRecordingPrimitive struct {
	mu    sync.Mutex
	calls []string
}

func (p *htnRecordingPrimitive) record(id string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.calls = append(p.calls, id)
}

func (p *htnRecordingPrimitive) ids() []string {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]string(nil), p.calls...)
}

func (p *htnRecordingPrimitive) Initialize(*execution.Config) error { return nil }
func (p *htnRecordingPrimitive) Capabilities() []string             { return nil }
func (p *htnRecordingPrimitive) BuildGraph(context.Context, *execution.Task) (*graph.Graph, error) {
	g := graph.NewGraph()
	done := graph.NewTerminalNode("htn_test_done")
	if err := g.AddNode(done); err != nil {
		return nil, err
	}
	if err := g.SetStart(done.ID()); err != nil {
		return nil, err
	}
	return g, nil
}

func (p *htnRecordingPrimitive) Execute(_ context.Context, task *execution.Task, _ *contextdata.Envelope) (*execution.Result, error) {
	if task != nil {
		p.record(task.ID)
	}
	return &execution.Result{Success: true, Data: execution.NewToolResultPayload(map[string]any{"primitive": true})}, nil
}

func newHTNTestRegistry(t *testing.T, order *[]string, ids ...string) *registry.CapabilityRegistry {
	t.Helper()
	reg := registry.NewRegistry()
	for _, id := range ids {
		if err := reg.RegisterInvocableCapability(context.Background(), &htnProbeCapability{id: id, order: order}); err != nil {
			t.Fatalf("register %s: %v", id, err)
		}
	}
	return reg
}

func newAuthoredHTNAgent(t *testing.T, reg *registry.CapabilityRegistry, primitive *htnRecordingPrimitive, opts ...Option) *HTNAgent {
	t.Helper()
	deps := &paradigm.Deps{Model: nil, Registry: reg, Config: &execution.Config{Name: "htn-test"}}
	base := []Option{WithPrimitiveExec(primitive)}
	base = append(base, opts...)
	return New(deps, runtime.NewMethodLibrary(), base...)
}

func htnTask() *execution.Task {
	return &execution.Task{ID: "task", Type: "explain", Instruction: "do the work"}
}

// TestAuthoredMethodDecomposesExactly proves the D4 contract: three authored
// tasks become exactly three plan steps in declaration order with zero
// decomposition model calls.
func TestAuthoredMethodDecomposesExactly(t *testing.T) {
	order := []string{}
	reg := newHTNTestRegistry(t, &order, "euclo:cap.probe_a", "euclo:cap.probe_b", "euclo:cap.probe_c")
	primitive := &htnRecordingPrimitive{}
	opt, err := WithAuthoredMethod("full_analysis", []AuthoredTask{
		{Text: "explore", Capability: "euclo:cap.probe_a"},
		{Text: "check", Capability: "euclo:cap.probe_b"},
		{Text: "plan", Capability: "euclo:cap.probe_c"},
	})
	if err != nil {
		t.Fatalf("WithAuthoredMethod: %v", err)
	}
	agent := newAuthoredHTNAgent(t, reg, primitive, opt)

	env := contextdata.NewEnvelope("task", "session")
	if _, err := agent.Execute(context.Background(), htnTask(), env); err != nil {
		t.Fatalf("execute: %v", err)
	}
	want := []string{"euclo:cap.probe_a", "euclo:cap.probe_b", "euclo:cap.probe_c"}
	if len(order) != len(want) {
		t.Fatalf("decomposed tasks executed = %v, want %v", order, want)
	}
	for i := range want {
		if order[i] != want[i] {
			t.Fatalf("task execution order = %v, want %v", order, want)
		}
	}
	if got := len(primitive.ids()); got != 0 {
		t.Fatalf("react primitive ran %d times, want 0 (all tasks pinned)", got)
	}
	if total, _ := contextdata.GetTyped[int](env, "htn.tasks_total"); total != 3 {
		t.Fatalf("htn.tasks_total = %d, want 3", total)
	}
	if completed, _ := contextdata.GetTyped[int](env, "htn.tasks_completed"); completed != 3 {
		t.Fatalf("htn.tasks_completed = %d, want 3", completed)
	}
	if method, _ := contextdata.GetTyped[string](env, "htn.method"); method != "full_analysis" {
		t.Fatalf("htn.method = %q, want full_analysis", method)
	}
}

// TestAuthoredTaskCapabilityPin proves a task's `do` capability is the
// dispatched target: the pinned capability runs and the react primitive never
// sees the task.
func TestAuthoredTaskCapabilityPin(t *testing.T) {
	order := []string{}
	reg := newHTNTestRegistry(t, &order, "euclo:cap.probe_pin")
	primitive := &htnRecordingPrimitive{}
	opt, err := WithAuthoredMethod("pinned", []AuthoredTask{
		{Text: "inspect", Capability: "euclo:cap.probe_pin"},
	})
	if err != nil {
		t.Fatalf("WithAuthoredMethod: %v", err)
	}
	agent := newAuthoredHTNAgent(t, reg, primitive, opt)

	env := contextdata.NewEnvelope("task", "session")
	if _, err := agent.Execute(context.Background(), htnTask(), env); err != nil {
		t.Fatalf("execute: %v", err)
	}
	if len(order) != 1 || order[0] != "euclo:cap.probe_pin" {
		t.Fatalf("capability invocations = %v, want [euclo:cap.probe_pin]", order)
	}
	if got := primitive.ids(); len(got) != 0 {
		t.Fatalf("primitive ran %d times, want 0 (the task was pinned)", len(got))
	}
}

// TestAuthoredUnpinnedTaskRunsPrimitive proves a task without a `do` clause
// runs the react primitive with the task text as the goal.
func TestAuthoredUnpinnedTaskRunsPrimitive(t *testing.T) {
	reg := registry.NewRegistry()
	primitive := &htnRecordingPrimitive{}
	opt, err := WithAuthoredMethod("unpinned", []AuthoredTask{
		{Text: "think about the result"},
	})
	if err != nil {
		t.Fatalf("WithAuthoredMethod: %v", err)
	}
	agent := newAuthoredHTNAgent(t, reg, primitive, opt)

	env := contextdata.NewEnvelope("task", "session")
	if _, err := agent.Execute(context.Background(), htnTask(), env); err != nil {
		t.Fatalf("execute: %v", err)
	}
	if got := primitive.ids(); len(got) != 1 {
		t.Fatalf("primitive ran %d times, want 1 (the unpinned task)", len(got))
	}
	if _, ok := contextdata.GetTyped[int](env, "htn.tasks_completed"); !ok {
		t.Fatal("expected the authored tasks_completed field")
	}
}

// TestAuthoredMethodResume proves completed tasks are not re-executed.
func TestAuthoredMethodResume(t *testing.T) {
	order := []string{}
	reg := newHTNTestRegistry(t, &order, "euclo:cap.probe_a", "euclo:cap.probe_b")
	primitive := &htnRecordingPrimitive{}
	opt, err := WithAuthoredMethod("resume", []AuthoredTask{
		{Text: "first", Capability: "euclo:cap.probe_a"},
		{Text: "second", Capability: "euclo:cap.probe_b"},
	})
	if err != nil {
		t.Fatalf("WithAuthoredMethod: %v", err)
	}
	agent := newAuthoredHTNAgent(t, reg, primitive, opt)

	env := contextdata.NewEnvelope("task", "session")
	env.SetWorkingValueWithClass("plan.completed_steps", []string{"t1"}, contextdata.MemoryClassTask)
	if _, err := agent.Execute(context.Background(), htnTask(), env); err != nil {
		t.Fatalf("execute: %v", err)
	}
	if len(order) != 1 || order[0] != "euclo:cap.probe_b" {
		t.Fatalf("executed tasks = %v, want [euclo:cap.probe_b] (t1 resumed)", order)
	}
	if completed, _ := contextdata.GetTyped[int](env, "htn.tasks_completed"); completed != 2 {
		t.Fatalf("htn.tasks_completed = %d, want 2", completed)
	}
}

// TestAuthoredMethodValidationErrors proves a spec-invalid authored method is an
// option-construction error.
func TestAuthoredMethodValidationErrors(t *testing.T) {
	if _, err := WithAuthoredMethod("", []AuthoredTask{{Text: "x"}}); err == nil {
		t.Fatal("expected an error for an empty method name")
	}
	if _, err := WithAuthoredMethod("m", nil); err == nil {
		t.Fatal("expected an error for an empty task set")
	}
	if _, err := WithAuthoredMethod("m", []AuthoredTask{{Text: "x", Capability: "two words"}}); err == nil {
		t.Fatal("expected an error for an executor with whitespace")
	}
	if _, err := WithAuthoredMethod("m", []AuthoredTask{{}}); err == nil {
		t.Fatal("expected an error for a task without text")
	}
	if _, err := WithAuthoredMethod("m", []AuthoredTask{{Text: "x"}, {Text: "y"}}); err != nil {
		t.Fatalf("valid authored method rejected: %v", err)
	}
}

// TestHTNZeroOptionsUnchanged pins FR-9: an agent with no directive options
// keeps the library method-lookup path (the explain method decomposes through
// the react primitive, and no authored result fields are recorded).
func TestHTNZeroOptionsUnchanged(t *testing.T) {
	reg := registry.NewRegistry()
	primitive := &htnRecordingPrimitive{}
	deps := &paradigm.Deps{Registry: reg, Config: &execution.Config{Name: "htn-test"}}
	agent := New(deps, runtime.NewMethodLibrary(), WithPrimitiveExec(primitive))

	env := contextdata.NewEnvelope("task", "session")
	if _, err := agent.Execute(context.Background(), htnTask(), env); err != nil {
		t.Fatalf("execute: %v", err)
	}
	if got := primitive.ids(); len(got) == 0 {
		t.Fatal("library mode must decompose through the primitive executor")
	}
	if _, ok := contextdata.GetTyped[string](env, "htn.method"); ok {
		t.Fatal("library mode must not record the authored method field")
	}
}
