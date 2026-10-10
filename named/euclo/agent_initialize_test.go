package euclo

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"codeburg.org/lexbit/relurpify/capability/agentspec"
	registry "codeburg.org/lexbit/relurpify/capability/registry"
	"codeburg.org/lexbit/relurpify/cognitionzoo/paradigm"
	"codeburg.org/lexbit/relurpify/context/contextdata"
	execution "codeburg.org/lexbit/relurpify/execution"
	"codeburg.org/lexbit/relurpify/named/euclo/grounding"
	telemetry "codeburg.org/lexbit/relurpify/telemetry"
	"codeburg.org/lexbit/relurpify/testsuite/testhelper"
	"codeburg.org/lexbit/relurpify/testsuite/testsupport"
)

var testRelurpicCapabilities = []string{ //nolint:gochecknoglobals // test fixture data
	"euclo:cap.test_run",
	"euclo:cap.ast_query",
	"euclo:cap.symbol_trace",
	"euclo:cap.call_graph",
	"euclo:cap.blame_trace",
	"euclo:cap.bisect",
	"euclo:cap.code_review",
	"euclo:cap.diff_summary",
	"euclo:cap.targeted_refactor",
	"euclo:cap.rename_symbol",
	"euclo:cap.api_compat",
	"euclo:cap.coverage_check",
}

func TestAgentInitializeDoesNotPanic(t *testing.T) {
	workspace := writeRecipeWorkspace(t)
	deps := &paradigm.Deps{
		Config: &execution.Config{
			AgentSpec: &agentspec.AgentRuntimeSpec{
				Capabilities: agentspec.AgentCapabilitiesSpec{Relurpic: append([]string{}, testRelurpicCapabilities...)},
			},
		},
		Registry: registry.NewRegistry(),
	}

	agent := New(deps)

	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("agent.Initialize panicked: %v", r)
		}
	}()

	err := initializeAgentIn(t, agent, workspace)
	if err != nil {
		t.Fatalf("agent.Initialize failed: %v", err)
	}

	if !agent.initialized {
		t.Fatal("agent should be initialized after Initialize call")
	}

	if agent.thoughtrecipeRegistry == nil {
		t.Fatal("thoughtrecipeRegistry should be set after Initialize")
	}
}

func TestAgentInitializeWithNilRegistry(t *testing.T) {
	deps := &paradigm.Deps{
		Registry: nil,
	}

	agent := New(deps)

	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("agent.Initialize with nil registry panicked: %v", r)
		}
	}()

	err := agent.Initialize(nil)
	if err == nil {
		t.Fatal("expected error when Registry is nil")
	}
}

// TestGroundingPortsUnwiredEmittedOnceForColdStart is AC-20's nil mode at boot:
// exactly one info-level grounding.ports_unwired event appears when no
// StateReground source is composed, and Initialize emits it only once.
func TestGroundingPortsUnwiredEmittedOnceForColdStart(t *testing.T) {
	sink := &recordingTelemetrySink{}
	deps := &paradigm.Deps{
		Config: &execution.Config{
			AgentSpec: &agentspec.AgentRuntimeSpec{
				Capabilities: agentspec.AgentCapabilitiesSpec{Relurpic: append([]string{}, testRelurpicCapabilities...)},
			},
		},
		Registry:  registry.NewRegistry(),
		Telemetry: sink,
	}
	agent := New(deps, WithHITLBroker(testsupport.NewAutoApprovingBroker()), WithInteractionResolver(testhelper.NewPermissiveResolver()))

	workspace := writeRecipeWorkspace(t)
	if err := initializeAgentIn(t, agent, workspace); err != nil {
		t.Fatalf("Initialize: %v", err)
	}
	if err := initializeAgentIn(t, agent, workspace); err != nil {
		t.Fatalf("second Initialize: %v", err)
	}

	if got := sink.count(telemetry.EventGroundingPortsUnwired); got != 1 {
		t.Fatalf("grounding.ports_unwired events = %d, want exactly 1", got)
	}
	if got := agent.GroundingComposition(); got != "cold_start" {
		t.Fatalf("GroundingComposition = %q, want cold_start", got)
	}
}

// TestGroundingPortsUnwiredAbsentWhenWired covers the composed source: no boot
// event and the status surface reports wired.
func TestGroundingPortsUnwiredAbsentWhenWired(t *testing.T) {
	sink := &recordingTelemetrySink{}
	deps := &paradigm.Deps{
		Config: &execution.Config{
			AgentSpec: &agentspec.AgentRuntimeSpec{
				Capabilities: agentspec.AgentCapabilitiesSpec{Relurpic: append([]string{}, testRelurpicCapabilities...)},
			},
		},
		Registry:  registry.NewRegistry(),
		Telemetry: sink,
	}
	agent := New(deps,
		WithConfig(EucloConfig{StateReground: &fakeRegroundSource{}}),
		WithHITLBroker(testsupport.NewAutoApprovingBroker()), WithInteractionResolver(testhelper.NewPermissiveResolver()),
	)

	if err := initializeAgentIn(t, agent, writeRecipeWorkspace(t)); err != nil {
		t.Fatalf("Initialize: %v", err)
	}
	if got := sink.count(telemetry.EventGroundingPortsUnwired); got != 0 {
		t.Fatalf("wired grounding must not emit ports_unwired, got %d", got)
	}
	if got := agent.GroundingComposition(); got != "wired" {
		t.Fatalf("GroundingComposition = %q, want wired", got)
	}
}

// fakeRegroundSource satisfies grounding.StateRegroundSource for boot-wiring
// tests without touching the knowledge layer.
type fakeRegroundSource struct{}

func (fakeRegroundSource) Reground(context.Context, grounding.RegroundRequest) (grounding.RegroundResult, error) {
	return grounding.RegroundResult{Grounded: false}, nil
}

// recordingTelemetrySink captures emitted telemetry in memory.
type recordingTelemetrySink struct {
	mu     sync.Mutex
	events []telemetry.Event
}

func (s *recordingTelemetrySink) Emit(event telemetry.Event) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.events = append(s.events, event)
}

func (s *recordingTelemetrySink) count(eventType telemetry.EventType) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	count := 0
	for _, event := range s.events {
		if event.Type == eventType {
			count++
		}
	}
	return count
}

// TestInitializeResolvesWorkspaceFromDeps is FR-1: with no config override,
// the recipe workspace comes from the embedding (IndexManager.WorkspacePath),
// and the loaded registry is observable via ThoughtRecipeIDs.
func TestInitializeResolvesWorkspaceFromDeps(t *testing.T) {
	workspace := writeRecipeWorkspace(t)
	deps := &paradigm.Deps{
		Config: &execution.Config{
			AgentSpec: &agentspec.AgentRuntimeSpec{
				Capabilities: agentspec.AgentCapabilitiesSpec{Relurpic: append([]string{}, testRelurpicCapabilities...)},
			},
		},
		Registry:     registry.NewRegistry(),
		IndexManager: newTestIndexManager(t, workspace),
	}
	agent := New(deps)

	if err := agent.Initialize(nil); err != nil {
		t.Fatalf("Initialize: %v", err)
	}
	ids := agent.ThoughtRecipeIDs()
	if len(ids) != 1 || ids[0] != "euclo.thoughtrecipe.probe" {
		t.Fatalf("ThoughtRecipeIDs = %v, want [euclo.thoughtrecipe.probe]", ids)
	}
}

// TestInitializeConfigWorkspaceOverridesDeps: the explicit config workspace
// wins over the embedding workspace.
func TestInitializeConfigWorkspaceOverridesDeps(t *testing.T) {
	depsWorkspace := writeRecipeWorkspace(t)
	overrideWorkspace := writeRecipeWorkspace(t)
	deps := &paradigm.Deps{
		Config: &execution.Config{
			AgentSpec: &agentspec.AgentRuntimeSpec{
				Capabilities: agentspec.AgentCapabilitiesSpec{Relurpic: append([]string{}, testRelurpicCapabilities...)},
			},
		},
		Registry:     registry.NewRegistry(),
		IndexManager: newTestIndexManager(t, depsWorkspace),
	}
	agent := New(deps)

	if err := agent.Initialize(&execution.Config{Workspace: overrideWorkspace}); err != nil {
		t.Fatalf("Initialize: %v", err)
	}
	// Both workspaces carry the same probe recipe, so resolve from the
	// override path: corrupt the override's recipe and re-initialize a fresh
	// agent to prove the override directory is the one read.
	if err := os.WriteFile(
		filepath.Join(overrideWorkspace, "relurpify_cfg", "euclo", "broken.erpe"),
		[]byte("thoughtrecipe broken\n"), 0o644,
	); err != nil {
		t.Fatal(err)
	}
	agent2 := New(deps)
	err := agent2.Initialize(&execution.Config{Workspace: overrideWorkspace})
	if err == nil {
		t.Fatal("expected Initialize to read the override workspace (broken recipe must fail the load)")
	}
}

// TestInitializeMissingRecipeDirErrors: a resolved workspace without
// relurpify_cfg/euclo is a boot error with the contract prefix, never a
// silently empty registry.
func TestInitializeMissingRecipeDirErrors(t *testing.T) {
	deps := &paradigm.Deps{
		Config: &execution.Config{
			AgentSpec: &agentspec.AgentRuntimeSpec{
				Capabilities: agentspec.AgentCapabilitiesSpec{Relurpic: append([]string{}, testRelurpicCapabilities...)},
			},
		},
		Registry:     registry.NewRegistry(),
		IndexManager: newTestIndexManager(t, t.TempDir()),
	}
	agent := New(deps)

	err := agent.Initialize(nil)
	if err == nil {
		t.Fatal("expected error for workspace without relurpify_cfg/euclo")
	}
	if !strings.HasPrefix(err.Error(), "euclo: initialize: no recipes:") {
		t.Fatalf("error %q does not carry the contract prefix", err)
	}
}

// TestExecuteUninitializedErrors and TestBuildGraphUninitializedErrors: D-8
// removed the lazy Initialize(nil) path — an uninitialized agent fails loudly.
func TestExecuteUninitializedErrors(t *testing.T) {
	agent := New(&paradigm.Deps{Registry: registry.NewRegistry()})
	_, err := agent.Execute(context.Background(), &execution.Task{ID: "t"}, contextdata.NewEnvelope("t", "s"))
	if err == nil || !strings.Contains(err.Error(), "euclo agent not initialized") {
		t.Fatalf("Execute on uninitialized agent = %v, want not-initialized error", err)
	}
}

func TestBuildGraphUninitializedErrors(t *testing.T) {
	agent := New(&paradigm.Deps{Registry: registry.NewRegistry()})
	_, err := agent.BuildGraph(context.Background(), &execution.Task{ID: "t"})
	if err == nil || !strings.Contains(err.Error(), "euclo agent not initialized") {
		t.Fatalf("BuildGraph on uninitialized agent = %v, want not-initialized error", err)
	}
}
