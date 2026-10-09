package runtime

import (
	"bytes"
	"encoding/json"
	"log"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"codeburg.org/lexbit/relurpify/execution/workspace"
	"codeburg.org/lexbit/relurpify/telemetry"
)

func TestNewDegradedRuntime_EmitsBootDegraded(t *testing.T) {
	var buf bytes.Buffer
	prev := log.Writer()
	log.SetOutput(&buf)
	defer log.SetOutput(prev)

	cfg := Config{Workspace: t.TempDir()}
	rt := newDegradedRuntime(nil, cfg, &testDegradedErr{s: "sandbox unavailable"})
	require.NotNil(t, rt)

	output := buf.String()
	require.Contains(t, output, "boot.degraded",
		"newDegradedRuntime must emit boot.degraded through the logger-backed telemetry sink")
	require.Contains(t, output, "sandbox unavailable",
		"boot.degraded must include reason")
	// The structured event's metadata must survive serialization to the logger
	// sink: degraded flag and readiness are part of the diagnostic payload
	// (FR-16).
	require.Contains(t, output, "degraded:true")
	require.Contains(t, output, "model_ready:false")
	require.Contains(t, output, "sandbox_ready:false")

	// AC-10: the structured event must also be durably recorded in the
	// workspace telemetry JSONL, not only written to the log.
	jsonlPath := filepath.Join(workspace.StateDir(cfg.Workspace), "telemetry", "workspace.jsonl")
	data, err := os.ReadFile(jsonlPath)
	require.NoError(t, err, "degraded boot must write a JSONL telemetry file")
	var ev telemetry.Event
	require.NoError(t, json.Unmarshal(data, &ev), "JSONL line must be a telemetry event")
	require.Equal(t, telemetry.EventBootDegraded, ev.Type)
	require.Equal(t, "sandbox unavailable", ev.Metadata["reason"])
}

func TestNewDegradedRuntime_NonNilWorkspace(t *testing.T) {
	prev := log.Writer()
	log.SetOutput(log.Writer())
	defer log.SetOutput(prev)

	cfg := Config{Workspace: t.TempDir()}
	rt := newDegradedRuntime(nil, cfg, &testDegradedErr{s: "test"})
	require.NotNil(t, rt)
	require.NotNil(t, rt.AgentWorkspace())
	require.True(t, rt.AgentWorkspace().Readiness.Degraded)
}

type testDegradedErr struct {
	s string
}

func (e *testDegradedErr) Error() string { return e.s }
