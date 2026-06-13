package subprocess

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"codeburg.org/lexbit/relurpify/capability/ports"
	"codeburg.org/lexbit/relurpify/platform/fs"
)

const (
	build = "build"
	cargo = "cargo"
	cargo_toml = "Cargo.toml"
	cli_cargo = "cli_cargo"
	_fmt = "_fmt"
	manifest_path = "--manifest-path"
	nested = "nested"
	release = "--release"
	skip = "skip"
	src = "src"
	target = "target"
	test = "test"
	working_directory = "working_directory"
	workspace_n = "[workspace]\n"
)


func TestCargoIsolationNotAppliedToNonCargoTool(t *testing.T) {
	runner := &recordingRunner{stdout: ok}
	tool := NewTool(ports.ToolManifest{
		Name:   "cli_echo",
		Family: text,
		Execution: ports.ToolManifestExecution{
			Backend: ports.ToolBackendSubprocess,
			Command: &ports.ToolManifestCommand{Base: []string{echo}},
		},
		SourcePath: t.TempDir(),
	}, runner)

	result, err := tool.Execute(context.Background(), map[string]any{
		args:              []any{hello},
		working_directory: ".",
	})
	require.NoError(t, err)
	require.True(t, result.Success)
	require.Len(t, runner.requests, 1)
	require.Equal(t, []string{echo, hello}, runner.requests[0].Args)
}

func TestCargoIsolationStandaloneCrateNotIsolated(t *testing.T) {
	base := t.TempDir()
	crateDir := filepath.Join(base, "my_crate")
	require.NoError(t, fs.MkdirAllSecure(crateDir))
	require.NoError(t, fs.WriteFileSecure(filepath.Join(crateDir, cargo_toml), []byte("[package]\nname = \"demo\"\nversion = \"0.1.0\"\n")))

	runner := &recordingRunner{stdout: ok}
	tool := NewTool(ports.ToolManifest{
		Name:   cli_cargo,
		Family: build,
		Execution: ports.ToolManifestExecution{
			Backend: ports.ToolBackendSubprocess,
			Command: &ports.ToolManifestCommand{Base: []string{cargo}},
		},
		SourcePath: base,
	}, runner)

	result, err := tool.Execute(context.Background(), map[string]any{
		args:              []any{test},
		working_directory: crateDir,
	})
	require.NoError(t, err)
	require.True(t, result.Success)
	require.Len(t, runner.requests, 1)
	// Standalone crate with no parent workspace: no isolation, workdir stays
	require.Equal(t, []string{cargo, test}, runner.requests[0].Args)
	require.Equal(t, crateDir, runner.requests[0].Workdir)
}

func TestCargoIsolationNestedWorkspaceIsolated(t *testing.T) {
	base := t.TempDir()
	// Root workspace Cargo.toml
	require.NoError(t, fs.WriteFileSecure(filepath.Join(base, cargo_toml), []byte("[workspace]\nmembers = [\"nested\"]\n")))

	// Nested crate
	crateDir := filepath.Join(base, nested)
	require.NoError(t, fs.MkdirAllSecure(filepath.Join(crateDir, src)))
	require.NoError(t, fs.WriteFileSecure(filepath.Join(crateDir, cargo_toml), []byte("[package]\nname = \"demo\"\nversion = \"0.1.0\"\nedition = \"2021\"\n")))
	require.NoError(t, fs.WriteFileSecure(filepath.Join(crateDir, src, "lib.rs"), []byte("pub fn add(a:i32,b:i32)->i32{a+b}\n")))

	runner := &recordingRunner{stdout: ok}
	tool := NewTool(ports.ToolManifest{
		Name:   cli_cargo,
		Family: build,
		Execution: ports.ToolManifestExecution{
			Backend: ports.ToolBackendSubprocess,
			Command: &ports.ToolManifestCommand{Base: []string{cargo}},
		},
		SourcePath: base,
	}, runner)

	result, err := tool.Execute(context.Background(), map[string]any{
		args:              []any{test},
		working_directory: crateDir,
	})
	require.NoError(t, err)
	require.True(t, result.Success)
	require.Len(t, runner.requests, 1)

	// The command should have --manifest-path pointing to the isolated copy
	require.Equal(t, cargo, runner.requests[0].Args[0])
	require.Equal(t, test, runner.requests[0].Args[1])
	require.Equal(t, manifest_path, runner.requests[0].Args[2])
	// The manifest path should be inside a temp dir (not the real workspace)
	require.NotContains(t, runner.requests[0].Args[3], base, "isolated manifest path must not be in the workspace")
	require.Contains(t, runner.requests[0].Args[3], cargo_toml)

	// Workdir should be the workspace root (not the nested crate)
	require.Equal(t, base, runner.requests[0].Workdir)
}

func TestCargoIsolationNotAppliedForNonIsolatedSubcommands(t *testing.T) {
	base := t.TempDir()
	require.NoError(t, fs.WriteFileSecure(filepath.Join(base, cargo_toml), []byte(workspace_n)))
	crateDir := filepath.Join(base, nested)
	require.NoError(t, fs.MkdirAllSecure(crateDir))
	require.NoError(t, fs.WriteFileSecure(filepath.Join(crateDir, cargo_toml), []byte("[package]\nname = \"demo\"\n")))

	runner := &recordingRunner{stdout: ok}
	tool := NewTool(ports.ToolManifest{
		Name:   cli_cargo,
		Family: build,
		Execution: ports.ToolManifestExecution{
			Backend: ports.ToolBackendSubprocess,
			Command: &ports.ToolManifestCommand{Base: []string{cargo}},
		},
		SourcePath: base,
	}, runner)

	// _fmt is not in the isolation list
	result, err := tool.Execute(context.Background(), map[string]any{
		args:              []any{_fmt},
		working_directory: crateDir,
	})
	require.NoError(t, err)
	require.True(t, result.Success)
	require.Len(t, runner.requests, 1)
	require.Equal(t, []string{cargo, _fmt}, runner.requests[0].Args)
	require.Equal(t, crateDir, runner.requests[0].Workdir)
}

func TestCargoIsolationSkipsWhenNoArgs(t *testing.T) {
	base := t.TempDir()
	require.NoError(t, fs.WriteFileSecure(filepath.Join(base, cargo_toml), []byte(workspace_n)))

	runner := &recordingRunner{stdout: ok}
	tool := NewTool(ports.ToolManifest{
		Name:   cli_cargo,
		Family: build,
		Execution: ports.ToolManifestExecution{
			Backend: ports.ToolBackendSubprocess,
			Command: &ports.ToolManifestCommand{Base: []string{cargo}},
		},
		SourcePath: base,
	}, runner)

	// No args at all
	result, err := tool.Execute(context.Background(), nil)
	require.NoError(t, err)
	require.True(t, result.Success)
	require.Len(t, runner.requests, 1)
	require.Equal(t, []string{cargo}, runner.requests[0].Args)
}

func TestCargoIsolationAlreadyHasManifestPath(t *testing.T) {
	base := t.TempDir()
	require.NoError(t, fs.WriteFileSecure(filepath.Join(base, cargo_toml), []byte(workspace_n)))
	crateDir := filepath.Join(base, nested)
	require.NoError(t, fs.MkdirAllSecure(crateDir))
	require.NoError(t, fs.WriteFileSecure(filepath.Join(crateDir, cargo_toml), []byte("[package]\nname = \"demo\"\n")))
	existingManifest := filepath.Join(crateDir, cargo_toml)

	runner := &recordingRunner{stdout: ok}
	tool := NewTool(ports.ToolManifest{
		Name:   cli_cargo,
		Family: build,
		Execution: ports.ToolManifestExecution{
			Backend: ports.ToolBackendSubprocess,
			Command: &ports.ToolManifestCommand{Base: []string{cargo}},
		},
		SourcePath: base,
	}, runner)
	// Note: applying the hook to already-expanded cmd; we call the low-level function directly
	cmd, workdir, cleanup, err := applyCargoIsolation(tool.(*subprocessTool).manifest,
		[]string{cargo, test, manifest_path, existingManifest},
		crateDir)
	defer cleanup()
	require.NoError(t, err)
	require.Equal(t, []string{cargo, test, manifest_path, existingManifest}, cmd,
		"must not duplicate --manifest-path")
	require.Equal(t, base, workdir)
}

func TestCargoIsolationIsCargoTool(t *testing.T) {
	cargoManifest := ports.ToolManifest{
		Execution: ports.ToolManifestExecution{
			Command: &ports.ToolManifestCommand{Base: []string{cargo}},
		},
	}
	require.True(t, isCargoTool(cargoManifest))

	nonCargoManifest := ports.ToolManifest{
		Execution: ports.ToolManifestExecution{
			Command: &ports.ToolManifestCommand{Base: []string{echo}},
		},
	}
	require.False(t, isCargoTool(nonCargoManifest))

	noCommandManifest := ports.ToolManifest{
		Execution: ports.ToolManifestExecution{},
	}
	require.False(t, isCargoTool(noCommandManifest))
}

func TestCargoSubcommandExtraction(t *testing.T) {
	require.Equal(t, test, cargoSubcommand([]string{cargo, test}))
	require.Equal(t, build, cargoSubcommand([]string{cargo, build, release}))
	require.Equal(t, "check", cargoSubcommand([]string{cargo, "check"}))
	require.Equal(t, "metadata", cargoSubcommand([]string{cargo, "metadata", "--no-deps"}))
	require.Empty(t, cargoSubcommand([]string{cargo, _fmt}))
	require.Empty(t, cargoSubcommand([]string{cargo}))
	require.Empty(t, cargoSubcommand([]string{}))
}

func TestCargoCopyDirSkipsGitTargetAndBak(t *testing.T) {
	base := t.TempDir()
	src := filepath.Join(base, src)
	require.NoError(t, fs.MkdirAllSecure(filepath.Join(src, nested)))
	require.NoError(t, fs.WriteFileSecure(filepath.Join(src, "keep.txt"), []byte("keep")))
	require.NoError(t, fs.WriteFileSecure(filepath.Join(src, "ignore.bak"), []byte(skip)))
	require.NoError(t, fs.MkdirAllSecure(filepath.Join(src, ".git")))
	require.NoError(t, fs.WriteFileSecure(filepath.Join(src, ".git", "config"), []byte(skip)))
	require.NoError(t, fs.MkdirAllSecure(filepath.Join(src, target)))
	require.NoError(t, fs.WriteFileSecure(filepath.Join(src, target, "artifact"), []byte(skip)))

	dst := filepath.Join(t.TempDir(), "mirror")
	require.NoError(t, copyDir(src, dst))

	_, err := os.Stat(filepath.Join(dst, "keep.txt"))
	require.NoError(t, err)
	_, err = os.Stat(filepath.Join(dst, ".git"))
	require.Error(t, err, ".git must be excluded")
	_, err = os.Stat(filepath.Join(dst, target))
	require.Error(t, err, "target must be excluded")
	_, err = os.Stat(filepath.Join(dst, "ignore.bak"))
	require.Error(t, err, ".bak files must be excluded")

	// nested dirs inside git/target are also skipped
	_, err = os.Stat(filepath.Join(dst, ".git", "config"))
	require.Error(t, err)
}

func TestInjectManifestPath(t *testing.T) {
	manifestPath := "/tmp/test/Cargo.toml"

	// Already has --manifest-path
	require.Equal(t,
		[]string{cargo, test, manifest_path, "/existing/Cargo.toml"},
		injectManifestPath([]string{cargo, test, manifest_path, "/existing/Cargo.toml"}, test, manifestPath))

	// Insert after subcommand
	require.Equal(t,
		[]string{cargo, test, manifest_path, manifestPath},
		injectManifestPath([]string{cargo, test}, test, manifestPath))

	// Subcommand with flags after it
	require.Equal(t,
		[]string{cargo, test, manifest_path, manifestPath, release},
		injectManifestPath([]string{cargo, test, release}, test, manifestPath))
}
