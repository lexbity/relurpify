package git_test

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"codeburg.org/lexbit/relurpify/capability/ports"
	"codeburg.org/lexbit/relurpify/platform/tools/native/git"
)

// execRunner is a real command runner backed by os/exec so git tools can run
// the actual git binary inside the test workspace.
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

func TestConstructors_KeysAndWiring(t *testing.T) {
	constructors := git.Constructors()

	wantKeys := []string{"git_blame", "git_branch", "git_commit", "git_diff", "git_history"}
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

func initRepo(t *testing.T) string {
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

func toolWithRunner(t *testing.T, key string, repo string) ports.Tool {
	t.Helper()
	tool := git.Constructors()[key](repo)
	setter, ok := tool.(interface{ SetCommandRunner(ports.CommandRunner) })
	require.True(t, ok, "%s tool must accept a command runner", key)
	setter.SetCommandRunner(execRunner{})
	return tool
}

func TestConstructors_GitDiffShowsWorkingTreeChanges(t *testing.T) {
	repo := initRepo(t)
	require.NoError(t, os.WriteFile(filepath.Join(repo, "a.txt"), []byte("one\nCHANGED\n"), 0o600))

	diff := toolWithRunner(t, "git_diff", repo)
	require.True(t, diff.IsAvailable(context.Background()), "git tool must detect the repository")

	res, err := diff.Execute(context.Background(), map[string]any{})
	require.NoError(t, err)
	require.NotNil(t, res)
	require.True(t, res.Success, "git diff must succeed: %+v", res)

	data, err := json.Marshal(res.Data)
	require.NoError(t, err)
	require.Contains(t, string(data), "CHANGED")
}

func TestConstructors_GitHistoryListsCommits(t *testing.T) {
	repo := initRepo(t)

	history := toolWithRunner(t, "git_history", repo)
	res, err := history.Execute(context.Background(), map[string]any{"file": "a.txt", "limit": 5})
	require.NoError(t, err)
	require.NotNil(t, res)
	require.True(t, res.Success)

	data, err := json.Marshal(res.Data)
	require.NoError(t, err)
	require.Contains(t, string(data), "initial")
}

func TestConstructors_GitCommitAddsFiles(t *testing.T) {
	repo := initRepo(t)
	require.NoError(t, os.WriteFile(filepath.Join(repo, "b.txt"), []byte("new file\n"), 0o600))

	commit := toolWithRunner(t, "git_commit", repo)
	res, err := commit.Execute(context.Background(), map[string]any{
		"message": "add b",
		"files":   []any{"b.txt"},
	})
	require.NoError(t, err)
	require.NotNil(t, res)
	require.True(t, res.Success, "git commit must succeed: %+v", res)
}

func TestConstructors_ExecuteFailsOutsideRepository(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	plainDir := t.TempDir()
	diff := toolWithRunner(t, "git_diff", plainDir)
	// Availability is a cheap runner check (never a repository probe, which
	// would run a HITL-gated command at boot). The repository failure is
	// observed at Execute, where git reports it as a failed command.
	require.True(t, diff.IsAvailable(context.Background()), "a runner-backed git tool reports available")

	res, err := diff.Execute(context.Background(), map[string]any{})
	require.NoError(t, err)
	require.NotNil(t, res)
	require.False(t, res.Success, "execute outside a git repository must fail")
}
