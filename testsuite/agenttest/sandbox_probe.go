package agenttest

import (
	"context"
	"errors"
	"testing"

	"codeburg.org/lexbit/relurpify/framework/sandbox"
)

// RequireSandbox returns a verified sandbox CommandRunner for the given
// workspace root. If no sandbox backend is available, the test is skipped with
// a clear message. This is the standard way for e2e suites to obtain a runner
// without silently falling back to host exec.
func RequireSandbox(t *testing.T, ctx context.Context, workspaceRoot, backend string) sandbox.CommandRunner {
	t.Helper()
	runner, err := NewWorkspaceSandboxRunner(ctx, workspaceRoot, backend)
	if err != nil {
		if errors.Is(err, ErrNoSandboxBackend) {
			t.Skipf("e2e requires a verified sandbox (gvisor/docker); skipping: %v", err)
		}
		t.Fatalf("NewWorkspaceSandboxRunner failed: %v", err)
	}
	return runner
}
