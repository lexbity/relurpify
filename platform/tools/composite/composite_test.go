package composite

import (
	"context"
	"testing"

	"codeburg.org/lexbit/relurpify/platform/contracts"
	"github.com/stretchr/testify/require"
)

func TestCompositeToolName(t *testing.T) {
	tool := New(contracts.ToolManifest{
		Name:        "pipe_tool",
		Description: "pipeline",
		Composition: &contracts.ToolManifestComposition{
			Steps: []contracts.ToolManifestCompositionStep{
				{Tool: "cli_fd", Alias: "files"},
			},
		},
	}, nil)
	require.Equal(t, "pipe_tool", tool.Name())
	require.Equal(t, "pipeline", tool.Description())
}

func TestCompositeToolSteps(t *testing.T) {
	fdResult := &contracts.ToolResult{Success: true, Data: map[string]interface{}{"stdout": "file1\nfile2"}}
	resolver := func(name string) (contracts.Tool, bool) {
		if name == "cli_fd" {
			return &fakeTool{name: "cli_fd", result: fdResult}, true
		}
		return nil, false
	}

	tool := New(contracts.ToolManifest{
		Name:        "pipe_tool",
		Description: "pipeline",
		Composition: &contracts.ToolManifestComposition{
			Steps: []contracts.ToolManifestCompositionStep{
				{Tool: "cli_fd", Alias: "files"},
			},
		},
	}, resolver)

	result, err := tool.Execute(context.Background(), nil)
	require.NoError(t, err)
	require.True(t, result.Success)
	require.Contains(t, result.Data, "files")
}

func TestCompositeToolFailsOnFirstError(t *testing.T) {
	resolver := func(name string) (contracts.Tool, bool) {
		if name == "ok" {
			return &fakeTool{name: "ok", result: &contracts.ToolResult{Success: true}}, true
		}
		if name == "fail" {
			return &fakeTool{name: "fail", result: &contracts.ToolResult{Success: false, Error: "step failed"}}, true
		}
		return nil, false
	}

	tool := New(contracts.ToolManifest{
		Name:        "pipe_tool",
		Description: "pipeline",
		Composition: &contracts.ToolManifestComposition{
			Steps: []contracts.ToolManifestCompositionStep{
				{Tool: "ok", Alias: "first"},
				{Tool: "fail", Alias: "second"},
			},
		},
	}, resolver)

	result, err := tool.Execute(context.Background(), nil)
	require.NoError(t, err)
	require.False(t, result.Success)
	require.Contains(t, result.Error, "step failed")
}

func TestCompositeToolMissingStepTool(t *testing.T) {
	resolver := func(name string) (contracts.Tool, bool) {
		return nil, false
	}

	tool := New(contracts.ToolManifest{
		Name:        "pipe_tool",
		Description: "pipeline",
		Composition: &contracts.ToolManifestComposition{
			Steps: []contracts.ToolManifestCompositionStep{
				{Tool: "missing_tool"},
			},
		},
	}, resolver)

	result, err := tool.Execute(context.Background(), nil)
	require.NoError(t, err)
	require.False(t, result.Success)
	require.Contains(t, result.Error, "not found")
}

type fakeTool struct {
	contracts.Tool
	name   string
	result *contracts.ToolResult
}

func (f *fakeTool) Name() string                       { return f.name }
func (f *fakeTool) Description() string                { return "fake: " + f.name }
func (f *fakeTool) Category() string                   { return "fake" }
func (f *fakeTool) Parameters() []contracts.ToolParameter { return nil }
func (f *fakeTool) Tags() []string                     { return nil }
func (f *fakeTool) Permissions() contracts.ToolPermissions {
	return contracts.ToolPermissions{}
}
func (f *fakeTool) IsAvailable(ctx context.Context) bool { return true }
func (f *fakeTool) Execute(ctx context.Context, args map[string]interface{}) (*contracts.ToolResult, error) {
	return f.result, nil
}
