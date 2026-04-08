package react

import (
	"testing"

	"github.com/lexcodex/relurpify/framework/core"
	"github.com/stretchr/testify/assert"
)

func TestLatestReadOnlyFileSummaryUsesLatestSuccessfulRead(t *testing.T) {
	summary, ok := latestReadOnlyFileSummary([]ToolObservation{
		{
			Tool:    "file_read",
			Success: true,
			Args:    map[string]interface{}{"path": "README.md"},
			Data:    map[string]interface{}{"snippet": "first"},
		},
		{
			Tool:    "file_read",
			Success: true,
			Args:    map[string]interface{}{"path": "README.md"},
			Data:    map[string]interface{}{"snippet": "latest"},
		},
	})

	assert.True(t, ok)
	assert.Contains(t, summary, "latest")
}

func TestReadOnlySummaryFromStateUsesSharedReadOnlyScan(t *testing.T) {
	task := &core.Task{Instruction: "Summarize README.md in 5 bullets."}
	state := core.NewContext()
	state.Set("react.tool_observations", []ToolObservation{
		{
			Tool:    "file_read",
			Success: true,
			Args:    map[string]interface{}{"path": "README.md"},
			Data:    map[string]interface{}{"snippet": "Relurpify is a local agentic automation framework."},
		},
	})

	summary, ok := readOnlySummaryFromState(task, state, map[string]interface{}{})

	assert.True(t, ok)
	assert.Contains(t, summary, "README.md")
	assert.Contains(t, summary, "Relurpify")
}

func TestDirectCompletionSummaryUsesSharedReadOnlyScan(t *testing.T) {
	task := &core.Task{Instruction: "Summarize README.md in 5 bullets."}
	state := core.NewContext()
	state.Set("react.tool_observations", []ToolObservation{
		{
			Tool:    "file_read",
			Success: true,
			Args:    map[string]interface{}{"path": "README.md"},
			Data:    map[string]interface{}{"snippet": "Relurpify overview"},
		},
	})

	summary, ok := directCompletionSummary(task, state)

	assert.True(t, ok)
	assert.Contains(t, summary, "README.md")
}

func TestRepeatedReadCompletionSummaryUsesSharedReadOnlyScan(t *testing.T) {
	task := &core.Task{Instruction: "Summarize README.md in 5 bullets."}
	state := core.NewContext()
	state.Set("react.tool_observations", []ToolObservation{
		{Tool: "file_read", Success: true, Args: map[string]interface{}{"path": "README.md"}, Data: map[string]interface{}{"snippet": "Relurpify overview"}},
		{Tool: "file_read", Success: true, Args: map[string]interface{}{"path": "README.md"}, Data: map[string]interface{}{"snippet": "Relurpify overview"}},
		{Tool: "file_read", Success: true, Args: map[string]interface{}{"path": "README.md"}, Data: map[string]interface{}{"snippet": "Relurpify overview"}},
	})

	summary, ok := repeatedReadCompletionSummary(task, state)

	assert.True(t, ok)
	assert.Contains(t, summary, "README.md")
}

func TestCompletionSummaryFromStateDoesNotNeedFallbackForReadOnlyTask(t *testing.T) {
	task := &core.Task{Instruction: "Summarize README.md in 5 bullets."}
	state := core.NewContext()
	state.Set("react.tool_observations", []ToolObservation{
		{
			Tool:    "file_read",
			Success: true,
			Args:    map[string]interface{}{"path": "README.md"},
			Data:    map[string]interface{}{"snippet": "Relurpify is a local agentic automation framework."},
		},
	})

	summary, ok := completionSummaryFromState(nil, task, state, map[string]interface{}{})

	assert.True(t, ok)
	assert.Contains(t, summary, "README.md")
}
