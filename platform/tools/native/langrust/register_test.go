package langrust_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"codeburg.org/lexbit/relurpify/platform/tools/native/langrust"
)

func TestConstructors_KeysAndWiring(t *testing.T) {
	constructors := langrust.Constructors()

	wantKeys := []string{"rust_cargo_check", "rust_cargo_metadata", "rust_cargo_test", "rust_workspace_detect"}
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

func TestConstructors_WorkspaceDetectWithoutCargoToml(t *testing.T) {
	detect := langrust.Constructors()["rust_workspace_detect"](t.TempDir())
	res, err := detect.Execute(context.Background(), map[string]any{})
	require.NoError(t, err, "workspace detect must not error outside a cargo project")
	require.NotNil(t, res)
	require.False(t, res.Success, "empty dir must not be detected as a cargo project")
}
