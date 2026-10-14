package promptprovider

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"codeburg.org/lexbit/relurpify/capability/ports"
	"codeburg.org/lexbit/relurpify/execution/prompt"
)

// stubTool is a minimal ports.Tool fake; providers only read Name and Description.
type stubTool struct {
	name string
	desc string
}

func (t *stubTool) Name() string                      { return t.name }
func (t *stubTool) Description() string               { return t.desc }
func (t *stubTool) Category() string                  { return "test" }
func (t *stubTool) Parameters() []ports.ToolParameter { return nil }
func (t *stubTool) Execute(_ context.Context, _ map[string]any) (*ports.ToolResult, error) {
	return &ports.ToolResult{Success: true}, nil
}
func (t *stubTool) IsAvailable(_ context.Context) bool { return true }
func (t *stubTool) Permissions() ports.ToolPermissions { return ports.ToolPermissions{} }
func (t *stubTool) Tags() []string                     { return nil }

func TestRegisterAllRegistersEveryProvider(t *testing.T) {
	registry := prompt.NewRegistry()
	require.NoError(t, RegisterAll(registry))

	for _, name := range []string{
		"react.tools",
		"react.capability_catalog",
		"react.phase",
		"react.plan_goal",
		"react.current_step",
		"react.prior_step_result",
		"react.external_state",
		"react.declarative_memory",
		"react.workflow_retrieval",
		"react.observations",
		"react.history",
		"react.context_files",
		"pipeline.stage_outputs",
		"pipeline.task_instruction",
	} {
		assert.NotEmpty(t, name)
		// The name must already be taken by RegisterAll; a duplicate
		// registration attempt proves the provider is present.
		err := registry.RegisterProvider(name, stubProvider{})
		require.Error(t, err, "provider %s should be registered by RegisterAll", name)
		assert.True(t, prompt.IsAlreadyRegistered(err), "expected already-registered error for %s, got %v", name, err)
	}
}

func TestRegisterAllIsIdempotent(t *testing.T) {
	registry := prompt.NewRegistry()
	require.NoError(t, RegisterAll(registry))
	require.NoError(t, RegisterAll(registry), "second RegisterAll must skip already-registered providers")
}

type stubProvider struct{}

func (stubProvider) Provide(_ prompt.RuntimeContext) prompt.ContextChunk {
	return prompt.ContextChunk{}
}
func (stubProvider) Describe() prompt.ProviderMetadata {
	return prompt.ProviderMetadata{Name: "test.stub"}
}

func TestAllProvidersDeclareUniqueNamesAndParadigms(t *testing.T) {
	providers := []prompt.DescribingProvider{
		reactToolsProvider{},
		reactCapabilityCatalogProvider{},
		reactPhaseProvider{},
		reactPlanGoalProvider{},
		reactCurrentStepProvider{},
		reactPriorStepProvider{},
		reactExternalStateProvider{},
		reactDeclarativeMemoryProvider{},
		reactWorkflowRetrievalProvider{},
		reactObservationsProvider{},
		reactHistoryProvider{},
		reactContextFilesProvider{},
		pipelineStageOutputsProvider{},
		pipelineTaskInstructionProvider{},
	}

	seen := make(map[string]bool, len(providers))
	for _, p := range providers {
		meta := p.Describe()
		require.NotEmpty(t, meta.Name)
		require.NotEmpty(t, meta.Description)
		require.NotEmpty(t, meta.Paradigms, "provider %s must declare paradigms", meta.Name)
		assert.False(t, seen[meta.Name], "duplicate provider name %s", meta.Name)
		seen[meta.Name] = true

		// Every provider must tolerate an empty runtime context without panicking.
		assert.NotPanics(t, func() { _ = p.Provide(prompt.RuntimeContext{}) })
	}
	assert.Len(t, seen, len(providers))
}
