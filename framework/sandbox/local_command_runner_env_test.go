package sandbox

import (
	"context"
	"strings"
	"testing"
)

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
