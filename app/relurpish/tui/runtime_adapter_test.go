package tui

import (
	"context"
	"errors"
	"testing"

	"codeburg.org/lexbit/relurpify/context/contextdata"
	execution "codeburg.org/lexbit/relurpify/execution"
	"codeburg.org/lexbit/relurpify/execution/agentgraph"
)

type profileAgent struct{}

func (profileAgent) Initialize(*execution.Config) error { return nil }

func (profileAgent) Execute(context.Context, *execution.Task, *contextdata.Envelope) (*execution.Result, error) {
	return nil, errors.New("mock not implemented")
}

func (profileAgent) Capabilities() []string { return nil }

func (profileAgent) BuildGraph(context.Context, *execution.Task) (*agentgraph.Graph, error) {
	return nil, errors.New("mock not implemented")
}

func (profileAgent) RuntimeProfile() (string, string) { return "analysis", "route-dispatch" }

type plainAgent struct{}

func (plainAgent) Initialize(*execution.Config) error { return nil }

func (plainAgent) Execute(context.Context, *execution.Task, *contextdata.Envelope) (*execution.Result, error) {
	return nil, errors.New("mock not implemented")
}

func (plainAgent) Capabilities() []string { return nil }

func (plainAgent) BuildGraph(context.Context, *execution.Task) (*agentgraph.Graph, error) {
	return nil, errors.New("mock not implemented")
}

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
