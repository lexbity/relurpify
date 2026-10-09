package conformance

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"

	"codeburg.org/lexbit/relurpify/capability/ports"
	"codeburg.org/lexbit/relurpify/capability/registry"
)

var (
	_ = "ghp_conformance123"
	_ = "sk-proj-conformance"
)

// leakProofTool implements the worst case for the redaction choke point: it
// echoes its raw (secret-bearing) args back into the result metadata under the
// canonical "args" key. The registry's invocation path must overwrite that
// key with the redacted surface before the result leaves the registry
// (SBH-1 D-9, INV-7). The grant-semantics guarantee is shared with the
// registry's own revertible/rollback tests; this row adds the redaction
// guarantee on top through a real registry invocation.
type leakProofTool struct{}

func (leakProofTool) Name() string        { return "no_leak_tool" }
func (leakProofTool) Description() string { return "echoes raw args into metadata" }
func (leakProofTool) Category() string    { return "test" }
func (leakProofTool) Parameters() []ports.ToolParameter {
	return nil
}
func (leakProofTool) IsAvailable(context.Context) bool { return true }
func (leakProofTool) Permissions() ports.ToolPermissions {
	return ports.ToolPermissions{}
}
func (leakProofTool) Tags() []string { return nil }

func (leakProofTool) Execute(_ context.Context, args map[string]any) (*ports.ToolResult, error) {
	// Deliberately leak back the raw args to prove the registry choke point —
	// not the tool's cooperation — enforces the no-leak guarantee.
	return &ports.ToolResult{
		Success:  true,
		Metadata: map[string]any{"args": args},
	}, nil
}

func runNoLeakMatrixRow(t *testing.T) {
	t.Helper()
	reg := registry.NewRegistry()
	require.NoError(t, reg.RegisterLegacyTool(context.Background(), leakProofTool{}))

	result, err := reg.InvokeCapability(context.Background(), nil, "no_leak_tool", map[string]any{
		"repo":   "org/repo",
		"token":  "ghp_conformance123",
		"header": "Authorization: Bearer sk-proj-conformance",
	})
	require.NoError(t, err)
	require.NotNil(t, result)

	// The invocation metadata surface is redacted.
	meta, ok := result.Metadata["args"].(map[string]any)
	require.True(t, ok, "expected redacted args in result metadata")
	require.Equal(t, "[REDACTED]", meta["token"])
	require.Equal(t, "[REDACTED]", meta["header"])
	require.Equal(t, "org/repo", meta["repo"])

	// No secret-bearing value survives anywhere in the serialized result,
	// even though the tool echoed the raw args back.
	raw, err := json.Marshal(result)
	require.NoError(t, err)
	ser := string(raw)
	require.NotContains(t, ser, "ghp_conformance123")
	require.NotContains(t, ser, "sk-proj-conformance")
	require.NotContains(t, ser, "Authorization: Bearer")
	require.Contains(t, ser, "[REDACTED]")
}
