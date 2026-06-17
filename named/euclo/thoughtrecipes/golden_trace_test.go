package thoughtrecipe

import (
	"context"
	"encoding/json"
	"os"
	"sort"
	"strings"
	"sync"
	"testing"

	regpkg "codeburg.org/lexbit/relurpify/capability/registry"
	"codeburg.org/lexbit/relurpify/capability/descriptor"
	"codeburg.org/lexbit/relurpify/capability/agentspec"
	"codeburg.org/lexbit/relurpify/capability/ports"
	"codeburg.org/lexbit/relurpify/cognitionzoo/paradigm"
	"codeburg.org/lexbit/relurpify/context/contextdata"
	"codeburg.org/lexbit/relurpify/execution"
	"codeburg.org/lexbit/relurpify/execution/agentgraph"
	"codeburg.org/lexbit/relurpify/telemetry"
)

func TestGoldenTraces(t *testing.T) {
	recipes := loadGoldenRecipes(t)
	for name, doc := range recipes {
		t.Run(name, func(t *testing.T) {
			plan, err := LowerDocument(doc)
			if err != nil {
				t.Fatalf("LowerDocument(%s) failed: %v", name, err)
			}

			deps, reg := minimalTraceDeps(t)
			ctx := context.Background()
			goldenPath := goldenPath(t, name, "trace.json")
			trace := captureTrace(t, ctx, plan, deps, reg)

			if update := os.Getenv("UPDATE_GOLDEN"); update != "" {
				if err := os.WriteFile(goldenPath, trace, 0o644); err != nil {
					t.Fatalf("write golden %s: %v", goldenPath, err)
				}
				return
			}

			want, err := os.ReadFile(goldenPath)
			if err != nil {
				t.Fatalf("read golden %s: %v (set UPDATE_GOLDEN=1 to create)", goldenPath, err)
			}
			if string(trace) != string(want) {
				t.Fatalf("golden trace mismatch for %s\ngot:\n%s\nwant:\n%s\n(set UPDATE_GOLDEN=1 to re-baseline)",
					name, string(trace), string(want))
			}
		})
	}
}

func TestGoldenTraceFailsOnMutation(t *testing.T) {
	doc := mustParseDoc(t, `thoughtrecipe mutate_trace
"Mutation test."

trigger as capability:
  may read workspace

agent default uses react

run default:
  do relurpic:test_cap on input.workspace
`)
	plan, err := LowerDocument(doc)
	if err != nil {
		t.Fatalf("LowerDocument failed: %v", err)
	}
	deps, reg := minimalTraceDeps(t)
	ctx := context.Background()
	original := captureTrace(t, ctx, plan, deps, reg)

	reg.RegisterInvocableCapability(ctx, &stubCapHandler{
		id: "euclo:cap.test_cap",
		fn: func(ctx context.Context, env ports.State, args map[string]any) (*ports.ToolResult, error) {
			return &ports.ToolResult{Success: true, Data: map[string]any{"mutated": true}}, nil
		},
	})
	mutated := captureTrace(t, ctx, plan, deps, reg)
	if string(original) == string(mutated) {
		t.Fatal("mutation test: mutated trace should differ from original")
	}
}

// traceResult captures the observable output of a single graph execution.
type traceResult struct {
	GraphValid   bool              `json:"graph_valid"`
	StartNode    string            `json:"start_node"`
	NodeCount    int               `json:"node_count"`
	EdgeCount    int               `json:"edge_count"`
	ExecPath     []string          `json:"exec_path"`
	ExecSuccess  bool              `json:"exec_success"`
	ExecError    string            `json:"exec_error,omitempty"`
	EnvelopeKeys []string          `json:"envelope_keys"`
	ResultKeys   []string          `json:"result_keys"`
}

type recordingTelemetrySink struct {
	mu     sync.Mutex
	events []telemetry.Event
}

func (s *recordingTelemetrySink) Emit(event telemetry.Event) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.events = append(s.events, event)
}

func (s *recordingTelemetrySink) Events() []telemetry.Event {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]telemetry.Event, len(s.events))
	copy(out, s.events)
	return out
}

func captureTrace(t *testing.T, ctx context.Context, plan *ExecutionPlan, deps *paradigm.Deps, reg *regpkg.CapabilityRegistry) []byte {
	t.Helper()

	graph, err := BuildThoughtRecipeGraph(plan, deps, nil)
	if err != nil {
		t.Fatalf("BuildThoughtRecipeGraph failed: %v", err)
	}

	valid := graph.Validate() == nil

	telemetrySink := &recordingTelemetrySink{}
	graph.SetTelemetry(telemetrySink)

	env := contextdata.NewEnvelope("golden-task-"+plan.ThoughtRecipe.Name, "golden-session")
	env.SetWorkingValueWithClass("euclo.execution.step_total", len(plan.Steps), contextdata.MemoryClassTask)

	result, execErr := graph.Execute(ctx, env)

	execSuccess := execErr == nil && result != nil && result.Success
	execError := ""
	if execErr != nil {
		execError = execErr.Error()
	} else if result != nil && !result.Success {
		execError = result.Error
	}

	var execPath []string
	for _, ev := range telemetrySink.Events() {
		if ev.Type == telemetry.EventNodeStart {
			execPath = append(execPath, ev.NodeID)
		}
	}

	envKeys := sortedEnvKeys(env)
	resultKeys := filterResultKeys(envKeys)

	trace := traceResult{
		GraphValid:   valid,
		StartNode:    graph.StartNodeID(),
		NodeCount:    len(graph.NodeIDs()),
		EdgeCount:    countEdges(graph),
		ExecPath:     execPath,
		ExecSuccess:  execSuccess,
		ExecError:    execError,
		EnvelopeKeys: envKeys,
		ResultKeys:   resultKeys,
	}
	data, err := json.MarshalIndent(trace, "", "  ")
	if err != nil {
		t.Fatalf("json marshal trace: %v", err)
	}
	return data
}

func countEdges(g *agentgraph.Graph) int {
	var count int
	for _, id := range g.NodeIDs() {
		count += len(g.OutgoingEdges(id))
	}
	return count
}

func sortedEnvKeys(env *contextdata.Envelope) []string {
	snap := env.Snapshot()
	keys := make([]string, 0, len(snap))
	for k := range snap {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func filterResultKeys(keys []string) []string {
	var out []string
	for _, k := range keys {
		if strings.Contains(k, "euclo.execution.step.") {
			out = append(out, k)
		}
	}
	sort.Strings(out)
	return out
}

// stubCapHandler implements capabilty.registry.InvocableCapability.
type stubCapHandler struct {
	id string
	fn func(ctx context.Context, env ports.State, args map[string]any) (*ports.ToolResult, error)
}

func (h *stubCapHandler) Descriptor(ctx context.Context, env ports.State) descriptor.CapabilityDescriptor {
	return descriptor.CapabilityDescriptor{
		ID:            h.id,
		Name:          h.id,
		Kind:          agentspec.CapabilityKindTool,
		RuntimeFamily: agentspec.CapabilityRuntimeFamilyRelurpic,
		Availability:  descriptor.AvailabilitySpec{Available: true},
	}
}

func (h *stubCapHandler) Invoke(ctx context.Context, env ports.State, args map[string]any) (*ports.ToolResult, error) {
	if h.fn != nil {
		return h.fn(ctx, env, args)
	}
	return &ports.ToolResult{
		Success: true,
		Data:    map[string]any{"answer": "stub"},
	}, nil
}

func minimalTraceDeps(t *testing.T) (*paradigm.Deps, *regpkg.CapabilityRegistry) {
	t.Helper()
	reg := regpkg.NewRegistry()
	deps := &paradigm.Deps{
		Config: &execution.Config{
			Name:  "golden-trace",
			Model: "offline",
		},
		Registry: reg,
	}
	return deps, reg
}
