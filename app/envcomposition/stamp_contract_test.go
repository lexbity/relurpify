package envcomposition

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"codeburg.org/lexbit/relurpify/capability/fs"
	regpkg "codeburg.org/lexbit/relurpify/capability/registry"
	fsandbox "codeburg.org/lexbit/relurpify/capability/sandbox"
	"codeburg.org/lexbit/relurpify/capability/toolcapabilities"
	_ "codeburg.org/lexbit/relurpify/platform/tools/native/fs" // go_native ctor registration for the file tools
	"codeburg.org/lexbit/relurpify/userconfig/config"
)

// TestStampContractManifestToolsEnforceWorkspaceScope is the F1 regression
// gate: through the real Build → Register → UseSandboxScope chain, a manifest
// file tool wrapped by the manifest-capability decorator must still receive
// the sandbox-scope stamp — reading inside the workspace admits, and a
// scope-derived deny fires for a protected path. The wrapper historically
// swallowed the stamp, so every file operation denied with
// ErrSandboxScopeUnset regardless of composition.
func TestStampContractManifestToolsEnforceWorkspaceScope(t *testing.T) {
	workspace := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(workspace, "hello.txt"), []byte("hello stamp contract"), 0o600))
	require.NoError(t, os.MkdirAll(filepath.Join(workspace, "relurpify_cfg"), 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(workspace, "relurpify_cfg", "sealed.txt"), []byte("sealed"), 0o600))

	manifests, err := config.LoadToolManifests(filepath.Join("..", "..", "relurpify_cfg", "tools", "file"))
	require.NoError(t, err)

	registry := regpkg.NewRegistry()
	// Production order: the scope is set before Build+Register
	// (app/envcomposition/capabilities.go), so registration-time stamping is
	// what must reach the concrete tool through the decorator chain.
	registry.UseSandboxScope(fsandbox.NewFileScopePolicy(workspace, nil))

	for _, tool := range toolcapabilities.Build(workspace, nil, manifests) {
		require.NoError(t, registry.Register(context.Background(), tool))
	}

	desc, ok := registry.GetCapability("file_read")
	require.True(t, ok, "file_read must resolve in the composed registry")

	child := registry.WithAllowlist([]string{desc.ID})

	inside, err := child.InvokeCapability(context.Background(), nil, desc.ID, map[string]any{"path": "hello.txt"})
	require.NoError(t, err)
	require.True(t, inside.Success, "inside-workspace read must succeed: %s", inside.Error)
	require.Equal(t, "hello stamp contract", inside.Data["content"])

	_, err = child.InvokeCapability(context.Background(), nil, desc.ID, map[string]any{
		"path": filepath.Join("relurpify_cfg", "sealed.txt"),
	})
	require.Error(t, err, "protected-path read must deny")
	require.True(t, errors.Is(err, fs.ErrFileScopeProtectedPath),
		"deny must come from the stamped sandbox scope, not the base-path resolver or a missing stamp (ErrSandboxScopeUnset): %v", err)

	outside := filepath.Join(workspace, "..", filepath.Base(workspace)+"-outside.txt")
	require.NoError(t, os.WriteFile(outside, []byte("outside"), 0o600))
	_, err = child.InvokeCapability(context.Background(), nil, desc.ID, map[string]any{"path": outside})
	require.Error(t, err, "outside-workspace read must deny")
}
