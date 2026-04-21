package capability

import (
	"context"
	"testing"

	"codeburg.org/lexbit/relurpify/framework/core"
	"github.com/stretchr/testify/require"
)

type stubNodeProvider struct {
	desc         core.ProviderDescriptor
	nodeDesc     core.NodeDescriptor
	health       core.NodeHealth
	capabilities []core.CapabilityDescriptor
}

func (s *stubNodeProvider) Descriptor() core.ProviderDescriptor                    { return s.desc }
func (s *stubNodeProvider) Initialize(context.Context, core.ProviderRuntime) error { return nil }
func (s *stubNodeProvider) RegisterCapabilities(_ context.Context, registrar core.CapabilityRegistrar) error {
	for _, capability := range s.capabilities {
		if err := registrar.RegisterCapability(capability); err != nil {
			return err
		}
	}
	return nil
}
func (s *stubNodeProvider) ListSessions(context.Context) ([]core.ProviderSession, error) {
	return nil, nil
}
func (s *stubNodeProvider) HealthSnapshot(context.Context) (core.ProviderHealthSnapshot, error) {
	return core.ProviderHealthSnapshot{Status: "ok"}, nil
}
func (s *stubNodeProvider) Close(context.Context) error                         { return nil }
func (s *stubNodeProvider) NodeDescriptor() core.NodeDescriptor                 { return s.nodeDesc }
func (s *stubNodeProvider) NodeHealth(context.Context) (core.NodeHealth, error) { return s.health, nil }
func (s *stubNodeProvider) StreamHealth(context.Context) (<-chan core.NodeHealth, error) {
	ch := make(chan core.NodeHealth)
	close(ch)
	return ch, nil
}
func (s *stubNodeProvider) VerifyCredential(core.NodeCredential) error { return nil }

func TestRegisterNodeProvider(t *testing.T) {
	registry := NewCapabilityRegistry()
	provider := &stubNodeProvider{
		desc: core.ProviderDescriptor{
			ID:   "node-1",
			Kind: core.ProviderKindNodeDevice,
			Security: core.ProviderSecurityProfile{
				Origin: core.ProviderOriginLocal,
			},
		},
		nodeDesc: core.NodeDescriptor{
			ID:         "node-1",
			Name:       "Laptop",
			Platform:   core.NodePlatformLinux,
			TrustClass: core.TrustClassWorkspaceTrusted,
		},
		health: core.NodeHealth{Online: true, Foreground: true},
		capabilities: []core.CapabilityDescriptor{{
			ID:            "camera.capture",
			Name:          "camera.capture",
			Kind:          core.CapabilityKindTool,
			RuntimeFamily: core.CapabilityRuntimeFamilyProvider,
		}},
	}

	require.NoError(t, registry.RegisterNodeProvider(context.Background(), provider))
	require.True(t, registry.HasCapability("camera.capture"))
}
