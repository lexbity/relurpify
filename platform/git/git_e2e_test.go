package git

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"codeburg.org/lexbit/relurpify/capability/ports"
)

// execRunner runs commands through os/exec so the GitCommandTool exercises the
// real git binary inside the test repository (mirrors the native git tests).
type execRunner struct{}

func (execRunner) Run(ctx context.Context, req ports.CommandRequest) (*ports.CommandResult, error) {
	if len(req.Args) == 0 {
		return nil, os.ErrInvalid
	}
	ctx, cancel := context.WithTimeout(ctx, req.Timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, req.Args[0], req.Args[1:]...)
	cmd.Dir = req.Workdir
	out, err := cmd.CombinedOutput()
	res := &ports.CommandResult{Stdout: string(out), Duration: time.Millisecond}
	if exitErr, ok := err.(*exec.ExitError); ok {
		res.ExitCode = exitErr.ExitCode()
		res.Stderr = string(out)
		return res, nil
	}
	if err != nil {
		return nil, err
	}
	return res, nil
}

func initGitRepo(t *testing.T) string {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	repo := t.TempDir()
	run := func(args ...string) {
		out, err := exec.Command("git", append([]string{"-C", repo}, args...)...).CombinedOutput()
		require.NoError(t, err, "git %v: %s", args, out)
	}
	run("init")
	run("config", "user.email", "test@example.com")
	run("config", "user.name", "Test")
	require.NoError(t, os.WriteFile(filepath.Join(repo, "a.txt"), []byte("one\ntwo\n"), 0o600))
	run("add", "a.txt")
	run("commit", "-m", "initial")
	return repo
}

func newExecTool(command string, repo string) *GitCommandTool {
	tool := &GitCommandTool{RepoPath: repo, Command: command}
	tool.SetCommandRunner(execRunner{})
	return tool
}

func TestGitCommandTool_RealGitLifecycle(t *testing.T) {
	repo := initGitRepo(t)
	ctx := context.Background()
	require.NoError(t, os.WriteFile(filepath.Join(repo, "a.txt"), []byte("one\nCHANGED\n"), 0o600))

	diff := newExecTool("diff", repo)
	require.True(t, diff.IsAvailable(ctx), "tool must detect the repository")
	res, err := diff.Execute(ctx, map[string]any{})
	require.NoError(t, err)
	require.True(t, res.Success)
	require.Contains(t, res.Data["output"], "CHANGED")

	blame := newExecTool("blame", repo)
	res, err = blame.Execute(ctx, map[string]any{"file": "a.txt", "start": 1, "end": 2})
	require.NoError(t, err)
	require.True(t, res.Success)
	require.Contains(t, res.Data["output"], "CHANGED")

	history := newExecTool("history", repo)
	res, err = history.Execute(ctx, map[string]any{"file": "a.txt", "limit": 5})
	require.NoError(t, err)
	require.True(t, res.Success)
	require.Contains(t, res.Data["output"], "initial")

	commit := newExecTool("commit", repo)
	res, err = commit.Execute(ctx, map[string]any{"message": "change a", "files": []any{"a.txt"}})
	require.NoError(t, err)
	require.True(t, res.Success)

	res, err = commit.Execute(ctx, map[string]any{"message": "nothing to commit"})
	require.NoError(t, err)
	require.False(t, res.Success, "commit without staged changes must fail through the result envelope")
	require.NotEmpty(t, res.Error)

	branch := newExecTool("branch", repo)
	res, err = branch.Execute(ctx, map[string]any{"name": "feature-branch"})
	require.NoError(t, err)
	require.True(t, res.Success)
}
