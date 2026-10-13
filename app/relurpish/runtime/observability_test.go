package runtime

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"codeburg.org/lexbit/relurpify/execution/workspace"
)

// TestNewDegradedRuntime_EmitsBootDegraded proves AC-7: a degraded boot records
// boot.degraded durably in the OTLP-JSON file leg.
func TestNewDegradedRuntime_EmitsBootDegraded(t *testing.T) {
	cfg := Config{Workspace: t.TempDir()}
	rt := newDegradedRuntime(context.Background(), cfg, &testDegradedErr{s: "sandbox unavailable"})
	require.NotNil(t, rt)
	require.NoError(t, rt.Close(context.Background()))

	segment := otlpSegmentFile(t, filepath.Join(workspace.StateDir(cfg.Workspace), "telemetry", "otlp"))
	data, err := os.ReadFile(segment)
	require.NoError(t, err, "degraded boot must write an OTLP-JSON segment")
	require.Contains(t, string(data), "boot.degraded", "segment must carry the boot.degraded eventName")
	require.Contains(t, string(data), "sandbox unavailable", "segment must carry the boot failure reason")
	// The diagnostic payload survives attribute projection (FR-16): the
	// degraded/readiness flags ride the record as projected metadata keys.
	for _, key := range []string{"degraded", "model_ready", "sandbox_ready"} {
		require.Contains(t, string(data), `"key":"`+key+`"`, "segment must carry the "+key+" attribute")
	}
}

func TestNewDegradedRuntime_NonNilWorkspace(t *testing.T) {
	cfg := Config{Workspace: t.TempDir()}
	rt := newDegradedRuntime(context.Background(), cfg, &testDegradedErr{s: "test"})
	require.NotNil(t, rt)
	require.NotNil(t, rt.AgentWorkspace())
	require.True(t, rt.AgentWorkspace().Readiness.Degraded)
	require.NoError(t, rt.Close(context.Background()))
}

func otlpSegmentFile(t *testing.T, dir string) string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	require.NoError(t, err, "read OTLP segment directory")
	for _, entry := range entries {
		if !entry.IsDir() && filepath.Ext(entry.Name()) == ".jsonl" {
			return filepath.Join(dir, entry.Name())
		}
	}
	t.Fatalf("no OTLP segment found in %s", dir)
	return ""
}

type testDegradedErr struct {
	s string
}

func (e *testDegradedErr) Error() string { return e.s }
