package shell

import (
	"context"
	"errors"
	"testing"
	"time"

	"codeburg.org/lexbit/relurpify/framework/core"
	"github.com/stretchr/testify/require"
)

func TestLegacyExecutionHelpersCoverCorePaths(t *testing.T) {
	base := t.TempDir()
	runner := &recordingRunner{stdout: "stdout", stderr: "stderr"}
	spec := &core.AgentRuntimeSpec{}

	testsTool := &RunTestsTool{
		Command: []string{"go", "test"},
		Workdir: base,
		Timeout: 2 * time.Second,
		Runner:  runner,
	}
	testsTool.SetPermissionManager("manager", "agent-tests")
	testsTool.SetAgentSpec(spec, "agent-tests")
	require.Equal(t, "manager", testsTool.manager)
	require.Equal(t, spec, testsTool.spec)
	require.Equal(t, "agent-tests", testsTool.agentID)
	require.True(t, testsTool.IsAvailable(context.Background(), core.NewContext()))
	testsPerms := testsTool.Permissions()
	require.NotNil(t, testsPerms.Permissions)
	require.Len(t, testsPerms.Permissions.Executables, 1)
	require.Equal(t, "go", testsPerms.Permissions.Executables[0].Binary)
	require.Equal(t, []string{"test"}, testsPerms.Permissions.Executables[0].Args)
	result, err := testsTool.Execute(context.Background(), core.NewContext(), map[string]interface{}{"pattern": "./..."})
	require.NoError(t, err)
	require.True(t, result.Success)
	require.Equal(t, "stdout", result.Data["stdout"])
	require.Equal(t, []string{"go", "test", "./..."}, runner.requests[0].Args)

	codeRunner := &recordingRunner{stdout: "code-out", stderr: "code-err", err: nil}
	codeTool := &ExecuteCodeTool{
		Command: []string{"python3", "-c"},
		Workdir: base,
		Timeout: time.Second,
		Runner:  codeRunner,
	}
	codeTool.SetPermissionManager("manager", "agent-code")
	codeTool.SetAgentSpec(spec, "agent-code")
	require.Equal(t, "manager", codeTool.manager)
	require.Equal(t, spec, codeTool.spec)
	require.Equal(t, "agent-code", codeTool.agentID)
	require.True(t, codeTool.IsAvailable(context.Background(), core.NewContext()))
	codePerms := codeTool.Permissions()
	require.NotNil(t, codePerms.Permissions)
	require.Len(t, codePerms.Permissions.FileSystem, 4)
	require.True(t, codePerms.Permissions.FileSystem[0].HITLRequired)
	require.True(t, codePerms.Permissions.Executables[0].HITLRequired)
	codeResult, err := codeTool.Execute(context.Background(), core.NewContext(), map[string]interface{}{"code": "print(1)"})
	require.NoError(t, err)
	require.True(t, codeResult.Success)
	require.Equal(t, []string{"python3", "-c", "print(1)"}, codeRunner.requests[0].Args)

	lintRunner := &recordingRunner{stdout: "lint-out", stderr: "lint-err", err: errors.New("lint failed")}
	lintTool := &RunLinterTool{
		Command: []string{"golangci-lint", "run"},
		Workdir: base,
		Timeout: time.Second,
		Runner:  lintRunner,
	}
	lintTool.SetPermissionManager("manager", "agent-lint")
	lintTool.SetAgentSpec(spec, "agent-lint")
	require.Equal(t, "manager", lintTool.manager)
	require.Equal(t, spec, lintTool.spec)
	require.True(t, lintTool.IsAvailable(context.Background(), core.NewContext()))
	lintPerms := lintTool.Permissions()
	require.NotNil(t, lintPerms.Permissions)
	require.Len(t, lintPerms.Permissions.Executables, 1)
	require.Equal(t, "golangci-lint", lintPerms.Permissions.Executables[0].Binary)
	lintResult, err := lintTool.Execute(context.Background(), core.NewContext(), map[string]interface{}{"path": "./..."})
	require.NoError(t, err)
	require.False(t, lintResult.Success)
	require.Equal(t, "lint failed", lintResult.Error)
	require.Equal(t, []string{"golangci-lint", "run", "./..."}, lintRunner.requests[0].Args)

	buildRunner := &recordingRunner{stdout: "build-out", stderr: "build-err"}
	buildTool := &RunBuildTool{
		Command: []string{"go", "test"},
		Workdir: base,
		Timeout: time.Second,
		Runner:  buildRunner,
	}
	buildTool.SetPermissionManager("manager", "agent-build")
	buildTool.SetAgentSpec(spec, "agent-build")
	require.Equal(t, "manager", buildTool.manager)
	require.Equal(t, spec, buildTool.spec)
	require.True(t, buildTool.IsAvailable(context.Background(), core.NewContext()))
	buildPerms := buildTool.Permissions()
	require.NotNil(t, buildPerms.Permissions)
	require.Len(t, buildPerms.Permissions.Executables, 1)
	require.Equal(t, "go", buildPerms.Permissions.Executables[0].Binary)
	buildResult, err := buildTool.Execute(context.Background(), core.NewContext(), map[string]interface{}{})
	require.NoError(t, err)
	require.True(t, buildResult.Success)
	require.Equal(t, []string{"go", "test"}, buildRunner.requests[0].Args)

	empty := &RunBuildTool{Workdir: base}
	require.False(t, empty.IsAvailable(context.Background(), core.NewContext()))
	emptyPerms := empty.Permissions()
	require.NotNil(t, emptyPerms.Permissions)
	require.Len(t, emptyPerms.Permissions.FileSystem, 1)
	require.Len(t, emptyPerms.Permissions.Executables, 0)
}
