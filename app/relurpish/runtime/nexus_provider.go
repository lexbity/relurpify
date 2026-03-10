package runtime

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"

	"github.com/lexcodex/relurpify/framework/core"
)

type nexusGatewayRuntimeProvider struct {
	client *NexusClient
	mu     sync.Mutex
	ids    map[string]struct{}
}

func (p *nexusGatewayRuntimeProvider) Initialize(ctx context.Context, rt *Runtime) error {
	if rt == nil || rt.Tools == nil {
		return fmt.Errorf("runtime unavailable")
	}
	if p == nil || p.client == nil {
		return fmt.Errorf("nexus client unavailable")
	}
	p.client.OnConnected = func(capabilities []core.CapabilityDescriptor) {
		_ = p.syncCapabilities(context.Background(), rt, capabilities)
	}
	if err := p.client.Start(ctx); err != nil {
		return err
	}
	if err := p.syncCapabilities(ctx, rt, p.client.Capabilities()); err != nil {
		return err
	}
	rt.NexusClient = p.client
	if rt.Context != nil {
		rt.Context.Set("nexus.enabled", true)
		rt.Context.Set("nexus.session_id", p.client.SessionID())
	}
	runCtx, cancel := context.WithCancel(context.Background())
	rt.nexusCancel = cancel
	go consumeNexusEventsWithDispatcher(runCtx, rt, p.client)
	return nil
}

func (p *nexusGatewayRuntimeProvider) syncCapabilities(ctx context.Context, rt *Runtime, capabilities []core.CapabilityDescriptor) error {
	if rt == nil || rt.Tools == nil {
		return fmt.Errorf("runtime unavailable")
	}
	registrar, err := rt.Tools.ProviderCapabilityRegistrar(p.Descriptor(), core.ProviderPolicy{DefaultTrust: core.TrustClassRemoteApproved})
	if err != nil {
		return err
	}
	current := make(map[string]struct{}, len(capabilities))
	for _, desc := range capabilities {
		normalized := desc
		normalized.Source.ProviderID = p.Descriptor().ID
		normalized.Source.Scope = core.CapabilityScopeProvider
		normalized.RuntimeFamily = core.CapabilityRuntimeFamilyProvider
		current[normalized.ID] = struct{}{}
		switch normalized.Kind {
		case core.CapabilityKindTool:
			if _, ok := rt.Tools.GetCapability(normalized.ID); ok {
				continue
			}
			if err := rt.Tools.RegisterInvocableCapability(nexusRemoteInvocableCapability{
				client: p.client,
				desc:   normalized,
			}); err != nil && !isAlreadyRegistered(err) {
				return err
			}
		default:
			if err := registrar.RegisterCapability(normalized); err != nil && !isAlreadyRegistered(err) {
				return err
			}
		}
	}
	p.mu.Lock()
	for id := range p.ids {
		if _, ok := current[id]; !ok {
			rt.Tools.RevokeCapability(id, "nexus gateway capability removed")
		}
	}
	p.ids = current
	p.mu.Unlock()
	return nil
}

func (p *nexusGatewayRuntimeProvider) Close() error {
	if p == nil || p.client == nil {
		return nil
	}
	return p.client.Close()
}

func (p *nexusGatewayRuntimeProvider) Descriptor() core.ProviderDescriptor {
	return core.ProviderDescriptor{
		ID:                 "nexus-gateway",
		Kind:               core.ProviderKindAgentRuntime,
		ConfiguredSource:   "nexus/ws",
		ActivationScope:    "workspace",
		TrustBaseline:      core.TrustClassRemoteApproved,
		RecoverabilityMode: core.RecoverabilityInProcess,
		SupportsHealth:     true,
		Security: core.ProviderSecurityProfile{
			Origin:                     core.ProviderOriginRemote,
			RequiresFrameworkMediation: true,
		},
	}
}

func registerNexusGatewayProvider(ctx context.Context, rt *Runtime) error {
	if rt == nil {
		return nil
	}
	cfg := rt.Workspace.Nexus
	if !cfg.Enabled || cfg.Address == "" {
		return nil
	}
	client := NewNexusClient(cfg)
	return rt.RegisterProvider(ctx, &nexusGatewayRuntimeProvider{client: client})
}

type nexusRemoteInvocableCapability struct {
	client *NexusClient
	desc   core.CapabilityDescriptor
}

func (c nexusRemoteInvocableCapability) Descriptor(context.Context, *core.Context) core.CapabilityDescriptor {
	return c.desc
}

func (c nexusRemoteInvocableCapability) Invoke(ctx context.Context, state *core.Context, args map[string]interface{}) (*core.CapabilityExecutionResult, error) {
	if c.client == nil {
		return nil, fmt.Errorf("nexus client unavailable")
	}
	request := make(map[string]any, len(args))
	for key, value := range args {
		request[key] = value
	}
	return c.client.InvokeCapability(ctx, activeNexusSessionKey(state), c.desc.ID, request)
}

func decodeNexusInstruction(event core.FrameworkEvent) (instruction string, sessionKey string, metadata map[string]any, ok bool) {
	var payload map[string]any
	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		return "", "", nil, false
	}
	metadata = map[string]any{
		"nexus_event_type": event.Type,
		"nexus_event_seq":  fmt.Sprintf("%d", event.Seq),
	}
	if value, ok := payload["session_key"].(string); ok && value != "" {
		sessionKey = value
	}
	if sessionKey == "" {
		sessionKey = event.Actor.ID
	}
	if channelName, ok := payload["channel"].(string); ok && channelName != "" {
		metadata["channel"] = channelName
	}
	if content, ok := payload["content"].(map[string]any); ok {
		if text, ok := content["text"].(string); ok && strings.TrimSpace(text) != "" {
			return text, sessionKey, metadata, true
		}
	}
	if text, ok := payload["text"].(string); ok && strings.TrimSpace(text) != "" {
		return text, sessionKey, metadata, true
	}
	return "", sessionKey, metadata, false
}

func formatNexusResult(result *core.Result) string {
	if result == nil {
		return ""
	}
	if result.Error != nil {
		return result.Error.Error()
	}
	for _, key := range []string{"response", "summary", "message", "result"} {
		if value, ok := result.Data[key]; ok {
			if text := strings.TrimSpace(fmt.Sprint(value)); text != "" {
				return text
			}
		}
	}
	if len(result.Data) > 0 {
		data, err := json.Marshal(result.Data)
		if err == nil {
			return string(data)
		}
	}
	if result.Success {
		return "completed"
	}
	return "failed"
}

func activeNexusSessionKey(state *core.Context) string {
	if state == nil {
		return ""
	}
	for _, key := range []string{"session_key", "nexus.session_key"} {
		if value := strings.TrimSpace(state.GetString(key)); value != "" {
			return value
		}
	}
	return ""
}
