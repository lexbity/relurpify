package tui

import (
	"context"
	"testing"

	"codeburg.org/lexbit/relurpify/framework/agentgraph"
	"codeburg.org/lexbit/relurpify/framework/contextdata"
	"codeburg.org/lexbit/relurpify/framework/core"
)

type profileAgent struct{}

func (profileAgent) Initialize(*core.Config) error { return nil }

func (profileAgent) Execute(context.Context, *core.Task, *contextdata.Envelope) (*core.Result, error) {
	return nil, nil
}

func (profileAgent) Capabilities() []string { return nil }

func (profileAgent) BuildGraph(*core.Task) (*agentgraph.Graph, error) { return nil, nil }

func (profileAgent) RuntimeProfile() (string, string) { return "analysis", "route-dispatch" }

type plainAgent struct{}

func (plainAgent) Initialize(*core.Config) error { return nil }

func (plainAgent) Execute(context.Context, *core.Task, *contextdata.Envelope) (*core.Result, error) {
	return nil, nil
}

func (plainAgent) Capabilities() []string { return nil }

func (plainAgent) BuildGraph(*core.Task) (*agentgraph.Graph, error) { return nil, nil }

func TestDescribeAgentRuntimeUsesOptionalProfileInterface(t *testing.T) {
	mode, strategy := describeAgentRuntime(profileAgent{})
	if mode != "analysis" {
		t.Fatalf("mode = %q, want analysis", mode)
	}
	if strategy != "route-dispatch" {
		t.Fatalf("strategy = %q, want route-dispatch", strategy)
	}
}

func TestDescribeAgentRuntimeFallsBackWithoutProfileInterface(t *testing.T) {
	mode, strategy := describeAgentRuntime(plainAgent{})
	if mode != "" {
		t.Fatalf("mode = %q, want empty", mode)
	}
	if strategy != "" {
		t.Fatalf("strategy = %q, want empty", strategy)
	}
}
