package langgo_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"codeburg.org/lexbit/relurpify/platform/tools/native/langgo"
)

func TestConstructors_KeysAndWiring(t *testing.T) {
	constructors := langgo.Constructors()

	wantKeys := []string{"go_build", "go_module_metadata", "go_test", "go_workspace_detect"}
	require.Len(t, constructors, len(wantKeys))
	for _, key := range wantKeys {
		require.Contains(t, constructors, key)
	}

	for key, ctor := range constructors {
		tool := ctor(t.TempDir())
		require.NotNil(t, tool, "constructor %q returned nil", key)
		require.Equal(t, key, tool.Name(), "tool name must match registry key")
		require.NotEmpty(t, tool.Description())
		require.NotEmpty(t, tool.Category())
		require.NotNil(t, tool.Parameters())
		require.NotNil(t, tool.Permissions().Permissions)
	}
}

func TestConstructors_WorkspaceDetectFindsGoModule(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(
		filepath.Join(dir, "go.mod"), []byte("module example.com/tmp\n\ngo 1.27\n"), 0o600))

	detect := langgo.Constructors()["go_workspace_detect"](dir)
	res, err := detect.Execute(context.Background(), map[string]any{})
	require.NoError(t, err)
	require.NotNil(t, res)
	require.True(t, res.Success, "workspace detect must succeed in a go module dir: %+v", res)

	data, err := json.Marshal(res.Data)
	require.NoError(t, err)
	require.Contains(t, string(data), "Go module detected at")
	require.Contains(t, string(data), dir)
}

func TestConstructors_WorkspaceDetectReportsMiss(t *testing.T) {
	detect := langgo.Constructors()["go_workspace_detect"](t.TempDir())
	res, err := detect.Execute(context.Background(), map[string]any{})
	require.NoError(t, err)
	require.NotNil(t, res)
	require.False(t, res.Success, "detect outside a go module must report a miss")
	require.NotEmpty(t, res.Error)
}
