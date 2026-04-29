package agenttest

import (
	"context"
	"testing"

	graph "codeburg.org/lexbit/relurpify/framework/agentgraph"
	"codeburg.org/lexbit/relurpify/framework/capability"
	"codeburg.org/lexbit/relurpify/framework/contextdata"
	"codeburg.org/lexbit/relurpify/framework/core"
)

// mockCapabilityRegistryProvider implements graph.WorkflowExecutor for testing
type mockCapabilityRegistryProvider struct {
	registry *capability.Registry
}

func (m *mockCapabilityRegistryProvider) CapabilityRegistry() *capability.Registry {
	return m.registry
}

// Execute implements graph.WorkflowExecutor
func (m *mockCapabilityRegistryProvider) Execute(ctx context.Context, task *core.Task, state *contextdata.Envelope) (*core.Result, error) {
	return nil, nil
}

// Initialize implements graph.WorkflowExecutor
func (m *mockCapabilityRegistryProvider) Initialize(config *core.Config) error {
	return nil
}

// Capabilities implements graph.WorkflowExecutor
func (m *mockCapabilityRegistryProvider) Capabilities() []string {
	return nil
}

// BuildGraph implements graph.WorkflowExecutor
func (m *mockCapabilityRegistryProvider) BuildGraph(task *core.Task) (*graph.Graph, error) {
	return nil, nil
}

var _ graph.WorkflowExecutor = (*mockCapabilityRegistryProvider)(nil)

func TestExtractCapabilityRegistry(t *testing.T) {
	reg := capability.NewRegistry()
	reg.Register(&mockTool{name: "tool1"})
	reg.Register(&mockTool{name: "tool2"})
	reg.Register(&mockTool{name: "tool3"})

	agent := &mockCapabilityRegistryProvider{registry: reg}

	coverage, err := ExtractCapabilityRegistry(agent)
	if err != nil {
		t.Fatalf("ExtractCapabilityRegistry failed: %v", err)
	}

	if coverage == nil {
		t.Fatal("Expected coverage to not be nil")
	}

	if len(coverage.RegisteredTools) != 3 {
		t.Errorf("Expected 3 registered tools, got %d", len(coverage.RegisteredTools))
	}

	// Check that all tools are registered
	toolsMap := make(map[string]bool)
	for _, tool := range coverage.RegisteredTools {
		toolsMap[tool] = true
	}

	for _, expected := range []string{"tool1", "tool2", "tool3"} {
		if !toolsMap[expected] {
			t.Errorf("Expected tool %s to be registered", expected)
		}
	}
}

func TestExtractCapabilityRegistry_NilAgent(t *testing.T) {
	_, err := ExtractCapabilityRegistry(nil)
	if err == nil {
		t.Error("Expected error for nil agent")
	}
}

func TestExtractCapabilityRegistry_NoRegistry(t *testing.T) {
	agent := &mockNoRegistryAgent{}
	_, err := ExtractCapabilityRegistry(agent)
	if err == nil {
		t.Error("Expected error when agent has no capability registry")
	}
}

func TestComputeCoverage(t *testing.T) {
	coverage := &CapabilityCoverage{
		RegistryID:      "test-reg",
		RegisteredTools: []string{"tool1", "tool2", "tool3", "tool4"},
		ExercisedTools:  make(map[string]int),
	}

	toolCounts := map[string]int{
		"tool1": 2,
		"tool2": 5,
		"tool3": 1,
	}

	err := ComputeCoverage(coverage, toolCounts)
	if err != nil {
		t.Fatalf("ComputeCoverage failed: %v", err)
	}

	// Check exercised tools
	if len(coverage.ExercisedTools) != 3 {
		t.Errorf("Expected 3 exercised tools, got %d", len(coverage.ExercisedTools))
	}

	// Check unexercised tools
	if len(coverage.UnexercisedTools) != 1 {
		t.Errorf("Expected 1 unexercised tool, got %d", len(coverage.UnexercisedTools))
	}

	if len(coverage.UnexercisedTools) > 0 && coverage.UnexercisedTools[0] != "tool4" {
		t.Errorf("Expected tool4 to be unexercised, got %s", coverage.UnexercisedTools[0])
	}

	// Check coverage ratio (3/4 = 0.75)
	expectedRatio := 0.75
	if coverage.CoverageRatio != expectedRatio {
		t.Errorf("Expected coverage ratio %f, got %f", expectedRatio, coverage.CoverageRatio)
	}
}

func TestComputeCoverage_NilCoverage(t *testing.T) {
	err := ComputeCoverage(nil, map[string]int{"tool1": 1})
	if err == nil {
		t.Error("Expected error for nil coverage")
	}
}

func TestComputeCoverage_EmptyRegistry(t *testing.T) {
	coverage := &CapabilityCoverage{
		RegistryID:      "test-reg",
		RegisteredTools: []string{},
		ExercisedTools:  make(map[string]int),
	}

	err := ComputeCoverage(coverage, map[string]int{"tool1": 1})
	if err != nil {
		t.Fatalf("ComputeCoverage failed: %v", err)
	}

	if coverage.CoverageRatio != 0.0 {
		t.Errorf("Expected 0.0 coverage ratio for empty registry, got %f", coverage.CoverageRatio)
	}
}

func TestRegistryHasTool(t *testing.T) {
	reg := capability.NewRegistry()
	reg.Register(&mockTool{name: "existing_tool"})

	agent := &mockCapabilityRegistryProvider{registry: reg}

	if !RegistryHasTool(agent, "existing_tool") {
		t.Error("Expected RegistryHasTool to return true for existing tool")
	}

	if RegistryHasTool(agent, "nonexistent_tool") {
		t.Error("Expected RegistryHasTool to return false for nonexistent tool")
	}

	if RegistryHasTool(nil, "existing_tool") {
		t.Error("Expected RegistryHasTool to return false for nil agent")
	}

	if RegistryHasTool(agent, "") {
		t.Error("Expected RegistryHasTool to return false for empty tool name")
	}
}

func TestValidateToolsRequired(t *testing.T) {
	coverage := &CapabilityCoverage{
		ExercisedTools: map[string]int{
			"tool1": 2,
			"tool2": 1,
		},
	}

	// All required tools present
	failures := ValidateToolsRequired(coverage, []string{"tool1", "tool2"})
	if len(failures) > 0 {
		t.Errorf("Expected no failures, got: %v", failures)
	}

	// Missing required tool
	failures = ValidateToolsRequired(coverage, []string{"tool1", "tool3"})
	if len(failures) != 1 {
		t.Errorf("Expected 1 failure, got %d: %v", len(failures), failures)
	}

	// Nil coverage
	failures = ValidateToolsRequired(nil, []string{"tool1"})
	if len(failures) != 1 {
		t.Errorf("Expected 1 failure for nil coverage, got %d", len(failures))
	}

	// Empty required tools
	failures = ValidateToolsRequired(coverage, []string{})
	if len(failures) > 0 {
		t.Errorf("Expected no failures for empty required tools, got: %v", failures)
	}
}

func TestBuildCoverageFromEvents(t *testing.T) {
	reg := capability.NewRegistry()
	reg.Register(&mockTool{name: "go_test"})
	reg.Register(&mockTool{name: "file_read"})
	reg.Register(&mockTool{name: "file_write"})

	agent := &mockCapabilityRegistryProvider{registry: reg}

	events := []core.Event{
		{Type: core.EventToolCall, Metadata: map[string]any{"tool": "go_test"}},
		{Type: core.EventToolCall, Metadata: map[string]any{"tool": "file_read"}},
		{Type: core.EventToolCall, Metadata: map[string]any{"tool": "go_test"}},
		{Type: core.EventLLMResponse, Metadata: map[string]any{}},
	}

	coverage, err := BuildCoverageFromEvents(agent, events)
	if err != nil {
		t.Fatalf("BuildCoverageFromEvents failed: %v", err)
	}

	if coverage == nil {
		t.Fatal("Expected coverage to not be nil")
	}

	// Check exercised tools
	if coverage.ExercisedTools["go_test"] != 2 {
		t.Errorf("Expected go_test to be called 2 times, got %d", coverage.ExercisedTools["go_test"])
	}

	if coverage.ExercisedTools["file_read"] != 1 {
		t.Errorf("Expected file_read to be called 1 time, got %d", coverage.ExercisedTools["file_read"])
	}

	// file_write should be in unexercised
	found := false
	for _, tool := range coverage.UnexercisedTools {
		if tool == "file_write" {
			found = true
			break
		}
	}
	if !found {
		t.Error("Expected file_write to be in unexercised tools")
	}
}

func TestCoverageReport(t *testing.T) {
	coverage := &CapabilityCoverage{
		RegistryID:       "test-reg-123",
		RegisteredTools:  []string{"tool1", "tool2", "tool3"},
		ExercisedTools:   map[string]int{"tool1": 2, "tool2": 1},
		UnexercisedTools: []string{"tool3"},
		CoverageRatio:    0.6667,
	}

	report := CoverageReport(coverage)

	// Check that report contains expected information
	expectedSubstrings := []string{
		"Capability Coverage Report",
		"test-reg-123",
		"Total Tools: 3",
		"Exercised Tools: 2",
		"Unexercised Tools: 1",
		"tool1 (called 2 times)",
		"tool2 (called 1 times)",
		"tool3",
	}

	for _, expected := range expectedSubstrings {
		if !contains(report, expected) {
			t.Errorf("Expected report to contain %q", expected)
		}
	}
}

func TestCoverageReport_Nil(t *testing.T) {
	report := CoverageReport(nil)
	if !contains(report, "nil") {
		t.Error("Expected nil coverage report to indicate nil")
	}
}

// mockNoRegistryAgent implements graph.WorkflowExecutor without capability registry
type mockNoRegistryAgent struct{}

func (m *mockNoRegistryAgent) Execute(ctx context.Context, task *core.Task, state *contextdata.Envelope) (*core.Result, error) {
	return nil, nil
}

func (m *mockNoRegistryAgent) Initialize(config *core.Config) error {
	return nil
}

func (m *mockNoRegistryAgent) Capabilities() []string {
	return nil
}

func (m *mockNoRegistryAgent) BuildGraph(task *core.Task) (*graph.Graph, error) {
	return nil, nil
}

var _ graph.WorkflowExecutor = (*mockNoRegistryAgent)(nil)

// mockTool implements a minimal core.Tool for testing
type mockTool struct {
	name string
}

func (m *mockTool) Name() string                     { return m.name }
func (m *mockTool) Description() string              { return "Mock tool " + m.name }
func (m *mockTool) Category() string                 { return "test" }
func (m *mockTool) Parameters() []core.ToolParameter { return nil }
func (m *mockTool) Execute(ctx context.Context, args map[string]any) (*core.ToolResult, error) {
	return &core.ToolResult{Data: map[string]any{"result": "ok"}}, nil
}
func (m *mockTool) IsAvailable(ctx context.Context) bool { return true }
func (m *mockTool) Permissions() core.ToolPermissions                         { return core.ToolPermissions{} }
func (m *mockTool) Tags() []string                                            { return nil }

var _ core.Tool = (*mockTool)(nil)

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 || containsInternal(s, substr))
}

func containsInternal(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
