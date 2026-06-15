package conformance

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	capports "codeburg.org/lexbit/relurpify/capability/ports"
	"codeburg.org/lexbit/relurpify/capability/registry"
	"codeburg.org/lexbit/relurpify/cognitionzoo/paradigm"
	"codeburg.org/lexbit/relurpify/context/contextdata"
	"codeburg.org/lexbit/relurpify/execution"
	"codeburg.org/lexbit/relurpify/execution/prompt"
	"codeburg.org/lexbit/relurpify/named/euclo/promptprovider"
	"codeburg.org/lexbit/relurpify/named/euclo/surface"
	thoughtrecipepkg "codeburg.org/lexbit/relurpify/named/euclo/thoughtrecipes"
	"codeburg.org/lexbit/relurpify/platform/fs"
	"codeburg.org/lexbit/relurpify/platform/llm/toolwire"
)

func TestConformanceMatrix(t *testing.T) {
	t.Run("traversal/native", func(t *testing.T) {
		runTraversalTurn(t, true)
	})
	t.Run("traversal/fallback", func(t *testing.T) {
		runTraversalTurn(t, false)
	})
	t.Run("grep/native", func(t *testing.T) {
		runGrepTurn(t, true)
	})
	t.Run("grep/fallback", func(t *testing.T) {
		runGrepTurn(t, false)
	})
	t.Run("edit/native", func(t *testing.T) {
		runEditTurn(t, true)
	})
	t.Run("edit/fallback", func(t *testing.T) {
		runEditTurn(t, false)
	})
	// Visibility, grant, and context control are calling-mode-agnostic: filtering,
	// invoke-time denial, and step-context injection happen before/around the model
	// call and behave identically for native and fallback. They are run once here (not
	// duplicated under a fake native/fallback label that would inflate coverage). The
	// fallback renderer's no-leak guarantee is asserted inside runRecipeVisibilityMatrixRow.
	t.Run("visibility", func(t *testing.T) {
		runRecipeVisibilityMatrixRow(t)
	})
	t.Run("grant", func(t *testing.T) {
		runRecipeGrantMatrixRow(t)
	})
	t.Run("context", func(t *testing.T) {
		runContextControlMatrixRow(t)
	})
}

func runRecipeVisibilityMatrixRow(t *testing.T) {
	t.Helper()
	workspace := t.TempDir()

	base := registry.NewRegistry()
	require.NoError(t, base.RegisterLegacyTool(context.Background(), &fs.ReadFileTool{BasePath: workspace}))
	require.NoError(t, base.RegisterLegacyTool(context.Background(), &fs.EditFileTool{BasePath: workspace}))

	readDesc, ok := base.GetCapability("file_read")
	require.True(t, ok)
	editDesc, ok := base.GetCapability("file_edit")
	require.True(t, ok)

	filtered := registry.NewFilteredRegistry(base, []string{readDesc.ID})
	require.True(t, filtered.IsAllowed(readDesc.ID))
	require.False(t, filtered.IsAllowed(editDesc.ID))

	callable := filtered.ModelCallableTools(context.Background())
	require.Len(t, callable, 1)
	require.Equal(t, "file_read", callable[0].Name())

	rendered := toolwire.RenderToolsToPrompt(capports.LLMToolSpecsFromTools(callable))
	require.Contains(t, rendered, "file_read")
	require.NotContains(t, rendered, "file_edit")
	require.NotContains(t, rendered, "old_string")
	require.NotContains(t, rendered, "new_string")
	require.True(t, strings.Contains(rendered, "```tool"))
}

func runRecipeGrantMatrixRow(t *testing.T) {
	t.Helper()
	base := registry.NewRegistry()
	handler := &grantRecordingCapability{id: "euclo:cap.restricted"}
	require.NoError(t, base.RegisterInvocableCapability(context.Background(), handler))

	scoped := base.WithAllowlist([]string{"euclo:cap.allowed"})
	require.NotNil(t, scoped)

	env := contextdata.NewEnvelope("task-grant", "session-grant")
	step := thoughtrecipepkg.ExecutionStep{
		ID:           "grant.step",
		CapabilityID: "euclo:cap.restricted",
		Step: surface.ThoughtRecipeStep{
			ID:           "grant.step",
			CapabilityID: "euclo:cap.restricted",
			OnError: &surface.StepErrorPolicy{
				Action:   "skip",
				RetryMax: 0,
				Fallback: "grant.step.recover",
			},
			Config: map[string]any{},
		},
	}

	node := thoughtrecipepkg.NewThoughtRecipeStepNode("grant.step.execute", &paradigm.Deps{Registry: scoped}, step)
	result, err := node.Execute(context.Background(), env)

	require.NoError(t, err)
	require.NotNil(t, result)
	require.True(t, result.Success)
	require.False(t, handler.called)

	skipped, ok := execution.ResultField(result.Data, "skipped")
	require.True(t, ok)
	require.Equal(t, true, skipped)

	reason, ok := execution.ResultField(result.Data, "skipped_reason")
	require.True(t, ok)
	require.Contains(t, reason, "not permitted in this context")

	action, ok := execution.ResultField(result.Data, "on_error_action")
	require.True(t, ok)
	require.Equal(t, "skip", action)

	fallback, ok := execution.ResultField(result.Data, "on_error_fallback")
	require.True(t, ok)
	require.Equal(t, "grant.step.recover", fallback)

	require.Equal(t, "skipped", result.Metadata["on_error_resolved"])
}

func runContextControlMatrixRow(t *testing.T) {
	t.Helper()
	env := contextdata.NewEnvelope("task-context", "session-context")
	provider := promptprovider.NewThoughtRecipeStepContextProvider()

	base := provider.Provide(prompt.NewRuntimeContext(env, "react", "thoughtrecipe"))
	riched := provider.Provide(prompt.NewRuntimeContext(env, "react", "thoughtrecipe").
		WithStateValue("execution_step_context_stream_query", "find relevant symbols").
		WithStateValue("execution_step_context_stream_max_tokens", 128).
		WithStateValue("execution_step_context_ingest_mode", "files_only"))

	require.NotEmpty(t, base.Content)
	require.Contains(t, riched.Content, "Step Context Stream Query: find relevant symbols")
	require.Contains(t, riched.Content, "Step Context Stream Max Tokens: 128")
	require.Contains(t, riched.Content, "Step Context Ingest Mode: files_only")
	require.NotEqual(t, base.Content, riched.Content)
}
