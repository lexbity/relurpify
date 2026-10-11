package envcomposition

import (
	"context"
	"os"
	"testing"

	"github.com/stretchr/testify/require"

	"codeburg.org/lexbit/relurpify/capability/ports"
	fsandbox "codeburg.org/lexbit/relurpify/capability/sandbox"
)

// noopCommandRunner satisfies CommandRunner without executing anything: the
// probe only resolves capability descriptors, never runs commands.
type noopCommandRunner struct{}

func (noopCommandRunner) Run(context.Context, ports.CommandRequest) (*ports.CommandResult, error) {
	return &ports.CommandResult{}, nil
}

// TestCapabilityRuntimeResolvesCanonicalToolScope pins the runtime registry
// vocabulary the recipe scoping path depends on: every tool name the shipped
// recipes declare under `may invoke` (trigger- or run-level) MUST resolve via
// GetCapability. A declared name that fails to resolve is silently dropped
// from the agent's tool scope (the live scratch-turn finding: the react agent
// ran with file_read+file_search only — no file_write, no test_run).
func TestCapabilityRuntimeResolvesCanonicalToolScope(t *testing.T) {
	workspace := os.Getenv("RELURPIFY_LIVE_WORKSPACE")
	if workspace == "" {
		t.Skip("needs RELURPIFY_LIVE_WORKSPACE (a materialized workspace with relurpify_cfg/tools)")
	}
	runner, err := fsandbox.NewAuthorizedRunner(noopCommandRunner{}, fsandbox.CommandPolicyFunc(func(context.Context, fsandbox.CommandRequest) error {
		return nil
	}))
	require.NoError(t, err)

	rt, err := BuildCapabilityRuntime(context.Background(), workspace, runner)
	require.NoError(t, err)

	ids := make([]string, 0)
	for _, desc := range rt.Registry.InspectableCapabilities() {
		ids = append(ids, desc.ID)
	}
	t.Logf("registry holds %d inspectable capabilities: %v", len(ids), ids)

	for _, name := range []string{"file_read", "grep", "test_run", "file_write"} {
		desc, ok := rt.Registry.GetCapability(name)
		t.Logf("GetCapability(%q) resolved=%v id=%q", name, ok, desc.ID)
	}
}
