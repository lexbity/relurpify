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

func TestCompositeAliasPropagation(t *testing.T) {
	// First step produces stdout; second step references it via ${files.stdout}
	resolver := func(name string) (contracts.Tool, bool) {
		if name == "cli_fd" {
			return &fakeTool{name: "cli_fd", result: &contracts.ToolResult{
				Success: true,
				Data:    map[string]interface{}{"stdout": "src/main.go\nsrc/utils.go"},
			}}, true
		}
		if name == "cli_rg" {
			return &fakeTool{name: "cli_rg", result: &contracts.ToolResult{
				Success: true,
				Data:    map[string]interface{}{"stdout": "match1\nmatch2"},
			}}, true
		}
		return nil, false
	}

	tool := New(contracts.ToolManifest{
		Name:        "fd_pipe_rg",
		Description: "fd | rg pipeline",
		Composition: &contracts.ToolManifestComposition{
			Steps: []contracts.ToolManifestCompositionStep{
				{Tool: "cli_fd", Args: map[string]any{"pattern": "*.go"}, Alias: "files"},
				{Tool: "cli_rg", Args: map[string]any{"pattern": "func", "paths": "${files.stdout}"}, Alias: "matches"},
			},
		},
	}, resolver)

	result, err := tool.Execute(context.Background(), nil)
	require.NoError(t, err)
	require.True(t, result.Success)
	require.Contains(t, result.Data, "files")
	require.Contains(t, result.Data, "matches")
}

func TestCompositeThreeStepPipeline(t *testing.T) {
	step1 := &fakeTool{name: "step1", result: &contracts.ToolResult{Success: true, Data: map[string]interface{}{"stdout": "a\nb\nc"}}}
	step2 := &fakeTool{name: "step2", result: &contracts.ToolResult{Success: true, Data: map[string]interface{}{"stdout": "d\ne\nf"}}}
	step3 := &fakeTool{name: "step3", result: &contracts.ToolResult{Success: true, Data: map[string]interface{}{"stdout": "g"}}}

	resolver := func(name string) (contracts.Tool, bool) {
		switch name {
		case "s1":
			return step1, true
		case "s2":
			return step2, true
		case "s3":
			return step3, true
		}
		return nil, false
	}

	tool := New(contracts.ToolManifest{
		Name:        "three_step",
		Description: "three-step pipeline",
		Composition: &contracts.ToolManifestComposition{
			Steps: []contracts.ToolManifestCompositionStep{
				{Tool: "s1", Alias: "first"},
				{Tool: "s2", Args: map[string]any{"input": "${first.stdout}"}, Alias: "second"},
				{Tool: "s3", Args: map[string]any{"input": "${second.stdout}"}, Alias: "third"},
			},
		},
	}, resolver)

	result, err := tool.Execute(context.Background(), nil)
	require.NoError(t, err)
	require.True(t, result.Success)
	require.Contains(t, result.Data, "first")
	require.Contains(t, result.Data, "second")
	require.Contains(t, result.Data, "third")
}

func TestCompositeDuplicateAliasRejected(t *testing.T) {
	tool := New(contracts.ToolManifest{
		Name:        "dup",
		Description: "duplicate alias",
		Composition: &contracts.ToolManifestComposition{
			Steps: []contracts.ToolManifestCompositionStep{
				{Tool: "a", Alias: "output"},
				{Tool: "b", Alias: "output"},
			},
		},
	}, nil)

	result, err := tool.Execute(context.Background(), nil)
	require.NoError(t, err)
	require.False(t, result.Success)
	require.Contains(t, result.Error, "duplicate alias")
}

func TestCompositeUnnamedAliasesAreFine(t *testing.T) {
	tool := New(contracts.ToolManifest{
		Name:        "no_aliases",
		Description: "no aliases",
		Composition: &contracts.ToolManifestComposition{
			Steps: []contracts.ToolManifestCompositionStep{
				{Tool: "first"},
				{Tool: "second"},
			},
		},
	}, func(name string) (contracts.Tool, bool) {
		return &fakeTool{name: name, result: &contracts.ToolResult{Success: true}}, true
	})

	result, err := tool.Execute(context.Background(), nil)
	require.NoError(t, err)
	require.True(t, result.Success)
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
