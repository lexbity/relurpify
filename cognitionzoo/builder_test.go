package agents

import (
	"context"
	"testing"

	"codeburg.org/lexbit/relurpify/capability/agentspec"
	capability "codeburg.org/lexbit/relurpify/capability/registry"
	"codeburg.org/lexbit/relurpify/cognitionzoo/paradigm"
	execution "codeburg.org/lexbit/relurpify/execution"
	"codeburg.org/lexbit/relurpify/model"
)

func TestBuildFromSpec_ReturnsReActForReactType(t *testing.T) {
	deps := &paradigm.Deps{
		Config:   &execution.Config{},
		Registry: capability.NewRegistry(),
	}

	spec := agentspec.AgentRuntimeSpec{Implementation: "react"}
	executor, err := BuildFromSpec(deps, spec)
	if err != nil {
		t.Fatalf("BuildFromSpec failed: %v", err)
	}
	if executor == nil {
		t.Fatal("expected non-nil executor")
	}
}

func TestBuildFromSpec_ReturnsPipelineForPipelineType(t *testing.T) {
	deps := &paradigm.Deps{
		Config:   &execution.Config{},
		Registry: capability.NewRegistry(),
	}

	spec := agentspec.AgentRuntimeSpec{Implementation: "pipeline"}
	executor, err := BuildFromSpec(deps, spec)
	if err != nil {
		t.Fatalf("BuildFromSpec failed: %v", err)
	}
	if executor == nil {
		t.Fatal("expected non-nil executor")
	}
}

func TestBuildFromSpec_UnknownTypeReturnsError(t *testing.T) {
	deps := &paradigm.Deps{
		Config:   &execution.Config{},
		Registry: capability.NewRegistry(),
	}

	spec := agentspec.AgentRuntimeSpec{Implementation: "unknown_agent_type"}
	_, err := BuildFromSpec(deps, spec)
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

func (m *mockModel) Complete(ctx context.Context, prompt string, opts *model.LLMOptions) (*model.LLMResponse, error) {
	return &model.LLMResponse{Text: "mock response"}, nil
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
