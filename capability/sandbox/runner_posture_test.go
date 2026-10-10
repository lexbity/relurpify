package sandbox

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"codeburg.org/lexbit/relurpify/governance/sandbox"
	"codeburg.org/lexbit/relurpify/telemetry"
)

func newPostureRunner(t *testing.T, cfg *CommandRunnerConfig) *SandboxCommandRunner {
	t.Helper()
	if cfg == nil {
		cfg = &CommandRunnerConfig{Workspace: t.TempDir()}
	}
	if cfg.Workspace == "" {
		cfg.Workspace = t.TempDir()
	}
	rt := NewSandboxRuntime(sandbox.SandboxConfig{})
	require.NoError(t, rt.ApplyPolicy(context.Background(), sandbox.SandboxPolicy{}))
	runner, err := NewSandboxCommandRunner(cfg, rt)
	require.NoError(t, err)
	return runner
}

// TestRunnerPosture_ManifestLimitsReachArgv is the acceptance-10 arg-vector
// red-line: every declared resource limit hits `docker run` unmodified.
func TestRunnerPosture_ManifestLimitsReachArgv(t *testing.T) {
	logPath := fakeDockerEnv(t)
	runner := newPostureRunner(t, nil)

	res, err := runner.Run(context.Background(), CommandRequest{
		Args:          []string{"worker"},
		MemoryBytes:   1 << 30,
		PidsLimit:     128,
		CPUs:          2.5,
		OutputCeiling: 1 << 20,
		GracePeriod:   2 * time.Second,
	})
	require.NoError(t, err)
	require.NotNil(t, res)

	log := readLog(t, logPath)
	runLine := ""
	for _, line := range strings.Split(log, "\n") {
		if strings.HasPrefix(line, "cmd run ") {
			runLine = line
			break
		}
	}
	require.NotEmpty(t, runLine)
	containsArg(t, runLine, "--memory 1073741824")
	containsArg(t, runLine, "--pids-limit 128")
	containsArg(t, runLine, "--cpus 2.5")
}

// TestRunnerPosture_OutputCeilingTruncatesAndSpills is the D-12 red-line: a
// ceiling-exceeded stream kills the process group, marks Truncated+TornDown,
// spills the retained prefix, and reports the event.
func TestRunnerPosture_OutputCeilingTruncatesAndSpills(t *testing.T) {
	fakeDockerEnv(t)
	sink := &commandRecordingSink{}
	spillDir := filepath.Join(t.TempDir(), "spill")
	runner := newPostureRunner(t, &CommandRunnerConfig{Workspace: t.TempDir(), SpillDir: spillDir, Events: sink})
	t.Setenv("FAKE_DOCKER_MODE", "spam")
	t.Setenv("FAKE_DOCKER_SPAM_BYTES", "2097152") // 2 MiB vs 1 MiB ceiling

	res, err := runner.Run(context.Background(), CommandRequest{
		Args:          []string{"noisy"},
		OutputCeiling: 1 << 20,
		Timeout:       10 * time.Second,
	})
	require.NoError(t, err)
	require.True(t, res.Truncated, "ceiling exceeded must report Truncated")
	require.True(t, res.TornDown, "ceiling exceeded must tear the process down")
	require.NotEmpty(t, res.StdoutRef, "spill must populate StdoutRef")
	require.Equal(t, int64(2097152), res.StdoutBytes, "StdoutBytes is the total streamed bytes")

	spilled, err := os.ReadFile(res.StdoutRef)
	require.NoError(t, err)
	require.Len(t, spilled, 1<<20, "spill must hold the retained prefix up to the ceiling")
	// `yes x` emits "x\n" pairs; the retained prefix is exactly that stream.
	require.Equal(t, string(bytesRepeatXNewline(1<<19)), string(spilled))

	ev, ok := sink.Find(telemetry.EventSandboxOutputCeilingExceeded)
	require.True(t, ok, "ceiling exceed event must be emitted")
	require.Equal(t, "noisy", ev.Metadata["command"])
	require.Equal(t, true, ev.Metadata["spilled"])
}

// TestRunnerPosture_OutputCeilingWithoutSpillDirLeavesRefsEmpty: truncation is
// still reported when no SpillDir is configured; the refs stay empty.
func TestRunnerPosture_OutputCeilingWithoutSpillDirLeavesRefsEmpty(t *testing.T) {
	fakeDockerEnv(t)
	runner := newPostureRunner(t, &CommandRunnerConfig{Workspace: t.TempDir()}) // no SpillDir
	t.Setenv("FAKE_DOCKER_MODE", "spam")
	t.Setenv("FAKE_DOCKER_SPAM_BYTES", "2097152")

	res, err := runner.Run(context.Background(), CommandRequest{Args: []string{"noisy"}, OutputCeiling: 1 << 20, Timeout: 10 * time.Second})
	require.NoError(t, err)
	require.True(t, res.Truncated)
	require.Empty(t, res.StdoutRef, "without SpillDir the ref must stay empty")
	require.Empty(t, res.StderrRef)
}

// TestRunnerPosture_ProtectedPathSymlinkEscapeDropped: a protected path whose
// symlink resolves outside the workspace must NOT be mounted, and the escape
// is surfaced as an event (D-14).
func TestRunnerPosture_ProtectedPathSymlinkEscapeDropped(t *testing.T) {
	ws := t.TempDir()
	// link -> /etc (outside the workspace)
	require.NoError(t, os.Symlink("/etc", filepath.Join(ws, "link")))
	// .git is a real protected dir inside the workspace
	require.NoError(t, os.MkdirAll(filepath.Join(ws, ".git"), 0o755))

	sink := &commandRecordingSink{}
	rt := NewSandboxRuntime(sandbox.SandboxConfig{})
	require.NoError(t, rt.ApplyPolicy(context.Background(), sandbox.SandboxPolicy{
		ProtectedPaths: []string{
			filepath.Join(ws, "link"),
			filepath.Join(ws, ".git"),
		},
	}))
	runner, err := NewSandboxCommandRunner(&CommandRunnerConfig{Workspace: ws, Events: sink}, rt)
	require.NoError(t, err)

	mounts := runner.protectedMounts(context.Background())
	var gitMount string
	for _, m := range mounts {
		if strings.Contains(m, ".git") {
			gitMount = m
		}
		if strings.Contains(m, ":link:ro") || strings.Contains(m, "link:ro") {
			t.Fatalf("escaping symlink must not be mounted: %s", m)
		}
	}
	require.NotEmpty(t, gitMount, ".git must still be mounted read-only")
	require.True(t, strings.HasSuffix(gitMount, ".git:/workspace/.git:ro"))

	ev, ok := sink.Find(telemetry.EventSandboxProtectedPathEscaped)
	require.True(t, ok, "escape must emit sandbox.protected_path_escaped")
	if resolved, _ := ev.Metadata["resolved"].(string); resolved != "/etc" {
		t.Fatalf("escape event resolved = %q, want /etc", resolved)
	}
}

// TestRunnerPosture_ProtectedMountsDedupeResolved: a symlink alias and its
// target resolve to one source and must produce exactly one read-only mount —
// duplicate source→target binds are rejected by the engine (D8).
func TestRunnerPosture_ProtectedMountsDedupeResolved(t *testing.T) {
	ws := t.TempDir()
	target := filepath.Join(ws, "secrets")
	require.NoError(t, os.MkdirAll(target, 0o750))
	alias := filepath.Join(ws, "alias")
	require.NoError(t, os.Symlink(target, alias))

	rt := NewSandboxRuntime(sandbox.SandboxConfig{})
	require.NoError(t, rt.ApplyPolicy(context.Background(), sandbox.SandboxPolicy{
		// The same resolved source appears three times: the real path twice and
		// a symlink alias once.
		ProtectedPaths: []string{target, alias, target},
	}))
	runner, err := NewSandboxCommandRunner(&CommandRunnerConfig{Workspace: ws}, rt)
	require.NoError(t, err)

	mounts := runner.protectedMounts(context.Background())
	require.Len(t, mounts, 1, "resolved aliases must dedupe to one mount: %v", mounts)
	require.Equal(t, target+":/workspace/secrets:ro", mounts[0])
}

// TestRunnerPosture_ProtectedPathSelfSkipped: a protected path that resolves to
// the workspace root cannot be enforced read-only over the rw workspace bind;
// it is skipped and reported as sandbox.protected_path_self (D8).
func TestRunnerPosture_ProtectedPathSelfSkipped(t *testing.T) {
	ws := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(ws, ".git"), 0o750))

	sink := &commandRecordingSink{}
	rt := NewSandboxRuntime(sandbox.SandboxConfig{})
	require.NoError(t, rt.ApplyPolicy(context.Background(), sandbox.SandboxPolicy{
		ProtectedPaths: []string{ws, filepath.Join(ws, ".git")},
	}))
	runner, err := NewSandboxCommandRunner(&CommandRunnerConfig{Workspace: ws, Events: sink}, rt)
	require.NoError(t, err)

	mounts := runner.protectedMounts(context.Background())
	require.Len(t, mounts, 1, "the workspace root must be skipped: %v", mounts)
	require.True(t, strings.HasSuffix(mounts[0], ".git:/workspace/.git:ro"))
	require.NotEqual(t, ws+":/workspace:ro", mounts[0],
		"the workspace root must never be mounted read-only over its own rw bind")

	ev, ok := sink.Find(telemetry.EventSandboxProtectedPathSelf)
	require.True(t, ok, "a self-resolving protected path must emit sandbox.protected_path_self")
	require.Equal(t, ws, ev.Metadata["resolved"])
}

// TestRunnerPosture_ImageDigestResolution covers D-13 precedence against the
// fake docker daemon: explicit ref, configured digest, local inspect, unpinned.
func TestRunnerPosture_ImageDigestResolution(t *testing.T) {
	ctx := context.Background()

	t.Run("explicit digest skips inspect", func(t *testing.T) {
		logPath := fakeDockerEnv(t)
		pin := ResolveImageRef(ctx, "docker", "img:tag@sha256:aaaa1111", "")
		require.Equal(t, PinExplicit, pin.Source)
		require.Equal(t, "img:tag@sha256:aaaa1111", pin.Ref)
		// The docker CLI is never invoked for an explicit digest: the harness
		// log file is never even created.
		require.NoFileExists(t, logPath)
	})

	t.Run("configured digest pins", func(t *testing.T) {
		fakeDockerEnv(t)
		pin := ResolveImageRef(ctx, "docker", "img:tag", "sha256:bbbb2222")
		require.Equal(t, PinConfigured, pin.Source)
		require.Equal(t, "img@sha256:bbbb2222", pin.Ref)
	})

	t.Run("local inspect resolves digest", func(t *testing.T) {
		fakeDockerEnv(t)
		t.Setenv("FAKE_DOCKER_INSPECT_OUTPUT", "sha256:cccc3333")
		pin := ResolveImageRef(ctx, "docker", "img:tag", "")
		require.Equal(t, PinInspected, pin.Source)
		require.Equal(t, "img@sha256:cccc3333", pin.Ref)
	})

	t.Run("inspect failure degrades to unpinned", func(t *testing.T) {
		fakeDockerEnv(t) // fake inspect prints empty → no digest
		pin := ResolveImageRef(ctx, "docker", "img:tag", "")
		require.Equal(t, PinUnpinned, pin.Source)
		require.Equal(t, "img:tag", pin.Ref)
	})
}

// TestRunnerPosture_OutputBytesReportedOnce checks the debt resolution: byte
// counts are set from the spill writers exactly (no len-then-overwrite).
func TestRunnerPosture_OutputBytesReportedOnce(t *testing.T) {
	fakeDockerEnv(t)
	runner := newPostureRunner(t, nil)
	t.Setenv("FAKE_DOCKER_ATTACH_OUTPUT", "hello world")

	res, err := runner.Run(context.Background(), CommandRequest{Args: []string{"say"}})
	require.NoError(t, err)
	require.Equal(t, int64(len("hello world")), res.StdoutBytes)
	require.Equal(t, "hello world", res.Stdout)
	require.Equal(t, res.StdoutBytes, int64(len(res.Stdout)))
}

func bytesRepeatXNewline(n int) []byte {
	out := make([]byte, 0, n*2)
	for i := 0; i < n; i++ {
		out = append(out, 'x', '\n')
	}
	return out
}
