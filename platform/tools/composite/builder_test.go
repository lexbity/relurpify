package composite

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"codeburg.org/lexbit/relurpify/capability/ports"
)

func TestBackendBuilderResolvesSubTools(t *testing.T) {
	resolved := ""
	resolver := func(name string) (ports.Tool, bool) {
		resolved = name
		return &fakeTool{name: name, result: &ports.ToolResult{Success: true, Data: map[string]any{"stdout": "ok"}}}, true
	}

	builder := BackendBuilder(resolver)
	tool, err := builder.BuildTool(ports.ToolManifest{
		Name:        "pipe",
		Description: "composite",
		Composition: &ports.ToolManifestComposition{
			Steps: []ports.ToolManifestCompositionStep{{Tool: "sub", Alias: "first"}},
		},
	}, nil)
	require.NoError(t, err)
	require.NotNil(t, tool)

	result, err := tool.Execute(context.Background(), nil)
	require.NoError(t, err)
	require.True(t, result.Success)
	require.Equal(t, "sub", resolved)
}

func TestBackendBuilderRequiresResolver(t *testing.T) {
	builder := BackendBuilder(nil)
	_, err := builder.BuildTool(ports.ToolManifest{Name: "x"}, nil)
	require.Error(t, err)
}
