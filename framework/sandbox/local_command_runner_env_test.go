package sandbox

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestLocalCommandRunnerDoesNotInheritHostEnvironmentWithoutAllowlist(t *testing.T) {
	t.Setenv("VISIBLE_FROM_HOST", "host-value")
	t.Setenv("RELURPIFY_LLM_API_KEY", "llm-secret")

	runner := NewLocalCommandRunner(t.TempDir(), nil, nil)

	stdout, stderr, err := runner.Run(context.Background(), CommandRequest{
		Args: []string{
			"sh",
			"-c",
			`printf '%s|%s' "$VISIBLE_FROM_HOST" "$RELURPIFY_LLM_API_KEY"`,
		},
	})
	if err != nil {
		t.Fatalf("run failed: %v (stderr: %s)", err, stderr)
	}

	if got := strings.TrimSpace(stdout); got != "|" {
		t.Fatalf("expected unlisted host vars to be absent, got %q", got)
	}
}

func TestLocalCommandRunnerFiltersHostEnvironment(t *testing.T) {
	t.Setenv("ALLOW_ME", "host-visible")
	t.Setenv("RELURPIFY_LLM_API_KEY", "llm-secret")
	t.Setenv("RELURPIFY_NEXUS_TOKEN", "nexus-secret")

	runner := NewLocalCommandRunner(t.TempDir(), []string{"ALLOW_ME"}, nil)

	stdout, stderr, err := runner.Run(context.Background(), CommandRequest{
		Args: []string{
			"sh",
			"-c",
			`printf '%s|%s|%s' "$ALLOW_ME" "$RELURPIFY_LLM_API_KEY" "$RELURPIFY_NEXUS_TOKEN"`,
		},
		Env: []string{"ALLOW_ME=tool-visible"},
	})
	if err != nil {
		t.Fatalf("run failed: %v (stderr: %s)", err, stderr)
	}

	if got := strings.TrimSpace(stdout); got != "tool-visible||" {
		t.Fatalf("unexpected child env output: %q", got)
	}
}

func TestLocalRunnerTruncatesLargeOutput(t *testing.T) {
	// Produce hundreds of KB of output via a loop, with a 1KB limit.
	// The runner's cappedBuffer must only capture up to 1KB. The child
	// process may be killed by SIGPIPE once we close the read end;
	// a non-zero exit is acceptable and expected.
	stdout, stderr, err := NewLocalCommandRunner(t.TempDir(), nil, nil).Run(
		context.Background(),
		CommandRequest{
			Args:           []string{"sh", "-c", "i=0; while [ $i -lt 10000 ]; do printf 'abcdefghij'; i=$((i+1)); done"},
			Timeout:        5 * time.Second,
			MaxOutputBytes: 1024,
		},
	)
	// err may be non-nil (SIGPIPE 141) when the process is killed after
	// we've read enough output. The important thing is that stdout was
	// captured and truncated.
	if err != nil {
		t.Logf("run returned error (acceptable): %v (stderr: %s)", err, stderr)
	}
	if len(stdout) > 1024 {
		t.Fatalf("expected at most 1024 bytes of stdout, got %d", len(stdout))
	}
	if len(stdout) < 1024 {
		t.Logf("stdout was %d bytes (expected ~1024)", len(stdout))
	}
}

func TestLocalCommandRunnerDoesNotInheritUnlistedHostVars(t *testing.T) {
	t.Setenv("VISIBLE_FROM_HOST", "host-value")
	t.Setenv("RELURPIFY_LLM_API_KEY", "llm-secret")

	runner := NewLocalCommandRunner(t.TempDir(), []string{"PATH"}, nil)

	stdout, stderr, err := runner.Run(context.Background(), CommandRequest{
		Args: []string{
			"sh",
			"-c",
			`printf '%s|%s' "$VISIBLE_FROM_HOST" "$RELURPIFY_LLM_API_KEY"`,
		},
	})
	if err != nil {
		t.Fatalf("run failed: %v (stderr: %s)", err, stderr)
	}

	if got := strings.TrimSpace(stdout); got != "|" {
		t.Fatalf("expected unlisted host vars to be absent, got %q", got)
	}
}
