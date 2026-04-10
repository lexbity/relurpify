package execute

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestNewPresetAppliesDefaults(t *testing.T) {
	preset := NewPreset(CommandPreset{})
	require.Equal(t, "cli", preset.Category)
	require.Equal(t, 60*time.Second, preset.Timeout)
	require.Equal(t, "workspace", preset.WorkdirMode)
}

func TestExecutorExecuteBuildsRequestsAndMetadata(t *testing.T) {
	base := t.TempDir()
	runner := &recordingRunner{stdout: "out", stderr: "err"}
	exec := NewExecutor(base, CommandPreset{
		Name:        "cli_echo",
		Command:     "echo",
		DefaultArgs: []string{"-n"},
		Timeout:     5 * time.Second,
		AllowStdin:  true,
	}, runner)

	envelope, err := exec.Execute(context.Background(), "nested", []interface{}{"hello"}, "stdin")
	require.NoError(t, err)
	require.True(t, envelope.Success)
	require.Equal(t, "out", envelope.Stdout)
	require.Equal(t, "err", envelope.Stderr)
	require.Equal(t, []string{"echo", "-n", "hello"}, envelope.Command)
	require.Equal(t, "cli_echo", envelope.Preset)
	require.Equal(t, "echo", envelope.Metadata["command"])
	require.Equal(t, []string{"-n", "hello"}, envelope.Metadata["args"])
	require.Equal(t, filepath.Join(base, "nested"), envelope.Workdir)
	require.Equal(t, filepath.Join(base, "nested"), envelope.Metadata["work_dir"])
	require.NotZero(t, envelope.Elapsed)
	require.Len(t, runner.requests, 1)
	require.Equal(t, filepath.Join(base, "nested"), runner.requests[0].Workdir)
	require.Equal(t, []string{"echo", "-n", "hello"}, runner.requests[0].Args)
	require.Equal(t, "stdin", runner.requests[0].Input)
	require.Equal(t, 5*time.Second, runner.requests[0].Timeout)
}

func TestExecutorRejectsMissingRunnerAndPropagatesErrors(t *testing.T) {
	base := t.TempDir()
	_, err := NewExecutor(base, CommandPreset{Name: "cli_echo", Command: "echo"}, nil).Execute(context.Background(), "", nil, "")
	require.Error(t, err)
	require.Contains(t, err.Error(), "command runner missing")

	runner := &recordingRunner{err: errors.New("boom")}
	envelope, err := NewExecutor(base, CommandPreset{Name: "cli_echo", Command: "echo"}, runner).Execute(context.Background(), "", []interface{}{"hello"}, "")
	require.NoError(t, err)
	require.False(t, envelope.Success)
	require.Equal(t, "boom", envelope.Error)
	require.Len(t, runner.requests, 1)
}

func TestExecutorResolvesCargoHelpersAndCopiesTrees(t *testing.T) {
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

	exec := NewExecutor(base, CommandPreset{Name: "cli_cargo", Command: "cargo"}, &recordingRunner{})

	require.Equal(t, filepath.Join(base, "nested"), resolvePath(base, "nested"))
	require.Equal(t, "nested", resolvePath("", "nested"))
	require.Equal(t, filepath.Join(base, "nested"), resolvePath("", filepath.Join(base, "nested")))
	converted, err := toStringSlice([]interface{}{"123"})
	require.NoError(t, err)
	require.Equal(t, []string{"123"}, converted)
	require.Equal(t, []string{"--manifest-path", filepath.Join(crate, "Cargo.toml")}, exec.prepareArgsForWorkingDir(nil, crate))
	require.Equal(t, []string{"test", "--manifest-path", filepath.Join(crate, "Cargo.toml")}, exec.prepareArgsForWorkingDir([]string{"test"}, crate))
	require.Equal(t, []string{"--manifest-path", filepath.Join(crate, "Cargo.toml"), "--verbose"}, exec.prepareArgsForWorkingDir([]string{"--verbose"}, crate))
	require.Equal(t, rootCargo, findParentCargoManifest(crate, base))
	require.Empty(t, findParentCargoManifest(base, base))
	require.True(t, exec.shouldIsolateCargoRun(crate, []string{"test"}))
	require.False(t, exec.shouldIsolateCargoRun(crate, []string{"fmt"}))

	workdir, args, cleanup, err := exec.prepareExecution(crate, []string{"test"})
	require.NoError(t, err)
	require.Equal(t, base, workdir)
	require.Len(t, args, 3)
	require.Equal(t, "test", args[0])
	require.Equal(t, "--manifest-path", args[1])
	require.NotContains(t, args[2], base)
	_, err = os.Stat(args[2])
	require.NoError(t, err)
	cleanup()
	_, err = os.Stat(args[2])
	require.Error(t, err)

	isolated, err := isolateCargoWorkdir(crate)
	require.NoError(t, err)
	require.NoError(t, os.RemoveAll(filepath.Dir(isolated)))

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
