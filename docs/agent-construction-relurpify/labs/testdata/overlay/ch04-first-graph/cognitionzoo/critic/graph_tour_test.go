package critic_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"codeburg.org/lexbit/relurpify/capability/descriptor"
	"codeburg.org/lexbit/relurpify/capability/ports"
	"codeburg.org/lexbit/relurpify/capability/registry"
	capresult "codeburg.org/lexbit/relurpify/capability/result"
	"codeburg.org/lexbit/relurpify/cognitionzoo/critic"
	"codeburg.org/lexbit/relurpify/context/contextdata"
	"codeburg.org/lexbit/relurpify/execution"
	agentgraph "codeburg.org/lexbit/relurpify/execution/agentgraph"
)

// stringkitSource is the file the fake tool "reads" — stringkit.py, as
// reset for book 2. Scripted, not read from disk: the graph tour must be
// deterministic on every machine.
const stringkitSource = `def slugify(text: str) -> str:
    slug = text.strip().lower()
    slug = re.sub(r"[^\w\s]+", "", slug)
    slug = re.sub(r"\s+", "-", slug)
    return slug
`

// stringkitReadTool is a hand-written ports.Tool: eight small methods, one
// of which does the work. It records its own executions so tests can prove
// the call went through the governed path — the real capability registry —
// and into the tool.
type stringkitReadTool struct {
	calls int
}

func (t *stringkitReadTool) Name() string        { return "read_stringkit" }
func (t *stringkitReadTool) Description() string { return "read the stringkit demo source" }
func (t *stringkitReadTool) Category() string    { return "filesystem" }
func (t *stringkitReadTool) Parameters() []ports.ToolParameter {
	return []ports.ToolParameter{{Name: "path", Type: "string", Required: true}}
}
func (t *stringkitReadTool) Execute(_ context.Context, _ map[string]any) (*ports.ToolResult, error) {
	t.calls++
	return &ports.ToolResult{
		Success: true,
		Data:    map[string]any{"content": stringkitSource},
	}, nil
}
func (t *stringkitReadTool) IsAvailable(context.Context) bool { return true }
func (t *stringkitReadTool) Permissions() ports.ToolPermissions {
	return ports.ToolPermissions{}
}
func (t *stringkitReadTool) Tags() []string { return []string{"demo", "read-only"} }

// governedInvoker adapts the real capability registry to the graph's
// narrow CapabilityInvoker. It forwards — it never re-implements: every
// invocation goes through the registry's full pipeline (admission, policy
// evaluation, schema validation, insertion decisions), exactly as it does
// in production. The book never teaches an ungoverned path, not even in a
// test.
type governedInvoker struct {
	reg *registry.CapabilityRegistry
}

func (g governedInvoker) InvokeCapability(ctx context.Context, env *contextdata.Envelope, idOrName string, args map[string]any) (*ports.ToolResult, error) {
	return g.reg.InvokeCapability(ctx, env.State(), idOrName, args)
}
func (g governedInvoker) CapturePolicySnapshot() *capresult.PolicySnapshot {
	return g.reg.CapturePolicySnapshot()
}
func (g governedInvoker) GetCapability(idOrName string) (descriptor.CapabilityDescriptor, bool) {
	return g.reg.GetCapability(idOrName)
}

// draftNode is a hand-implemented agentgraph.Node — the three-method
// interface from this chapter — driven by the fake model.
type draftNode struct {
	id    string
	model *critic.FakeModel
}

func (n *draftNode) ID() string                { return n.id }
func (n *draftNode) Type() agentgraph.NodeType { return agentgraph.NodeTypeObservation }

func (n *draftNode) Execute(ctx context.Context, env *contextdata.Envelope) (*execution.Result, error) {
	ch, err := n.model.GenerateStream(ctx, "draft a plan against stringkit", nil)
	if err != nil {
		return nil, err
	}
	var b strings.Builder
	for chunk := range ch {
		b.WriteString(chunk)
	}
	draft := b.String()
	env.SetWorkingValueWithOrigin("critic.draft", draft,
		contextdata.MemoryClassTask, contextdata.OriginLLM)
	return &execution.Result{
		NodeID:  n.id,
		Success: true,
		Data:    execution.NewToolResultPayload(map[string]any{"draft": draft}),
	}, nil
}

func newTourEnvelope() *contextdata.Envelope {
	env := &contextdata.Envelope{}
	env.SetTaskID("tour-graph")
	return env
}

// TestFirstGraphRuns builds intake -> drafter -> done and executes it.
func TestFirstGraphRuns(t *testing.T) {
	fm := critic.NewFakeModel(critic.FakeTurn{Chunks: []string{"read ", "slugify, ", "then critique"}})
	drafter := &draftNode{id: "drafter", model: fm}

	intake := agentgraph.NewSystemNode("intake", func(_ context.Context, env *contextdata.Envelope) (*execution.Result, error) {
		env.SetWorkingValueWithOrigin("intake.message", "system: draft against stringkit",
			contextdata.MemoryClassTask, contextdata.OriginTool)
		return &execution.Result{Success: true}, nil
	})

	g := agentgraph.NewGraph()
	mustBuild(t,
		g.AddNode(intake),
		g.AddNode(drafter),
		g.AddNode(agentgraph.NewTerminalNode("done")),
		g.AddEdge("intake", "drafter", nil, false),
		g.AddEdge("drafter", "done", nil, false),
		g.SetStart("intake"),
	)

	env := newTourEnvelope()
	result, err := g.Execute(context.Background(), env)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if result == nil || !result.Success {
		t.Fatalf("result = %+v, want success", result)
	}

	// The node wrote its own envelope value...
	if draft, ok := contextdata.GetTyped[string](env, "critic.draft"); !ok || draft != "read slugify, then critique" {
		t.Fatalf("critic.draft = %q, %v", draft, ok)
	}
	// ...and the runtime captured the result fields under "<node>.<key>".
	if captured, ok := contextdata.GetTyped[string](env, "drafter.draft"); !ok || captured != "read slugify, then critique" {
		t.Fatalf("drafter.draft = %q, %v", captured, ok)
	}
	if fm.Calls() != 1 {
		t.Fatalf("model calls = %d, want 1", fm.Calls())
	}

	// Preflight with no capability catalog wired: a report with nothing to
	// say. Its teeth arrive with a catalog (TestPreflightGatesToolPlacement).
	report, err := g.Preflight()
	if err != nil {
		t.Fatalf("Preflight: %v", err)
	}
	if len(report.Issues) != 0 || len(report.Placements) != 0 {
		t.Fatalf("preflight = %+v, want empty without a catalog", report)
	}
}

// TestConditionalEdgesRoutes shows a ConditionFunc choosing between two
// serial edges.
func TestConditionalEdgesRoutes(t *testing.T) {
	fm := critic.NewFakeModel(critic.FakeTurn{Chunks: []string{"draft mentions slugify"}})
	drafter := &draftNode{id: "drafter", model: fm}

	polish := agentgraph.NewSystemNode("polish", func(_ context.Context, env *contextdata.Envelope) (*execution.Result, error) {
		env.SetWorkingValueWithOrigin("critic.polish", "tightened",
			contextdata.MemoryClassTask, contextdata.OriginTool)
		return &execution.Result{Success: true}, nil
	})

	g := agentgraph.NewGraph()
	mustBuild(t,
		g.AddNode(drafter),
		g.AddNode(polish),
		g.AddNode(agentgraph.NewTerminalNode("done")),
		g.AddEdge("drafter", "polish", func(_ *execution.Result, env *contextdata.Envelope) bool {
			draft, ok := contextdata.GetTyped[string](env, "critic.draft")
			return ok && strings.Contains(draft, "slugify")
		}, false),
		g.AddEdge("drafter", "done", func(_ *execution.Result, env *contextdata.Envelope) bool {
			draft, ok := contextdata.GetTyped[string](env, "critic.draft")
			return !ok || !strings.Contains(draft, "slugify")
		}, false),
		g.AddEdge("polish", "done", nil, false),
		g.SetStart("drafter"),
	)

	env := newTourEnvelope()
	if _, err := g.Execute(context.Background(), env); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if _, ok := contextdata.GetTyped[string](env, "critic.polish"); !ok {
		t.Fatal("condition did not route to the polish branch")
	}
}

// TestGraphSealsOnExecute proves the two-phase lifecycle: mutation is
// refused after Execute, but Execute itself may run again.
func TestGraphSealsOnExecute(t *testing.T) {
	fm := critic.NewFakeModel(critic.FakeTurn{Chunks: []string{"once"}})
	g := agentgraph.NewGraph()
	mustBuild(t,
		g.AddNode(&draftNode{id: "drafter", model: fm}),
		g.AddNode(agentgraph.NewTerminalNode("done")),
		g.AddEdge("drafter", "done", nil, false),
		g.SetStart("drafter"),
	)

	env := newTourEnvelope()
	if _, err := g.Execute(context.Background(), env); err != nil {
		t.Fatalf("first Execute: %v", err)
	}
	if err := g.AddNode(agentgraph.NewTerminalNode("late")); !errors.Is(err, agentgraph.ErrGraphSealed) {
		t.Fatalf("post-seal AddNode err = %v, want ErrGraphSealed", err)
	}
	if err := g.SetStart("late"); !errors.Is(err, agentgraph.ErrGraphSealed) {
		t.Fatalf("post-seal SetStart err = %v, want ErrGraphSealed", err)
	}
	// Sealing governs mutation, not execution.
	if _, err := g.Execute(context.Background(), newTourEnvelope()); err != nil {
		t.Fatalf("second Execute: %v", err)
	}
}

// TestVisitCapCatchesCycle shows the cycle guard refusing an accidental
// loop instead of hanging.
func TestVisitCapCatchesCycle(t *testing.T) {
	fm := critic.NewFakeModel(critic.FakeTurn{Chunks: []string{"loop"}})
	a := &draftNode{id: "a", model: fm}
	b := &draftNode{id: "b", model: fm}

	g := agentgraph.NewGraph()
	mustBuild(t,
		g.AddNode(a),
		g.AddNode(b),
		g.AddEdge("a", "b", nil, false),
		g.AddEdge("b", "a", nil, false),
		g.SetStart("a"),
		g.SetMaxNodeVisits(2),
	)

	_, err := g.Execute(context.Background(), newTourEnvelope())
	if err == nil || !strings.Contains(err.Error(), "potential cycle detected") {
		t.Fatalf("err = %v, want potential cycle detected", err)
	}
}

// TestValidationCatchesBrokenWiring collects the graph's structural
// refusals before anything executes.
func TestValidationCatchesBrokenWiring(t *testing.T) {
	t.Run("start node must exist", func(t *testing.T) {
		g := agentgraph.NewGraph()
		mustBuild(t, g.AddNode(agentgraph.NewTerminalNode("done")))
		if err := g.SetStart("missing"); err == nil || !strings.Contains(err.Error(), "not found") {
			t.Fatalf("err = %v, want not found", err)
		}
	})
	t.Run("edges must reference defined nodes", func(t *testing.T) {
		g := agentgraph.NewGraph()
		mustBuild(t, g.AddNode(agentgraph.NewTerminalNode("done")))
		if err := g.AddEdge("done", "ghost", nil, false); err == nil || !strings.Contains(err.Error(), "not defined") {
			t.Fatalf("err = %v, want not defined", err)
		}
	})
	t.Run("execute without a start fails validation", func(t *testing.T) {
		g := agentgraph.NewGraph()
		mustBuild(t, g.AddNode(agentgraph.NewTerminalNode("done")))
		if _, err := g.Execute(context.Background(), newTourEnvelope()); err == nil || !strings.Contains(err.Error(), "no start node") {
			t.Fatalf("err = %v, want no start node", err)
		}
	})
}

// TestPreflightGatesToolPlacement shows the preflight report earning its
// name: a ToolNode's contract requires its capability, and the real
// capability registry — as both catalog and invoker — decides whether the
// graph may run.
func TestPreflightGatesToolPlacement(t *testing.T) {
	tool := &stringkitReadTool{}
	ctx := context.Background()
	desc := descriptor.ToolDescriptor(ctx, tool)

	t.Run("no catalog wired: no placement opinions", func(t *testing.T) {
		g := toolGraph(t, tool, nil)
		report, err := g.Preflight()
		if err != nil || len(report.Issues) != 0 || len(report.Placements) != 0 {
			t.Fatalf("report = %+v, err = %v, want empty", report, err)
		}
	})
	t.Run("empty registry: blocking capability_missing", func(t *testing.T) {
		g := toolGraph(t, tool, governedRegistry(t, false))
		_, err := g.Preflight()
		if err == nil || !strings.Contains(err.Error(), "no registered capability matches selector") {
			t.Fatalf("err = %v, want capability_missing", err)
		}
		if _, err := g.Execute(ctx, newTourEnvelope()); err == nil {
			t.Fatal("Execute should refuse to run past a blocking preflight")
		}
	})
	t.Run("registered tool: placement decided, invocation governed", func(t *testing.T) {
		reg := registry.NewRegistry()
		mustBuild(t, reg.RegisterLegacyTool(ctx, tool))
		g := toolGraph(t, tool, reg)
		report, err := g.Preflight()
		if err != nil {
			t.Fatalf("Preflight: %v", err)
		}
		if len(report.Placements) != 1 || report.Placements[0].SelectedCapabilityID != desc.ID {
			t.Fatalf("placements = %+v, want one for %q", report.Placements, desc.ID)
		}
		if report.Placements[0].Reason == "" {
			t.Fatal("placement decision should carry its reason")
		}
		if _, err := g.Execute(ctx, newTourEnvelope()); err != nil {
			t.Fatalf("Execute: %v", err)
		}
		if tool.calls != 1 {
			t.Fatalf("tool executions = %d, want 1 through the real registry", tool.calls)
		}
	})
}

// toolGraph builds reader(ToolNode) -> done with the given catalog. The
// invoker is always the governed adapter over the same real registry.
func toolGraph(t *testing.T, tool ports.Tool, cat agentgraph.CapabilityCatalog) *agentgraph.Graph {
	t.Helper()
	reg := registry.NewRegistry()
	if c, ok := cat.(*registry.CapabilityRegistry); ok {
		reg = c
	}
	g := agentgraph.NewGraph()
	mustBuild(t,
		g.AddNode(agentgraph.NewToolNode("reader", tool, map[string]any{"path": "stringkit.py"}, governedInvoker{reg: reg})),
		g.AddNode(agentgraph.NewTerminalNode("done")),
		g.AddEdge("reader", "done", nil, false),
		g.SetStart("reader"),
	)
	if cat != nil {
		mustBuild(t, g.SetCapabilityCatalog(cat))
	}
	return g
}

// governedRegistry returns a real capability registry. With withTool it
// registers the same tool instance the node holds, so tests can observe
// the governed invocation landing in the tool. This is the framework's own
// unit-test pattern (see capability/registry/invoke_test.go): NewRegistry
// plus RegisterLegacyTool — never a hand-rolled invoker.
func governedRegistry(t *testing.T, withTool bool) *registry.CapabilityRegistry {
	t.Helper()
	reg := registry.NewRegistry()
	if withTool {
		if err := reg.RegisterLegacyTool(context.Background(), &stringkitReadTool{}); err != nil {
			t.Fatalf("RegisterLegacyTool: %v", err)
		}
	}
	return reg
}

func mustBuild(t *testing.T, errs ...error) {
	t.Helper()
	for _, err := range errs {
		if err != nil {
			t.Fatalf("build step: %v", err)
		}
	}
}
