package tui

import (
	"context"
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/lexcodex/relurpify/framework/core"
)

// gitStatusMsg is returned when git status is queried.
type gitStatusMsg struct {
	Modified []string
	Err      error
}

// gitDiffStatMsg is returned when git diff --stat is queried.
type gitDiffStatMsg struct {
	Output string
	Err    error
}

// gitCommitMsg is returned when a commit completes.
type gitCommitMsg struct {
	Message string
	Err     error
}

// rootHandleCommit stages and commits modified files via the capability registry.
// If a message is provided it is used directly. If not, the LLM generates one
// from the current diff.
func rootHandleCommit(m *RootModel, args []string) (*RootModel, tea.Cmd) {
	if len(args) > 0 {
		message := strings.Join(args, " ")
		return m, gitCommitCmd(m.runtime, message)
	}
	return m, gitAutoCommitCmd(m.runtime)
}

// rootHandleLocalReview shows git diff --stat via the capability registry.
func rootHandleLocalReview(m *RootModel, args []string) (*RootModel, tea.Cmd) {
	return m, gitDiffStatInvokeCmd(m.runtime)
}

// gitAutoCommitCmd performs a full commit cycle when no message is provided:
//  1. Check git status — if nothing to commit, return gitStatusMsg{Modified: nil}.
//  2. Get git diff --stat for the LLM prompt.
//  3. Call the LLM (via ExecuteInstruction) to generate a commit message.
//  4. Stage all tracked changes and commit with the generated message.
func gitAutoCommitCmd(rt RuntimeAdapter) tea.Cmd {
	return func() tea.Msg {
		if rt == nil {
			return gitCommitMsg{Err: fmt.Errorf("runtime unavailable")}
		}

		// Step 1: check status.
		statusResult, err := rt.InvokeCapability(context.Background(), "cli_git", map[string]any{
			"args": []string{"status", "--porcelain"},
		})
		if err != nil {
			return gitCommitMsg{Err: fmt.Errorf("git status failed: %w", err)}
		}
		statusOut := ""
		if statusResult != nil {
			if s, ok := statusResult.Data["stdout"].(string); ok {
				statusOut = s
			}
		}
		modified := parseGitStatusOutput(statusOut)
		if len(modified) == 0 {
			return gitStatusMsg{Modified: nil} // reuses "nothing to commit" handler
		}

		// Step 2: get diff for the commit message prompt.
		diffResult, _ := rt.InvokeCapability(context.Background(), "cli_git", map[string]any{
			"args": []string{"diff", "--stat", "HEAD"},
		})
		diff := ""
		if diffResult != nil {
			if s, ok := diffResult.Data["stdout"].(string); ok {
				diff = strings.TrimSpace(s)
			}
		}
		if diff == "" {
			diff = strings.Join(modified, "\n")
		}

		// Step 3: ask the LLM for a commit message.
		prompt := fmt.Sprintf(
			"Generate a concise git commit message in imperative mood (≤72 characters). "+
				"Return only the message text, no explanation.\n\nChanges:\n%s",
			diff,
		)
		llmResult, err := rt.ExecuteInstruction(context.Background(), prompt, core.TaskTypeAnalysis, map[string]any{
			"compact": true,
		})
		if err != nil {
			return gitCommitMsg{Err: fmt.Errorf("failed to generate commit message: %w", err)}
		}
		commitMsg := extractCompactSummary(llmResult) // same key priority as compact
		if commitMsg == "" {
			return gitCommitMsg{Err: fmt.Errorf("model returned empty commit message")}
		}
		// Trim to first line only — some models add explanations after a blank line.
		if idx := strings.Index(commitMsg, "\n"); idx != -1 {
			commitMsg = strings.TrimSpace(commitMsg[:idx])
		}

		// Step 4: stage tracked changes and commit.
		_, err = rt.InvokeCapability(context.Background(), "cli_git", map[string]any{
			"args": []string{"add", "-u"},
		})
		if err != nil {
			return gitCommitMsg{Err: fmt.Errorf("git add failed: %w", err)}
		}
		result, err := rt.InvokeCapability(context.Background(), "cli_git", map[string]any{
			"args": []string{"commit", "-m", commitMsg},
		})
		if err != nil {
			return gitCommitMsg{Err: fmt.Errorf("git commit failed: %w", err)}
		}
		output := ""
		if result != nil {
			if s, ok := result.Data["stdout"].(string); ok {
				output = strings.TrimSpace(s)
			}
		}
		return gitCommitMsg{Message: output}
	}
}

// gitStatusCmd queries git for modified files through the capability registry.
func gitStatusCmd(rt RuntimeAdapter) tea.Cmd {
	return func() tea.Msg {
		if rt == nil {
			return gitStatusMsg{Err: fmt.Errorf("runtime unavailable")}
		}
		result, err := rt.InvokeCapability(context.Background(), "cli_git", map[string]any{
			"args": []string{"status", "--porcelain"},
		})
		if err != nil {
			return gitStatusMsg{Err: fmt.Errorf("git status failed: %w", err)}
		}
		output := ""
		if result != nil {
			if s, ok := result.Data["stdout"].(string); ok {
				output = s
			}
		}
		return gitStatusMsg{Modified: parseGitStatusOutput(output)}
	}
}

// parseGitStatusOutput parses porcelain git status output into a list of file paths.
func parseGitStatusOutput(output string) []string {
	lines := strings.Split(strings.TrimSpace(output), "\n")
	var modified []string
	for _, line := range lines {
		if line == "" {
			continue
		}
		// porcelain format: "XY filename" — skip the two status chars and space
		if len(line) > 3 {
			modified = append(modified, strings.TrimSpace(line[2:]))
		}
	}
	return modified
}

// gitDiffStatInvokeCmd queries git diff --stat through the capability registry.
// Shared by /commit (no-message path) and /local-review.
func gitDiffStatInvokeCmd(rt RuntimeAdapter) tea.Cmd {
	return func() tea.Msg {
		if rt == nil {
			return gitDiffStatMsg{Err: fmt.Errorf("runtime unavailable")}
		}
		result, err := rt.InvokeCapability(context.Background(), "cli_git", map[string]any{
			"args": []string{"diff", "--stat", "HEAD"},
		})
		if err != nil {
			// non-fatal (e.g. no HEAD yet) — return empty output
			return gitDiffStatMsg{}
		}
		output := ""
		if result != nil {
			if s, ok := result.Data["stdout"].(string); ok {
				output = strings.TrimSpace(s)
			}
		}
		return gitDiffStatMsg{Output: output}
	}
}

// gitCommitCmd stages tracked changes and commits with the given message
// through the capability registry.
func gitCommitCmd(rt RuntimeAdapter, message string) tea.Cmd {
	return func() tea.Msg {
		if rt == nil {
			return gitCommitMsg{Err: fmt.Errorf("runtime unavailable")}
		}
		// Stage all tracked changes
		_, err := rt.InvokeCapability(context.Background(), "cli_git", map[string]any{
			"args": []string{"add", "-u"},
		})
		if err != nil {
			return gitCommitMsg{Err: fmt.Errorf("git add failed: %w", err)}
		}
		// Commit
		result, err := rt.InvokeCapability(context.Background(), "cli_git", map[string]any{
			"args": []string{"commit", "-m", message},
		})
		if err != nil {
			return gitCommitMsg{Err: fmt.Errorf("git commit failed: %w", err)}
		}
		output := ""
		if result != nil {
			if s, ok := result.Data["stdout"].(string); ok {
				output = strings.TrimSpace(s)
			}
		}
		return gitCommitMsg{Message: output}
	}
}
