package conformance

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	capports "codeburg.org/lexbit/relurpify/capability/ports"
	"codeburg.org/lexbit/relurpify/model"
	"codeburg.org/lexbit/relurpify/platform/llm"
	"codeburg.org/lexbit/relurpify/platform/llm/offline"
	"codeburg.org/lexbit/relurpify/platform/search"
	"codeburg.org/lexbit/relurpify/testsuite/testhelper"
)

type grepRun struct {
	trace  grepTrace
	result string
}

type grepTrace struct {
	Calls []grepCall
}

type grepCall struct {
	AvailableTools  []string
	LastToolMessage string
	ResponseText    string
	ResponseCalls   []grepToolCall
}

type grepToolCall struct {
	Name string
	Args map[string]any
}

func TestGrepConformance_NativeAndFallbackMatch(t *testing.T) {
	native := runGrepTurn(t, true)
	fallback := runGrepTurn(t, false)

	require.NotEmpty(t, native.trace.Calls)
	require.NotEmpty(t, fallback.trace.Calls)
	require.Equal(t, native.trace, fallback.trace)
	require.Equal(t, native.result, fallback.result)
	require.True(t, hasGrepToolCall(native.trace, "search_grep"))
	require.Contains(t, native.result, "needle")
	require.Contains(t, native.result, "<workspace>/docs/target.txt")
	require.NotContains(t, native.result, "/outside/")
}

func runGrepTurn(t *testing.T, native bool) grepRun {
	t.Helper()

	workspace := t.TempDir()
	testhelper.WriteCleanWorkspace(t, workspace, testhelper.WorkspaceOpts{
		Provider: "offline",
		SeedFiles: map[string]string{
			"docs/other.txt":  "no match here\n",
			"docs/target.txt": "needle in a haystack\n",
		},
	})

	var llmModel model.LanguageModel
	if native {
		llmModel = offline.NewModel()
	} else {
		llmModel = llm.NewFallbackToolModel(offline.NewModel())
	}

	tools := []model.LLMToolSpec{
		capports.LLMToolSpecFromTool(&search.GrepTool{BasePath: workspace}),
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	messages := []model.Message{{Role: "user", Content: "grep the workspace for needle"}}
	opts := &model.LLMOptions{Config: map[string]any{"offline_scenario": "tool:search_grep:needle|docs"}}
	trace := grepTrace{}
	var finalText string

	for range [4]struct{}{} {
		resp, err := llmModel.ChatWithTools(ctx, messages, tools, opts)
		require.NoError(t, err)
		require.NotNil(t, resp)

		trace.Calls = append(trace.Calls, grepCall{
			AvailableTools:  toolNames(tools),
			LastToolMessage: normalizeText(lastMessageByRole(messages, "tool"), workspace),
			ResponseText:    normalizeText(strings.TrimSpace(resp.Text), workspace),
			ResponseCalls:   normalizeGrepToolCalls(resp.ToolCalls),
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
			summary := executeGrepTool(t, workspace, call)
			messages = append(messages, model.Message{
				Role:       "tool",
				Name:       call.Name,
				Content:    summary,
				ToolCallID: call.ID,
			})
		}
	}

	require.NotEmpty(t, finalText)
	return grepRun{trace: trace, result: finalText}
}

func executeGrepTool(t *testing.T, workspace string, call model.ToolCall) string {
	t.Helper()

	require.Equal(t, "search_grep", strings.TrimSpace(call.Name))
	tool := &search.GrepTool{BasePath: workspace}
	result, err := tool.Execute(context.Background(), call.Args)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.True(t, result.Success)

	data, err := json.Marshal(result.Data)
	require.NoError(t, err)
	return normalizeText(string(data), workspace)
}

func normalizeGrepToolCalls(calls []model.ToolCall) []grepToolCall {
	out := make([]grepToolCall, 0, len(calls))
	for _, call := range calls {
		out = append(out, grepToolCall{
			Name: strings.TrimSpace(call.Name),
			Args: normalizeMap(call.Args),
		})
	}
	return out
}

func hasGrepToolCall(trace grepTrace, name string) bool {
	for _, call := range trace.Calls {
		for _, toolCall := range call.ResponseCalls {
			if strings.TrimSpace(toolCall.Name) == strings.TrimSpace(name) {
				return true
			}
		}
	}
	return false
}
