package registry

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"codeburg.org/lexbit/relurpify/capability/ports"
	fwtelemetry "codeburg.org/lexbit/relurpify/telemetry"
	"codeburg.org/lexbit/relurpify/userconfig/tools/manifest"
)

var (
	_ = "ghp_fake123456"
	_ = "eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiJzZWNyZXQifQ.zz"
)

// fakeTool is a minimal ports.Tool whose Execute captures the raw args it
// received so tests can assert the handler saw real (unredacted) values.
type fakeTool struct {
	name     string
	params   []ports.ToolParameter
	rawArgs  map[string]any
	executed func(ctx context.Context, args map[string]any) (*ports.ToolResult, error)
}

func (f *fakeTool) Name() string        { return f.name }
func (f *fakeTool) Description() string { return "fake tool for registry tests" }
func (f *fakeTool) Category() string    { return "test" }
func (f *fakeTool) Parameters() []ports.ToolParameter {
	return f.params
}
func (f *fakeTool) IsAvailable(context.Context) bool { return true }
func (f *fakeTool) Permissions() ports.ToolPermissions {
	return ports.ToolPermissions{}
}
func (f *fakeTool) Tags() []string { return nil }

func (f *fakeTool) Execute(ctx context.Context, args map[string]any) (*ports.ToolResult, error) {
	f.rawArgs = args
	if f.executed != nil {
		return f.executed(ctx, args)
	}
	return &ports.ToolResult{Success: true}, nil
}

// fakeRevertibleTool extends fakeTool with rollback support.
type fakeRevertibleTool struct {
	*fakeTool
	rollbackCalled bool
}

func (f *fakeRevertibleTool) Rollback(context.Context, ports.RollbackToken) error {
	f.rollbackCalled = true
	return nil
}

func TestInvokeCapability_RedactsArgsInResultMetadata(t *testing.T) {
	reg := NewRegistry()
	tool := &fakeTool{name: "redact_tool"}
	require.NoError(t, reg.RegisterLegacyTool(context.Background(), tool))

	args := map[string]any{
		"repo":   "myorg/repo",
		"token":  "ghp_fake123456",
		"config": "eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiJzZWNyZXQifQ.zz",
	}
	result, err := reg.InvokeCapability(context.Background(), nil, "redact_tool", args)
	require.NoError(t, err)
	require.NotNil(t, result)

	// P-7 red-line: the invocation metadata surface must carry redacted
	// values, not the raw secret-bearing ones.
	meta, ok := result.Metadata["args"].(map[string]any)
	require.True(t, ok, "expected redacted args in result metadata")
	require.Equal(t, "[REDACTED]", meta["token"])
	require.Equal(t, "[REDACTED]", meta["config"])
	require.Equal(t, "myorg/repo", meta["repo"])

	// Raw args reached the handler — tools need real values.
	require.Equal(t, "ghp_fake123456", tool.rawArgs["token"])
	require.Equal(t, "eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiJzZWNyZXQifQ.zz", tool.rawArgs["config"])
}

func TestInvokeCapability_NonRevertibleToolStoresNoToken(t *testing.T) {
	reg := NewRegistry()
	require.NoError(t, reg.RegisterLegacyTool(context.Background(), &fakeTool{name: "plain_tool"}))

	result, err := reg.InvokeCapability(context.Background(), nil, "plain_tool", map[string]any{"x": 1})
	require.NoError(t, err)
	require.NotNil(t, result)
	require.NotContains(t, result.Metadata, "rollback_token")
	require.Equal(t, 0, reg.rollbacks.size(), "non-revertible tools must not store rollback tokens")
}

func TestInvokeCapability_RevertibleToolStoresExactlyOneToken(t *testing.T) {
	reg := NewRegistry()
	require.NoError(t, reg.RegisterLegacyTool(context.Background(), &fakeRevertibleTool{fakeTool: &fakeTool{name: "revert_tool"}}))

	result, err := reg.InvokeCapability(context.Background(), nil, "revert_tool", map[string]any{"path": "/tmp/x"})
	require.NoError(t, err)
	token, ok := result.Metadata["rollback_token"].(string)
	require.True(t, ok)
	require.NotEmpty(t, token)
	require.Equal(t, 1, reg.rollbacks.size())

	require.NoError(t, reg.RollbackCapability(context.Background(), token))
	require.Equal(t, 0, reg.rollbacks.size())
}

func TestInvokeCapability_RollbackRingEvictsOldest(t *testing.T) {
	reg := NewRegistry()
	require.NoError(t, reg.RegisterLegacyTool(context.Background(), &fakeRevertibleTool{fakeTool: &fakeTool{name: "evict_tool"}}))

	var firstToken string
	for i := 0; i < rollbackRingCapacity+1; i++ {
		result, err := reg.InvokeCapability(context.Background(), nil, "evict_tool", map[string]any{"n": i})
		require.NoError(t, err)
		token := result.Metadata["rollback_token"].(string)
		if i == 0 {
			firstToken = token
		}
	}
	require.Equal(t, rollbackRingCapacity, reg.rollbacks.size())

	// The 65th insertion evicted the 1st token; rolling it back reports
	// not-found instead of silently working.
	err := reg.RollbackCapability(context.Background(), firstToken)
	require.Error(t, err)
	require.Contains(t, err.Error(), "not found")
}

func TestInvokeCapability_TokenExpiryReportsWindowErrorAndScrubs(t *testing.T) {
	reg := NewRegistry()
	require.NoError(t, reg.RegisterLegacyTool(context.Background(), &fakeRevertibleTool{fakeTool: &fakeTool{name: "ttl_tool"}}))

	now := time.Now()
	reg.rollbacks.now = func() time.Time { return now }

	result, err := reg.InvokeCapability(context.Background(), nil, "ttl_tool", map[string]any{"secret": "sk-abc123"})
	require.NoError(t, err)
	token := result.Metadata["rollback_token"].(string)
	require.Equal(t, 1, reg.rollbacks.size())

	// Advance past the TTL and attempt the rollback: it must fail loudly and
	// the raw args backing the token are gone.
	now = now.Add(rollbackTokenTTL + time.Minute)
	err = reg.RollbackCapability(context.Background(), token)
	require.Error(t, err)
	require.Equal(t, "rollback window expired — re-run the tool", err.Error())
	require.Equal(t, 0, reg.rollbacks.size(), "expired token must be removed from the ring")
}

func TestInvokeCapability_RollbackEventsEmitted(t *testing.T) {
	sink := &captureTelemetry{}
	reg := NewRegistry()
	reg.UseTelemetry(sink)
	require.NoError(t, reg.RegisterLegacyTool(context.Background(), &fakeRevertibleTool{fakeTool: &fakeTool{name: "event_tool"}}))

	result, err := reg.InvokeCapability(context.Background(), nil, "event_tool", map[string]any{"path": "/tmp/x"})
	require.NoError(t, err)
	token := result.Metadata["rollback_token"].(string)

	var stored *fwtelemetry.Event
	for i := range sink.events {
		ev := sink.events[i]
		if ev.Type == fwtelemetry.EventRollbackTokenStored {
			stored = &ev
		}
	}
	require.NotNil(t, stored, "rollback.token_stored must be emitted")
	require.Equal(t, "event_tool", stored.Metadata["tool"])
	require.Equal(t, token, stored.Metadata["token_id"])

	// Expire the token via clock injection and roll back to exercise the
	// expiry event path.
	now := time.Now()
	reg.rollbacks.now = func() time.Time { return now }
	now = now.Add(rollbackTokenTTL + time.Minute)
	_ = reg.RollbackCapability(context.Background(), token)

	var expired *fwtelemetry.Event
	for i := range sink.events {
		ev := sink.events[i]
		if ev.Type == fwtelemetry.EventRollbackTokenExpired {
			expired = &ev
		}
	}
	require.NotNil(t, expired, "rollback.token_expired must be emitted")
	require.Equal(t, "event_tool", expired.Metadata["tool"])
}

func TestRollbackRing_EvictionDropsOldest(t *testing.T) {
	ring := newRollbackRing()
	var firstID string
	for i := 0; i < rollbackRingCapacity+1; i++ {
		id := ring.store(ports.RollbackToken{InvocationID: fmt.Sprintf("t%d", i), Args: map[string]any{"i": i}})
		if i == 0 {
			firstID = id
		}
	}
	require.Equal(t, rollbackRingCapacity, ring.size())
	_, err := ring.take(firstID)
	require.ErrorIs(t, err, errRollbackTokenNotFound)
	// Survivors remain resolvable.
	_, err = ring.take("t1")
	require.NoError(t, err)
}

func TestRollbackRing_ExpirySweepScrubsArgs(t *testing.T) {
	now := time.Unix(1_000_000, 0)
	ring := newRollbackRing()
	ring.now = func() time.Time { return now }

	ring.store(ports.RollbackToken{InvocationID: "one", Args: map[string]any{"k": "v"}, Result: &ports.ToolResult{Success: true}})
	require.Equal(t, 1, ring.size())
	swept := ring.entries[0]
	require.NotNil(t, swept.token.Args, "live entry must retain args")

	// Advance beyond the TTL; the next store lazily sweeps the expired entry
	// and scrubs its secret-bearing references.
	now = now.Add(rollbackTokenTTL + time.Minute)
	ring.store(ports.RollbackToken{InvocationID: "two", Args: map[string]any{"k2": "v2"}})

	require.Equal(t, 1, ring.size())
	require.Nil(t, swept.token.Args, "expired entry Args must be scrubbed on sweep")
	require.Nil(t, swept.token.Result, "expired entry Result must be scrubbed on sweep")
	_, err := ring.take("one")
	require.ErrorIs(t, err, errRollbackTokenNotFound)
}

func TestRollbackRing_TakeExpiredReturnsWindowError(t *testing.T) {
	now := time.Unix(2_000_000, 0)
	ring := newRollbackRing()
	ring.now = func() time.Time { return now }

	ring.store(ports.RollbackToken{InvocationID: "rb-1", ToolName: "ttl_tool", Args: map[string]any{"a": 1}})
	now = now.Add(rollbackTokenTTL + time.Minute)

	_, err := ring.take("rb-1")
	var expired *rollbackExpiredError
	require.ErrorAs(t, err, &expired)
	require.Equal(t, "ttl_tool", expired.tool)
	require.Equal(t, "rollback window expired — re-run the tool", err.Error())
	require.Equal(t, 0, ring.size())
}

// TestStoreRollbackToken_DeclaredShapeInArgsRedaction exercises the manifest
// parameter declaration path of ports.RedactArgs through the registry.
func TestStoreRollbackToken_DeclaredSensitiveParamRedacted(t *testing.T) {
	reg := NewRegistry()
	tool := &fakeRevertibleTool{fakeTool: &fakeTool{
		name: "declared_sensitive",
		params: []ports.ToolParameter{
			{Name: "connection_token", Type: manifest.ToolParamString},
		},
	}}
	require.NoError(t, reg.RegisterLegacyTool(context.Background(), tool))

	result, err := reg.InvokeCapability(context.Background(), nil, "declared_sensitive", map[string]any{
		"connection_token": "abc123def",
	})
	require.NoError(t, err)
	meta := result.Metadata["args"].(map[string]any)
	require.Equal(t, "[REDACTED]", meta["connection_token"])
	// The handler still saw the raw value.
	require.Equal(t, "abc123def", tool.rawArgs["connection_token"])
}
