package euclo

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/lexcodex/relurpify/framework/capability"
	"github.com/lexcodex/relurpify/framework/core"
	"github.com/stretchr/testify/require"
)

type eucloFileWriteTool struct{}

func (eucloFileWriteTool) Name() string        { return "file_write" }
func (eucloFileWriteTool) Description() string { return "writes a file" }
func (eucloFileWriteTool) Category() string    { return "file" }
func (eucloFileWriteTool) Parameters() []core.ToolParameter {
	return []core.ToolParameter{
		{Name: "path", Type: "string", Required: true},
		{Name: "content", Type: "string", Required: true},
	}
}
func (eucloFileWriteTool) Execute(_ context.Context, _ *core.Context, args map[string]interface{}) (*core.ToolResult, error) {
	path := filepath.Clean(args["path"].(string))
	content := args["content"].(string)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		return nil, err
	}
	return &core.ToolResult{Success: true, Data: map[string]any{"path": path}}, nil
}
func (eucloFileWriteTool) IsAvailable(_ context.Context, _ *core.Context) bool { return true }
func (eucloFileWriteTool) Permissions() core.ToolPermissions {
	return core.ToolPermissions{Permissions: &core.PermissionSet{
		FileSystem: []core.FileSystemPermission{{Action: core.FileSystemWrite, Path: "."}},
	}}
}
func (eucloFileWriteTool) Tags() []string { return []string{core.TagDestructive, "file", "edit"} }

func TestApplyEditIntentArtifactsExecutesWritesThroughCapabilityRegistry(t *testing.T) {
	registry := capability.NewRegistry()
	require.NoError(t, registry.Register(eucloFileWriteTool{}))

	target := filepath.Join(t.TempDir(), "hello.txt")
	state := core.NewContext()
	state.Set("pipeline.code", map[string]any{
		"summary": "one edit",
		"edits": []any{
			map[string]any{"path": target, "action": "update", "content": "hello world", "summary": "update file"},
		},
	})

	record, err := ApplyEditIntentArtifacts(context.Background(), registry, state)
	require.NoError(t, err)
	require.NotNil(t, record)
	require.Len(t, record.Requested, 1)
	require.Len(t, record.Executed, 1)
	require.Empty(t, record.Rejected)

	data, readErr := os.ReadFile(target)
	require.NoError(t, readErr)
	require.Equal(t, "hello world", string(data))
}

func TestApplyEditIntentArtifactsHonorsWritePathPrechecks(t *testing.T) {
	registry := capability.NewRegistry()
	require.NoError(t, registry.Register(eucloFileWriteTool{}))
	registry.AddPrecheck(capability.WritePathPrecheck{Globs: []string{"**/*.md"}})

	target := filepath.Join(t.TempDir(), "main.go")
	state := core.NewContext()
	state.Set("pipeline.code", map[string]any{
		"summary": "one edit",
		"edits": []any{
			map[string]any{"path": target, "action": "update", "content": "package main", "summary": "update file"},
		},
	})

	record, err := ApplyEditIntentArtifacts(context.Background(), registry, state)
	require.NoError(t, err)
	require.NotNil(t, record)
	require.Len(t, record.Rejected, 1)
	require.Contains(t, record.Rejected[0].Error, "blocked")
}

func TestApplyEditIntentArtifactsRejectsDeletesWithoutCapability(t *testing.T) {
	registry := capability.NewRegistry()
	state := core.NewContext()
	state.Set("pipeline.code", map[string]any{
		"summary": "delete file",
		"edits": []any{
			map[string]any{"path": "legacy.txt", "action": "delete", "summary": "remove file"},
		},
	})

	record, err := ApplyEditIntentArtifacts(context.Background(), registry, state)
	require.NoError(t, err)
	require.NotNil(t, record)
	require.Len(t, record.Requested, 1)
	require.Len(t, record.Rejected, 1)
	require.Equal(t, "file_delete", record.Rejected[0].Tool)
}
