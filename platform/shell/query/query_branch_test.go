package query

import (
	"context"
	"testing"

	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/platform/shell/catalog"
	shelltelemetry "github.com/lexcodex/relurpify/platform/shell/telemetry"
	"github.com/stretchr/testify/require"
)

func TestQueryToolsWithNilEngineAndEmptyResults(t *testing.T) {
	tools := Tools(nil)
	require.Len(t, tools, 2)
	for _, tool := range tools {
		require.True(t, tool.IsAvailable(context.Background(), core.NewContext()))
		require.Equal(t, []string{core.TagReadOnly, "search"}, tool.Tags())
	}

	require.Nil(t, discoveryMatchesToData(nil))
	require.Nil(t, cloneMap(nil))

	var nilEngine *Engine
	nilEngine.emitTelemetry("tool_call", "ignored", nil)

	telemetry := &struct {
		events []shelltelemetry.Event
	}{}
	engine := NewEngineWithTelemetry(nil, telemetrySinkFunc(func(event shelltelemetry.Event) {
		telemetry.events = append(telemetry.events, event)
	}))
	engine.emitTelemetry("tool_call", "recorded", nil)
	require.Len(t, telemetry.events, 1)
	require.Equal(t, "tool_call", telemetry.events[0].Type)
	require.NotZero(t, telemetry.events[0].Timestamp)

	require.Equal(t, "", (InstantiationQuery{}).ArgumentString("missing"))
	require.Equal(t, "123", (InstantiationQuery{Arguments: map[string]any{"n": 123}}).ArgumentString("n"))
	require.Error(t, DiscoveryQuery{}.Validate())
	require.Error(t, InstantiationQuery{}.Validate())
	_, err := ParseDiscoveryQuery(nil)
	require.Error(t, err)
	_, err = ParseInstantiationQuery(nil)
	require.Error(t, err)
}

func TestQueryToolExecuteRejectsBadInput(t *testing.T) {
	tools := Tools(testCatalog(t))
	var discovery, instantiation core.Tool
	for _, tool := range tools {
		switch tool.Name() {
		case discoveryToolName:
			discovery = tool
		case instantiationToolName:
			instantiation = tool
		}
	}

	res, err := discovery.Execute(context.Background(), core.NewContext(), map[string]interface{}{"unknown": true})
	require.NoError(t, err)
	require.False(t, res.Success)
	require.Contains(t, res.Error, "unknown discovery field")

	res, err = instantiation.Execute(context.Background(), core.NewContext(), map[string]interface{}{"unknown": true})
	require.NoError(t, err)
	require.False(t, res.Success)
	require.Contains(t, res.Error, "unknown instantiation field")
}

func TestQueryEngineSkipsDeprecatedAndTruncatesResults(t *testing.T) {
	cat := testCatalog(t)
	require.NoError(t, cat.Register(catalog.EntryFromCommandSpec(catalog.CommandToolSpec{
		Name:        "cli_old_git",
		Aliases:     []string{"old-git"},
		Family:      "fileops",
		Intent:      []string{"repository"},
		Description: "Deprecated git wrapper.",
		Command:     "git",
		Deprecated:  true,
		Replacement: "cli_git",
	})))
	engine := NewEngine(cat)

	result, err := engine.Search(DiscoveryQuery{
		Intent:          []string{"build"},
		MaxResults:      1,
		AllowDeprecated: false,
	})
	require.NoError(t, err)
	require.Len(t, result.Matches, 1)
	require.Equal(t, "cli_cargo", result.Matches[0].Entry.Name)

	_, err = engine.Instantiate(InstantiationQuery{
		Aliases:         []string{"old-git"},
		AllowDeprecated: false,
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "deprecated")
}

type telemetrySinkFunc func(shelltelemetry.Event)

func (f telemetrySinkFunc) Emit(event shelltelemetry.Event) {
	f(event)
}
