package langjs_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"codeburg.org/lexbit/relurpify/platform/tools/native/langjs"
)

func TestConstructors_KeysAndWiring(t *testing.T) {
	constructors := langjs.Constructors()

	wantKeys := []string{"node_npm_test", "node_project_metadata", "node_syntax_check", "node_workspace_detect"}
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

func TestConstructors_WorkspaceDetectFindsPackageJSON(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(
		filepath.Join(dir, "package.json"), []byte(`{"name":"tmp","version":"0.1.0"}`), 0o600))

	detect := langjs.Constructors()["node_workspace_detect"](dir)
	res, err := detect.Execute(context.Background(), map[string]any{})
	require.NoError(t, err)
	require.NotNil(t, res)
	require.True(t, res.Success, "workspace detect must succeed in an npm project: %+v", res)

	data, err := json.Marshal(res.Data)
	require.NoError(t, err)
	require.Contains(t, string(data), "tmp")
}
