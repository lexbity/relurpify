package security

import (
	"context"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"codeburg.org/lexbit/relurpify/capability/sandbox"
)

// TestSandboxLifecycle_OwnedRunThenBootReap drives the full lifecycle with the
// hermetic fake-docker CLI: a Run creates an owner-labeled container; the same
// container, if its owner crashed, is reaped by the next boot's orphan sweep;
// if its owner is still alive it is kept (R-7 non-interference).
func TestSandboxLifecycle_OwnedRunThenBootReap(t *testing.T) {
	// Go tests run with CWD = the package directory (testsuite/framework/
	// security); the fake docker lives three levels up.
	fakeDir, err := filepath.Abs(filepath.Join("..", "..", "..", "capability", "sandbox", "testdata", "fake-docker"))
	require.NoError(t, err)
	t.Setenv("PATH", fakeDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	logPath := filepath.Join(t.TempDir(), "docker.log")
	t.Setenv("FAKE_DOCKER_LOG", logPath)
	t.Setenv("FAKE_DOCKER_MODE", "echo")
	t.Setenv("FAKE_DOCKER_WAIT_EXIT", "0")

	// 1. Run a command through the real runner (fake docker on PATH).
	rt := sandbox.NewSandboxRuntime(sandbox.SandboxConfig{})
	runner, err := sandbox.NewSandboxCommandRunner(&sandbox.CommandRunnerConfig{Workspace: t.TempDir()}, rt)
	require.NoError(t, err)
	res, err := runner.Run(context.Background(), sandbox.CommandRequest{Args: []string{"echo", "hi"}})
	require.NoError(t, err)
	require.Equal(t, 0, res.ExitCode)

	// 2. Extract the container name and owner pid the run assigned.
	logData, err := os.ReadFile(logPath)
	require.NoError(t, err)
	logLine := firstRunLine(t, string(logData))
	name := extractFlag(t, logLine, "--name")
	require.True(t, strings.HasPrefix(name, "relurpify-"), "container name = %q", name)
	pidLabel := extractLabel(t, logLine, sandbox.LabelPID)
	require.Equal(t, strconv.Itoa(os.Getpid()), pidLabel, "the run must tag itself as the owner")

	// 3a. Same container, owner pid alive → kept by the boot sweep.
	now := time.Now()
	liveContent := psRow(name, now.Add(-2*time.Minute), os.Getpid())
	report := runReap(t, liveContent, now)
	require.Equal(t, 1, report.Scanned)
	require.Equal(t, 0, report.Reaped, "a container owned by a live supervisor must survive the sweep")

	// 3b. Same container, owner gone (dead pid) + old → reaped by next boot.
	staleContent := psRow(name, now.Add(-25*time.Minute), 2147483647)
	report = runReap(t, staleContent, now)
	require.Equal(t, 1, report.Reaped, "an orphaned container owned by a dead pid must be reaped")
	require.Equal(t, []string{name}, report.ReapedNames)
}

func runReap(t *testing.T, content string, now time.Time) sandbox.ReapReport {
	t.Helper()
	t.Setenv("FAKE_DOCKER_PS_CONTENT", content)
	report, err := sandbox.ReapOrphans(context.Background(), sandbox.ReapOptions{Now: func() time.Time { return now }})
	require.NoError(t, err)
	return report
}

func psRow(name string, created time.Time, ownerPID int) string {
	labels := "relurpify.managed=true"
	labels += ",relurpify.workspace=ab12cd34"
	labels += ",relurpify.pid=" + strconv.Itoa(ownerPID)
	labels += ",relurpify.created=" + strconv.FormatInt(created.Unix(), 10)
	return name + "\t" + labels
}

func firstRunLine(t *testing.T, log string) string {
	t.Helper()
	for _, line := range strings.Split(log, "\n") {
		if strings.HasPrefix(line, "cmd run ") {
			return line
		}
	}
	t.Fatal("no docker run invocation recorded")
	return ""
}

var flagRe = regexp.MustCompile(`\s(--[a-z][a-z-]*)\s+(\S+)`)

func extractFlag(t *testing.T, line, flag string) string {
	t.Helper()
	for _, m := range flagRe.FindAllStringSubmatch(line, -1) {
		if m[1] == flag {
			return m[2]
		}
	}
	t.Fatalf("arg line %q missing %s", line, flag)
	return ""
}

var labelRe = regexp.MustCompile(`--label relurpify\.([a-z]+)=([^\s]+)`)

func extractLabel(t *testing.T, line, label string) string {
	t.Helper()
	for _, m := range labelRe.FindAllStringSubmatch(line, -1) {
		if m[1] == strings.TrimPrefix(label, "relurpify.") {
			return m[2]
		}
	}
	t.Fatalf("arg line %q missing label %s", line, label)
	return ""
}
