package offline

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"codeburg.org/lexbit/relurpify/model"
)

func TestModelGreetingIsDeterministic(t *testing.T) {
	m := NewModel()
	resp1, err := m.Chat(context.Background(), nil, &model.LLMOptions{})
	require.NoError(t, err)
	resp2, err := m.Chat(context.Background(), nil, &model.LLMOptions{})
	require.NoError(t, err)
	require.Equal(t, "euclo online (offline backend).", resp1.Text)
	require.Equal(t, resp1, resp2)
}

func TestModelEchoUsesLatestUserMessage(t *testing.T) {
	m := NewModel()
	resp, err := m.Chat(context.Background(), []model.Message{{Role: "user", Content: "hello"}}, &model.LLMOptions{Config: map[string]any{"offline_scenario": "echo"}})
	require.NoError(t, err)
	require.Equal(t, "hello", resp.Text)
}

func TestModelGenerateFallbackEmitsToolWireText(t *testing.T) {
	m := NewModel()
	resp, err := m.Generate(context.Background(), "Conversation:\n[user]\nread\n", &model.LLMOptions{Config: map[string]any{"offline_scenario": "tool:file_read:/tmp/demo.txt"}})
	require.NoError(t, err)
	require.Contains(t, resp.Text, "```tool")
	require.Contains(t, resp.Text, `"tool":"file_read"`)
	require.Contains(t, resp.Text, `"/tmp/demo.txt"`)
}

func TestModelGenerateFallbackSummarizesToolResult(t *testing.T) {
	m := NewModel()
	resp, err := m.Generate(context.Background(), "Conversation:\n[user]\nread\n\n[tool]\n123456\n", &model.LLMOptions{Config: map[string]any{"offline_scenario": "tool:file_read:/tmp/demo.txt"}})
	require.NoError(t, err)
	require.Equal(t, "offline file_read result: 123456", resp.Text)
}

func TestModelFileReadScenarioEmitsToolThenSummarizes(t *testing.T) {
	m := NewModel()
	opts := &model.LLMOptions{Config: map[string]any{"offline_scenario": "tool:file_read:/tmp/demo.txt"}}

	first, err := m.ChatWithTools(context.Background(), []model.Message{{Role: "user", Content: "read"}}, nil, opts)
	require.NoError(t, err)
	require.Len(t, first.ToolCalls, 1)
	require.Equal(t, "file_read", first.ToolCalls[0].Name)
	require.Equal(t, map[string]any{"path": "/tmp/demo.txt"}, first.ToolCalls[0].Args)

	second, err := m.ChatWithTools(context.Background(), []model.Message{
		{Role: "user", Content: "read"},
		{Role: "tool", Content: "123456"},
	}, nil, opts)
	require.NoError(t, err)
	require.Empty(t, second.ToolCalls)
	require.Equal(t, "offline file_read result: 6 bytes", second.Text)
}

func TestModelFileListScenarioEmitsListThenRead(t *testing.T) {
	m := NewModel()
	opts := &model.LLMOptions{Config: map[string]any{"offline_scenario": "tool:file_list:/tmp/workspace/docs"}}

	first, err := m.ChatWithTools(context.Background(), []model.Message{{Role: "user", Content: "inspect"}}, nil, opts)
	require.NoError(t, err)
	require.Len(t, first.ToolCalls, 1)
	require.Equal(t, "file_list", first.ToolCalls[0].Name)
	require.Equal(t, map[string]any{"directory": "/tmp/workspace/docs", "pattern": "*"}, first.ToolCalls[0].Args)

	second, err := m.ChatWithTools(context.Background(), []model.Message{
		{Role: "user", Content: "inspect"},
		{Role: "tool", Content: "success=true Listed files: [/tmp/workspace/docs/target.txt /tmp/workspace/docs/other.txt]"},
	}, nil, opts)
	require.NoError(t, err)
	require.Len(t, second.ToolCalls, 1)
	require.Equal(t, "file_read", second.ToolCalls[0].Name)
	require.Equal(t, map[string]any{"path": "/tmp/workspace/docs/target.txt"}, second.ToolCalls[0].Args)

	third, err := m.ChatWithTools(context.Background(), []model.Message{
		{Role: "user", Content: "inspect"},
		{Role: "tool", Content: "success=true Listed files: [/tmp/workspace/docs/target.txt /tmp/workspace/docs/other.txt]"},
		{Role: "tool", Content: "success=true Read /tmp/workspace/docs/target.txt target payload"},
	}, nil, opts)
	require.NoError(t, err)
	require.Empty(t, third.ToolCalls)
	require.Contains(t, third.Text, "offline file_list result:")
	require.Contains(t, third.Text, "target payload")
}

func TestModelGenerateFallbackFileListScenarioEmitsToolWireText(t *testing.T) {
	m := NewModel()
	opts := &model.LLMOptions{Config: map[string]any{"offline_scenario": "tool:file_list:/tmp/workspace/docs"}}

	resp, err := m.Generate(context.Background(), "Conversation:\n[user]\ninspect\n", opts)
	require.NoError(t, err)
	require.Contains(t, resp.Text, "```tool")
	require.Contains(t, resp.Text, `"tool":"file_list"`)
	require.Contains(t, resp.Text, `"/tmp/workspace/docs"`)

	next, err := m.Generate(context.Background(), "Conversation:\n[user]\ninspect\n\n[tool]\nsuccess=true Listed files: [/tmp/workspace/docs/target.txt /tmp/workspace/docs/other.txt]\n", opts)
	require.NoError(t, err)
	require.Contains(t, next.Text, "```tool")
	require.Contains(t, next.Text, `"tool":"file_read"`)
	require.Contains(t, next.Text, `"/tmp/workspace/docs/target.txt"`)

	final, err := m.Generate(context.Background(), "Conversation:\n[user]\ninspect\n\n[tool]\nsuccess=true Listed files: [/tmp/workspace/docs/target.txt /tmp/workspace/docs/other.txt]\n\n[tool]\nsuccess=true Read /tmp/workspace/docs/target.txt target payload\n", opts)
	require.NoError(t, err)
	require.Contains(t, final.Text, "offline file_list result:")
	require.Contains(t, final.Text, "target payload")
}

func TestModelSearchGrepScenarioEmitsToolThenSummarizes(t *testing.T) {
	m := NewModel()
	opts := &model.LLMOptions{Config: map[string]any{"offline_scenario": "tool:search_grep:needle|docs"}}

	first, err := m.ChatWithTools(context.Background(), []model.Message{{Role: "user", Content: "grep"}}, nil, opts)
	require.NoError(t, err)
	require.Len(t, first.ToolCalls, 1)
	require.Equal(t, "search_grep", first.ToolCalls[0].Name)
	require.Equal(t, map[string]any{"pattern": "needle", "directory": "docs"}, first.ToolCalls[0].Args)

	second, err := m.ChatWithTools(context.Background(), []model.Message{
		{Role: "user", Content: "grep"},
		{Role: "tool", Content: `{"matches":[{"file":"docs/target.txt","line":1,"content":"needle here"}]}`},
	}, nil, opts)
	require.NoError(t, err)
	require.Empty(t, second.ToolCalls)
	require.Equal(t, "offline search_grep result: {\"matches\":[{\"file\":\"docs/target.txt\",\"line\":1,\"content\":\"needle here\"}]}", second.Text)
}

func TestModelGenerateFallbackSearchGrepScenarioEmitsToolWireText(t *testing.T) {
	m := NewModel()
	opts := &model.LLMOptions{Config: map[string]any{"offline_scenario": "tool:search_grep:needle|docs"}}

	resp, err := m.Generate(context.Background(), "Conversation:\n[user]\ngrep\n", opts)
	require.NoError(t, err)
	require.Contains(t, resp.Text, "```tool")
	require.Contains(t, resp.Text, `"tool":"search_grep"`)
	require.Contains(t, resp.Text, `"pattern":"needle"`)
	require.Contains(t, resp.Text, `"directory":"docs"`)

	next, err := m.Generate(context.Background(), "Conversation:\n[user]\ngrep\n\n[tool]\n{\"matches\":[{\"file\":\"docs/target.txt\",\"line\":1,\"content\":\"needle here\"}]}\n", opts)
	require.NoError(t, err)
	require.Equal(t, "offline search_grep result: {\"matches\":[{\"file\":\"docs/target.txt\",\"line\":1,\"content\":\"needle here\"}]}", next.Text)
}

func TestModelFileEditScenarioEmitsToolThenSummarizes(t *testing.T) {
	m := NewModel()
	opts := &model.LLMOptions{Config: map[string]any{"offline_scenario": "tool:file_edit:docs/target.txt|alpha|beta|1"}}

	first, err := m.ChatWithTools(context.Background(), []model.Message{{Role: "user", Content: "edit"}}, nil, opts)
	require.NoError(t, err)
	require.Len(t, first.ToolCalls, 1)
	require.Equal(t, "file_edit", first.ToolCalls[0].Name)
	require.Equal(t, map[string]any{
		"path":           "docs/target.txt",
		"old_string":     "alpha",
		"new_string":     "beta",
		"expected_count": 1,
	}, first.ToolCalls[0].Args)

	second, err := m.ChatWithTools(context.Background(), []model.Message{
		{Role: "user", Content: "edit"},
		{Role: "tool", Content: "success=true content=beta"},
	}, nil, opts)
	require.NoError(t, err)
	require.Empty(t, second.ToolCalls)
	require.Equal(t, "offline file_edit result: success=true content=beta", second.Text)
}

func TestModelGenerateFallbackFileEditScenarioEmitsToolWireText(t *testing.T) {
	m := NewModel()
	opts := &model.LLMOptions{Config: map[string]any{"offline_scenario": "tool:file_edit:docs/target.txt|alpha|beta|1"}}

	resp, err := m.Generate(context.Background(), "Conversation:\n[user]\nedit\n", opts)
	require.NoError(t, err)
	require.Contains(t, resp.Text, "```tool")
	require.Contains(t, resp.Text, `"tool":"file_edit"`)
	require.Contains(t, resp.Text, `"path":"docs/target.txt"`)
	require.Contains(t, resp.Text, `"old_string":"alpha"`)
	require.Contains(t, resp.Text, `"new_string":"beta"`)

	next, err := m.Generate(context.Background(), "Conversation:\n[user]\nedit\n\n[tool]\nsuccess=true content=beta\n", opts)
	require.NoError(t, err)
	require.Equal(t, "offline file_edit result: success=true content=beta", next.Text)
}

func TestModelMultiScenarioAdvancesByToolHistory(t *testing.T) {
	m := NewModel()
	opts := &model.LLMOptions{Config: map[string]any{"offline_scenario": "multi:2"}}

	first, err := m.ChatWithTools(context.Background(), []model.Message{{Role: "user", Content: "go"}}, nil, opts)
	require.NoError(t, err)
	require.Len(t, first.ToolCalls, 1)
	require.Equal(t, "multi_1", first.ToolCalls[0].Name)

	second, err := m.ChatWithTools(context.Background(), []model.Message{
		{Role: "user", Content: "go"},
		{Role: "tool", Content: "step-1"},
	}, nil, opts)
	require.NoError(t, err)
	require.Len(t, second.ToolCalls, 1)
	require.Equal(t, "multi_2", second.ToolCalls[0].Name)

	third, err := m.ChatWithTools(context.Background(), []model.Message{
		{Role: "user", Content: "go"},
		{Role: "tool", Content: "step-1"},
		{Role: "tool", Content: "step-2"},
	}, nil, opts)
	require.NoError(t, err)
	require.Empty(t, third.ToolCalls)
	require.Equal(t, "offline multi complete (2/2)", third.Text)
}
