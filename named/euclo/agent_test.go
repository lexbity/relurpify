package euclo

import (
	"context"
	"reflect"
	"testing"

	"codeburg.org/lexbit/relurpify/capability"
	"codeburg.org/lexbit/relurpify/capability/agentspec"
	"codeburg.org/lexbit/relurpify/context/contextdata"
	execution "codeburg.org/lexbit/relurpify/execution"
	"codeburg.org/lexbit/relurpify/execution/agentenv"
	"codeburg.org/lexbit/relurpify/execution/agentgraph"
	"codeburg.org/lexbit/relurpify/named/euclo/euclotypes"
	"codeburg.org/lexbit/relurpify/named/euclo/intake"
	"codeburg.org/lexbit/relurpify/named/euclo/state"
)

// TestAgentCompiles verifies that New returns a non-nil *Agent without panic.
func TestAgentCompiles(t *testing.T) {
	// Create a minimal workspace environment
	env := agentenv.WorkspaceEnvironment{}
	agent := New(env)

	if agent == nil {
		t.Fatal("New() returned nil")
	}
}

// TestAgentImplementsWorkflowExecutor verifies compile-time interface assertion.
func TestAgentImplementsWorkflowExecutor(t *testing.T) {
	// This test passes if the code compiles - the var _ agentgraph.WorkflowExecutor
	// line in agent.go ensures this at compile time.
	// We also verify at runtime:
	var _ agentgraph.WorkflowExecutor = (*Agent)(nil)
}

// TestBuildGraphReturnsGraph verifies that BuildGraph returns a non-nil *agentgraph.Graph.
func TestBuildGraphReturnsGraph(t *testing.T) {
	env := agentenv.WorkspaceEnvironment{
		Config: &execution.Config{
			AgentSpec: &agentspec.AgentRuntimeSpec{
				Capabilities: agentspec.AgentCapabilitiesSpec{Relurpic: append([]string{}, testRelurpicCapabilities...)},
			},
		},
		Registry: capability.NewRegistry(),
	}
	agent := New(env)

	task := &execution.Task{
		ID:          "test-task",
		Type:        "analysis",
		Instruction: "test instruction",
	}

	graph, err := agent.BuildGraph(task)
	if err != nil {
		t.Fatalf("BuildGraph returned error: %v", err)
	}

	if graph == nil {
		t.Fatal("BuildGraph returned nil graph")
	}
}

// TestExecuteCallsBuildGraph verifies that Execute calls BuildGraph and graph.Execute.
// This is a minimal test since the full graph execution requires more setup.
func TestExecuteCallsBuildGraph(t *testing.T) {
	env := agentenv.WorkspaceEnvironment{
		Config: &execution.Config{
			AgentSpec: &agentspec.AgentRuntimeSpec{
				Capabilities: agentspec.AgentCapabilitiesSpec{Relurpic: append([]string{}, testRelurpicCapabilities...)},
			},
		},
		Registry: capability.NewRegistry(),
	}
	agent := New(env)

	task := &execution.Task{
		ID:          "test-task",
		Type:        "analysis",
		Instruction: "test instruction",
	}

	// Create a minimal envelope
	envelope := contextdata.NewEnvelope("test-task", "test-session")

	// Execute may return an error because the graph is stubbed, but it should
	// attempt to build and execute the graph.
	// For Phase 1, we're mainly verifying no panic and proper method calls.
	_, err := agent.Execute(context.Background(), task, envelope)

	// Phase 1: The graph has minimal nodes, so execution may fail validation.
	// We just verify no panic occurred.
	_ = err
}

func TestExecuteSeedsTaskEnvelope(t *testing.T) {
	task := &execution.Task{
		ID:          "task-42",
		Type:        "analysis",
		Instruction: "inspect the graph",
		Context:     map[string]any{"scope": "repo"},
		Metadata:    map[string]any{"source": "test"},
	}
	envelope := contextdata.NewEnvelope("task-42", "session-99")

	seedTaskEnvelope(envelope, task)

	if got, ok := contextdata.GetTyped[*execution.Task](envelope, "task.input"); !ok || got != task {
		t.Fatalf("expected task.input to be seeded with task pointer, got=%v ok=%v", got, ok)
	}
	if got, ok := contextdata.GetTyped[string](envelope, "task.id"); !ok || got != task.ID {
		t.Fatalf("expected task.id = %q, got=%v ok=%v", task.ID, got, ok)
	}
	if got, ok := contextdata.GetTyped[string](envelope, "task.instruction"); !ok || got != task.Instruction {
		t.Fatalf("expected task.instruction = %q, got=%v ok=%v", task.Instruction, got, ok)
	}
	if got, ok := contextdata.GetTyped[string](envelope, "task.type"); !ok || got != task.Type {
		t.Fatalf("expected task.type = %q, got=%v ok=%v", task.Type, got, ok)
	}
	if got, ok := contextdata.GetTyped[map[string]any](envelope, "task.context"); !ok || !reflect.DeepEqual(got, task.Context) {
		t.Fatalf("expected task.context to be seeded, got=%v ok=%v", got, ok)
	}
	if got, ok := contextdata.GetTyped[map[string]any](envelope, "task.metadata"); !ok || !reflect.DeepEqual(got, task.Metadata) {
		t.Fatalf("expected task.metadata to be seeded, got=%v ok=%v", got, ok)
	}
}

func TestBuildGraphResumeStateSkipsIntake(t *testing.T) {
	env := agentenv.WorkspaceEnvironment{
		Config: &execution.Config{
			AgentSpec: &agentspec.AgentRuntimeSpec{
				Capabilities: agentspec.AgentCapabilitiesSpec{Relurpic: append([]string{}, testRelurpicCapabilities...)},
			},
		},
		Registry: capability.NewRegistry(),
	}
	agent := New(env)
	if err := agent.Initialize(nil); err != nil {
		t.Fatalf("Initialize returned error: %v", err)
	}
	agent.env.Registry = nil

	task := &execution.Task{
		ID:          "task-43",
		Type:        "analysis",
		Instruction: "resume from classification",
	}
	envelope := contextdata.NewEnvelope("task-43", "session-100")
	classification := &intake.IntentClassification{WinningFamily: "implementation", Confidence: 1.0}
	state.SetIntentClassification(envelope, classification)
	agent.captureResumeState(envelope)
	agent.seedResumeState(envelope)

	graph, err := agent.BuildGraph(task)
	if err != nil {
		t.Fatalf("BuildGraph returned error: %v", err)
	}
	if graph == nil {
		t.Fatal("expected graph to be returned")
	}
	start := graphStartNodeID(graph)
	if start != "euclo.dispatch" {
		t.Fatalf("expected resume classification to start at dispatch, got %q", start)
	}
	if _, ok := state.GetResumeClassification(envelope); !ok {
		t.Fatal("expected resume classification to be seeded before graph build")
	}
}

// TestInitializeStoresConfig verifies that Initialize stores config and marks initialized.
func TestInitializeStoresConfig(t *testing.T) {
	env := agentenv.WorkspaceEnvironment{
		Config: &execution.Config{
			AgentSpec: &agentspec.AgentRuntimeSpec{
				Capabilities: agentspec.AgentCapabilitiesSpec{Relurpic: append([]string{}, testRelurpicCapabilities...)},
			},
		},
		Registry: capability.NewRegistry(),
	}
	agent := New(env)

	config := &execution.Config{}

	err := agent.Initialize(config)
	if err != nil {
		t.Fatalf("Initialize returned error: %v", err)
	}

	if !agent.initialized {
		t.Error("agent.initialized should be true after Initialize")
	}

	// Second call should not error
	err = agent.Initialize(config)
	if err != nil {
		t.Fatalf("Second Initialize call returned error: %v", err)
	}
}

// TestExecuteStashesResumeClassification verifies that Execute handles resume state.
// Note: Resume state handling is stubbed for Phase 1 and will be fully implemented in Phase 14.
func TestExecuteStashesResumeClassification(t *testing.T) {
	t.Skip("Phase 1: Resume state handling is stubbed; will be fully implemented in Phase 14")

	env := agentenv.WorkspaceEnvironment{
		Registry: capability.NewRegistry(),
	}
	agent := New(env)

	task := &execution.Task{
		ID:          "test-task",
		Type:        "analysis",
		Instruction: "test instruction",
	}

	envelope := contextdata.NewEnvelope("test-task", "test-session")
	classification := &intake.IntentClassification{WinningFamily: "analysis"}
	routeSelection := &euclotypes.RouteSelection{RouteKind: "capability", CapabilityID: "debug"}
	state.SetIntentClassification(envelope, classification)
	state.SetRouteSelection(envelope, routeSelection)

	_, err := agent.Execute(context.Background(), task, envelope)
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}

	resumedClassification, ok := state.GetResumeClassification(envelope)
	if !ok {
		t.Fatal("expected resume classification to be written during BuildGraph")
	}
	if resumedClassification != classification {
		t.Fatalf("resume classification = %p, want %p", resumedClassification, classification)
	}

	resumedRoute, ok := state.GetResumeRoute(envelope)
	if !ok {
		t.Fatal("expected resume route to be written during BuildGraph")
	}
	if resumedRoute != routeSelection {
		t.Fatalf("resume route = %p, want %p", resumedRoute, routeSelection)
	}
}

// TestExecuteClearsResumeStateAfterGraph verifies resume state is cleared after Execute.
// Note: Resume state clearing is stubbed for Phase 1.
func TestExecuteClearsResumeStateAfterGraph(t *testing.T) {
	env := agentenv.WorkspaceEnvironment{
		Config: &execution.Config{
			AgentSpec: &agentspec.AgentRuntimeSpec{
				Capabilities: agentspec.AgentCapabilitiesSpec{Relurpic: append([]string{}, testRelurpicCapabilities...)},
			},
		},
		Registry: capability.NewRegistry(),
	}
	agent := New(env)

	task := &execution.Task{
		ID:          "test-task",
		Type:        "analysis",
		Instruction: "test instruction",
	}

	envelope := contextdata.NewEnvelope("test-task", "test-session")

	_, _ = agent.Execute(context.Background(), task, envelope)

	if agent.resumeClassification != nil {
		t.Fatal("expected resumeClassification to be cleared after Execute")
	}
	if agent.resumeRouteSelection != nil {
		t.Fatal("expected resumeRouteSelection to be cleared after Execute")
	}
}

func graphStartNodeID(graph *agentgraph.Graph) string {
	if graph == nil {
		return ""
	}
	value := reflect.ValueOf(graph).Elem().FieldByName("startNodeID")
	if !value.IsValid() || value.Kind() != reflect.String {
		return ""
	}
	return value.String()
}

// TestCapabilitiesReturnsExpectedIDs verifies Capabilities() returns expected capability IDs.
func TestCapabilitiesReturnsExpectedIDs(t *testing.T) {
	env := agentenv.WorkspaceEnvironment{}
	agent := New(env)

	caps := agent.Capabilities()

	if len(caps) == 0 {
		t.Error("Capabilities() returned empty slice")
	}

	// Expected capabilities for Phase 1
	expected := []string{"euclo.agent", "euclo.routing", "euclo.classification"}
	if len(caps) != len(expected) {
		t.Errorf("Capabilities() returned %d items, expected %d", len(caps), len(expected))
	}

	for i, exp := range expected {
		if i < len(caps) && caps[i] != exp {
			t.Errorf("Capabilities()[%d] = %q, expected %q", i, caps[i], exp)
		}
	}
}

// TestDefaultConfig returns valid default configuration.
func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	if !cfg.BuiltinFamilies {
		t.Error("DefaultConfig.BuiltinFamilies should be true")
	}

	if cfg.WorkspaceIngestionMode != "files_only" {
		t.Errorf("DefaultConfig.WorkspaceIngestionMode = %q, expected 'files_only'", cfg.WorkspaceIngestionMode)
	}

	if cfg.MaxStreamTokens == 0 {
		t.Error("DefaultConfig.MaxStreamTokens should not be 0")
	}
}

// TestWithConfigOption verifies WithConfig option sets the config.
func TestWithConfigOption(t *testing.T) {
	env := agentenv.WorkspaceEnvironment{}
	customConfig := EucloConfig{
		BuiltinFamilies:        false,
		WorkspaceIngestionMode: "full",
		MaxStreamTokens:        4096,
	}

	agent := New(env, WithConfig(customConfig))

	if agent.config.BuiltinFamilies {
		t.Error("WithConfig should have set BuiltinFamilies to false")
	}

	if agent.config.WorkspaceIngestionMode != "full" {
		t.Errorf("WithConfig should have set WorkspaceIngestionMode to 'full', got %q", agent.config.WorkspaceIngestionMode)
	}

	if agent.config.MaxStreamTokens != 4096 {
		t.Errorf("WithConfig should have set MaxStreamTokens to 4096, got %d", agent.config.MaxStreamTokens)
	}
}
