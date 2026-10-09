package sandbox

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestContainerHandleTeardownIdempotent(t *testing.T) {
	logPath := fakeDockerEnv(t)
	h := NewContainerHandle("relurpify-test-00001", map[string]string{LabelManaged: "true"}, "docker")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	h.Teardown(ctx, 2*time.Second)
	h.Teardown(ctx, 2*time.Second) // second call no-ops

	log := readLog(t, logPath)
	require.Equal(t, 1, strings.Count(log, "cmd stop "), "teardown must run stop exactly once:\n%s", log)
	require.Equal(t, 1, strings.Count(log, "cmd rm -f"), "teardown must run rm -f exactly once:\n%s", log)
}

func TestContainerHandleStopFailureStillRemoves(t *testing.T) {
	logPath := fakeDockerEnv(t)
	t.Setenv("FAKE_DOCKER_STOP_FAIL", "1")
	h := NewContainerHandle("relurpify-test-stopfail", nil, "docker")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	h.Teardown(ctx, 1*time.Second)

	log := readLog(t, logPath)
	require.Contains(t, log, "cmd stop ")
	if !strings.Contains(log, "cmd rm -f") {
		t.Fatalf("rm -f must run even when stop fails:\n%s", log)
	}
}

func TestContainerHandleNilHandle(t *testing.T) {
	var h *ContainerHandle
	h.Teardown(context.Background(), time.Second) // must not panic
}

func TestContainerHandleEmptyName(t *testing.T) {
	h := NewContainerHandle("", nil, "docker")
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	h.Teardown(ctx, time.Second) // must not panic
}

func TestContainerHandleEmptyBinaryDefaultsToDocker(t *testing.T) {
	logPath := fakeDockerEnv(t)
	h := NewContainerHandle("relurpify-test-default-bin", nil, "")
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	h.Teardown(ctx, time.Second)

	log := readLog(t, logPath)
	require.Contains(t, log, "cmd stop ")
	require.Contains(t, log, "cmd rm -f")
}
