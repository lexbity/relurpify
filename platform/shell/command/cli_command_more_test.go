package command

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"codeburg.org/lexbit/relurpify/framework/core"
	"github.com/stretchr/testify/require"
)

func TestCommandToolMetadataAndPermissionDefaults(t *testing.T) {
	base := t.TempDir()
	tool := NewCommandTool(base, CommandToolConfig{
		Name:        "cli_echo",
		Description: "echo",
		Command:     "echo",
		DefaultArgs: []string{"-n"},
		Tags:        []string{core.TagExecute},
	})

	require.Equal(t, "cli", tool.Category())
	require.Equal(t, "cli_echo", tool.Name())
	require.Equal(t, "echo", tool.Description())
	require.Len(t, tool.Parameters(), 3)
	require.False(t, tool.IsAvailable(context.Background(), core.NewContext()))

	runner := &responseRunner{}
	tool.SetCommandRunner(runner)
	require.True(t, tool.IsAvailable(context.Background(), core.NewContext()))

	perms := tool.Permissions()
	require.NotNil(t, perms.Permissions)
	require.Len(t, perms.Permissions.Executables, 1)
	require.Equal(t, "echo", perms.Permissions.Executables[0].Binary)
	require.Equal(t, []string{"-n"}, perms.Permissions.Executables[0].Args)
}

func TestCommandToolExecuteBuildsRequestAndMapsResult(t *testing.T) {
	base := t.TempDir()
	runner := &responseRunner{stdout: "out", stderr: "err"}
	tool := NewCommandTool(base, CommandToolConfig{
		Name:        "cli_echo",
		Description: "echo",
		Command:     "echo",
		DefaultArgs: []string{"-n"},
		Timeout:     5 * time.Second,
		Tags:        []string{core.TagExecute},
	})
	tool.SetCommandRunner(runner)

	result, err := tool.Execute(context.Background(), core.NewContext(), map[string]interface{}{
		"args":              []interface{}{"hello"},
		"stdin":             "stdin",
		"working_directory": "nested",
	})
	require.NoError(t, err)
	require.True(t, result.Success)
	require.Equal(t, "out", result.Data["stdout"])
	require.Equal(t, "err", result.Data["stderr"])
	require.Equal(t, "", result.Error)
	require.Equal(t, "echo", result.Metadata["command"])
	require.Equal(t, []string{"-n", "hello"}, result.Metadata["args"])
	require.Equal(t, filepath.Join(base, "nested"), result.Metadata["work_dir"])
	require.Equal(t, "cli_echo", result.Metadata["preset"])
	require.Len(t, runner.requests, 1)
	require.Equal(t, []string{"echo", "-n", "hello"}, runner.requests[0].Args)
	require.Equal(t, filepath.Join(base, "nested"), runner.requests[0].Workdir)
	require.Equal(t, "stdin", runner.requests[0].Input)
	require.Equal(t, 5*time.Second, runner.requests[0].Timeout)
}

func TestCommandToolExecuteReportsRunnerErrors(t *testing.T) {
	base := t.TempDir()
	runner := &responseRunner{stdout: "out", stderr: "err", err: errors.New("boom")}
	tool := NewCommandTool(base, CommandToolConfig{
		Name:        "cli_echo",
		Description: "echo",
		Command:     "echo",
		Tags:        []string{core.TagExecute},
	})
	tool.SetCommandRunner(runner)

	result, err := tool.Execute(context.Background(), core.NewContext(), map[string]interface{}{
		"args": []interface{}{"hello"},
	})
	require.NoError(t, err)
	require.False(t, result.Success)
	require.Equal(t, "boom", result.Error)
	require.Equal(t, "out", result.Data["stdout"])
	require.Equal(t, "err", result.Data["stderr"])
	require.Len(t, runner.requests, 1)
	require.Equal(t, []string{"echo", "hello"}, runner.requests[0].Args)
}

func TestCommandToolPermissionsAndCargoHelpers(t *testing.T) {
	base := t.TempDir()
	rootCargo := filepath.Join(base, "Cargo.toml")
	require.NoError(t, os.WriteFile(rootCargo, []byte("[package]\nname = \"root\"\nversion = \"0.1.0\"\n"), 0o644))

	crate := filepath.Join(base, "nested")
	require.NoError(t, os.MkdirAll(filepath.Join(crate, "src"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(crate, "Cargo.toml"), []byte("[package]\nname = \"nested\"\nversion = \"0.1.0\"\nedition = \"2021\"\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(crate, "src", "lib.rs"), []byte("pub fn add(a:i32,b:i32)->i32{a+b}\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(crate, "keep.txt"), []byte("keep"), 0o644))
	require.NoError(t, os.MkdirAll(filepath.Join(crate, ".git"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(crate, ".git", "config"), []byte("skip"), 0o644))
	require.NoError(t, os.MkdirAll(filepath.Join(crate, "target"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(crate, "target", "artifact"), []byte("skip"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(crate, "ignore.bak"), []byte("skip"), 0o644))

	cargo := NewCommandTool(base, CommandToolConfig{
		Name:        "cli_cargo",
		Description: "cargo",
		Command:     "cargo",
	})

	require.Equal(t, filepath.Join(base, "nested"), resolvePath(base, "nested"))
	require.Equal(t, "nested", resolvePath("", "nested"))
	require.Equal(t, filepath.Join(base, "nested"), resolvePath("", filepath.Join(base, "nested")))
	require.Equal(t, "123", mapStringArg(map[string]interface{}{"value": 123}, "value"))
	require.Equal(t, "hello", mapStringArg(map[string]interface{}{"value": "hello"}, "value"))
	require.Empty(t, mapStringArg(nil, "value"))

	require.Equal(t, []string{"--manifest-path", filepath.Join(crate, "Cargo.toml")}, cargo.prepareArgsForWorkingDir(nil, crate))
	require.Equal(t, []string{"test", "--manifest-path", filepath.Join(crate, "Cargo.toml")}, cargo.prepareArgsForWorkingDir([]string{"test"}, crate))
	require.Equal(t, []string{"--manifest-path", filepath.Join(crate, "Cargo.toml"), "--verbose"}, cargo.prepareArgsForWorkingDir([]string{"--verbose"}, crate))
	require.Equal(t, []string{"test", "--manifest-path", filepath.Join(crate, "Cargo.toml")}, cargo.prepareArgsForWorkingDir([]string{"test", "--manifest-path", filepath.Join(crate, "Cargo.toml")}, crate))
	require.Equal(t, []string{"echo"}, NewCommandTool(base, CommandToolConfig{Name: "cli_echo", Command: "echo"}).prepareArgsForWorkingDir([]string{"echo"}, crate))

	require.Equal(t, rootCargo, findParentCargoManifest(crate, base))
	require.Empty(t, findParentCargoManifest(base, base))

	workdir, args, cleanup, err := cargo.prepareExecution(crate, []string{"test"})
	require.NoError(t, err)
	require.Equal(t, base, workdir)
	require.Len(t, args, 3)
	require.Equal(t, "test", args[0])
	require.Equal(t, "--manifest-path", args[1])
	require.NotContains(t, args[2], base)
	_, statErr := os.Stat(args[2])
	require.NoError(t, statErr)
	cleanup()
	_, cleanupErr := os.Stat(args[2])
	require.Error(t, cleanupErr)

	isolated, err := isolateCargoWorkdir(crate)
	require.NoError(t, err)
	t.Cleanup(func() { _ = os.RemoveAll(filepath.Dir(isolated)) })
	require.NoError(t, copyDir(crate, filepath.Join(t.TempDir(), "copy")))

	mirror := filepath.Join(t.TempDir(), "mirror")
	require.NoError(t, copyDir(crate, mirror))
	_, err = os.Stat(filepath.Join(mirror, "keep.txt"))
	require.NoError(t, err)
	_, err = os.Stat(filepath.Join(mirror, "src", "lib.rs"))
	require.NoError(t, err)
	_, err = os.Stat(filepath.Join(mirror, ".git"))
	require.Error(t, err)
	_, err = os.Stat(filepath.Join(mirror, "target"))
	require.Error(t, err)
	_, err = os.Stat(filepath.Join(mirror, "ignore.bak"))
	require.Error(t, err)

	copied := filepath.Join(t.TempDir(), "copied.txt")
	require.NoError(t, copyFile(filepath.Join(crate, "keep.txt"), copied, 0o644))
	data, err := os.ReadFile(copied)
	require.NoError(t, err)
	require.Equal(t, "keep", string(data))
}
