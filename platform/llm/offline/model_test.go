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
