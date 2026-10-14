package pipeline

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"codeburg.org/lexbit/relurpify/capability/ports"
	"codeburg.org/lexbit/relurpify/model"
)

func TestResolveStageToolsEdgeCases(t *testing.T) {
	ctx := context.Background()

	assert.Nil(t, resolveStageTools(ctx, nil, nil, []ports.Tool{&fakeTool{name: "a"}}),
		"nil stage resolves no tools")
	assert.Nil(t, resolveStageTools(ctx, nil, newFakeStage("s"), nil),
		"no available tools resolves none")

	// Stage without ToolScopedStage narrows to nothing.
	assert.Nil(t, resolveStageTools(ctx, nil, newFakeStage("s"), []ports.Tool{&fakeTool{name: "a"}}))

	// Scoped stage with no allowed names resolves none.
	scoped := newFakeStage("s")
	scoped.allowedToolNames = []string{"  ", ""}
	assert.Nil(t, resolveStageTools(ctx, nil, scoped, []ports.Tool{&fakeTool{name: "a"}}))

	// Nil entries in the available list are skipped, unavailable tools excluded,
	// and whitespace-padded allowed names match after trimming.
	scoped = newFakeStage("s")
	scoped.allowedToolNames = []string{"read", " gone "}
	tools := resolveStageTools(ctx, nil, scoped, []ports.Tool{
		nil,
		&fakeTool{name: "read"},
		&unavailableTool{fakeTool: fakeTool{name: "gone"}},
		&fakeTool{name: "other"},
	})
	require.Len(t, tools, 1)
	assert.Equal(t, "read", tools[0].Name())
}

type unavailableTool struct {
	fakeTool
}

func (t *unavailableTool) IsAvailable(_ context.Context) bool { return false }

func TestFormatToolObservations(t *testing.T) {
	assert.Empty(t, formatToolObservations(nil))

	encoded := formatToolObservations([]toolObservation{
		{Name: "read", Success: true, Data: map[string]any{"n": 1}},
		{Name: "write", Success: false, Error: "denied"},
	})
	var decoded []toolObservation
	require.NoError(t, json.Unmarshal([]byte(encoded), &decoded))
	require.Len(t, decoded, 2)
	assert.Equal(t, "read", decoded[0].Name)
	assert.True(t, decoded[0].Success)
	assert.Equal(t, "denied", decoded[1].Error)

	// Arguments that cannot marshal degrade to fmt.Sprint instead of failing.
	obs := []toolObservation{{Name: "bad", Args: map[string]any{"ch": make(chan int)}}}
	degraded := formatToolObservations(obs)
	assert.True(t, strings.Contains(degraded, "bad"), "fallback rendering keeps observation data, got %q", degraded)
}

func TestExecuteToolCallsNilToolEntriesAreSkippedFromIndex(t *testing.T) {
	calls := []model.ToolCall{{Name: "real", Args: map[string]any{}}}
	tools := []ports.Tool{nil, &fakeTool{name: "real"}}
	observations, err := executeToolCalls(context.Background(), nil, calls, tools, &mockInvoker{})
	require.NoError(t, err)
	require.Len(t, observations, 1)
	assert.Equal(t, "real", observations[0].Name)
}

func TestExecuteToolCallsFailureResultRecordsErrorAndData(t *testing.T) {
	calls := []model.ToolCall{{Name: "t", Args: map[string]any{"k": "v"}}}
	invoker := &mockInvoker{results: map[string]*ports.ToolResult{
		"t": {Success: false, Error: "nope", Data: map[string]any{"reason": "locked"}},
	}}
	observations, err := executeToolCalls(context.Background(), nil, calls, []ports.Tool{&fakeTool{name: "t"}}, invoker)
	require.NoError(t, err)
	require.Len(t, observations, 1)
	assert.False(t, observations[0].Success)
	assert.Equal(t, "nope", observations[0].Error)
	assert.Equal(t, "locked", observations[0].Data["reason"])
	assert.Equal(t, "v", observations[0].Args["k"])
}

func TestExecuteToolCallsMultipleCallsAllIndexed(t *testing.T) {
	calls := []model.ToolCall{
		{Name: "a"},
		{Name: "b"},
	}
	observations, err := executeToolCalls(context.Background(), nil, calls,
		[]ports.Tool{&fakeTool{name: "a"}, &fakeTool{name: "b"}}, &mockInvoker{})
	require.NoError(t, err)
	require.Len(t, observations, 2)
	assert.Equal(t, "a", observations[0].Name)
	assert.Equal(t, "b", observations[1].Name)
}
