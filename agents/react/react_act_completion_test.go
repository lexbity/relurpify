package react

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/lexcodex/relurpify/framework/capability"
	"github.com/lexcodex/relurpify/framework/core"
	"github.com/stretchr/testify/assert"
)

type offlineStubTool struct {
	stubTool
}

type recordingReadTool struct {
	calls []map[string]interface{}
}

func (t offlineStubTool) IsAvailable(ctx context.Context, state *core.Context) bool {
	return false
}

func (t *recordingReadTool) Name() string        { return "file_read" }
func (t *recordingReadTool) Description() string { return "reads a file" }
func (t *recordingReadTool) Category() string    { return "file" }
func (t *recordingReadTool) Parameters() []core.ToolParameter {
	return []core.ToolParameter{{Name: "path", Type: "string", Required: true}}
}
func (t *recordingReadTool) Execute(ctx context.Context, state *core.Context, args map[string]interface{}) (*core.ToolResult, error) {
	copyArgs := map[string]interface{}{}
	for k, v := range args {
		copyArgs[k] = v
	}
	t.calls = append(t.calls, copyArgs)
	return &core.ToolResult{
		Success: true,
		Data: map[string]interface{}{
			"path":    fmt.Sprint(args["path"]),
			"snippet": "retrieved content",
		},
	}, nil
}
func (t *recordingReadTool) IsAvailable(ctx context.Context, state *core.Context) bool { return true }
func (t *recordingReadTool) Permissions() core.ToolPermissions                         { return core.ToolPermissions{} }
func (t *recordingReadTool) Tags() []string                                            { return []string{core.TagReadOnly} }

func TestReactActNodeExecuteProcessesQueuedToolCallsAndClearsQueue(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "queued.txt")

	registry := capability.NewRegistry()
	assert.NoError(t, registry.Register(fileWriteStubTool{}))
	assert.NoError(t, registry.Register(offlineStubTool{stubTool: stubTool{name: "offline_tool"}}))

	agent := &ReActAgent{Tools: registry}
	node := &reactActNode{
		id:    "act",
		agent: agent,
		task:  &core.Task{Instruction: "Fix the bug and update the file."},
	}

	state := core.NewContext()
	state.Set("task.id", "task-queue")
	state.Set("react.tool_calls", []core.ToolCall{
		{Name: "missing_tool"},
		{Name: "offline_tool"},
		{
			Name: "file_write",
			Args: map[string]interface{}{
				"path":    target,
				"content": "queued result",
			},
		},
	})

	result, err := node.Execute(context.Background(), state)
	assert.NoError(t, err)
	if assert.NotNil(t, result) {
		assert.False(t, result.Success)
		assert.Contains(t, fmt.Sprint(result.Error), "unknown tool missing_tool")
		assert.Contains(t, fmt.Sprint(result.Error), "unavailable tool offline_tool")
	}

	rawCalls, ok := state.Get("react.tool_calls")
	assert.True(t, ok)
	assert.Empty(t, rawCalls.([]core.ToolCall))

	rawResult, ok := state.Get("react.last_tool_result")
	assert.True(t, ok)
	results, ok := rawResult.(map[string]interface{})
	assert.True(t, ok)
	if assert.Contains(t, results, "file_write") {
		writeResult, ok := results["file_write"].(map[string]interface{})
		assert.True(t, ok)
		assert.True(t, writeResult["success"].(bool))
	}

	_, err = os.Stat(target)
	assert.NoError(t, err)
}

func TestReactActNodeMutationAndPathHelpers(t *testing.T) {
	call := core.ToolCall{
		Name: "file_write",
		Args: map[string]interface{}{"path": "path-from-args.txt"},
	}
	res := &core.ToolResult{
		Data: map[string]interface{}{"path": "path-from-result.txt"},
	}

	assert.Equal(t, "path-from-result.txt", resultPathOrArg(call, res))
	assert.Equal(t, "path-from-args.txt", resultPathOrArg(call, &core.ToolResult{}))
	assert.Empty(t, resultPathOrArg(core.ToolCall{Name: "file_write", Args: map[string]interface{}{"path": "<nil>"}}, nil))

	assert.Equal(t, []string{"path-from-result.txt"}, mutationPaths(call, &core.ToolResult{Data: map[string]interface{}{"path": "path-from-result.txt"}}))
	assert.Equal(t, []string{"path-from-args.txt"}, mutationPaths(core.ToolCall{Name: "file_create", Args: map[string]interface{}{"path": "path-from-args.txt"}}, nil))
	assert.Equal(t, []string{"path-from-args.txt"}, mutationPaths(core.ToolCall{Name: "file_delete", Args: map[string]interface{}{"path": "path-from-args.txt"}}, nil))
	assert.Nil(t, mutationPaths(core.ToolCall{Name: "search_grep", Args: map[string]interface{}{"path": "ignored"}}, nil))
}

func TestReactCompletionFallbackHelpers(t *testing.T) {
	task := &core.Task{Instruction: "Summarize README.md for the team."}
	state := core.NewContext()
	state.Set("react.tool_observations", []ToolObservation{
		{
			Tool:    "file_read",
			Success: true,
			Args:    map[string]interface{}{"path": "README.md"},
			Data:    map[string]interface{}{"snippet": "", "summary": "manual summary"},
			Summary: "manual summary",
		},
	})

	summary, ok := readOnlySummaryFromState(task, state, map[string]interface{}{})
	assert.True(t, ok)
	assert.Equal(t, "Summary of README.md: manual summary", summary)

	fallback, ok := directCompletionSummary(&core.Task{Instruction: "Edit foo.txt"}, core.NewContext())
	assert.False(t, ok)
	assert.Empty(t, fallback)

	editState := core.NewContext()
	editState.Set("react.tool_observations", []ToolObservation{
		{Tool: "file_write", Success: true},
	})
	fallback, ok = directCompletionSummary(&core.Task{Instruction: "Edit foo.txt by appending DONE."}, editState)
	assert.True(t, ok)
	assert.Contains(t, fallback, "applied the requested changes")

	assert.Equal(t, "stderr text", verificationNoEditSummary("cli_go", "", "stderr text"))
	assert.Equal(t, "cli_go verification passed", verificationNoEditSummary("cli_go", "", ""))

	assert.False(t, verificationStopAllowed(nil, &core.Task{Instruction: "Fix the bug."}))
	assert.True(t, verificationStopAllowed(nil, &core.Task{Instruction: "Run tests and confirm the result."}))

	completeTask := &core.Task{Instruction: "Summarize README.md and explain it."}
	readOnlyState := core.NewContext()
	readOnlyState.Set("react.tool_observations", []ToolObservation{
		{
			Tool:    "file_read",
			Success: true,
			Args:    map[string]interface{}{"path": "README.md"},
			Data:    map[string]interface{}{"snippet": "", "summary": "project summary"},
		},
	})
	completed, ok := readOnlySummaryFromState(completeTask, readOnlyState, map[string]interface{}{})
	assert.True(t, ok)
	assert.Equal(t, "Summary of README.md: project summary", completed)
}

func TestReactPromptContextSummariesAndDisplayHelpers(t *testing.T) {
	tool := stubTool{name: "cli_go_test", tags: []string{core.TagExecute}}
	assert.True(t, verificationLikeTool(tool))
	assert.Equal(t, 5, toolSummaryBudgetForPhase(contextmgrPhaseExplore))
	assert.Equal(t, 4, toolSummaryBudgetForPhase(contextmgrPhaseEdit))
	assert.Equal(t, 6, toolSummaryBudgetForPhase(contextmgrPhaseVerify))

	payload := &core.ToolResult{
		Success: true,
		Data: map[string]interface{}{
			"summary": "line one\nline two",
		},
	}
	assert.Contains(t, summarizeToolPayload(payload), "line one")
	assert.Equal(t, "line one", truncateForPrompt(" line one ", 20))
	assert.Equal(t, "line on...", truncateForPrompt("line one line two", 7))
}

func TestReactExplicitReadOnlyRetrievalAndCapabilitySnapshotHelpers(t *testing.T) {
	readTool := &recordingReadTool{}
	registry := capability.NewRegistry()
	assert.NoError(t, registry.Register(readTool))
	assert.NoError(t, registry.Register(stubTool{name: "cli_go_test"}))

	agent := &ReActAgent{Tools: registry}

	state := core.NewContext()
	state.Set("react.last_tool_result", map[string]interface{}{
		"results": []any{
			map[string]any{"file": "docs/README.md"},
		},
	})

	task := &core.Task{Instruction: "Use search_semantic to find the docs, then file_read the result."}
	err := agent.completeExplicitReadOnlyRetrieval(context.Background(), task, state)
	assert.NoError(t, err)
	if assert.Len(t, readTool.calls, 1) {
		assert.Equal(t, "docs/README.md", readTool.calls[0]["path"])
	}
	rawLast, ok := state.Get("react.last_tool_result")
	assert.True(t, ok)
	lastMap, ok := rawLast.(map[string]interface{})
	assert.True(t, ok)
	assert.Equal(t, "retrieved content", lastMap["snippet"])
	rawObs, ok := state.Get("react.tool_observations")
	assert.True(t, ok)
	assert.NotEmpty(t, rawObs.([]ToolObservation))

	assert.True(t, isLanguageExecutionTool("cli_cargo", &core.Task{Instruction: "run tests"}))
	assert.True(t, isLanguageExecutionTool("cli_sqlite3", nil))
	assert.False(t, isLanguageExecutionTool("cli_unknown", &core.Task{Instruction: "inspect code"}))

	snapshot := registry.CaptureExecutionCatalogSnapshot()
	assert.NotNil(t, snapshot)
	assert.NotEmpty(t, executionCallableTools(registry, snapshot))

	agent.executionCatalog = snapshot
	catalogSnapshot := agent.executionPolicySnapshot()
	assert.NotNil(t, catalogSnapshot)

	if desc, ok := agent.executionCapabilityDescriptor("file_read"); assert.True(t, ok) {
		assert.Equal(t, "file_read", desc.Name)
	}
	if desc, ok := agent.executionCapabilityDescriptor("tool:file_read"); assert.True(t, ok) {
		assert.Equal(t, "file_read", desc.Name)
	}
}

func TestReactCapabilitySelectionAndDisplayHelpers(t *testing.T) {
	registry := capability.NewRegistry()
	assert.NoError(t, registry.Register(stubTool{name: "echo"}))

	catalog := registry.CaptureExecutionCatalogSnapshot()
	registryTools := executionCallableTools(registry, nil)
	assert.Len(t, registryTools, 1)
	assert.Equal(t, "echo", registryTools[0].Name())

	catalogTools := executionCallableTools(nil, catalog)
	assert.Len(t, catalogTools, 1)
	assert.Equal(t, "echo", catalogTools[0].Name())

	agent := &ReActAgent{Tools: registry}
	assert.NotNil(t, agent.executionPolicySnapshot())

	desc, ok := agent.executionCapabilityDescriptor("echo")
	assert.True(t, ok)
	assert.Equal(t, "echo", desc.Name)

	assert.Equal(t, "echo", capabilityDisplayName("echo", nil))
	assert.Equal(t, "tool:echo", capabilityDisplayName("", &core.CapabilityResultEnvelope{
		Descriptor: core.CapabilityDescriptor{ID: "tool:echo"},
	}))
	assert.Equal(t, "capability", capabilityDisplayName("", nil))
}
