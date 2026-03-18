package euclotest

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/lexcodex/relurpify/framework/capability"
	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/named/euclo/internal/testutil"
	eucloruntime "github.com/lexcodex/relurpify/named/euclo/runtime"
	"github.com/stretchr/testify/require"
)

func TestApplyEditIntentArtifactsExecutesWritesThroughCapabilityRegistry(t *testing.T) {
	registry := capability.NewRegistry()
	require.NoError(t, registry.Register(testutil.FileWriteTool{}))

	target := filepath.Join(t.TempDir(), "hello.txt")
	state := core.NewContext()
	state.Set("pipeline.code", map[string]any{
		"summary": "one edit",
		"edits": []any{
			map[string]any{"path": target, "action": "update", "content": "hello world", "summary": "update file"},
		},
	})

	record, err := eucloruntime.ApplyEditIntentArtifacts(context.Background(), registry, state)
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
	require.NoError(t, registry.Register(testutil.FileWriteTool{}))
	registry.AddPrecheck(capability.WritePathPrecheck{Globs: []string{"**/*.md"}})

	target := filepath.Join(t.TempDir(), "main.go")
	state := core.NewContext()
	state.Set("pipeline.code", map[string]any{
		"summary": "one edit",
		"edits": []any{
			map[string]any{"path": target, "action": "update", "content": "package main", "summary": "update file"},
		},
	})

	record, err := eucloruntime.ApplyEditIntentArtifacts(context.Background(), registry, state)
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

	record, err := eucloruntime.ApplyEditIntentArtifacts(context.Background(), registry, state)
	require.NoError(t, err)
	require.NotNil(t, record)
	require.Len(t, record.Requested, 1)
	require.Len(t, record.Rejected, 1)
	require.Equal(t, "file_delete", record.Rejected[0].Tool)
}
