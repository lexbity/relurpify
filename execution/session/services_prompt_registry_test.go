package session

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"codeburg.org/lexbit/relurpify/execution/prompt"
	"codeburg.org/lexbit/relurpify/telemetry"
)

// recordingTelemetry is a telemetry.Telemetry double capturing every event.
type recordingTelemetry struct {
	mu    sync.Mutex
	order []telemetry.Event
}

func (r *recordingTelemetry) Emit(ev telemetry.Event) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.order = append(r.order, ev)
}

func (r *recordingTelemetry) events() []telemetry.Event {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]telemetry.Event(nil), r.order...)
}

func TestBuildPromptRegistryLoadsWorkspacePromptFiles(t *testing.T) {
	ws := t.TempDir()
	promptDir := filepath.Join(ws, "relurpify_cfg", "prompts")
	require.NoError(t, os.MkdirAll(promptDir, 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(promptDir, "greet.prompt"), []byte(
		"---\nschema framework.prompt/v2\nid greet\ntag \"system\"\n---\n\nHello body\n"), 0o600))

	registry, err := BuildPromptRegistry(ws, nil)
	require.NoError(t, err)
	assert.Equal(t, 1, registry.Count(), "the workspace .prompt file must be loaded")
}

func TestBuildPromptRegistryToleratesMissingPromptDirectories(t *testing.T) {
	registry, err := BuildPromptRegistry(t.TempDir(), nil)
	require.NoError(t, err)
	assert.Equal(t, 0, registry.Count())
}

func TestBuildPromptRegistryWithTelemetryAdapter(t *testing.T) {
	tel := &recordingTelemetry{}
	registry, err := BuildPromptRegistry(t.TempDir(), tel)
	require.NoError(t, err)
	require.NotNil(t, registry)
}

func TestPromptTelemetryAdapterEmitsResolvedEvent(t *testing.T) {
	tel := &recordingTelemetry{}
	adapter := promptTelemetryAdapter{inner: tel}

	ctx := context.Background()
	adapter.EmitPromptResolved(ctx, prompt.ResolvedEvent{
		ID:             "greet",
		Paradigm:       "react",
		OutputLength:   128,
		BlocksIncluded: 2,
		BlocksExcluded: 1,
		ProvidersUsed:  []string{"core"},
		DurationMs:     12,
		CacheHit:       true,
	})

	events := tel.events()
	require.Len(t, events, 1)
	ev := events[0]
	assert.Equal(t, telemetry.EventType("prompt.resolved"), ev.Type)
	assert.Equal(t, "greet", ev.TaskID)
	assert.Equal(t, "prompt greet resolved: 128 chars, 2 blocks", ev.Message)
	assert.Equal(t, "react", ev.Metadata["paradigm"])
	assert.Equal(t, 2, ev.Metadata["blocks_included"])
	assert.Equal(t, 1, ev.Metadata["blocks_excluded"])
	assert.Equal(t, int64(12), ev.Metadata["duration_ms"])
	assert.Equal(t, true, ev.Metadata["cache_hit"])
	require.Len(t, ev.Metadata["providers_used"].([]string), 1)
}

func TestPromptTelemetryAdapterEmitsResolveFailedEvent(t *testing.T) {
	tel := &recordingTelemetry{}
	adapter := promptTelemetryAdapter{inner: tel}

	adapter.EmitPromptResolveFailed(context.Background(), prompt.ResolveFailedEvent{
		ID:         "greet",
		Paradigm:   "react",
		Error:      "no such provider",
		DurationMs: 4,
	})

	events := tel.events()
	require.Len(t, events, 1)
	assert.Equal(t, telemetry.EventType("prompt.resolve_failed"), events[0].Type)
	assert.Equal(t, "prompt greet resolve failed: no such provider", events[0].Message)
	assert.Equal(t, "no such provider", events[0].Metadata["error"])
}

func TestPromptTelemetryAdapterEmitsContextMissingEvent(t *testing.T) {
	tel := &recordingTelemetry{}
	adapter := promptTelemetryAdapter{inner: tel}

	adapter.EmitPromptContextMissing(context.Background(), prompt.ContextMissingEvent{
		PromptID: "greet",
		BlockID:  "header",
		Key:      "session.user",
		Message:  "undefined state key",
	})

	events := tel.events()
	require.Len(t, events, 1)
	assert.Equal(t, telemetry.EventType("prompt.context_missing"), events[0].Type)
	assert.Equal(t, "prompt greet/header: undefined state key", events[0].Message)
	assert.Equal(t, "header", events[0].Metadata["block_id"])
	assert.Equal(t, "session.user", events[0].Metadata["key"])
}

func TestPromptTelemetryAdapterEmitsValidationIssueEvent(t *testing.T) {
	tel := &recordingTelemetry{}
	adapter := promptTelemetryAdapter{inner: tel}

	adapter.EmitPromptValidationIssue(context.Background(), prompt.ValidationIssueEvent{
		Issue: prompt.ValidationIssue{
			PromptID: "greet",
			BlockID:  "header",
			Severity: prompt.SeverityError,
			Message:  "unknown schema",
		},
	})

	events := tel.events()
	require.Len(t, events, 1)
	assert.Equal(t, telemetry.EventType("prompt.validation_issue"), events[0].Type)
	assert.Equal(t, "greet", events[0].TaskID)
	assert.Equal(t, "error: [greet/header] unknown schema", events[0].Message)
	assert.Equal(t, "error", events[0].Metadata["severity"])
	assert.Equal(t, "header", events[0].Metadata["block_id"])
}

func TestPromptTelemetryAdapterEmitsProviderFailedEvent(t *testing.T) {
	tel := &recordingTelemetry{}
	adapter := promptTelemetryAdapter{inner: tel}

	adapter.EmitPromptProviderFailed(context.Background(), prompt.ProviderFailedEvent{
		PromptID:     "greet",
		BlockID:      "header",
		ProviderName: "clock",
		Error:        "provider exploded",
	})

	events := tel.events()
	require.Len(t, events, 1)
	assert.Equal(t, telemetry.EventType("prompt.provider_failed"), events[0].Type)
	assert.Equal(t, "prompt greet/header provider clock failed: provider exploded", events[0].Message)
	assert.Equal(t, "clock", events[0].Metadata["provider_name"])
}

func TestPromptTelemetryAdapterNilInnerIsNoop(t *testing.T) {
	adapter := promptTelemetryAdapter{inner: nil}
	ctx := context.Background()
	assert.NotPanics(t, func() {
		adapter.EmitPromptResolved(ctx, prompt.ResolvedEvent{ID: "x"})
		adapter.EmitPromptResolveFailed(ctx, prompt.ResolveFailedEvent{ID: "x"})
		adapter.EmitPromptContextMissing(ctx, prompt.ContextMissingEvent{PromptID: "x"})
		adapter.EmitPromptValidationIssue(ctx, prompt.ValidationIssueEvent{})
		adapter.EmitPromptProviderFailed(ctx, prompt.ProviderFailedEvent{PromptID: "x"})
	})
}
