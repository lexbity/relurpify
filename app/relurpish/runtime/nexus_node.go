package runtime

import (
	"context"
	"fmt"
	"runtime"
	"strings"
	"sync"
	"time"

	"codeburg.org/lexbit/relurpify/framework/capability"
	"codeburg.org/lexbit/relurpify/framework/core"
)

type LocalNexusNodeProvider struct {
	registry    *capability.Registry
	descriptor  core.NodeDescriptor
	provider    core.ProviderDescriptor
	credential  core.NodeCredential
	healthMu    sync.RWMutex
	health      core.NodeHealth
	healthWatch chan core.NodeHealth
	closeOnce   sync.Once
}

func NewLocalNexusNodeProvider(registry *capability.Registry, cfg NodeRegistrationConfig) (*LocalNexusNodeProvider, error) {
	if registry == nil {
		return nil, fmt.Errorf("capability registry required")
	}
	nodeID := strings.TrimSpace(cfg.NodeID)
	if nodeID == "" {
		nodeID = "relurpish-local-node"
	}
	name := strings.TrimSpace(cfg.Name)
	if name == "" {
		name = "Relurpish Local Node"
	}
	platform := cfg.Platform
	if platform == "" {
		platform = defaultNodePlatform()
	}
	descriptor := core.NodeDescriptor{
		ID:         nodeID,
		Name:       name,
		Platform:   platform,
		TrustClass: core.TrustClassWorkspaceTrusted,
		PairedAt:   time.Now().UTC().Unix(),
		Tags:       cloneTags(cfg.Tags),
	}
	cred := core.NodeCredential{
		DeviceID:  nodeID,
		PublicKey: []byte("local-only"),
		IssuedAt:  time.Now().UTC().Unix(),
	}
	provider := core.ProviderDescriptor{
		ID:                 "node:" + nodeID,
		Kind:               core.ProviderKindNodeDevice,
		ConfiguredSource:   "relurpish/local",
		ActivationScope:    "workspace",
		TrustBaseline:      core.TrustClassWorkspaceTrusted,
		RecoverabilityMode: core.RecoverabilityInProcess,
		SupportsHealth:     true,
		Security: core.ProviderSecurityProfile{
			Origin:                     core.ProviderOriginLocal,
			RequiresFrameworkMediation: true,
		},
	}
	return &LocalNexusNodeProvider{
		registry:   registry,
		descriptor: descriptor,
		provider:   provider,
		credential: cred,
		health: core.NodeHealth{
			Online:     true,
			Foreground: true,
			LastSeenAt: time.Now().UTC().Unix(),
		},
		healthWatch: make(chan core.NodeHealth, 1),
	}, nil
}

func (p *LocalNexusNodeProvider) Descriptor() core.ProviderDescriptor {
	return p.provider
}

func (p *LocalNexusNodeProvider) Initialize(context.Context, core.ProviderRuntime) error {
	return nil
}

func (p *LocalNexusNodeProvider) RegisterCapabilities(_ context.Context, registrar core.CapabilityRegistrar) error {
	if registrar == nil {
		return fmt.Errorf("capability registrar required")
	}
	for _, desc := range p.registry.CallableCapabilities() {
		normalized := desc
		normalized.Source.ProviderID = p.provider.ID
		normalized.Source.Scope = core.CapabilityScopeProvider
		normalized.RuntimeFamily = core.CapabilityRuntimeFamilyProvider
		if normalized.TrustClass == "" {
			normalized.TrustClass = core.TrustClassWorkspaceTrusted
		}
		if err := registrar.RegisterCapability(normalized); err != nil {
			return err
		}
	}
	return nil
}

func (p *LocalNexusNodeProvider) ListSessions(context.Context) ([]core.ProviderSession, error) {
	return nil, nil
}

func (p *LocalNexusNodeProvider) HealthSnapshot(context.Context) (core.ProviderHealthSnapshot, error) {
	p.healthMu.RLock()
	defer p.healthMu.RUnlock()
	return core.ProviderHealthSnapshot{
		Status:  "online",
		Message: "local nexus node active",
		Metadata: map[string]interface{}{
			"last_seen_at": p.health.LastSeenAt,
			"platform":     string(p.descriptor.Platform),
		},
	}, nil
}

func (p *LocalNexusNodeProvider) Close(context.Context) error {
	p.closeOnce.Do(func() {
		close(p.healthWatch)
	})
	return nil
}

func (p *LocalNexusNodeProvider) NodeDescriptor() core.NodeDescriptor {
	return p.descriptor
}

func (p *LocalNexusNodeProvider) NodeHealth(context.Context) (core.NodeHealth, error) {
	p.healthMu.RLock()
	defer p.healthMu.RUnlock()
	return p.health, nil
}

func (p *LocalNexusNodeProvider) StreamHealth(context.Context) (<-chan core.NodeHealth, error) {
	return p.healthWatch, nil
}

func (p *LocalNexusNodeProvider) VerifyCredential(cred core.NodeCredential) error {
	if cred.DeviceID != p.credential.DeviceID {
		return fmt.Errorf("credential device id mismatch")
	}
	return nil
}

func defaultNodePlatform() core.NodePlatform {
	switch runtime.GOOS {
	case "darwin":
		return core.NodePlatformMacOS
	case "linux":
		return core.NodePlatformLinux
	case "windows":
		return core.NodePlatformWindows
	default:
		return core.NodePlatformHeadless
	}
}

func cloneTags(tags map[string]string) map[string]string {
	if len(tags) == 0 {
		return nil
	}
	out := make(map[string]string, len(tags))
	for key, value := range tags {
		out[key] = value
	}
	return out
}

func registerLocalNexusNodeProvider(ctx context.Context, rt *Runtime) error {
	if rt == nil || rt.Tools == nil {
		return nil
	}
	cfg := rt.Workspace.NodeRegistration
	if !cfg.Enabled {
		return nil
	}
	provider, err := NewLocalNexusNodeProvider(rt.Tools, cfg)
	if err != nil {
		return err
	}
	if rt.Workspace.Nexus.Address == "" && !cfg.LocalOnly {
		if rt.Logger != nil {
			rt.Logger.Printf("node registration enabled but nexus.address is empty; exposing local node provider only")
		}
	}
	return rt.RegisterProvider(ctx, &localNexusNodeRuntimeProvider{provider: provider})
}

type localNexusNodeRuntimeProvider struct {
	provider *LocalNexusNodeProvider
}

func (p *localNexusNodeRuntimeProvider) Initialize(_ context.Context, rt *Runtime) error {
	if rt == nil {
		return fmt.Errorf("runtime unavailable")
	}
	rt.NexusNodeProvider = p.provider
	if rt.Context != nil {
		rt.Context.Set("nexus.node_registration.enabled", true)
		rt.Context.Set("nexus.node_registration.node_id", p.provider.NodeDescriptor().ID)
	}
	return nil
}

func (p *localNexusNodeRuntimeProvider) Close() error {
	if p == nil || p.provider == nil {
		return nil
	}
	return p.provider.Close(context.Background())
}

func (p *localNexusNodeRuntimeProvider) Descriptor() core.ProviderDescriptor {
	if p == nil || p.provider == nil {
		return core.ProviderDescriptor{}
	}
	return p.provider.Descriptor()
}
