package euclo

import (
	"testing"

	"codeburg.org/lexbit/relurpify/capability/agentspec"
	registry "codeburg.org/lexbit/relurpify/capability/registry"
	execution "codeburg.org/lexbit/relurpify/execution"
	"codeburg.org/lexbit/relurpify/cognitionzoo/paradigm"
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

	err := agent.Initialize(nil)
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
