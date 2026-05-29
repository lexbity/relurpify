package runtime

import (
	"strings"
	"testing"
)

func TestFormatSandboxDetail_EmptyShowsFailClosedMessage(t *testing.T) {
	got := formatSandboxDetail("")
	if !strings.Contains(got, "FAIL TO START") {
		t.Errorf("expected FAIL TO START message for empty detail, got: %q", got)
	}
	if strings.Contains(got, "tool sandboxing disabled") {
		t.Errorf("should not contain old 'tool sandboxing disabled', got: %q", got)
	}
}

func TestFormatSandboxDetail_ErrorMessageShowsFailClosed(t *testing.T) {
	got := formatSandboxDetail("runsc not found")
	if !strings.Contains(got, "FAIL TO START") {
		t.Errorf("expected FAIL TO START message for error detail, got: %q", got)
	}
	if !strings.Contains(got, "runsc not found") {
		t.Errorf("expected original error preserved, got: %q", got)
	}
}

func TestFormatSandboxDetail_VersionStringPassesThrough(t *testing.T) {
	got := formatSandboxDetail("runsc version 1.2.3")
	if got != "runsc version 1.2.3" {
		t.Errorf("version string should pass through unchanged, got: %q", got)
	}
}

func TestFormatSandboxDetail_DockerErrorShowsFailClosed(t *testing.T) {
	got := formatSandboxDetail("error: docker daemon not running")
	if !strings.Contains(got, "FAIL TO START") {
		t.Errorf("expected FAIL TO START for docker error, got: %q", got)
	}
	if !strings.Contains(got, "docker daemon not running") {
		t.Errorf("expected original error preserved, got: %q", got)
	}
}
