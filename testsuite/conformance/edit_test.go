package conformance

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	capports "codeburg.org/lexbit/relurpify/capability/ports"
	"codeburg.org/lexbit/relurpify/model"
	"codeburg.org/lexbit/relurpify/platform/fs"
	"codeburg.org/lexbit/relurpify/platform/llm"
	"codeburg.org/lexbit/relurpify/platform/llm/offline"
	"codeburg.org/lexbit/relurpify/testsuite/testhelper"
)

type editRun struct {
	trace  editTrace
	result string
}

type editTrace struct {
	Calls []editCall
}

type editCall struct {
	AvailableTools  []string
	LastToolMessage string
	ResponseText    string
	ResponseCalls   []traversalToolCall
}

func TestEditConformance_NativeAndFallbackMatch(t *testing.T) {
	native := runEditTurn(t, true)
	fallback := runEditTurn(t, false)

	require.NotEmpty(t, native.trace.Calls)
	require.NotEmpty(t, fallback.trace.Calls)
	require.Equal(t, native.trace, fallback.trace)
	require.Equal(t, native.result, fallback.result)
	require.True(t, hasEditToolCall(native.trace, "file_edit"))
}

func runEditTurn(t *testing.T, native bool) editRun {
	t.Helper()

	workspace := t.TempDir()
	testhelper.WriteCleanWorkspace(t, workspace, testhelper.WorkspaceOpts{
		Provider: "offline",
		SeedFiles: map[string]string{
			"docs/target.txt": "alpha\n",
		},
	})

	var llmModel model.LanguageModel
	if native {
		llmModel = offline.NewModel()
	} else {
		llmModel = llm.NewFallbackToolModel(offline.NewModel())
	}

	editTool := &fs.EditFileTool{BasePath: workspace}
	editTool.SetSandboxScope(fs.NewFileScopePolicy(workspace, nil))

	tools := []model.LLMToolSpec{
		capports.LLMToolSpecFromTool(editTool),
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	messages := []model.Message{{Role: "user", Content: "replace alpha with beta in docs/target.txt"}}
	opts := &model.LLMOptions{Config: map[string]any{"offline_scenario": "tool:file_edit:docs/target.txt|alpha|beta|1"}}
	trace := editTrace{}
	var finalText string

	for step := 0; step < 4; step++ {
		resp, err := llmModel.ChatWithTools(ctx, messages, tools, opts)
		require.NoError(t, err)
		require.NotNil(t, resp)

		trace.Calls = append(trace.Calls, editCall{
			AvailableTools:  toolNames(tools),
			LastToolMessage: normalizeText(lastMessageByRole(messages, "tool"), workspace),
			ResponseText:    normalizeText(strings.TrimSpace(resp.Text), workspace),
			ResponseCalls:   normalizeToolCalls(resp.ToolCalls),
		})

		if len(resp.ToolCalls) == 0 {
			finalText = strings.TrimSpace(resp.Text)
			break
		}

		messages = append(messages, model.Message{
			Role:      "assistant",
			Content:   resp.Text,
			ToolCalls: append([]model.ToolCall(nil), resp.ToolCalls...),
		})
		for _, call := range resp.ToolCalls {
			summary := executeEditTool(t, workspace, call)
			messages = append(messages, model.Message{
				Role:       "tool",
				Name:       call.Name,
				Content:    summary,
				ToolCallID: call.ID,
			})
		}
	}

	content, err := os.ReadFile(filepath.Join(workspace, "docs", "target.txt"))
	require.NoError(t, err)
	require.Equal(t, "beta\n", string(content))
	require.NotEmpty(t, finalText)
	return editRun{trace: trace, result: finalText}
}

func executeEditTool(t *testing.T, workspace string, call model.ToolCall) string {
	t.Helper()

	require.Equal(t, "file_edit", strings.TrimSpace(call.Name))
	tool := &fs.EditFileTool{BasePath: workspace}
	tool.SetSandboxScope(fs.NewFileScopePolicy(workspace, nil))
	result, err := tool.Execute(context.Background(), call.Args)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.True(t, result.Success)

	content, err := os.ReadFile(filepath.Join(workspace, "docs", "target.txt"))
	require.NoError(t, err)
	return fmt.Sprintf("success=true content=%s", strings.TrimSpace(string(content)))
}

func hasEditToolCall(trace editTrace, name string) bool {
	for _, call := range trace.Calls {
		for _, toolCall := range call.ResponseCalls {
			if strings.TrimSpace(toolCall.Name) == strings.TrimSpace(name) {
				return true
			}
		}
	}
	return false
}
