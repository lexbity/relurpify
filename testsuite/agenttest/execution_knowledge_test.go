package agenttest

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"testing"

	"codeburg.org/lexbit/relurpify/capability/agentspec"
	"codeburg.org/lexbit/relurpify/capability/descriptor"
	"codeburg.org/lexbit/relurpify/capability/ports"
	"codeburg.org/lexbit/relurpify/context/contextdata"
	"codeburg.org/lexbit/relurpify/named/euclo/thoughtrecipes"
)

func TestExecutorBuildsKnowledgeRuntime(t *testing.T) {
	ws := t.TempDir()
	desc := validDescriptorWithWorkspace(t, ws)
	exec := (&PreparedRunExecutor{}).WithRunnerOverride(fakeRunner{})

	if err := exec.Execute(context.Background(), desc, io.Discard); err != nil {
		t.Fatalf("Execute: %v", err)
	}

	if exec.knowledge == nil {
		t.Fatal("knowledge runtime is nil after Execute")
	}
	if exec.knowledge.KnowledgeStore == nil {
		t.Fatal("knowledge.KnowledgeStore is nil")
	}
	if exec.knowledge.KnowledgeEvents == nil {
		t.Fatal("knowledge.KnowledgeEvents is nil")
	}
	if exec.knowledge.Retriever == nil {
		t.Fatal("knowledge.Retriever is nil")
	}
	if exec.knowledge.Compiler == nil {
		t.Fatal("knowledge.Compiler is nil")
	}
	if exec.knowledge.StreamTrigger == nil {
		t.Fatal("knowledge.StreamTrigger is nil")
	}
}

func TestExecutorKnowledgeReusesCapabilityGraphDB(t *testing.T) {
	ws := t.TempDir()
	desc := validDescriptorWithWorkspace(t, ws)
	exec := (&PreparedRunExecutor{}).WithRunnerOverride(fakeRunner{})

	if err := exec.Execute(context.Background(), desc, io.Discard); err != nil {
		t.Fatalf("Execute: %v", err)
	}

	capDB := exec.capability.IndexManager.GraphDB
	knowDB := exec.knowledge.KnowledgeStore.Graph

	if capDB == nil {
		t.Fatal("capability.IndexManager.GraphDB is nil")
	}
	if knowDB == nil {
		t.Fatal("knowledge.KnowledgeStore.Graph is nil")
	}
	if capDB != knowDB {
		t.Fatal("capability and knowledge GraphDB pointers differ — expected single shared engine")
	}
}

// harnessEchoCapability is the deterministic `do` target for the forward-pass
// case: it echoes the requested payload back.
type harnessEchoCapability struct{}

func (h *harnessEchoCapability) Descriptor(_ context.Context, _ ports.State) descriptor.CapabilityDescriptor {
	return descriptor.CapabilityDescriptor{
		ID:            "euclo:cap.harness_capture_echo",
		Name:          "euclo:cap.harness_capture_echo",
		Kind:          agentspec.CapabilityKindTool,
		RuntimeFamily: agentspec.CapabilityRuntimeFamilyProvider,
		Availability:  descriptor.AvailabilitySpec{Available: true},
	}
}

func (h *harnessEchoCapability) Invoke(_ context.Context, _ ports.State, _ map[string]any) (*ports.ToolResult, error) {
	return &ports.ToolResult{Success: true, Data: map[string]any{"output": "echoed"}}, nil
}

// TestHarnessForwardPassGroundsAtBarrier is the harness write boundary (D-5,
// FR-7): the executor's deps carry the grounding composition production uses
// (WireGrounding), and a capability-driven authored source running through the
// real recipe graph grounds its write into the harness's own GraphDB at the
// epoch barrier — no LLM involved.
func TestHarnessForwardPassGroundsAtBarrier(t *testing.T) {
	ws := t.TempDir()
	desc := validDescriptorWithWorkspace(t, ws)
	exec := (&PreparedRunExecutor{}).WithRunnerOverride(fakeRunner{})
	// Compose the harness stages directly (the same functions Execute runs)
	// so the GraphDB stays open for the assertion; Execute's deferred cleanup
	// closes it before a post-Execute check could observe grounded state.
	ctx := context.Background()
	if err := exec.buildSecurity(ctx, desc); err != nil {
		t.Fatalf("security: %v", err)
	}
	if err := exec.buildCapability(ctx, desc); err != nil {
		t.Fatalf("capability: %v", err)
	}
	if err := exec.buildKnowledge(); err != nil {
		t.Fatalf("knowledge: %v", err)
	}
	exec.telemetry = exec.buildTelemetry(desc)
	if err := exec.buildModel(ctx, desc); err != nil {
		t.Fatalf("model: %v", err)
	}

	deps := exec.assembleDeps(desc, exec.telemetry)
	if deps.Grounder == nil {
		t.Fatal("harness deps carry no grounding boundary — WireGrounding missing")
	}
	if deps.EpochDrain == nil {
		t.Fatal("harness deps carry no epoch drain — WireGrounding missing")
	}
	if err := exec.capability.Registry.RegisterInvocableCapability(context.Background(), &harnessEchoCapability{}); err != nil {
		t.Fatalf("register echo capability: %v", err)
	}

	src, err := os.ReadFile(filepath.Join("testdata", "harness_capture.erpe"))
	if err != nil {
		t.Fatal(err)
	}
	doc, err := thoughtrecipe.ParseSource("harness_capture.erpe", string(src))
	if err != nil {
		t.Fatalf("ParseSource: %v", err)
	}
	plan, err := thoughtrecipe.LowerDocument(doc)
	if err != nil {
		t.Fatalf("LowerDocument: %v", err)
	}
	graph, err := thoughtrecipe.BuildThoughtRecipeGraph(plan, deps, nil)
	if err != nil {
		t.Fatalf("BuildThoughtRecipeGraph: %v", err)
	}
	graph.SetGrounder(deps.Grounder)
	graph.SetDrain(deps.EpochDrain)

	env := contextdata.NewEnvelope("task-harness-capture", "session-harness-capture")
	env.SetWorkingValueWithClass("state.note", "harness-seed-note", contextdata.MemoryClassTask)
	if _, err := graph.Execute(ctx, env); err != nil {
		t.Fatalf("graph execute: %v", err)
	}

	all, err := exec.knowledge.KnowledgeStore.FindAll()
	if err != nil {
		t.Fatalf("FindAll: %v", err)
	}
	var grounded bool
	for _, chunk := range all {
		if key, _ := chunk.Body.Fields["state_key"].(string); key == "state.echo_out" {
			grounded = true
		}
	}
	if !grounded {
		t.Fatalf("no grounded chunk for state.echo_out in the harness GraphDB (%d chunks) — forward pass broken", len(all))
	}
}
