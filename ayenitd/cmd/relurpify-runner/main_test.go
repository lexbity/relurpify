package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"codeburg.org/lexbit/relurpify/context/jobsstore"
	"codeburg.org/lexbit/relurpify/telemetry"
)

// runForTest invokes run with a context that cancels itself after d so the
// runner performs a real start/drain cycle against a temp state dir.
func runForTest(t *testing.T, d time.Duration, args ...string) (int, string) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(d)
		cancel()
	}()
	var stderr strings.Builder
	code := run(ctx, args, &stderr)
	return code, stderr.String()
}

func TestRunRejectsMissingWorkspaceAndStateDir(t *testing.T) {
	code, stderr := runForTest(t, time.Second)
	assert.Equal(t, 2, code)
	assert.Contains(t, stderr, "--workspace and --state-dir are required")

	code, stderr = runForTest(t, time.Second, "--workspace", t.TempDir())
	assert.Equal(t, 2, code)
	assert.Contains(t, stderr, "--workspace and --state-dir are required")
}

func TestRunRejectsUnknownFlag(t *testing.T) {
	code, stderr := runForTest(t, time.Second, "--bogus", "1", "--workspace", "w", "--state-dir", "s")
	assert.Equal(t, 2, code)
	assert.Contains(t, stderr, "flag provided but not defined")
}

func TestRunRejectsUncreatableLogDir(t *testing.T) {
	stateDir := t.TempDir()
	// A file where the logs directory must be created aborts the boot.
	require.NoError(t, os.WriteFile(filepath.Join(stateDir, "logs"), []byte("occupied"), 0o600))

	code, stderr := runForTest(t, time.Second, "--workspace", t.TempDir(), "--state-dir", stateDir)
	assert.Equal(t, 1, code)
	assert.Contains(t, stderr, "create log dir")
}

func TestRunRejectsUncreatableIndexDir(t *testing.T) {
	stateDir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(stateDir, "ast"), []byte("occupied"), 0o600))

	code, stderr := runForTest(t, time.Second, "--workspace", t.TempDir(), "--state-dir", stateDir)
	assert.Equal(t, 1, code)
	assert.Contains(t, stderr, "knowledge deps")
}

func TestRunExitsNonzeroWhenJobsStoreLockHeld(t *testing.T) {
	workspace := t.TempDir()
	stateDir := t.TempDir()

	// Hold the durable jobs store lock: the runner must attach-not-fight (FR-17).
	store, err := jobsstore.Open(jobsstore.Options{Dir: filepath.Join(stateDir, "jobs", "store")})
	require.NoError(t, err)
	t.Cleanup(func() {
		if closer, ok := store.(interface{ Close() error }); ok {
			_ = closer.Close()
		}
	})

	code, stderr := runForTest(t, time.Second, "--workspace", workspace, "--state-dir", stateDir)
	assert.Equal(t, 1, code)
	assert.Contains(t, stderr, "open jobs store")
}

func TestRunDrainsCleanlyAndWritesTelemetry(t *testing.T) {
	workspace := t.TempDir()
	stateDir := t.TempDir()

	code, stderr := runForTest(t, 400*time.Millisecond,
		"--workspace", workspace,
		"--state-dir", stateDir,
		"--queues", "knowledge,maintenance",
		"--workers", "2",
	)
	require.Equal(t, 0, code, "stderr: %s", stderr)

	// The telemetry leg recorded the lifecycle as JSONL.
	data, err := os.ReadFile(filepath.Join(stateDir, "logs", "runner.jsonl"))
	require.NoError(t, err)
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	require.NotEmpty(t, lines)

	var types []string
	for _, line := range lines {
		var ev struct {
			Type string `json:"type"`
		}
		require.NoError(t, json.Unmarshal([]byte(line), &ev), "line %q is not JSON", line)
		types = append(types, ev.Type)
	}
	assert.Contains(t, types, "runner.started")
	assert.Contains(t, types, "runner.stopped")

	// The knowledge deps engine materialized under the state dir and was
	// closed again by the deferred cleanup (no stale lock left behind).
	_, err = os.Stat(filepath.Join(stateDir, "ast"))
	assert.NoError(t, err)
}

func TestFileTelemetryMarshalFailureIsDropped(t *testing.T) {
	// Events that cannot marshal (channels are not JSON-encodable) are
	// dropped instead of taking the runner down.
	tel, err := newFileTelemetry(filepath.Join(t.TempDir(), "runner.jsonl"))
	require.NoError(t, err)
	assert.NotPanics(t, func() {
		tel.Emit(telemetry.Event{
			Type:     "unmarshalable",
			Metadata: map[string]any{"ch": make(chan int)},
		})
	})
}

func TestNewFileTelemetryFailsWithoutParentDir(t *testing.T) {
	// The parent directory does not exist, so opening the log must fail.
	_, err := newFileTelemetry(filepath.Join(t.TempDir(), "missing", "runner.jsonl"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "open telemetry log")
}
