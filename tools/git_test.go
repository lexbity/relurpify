package tools

import (
	"context"
	"testing"

	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/framework/runtime"
	"github.com/stretchr/testify/require"
)

type recordingRunner struct {
	requests []runtime.CommandRequest
}

func (r *recordingRunner) Run(ctx context.Context, req runtime.CommandRequest) (string, string, error) {
	r.requests = append(r.requests, req)
	if len(req.Args) >= 3 && req.Args[0] == "git" && req.Args[1] == "rev-parse" && req.Args[2] == "--is-inside-work-tree" {
		return "true\n", "", nil
	}
	return "", "", nil
}

func TestGitCommitUsesExplicitFilesFromDecodedToolArgs(t *testing.T) {
	runner := &recordingRunner{}
	tool := &GitCommandTool{RepoPath: t.TempDir(), Command: "commit", Runner: runner}

	res, err := tool.Execute(context.Background(), core.NewContext(), map[string]interface{}{
		"message": "demo commit",
		"files":   []interface{}{"main.go", "go.mod"},
	})
	require.NoError(t, err)
	require.NotNil(t, res)
	require.True(t, res.Success)
	require.Len(t, runner.requests, 3)
	require.Equal(t, []string{"git", "add", "main.go", "go.mod"}, runner.requests[1].Args)
	require.Equal(t, []string{"git", "commit", "-m", "demo commit"}, runner.requests[2].Args)
}

func TestGitCommitUsesExplicitFilesFromStringSlice(t *testing.T) {
	runner := &recordingRunner{}
	tool := &GitCommandTool{RepoPath: t.TempDir(), Command: "commit", Runner: runner}

	res, err := tool.Execute(context.Background(), core.NewContext(), map[string]interface{}{
		"message": "demo commit",
		"files":   []string{"main.go"},
	})
	require.NoError(t, err)
	require.NotNil(t, res)
	require.True(t, res.Success)
	require.Len(t, runner.requests, 3)
	require.Equal(t, []string{"git", "add", "main.go"}, runner.requests[1].Args)
	require.Equal(t, []string{"git", "commit", "-m", "demo commit"}, runner.requests[2].Args)
}
