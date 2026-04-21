package git

import (
	"context"
	"testing"

	"codeburg.org/lexbit/relurpify/framework/core"
	"codeburg.org/lexbit/relurpify/framework/sandbox"
	"github.com/stretchr/testify/require"
)

type recordingRunner struct {
	requests []sandbox.CommandRequest
	failFor  map[string]error
}

func (r *recordingRunner) Run(ctx context.Context, req sandbox.CommandRequest) (string, string, error) {
	r.requests = append(r.requests, req)
	if r.failFor != nil {
		if err, ok := r.failFor[req.Args[1]]; ok {
			return "", err.Error(), err
		}
	}
	if len(req.Args) >= 3 && req.Args[0] == "git" && req.Args[1] == "rev-parse" && req.Args[2] == "--is-inside-work-tree" {
		return "true\n", "", nil
	}
	return "", "", nil
}

func TestGitToolMetadataAndHelpers(t *testing.T) {
	tool := &GitCommandTool{Command: "diff"}
	require.Equal(t, "git_diff", tool.Name())
	require.Equal(t, "Shows changes in the working tree.", tool.Description())
	require.Equal(t, []string{core.TagReadOnly}, tool.Tags())
	require.Len(t, tool.Parameters(), 0)

	require.Equal(t, 0, toInt(nil))
	require.Equal(t, 7, toInt(7))
	require.Equal(t, 12, toInt(int64(12)))
	require.Equal(t, 42, toInt("42"))
	require.Equal(t, 3, toInt("3abc"))
}

func TestGitToolInventorySurface(t *testing.T) {
	cases := []struct {
		name     string
		command  string
		wantName string
		wantDesc string
		wantTags []string
		wantArgs []core.ToolParameter
	}{
		{
			name:     "diff",
			command:  "diff",
			wantName: "git_diff",
			wantDesc: "Shows changes in the working tree.",
			wantTags: []string{core.TagReadOnly},
			wantArgs: []core.ToolParameter{},
		},
		{
			name:     "history",
			command:  "history",
			wantName: "git_history",
			wantDesc: "Retrieves git history for a file.",
			wantTags: []string{core.TagReadOnly},
			wantArgs: []core.ToolParameter{
				{Name: "file", Type: "string", Required: true},
				{Name: "limit", Type: "int", Required: false, Default: 5},
			},
		},
		{
			name:     "branch",
			command:  "branch",
			wantName: "git_branch",
			wantDesc: "Creates a new branch.",
			wantTags: []string{core.TagExecute, core.TagDestructive},
			wantArgs: []core.ToolParameter{{Name: "name", Type: "string", Required: true}},
		},
		{
			name:     "commit",
			command:  "commit",
			wantName: "git_commit",
			wantDesc: "Creates a commit (without pushing).",
			wantTags: []string{core.TagExecute, core.TagDestructive},
			wantArgs: []core.ToolParameter{
				{Name: "message", Type: "string", Required: true},
				{Name: "files", Type: "array", Required: false},
			},
		},
		{
			name:     "blame",
			command:  "blame",
			wantName: "git_blame",
			wantDesc: "Shows blame information.",
			wantTags: []string{core.TagReadOnly},
			wantArgs: []core.ToolParameter{
				{Name: "file", Type: "string", Required: true},
				{Name: "start", Type: "int", Required: false, Default: 1},
				{Name: "end", Type: "int", Required: false, Default: 1},
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tool := &GitCommandTool{Command: tc.command}
			require.Equal(t, tc.wantName, tool.Name())
			require.Equal(t, tc.wantDesc, tool.Description())
			require.Equal(t, tc.wantTags, tool.Tags())
			require.Equal(t, tc.wantArgs, tool.Parameters())
		})
	}
}

func TestGitToolExecuteCoversCommonCommands(t *testing.T) {
	dir := t.TempDir()
	runner := &recordingRunner{}
	tool := &GitCommandTool{RepoPath: dir, Command: "history", Runner: runner}

	res, err := tool.Execute(context.Background(), core.NewContext(), map[string]interface{}{
		"file":  "README.md",
		"limit": 3,
	})
	require.NoError(t, err)
	require.NotNil(t, res)
	require.True(t, res.Success)
	require.GreaterOrEqual(t, len(runner.requests), 2)
	require.Equal(t, []string{"git", "log", "-n3", "--oneline", "--", "README.md"}, runner.requests[1].Args)

	tool = &GitCommandTool{RepoPath: dir, Command: "branch", Runner: runner}
	_, err = tool.Execute(context.Background(), core.NewContext(), map[string]interface{}{"name": "feature/test"})
	require.NoError(t, err)
	require.Equal(t, []string{"git", "checkout", "-b", "feature/test"}, runner.requests[len(runner.requests)-1].Args)

	tool = &GitCommandTool{RepoPath: dir, Command: "blame", Runner: runner}
	_, err = tool.Execute(context.Background(), core.NewContext(), map[string]interface{}{"file": "main.go", "start": 2, "end": 4})
	require.NoError(t, err)
	require.Equal(t, []string{"git", "blame", "-L2,4", "main.go"}, runner.requests[len(runner.requests)-1].Args)
}

func TestGitToolIsAvailableAndFailurePaths(t *testing.T) {
	dir := t.TempDir()
	tool := &GitCommandTool{RepoPath: dir}
	require.False(t, tool.IsAvailable(context.Background(), core.NewContext()))

	runner := &recordingRunner{}
	tool.Runner = runner
	require.True(t, tool.IsAvailable(context.Background(), core.NewContext()))

	failing := &GitCommandTool{
		RepoPath: dir,
		Command:  "diff",
		Runner:   &recordingRunner{failFor: map[string]error{"diff": context.Canceled}},
	}
	res, err := failing.Execute(context.Background(), core.NewContext(), nil)
	require.NoError(t, err)
	require.NotNil(t, res)
	require.False(t, res.Success)

	tool = &GitCommandTool{RepoPath: dir, Command: "merge", Runner: runner}
	_, err = tool.Execute(context.Background(), core.NewContext(), nil)
	require.Error(t, err)
	require.Contains(t, err.Error(), "unsupported git command")
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
