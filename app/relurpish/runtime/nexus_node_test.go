package runtime

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"codeburg.org/lexbit/relurpify/framework/core"
	fsandbox "codeburg.org/lexbit/relurpify/framework/sandbox"
	"github.com/stretchr/testify/require"
)

type captureRegistrar struct {
	ids []string
}

func (r *captureRegistrar) RegisterCapability(desc core.CapabilityDescriptor) error {
	r.ids = append(r.ids, desc.ID)
	if desc.Source.ProviderID == "" {
		return fmt.Errorf("provider id missing")
	}
	if desc.RuntimeFamily != core.CapabilityRuntimeFamilyProvider {
		return fmt.Errorf("runtime family = %s", desc.RuntimeFamily)
	}
	return nil
}

func TestLoadWorkspaceConfigParsesNodeRegistration(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	require.NoError(t, os.WriteFile(path, []byte(`
nexus:
  address: ws://127.0.0.1:18789/gateway
node_registration:
  enabled: true
  node_id: relurpish-dev
  name: Dev Laptop
  local_only: true
`), 0o644))

	cfg, err := LoadWorkspaceConfig(path)
	require.NoError(t, err)
	require.True(t, cfg.NodeRegistration.Enabled)
	require.Equal(t, "relurpish-dev", cfg.NodeRegistration.NodeID)
	require.Equal(t, "ws://127.0.0.1:18789/gateway", cfg.Nexus.Address)
	require.True(t, cfg.NodeRegistration.LocalOnly)
}

func TestLocalNexusNodeProviderRegistersProviderScopedCapabilities(t *testing.T) {
	dir := t.TempDir()
	runner := fsandbox.NewLocalCommandRunner(dir, nil)
	capabilities, err := BuildBuiltinCapabilityBundle(dir, runner)
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, capabilities.Close())
	})
	registry := capabilities.Registry

	provider, err := NewLocalNexusNodeProvider(registry, NodeRegistrationConfig{
		Enabled: true,
		NodeID:  "relurpish-dev",
		Name:    "Dev Laptop",
	})
	require.NoError(t, err)

	registrar := &captureRegistrar{}
	require.NoError(t, provider.RegisterCapabilities(context.Background(), registrar))
	require.NotEmpty(t, registrar.ids)
	require.Equal(t, "relurpish-dev", provider.NodeDescriptor().ID)
}
