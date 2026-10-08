package sandbox

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// fakeDockerEnv exposes the fake-docker CLI on PATH and returns the command
// log file the harness appends every invocation to.
func fakeDockerEnv(t *testing.T) string {
	t.Helper()
	fakeDir, err := filepath.Abs(filepath.Join("testdata", "fake-docker"))
	require.NoError(t, err)
	t.Setenv("PATH", fakeDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	logPath := filepath.Join(t.TempDir(), "docker.log")
	t.Setenv("FAKE_DOCKER_LOG", logPath)
	return logPath
}

func readLog(t *testing.T, logPath string) string {
	t.Helper()
	data, err := os.ReadFile(logPath)
	require.NoError(t, err)
	return string(data)
}

func containsArg(t *testing.T, log string, needle string) {
	t.Helper()
	if !strings.Contains(log, needle) {
		t.Fatalf("invocation log %q missing %q:\n%s", "cmd run", needle, log)
	}
}

func newTestRunner(t *testing.T) *SandboxCommandRunner {
	t.Helper()
	rt := NewSandboxRuntime(SandboxConfig{})
	runner, err := NewSandboxCommandRunner(&CommandRunnerConfig{Workspace: t.TempDir()}, rt)
	require.NoError(t, err)
	return runner
}

func TestSandboxCommandRunner_DetachedLifecycleArgvAndExit(t *testing.T) {
	logPath := fakeDockerEnv(t)
	runner := newTestRunner(t)
	t.Setenv("FAKE_DOCKER_WAIT_EXIT", "42")

	res, err := runner.Run(context.Background(), CommandRequest{Args: []string{"echo", "hi"}})
	require.NoError(t, err)
	require.NotNil(t, res)
	require.Equal(t, 42, res.ExitCode, "exit code must propagate from docker wait")

	log := readLog(t, logPath)
	runLine := ""
	for _, line := range strings.Split(log, "\n") {
		if strings.HasPrefix(line, "cmd run ") {
			runLine = line
			break
		}
	}
	require.NotEmpty(t, runLine, "docker run must have been invoked")
	// Detached lifecycle flags (SBH-1 D-11).
	containsArg(t, runLine, " -d ")
	containsArg(t, runLine, " -i ")
	containsArg(t, runLine, " --name relurpify-")
	// Owner labels: managed, workspace hash, pid, created.
	containsArg(t, runLine, "--label "+LabelManaged+"=true")
	containsArg(t, runLine, "--label "+LabelWorkspace+"=")
	containsArg(t, runLine, "--label "+LabelPID+"=")
	containsArg(t, runLine, "--label "+LabelCreated+"=")
	// Isolation flags are unchanged.
	containsArg(t, runLine, "--network none")
	containsArg(t, runLine, "--runtime runsc")
	// --rm auto-cleanup is replaced by managed teardown.
	if strings.Contains(runLine, "--rm") {
		t.Fatalf("detached lifecycle must not use --rm: %s", runLine)
	}
	// The guaranteed deferred teardown removed the container.
	if !strings.Contains(log, "cmd rm -f") {
		t.Fatalf("managed teardown must rm -f the container:\n%s", log)
	}
}

func TestSandboxCommandRunner_TimeoutInvokesContainerTeardown(t *testing.T) {
	logPath := fakeDockerEnv(t)
	runner := newTestRunner(t)
	t.Setenv("FAKE_DOCKER_MODE", "hang")

	res, err := runner.Run(context.Background(), CommandRequest{
		Args:    []string{"sh", "-c", "sleep 60"},
		Timeout: 150 * time.Millisecond,
	})
	require.NoError(t, err)
	require.NotNil(t, res)
	require.True(t, res.TornDown, "timeout must mark the result torn down")
	require.True(t, res.TimedOut)
	require.Equal(t, -1, res.ExitCode)

	log := readLog(t, logPath)
	// The watchdog issued stop then rm; order matters.
	stopIdx := strings.Index(log, "cmd stop ")
	rmIdx := strings.Index(log, "cmd rm -f")
	if stopIdx < 0 || rmIdx < 0 {
		t.Fatalf("timeout teardown must stop then rm -f:\n%s", log)
	}
	if stopIdx > rmIdx {
		t.Fatalf("stop must precede rm -f:\n%s", log)
	}
}

func TestSandboxCommandRunner_ContextCancelInvokesContainerTeardown(t *testing.T) {
	logPath := fakeDockerEnv(t)
	runner := newTestRunner(t)
	t.Setenv("FAKE_DOCKER_MODE", "hang")

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(100 * time.Millisecond)
		cancel()
	}()
	res, err := runner.Run(ctx, CommandRequest{Args: []string{"sleep", "60"}})
	require.NoError(t, err)
	require.True(t, res.TornDown, "context cancel must tear the container down")

	log := readLog(t, logPath)
	require.Contains(t, log, "cmd stop ")
	require.Contains(t, log, "cmd rm -f")
}

func TestSandboxCommandRunner_StdinDeliveredToAttach(t *testing.T) {
	fakeDockerEnv(t)
	runner := newTestRunner(t)
	stdinCapture := filepath.Join(t.TempDir(), "stdin.capture")
	t.Setenv("FAKE_DOCKER_MODE", "stdin")
	t.Setenv("FAKE_DOCKER_STDIN_CAPTURE", stdinCapture)

	res, err := runner.Run(context.Background(), CommandRequest{Args: []string{"cat"}, Input: "payload-from-stdin\n"})
	require.NoError(t, err)
	require.NotNil(t, res)
	require.Equal(t, 0, res.ExitCode)

	data, err := os.ReadFile(stdinCapture)
	require.NoError(t, err)
	require.Equal(t, "payload-from-stdin\n", string(data), "stdin must reach the attach stream")
}

func TestSandboxCommandRunner_AttachCrashStillYieldsExitCode(t *testing.T) {
	fakeDockerEnv(t)
	runner := newTestRunner(t)
	// attach dies immediately; wait still produces the exit code.
	t.Setenv("FAKE_DOCKER_ATTACH_EXIT", "1")
	t.Setenv("FAKE_DOCKER_WAIT_EXIT", "3")

	res, err := runner.Run(context.Background(), CommandRequest{Args: []string{"true"}})
	require.NoError(t, err)
	require.Equal(t, 3, res.ExitCode, "docker wait must dominate a crashed attach")
}

func TestSandboxCommandRunner_OOMExitClassified(t *testing.T) {
	fakeDockerEnv(t)
	runner := newTestRunner(t)
	t.Setenv("FAKE_DOCKER_WAIT_EXIT", "137")

	res, err := runner.Run(context.Background(), CommandRequest{Args: []string{"crasher"}})
	require.NoError(t, err)
	require.Equal(t, 137, res.ExitCode)
	require.True(t, res.OOMKilled, "exit 137 from the container must classify as OOM")
}

func TestContainerNameDeterministicAndLabeled(t *testing.T) {
	ws := "/tmp/ws-one"
	name := newContainerName(ws)
	require.True(t, strings.HasPrefix(name, "relurpify-"), "name = %q", name)
	require.Equal(t, "relurpify-"+workspaceHash8(ws)+"-", name[:len("relurpify-"+workspaceHash8(ws)+"-")])

	labels := containerLabels(ws)
	require.Equal(t, "true", labels[LabelManaged])
	require.Equal(t, workspaceHash8(ws), labels[LabelWorkspace])
	pid, err := strconv.Atoi(labels[LabelPID])
	require.NoError(t, err)
	require.Equal(t, os.Getpid(), pid)
	require.NotEmpty(t, labels[LabelCreated])
}
