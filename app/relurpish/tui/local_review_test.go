package tui

import (
	"fmt"
	"testing"

	"github.com/lexcodex/relurpify/framework/core"
	"github.com/stretchr/testify/require"
)

// TestGitDiffStatInvokesCliGit verifies gitDiffStatInvokeCmd routes through cli_git.
func TestGitDiffStatInvokesCliGit(t *testing.T) {
	rt := &recordingAdapter{
		response: &core.ToolResult{Data: map[string]any{"stdout": " test.txt | 2 +-\n 1 file changed"}},
	}
	cmd := gitDiffStatInvokeCmd(rt)
	msg := cmd()

	require.IsType(t, gitDiffStatMsg{}, msg)
	require.Len(t, rt.calls, 1)
	require.Equal(t, "cli_git", rt.calls[0].Name)

	args := rt.calls[0].Args["args"].([]string)
	require.Equal(t, []string{"diff", "--stat", "HEAD"}, args)

	diffMsg := msg.(gitDiffStatMsg)
	require.Nil(t, diffMsg.Err)
	require.Contains(t, diffMsg.Output, "test.txt")
}

// TestGitDiffStatEmpty verifies gitDiffStatInvokeCmd returns empty output when no changes.
func TestGitDiffStatEmpty(t *testing.T) {
	rt := &recordingAdapter{
		response: &core.ToolResult{Data: map[string]any{"stdout": ""}},
	}
	cmd := gitDiffStatInvokeCmd(rt)
	msg := cmd().(gitDiffStatMsg)
	require.Nil(t, msg.Err)
	require.Equal(t, "", msg.Output)
}

// TestGitDiffStatCapabilityError verifies non-fatal error handling (empty output returned).
func TestGitDiffStatCapabilityError(t *testing.T) {
	rt := &recordingAdapter{err: fmt.Errorf("no HEAD yet")}
	cmd := gitDiffStatInvokeCmd(rt)
	msg := cmd().(gitDiffStatMsg)
	// errors from cli_git are non-fatal for local-review — empty output, no error
	require.Nil(t, msg.Err)
	require.Equal(t, "", msg.Output)
}

// TestLocalReviewMsgNoChanges verifies diff stat message with empty output.
func TestLocalReviewMsgNoChanges(t *testing.T) {
	adapter := newMinimalCommitTestAdapter()
	m := newRootModel(adapter)

	msg := gitDiffStatMsg{Output: "", Err: nil}
	_, _ = m.Update(msg)
	messages := m.chat.Messages()

	require.True(t, len(messages) > 0)
	lastMsg := messages[len(messages)-1]
	require.Equal(t, RoleSystem, lastMsg.Role)
	require.Contains(t, lastMsg.Content.Text, "no changes since last commit")
}

// TestLocalReviewMsgWithOutput verifies diff stat message with output.
func TestLocalReviewMsgWithOutput(t *testing.T) {
	adapter := newMinimalCommitTestAdapter()
	m := newRootModel(adapter)

	diffOutput := `app/main.go                | 10 +++++
 app/config.go              |  5 +-
 2 files changed, 12 insertions(+), 3 deletions(-)`

	msg := gitDiffStatMsg{Output: diffOutput, Err: nil}
	_, _ = m.Update(msg)
	messages := m.chat.Messages()

	require.True(t, len(messages) > 0)
	lastMsg := messages[len(messages)-1]
	require.Equal(t, RoleSystem, lastMsg.Role)
	require.Contains(t, lastMsg.Content.Text, "Changes since last commit")
	require.Contains(t, lastMsg.Content.Text, "main.go")
	require.Contains(t, lastMsg.Content.Text, "config.go")
}

// TestLocalReviewMsgError verifies error handling in diff stat message.
func TestLocalReviewMsgError(t *testing.T) {
	adapter := newMinimalCommitTestAdapter()
	m := newRootModel(adapter)

	msg := gitDiffStatMsg{Output: "", Err: fmt.Errorf("permission denied")}
	_, _ = m.Update(msg)
	messages := m.chat.Messages()

	require.True(t, len(messages) > 0)
	lastMsg := messages[len(messages)-1]
	require.Equal(t, RoleSystem, lastMsg.Role)
	require.Contains(t, lastMsg.Content.Text, "Review failed")
	require.Contains(t, lastMsg.Content.Text, "permission denied")
}
