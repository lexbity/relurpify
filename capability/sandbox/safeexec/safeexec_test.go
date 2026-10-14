package safeexec

import (
	"context"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPrepareSanitizesNameWithBaseAndArgsWithClean(t *testing.T) {
	prepared := Prepare("/usr/local/bin/tool", "../sneaky/./path", "/etc//passwd", "plain.txt")

	assert.Equal(t, "tool", prepared.Name, "the command name is reduced to its base")
	assert.Equal(t, []string{
		filepath.Clean("../sneaky/./path"),
		filepath.Clean("/etc//passwd"),
		"plain.txt",
	}, prepared.Args)
}

func TestPrepareEmptyNameAndNoArgs(t *testing.T) {
	prepared := Prepare("")
	assert.Equal(t, ".", prepared.Name, "filepath.Base of the empty string is a single dot")
	assert.NotNil(t, prepared.Args)
	assert.Empty(t, prepared.Args)
}

func TestCommandResolvesPathAndBuildsArgv(t *testing.T) {
	cmd := Command(Prepare("echo", "hello"))

	require.NotNil(t, cmd)
	assert.True(t, strings.ContainsRune(cmd.Path, filepath.Separator),
		"a found binary must be resolved to a path, got %q", cmd.Path)
	assert.Equal(t, "echo", cmd.Args[0], "argv[0] stays the sanitized base name")
	assert.Equal(t, "hello", cmd.Args[1])
	assert.Len(t, cmd.Args, 2)

	// The prepared command actually runs and behaves like the real binary.
	out, err := cmd.Output()
	require.NoError(t, err)
	assert.Equal(t, "hello\n", string(out))
}

func TestCommandKeepsBaseNameWhenLookupFails(t *testing.T) {
	const missing = "definitely-not-a-real-binary-42f9"

	cmd := Command(Prepare(missing, "arg"))
	require.NotNil(t, cmd)
	assert.Equal(t, missing, cmd.Path, "an unresolvable name is left for the Run error to surface")
	assert.Equal(t, []string{missing, "arg"}, cmd.Args)
}

func TestCommandContextKillsProcessOnCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	cmd := CommandContext(ctx, Prepare("sleep", "30"))
	require.NotNil(t, cmd)
	require.NoError(t, cmd.Start())

	// Give the watcher goroutine a moment to observe the started process.
	time.Sleep(100 * time.Millisecond)

	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()

	cancel()

	select {
	case err := <-done:
		require.Error(t, err, "a killed process must surface an error from Wait")
		assert.ErrorContains(t, err, "killed")
	case <-time.After(5 * time.Second):
		if cmd.Process != nil {
			_ = cmd.Process.Kill()
		}
		t.Fatal("cancelling the context did not terminate the process within 5s")
	}
}

func TestCommandContextWithoutStartDoesNotBlock(t *testing.T) {
	// Cancelling a context whose command was never started must be safe: the
	// watcher goroutine observes a nil Process and exits.
	ctx, cancel := context.WithCancel(context.Background())
	cmd := CommandContext(ctx, Prepare("sleep", "30"))
	require.NotNil(t, cmd)
	cancel()
	// Give the watcher goroutine time to run its nil-Process branch.
	time.Sleep(50 * time.Millisecond)
}

func TestResolvePathFallsBackToInputName(t *testing.T) {
	assert.Equal(t, "definitely-not-a-real-binary-42f9", resolvePath("definitely-not-a-real-binary-42f9"))
	if resolved, err := exec.LookPath("echo"); err == nil {
		assert.Equal(t, resolved, resolvePath("echo"))
	}
}
