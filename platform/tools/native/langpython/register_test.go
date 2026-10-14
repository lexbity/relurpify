package langpython_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"codeburg.org/lexbit/relurpify/platform/tools/native/langpython"
)

func TestConstructors_KeysAndWiring(t *testing.T) {
	constructors := langpython.Constructors()

	wantKeys := []string{
		"python_compile_check", "python_project_metadata",
		"python_pytest", "python_unittest", "python_workspace_detect",
	}
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

func TestConstructors_WorkspaceDetectWithoutProject(t *testing.T) {
	detect := langpython.Constructors()["python_workspace_detect"](t.TempDir())
	res, err := detect.Execute(context.Background(), map[string]any{})
	require.NoError(t, err, "workspace detect must not error outside a python project")
	require.NotNil(t, res)
	require.False(t, res.Success, "empty dir must not be detected as a python project")
}
