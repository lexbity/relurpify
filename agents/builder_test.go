package agents

import (
	"context"
	"testing"

	"codeburg.org/lexbit/relurpify/framework/agentenv"
	"codeburg.org/lexbit/relurpify/framework/capability"
	"codeburg.org/lexbit/relurpify/framework/core"
)

func TestBuildFromSpec_ReturnsReActForReactType(t *testing.T) {
	env := &agentenv.WorkspaceEnvironment{
		Config:   &core.Config{},
		Registry: capability.NewRegistry(),
	}

	spec := core.AgentRuntimeSpec{Implementation: "react"}
	executor, err := BuildFromSpec(env, spec)
	if err != nil {
		t.Fatalf("BuildFromSpec failed: %v", err)
	}
	if executor == nil {
		t.Fatal("expected non-nil executor")
	}
}

func TestBuildFromSpec_ReturnsPipelineForPipelineType(t *testing.T) {
	env := &agentenv.WorkspaceEnvironment{
		Config:   &core.Config{},
		Registry: capability.NewRegistry(),
	}

	spec := core.AgentRuntimeSpec{Implementation: "pipeline"}
	executor, err := BuildFromSpec(env, spec)
	if err != nil {
		t.Fatalf("BuildFromSpec failed: %v", err)
	}
	if executor == nil {
		t.Fatal("expected non-nil executor")
	}
}

func TestBuildFromSpec_UnknownTypeReturnsError(t *testing.T) {
	env := &agentenv.WorkspaceEnvironment{
		Config:   &core.Config{},
		Registry: capability.NewRegistry(),
	}

	spec := core.AgentRuntimeSpec{Implementation: "unknown_agent_type"}
	_, err := BuildFromSpec(env, spec)
	if err == nil {
		t.Fatal("expected error for unknown agent type")
	}
}

func TestAgentBuilder_RequiresEnvironment(t *testing.T) {
	builder := NewAgentBuilder()
	_, err := builder.Build("react")
	if err == nil {
		t.Fatal("expected error when environment is not set")
	}
}

// Mock types for testing

type mockModel struct{}

func (m *mockModel) Complete(ctx context.Context, prompt string, opts *core.LLMOptions) (*core.LLMResponse, error) {
	return &core.LLMResponse{Text: "mock response"}, nil
}

type mockMemory struct{}

func (m *mockMemory) Get(ctx context.Context, key string) (any, bool) {
	return nil, false
}

func (m *mockMemory) Set(ctx context.Context, key string, value any) error {
	return nil
}

func (m *mockMemory) Delete(ctx context.Context, key string) error {
	return nil
}

func (m *mockMemory) List(ctx context.Context, prefix string) ([]string, error) {
	return nil, nil
}
