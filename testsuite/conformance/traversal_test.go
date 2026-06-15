package conformance

import (
	"context"
	"fmt"
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

type traversalRun struct {
	trace  traversalTrace
	result string
}

type traversalTrace struct {
	Calls []traversalCall
}

type traversalCall struct {
	AvailableTools  []string
	LastToolMessage string
	ResponseText    string
	ResponseCalls   []traversalToolCall
}

type traversalToolCall struct {
	Name string
	Args map[string]any
}

func TestTraversalConformance_NativeAndFallbackMatch(t *testing.T) {
	native := runTraversalTurn(t, true)
	fallback := runTraversalTurn(t, false)

	require.NotEmpty(t, native.trace.Calls)
	require.NotEmpty(t, fallback.trace.Calls)
	require.Equal(t, native.trace, fallback.trace)
	require.Equal(t, native.result, fallback.result)
	require.True(t, hasTraversalToolCall(native.trace, "file_list"))
	require.True(t, hasTraversalToolCall(native.trace, "file_read"))
}

func runTraversalTurn(t *testing.T, native bool) traversalRun {
	t.Helper()

	workspace := t.TempDir()
	testhelper.WriteCleanWorkspace(t, workspace, testhelper.WorkspaceOpts{
		Provider: "offline",
		SeedFiles: map[string]string{
			"docs/other.txt":  "other payload\n",
			"docs/target.txt": "target payload\n",
		},
	})

	var llmModel model.LanguageModel
	if native {
		llmModel = offline.NewModel()
	} else {
		llmModel = llm.NewFallbackToolModel(offline.NewModel())
	}

	scope := fs.NewFileScopePolicy(workspace, nil)
	listTool := &fs.ListFilesTool{BasePath: workspace}
	listTool.SetSandboxScope(scope)
	readTool := &fs.ReadFileTool{BasePath: workspace}
	readTool.SetSandboxScope(scope)

	tools := []model.LLMToolSpec{
		capports.LLMToolSpecFromTool(listTool),
		capports.LLMToolSpecFromTool(readTool),
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	messages := []model.Message{{Role: "user", Content: "list the docs directory and read the target file"}}
	opts := &model.LLMOptions{Config: map[string]any{"offline_scenario": "tool:file_list:docs"}}
	trace := traversalTrace{}
	var finalText string

	for step := 0; step < 8; step++ {
		resp, err := llmModel.ChatWithTools(ctx, messages, tools, opts)
		require.NoError(t, err)
		require.NotNil(t, resp)

		trace.Calls = append(trace.Calls, traversalCall{
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
			summary := executeTraversalTool(t, workspace, call)
			messages = append(messages, model.Message{
				Role:       "tool",
				Name:       call.Name,
				Content:    summary,
				ToolCallID: call.ID,
			})
		}
	}

	require.NotEmpty(t, finalText)
	return traversalRun{trace: trace, result: finalText}
}

func executeTraversalTool(t *testing.T, workspace string, call model.ToolCall) string {
	t.Helper()

	scope := fs.NewFileScopePolicy(workspace, nil)
	switch strings.TrimSpace(call.Name) {
	case "file_list":
		tool := &fs.ListFilesTool{BasePath: workspace}
		tool.SetSandboxScope(scope)
		result, err := tool.Execute(context.Background(), call.Args)
		require.NoError(t, err)
		require.NotNil(t, result)
		require.True(t, result.Success)
		files := normalizeStringSlice(asStringSlice(result.Data["files"]))
		return fmt.Sprintf("success=true Listed files: [%s]", strings.Join(files, " "))
	case "file_read":
		tool := &fs.ReadFileTool{BasePath: workspace}
		tool.SetSandboxScope(scope)
		result, err := tool.Execute(context.Background(), call.Args)
		require.NoError(t, err)
		require.NotNil(t, result)
		require.True(t, result.Success)
		path := strings.TrimSpace(fmt.Sprint(call.Args["path"]))
		content := strings.TrimSpace(fmt.Sprint(result.Data["content"]))
		return fmt.Sprintf("success=true Read %s %s", path, content)
	default:
		t.Fatalf("unexpected tool call %q", call.Name)
	}
	return ""
}

func toolNames(tools []model.LLMToolSpec) []string {
	names := make([]string, 0, len(tools))
	for _, tool := range tools {
		names = append(names, strings.TrimSpace(tool.Name))
	}
	return names
}

func normalizeToolCalls(calls []model.ToolCall) []traversalToolCall {
	out := make([]traversalToolCall, 0, len(calls))
	for _, call := range calls {
		out = append(out, traversalToolCall{
			Name: strings.TrimSpace(call.Name),
			Args: normalizeMap(call.Args),
		})
	}
	return out
}

func normalizeMap(value map[string]any) map[string]any {
	if value == nil {
		return nil
	}
	out := make(map[string]any, len(value))
	for k, v := range value {
		out[k] = normalizeAny(v)
	}
	return out
}

func normalizeAny(value any) any {
	switch typed := value.(type) {
	case string:
		return strings.TrimSpace(typed)
	case float64:
		if typed == float64(int64(typed)) {
			return int64(typed)
		}
		return typed
	case int64:
		return typed
	case int:
		return int64(typed)
	case map[string]any:
		return normalizeMap(typed)
	case []any:
		out := make([]any, len(typed))
		for i := range typed {
			out[i] = normalizeAny(typed[i])
		}
		return out
	case []string:
		out := make([]any, len(typed))
		for i := range typed {
			out[i] = strings.TrimSpace(typed[i])
		}
		return out
	default:
		return value
	}
}

func normalizeStringSlice(values []string) []string {
	out := make([]string, len(values))
	for i, value := range values {
		out[i] = strings.TrimSpace(value)
	}
	return out
}

func asStringSlice(value any) []string {
	switch typed := value.(type) {
	case []string:
		return append([]string(nil), typed...)
	case []any:
		out := make([]string, 0, len(typed))
		for _, item := range typed {
			out = append(out, strings.TrimSpace(fmt.Sprint(item)))
		}
		return out
	default:
		if value == nil {
			return nil
		}
		return []string{strings.TrimSpace(fmt.Sprint(value))}
	}
}

func lastMessageByRole(messages []llm.Message, role string) string {
	for i := len(messages) - 1; i >= 0; i-- {
		if strings.EqualFold(strings.TrimSpace(messages[i].Role), role) {
			return strings.TrimSpace(messages[i].Content)
		}
	}
	return ""
}

func normalizeText(text, workspace string) string {
	text = strings.TrimSpace(text)
	if text == "" || workspace == "" {
		return text
	}
	return strings.ReplaceAll(text, workspace, "<workspace>")
}

func hasTraversalToolCall(trace traversalTrace, name string) bool {
	for _, call := range trace.Calls {
		for _, toolCall := range call.ResponseCalls {
			if strings.TrimSpace(toolCall.Name) == strings.TrimSpace(name) {
				return true
			}
		}
	}
	return false
}
