package tui

import (
	"context"
	"fmt"
	"strings"
	"sync/atomic"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/lexcodex/relurpify/framework/core"
)

// RunState tracks a single in-flight execution.
type RunState struct {
	ID      string
	Prompt  string
	Started time.Time
	Builder *MessageBuilder
	Ch      chan tea.Msg
	Cancel  context.CancelFunc
	Dropped int64
}

func (m Model) hasActiveRuns() bool {
	return len(m.runStates) > 0
}

func (m Model) startRun(prompt string) (Model, tea.Cmd) {
	if prompt == "" {
		return m, nil
	}
	if m.runtime == nil {
		return m.addSystemMessage("Runtime unavailable: cannot start run"), nil
	}
	if !m.allowParallel && m.hasActiveRuns() {
		return m.addSystemMessage("Run already in progress. Use /stop to cancel."), nil
	}

	runID := generateID()
	ch := make(chan tea.Msg, 256)
	builder := NewMessageBuilder(runID)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	run := &RunState{
		ID:      runID,
		Prompt:  prompt,
		Started: time.Now(),
		Builder: builder,
		Ch:      ch,
		Cancel:  cancel,
	}
	if m.runStates == nil {
		m.runStates = make(map[string]*RunState)
	}
	m.runStates[runID] = run
	m.lastPrompt = prompt

	payload := m.prepareContextPayload(ctx)
	for _, line := range payload.errors {
		m = m.addSystemMessage(line)
	}

	go m.runAgentStream(ctx, run, payload.metadata)
	return m, listenToStream(ch)
}

func (m Model) stopLatestRun() (Model, tea.Cmd) {
	if len(m.runStates) == 0 {
		return m.addSystemMessage("No active run to stop."), nil
	}
	var latest *RunState
	for _, run := range m.runStates {
		if latest == nil || run.Started.After(latest.Started) {
			latest = run
		}
	}
	if latest == nil || latest.Cancel == nil {
		return m.addSystemMessage("No active run to stop."), nil
	}
	latest.Cancel()
	return m.addSystemMessage(fmt.Sprintf("Stopping run %s", latest.ID)), nil
}

func (m Model) retryLastRun() (Model, tea.Cmd) {
	if strings.TrimSpace(m.lastPrompt) == "" {
		return m.addSystemMessage("No prior prompt to retry."), nil
	}
	return m.startRun(m.lastPrompt)
}

type contextPayload struct {
	metadata map[string]any
	errors   []string
}

func (m Model) prepareContextPayload(ctx context.Context) contextPayload {
	payload := contextPayload{
		metadata: map[string]any{
			"source": "relurpish",
		},
	}
	if m.context != nil {
		files := m.context.List()
		if len(files) > 0 {
			payload.metadata["context_files"] = files
		}
		if m.runtime != nil {
			resolution := m.runtime.ResolveContextFiles(ctx, files)
			if len(resolution.Allowed) > 0 {
				payload.metadata["context_files"] = resolution.Allowed
				if len(resolution.Contents) > 0 {
					payload.metadata["context_file_contents"] = resolution.Contents
				}
			}
			if resolution.HasErrors() {
				for _, line := range resolution.ErrorLines() {
					payload.errors = append(payload.errors, fmt.Sprintf("Context error: %s", line))
				}
			}
		}
	}
	if m.session != nil {
		if m.session.Mode != "" {
			payload.metadata["mode"] = m.session.Mode
		}
		if m.session.Strategy != "" {
			payload.metadata["strategy"] = m.session.Strategy
		}
	}
	return payload
}

// runAgentStream executes the runtime instruction and emits streaming events.
func (m Model) runAgentStream(ctx context.Context, run *RunState, metadata map[string]any) {
	if run == nil || run.Ch == nil {
		return
	}
	start := time.Now()
	sendRunMsg(run, StreamTokenMsg{
		RunID:     run.ID,
		TokenType: TokenThinking,
		Metadata: map[string]interface{}{
			"kind":        "start",
			"stepType":    string(StepAnalyzing),
			"description": "Analyzing request",
		},
	})

	if run.Ch != nil {
		metadata["stream_callback"] = func(token string) {
			sendRunMsg(run, StreamTokenMsg{RunID: run.ID, TokenType: TokenText, Token: token})
		}
	}

	result, err := m.runtime.ExecuteInstruction(ctx, run.Prompt, core.TaskTypeCodeGeneration, metadata)
	if err != nil {
		sendRunFinal(run, StreamErrorMsg{RunID: run.ID, Error: err})
		sendRunFinal(run, StreamCompleteMsg{RunID: run.ID, Duration: time.Since(start), TokensUsed: 0})
		close(run.Ch)
		return
	}

	summary := summarizeResult(result)
	if summary != "" {
		sendRunMsg(run, StreamTokenMsg{RunID: run.ID, TokenType: TokenText, Token: summary})
	}

	sendRunFinal(run, StreamCompleteMsg{RunID: run.ID, Duration: time.Since(start), TokensUsed: estimateTokens(summary)})
	close(run.Ch)
}

// summarizeResult turns a core.Result into human readable feed text.
func summarizeResult(res *core.Result) string {
	if res == nil {
		return ""
	}
	var b strings.Builder
	b.WriteString("Task node: ")
	b.WriteString(res.NodeID)
	b.WriteString("\nSuccess: ")
	if res.Success {
		b.WriteString("true")
	} else {
		b.WriteString("false")
	}
	if len(res.Data) > 0 {
		b.WriteString("\nData: ")
		b.WriteString(fmt.Sprintf("%v", res.Data))
	}
	if res.Error != nil {
		b.WriteString("\nError: ")
		b.WriteString(res.Error.Error())
	}
	return b.String()
}

func sendRunMsg(run *RunState, msg tea.Msg) {
	if run == nil || run.Ch == nil {
		return
	}
	select {
	case run.Ch <- msg:
	default:
		atomic.AddInt64(&run.Dropped, 1)
	}
}

func sendRunFinal(run *RunState, msg tea.Msg) {
	if run == nil || run.Ch == nil {
		return
	}
	select {
	case run.Ch <- msg:
	default:
		go func() {
			run.Ch <- msg
		}()
	}
}
