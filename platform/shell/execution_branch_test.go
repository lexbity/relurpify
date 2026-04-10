package shell

import (
	"context"
	"testing"
	"time"

	"github.com/lexcodex/relurpify/platform/shell/execute"
	"github.com/stretchr/testify/require"
)

func TestExecutionToolPermissionEdgeBranches(t *testing.T) {
	base := t.TempDir()

	testsTool := &RunTestsTool{Workdir: base}
	testsPerms := testsTool.Permissions()
	require.Len(t, testsPerms.Permissions.FileSystem, 2)
	require.Len(t, testsPerms.Permissions.Executables, 0)
	_, _, err := testsTool.run(context.Background(), []string{"go", "test"}, "")
	require.Error(t, err)

	codeTool := &ExecuteCodeTool{Workdir: base}
	codePerms := codeTool.Permissions()
	require.Len(t, codePerms.Permissions.FileSystem, 1)
	require.Len(t, codePerms.Permissions.Executables, 0)
	_, _, err = codeTool.run(context.Background(), []string{"python", "-c", "print(1)"}, "")
	require.Error(t, err)

	lintTool := &RunLinterTool{Workdir: base}
	lintPerms := lintTool.Permissions()
	require.Len(t, lintPerms.Permissions.FileSystem, 1)
	require.Len(t, lintPerms.Permissions.Executables, 0)
	_, _, err = lintTool.run(context.Background(), []string{"golangci-lint", "run"})
	require.Error(t, err)

	buildTool := &RunBuildTool{Workdir: base}
	buildPerms := buildTool.Permissions()
	require.Len(t, buildPerms.Permissions.FileSystem, 1)
	require.Len(t, buildPerms.Permissions.Executables, 0)
	_, _, err = buildTool.run(context.Background())
	require.Error(t, err)
}

func TestNewExecutorKeepsExplicitPresetValues(t *testing.T) {
	exec := execute.NewExecutor("", execute.CommandPreset{
		Name:        "custom",
		Command:     "echo",
		Category:    "custom",
		Timeout:     7 * time.Second,
		WorkdirMode: "fixed",
	}, nil)
	require.Equal(t, "custom", exec.Preset.Category)
	require.Equal(t, 7*time.Second, exec.Preset.Timeout)
	require.Equal(t, "fixed", exec.Preset.WorkdirMode)
	require.Nil(t, exec.Runner)
}
