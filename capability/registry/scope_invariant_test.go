package registry

import (
	"context"
	"fmt"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"

	"codeburg.org/lexbit/relurpify/capability/ports"
	"codeburg.org/lexbit/relurpify/governance/permissions"
)

type scopeSpyTool struct {
	name  string
	scope *permissions.FileScopePolicy
	mu    sync.Mutex
}

func newScopeSpy(name string) *scopeSpyTool {
	return &scopeSpyTool{name: name}
}

func (t *scopeSpyTool) Name() string        { return t.name }
func (t *scopeSpyTool) Description() string { return "spy tool for scope invariant testing" }
func (t *scopeSpyTool) Category() string    { return "test" }
func (t *scopeSpyTool) Parameters() []ports.ToolParameter {
	return []ports.ToolParameter{}
}
func (t *scopeSpyTool) Execute(_ context.Context, args map[string]any) (*ports.ToolResult, error) {
	t.mu.Lock()
	s := t.scope
	t.mu.Unlock()
	if s == nil {
		return &ports.ToolResult{Success: false, Error: "nil scope"}, nil
	}
	return &ports.ToolResult{Success: true, Data: map[string]any{"scoped": true}}, nil
}
func (t *scopeSpyTool) IsAvailable(_ context.Context) bool { return true }
func (t *scopeSpyTool) Permissions() ports.ToolPermissions {
	return ports.ToolPermissions{Permissions: &permissions.PermissionSet{}}
}
func (t *scopeSpyTool) Tags() []string { return []string{"test"} }

func (t *scopeSpyTool) SetSandboxScope(scope *permissions.FileScopePolicy) {
	t.mu.Lock()
	t.scope = scope
	t.mu.Unlock()
}

func (t *scopeSpyTool) getScope() *permissions.FileScopePolicy {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.scope
}

func TestNewRegistry_HasDenyAllScope(t *testing.T) {
	reg := NewRegistry()
	require.NotNil(t, reg.sandboxScope, "fresh registry must have non-nil sandbox scope")

	err := reg.sandboxScope.Check(permissions.FileSystemRead, "/tmp/test.txt")
	require.Error(t, err, "deny-all scope must deny all paths")
}

func TestRegisterLegacyTool_ReceivesDenyAllScope(t *testing.T) {
	reg := NewRegistry()
	spy := newScopeSpy("scope_spy")

	require.NotNil(t, reg.sandboxScope)
	require.Nil(t, spy.getScope(), "spy starts with nil scope")

	err := reg.RegisterLegacyTool(context.Background(), spy)
	require.NoError(t, err)

	got := spy.getScope()
	require.NotNil(t, got, "tool must receive non-nil scope after registration")

	err = got.Check(permissions.FileSystemRead, "/tmp/test.txt")
	require.Error(t, err, "registered tool's scope must be deny-all")
}

func TestAllRegisteredTools_HaveNonNilScope(t *testing.T) {
	reg := NewRegistry()

	spies := []*scopeSpyTool{
		newScopeSpy("scope_spy_a"),
		newScopeSpy("scope_spy_b"),
		newScopeSpy("scope_spy_c"),
	}
	for _, spy := range spies {
		err := reg.RegisterLegacyTool(context.Background(), spy)
		require.NoError(t, err)
	}

	for i, spy := range spies {
		got := spy.getScope()
		require.NotNil(t, got, "tool %d must have non-nil scope after registration", i)
		err := got.Check(permissions.FileSystemRead, "/tmp/test.txt")
		require.Error(t, err, "tool %d scope must deny paths", i)
	}
}

func TestUseSandboxScope_ReplacesDenyAllWithVerified(t *testing.T) {
	reg := NewRegistry()
	spy := newScopeSpy("scope_spy")

	err := reg.RegisterLegacyTool(context.Background(), spy)
	require.NoError(t, err)

	require.NotNil(t, spy.getScope())
	require.Error(t, spy.getScope().Check(permissions.FileSystemRead, "/tmp/test.txt"),
		"before UseSandboxScope, scope must be deny-all")

	tmp := t.TempDir()
	verifiedScope := permissions.NewFileScopePolicy(tmp, nil)
	reg.UseSandboxScope(verifiedScope)

	got := spy.getScope()
	require.NotNil(t, got, "scope must be non-nil after UseSandboxScope")
	require.NoError(t, got.Check(permissions.FileSystemRead, tmp+"/ok.txt"),
		"verified scope must allow reads inside workspace")
	require.Error(t, got.Check(permissions.FileSystemRead, "/tmp/outside.txt"),
		"verified scope must deny reads outside workspace")
}

func TestUseSandboxScope_ToolsRegisteredAfterAlsoReceiveVerified(t *testing.T) {
	reg := NewRegistry()

	tmp := t.TempDir()
	verifiedScope := permissions.NewFileScopePolicy(tmp, nil)
	reg.UseSandboxScope(verifiedScope)

	spy := newScopeSpy("scope_spy")
	err := reg.RegisterLegacyTool(context.Background(), spy)
	require.NoError(t, err)

	got := spy.getScope()
	require.NotNil(t, got)
	require.NoError(t, got.Check(permissions.FileSystemRead, tmp+"/ok.txt"),
		"tool registered after UseSandboxScope must receive verified scope")
}

func TestUseSandboxScope_AtomicSwapDoesNotLeaveNilWindow(t *testing.T) {
	reg := NewRegistry()
	spy := newScopeSpy("scope_spy")

	err := reg.RegisterLegacyTool(context.Background(), spy)
	require.NoError(t, err)

	beforeSwap := spy.getScope()
	require.NotNil(t, beforeSwap)

	tmp := t.TempDir()
	verifiedScope := permissions.NewFileScopePolicy(tmp, nil)
	reg.UseSandboxScope(verifiedScope)

	afterSwap := spy.getScope()
	require.NotNil(t, afterSwap)
	require.NotSame(t, beforeSwap, afterSwap, "scope must be swapped, not the same object")

	require.NoError(t, afterSwap.Check(permissions.FileSystemRead, tmp+"/ok.txt"))
}

func TestUseSandboxScope_NilArgNoop(t *testing.T) {
	reg := NewRegistry()
	spy := newScopeSpy("scope_spy")

	err := reg.RegisterLegacyTool(context.Background(), spy)
	require.NoError(t, err)

	before := spy.getScope()
	require.NotNil(t, before)

	reg.UseSandboxScope(nil)

	after := spy.getScope()
	require.Same(t, before, after, "UseSandboxScope(nil) must not mutate scope")
}

func TestCloneFiltered_PreservesSandboxScope(t *testing.T) {
	reg := NewRegistry()

	tmp := t.TempDir()
	verifiedScope := permissions.NewFileScopePolicy(tmp, nil)
	reg.UseSandboxScope(verifiedScope)

	spy := newScopeSpy("scope_spy")
	err := reg.RegisterLegacyTool(context.Background(), spy)
	require.NoError(t, err)

	clone := reg.CloneFiltered(func(_ ports.Tool) bool { return true })
	require.NotNil(t, clone.sandboxScope, "clone must preserve sandbox scope")
	require.Same(t, verifiedScope, clone.sandboxScope, "clone shares verified scope object")
}

func TestWithAllowlist_PreservesSandboxScope(t *testing.T) {
	reg := NewRegistry()

	tmp := t.TempDir()
	verifiedScope := permissions.NewFileScopePolicy(tmp, nil)
	reg.UseSandboxScope(verifiedScope)

	view := reg.WithAllowlist([]string{"scope_spy"})
	require.NotNil(t, view.sandboxScope, "allowlist view must have non-nil sandbox scope")
	require.Same(t, verifiedScope, view.sandboxScope)
}

func TestDenyAllScope_ConcurrentRegistration(t *testing.T) {
	reg := NewRegistry()

	const goroutines = 10
	errs := make(chan error, goroutines)
	for i := 0; i < goroutines; i++ {
		go func(i int) {
			spy := newScopeSpy(fmt.Sprintf("scope_spy_%d", i))
			errs <- reg.RegisterLegacyTool(context.Background(), spy)
		}(i)
	}

	for i := 0; i < goroutines; i++ {
		require.NoError(t, <-errs)
	}
}

func TestDenyAllScope_NoEscalation(t *testing.T) {
	reg := NewRegistry()

	// Verify that UseSandboxScope(nil) does not accidentally set nil scope.
	// The deny-all scope established at construction must not be escalated
	// to permissive through any code path.

	tmp := t.TempDir()
	verifiedScope := permissions.NewFileScopePolicy(tmp, nil)
	reg.UseSandboxScope(verifiedScope)
	require.Same(t, verifiedScope, reg.sandboxScope)

	reg.UseSandboxScope(nil)
	require.Same(t, verifiedScope, reg.sandboxScope,
		"UseSandboxScope(nil) must not downgrade verified scope to nil")
}

var _ ports.Tool = (*scopeSpyTool)(nil)
var _ SandboxScopeAware = (*scopeSpyTool)(nil)
