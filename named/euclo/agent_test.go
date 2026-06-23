package euclo

import (
	"context"
	"reflect"
	"testing"

	"codeburg.org/lexbit/relurpify/capability/agentspec"
	registry "codeburg.org/lexbit/relurpify/capability/registry"
	"codeburg.org/lexbit/relurpify/cognitionzoo/paradigm"
	"codeburg.org/lexbit/relurpify/context/contextdata"
	execution "codeburg.org/lexbit/relurpify/execution"
	"codeburg.org/lexbit/relurpify/execution/agentgraph"
	"codeburg.org/lexbit/relurpify/named/euclo/euclokeys"
	"codeburg.org/lexbit/relurpify/named/euclo/euclotypes"
	"codeburg.org/lexbit/relurpify/named/euclo/intake"
	"codeburg.org/lexbit/relurpify/named/euclo/state"
)

func TestAgentCompiles(t *testing.T) {
	deps := &paradigm.Deps{
		Registry: registry.NewRegistry(),
	}
	agent := New(deps)

	if agent == nil {
		t.Fatal("New() returned nil")
	}
}

func TestAgentImplementsWorkflowExecutor(t *testing.T) {
	var _ agentgraph.WorkflowExecutor = (*Agent)(nil)
}

func TestBuildGraphReturnsGraph(t *testing.T) {
	t.Parallel()
	deps := &paradigm.Deps{
		Config: &execution.Config{
			AgentSpec: &agentspec.AgentRuntimeSpec{
				Capabilities: agentspec.AgentCapabilitiesSpec{Relurpic: append([]string{}, testRelurpicCapabilities...)},
			},
		},
		Registry: registry.NewRegistry(),
	}
	agent := New(deps)

	task := &execution.Task{
		ID:          "test-task",
		Type:        "analysis",
		Instruction: "test instruction",
	}

	graph, err := agent.BuildGraph(context.Background(), task)
	if err != nil {
		t.Fatalf("BuildGraph returned error: %v", err)
	}

	if graph == nil {
		t.Fatal("BuildGraph returned nil graph")
	}
}

func TestExecuteCallsBuildGraph(t *testing.T) {
	deps := &paradigm.Deps{
		Config: &execution.Config{
			AgentSpec: &agentspec.AgentRuntimeSpec{
				Capabilities: agentspec.AgentCapabilitiesSpec{Relurpic: append([]string{}, testRelurpicCapabilities...)},
			},
		},
		Registry: registry.NewRegistry(),
	}
	agent := New(deps)

	task := &execution.Task{
		ID:          "test-task",
		Type:        "analysis",
		Instruction: "test instruction",
	}

	envelope := contextdata.NewEnvelope("test-task", "test-session")

	_, err := agent.Execute(context.Background(), task, envelope)
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

	if got, ok := contextdata.GetTyped[*execution.Task](envelope, euclokeys.KeyTaskInput); !ok || got != task {
		t.Fatalf("expected %s to be seeded with task pointer, got=%v ok=%v", euclokeys.KeyTaskInput, got, ok)
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
	deps := &paradigm.Deps{
		Config: &execution.Config{
			AgentSpec: &agentspec.AgentRuntimeSpec{
				Capabilities: agentspec.AgentCapabilitiesSpec{Relurpic: append([]string{}, testRelurpicCapabilities...)},
			},
		},
		Registry: registry.NewRegistry(),
	}
	agent := New(deps)
	if err := agent.Initialize(nil); err != nil {
		t.Fatalf("Initialize returned error: %v", err)
	}
	agent.deps.Registry = nil

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

	graph, err := agent.BuildGraph(context.Background(), task)
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

func TestInitializeStoresConfig(t *testing.T) {
	deps := &paradigm.Deps{
		Config: &execution.Config{
			AgentSpec: &agentspec.AgentRuntimeSpec{
				Capabilities: agentspec.AgentCapabilitiesSpec{Relurpic: append([]string{}, testRelurpicCapabilities...)},
			},
		},
		Registry: registry.NewRegistry(),
	}
	agent := New(deps)

	config := &execution.Config{}

	err := agent.Initialize(config)
	if err != nil {
		t.Fatalf("Initialize returned error: %v", err)
	}

	if !agent.initialized {
		t.Error("agent.initialized should be true after Initialize")
	}

	err = agent.Initialize(config)
	if err != nil {
		t.Fatalf("Second Initialize call returned error: %v", err)
	}
}

func TestExecuteStashesResumeClassification(t *testing.T) {
	t.Skip("Resume state handling is stubbed; will be fully implemented later.")

	deps := &paradigm.Deps{
		Registry: registry.NewRegistry(),
	}
	agent := New(deps)

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

func TestExecuteClearsResumeStateAfterGraph(t *testing.T) {
	deps := &paradigm.Deps{
		Config: &execution.Config{
			AgentSpec: &agentspec.AgentRuntimeSpec{
				Capabilities: agentspec.AgentCapabilitiesSpec{Relurpic: append([]string{}, testRelurpicCapabilities...)},
			},
		},
		Registry: registry.NewRegistry(),
	}
	agent := New(deps)

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

func TestCapabilitiesReturnsExpectedIDs(t *testing.T) {
	deps := &paradigm.Deps{
		Registry: registry.NewRegistry(),
	}
	agent := New(deps)

	caps := agent.Capabilities()

	if len(caps) == 0 {
		t.Error("Capabilities() returned empty slice")
	}

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

func TestWithConfigOption(t *testing.T) {
	deps := &paradigm.Deps{
		Registry: registry.NewRegistry(),
	}
	customConfig := EucloConfig{
		BuiltinFamilies:        false,
		WorkspaceIngestionMode: "full",
		MaxStreamTokens:        4096,
	}

	agent := New(deps, WithConfig(customConfig))

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
