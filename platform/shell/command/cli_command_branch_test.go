package command

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCommandToolHelperBranches(t *testing.T) {
	base := t.TempDir()
	rootCargo := filepath.Join(base, "Cargo.toml")
	require.NoError(t, os.WriteFile(rootCargo, []byte("[package]\nname = \"root\"\nversion = \"0.1.0\"\n"), 0o644))

	crate := filepath.Join(base, "nested")
	require.NoError(t, os.MkdirAll(crate, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(crate, "Cargo.toml"), []byte("[package]\nname = \"nested\"\nversion = \"0.1.0\"\n"), 0o644))

	tool := NewCommandTool(base, CommandToolConfig{
		Name:         "cli_cargo",
		Description:  "cargo",
		Command:      "cargo",
		DefaultArgs:  []string{"build"},
		HITLRequired: true,
	})

	perms := tool.Permissions()
	require.NotNil(t, perms.Permissions)
	require.Len(t, perms.Permissions.Executables, 1)
	require.True(t, perms.Permissions.Executables[0].HITLRequired)

	require.Equal(t, filepath.Join(base, "nested"), resolvePath(base, "nested"))
	require.Equal(t, "nested", resolvePath("", "nested"))
	abs := filepath.Join(base, "absolute")
	require.Equal(t, abs, resolvePath(base, abs))

	require.Equal(t, "", mapStringArg(nil, "missing"))
	require.Equal(t, "", mapStringArg(map[string]interface{}{"missing": nil}, "missing"))

	var nilTool *CommandTool
	require.Equal(t, []string{"go"}, nilTool.prepareArgsForWorkingDir([]string{"go"}, base))
	require.False(t, nilTool.shouldIsolateCargoRun(base, []string{"test"}))

	require.Equal(t, []string{"test", "--manifest-path", filepath.Join(crate, "Cargo.toml")}, tool.prepareArgsForWorkingDir([]string{"test"}, crate))
	require.Equal(t, []string{"--manifest-path", filepath.Join(crate, "Cargo.toml"), "--verbose"}, tool.prepareArgsForWorkingDir([]string{"--verbose"}, crate))
	require.Equal(t, []string{"--manifest-path", filepath.Join(crate, "Cargo.toml")}, withManifestPath(nil, filepath.Join(crate, "Cargo.toml")))
	require.Equal(t, []string{"test", "--manifest-path", filepath.Join(crate, "Cargo.toml")}, withManifestPath([]string{"test"}, filepath.Join(crate, "Cargo.toml")))
	require.Equal(t, []string{"--manifest-path", filepath.Join(crate, "Cargo.toml"), "--verbose"}, withManifestPath([]string{"--verbose"}, filepath.Join(crate, "Cargo.toml")))
	require.Equal(t, rootCargo, findParentCargoManifest(crate, base))
	require.Empty(t, findParentCargoManifest(base, base))
	require.False(t, tool.shouldIsolateCargoRun(crate, []string{"fmt"}))
	require.True(t, tool.shouldIsolateCargoRun(crate, []string{"test"}))
	require.False(t, tool.shouldIsolateCargoRun("", []string{"test"}))
	require.False(t, tool.shouldIsolateCargoRun(crate, nil))

	workdir, args, cleanup, err := tool.prepareExecution(crate, []string{"fmt"})
	require.NoError(t, err)
	require.Equal(t, crate, workdir)
	require.Equal(t, []string{"fmt"}, args)
	cleanup()
	noIsoWorkdir, noIsoArgs, noIsoCleanup, err := tool.prepareExecution(crate, nil)
	require.NoError(t, err)
	require.Equal(t, crate, noIsoWorkdir)
	require.Nil(t, noIsoArgs)
	noIsoCleanup()

	_, err = isolateCargoWorkdir(filepath.Join(base, "missing"))
	require.Error(t, err)

	require.Error(t, copyFile(filepath.Join(base, "missing.txt"), filepath.Join(t.TempDir(), "out.txt"), 0o644))
	require.Error(t, copyDir(filepath.Join(base, "missing"), filepath.Join(t.TempDir(), "mirror")))
}

func TestCommandToolExecuteWithNoRunnerStillReportsAvailability(t *testing.T) {
	tool := NewCommandTool(t.TempDir(), CommandToolConfig{
		Name:        "cli_echo",
		Description: "echo",
		Command:     "echo",
	})
	require.False(t, tool.IsAvailable(nil, nil))
	tool.SetCommandRunner(&responseRunner{})
	tool.SetPermissionManager(nil, "agent")
	tool.SetAgentSpec(nil, "agent")
	require.True(t, tool.IsAvailable(nil, nil))
	require.Equal(t, "agent", tool.agentID)
	require.Empty(t, tool.Tags())
}
