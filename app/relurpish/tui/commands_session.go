package tui

import (
	"fmt"
	"strings"
	"time"

	"codeburg.org/lexbit/relurpify/framework/core"
	tea "github.com/charmbracelet/bubbletea"
)

// rootHandleCheckpoint saves the current session as a named checkpoint.
func rootHandleCheckpoint(m *RootModel, args []string) (*RootModel, tea.Cmd) {
	if m.store == nil {
		m.addSystemMessage("checkpoint unavailable: no session store")
		return m, nil
	}

	label := strings.Join(args, "-")
	if label == "" {
		label = time.Now().Format("150405")
	}

	var msgs []Message
	if m.chat != nil {
		msgs = m.chat.Messages()
	}
	rec := SessionRecord{
		SessionMeta: SessionMeta{
			Agent:     m.sharedSess.Agent,
			Workspace: m.sharedSess.Workspace,
			StartTime: m.sharedSess.StartTime,
			Label:     label,
		},
		Messages: msgs,
		Context:  m.sharedCtx,
	}
	if err := m.store.SaveCheckpoint(rec); err != nil {
		m.addSystemMessage(fmt.Sprintf("Checkpoint failed: %v", err))
		return m, nil
	}
	m.addSystemMessage(fmt.Sprintf("checkpoint saved: %s", label))
	return m, nil
}

// compactResultMsg carries the outcome of a /compact operation.
type compactResultMsg struct {
	summary       string
	originalCount int
	err           error
}

// rootHandleCompact compresses the chat history to a single LLM-generated summary.
// The current feed is snapshotted onto the undo stack before replacement, so
// ctrl+z restores the full history. The LLM call runs via the streaming path so
// the spinner is visible and HasActiveRuns() returns true while it is in flight.
func rootHandleCompact(m *RootModel, _ []string) (*RootModel, tea.Cmd) {
	if m.chat == nil {
		return m, nil
	}
	if m.chat.HasActiveRuns() {
		m.addSystemMessage("cannot compact during active run")
		return m, nil
	}
	if m.runtime == nil {
		m.addSystemMessage("compact unavailable: no runtime")
		return m, nil
	}

	msgs := m.chat.Messages()
	if len(msgs) == 0 {
		m.addSystemMessage("nothing to compact")
		return m, nil
	}

	// Snapshot current feed onto undo stack so ctrl+z can restore it.
	m.chat.PushUndoSnapshot(msgs)

	originalCount := len(msgs)
	prompt := buildCompactPrompt(msgs)

	// StartRunSilent launches a streaming run without adding a user message to
	// the feed. handleStreamComplete intercepts it via compactRunID.
	cmd, runID := m.chat.StartRunSilent(prompt)
	m.chat.SetCompactRunID(runID, originalCount)

	return m, cmd
}

// extractCompactSummary pulls the text output from a core.Result returned by
// the agent. It checks the standard output keys in order of preference:
// "final_output" (ReActAgent), "text" (compressed summary), "summary" (PlannerAgent).
// Used as a fallback when the streamed builder text is empty.
func extractCompactSummary(result *core.Result) string {
	if result == nil || result.Data == nil {
		return ""
	}
	for _, key := range []string{"final_output", "text", "summary"} {
		if v, ok := result.Data[key]; ok {
			if s, ok := v.(string); ok {
				if trimmed := strings.TrimSpace(s); trimmed != "" {
					return trimmed
				}
			}
		}
	}
	return ""
}

// compactTokenBudget is the approximate token budget for the history prompt.
const compactTokenBudget = 4096

// buildCompactPrompt builds the summarisation prompt from the most recent messages
// up to the token budget. Messages are selected newest-first so that recent context
// is always included when the history exceeds the budget.
func buildCompactPrompt(msgs []Message) string {
	budget := compactTokenBudget * 4 // rough chars-to-tokens ratio
	// Collect lines from newest to oldest until budget is hit.
	lines := make([]string, 0, len(msgs))
	total := 0
	for i := len(msgs) - 1; i >= 0; i-- {
		if msgs[i].Role == RoleSystem {
			continue
		}
		line := fmt.Sprintf("[%s] %s\n", msgs[i].Role, msgs[i].Content.Text)
		if total+len(line) > budget {
			break
		}
		lines = append(lines, line)
		total += len(line)
	}
	// Reverse to chronological order.
	for i, j := 0, len(lines)-1; i < j; i, j = i+1, j-1 {
		lines[i], lines[j] = lines[j], lines[i]
	}
	return fmt.Sprintf(
		"Summarize the following conversation into a concise paragraph that captures "+
			"the key decisions, findings, and current state. Be factual and brief.\n\n%s",
		strings.Join(lines, ""),
	)
}
