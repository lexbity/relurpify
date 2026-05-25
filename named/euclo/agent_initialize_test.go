package euclo

import (
	"testing"

	"codeburg.org/lexbit/relurpify/framework/agentenv"
	"codeburg.org/lexbit/relurpify/framework/agentspec"
	"codeburg.org/lexbit/relurpify/framework/capability"
	"codeburg.org/lexbit/relurpify/framework/core"
)

var testRelurpicCapabilities = []string{
	"euclo:cap.test_run",
	"euclo:cap.ast_query",
	"euclo:cap.symbol_trace",
	"euclo:cap.call_graph",
	"euclo:cap.blame_trace",
	"euclo:cap.bisect",
	"euclo:cap.code_review",
	"euclo:cap.diff_summary",
	"euclo:cap.layer_check",
	"euclo:cap.targeted_refactor",
	"euclo:cap.rename_symbol",
	"euclo:cap.api_compat",
	"euclo:cap.boundary_report",
	"euclo:cap.coverage_check",
}

func TestAgentInitializeDoesNotPanic(t *testing.T) {
	// Create a minimal WorkspaceEnvironment
	env := agentenv.WorkspaceEnvironment{
		Config: &core.Config{
			AgentSpec: &agentspec.AgentRuntimeSpec{
				Capabilities: agentspec.AgentCapabilitiesSpec{Relurpic: append([]string{}, testRelurpicCapabilities...)},
			},
		},
		Registry: capability.NewCapabilityRegistry(),
	}

	// Create agent with the environment
	agent := New(env)

	// Initialize should not panic
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("agent.Initialize panicked: %v", r)
		}
	}()

	err := agent.Initialize(nil)
	if err != nil {
		t.Fatalf("agent.Initialize failed: %v", err)
	}

	// Verify initialization state
	if !agent.initialized {
		t.Fatal("agent should be initialized after Initialize call")
	}

	// Verify thoughtrecipeRegistry is set
	if agent.thoughtrecipeRegistry == nil {
		t.Fatal("thoughtrecipeRegistry should be set after Initialize")
	}
}

func TestAgentInitializeWithNilRegistry(t *testing.T) {
	// Test with nil registry - should error gracefully
	env := agentenv.WorkspaceEnvironment{
		Registry: nil,
	}

	agent := New(env)

	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("agent.Initialize with nil registry panicked: %v", r)
		}
	}()

	err := agent.Initialize(nil)
	// Should error because registry is nil
	if err == nil {
		t.Fatal("expected error when env.Registry is nil")
	}
}
