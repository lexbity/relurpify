package composite

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"codeburg.org/lexbit/relurpify/capability/ports"
)

func TestCompositeToolName(t *testing.T) {
	tool := New(ports.ToolManifest{
		Name:        "pipe_tool",
		Description: "pipeline",
		Composition: &ports.ToolManifestComposition{
			Steps: []ports.ToolManifestCompositionStep{
				{Tool: "cli_fd", Alias: "files"},
			},
		},
	}, nil)
	require.Equal(t, "pipe_tool", tool.Name())
	require.Equal(t, "pipeline", tool.Description())
}

func TestCompositeToolSteps(t *testing.T) {
	fdResult := &ports.ToolResult{Success: true, Data: map[string]any{"stdout": "file1\nfile2"}}
	resolver := func(name string) (ports.Tool, bool) {
		if name == "cli_fd" {
			return &fakeTool{name: "cli_fd", result: fdResult}, true
		}
		return nil, false
	}

	tool := New(ports.ToolManifest{
		Name:        "pipe_tool",
		Description: "pipeline",
		Composition: &ports.ToolManifestComposition{
			Steps: []ports.ToolManifestCompositionStep{
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
	resolver := func(name string) (ports.Tool, bool) {
		if name == "ok" {
			return &fakeTool{name: "ok", result: &ports.ToolResult{Success: true}}, true
		}
		if name == "fail" {
			return &fakeTool{name: "fail", result: &ports.ToolResult{Success: false, Error: "step failed"}}, true
		}
		return nil, false
	}

	tool := New(ports.ToolManifest{
		Name:        "pipe_tool",
		Description: "pipeline",
		Composition: &ports.ToolManifestComposition{
			Steps: []ports.ToolManifestCompositionStep{
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
	resolver := func(name string) (ports.Tool, bool) {
		return nil, false
	}

	tool := New(ports.ToolManifest{
		Name:        "pipe_tool",
		Description: "pipeline",
		Composition: &ports.ToolManifestComposition{
			Steps: []ports.ToolManifestCompositionStep{
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
	resolver := func(name string) (ports.Tool, bool) {
		if name == "cli_fd" {
			return &fakeTool{name: "cli_fd", result: &ports.ToolResult{
				Success: true,
				Data:    map[string]any{"stdout": "src/main.go\nsrc/utils.go"},
			}}, true
		}
		if name == "cli_rg" {
			return &fakeTool{name: "cli_rg", result: &ports.ToolResult{
				Success: true,
				Data:    map[string]any{"stdout": "match1\nmatch2"},
			}}, true
		}
		return nil, false
	}

	tool := New(ports.ToolManifest{
		Name:        "fd_pipe_rg",
		Description: "fd | rg pipeline",
		Composition: &ports.ToolManifestComposition{
			Steps: []ports.ToolManifestCompositionStep{
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
	step1 := &fakeTool{name: "step1", result: &ports.ToolResult{Success: true, Data: map[string]any{"stdout": "a\nb\nc"}}}
	step2 := &fakeTool{name: "step2", result: &ports.ToolResult{Success: true, Data: map[string]any{"stdout": "d\ne\nf"}}}
	step3 := &fakeTool{name: "step3", result: &ports.ToolResult{Success: true, Data: map[string]any{"stdout": "g"}}}

	resolver := func(name string) (ports.Tool, bool) {
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

	tool := New(ports.ToolManifest{
		Name:        "three_step",
		Description: "three-step pipeline",
		Composition: &ports.ToolManifestComposition{
			Steps: []ports.ToolManifestCompositionStep{
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
	tool := New(ports.ToolManifest{
		Name:        "dup",
		Description: "duplicate alias",
		Composition: &ports.ToolManifestComposition{
			Steps: []ports.ToolManifestCompositionStep{
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
	tool := New(ports.ToolManifest{
		Name:        "no_aliases",
		Description: "no aliases",
		Composition: &ports.ToolManifestComposition{
			Steps: []ports.ToolManifestCompositionStep{
				{Tool: "first"},
				{Tool: "second"},
			},
		},
	}, func(name string) (ports.Tool, bool) {
		return &fakeTool{name: name, result: &ports.ToolResult{Success: true}}, true
	})

	result, err := tool.Execute(context.Background(), nil)
	require.NoError(t, err)
	require.True(t, result.Success)
}

type fakeTool struct {
	ports.Tool
	name   string
	result *ports.ToolResult
}

func (f *fakeTool) Name() string                      { return f.name }
func (f *fakeTool) Description() string               { return "fake: " + f.name }
func (f *fakeTool) Category() string                  { return "fake" }
func (f *fakeTool) Parameters() []ports.ToolParameter { return nil }
func (f *fakeTool) Tags() []string                    { return nil }
func (f *fakeTool) Permissions() ports.ToolPermissions {
	return ports.ToolPermissions{}
}
func (f *fakeTool) IsAvailable(ctx context.Context) bool { return true }
func (f *fakeTool) Execute(ctx context.Context, args map[string]any) (*ports.ToolResult, error) {
	return f.result, nil
}
