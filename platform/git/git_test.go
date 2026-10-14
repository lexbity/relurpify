package git

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"codeburg.org/lexbit/relurpify/capability/ports"
)

// okResult is a successful command result with the given stdout.
func okResult(stdout string) *ports.CommandResult {
	return &ports.CommandResult{Stdout: stdout, Stderr: "", ExitCode: 0}
}

// queueRunner is a scripted ports.CommandRunner: it pops one queued outcome per
// Run call and records every request it received.
type queueRunner struct {
	err      error                  // returned for every Run when set
	results  []*ports.CommandResult // popped per Run; last one repeats
	requests []ports.CommandRequest
}

func (r *queueRunner) Run(_ context.Context, req ports.CommandRequest) (*ports.CommandResult, error) {
	r.requests = append(r.requests, req)
	if r.err != nil {
		return nil, r.err
	}
	if len(r.results) == 0 {
		return okResult(""), nil
	}
	res := r.results[0]
	if len(r.results) > 1 {
		r.results = r.results[1:]
	}
	return res, nil
}

func (r *queueRunner) args() [][]string {
	out := make([][]string, 0, len(r.requests))
	for _, req := range r.requests {
		out = append(out, req.Args)
	}
	return out
}

func disableProbe(t *testing.T) {
	t.Helper()
	prev := SkipAvailabilityProbe
	SkipAvailabilityProbe = true
	t.Cleanup(func() { SkipAvailabilityProbe = prev })
}

func TestGitCommandTool_Metadata(t *testing.T) {
	tests := []struct {
		command     string
		wantName    string
		wantDescr   string
		wantParams  []string
		wantTagSets []string
	}{
		{command: "diff", wantName: "git_diff", wantDescr: "Shows changes in the working tree.", wantParams: []string{}, wantTagSets: []string{ports.TagReadOnly}},
		{command: "history", wantName: "git_history", wantDescr: "Retrieves git history for a file.", wantParams: []string{"file", "limit"}, wantTagSets: []string{ports.TagReadOnly}},
		{command: "branch", wantName: "git_branch", wantDescr: "Creates a new branch.", wantParams: []string{"name"}, wantTagSets: []string{ports.TagExecute, ports.TagDestructive}},
		{command: "commit", wantName: "git_commit", wantDescr: "Creates a commit (without pushing).", wantParams: []string{"message", "files"}, wantTagSets: []string{ports.TagExecute, ports.TagDestructive}},
		{command: "blame", wantName: "git_blame", wantDescr: "Shows blame information.", wantParams: []string{"file", "start", "end"}, wantTagSets: []string{ports.TagReadOnly}},
		{command: "status", wantName: "git_status", wantDescr: "Git command", wantParams: []string{}, wantTagSets: []string{ports.TagExecute, ports.TagDestructive}},
	}
	for _, tc := range tests {
		t.Run(tc.command, func(t *testing.T) {
			disableProbe(t)
			tool := &GitCommandTool{Command: tc.command}
			assert.Equal(t, tc.wantName, tool.Name())
			assert.Equal(t, tc.wantDescr, tool.Description())
			assert.Equal(t, "git", tool.Category())
			assert.True(t, tool.IsAvailable(context.Background()), "probe disabled must report available")

			names := make([]string, 0, len(tool.Parameters()))
			for _, param := range tool.Parameters() {
				names = append(names, param.Name)
			}
			assert.Equal(t, tc.wantParams, names)
			assert.Equal(t, tc.wantTagSets, tool.Tags())
			assert.NotNil(t, tool.Permissions().Permissions)
		})
	}
}

func TestGitCommandTool_IsAvailableProbesRepository(t *testing.T) {
	tests := []struct {
		name   string
		runner ports.CommandRunner
		want   bool
	}{
		{name: "nil runner is unavailable", runner: nil, want: false},
		{name: "runner error is unavailable", runner: &queueRunner{err: errors.New("boom")}, want: false},
		{name: "non-zero exit is unavailable", runner: &queueRunner{results: []*ports.CommandResult{{ExitCode: 128}}}, want: false},
		{name: "zero exit is available", runner: &queueRunner{results: []*ports.CommandResult{{ExitCode: 0}}}, want: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tool := &GitCommandTool{Command: "diff", Runner: tc.runner}
			assert.Equal(t, tc.want, tool.IsAvailable(context.Background()))
		})
	}
}

func TestGitCommandTool_ExecuteFailsWhenRepositoryUnavailable(t *testing.T) {
	tool := &GitCommandTool{Command: "diff", Runner: &queueRunner{err: errors.New("probe failed")}}
	res, err := tool.Execute(context.Background(), map[string]any{})
	require.Error(t, err)
	assert.Nil(t, res)
	assert.Contains(t, err.Error(), "git repository not detected")
}

func TestGitCommandTool_ExecuteDiffReturnsOutput(t *testing.T) {
	disableProbe(t)
	runner := &queueRunner{results: []*ports.CommandResult{
		{Stdout: "diff --git a/a.txt b/a.txt", Stderr: "", ExitCode: 0, StdoutRef: "ref-out", StderrRef: "ref-err"},
	}}
	tool := &GitCommandTool{RepoPath: "/repo", Command: "diff", Runner: runner}

	res, err := tool.Execute(context.Background(), map[string]any{})
	require.NoError(t, err)
	require.NotNil(t, res)
	assert.True(t, res.Success)
	assert.Equal(t, "diff --git a/a.txt b/a.txt", res.Data["output"])
	assert.Equal(t, "ref-out", res.Data["stdout_ref"])
	assert.Equal(t, "ref-err", res.Data["stderr_ref"])
	assert.Equal(t, 0, res.Data["exit_code"])
	assert.Equal(t, [][]string{{"git", "diff"}}, runner.args())
	assert.Equal(t, "/repo", runner.requests[0].Workdir)
}

func TestGitCommandTool_ExecuteHistoryBuildsLogCommand(t *testing.T) {
	tests := []struct {
		name     string
		args     map[string]any
		wantArgs []string
	}{
		{
			name:     "default limit",
			args:     map[string]any{"file": "a.txt"},
			wantArgs: []string{"git", "log", "-n5", "--oneline", "--", "a.txt"},
		},
		{
			name:     "numeric limit",
			args:     map[string]any{"file": "a.txt", "limit": 12},
			wantArgs: []string{"git", "log", "-n12", "--oneline", "--", "a.txt"},
		},
		{
			name:     "string limit stops at non-digit",
			args:     map[string]any{"file": "a.txt", "limit": "7x"},
			wantArgs: []string{"git", "log", "-n7", "--oneline", "--", "a.txt"},
		},
		{
			name:     "float limit from decoded JSON",
			args:     map[string]any{"file": "a.txt", "limit": float64(3)},
			wantArgs: []string{"git", "log", "-n3", "--oneline", "--", "a.txt"},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			disableProbe(t)
			runner := &queueRunner{results: []*ports.CommandResult{okResult("abc123 initial")}}
			tool := &GitCommandTool{Command: "history", Runner: runner}

			res, err := tool.Execute(context.Background(), tc.args)
			require.NoError(t, err)
			require.NotNil(t, res)
			assert.True(t, res.Success)
			assert.Equal(t, [][]string{tc.wantArgs}, runner.args())
		})
	}
}

func TestGitCommandTool_ExecuteBranchCreatesBranch(t *testing.T) {
	disableProbe(t)
	runner := &queueRunner{results: []*ports.CommandResult{okResult("")}}
	tool := &GitCommandTool{Command: "branch", Runner: runner}

	res, err := tool.Execute(context.Background(), map[string]any{"name": "feature/x"})
	require.NoError(t, err)
	require.NotNil(t, res)
	assert.True(t, res.Success)
	assert.Equal(t, [][]string{{"git", "checkout", "-b", "feature/x"}}, runner.args())
}

func TestGitCommandTool_ExecuteCommitStagesAndCommits(t *testing.T) {
	tests := []struct {
		name     string
		args     map[string]any
		wantArgs [][]string
	}{
		{
			name:     "explicit file list",
			args:     map[string]any{"message": "add feature", "files": []any{"a.txt", "b.txt"}},
			wantArgs: [][]string{{"git", "add", "a.txt", "b.txt"}, {"git", "commit", "-m", "add feature"}},
		},
		{
			name:     "no files stages everything",
			args:     map[string]any{"message": "checkpoint"},
			wantArgs: [][]string{{"git", "add", "--all"}, {"git", "commit", "-m", "checkpoint"}},
		},
		{
			name:     "string slice files",
			args:     map[string]any{"message": "x", "files": []string{"only.txt"}},
			wantArgs: [][]string{{"git", "add", "only.txt"}, {"git", "commit", "-m", "x"}},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			disableProbe(t)
			runner := &queueRunner{results: []*ports.CommandResult{okResult(""), okResult("main 1234")}}
			tool := &GitCommandTool{Command: "commit", Runner: runner}

			res, err := tool.Execute(context.Background(), tc.args)
			require.NoError(t, err)
			require.NotNil(t, res)
			assert.True(t, res.Success)
			assert.Equal(t, "main 1234", res.Data["output"])
			assert.Equal(t, tc.wantArgs, runner.args())
		})
	}
}

func TestGitCommandTool_ExecuteCommitRejectsNonArrayFiles(t *testing.T) {
	disableProbe(t)
	tool := &GitCommandTool{Command: "commit", Runner: &queueRunner{}}

	res, err := tool.Execute(context.Background(), map[string]any{"message": "m", "files": "a.txt"})
	require.Error(t, err)
	assert.Nil(t, res)
	assert.Contains(t, err.Error(), "expected array")
}

func TestGitCommandTool_ExecuteCommitProceedsAfterNonZeroStagingExit(t *testing.T) {
	disableProbe(t)
	// Documents current behavior: a non-zero `git add` exit is reported as a
	// result envelope, not a Go error, so Execute still attempts the commit.
	runner := &queueRunner{results: []*ports.CommandResult{
		{ExitCode: 128, Stderr: "fatal: not a git repository"},
		{ExitCode: 1, Stderr: "nothing staged"},
	}}
	tool := &GitCommandTool{Command: "commit", Runner: runner}

	res, err := tool.Execute(context.Background(), map[string]any{"message": "m"})
	require.NoError(t, err)
	require.NotNil(t, res)
	assert.False(t, res.Success)
	assert.Contains(t, res.Error, "git commit -m m failed")
	assert.Contains(t, res.Error, "nothing staged")
	assert.Equal(t, [][]string{{"git", "add", "--all"}, {"git", "commit", "-m", "m"}}, runner.args())
}

func TestGitCommandTool_ExecuteBlameBuildsRangeCommand(t *testing.T) {
	disableProbe(t)
	runner := &queueRunner{results: []*ports.CommandResult{okResult("abc (Test 2026-01-01 1) one")}}
	tool := &GitCommandTool{Command: "blame", Runner: runner}

	res, err := tool.Execute(context.Background(), map[string]any{"file": "a.txt", "start": 2, "end": 4})
	require.NoError(t, err)
	require.NotNil(t, res)
	assert.True(t, res.Success)
	assert.Equal(t, [][]string{{"git", "blame", "-L2,4", "a.txt"}}, runner.args())
}

func TestGitCommandTool_ExecuteUnsupportedCommand(t *testing.T) {
	disableProbe(t)
	tool := &GitCommandTool{Command: "push", Runner: &queueRunner{}}

	res, err := tool.Execute(context.Background(), map[string]any{})
	require.Error(t, err)
	assert.Nil(t, res)
	assert.Contains(t, err.Error(), "unsupported git command push")
}

func TestGitCommandTool_ExecuteReportsFailureEnvelope(t *testing.T) {
	tests := []struct {
		name        string
		result      *ports.CommandResult
		wantInError string
	}{
		{
			name:        "stderr carried into error",
			result:      &ports.CommandResult{ExitCode: 1, Stderr: "error: pathspec did not match"},
			wantInError: "error: pathspec did not match",
		},
		{
			name:        "empty stderr falls back to exit code",
			result:      &ports.CommandResult{ExitCode: 69},
			wantInError: "exit code 69",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			disableProbe(t)
			runner := &queueRunner{results: []*ports.CommandResult{tc.result}}
			tool := &GitCommandTool{Command: "diff", Runner: runner}

			res, err := tool.Execute(context.Background(), map[string]any{})
			require.NoError(t, err)
			require.NotNil(t, res)
			assert.False(t, res.Success)
			assert.Contains(t, res.Error, "git diff failed")
			assert.Contains(t, res.Error, tc.wantInError)
			assert.Equal(t, tc.result.ExitCode, res.Data["exit_code"])
		})
	}
}

func TestGitCommandTool_ExecuteRunnerErrorIsWrapped(t *testing.T) {
	disableProbe(t)
	tool := &GitCommandTool{Command: "diff", Runner: &queueRunner{err: errors.New("sandbox denied")}}

	res, err := tool.Execute(context.Background(), map[string]any{})
	require.Error(t, err)
	assert.Nil(t, res)
	assert.Contains(t, err.Error(), "git diff failed")
	assert.Contains(t, err.Error(), "sandbox denied")
}

func TestGitCommandTool_ExecuteWithoutRunnerFails(t *testing.T) {
	disableProbe(t)
	tool := &GitCommandTool{Command: "diff"}

	res, err := tool.Execute(context.Background(), map[string]any{})
	require.Error(t, err)
	assert.Nil(t, res)
	assert.Contains(t, err.Error(), "command runner missing")
}

func TestGitCommandTool_SetCommandRunnerWiresRunner(t *testing.T) {
	tool := &GitCommandTool{Command: "diff"}
	assert.False(t, tool.IsAvailable(context.Background()), "nil runner must be unavailable")

	runner := &queueRunner{results: []*ports.CommandResult{{ExitCode: 0}}}
	tool.SetCommandRunner(runner)
	assert.True(t, tool.IsAvailable(context.Background()))
}

func TestGitCommandTool_SettersAreAccepted(t *testing.T) {
	var tool ports.Tool = &GitCommandTool{Command: "diff"}
	setter, ok := tool.(interface {
		SetPermissionManager(manager any, agentID string)
		SetAgentSpec(spec any, agentID string)
	})
	require.True(t, ok, "git tool must accept permission wiring")
	setter.SetPermissionManager(nil, "agent-1")
	setter.SetAgentSpec(nil, "agent-1")
}

func TestToInt(t *testing.T) {
	tests := []struct {
		name  string
		value any
		want  int
	}{
		{name: "int", value: 7, want: 7},
		{name: "int32", value: int32(8), want: 8},
		{name: "int64", value: int64(9), want: 9},
		{name: "float64 truncates", value: 3.75, want: 3},
		{name: "numeric string", value: "42", want: 42},
		{name: "string with trailing garbage", value: "12ab", want: 12},
		{name: "string starting with garbage", value: "x1", want: 0},
		{name: "empty string", value: "", want: 0},
		{name: "negative int", value: -5, want: -5},
		{name: "nil", value: nil, want: 0},
		{name: "bool", value: true, want: 0},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, toInt(tc.value))
		})
	}
}

func FuzzToInt(f *testing.F) {
	f.Add("42", 1)
	f.Add("12ab", 12)
	f.Add("007", 7)
	f.Add("x9", 0)
	f.Add("", 5)
	f.Add("999999999999999999999999", 24)
	f.Fuzz(func(t *testing.T, text string, num int) {
		fromString := toInt(text)
		if fromString < 0 {
			t.Errorf("toInt(%q) = %d, digit parsing must never go negative", text, fromString)
		}
		assert.Equal(t, num, toInt(num))
		assert.Equal(t, num, toInt(float64(num)))
		assert.Equal(t, num, toInt(int64(num)))
		assert.Equal(t, num, toInt(int32(num)))
	})
}
