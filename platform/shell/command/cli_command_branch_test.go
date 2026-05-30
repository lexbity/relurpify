package command

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"codeburg.org/lexbit/relurpify/platform/shell/execute"
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
	require.False(t, nilTool.shouldIsolateCargoRun(base, []string{"test"}))

	require.Equal(t, rootCargo, findParentCargoManifest(crate, base))
	require.Empty(t, findParentCargoManifest(base, base))
	require.False(t, tool.shouldIsolateCargoRun(crate, []string{"fmt"}))
	require.True(t, tool.shouldIsolateCargoRun(crate, []string{"test"}))
	require.False(t, tool.shouldIsolateCargoRun("", []string{"test"}))
	require.False(t, tool.shouldIsolateCargoRun(crate, nil))
}

func TestCommandToolBlockFlagsByDefault(t *testing.T) {
	runner := &responseRunner{stdout: "ok"}
	tool := NewCommandTool(t.TempDir(), CommandToolConfig{
		Name:        "cli_echo",
		Description: "echo",
		Command:     "echo",
	})
	tool.SetCommandRunner(runner)

	_, err := tool.Execute(context.Background(), map[string]interface{}{
		"args": []interface{}{"--version"},
	})
	if err == nil {
		t.Fatal("expected flag injection error when AllowFlags is not set")
	}
	if !strings.Contains(err.Error(), "flag injection") {
		t.Fatalf("expected 'flag injection' in error, got: %v", err)
	}
}

func TestCommandToolAllowFlagsOptIn(t *testing.T) {
	runner := &responseRunner{stdout: "ok"}
	tool := NewCommandTool(t.TempDir(), CommandToolConfig{
		Name:        "cli_echo",
		Description: "echo",
		Command:     "echo",
	})
	tool.SetCommandRunner(runner)

	// When the caller explicitly constructs an executor with AllowFlags=true,
	// flags are allowed. The CommandTool itself doesn't expose AllowFlags in
	// its config, but the Executor checks AllowFlags via the CommandPreset.
	executor := execute.NewExecutor(t.TempDir(), execute.CommandPreset{
		Name:       "cli_echo",
		Command:    "echo",
		AllowFlags: true,
	}, runner)
	envelope, err := executor.Execute(context.Background(), "", []interface{}{"--version"}, "")
	if err != nil {
		t.Fatalf("expected no error when AllowFlags=true, got: %v", err)
	}
	if !envelope.Success {
		t.Fatal("expected success")
	}
	require.Len(t, runner.requests, 1)
	require.Equal(t, "--version", runner.requests[len(runner.requests)-1].Args[1])
}

func TestCommandToolExecuteWithNoRunnerStillReportsAvailability(t *testing.T) {
	tool := NewCommandTool(t.TempDir(), CommandToolConfig{
		Name:        "cli_echo",
		Description: "echo",
		Command:     "echo",
	})
	require.False(t, tool.IsAvailable(nil))
	tool.SetCommandRunner(&responseRunner{})
	require.True(t, tool.IsAvailable(nil))
	require.Empty(t, tool.Tags())
}
