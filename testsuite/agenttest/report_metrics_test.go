package agenttest

import (
	"testing"
	"time"

	"codeburg.org/lexbit/relurpify/model"
	telemetry "codeburg.org/lexbit/relurpify/telemetry"
)

func TestApplyRecordedTelemetryFillsMeasurementFields(t *testing.T) {
	base := time.Date(2026, 10, 7, 12, 0, 0, 0, time.UTC)
	events := []telemetry.Event{
		{
			Type:      telemetry.EventToolCall,
			Timestamp: base,
			Metadata:  map[string]any{"tool": "file_read", "agent_id": "a1"},
		},
		{
			Type:      telemetry.EventToolCall,
			Timestamp: base.Add(1 * time.Second),
			Metadata:  map[string]any{"tool": "file_write", "agent_id": "a1"},
		},
		{
			Type:      telemetry.EventToolResult,
			Timestamp: base.Add(2 * time.Second),
			Metadata: map[string]any{
				"tool":        "file_read",
				"success":     true,
				"duration_ms": int64(2000),
			},
		},
		{
			Type:      telemetry.EventLLMResponse,
			Timestamp: base.Add(3 * time.Second),
			Metadata: map[string]any{
				"usage": model.TokenUsage{PromptTokens: 100, CompletionTokens: 40, TotalTokens: 140},
			},
		},
		{
			Type:      telemetry.EventLLMResponse,
			Timestamp: base.Add(4 * time.Second),
			Metadata: map[string]any{
				"usage": model.TokenUsage{PromptTokens: 10, CompletionTokens: 0, TotalTokens: 0},
			},
		},
		{
			Type:      telemetry.EventToolEdited,
			Timestamp: base.Add(5 * time.Second),
			Metadata:  map[string]any{"path": "src/lib.go", "origin": "file_write"},
		},
	}

	var report CaseReport
	applyRecordedTelemetry(&report, events)

	if len(report.ToolCalls) != 2 {
		t.Fatalf("expected 2 distinct tools, got %#v", report.ToolCalls)
	}
	if report.ToolCalls["file_read"] != 1 || report.ToolCalls["file_write"] != 1 {
		t.Fatalf("unexpected tool counts: %#v", report.ToolCalls)
	}
	if report.TokenUsage.LLMCalls != 2 {
		t.Fatalf("expected 2 LLM calls, got %d", report.TokenUsage.LLMCalls)
	}
	if report.TokenUsage.PromptTokens != 110 || report.TokenUsage.CompletionTokens != 40 || report.TokenUsage.TotalTokens != 150 {
		t.Fatalf("unexpected token usage: %+v", report.TokenUsage)
	}
	if report.ToolLatencies == nil {
		t.Fatal("expected tool latencies to be populated")
	}
	if stats := report.ToolLatencies["file_read"]; stats.MinMs != 2000 || stats.MaxMs != 2000 {
		t.Fatalf("unexpected file_read latency stats: %+v", stats)
	}
	if report.TotalToolTimeMs != 2000 {
		t.Fatalf("expected total tool time 2000ms, got %d", report.TotalToolTimeMs)
	}
	if len(report.ChangedFiles) != 1 || report.ChangedFiles[0] != "src/lib.go" {
		t.Fatalf("unexpected changed files: %#v", report.ChangedFiles)
	}
	if len(report.SecurityObservations) != 1 {
		t.Fatalf("expected 1 security observation from EventToolEdited, got %d", len(report.SecurityObservations))
	}
}

func TestApplyRecordedTelemetryTokenUsageMapShape(t *testing.T) {
	events := []telemetry.Event{
		{
			Type: telemetry.EventLLMResponse,
			Metadata: map[string]any{
				"usage": map[string]any{"prompt_tokens": 5, "completion_tokens": 3},
			},
		},
	}
	var report CaseReport
	applyRecordedTelemetry(&report, events)
	if report.TokenUsage.LLMCalls != 1 || report.TokenUsage.TotalTokens != 8 {
		t.Fatalf("unexpected token usage: %+v", report.TokenUsage)
	}
}

func TestExtractPhaseMetricsAttributesEventsToRunningPhase(t *testing.T) {
	base := time.Date(2026, 10, 7, 9, 0, 0, 0, time.UTC)
	events := []telemetry.Event{
		{Type: telemetry.EventType("euclo.step.started"), Timestamp: base, Metadata: map[string]any{"paradigm": "analysis"}},
		{Type: telemetry.EventLLMResponse, Timestamp: base.Add(1 * time.Second), Metadata: map[string]any{"usage": model.TokenUsage{TotalTokens: 50}}},
		{Type: telemetry.EventType("euclo.step.completed"), Timestamp: base.Add(2 * time.Second), Metadata: map[string]any{"paradigm": "analysis"}},
		{Type: telemetry.EventType("euclo.step.started"), Timestamp: base.Add(3 * time.Second), Metadata: map[string]any{"paradigm": "execution"}},
		{Type: telemetry.EventLLMResponse, Timestamp: base.Add(4 * time.Second), Metadata: map[string]any{"usage": model.TokenUsage{TotalTokens: 30}}},
		{Type: telemetry.EventType("euclo.step.completed"), Timestamp: base.Add(5 * time.Second), Metadata: map[string]any{"paradigm": "execution"}},
	}

	metrics := extractPhaseMetrics(events)
	if len(metrics) != 2 {
		t.Fatalf("expected 2 phase metrics, got %#v", metrics)
	}
	byPhase := map[string]PhaseMetric{}
	for _, m := range metrics {
		byPhase[m.Phase] = m
	}
	analysis := byPhase["analysis"]
	if analysis.LLMCalls != 1 || analysis.TokensUsed != 50 {
		t.Fatalf("unexpected analysis phase: %+v", analysis)
	}
	execution := byPhase["execution"]
	if execution.LLMCalls != 1 || execution.TokensUsed != 30 {
		t.Fatalf("unexpected execution phase: %+v", execution)
	}
}

func TestSecurityObservationsFromDenials(t *testing.T) {
	events := []telemetry.Event{
		{
			Type: telemetry.EventToolResult,
			Metadata: map[string]any{
				"tool":       "file_write",
				"tool_error": "tool file_write blocked: permission denied",
			},
		},
		{
			Type: telemetry.EventStateChange,
			Metadata: map[string]any{
				"security_event": "capability_denied",
				"capability_id":  "file_delete",
				"reason":         "denied by selector policy",
			},
		},
	}
	observations := securityObservationsFromEvents(events)
	if len(observations) != 2 {
		t.Fatalf("expected 2 observations, got %#v", observations)
	}
	if !observations[0].Blocked || observations[0].InScope {
		t.Fatalf("unexpected denial observation: %+v", observations[0])
	}
	if observations[1].PolicyRule != "denied by selector policy" {
		t.Fatalf("unexpected security event observation: %+v", observations[1])
	}
}
