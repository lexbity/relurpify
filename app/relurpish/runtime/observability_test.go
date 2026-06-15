package runtime

import (
	"bytes"
	"log"
	"testing"

	"github.com/stretchr/testify/require"

	"codeburg.org/lexbit/relurpify/userconfig/config"
)

func TestNewDegradedRuntime_LogsBootDegradedEvent(t *testing.T) {
	var buf bytes.Buffer
	prev := log.Writer()
	log.SetOutput(&buf)
	defer log.SetOutput(prev)

	cfg := Config{Workspace: "/tmp/test-degraded-boot"}
	rt := newDegradedRuntime(nil, cfg, config.Secrets{}, &testDegradedErr{s: "sandbox unavailable"})
	require.NotNil(t, rt)

	output := buf.String()
	require.Contains(t, output, "boot.degraded",
		"newDegradedRuntime must emit boot.degraded log event")
	require.Contains(t, output, "sandbox unavailable",
		"boot.degraded must include reason")
}

func TestNewDegradedRuntime_NonNilWorkspace(t *testing.T) {
	prev := log.Writer()
	log.SetOutput(log.Writer())
	defer log.SetOutput(prev)

	cfg := Config{Workspace: "/tmp/test-degraded-ws"}
	rt := newDegradedRuntime(nil, cfg, config.Secrets{}, &testDegradedErr{s: "test"})
	require.NotNil(t, rt)
	require.NotNil(t, rt.AgentWorkspace())
	require.True(t, rt.AgentWorkspace().Readiness.Degraded)
}

type testDegradedErr struct {
	s string
}

func (e *testDegradedErr) Error() string { return e.s }
