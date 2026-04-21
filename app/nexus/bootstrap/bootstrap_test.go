package bootstrap

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	nexuscfg "codeburg.org/lexbit/relurpify/app/nexus/config"
	frameworkconfig "codeburg.org/lexbit/relurpify/framework/config"
	"codeburg.org/lexbit/relurpify/framework/core"
	"github.com/stretchr/testify/require"
)

func TestResolveConfigUsesDefaultWorkspacePath(t *testing.T) {
	workspace := t.TempDir()
	paths := frameworkconfig.New(workspace)
	if err := os.MkdirAll(paths.ConfigRoot(), 0o755); err != nil {
		t.Fatal(err)
	}
	wantContents := []byte("gateway:\n  bind: 127.0.0.1:9123\nnodes:\n  auto_approve_local: true\n")
	if err := os.WriteFile(paths.NexusConfigFile(), wantContents, 0o644); err != nil {
		t.Fatal(err)
	}

	gotPaths, cfg, err := ResolveConfig(workspace, "")
	require.NoError(t, err)
	require.Equal(t, paths.ConfigRoot(), gotPaths.ConfigRoot())
	require.Equal(t, "127.0.0.1:9123", cfg.Gateway.Bind)
	require.True(t, cfg.Nodes.AutoApproveLocal)
}

func TestResolveConfigUsesExplicitConfigPath(t *testing.T) {
	workspace := t.TempDir()
	paths := frameworkconfig.New(workspace)
	if err := os.MkdirAll(paths.ConfigRoot(), 0o755); err != nil {
		t.Fatal(err)
	}
	explicit := filepath.Join(workspace, "custom-nexus.yaml")
	if err := os.WriteFile(explicit, []byte("gateway:\n  bind: localhost:9191\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	gotPaths, cfg, err := ResolveConfig(workspace, explicit)
	require.NoError(t, err)
	require.Equal(t, paths.ConfigRoot(), gotPaths.ConfigRoot())
	require.Equal(t, "localhost:9191", cfg.Gateway.Bind)
}

func TestOpenNodeManagerWiresPairingConfigAndStores(t *testing.T) {
	workspace := t.TempDir()
	paths := frameworkconfig.New(workspace)
	require.NoError(t, os.MkdirAll(paths.ConfigRoot(), 0o755))
	cfg := nexuscfg.Config{
		Nodes: nexuscfg.NodesConfig{
			AutoApproveLocal: true,
			PairingCodeTTL:   2 * time.Minute,
		},
	}

	manager, store, logStore, err := OpenNodeManager(paths, cfg)
	require.NoError(t, err)
	require.NotNil(t, manager)
	require.NotNil(t, store)
	require.NotNil(t, logStore)
	t.Cleanup(func() {
		require.NoError(t, store.Close())
		require.NoError(t, logStore.Close())
	})

	require.True(t, manager.Pairing.AutoApproveLocal)
	require.Equal(t, 2*time.Minute, manager.Pairing.PairingCodeTTL)

	now := time.Now().UTC()
	node := core.NodeDescriptor{
		ID:         "node-1",
		Name:       "node-1",
		Platform:   core.NodePlatformLinux,
		TrustClass: core.TrustClassWorkspaceTrusted,
		PairedAt:   now,
	}
	require.NoError(t, store.UpsertNode(context.Background(), node))
	gotNode, err := store.GetNode(context.Background(), "node-1")
	require.NoError(t, err)
	require.NotNil(t, gotNode)
	require.Equal(t, node.ID, gotNode.ID)
	require.Equal(t, node.Platform, gotNode.Platform)

	seqs, err := logStore.Append(context.Background(), "", []core.FrameworkEvent{{Type: "bootstrap.test"}})
	require.NoError(t, err)
	require.Len(t, seqs, 1)
}

func TestDefaultNodeDescriptorUsesTrustworthyHeadlessDefaults(t *testing.T) {
	before := time.Now().UTC().Add(-1 * time.Second)
	desc := DefaultNodeDescriptor("device-123")
	after := time.Now().UTC().Add(1 * time.Second)

	require.Equal(t, "device-123", desc.ID)
	require.Equal(t, "device-123", desc.Name)
	require.Equal(t, core.NodePlatformHeadless, desc.Platform)
	require.Equal(t, core.TrustClassWorkspaceTrusted, desc.TrustClass)
	require.False(t, desc.PairedAt.Before(before))
	require.False(t, desc.PairedAt.After(after))
}
