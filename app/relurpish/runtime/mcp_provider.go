package runtime

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	fauthorization "codeburg.org/lexbit/relurpify/framework/authorization"
	"codeburg.org/lexbit/relurpify/framework/capability"
	"codeburg.org/lexbit/relurpify/framework/core"
	"codeburg.org/lexbit/relurpify/framework/sandbox"
	mclient "codeburg.org/lexbit/relurpify/relurpnet/mcp/client"
	"codeburg.org/lexbit/relurpify/relurpnet/mcp/mapping"
	"codeburg.org/lexbit/relurpify/relurpnet/mcp/protocol"
	mserver "codeburg.org/lexbit/relurpify/relurpnet/mcp/server"
	msession "codeburg.org/lexbit/relurpify/relurpnet/mcp/session"
	mstdio "codeburg.org/lexbit/relurpify/relurpnet/mcp/transport/stdio"
)

type mcpClientProvider struct {
	config    core.ProviderConfig
	desc      core.ProviderDescriptor
	telemetry core.Telemetry
	launcher  mstdio.Launcher

	mu            sync.Mutex
	client        *mclient.Client
	session       core.ProviderSessionSnapshot
	capabilityIDs map[string]struct{}
	closed        bool
}

var mcpClientLauncherFactory func(core.ProviderConfig) mstdio.Launcher

type mcpServerProvider struct {
	config    core.ProviderConfig
	desc      core.ProviderDescriptor
	telemetry core.Telemetry
	runtime   *Runtime
	service   *mserver.Service
	peerSeq   int

	mu       sync.Mutex
	sessions map[string]core.ProviderSessionSnapshot
	closed   bool
}

func newMCPClientProvider(config core.ProviderConfig) *mcpClientProvider {
	provider := &mcpClientProvider{
		config: config,
		desc: core.ProviderDescriptor{
			ID:                 config.ID,
			Kind:               core.ProviderKindMCPClient,
			ConfiguredSource:   config.Target,
			ActivationScope:    config.ActivationScope,
			TrustBaseline:      defaultProviderTrust(config, core.TrustClassRemoteDeclared),
			RecoverabilityMode: defaultProviderRecoverability(config, core.RecoverabilityPersistedRestore),
			SupportsHealth:     true,
			Security: core.ProviderSecurityProfile{
				Origin:                     core.ProviderOriginRemote,
				RequiresFrameworkMediation: true,
			},
		},
		capabilityIDs: make(map[string]struct{}),
	}
	if mcpClientLauncherFactory != nil {
		provider.launcher = mcpClientLauncherFactory(config)
	}
	return provider
}

func newMCPServerProvider(config core.ProviderConfig) *mcpServerProvider {
	return &mcpServerProvider{
		config: config,
		desc: core.ProviderDescriptor{
			ID:                 config.ID,
			Kind:               core.ProviderKindMCPServer,
			ConfiguredSource:   config.Target,
			ActivationScope:    config.ActivationScope,
			TrustBaseline:      defaultProviderTrust(config, core.TrustClassProviderLocalUntrusted),
			RecoverabilityMode: defaultProviderRecoverability(config, core.RecoverabilityPersistedRestore),
			SupportsHealth:     true,
			Security: core.ProviderSecurityProfile{
				Origin:                     core.ProviderOriginLocal,
				RequiresFrameworkMediation: true,
			},
		},
		sessions: make(map[string]core.ProviderSessionSnapshot),
	}
}

func (p *mcpClientProvider) Initialize(ctx context.Context, rt *Runtime) error {
	if p == nil {
		return fmt.Errorf("provider unavailable")
	}
	if rt == nil || rt.Tools == nil {
		return fmt.Errorf("runtime tools unavailable")
	}
	p.telemetry = rt.Telemetry
	policy := providerPolicyFromRuntime(rt, p.desc.ID)
	registrar, err := rt.Tools.ProviderCapabilityRegistrar(p.desc, policy)
	if err != nil {
		return err
	}
	sessionID := primarySessionID(p.desc.ID)
	sessionDesc := sessionCapabilityDescriptor(p.desc, sessionID, p.config.Target)
	catalogDesc := clientCatalogCapabilityDescriptor(p.desc, sessionID, p.config.Target)
	if batchRegistrar, ok := registrar.(interface {
		RegisterCapabilitiesBatch([]core.CapabilityDescriptor) error
	}); ok {
		if err := batchRegistrar.RegisterCapabilitiesBatch([]core.CapabilityDescriptor{sessionDesc, catalogDesc}); err != nil && !isAlreadyRegistered(err) {
			return err
		}
	} else {
		if err := registrar.RegisterCapability(sessionDesc); err != nil && !isAlreadyRegistered(err) {
			return err
		}
		if err := registrar.RegisterCapability(catalogDesc); err != nil && !isAlreadyRegistered(err) {
			return err
		}
	}
	p.capabilityIDs[sessionDesc.ID] = struct{}{}
	p.capabilityIDs[catalogDesc.ID] = struct{}{}

	clientCfg, err := p.clientConfig(rt, sessionID)
	if err != nil {
		return err
	}
	client, err := mclient.ConnectStdio(ctx, p.launcher, clientCfg)
	if err != nil {
		return err
	}
	client.SetNotificationHandler(func(method string) {
		switch method {
		case "notifications/tools/list_changed", "notifications/prompts/list_changed", "notifications/resources/list_changed":
			_ = p.syncCatalog(context.Background(), rt, registrar)
		case "notifications/resources/updated":
			p.refreshSessionSnapshot(client.SessionSnapshot())
		}
	})
	client.SetRequestHandler(mcpRuntimeRequestHandler{runtime: rt, provider: p})
	p.mu.Lock()
	p.client = client
	p.mu.Unlock()
	if err := p.syncCatalog(ctx, rt, registrar); err != nil {
		_ = client.Close()
		return err
	}
	p.refreshSessionSnapshot(client.SessionSnapshot())
	return nil
}

func (p *mcpClientProvider) syncCatalog(ctx context.Context, rt *Runtime, registrar core.CapabilityRegistrar) error {
	p.mu.Lock()
	client := p.client
	p.mu.Unlock()
	if client == nil {
		return fmt.Errorf("mcp client not connected")
	}
	tools, err := client.ListTools(ctx)
	if err != nil {
		return err
	}
	prompts, err := client.ListPrompts(ctx)
	if err != nil {
		return err
	}
	resources, err := client.ListResources(ctx)
	if err != nil {
		return err
	}
	snap := client.SessionSnapshot()
	current := map[string]struct{}{
		sessionCapabilityID(p.desc.ID):    {},
		clientCatalogCapabilityID(p.desc): {},
	}
	invocableItems := make([]capability.RegistrationBatchItem, 0, len(tools)+len(resources))
	for _, remoteTool := range tools {
		desc, err := mapping.ImportedToolDescriptor(p.desc.ID, snap.SessionID, snap.NegotiatedVersion, remoteTool, p.desc.TrustBaseline)
		if err != nil {
			return err
		}
		if coordination := importedToolCoordinationMetadata(p.config.Config, remoteTool.Name); coordination != nil {
			desc.Coordination = coordination
			desc.Category = "mcp-coordination"
			desc.Tags = append(desc.Tags, "coordination", "remote-task-service")
			if desc.Annotations == nil {
				desc.Annotations = map[string]any{}
			}
			desc.Annotations["mcp_coordination_service"] = true
			desc = core.NormalizeCapabilityDescriptor(desc)
		}
		current[desc.ID] = struct{}{}
		if _, ok := rt.Tools.GetCapability(desc.ID); ok {
			continue
		}
		invocableItems = append(invocableItems, capability.RegistrationBatchItem{
			InvocableHandler: mcpRemoteToolCapability{
				desc:    desc,
				client:  client,
				tool:    remoteTool,
				session: snap.SessionID,
			},
		})
	}
	promptDescs := make([]core.CapabilityDescriptor, 0, len(prompts))
	for _, prompt := range prompts {
		desc := mapping.ImportedPromptDescriptor(p.desc.ID, snap.SessionID, snap.NegotiatedVersion, prompt, p.desc.TrustBaseline)
		current[desc.ID] = struct{}{}
		promptDescs = append(promptDescs, desc)
	}
	if len(promptDescs) > 0 {
		if batchRegistrar, ok := registrar.(interface {
			RegisterCapabilitiesBatch([]core.CapabilityDescriptor) error
		}); ok {
			if err := batchRegistrar.RegisterCapabilitiesBatch(promptDescs); err != nil && !isAlreadyRegistered(err) {
				return err
			}
		} else {
			for _, desc := range promptDescs {
				if err := registrar.RegisterCapability(desc); err != nil && !isAlreadyRegistered(err) {
					return err
				}
			}
		}
	}
	resourceDescs := make([]core.CapabilityDescriptor, 0, len(resources))
	for _, resource := range resources {
		desc := mapping.ImportedResourceDescriptor(p.desc.ID, snap.SessionID, snap.NegotiatedVersion, resource, p.desc.TrustBaseline)
		current[desc.ID] = struct{}{}
		resourceDescs = append(resourceDescs, desc)
		if supportsRemoteResourceSubscriptions(snap.RemoteCapabilities) {
			subDesc := importedResourceSubscriptionDescriptor(p.desc.ID, snap.SessionID, snap.NegotiatedVersion, resource, p.desc.TrustBaseline)
			current[subDesc.ID] = struct{}{}
			if _, ok := rt.Tools.GetCapability(subDesc.ID); !ok {
				invocableItems = append(invocableItems, capability.RegistrationBatchItem{
					InvocableHandler: mcpRemoteResourceSubscriptionCapability{
						desc:     subDesc,
						client:   client,
						uri:      resource.URI,
						session:  snap.SessionID,
						provider: p.desc.ID,
						owner:    p,
					},
				})
			}
		}
	}
	if len(invocableItems) > 0 {
		if err := rt.Tools.RegisterBatch(invocableItems); err != nil && !isAlreadyRegistered(err) {
			return err
		}
	}
	if len(resourceDescs) > 0 {
		if batchRegistrar, ok := registrar.(interface {
			RegisterCapabilitiesBatch([]core.CapabilityDescriptor) error
		}); ok {
			if err := batchRegistrar.RegisterCapabilitiesBatch(resourceDescs); err != nil && !isAlreadyRegistered(err) {
				return err
			}
		} else {
			for _, desc := range resourceDescs {
				if err := registrar.RegisterCapability(desc); err != nil && !isAlreadyRegistered(err) {
					return err
				}
			}
		}
	}
	p.mu.Lock()
	for id := range p.capabilityIDs {
		if _, ok := current[id]; !ok && rt.Tools != nil {
			rt.Tools.RevokeCapability(id, "mcp remote capability removed")
		}
	}
	p.capabilityIDs = current
	p.mu.Unlock()
	p.refreshSessionSnapshot(snap)
	return nil
}

func (p *mcpClientProvider) refreshSessionSnapshot(snapshot msession.Snapshot) {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	capabilityIDs := p.sortedCapabilityIDs()
	p.mu.Lock()
	defer p.mu.Unlock()
	createdAt := p.session.Session.CreatedAt
	if createdAt == "" {
		createdAt = now
	}
	p.session = core.ProviderSessionSnapshot{
		Session: core.ProviderSession{
			ID:             snapshot.SessionID,
			ProviderID:     p.desc.ID,
			CapabilityIDs:  capabilityIDs,
			TrustClass:     p.desc.TrustBaseline,
			Recoverability: p.desc.RecoverabilityMode,
			CreatedAt:      createdAt,
			LastActivityAt: now,
			Health:         fmt.Sprint(snapshot.State),
			Metadata: map[string]any{
				"target":              p.config.Target,
				"kind":                string(p.desc.Kind),
				"protocol_version":    snapshot.NegotiatedVersion,
				"transport":           snapshot.TransportKind,
				"remote_peer_name":    snapshot.RemotePeer.Name,
				"remote_peer_version": snapshot.RemotePeer.Version,
			},
		},
		State: map[string]any{
			"target":               p.config.Target,
			"requested_version":    snapshot.RequestedVersion,
			"negotiated_version":   snapshot.NegotiatedVersion,
			"transport":            snapshot.TransportKind,
			"remote_target":        snapshot.RemoteTarget,
			"active_requests":      snapshot.ActiveRequests,
			"remote_capabilities":  snapshot.RemoteCapabilities,
			"local_capabilities":   snapshot.LocalCapabilities,
			"active_subscriptions": snapshot.ActiveSubscriptions,
			"failure_reason":       snapshot.FailureReason,
		},
		CapturedAt: now,
	}
}

func (p *mcpClientProvider) Close() error {
	if p == nil {
		return nil
	}
	p.mu.Lock()
	client := p.client
	p.client = nil
	p.closed = true
	p.mu.Unlock()
	if client != nil {
		return client.Close()
	}
	return nil
}

func (p *mcpClientProvider) CloseSession(_ context.Context, sessionID string) error {
	if p == nil {
		return ErrSessionNotManaged
	}
	p.mu.Lock()
	client := p.client
	current := p.session.Session.ID
	p.mu.Unlock()
	if current != sessionID {
		return ErrSessionNotManaged
	}
	if client != nil {
		return client.Close()
	}
	return nil
}

func (p *mcpClientProvider) Descriptor() core.ProviderDescriptor {
	return p.desc
}

func (p *mcpClientProvider) ListSessions(context.Context) ([]core.ProviderSession, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.session.Session.ID == "" {
		return nil, nil
	}
	return []core.ProviderSession{p.session.Session}, nil
}

func (p *mcpClientProvider) HealthSnapshot(context.Context) (core.ProviderHealthSnapshot, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	status := "healthy"
	if p.closed {
		status = "closed"
	}
	metadata := map[string]any{
		"provider_kind":   string(p.desc.Kind),
		"configured_from": p.config.Target,
		"session_count":   0,
	}
	if p.session.Session.ID != "" {
		metadata["session_count"] = 1
		if version, ok := p.session.State.(map[string]any)["negotiated_version"]; ok {
			metadata["protocol_version"] = version
		}
	}
	return core.ProviderHealthSnapshot{
		Status:   status,
		Message:  "mcp client connected",
		Metadata: metadata,
	}, nil
}

func (p *mcpClientProvider) SnapshotProvider(ctx context.Context) (*core.ProviderSnapshot, error) {
	health, err := p.HealthSnapshot(ctx)
	if err != nil {
		return nil, err
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	return &core.ProviderSnapshot{
		ProviderID:     p.desc.ID,
		Recoverability: p.desc.RecoverabilityMode,
		Descriptor:     p.desc,
		Health:         health,
		CapabilityIDs:  p.sortedCapabilityIDs(),
		Metadata: map[string]any{
			"target":        p.config.Target,
			"provider_kind": string(p.desc.Kind),
		},
		State: map[string]any{
			"config": p.config.Config,
		},
		CapturedAt: time.Now().UTC().Format(time.RFC3339Nano),
	}, nil
}

func (p *mcpClientProvider) SnapshotSessions(context.Context) ([]core.ProviderSessionSnapshot, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.session.Session.ID == "" {
		return nil, nil
	}
	return []core.ProviderSessionSnapshot{p.session}, nil
}

func (p *mcpClientProvider) clientConfig(rt *Runtime, sessionID string) (mclient.StdioConfig, error) {
	command, ok := stringConfigValue(p.config.Config, "command")
	if !ok || strings.TrimSpace(command) == "" {
		return mclient.StdioConfig{}, fmt.Errorf("mcp client provider %s requires config.command for stdio transport", p.desc.ID)
	}
	args := stringSliceConfigValue(p.config.Config, "args")
	env := stringSliceConfigValue(p.config.Config, "env")
	workdir, _ := stringConfigValue(p.config.Config, "workdir")
	preferredVersions := stringSliceConfigValue(p.config.Config, "protocol_versions")
	if len(preferredVersions) == 0 {
		if version, ok := stringConfigValue(p.config.Config, "protocol_version"); ok && strings.TrimSpace(version) != "" {
			preferredVersions = []string{version}
		}
	}
	if len(preferredVersions) == 0 {
		preferredVersions = []string{protocol.Revision20250618}
	}
	var policy sandbox.CommandPolicy
	if rt != nil && rt.Registration != nil && rt.Registration.Permissions != nil {
		policy = fauthorization.NewCommandAuthorizationPolicy(rt.Registration.Permissions, rt.Registration.ID, rt.AgentSpec, "mcp")
	}
	return mclient.StdioConfig{
		Command:           command,
		Args:              args,
		Dir:               workdir,
		Env:               env,
		ProviderID:        p.desc.ID,
		SessionID:         sessionID,
		RemoteTarget:      p.config.Target,
		LocalPeer:         protocol.PeerInfo{Name: "relurpify", Version: "dev"},
		Capabilities:      p.localClientCapabilities(),
		PreferredVersions: preferredVersions,
		Recoverable:       p.desc.RecoverabilityMode != core.RecoverabilityEphemeral,
		Policy:            policy,
	}, nil
}

func (p *mcpClientProvider) localClientCapabilities() map[string]any {
	capabilities := map[string]any{}
	if boolConfigValue(p.config.Config, "enable_sampling") {
		capabilities["sampling"] = map[string]any{}
	}
	if boolConfigValue(p.config.Config, "enable_elicitation") {
		capabilities["elicitation"] = map[string]any{}
	}
	if boolConfigValue(p.config.Config, "enable_resource_subscriptions") {
		capabilities["resources"] = map[string]any{"subscribe": true}
	}
	return capabilities
}

func (p *mcpClientProvider) sortedCapabilityIDs() []string {
	out := make([]string, 0, len(p.capabilityIDs))
	for id := range p.capabilityIDs {
		out = append(out, id)
	}
	return out
}

type mcpRemoteToolCapability struct {
	desc    core.CapabilityDescriptor
	client  *mclient.Client
	tool    protocol.Tool
	session string
}

type mcpRemoteResourceSubscriptionCapability struct {
	desc     core.CapabilityDescriptor
	client   *mclient.Client
	uri      string
	session  string
	provider string
	owner    *mcpClientProvider
}

func (c mcpRemoteResourceSubscriptionCapability) Descriptor(context.Context, *core.Context) core.CapabilityDescriptor {
	return c.desc
}

func (c mcpRemoteResourceSubscriptionCapability) Invoke(ctx context.Context, _ *core.Context, args map[string]interface{}) (*core.ToolResult, error) {
	if c.client == nil {
		return nil, fmt.Errorf("mcp client unavailable")
	}
	action := strings.ToLower(strings.TrimSpace(fmt.Sprint(args["action"])))
	if action == "" || action == "<nil>" {
		action = "subscribe"
	}
	switch action {
	case "subscribe":
		if err := c.client.SubscribeResource(ctx, c.uri); err != nil {
			return nil, err
		}
	case "unsubscribe":
		if err := c.client.UnsubscribeResource(ctx, c.uri); err != nil {
			return nil, err
		}
	default:
		return nil, fmt.Errorf("unsupported subscription action %s", action)
	}
	if c.owner != nil {
		c.owner.refreshSessionSnapshot(c.client.SessionSnapshot())
	}
	return &core.ToolResult{
		Success: true,
		Data: map[string]interface{}{
			"uri":        c.uri,
			"action":     action,
			"session_id": c.session,
		},
	}, nil
}

func (c mcpRemoteResourceSubscriptionCapability) Availability(context.Context, *core.Context) core.AvailabilitySpec {
	if c.client == nil {
		return core.AvailabilitySpec{Available: false, Reason: "mcp client unavailable"}
	}
	return core.AvailabilitySpec{Available: true}
}

type mcpRuntimeRequestHandler struct {
	runtime  *Runtime
	provider *mcpClientProvider
}

func (h mcpRuntimeRequestHandler) HandleSamplingRequest(ctx context.Context, params protocol.CreateMessageParams) (*protocol.CreateMessageResult, error) {
	if h.runtime == nil || h.runtime.Model == nil {
		return nil, fmt.Errorf("runtime model unavailable")
	}
	messages := make([]core.Message, 0, len(params.Messages))
	for _, msg := range params.Messages {
		messages = append(messages, core.Message{
			Role:    msg.Role,
			Content: firstConfiguredNonEmpty(msg.Content.Text, fmt.Sprint(msg.Content.Data)),
		})
	}
	if strings.TrimSpace(params.SystemPrompt) != "" {
		messages = append([]core.Message{{Role: "system", Content: params.SystemPrompt}}, messages...)
	}
	resp, err := h.runtime.Model.Chat(ctx, messages, &core.LLMOptions{
		MaxTokens:   params.MaxTokens,
		Temperature: params.Temperature,
		Stop:        append([]string(nil), params.StopSequences...),
	})
	if err != nil {
		return nil, err
	}
	return &protocol.CreateMessageResult{
		Role:       "assistant",
		Content:    protocol.ContentBlock{Type: "text", Text: resp.Text},
		Model:      h.runtime.Config.InferenceModel,
		StopReason: resp.FinishReason,
	}, nil
}

func (h mcpRuntimeRequestHandler) HandleElicitationRequest(ctx context.Context, params protocol.ElicitationParams) (*protocol.ElicitationResult, error) {
	if h.runtime == nil {
		return &protocol.ElicitationResult{Action: "decline"}, nil
	}
	return h.runtime.handleMCPElicitation(ctx, params)
}

func (c mcpRemoteToolCapability) Descriptor(context.Context, *core.Context) core.CapabilityDescriptor {
	return c.desc
}

func (c mcpRemoteToolCapability) Invoke(ctx context.Context, _ *core.Context, args map[string]interface{}) (*core.ToolResult, error) {
	if c.client == nil {
		return nil, fmt.Errorf("mcp client unavailable")
	}
	result, err := c.client.CallTool(ctx, protocol.CallToolParams{Name: c.tool.Name, Arguments: args})
	if err != nil {
		return nil, err
	}
	data := map[string]interface{}{}
	for key, value := range result.StructuredContent {
		data[key] = value
	}
	if len(data) == 0 && len(result.Content) > 0 {
		for _, block := range result.Content {
			if block.Type == "text" && strings.TrimSpace(block.Text) != "" {
				data["summary"] = block.Text
				break
			}
		}
	}
	metadata := map[string]interface{}{
		"session_id":   c.session,
		"mcp_is_error": result.IsError,
	}
	if blocks := mapping.CoreContentBlocksFromProtocol(result.Content); len(blocks) > 0 {
		metadata["content_blocks"] = blocks
		for _, block := range result.Content {
			switch block.Type {
			case "resource":
				if strings.TrimSpace(block.URI) != "" && data["resource_uri"] == nil {
					data["resource_uri"] = block.URI
				}
			case "blob":
				if strings.TrimSpace(block.Blob) != "" && data["blob_ref"] == nil {
					data["blob_ref"] = block.Blob
				}
			}
		}
	}
	return &core.ToolResult{
		Success:  !result.IsError,
		Data:     data,
		Metadata: metadata,
	}, nil
}

func (c mcpRemoteToolCapability) Availability(context.Context, *core.Context) core.AvailabilitySpec {
	if c.client == nil {
		return core.AvailabilitySpec{Available: false, Reason: "mcp client unavailable"}
	}
	snap := c.client.SessionSnapshot()
	if fmt.Sprint(snap.State) != "initialized" {
		return core.AvailabilitySpec{Available: false, Reason: "mcp session not initialized"}
	}
	return core.AvailabilitySpec{Available: true}
}

func (p *mcpServerProvider) Initialize(ctx context.Context, rt *Runtime) error {
	if p == nil {
		return fmt.Errorf("provider unavailable")
	}
	if rt == nil || rt.Tools == nil {
		return fmt.Errorf("runtime tools unavailable")
	}
	p.telemetry = rt.Telemetry
	p.runtime = rt
	policy := providerPolicyFromRuntime(rt, p.desc.ID)
	registrar, err := rt.Tools.ProviderCapabilityRegistrar(p.desc, policy)
	if err != nil {
		return err
	}
	sessionID := primarySessionID(p.desc.ID)
	for _, desc := range serverBaselineCapabilities(p.desc, sessionID, p.config.Target) {
		if err := registrar.RegisterCapability(desc); err != nil && !isAlreadyRegistered(err) {
			return err
		}
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	p.mu.Lock()
	p.sessions[sessionID] = core.ProviderSessionSnapshot{
		Session: core.ProviderSession{
			ID:             sessionID,
			ProviderID:     p.desc.ID,
			CapabilityIDs:  []string{sessionCapabilityID(p.desc.ID), serverCatalogCapabilityID(p.desc)},
			TrustClass:     p.desc.TrustBaseline,
			Recoverability: p.desc.RecoverabilityMode,
			CreatedAt:      now,
			LastActivityAt: now,
			Health:         "active",
			Metadata: map[string]any{
				"target": p.config.Target,
				"kind":   string(p.desc.Kind),
			},
		},
		State: map[string]any{
			"target": p.config.Target,
			"config": p.config.Config,
		},
		CapturedAt: now,
	}
	p.mu.Unlock()
	p.service = mserver.New(protocol.PeerInfo{Name: "relurpify", Version: "dev"}, serverExporter{provider: p}, mserver.Hooks{
		OnSessionOpen: func(peerID, requested string) {
			p.trackPeerOpen(peerID, requested)
		},
		OnSessionInitialized: func(peerID, negotiated string, client protocol.PeerInfo) {
			p.trackPeerInitialized(peerID, negotiated, client)
		},
		OnSessionClosed: func(peerID string, err error) {
			p.trackPeerClosed(peerID, err)
		},
	})
	return nil
}

func (p *mcpServerProvider) Close() error {
	if p == nil {
		return nil
	}
	p.mu.Lock()
	p.closed = true
	p.sessions = map[string]core.ProviderSessionSnapshot{}
	p.mu.Unlock()
	return nil
}

func (p *mcpServerProvider) CloseSession(_ context.Context, sessionID string) error {
	if p == nil {
		return ErrSessionNotManaged
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if _, ok := p.sessions[sessionID]; !ok {
		return ErrSessionNotManaged
	}
	delete(p.sessions, sessionID)
	return nil
}

func (p *mcpServerProvider) Descriptor() core.ProviderDescriptor { return p.desc }

func (p *mcpServerProvider) HTTPHandler() http.Handler {
	if p == nil {
		return nil
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.service
}

func (p *mcpServerProvider) ListSessions(context.Context) ([]core.ProviderSession, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	out := make([]core.ProviderSession, 0, len(p.sessions))
	for _, snapshot := range p.sessions {
		out = append(out, snapshot.Session)
	}
	return out, nil
}

func (p *mcpServerProvider) HealthSnapshot(context.Context) (core.ProviderHealthSnapshot, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	status := "healthy"
	if p.closed {
		status = "closed"
	}
	return core.ProviderHealthSnapshot{
		Status:  status,
		Message: "mcp server configured",
		Metadata: map[string]any{
			"provider_kind":   string(p.desc.Kind),
			"configured_from": p.config.Target,
			"session_count":   len(p.sessions),
		},
	}, nil
}

func (p *mcpServerProvider) SnapshotProvider(ctx context.Context) (*core.ProviderSnapshot, error) {
	health, err := p.HealthSnapshot(ctx)
	if err != nil {
		return nil, err
	}
	p.mu.Lock()
	sessionCount := len(p.sessions)
	p.mu.Unlock()
	return &core.ProviderSnapshot{
		ProviderID:     p.desc.ID,
		Recoverability: p.desc.RecoverabilityMode,
		Descriptor:     p.desc,
		Health:         health,
		CapabilityIDs:  []string{sessionCapabilityID(p.desc.ID), serverCatalogCapabilityID(p.desc)},
		Metadata: map[string]any{
			"target":        p.config.Target,
			"provider_kind": string(p.desc.Kind),
			"session_count": sessionCount,
		},
		State: map[string]any{
			"config": p.config.Config,
		},
		CapturedAt: time.Now().UTC().Format(time.RFC3339Nano),
	}, nil
}

func (p *mcpServerProvider) SnapshotSessions(context.Context) ([]core.ProviderSessionSnapshot, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	out := make([]core.ProviderSessionSnapshot, 0, len(p.sessions))
	for _, snapshot := range p.sessions {
		out = append(out, snapshot)
	}
	return out, nil
}

func (p *mcpServerProvider) openLoopbackSession(ctx context.Context) (io.ReadWriteCloser, string, error) {
	p.mu.Lock()
	p.peerSeq++
	peerID := p.desc.ID + ":peer:" + strconv.Itoa(p.peerSeq)
	service := p.service
	p.mu.Unlock()
	if service == nil {
		return nil, "", fmt.Errorf("mcp server service unavailable")
	}
	serverConn, clientConn := net.Pipe()
	go func() {
		_ = service.ServeConn(ctx, peerID, serverConn)
	}()
	return clientConn, peerID, nil
}

func (p *mcpServerProvider) trackPeerOpen(peerID, requested string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	now := time.Now().UTC().Format(time.RFC3339Nano)
	p.sessions[peerID] = core.ProviderSessionSnapshot{
		Session: core.ProviderSession{
			ID:             peerID,
			ProviderID:     p.desc.ID,
			CapabilityIDs:  []string{sessionCapabilityID(p.desc.ID), serverCatalogCapabilityID(p.desc)},
			TrustClass:     p.desc.TrustBaseline,
			Recoverability: p.desc.RecoverabilityMode,
			CreatedAt:      now,
			LastActivityAt: now,
			Health:         "connecting",
			Metadata: map[string]any{
				"target":            p.config.Target,
				"kind":              string(p.desc.Kind),
				"requested_version": requested,
			},
		},
		State: map[string]any{
			"requested_version": requested,
		},
		CapturedAt: now,
	}
}

func (p *mcpServerProvider) trackPeerInitialized(peerID, negotiated string, client protocol.PeerInfo) {
	p.mu.Lock()
	defer p.mu.Unlock()
	snapshot, ok := p.sessions[peerID]
	if !ok {
		return
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	snapshot.Session.Health = "initialized"
	snapshot.Session.LastActivityAt = now
	if snapshot.Session.Metadata == nil {
		snapshot.Session.Metadata = map[string]any{}
	}
	snapshot.Session.Metadata["protocol_version"] = negotiated
	snapshot.Session.Metadata["client_name"] = client.Name
	snapshot.Session.Metadata["client_version"] = client.Version
	state, _ := snapshot.State.(map[string]any)
	if state == nil {
		state = map[string]any{}
	}
	state["negotiated_version"] = negotiated
	state["client_name"] = client.Name
	state["client_version"] = client.Version
	snapshot.State = state
	snapshot.CapturedAt = now
	p.sessions[peerID] = snapshot
}

func (p *mcpServerProvider) trackPeerClosed(peerID string, err error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	snapshot, ok := p.sessions[peerID]
	if !ok {
		return
	}
	snapshot.Session.Health = "closed"
	snapshot.Session.LastActivityAt = time.Now().UTC().Format(time.RFC3339Nano)
	if err != nil {
		if snapshot.Session.Metadata == nil {
			snapshot.Session.Metadata = map[string]any{}
		}
		snapshot.Session.Metadata["close_error"] = err.Error()
	}
	p.sessions[peerID] = snapshot
}

type serverExporter struct {
	provider *mcpServerProvider
}

func (e serverExporter) ListTools(ctx context.Context) ([]protocol.Tool, error) {
	descs, err := e.provider.exportableDescriptors(ctx, core.CapabilityKindTool, true)
	if err != nil {
		return nil, err
	}
	out := make([]protocol.Tool, 0, len(descs))
	for _, desc := range descs {
		out = append(out, mapping.ExportedTool(desc))
	}
	return out, nil
}

func (e serverExporter) CallTool(ctx context.Context, name string, args map[string]any) (*protocol.CallToolResult, error) {
	desc, err := e.provider.findExportedByNameOrID(ctx, core.CapabilityKindTool, name, true)
	if err != nil {
		return nil, err
	}
	result, err := e.provider.runtime.Tools.InvokeCapability(ctx, core.NewContext(), desc.ID, args)
	if err != nil {
		return nil, err
	}
	content := []protocol.ContentBlock{}
	if summary, ok := result.Data["summary"].(string); ok && strings.TrimSpace(summary) != "" {
		content = append(content, protocol.ContentBlock{Type: "text", Text: summary})
	} else if len(result.Data) > 0 {
		content = append(content, protocol.ContentBlock{Type: "text", Text: fmt.Sprint(result.Data)})
	}
	return &protocol.CallToolResult{
		Content:           content,
		StructuredContent: result.Data,
		IsError:           !result.Success,
	}, nil
}

func (e serverExporter) ListPrompts(ctx context.Context) ([]protocol.Prompt, error) {
	descs, err := e.provider.exportableDescriptors(ctx, core.CapabilityKindPrompt, false)
	if err != nil {
		return nil, err
	}
	out := make([]protocol.Prompt, 0, len(descs))
	for _, desc := range descs {
		out = append(out, mapping.ExportedPrompt(desc))
	}
	return out, nil
}

func (e serverExporter) GetPrompt(ctx context.Context, name string, args map[string]any) (*protocol.GetPromptResult, error) {
	desc, err := e.provider.findExportedByNameOrID(ctx, core.CapabilityKindPrompt, name, false)
	if err != nil {
		return nil, err
	}
	if result, ok := mapping.PromptResultFromAnnotation(desc, args); ok {
		return result, nil
	}
	promptResult, err := e.provider.runtime.Tools.RenderPrompt(ctx, core.NewContext(), desc.ID, args)
	if err != nil {
		return nil, err
	}
	return mapping.PromptResultFromCore(promptResult), nil
}

func (e serverExporter) ListResources(ctx context.Context) ([]protocol.Resource, error) {
	descs, err := e.provider.exportableDescriptors(ctx, core.CapabilityKindResource, false)
	if err != nil {
		return nil, err
	}
	out := make([]protocol.Resource, 0, len(descs))
	for _, desc := range descs {
		out = append(out, mapping.ExportedResource(desc))
	}
	return out, nil
}

func (e serverExporter) ReadResource(ctx context.Context, uri string) (*protocol.ReadResourceResult, error) {
	desc, err := e.provider.findExportedByResourceURI(ctx, uri)
	if err != nil {
		return nil, err
	}
	if result, ok := mapping.ResourceResultFromAnnotation(desc); ok {
		return result, nil
	}
	resourceResult, err := e.provider.runtime.Tools.ReadResource(ctx, core.NewContext(), desc.ID)
	if err != nil {
		return nil, err
	}
	return mapping.ResourceResultFromCore(resourceResult), nil
}

func (p *mcpServerProvider) exportableDescriptors(ctx context.Context, kind core.CapabilityKind, requireCallable bool) ([]core.CapabilityDescriptor, error) {
	if p.runtime == nil || p.runtime.Tools == nil {
		return nil, fmt.Errorf("runtime tools unavailable")
	}
	selectors := exportSelectorsFromConfig(p.config.Config, kind)
	allowedNames := exportNamesFromConfig(p.config.Config, exportConfigKey(kind))
	if len(selectors) == 0 && len(allowedNames) == 0 {
		return nil, nil
	}
	var source []core.CapabilityDescriptor
	if requireCallable {
		source = p.runtime.Tools.CallableCapabilities()
	} else {
		source = p.runtime.Tools.AllCapabilities()
	}
	selected := make([]core.CapabilityDescriptor, 0, len(source))
	for _, desc := range source {
		if desc.Kind != kind {
			continue
		}
		if len(selectors) > 0 && !matchesExportSelector(desc, selectors) {
			continue
		}
		if len(selectors) == 0 && !matchesExportName(desc, allowedNames) {
			continue
		}
		selected = append(selected, desc)
	}
	sort.Slice(selected, func(i, j int) bool { return selected[i].ID < selected[j].ID })
	return selected, nil
}

func (p *mcpServerProvider) findExportedByNameOrID(ctx context.Context, kind core.CapabilityKind, name string, requireCallable bool) (core.CapabilityDescriptor, error) {
	descs, err := p.exportableDescriptors(ctx, kind, requireCallable)
	if err != nil {
		return core.CapabilityDescriptor{}, err
	}
	for _, desc := range descs {
		if strings.EqualFold(desc.Name, name) || strings.EqualFold(desc.ID, name) {
			return desc, nil
		}
	}
	return core.CapabilityDescriptor{}, fmt.Errorf("exported %s %s not found", kind, name)
}

func (p *mcpServerProvider) findExportedByResourceURI(ctx context.Context, uri string) (core.CapabilityDescriptor, error) {
	descs, err := p.exportableDescriptors(ctx, core.CapabilityKindResource, false)
	if err != nil {
		return core.CapabilityDescriptor{}, err
	}
	for _, desc := range descs {
		if mapping.ExportedResource(desc).URI == uri {
			return desc, nil
		}
	}
	return core.CapabilityDescriptor{}, fmt.Errorf("exported resource %s not found", uri)
}

func exportConfigKey(kind core.CapabilityKind) string {
	switch kind {
	case core.CapabilityKindTool:
		return "export_tools"
	case core.CapabilityKindPrompt:
		return "export_prompts"
	case core.CapabilityKindResource:
		return "export_resources"
	default:
		return ""
	}
}

func exportNamesFromConfig(values map[string]any, key string) map[string]struct{} {
	out := map[string]struct{}{}
	if strings.TrimSpace(key) == "" {
		return out
	}
	for _, value := range stringSliceConfigValue(values, key) {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		out[strings.ToLower(value)] = struct{}{}
	}
	return out
}

func exportSelectorsFromConfig(values map[string]any, kind core.CapabilityKind) []core.CapabilitySelector {
	key := exportSelectorConfigKey(kind)
	raw, ok := values[key]
	if !ok || raw == nil {
		return nil
	}
	list, ok := raw.([]any)
	if !ok {
		return nil
	}
	selectors := make([]core.CapabilitySelector, 0, len(list))
	for _, item := range list {
		selector, ok := decodeCapabilitySelector(item)
		if !ok {
			continue
		}
		if selector.Kind == "" {
			selector.Kind = kind
		}
		selectors = append(selectors, selector)
	}
	return selectors
}

func exportSelectorConfigKey(kind core.CapabilityKind) string {
	switch kind {
	case core.CapabilityKindTool:
		return "export_tool_selectors"
	case core.CapabilityKindPrompt:
		return "export_prompt_selectors"
	case core.CapabilityKindResource:
		return "export_resource_selectors"
	default:
		return ""
	}
}

func decodeCapabilitySelector(value any) (core.CapabilitySelector, bool) {
	if value == nil {
		return core.CapabilitySelector{}, false
	}
	if selector, ok := value.(core.CapabilitySelector); ok {
		return selector, true
	}
	data, err := json.Marshal(value)
	if err != nil {
		return core.CapabilitySelector{}, false
	}
	var selector core.CapabilitySelector
	if err := json.Unmarshal(data, &selector); err != nil {
		return core.CapabilitySelector{}, false
	}
	if err := core.ValidateCapabilitySelector(selector); err != nil {
		return core.CapabilitySelector{}, false
	}
	return selector, true
}

func matchesExportName(desc core.CapabilityDescriptor, allowed map[string]struct{}) bool {
	if len(allowed) == 0 {
		return false
	}
	if _, ok := allowed[strings.ToLower(desc.ID)]; ok {
		return true
	}
	if _, ok := allowed[strings.ToLower(desc.Name)]; ok {
		return true
	}
	return false
}

func matchesExportSelector(desc core.CapabilityDescriptor, selectors []core.CapabilitySelector) bool {
	for _, selector := range selectors {
		if core.SelectorMatchesDescriptor(selector, desc) {
			return true
		}
	}
	return false
}

func providerPolicyFromRuntime(rt *Runtime, providerID string) core.ProviderPolicy {
	if rt == nil || rt.AgentSpec == nil || rt.AgentSpec.ProviderPolicies == nil {
		return core.ProviderPolicy{}
	}
	return rt.AgentSpec.ProviderPolicies[providerID]
}

func primarySessionID(providerID string) string {
	return providerID + ":primary"
}

func sessionCapabilityDescriptor(desc core.ProviderDescriptor, sessionID, target string) core.CapabilityDescriptor {
	return core.CapabilityDescriptor{
		ID:          sessionCapabilityID(desc.ID),
		Kind:        core.CapabilityKindSession,
		Name:        desc.ID + "-session",
		Version:     "v1",
		Description: "MCP provider session state",
		Category:    "mcp",
		Tags:        []string{"mcp", "session", string(desc.Kind)},
		Source: core.CapabilitySource{
			ProviderID: desc.ID,
			Scope:      core.CapabilityScopeProvider,
			SessionID:  sessionID,
		},
		TrustClass:      desc.TrustBaseline,
		SessionAffinity: sessionID,
		Availability:    core.AvailabilitySpec{Available: true},
		Annotations: map[string]any{
			"mcp_target": target,
		},
	}
}

func clientCatalogCapabilityDescriptor(desc core.ProviderDescriptor, sessionID, target string) core.CapabilityDescriptor {
	return core.CapabilityDescriptor{
		ID:          clientCatalogCapabilityID(desc),
		Kind:        core.CapabilityKindResource,
		Name:        desc.ID + "-catalog",
		Version:     "v1",
		Description: "Imported MCP capability catalog",
		Category:    "mcp",
		Tags:        []string{"mcp", string(desc.Kind), "remote"},
		Source: core.CapabilitySource{
			ProviderID: desc.ID,
			Scope:      core.CapabilityScopeProvider,
			SessionID:  sessionID,
		},
		TrustClass:      desc.TrustBaseline,
		SessionAffinity: sessionID,
		Availability:    core.AvailabilitySpec{Available: true},
		Annotations: map[string]any{
			"mcp_target": target,
		},
	}
}

func serverBaselineCapabilities(desc core.ProviderDescriptor, sessionID, target string) []core.CapabilityDescriptor {
	return []core.CapabilityDescriptor{
		{
			ID:          serverCatalogCapabilityID(desc),
			Kind:        core.CapabilityKindResource,
			Name:        desc.ID + "-exports",
			Version:     "v1",
			Description: "Exported MCP capability catalog",
			Category:    "mcp",
			Tags:        []string{"mcp", string(desc.Kind)},
			Source: core.CapabilitySource{
				ProviderID: desc.ID,
				Scope:      core.CapabilityScopeProvider,
				SessionID:  sessionID,
			},
			TrustClass:      desc.TrustBaseline,
			SessionAffinity: sessionID,
			Availability:    core.AvailabilitySpec{Available: true},
			Annotations: map[string]any{
				"mcp_target": target,
			},
		},
		sessionCapabilityDescriptor(desc, sessionID, target),
	}
}

func clientCatalogCapabilityID(desc core.ProviderDescriptor) string {
	return "resource:" + desc.ID + ":catalog"
}

func serverCatalogCapabilityID(desc core.ProviderDescriptor) string {
	return "resource:" + desc.ID + ":exports"
}

func sessionCapabilityID(providerID string) string {
	return "session:" + providerID + ":primary"
}

func defaultProviderTrust(config core.ProviderConfig, fallback core.TrustClass) core.TrustClass {
	if config.TrustBaseline != "" {
		return config.TrustBaseline
	}
	return fallback
}

func defaultProviderRecoverability(config core.ProviderConfig, fallback core.RecoverabilityMode) core.RecoverabilityMode {
	if config.Recoverability != "" {
		return config.Recoverability
	}
	return fallback
}

func importedToolCoordinationMetadata(values map[string]any, toolName string) *core.CoordinationTargetMetadata {
	if values == nil || strings.TrimSpace(toolName) == "" {
		return nil
	}
	raw, ok := values["coordination_tools"]
	if !ok || raw == nil {
		return nil
	}
	entries, ok := raw.(map[string]any)
	if !ok {
		return nil
	}
	entryRaw, ok := entries[toolName]
	if !ok || entryRaw == nil {
		return nil
	}
	entry, ok := entryRaw.(map[string]any)
	if !ok {
		return nil
	}
	roleText := strings.TrimSpace(fmt.Sprint(entry["role"]))
	if roleText == "" || strings.EqualFold(roleText, "<nil>") {
		return nil
	}
	taskTypes := stringSliceConfigValue(entry, "task_types")
	if len(taskTypes) == 0 {
		return nil
	}
	metadata := &core.CoordinationTargetMetadata{
		Target:                 true,
		Role:                   core.CoordinationRole(roleText),
		TaskTypes:              taskTypes,
		ExecutionModes:         coordinationExecutionModesFromConfig(entry),
		LongRunning:            boolConfigValue(entry, "long_running"),
		DirectInsertionAllowed: boolConfigValue(entry, "direct_insertion_allowed"),
		MaxDepth:               intConfigValue(entry, "max_depth"),
		MaxRuntimeSeconds:      intConfigValue(entry, "max_runtime_seconds"),
	}
	if metadata.LongRunning && !containsBackgroundExecutionModes(metadata.ExecutionModes) {
		metadata.ExecutionModes = append(metadata.ExecutionModes, core.CoordinationExecutionModeBackgroundAgent)
	}
	if err := core.ValidateCoordinationTargetMetadata(metadata); err != nil {
		return nil
	}
	return metadata
}

func providerFromConfig(config core.ProviderConfig) (RuntimeProvider, error) {
	if err := config.Validate(); err != nil {
		return nil, err
	}
	if !config.Enabled {
		return nil, nil
	}
	switch config.Kind {
	case core.ProviderKindMCPClient:
		return newMCPClientProvider(config), nil
	case core.ProviderKindMCPServer:
		return newMCPServerProvider(config), nil
	default:
		return nil, fmt.Errorf("provider kind %s not supported by runtime provider factory", config.Kind)
	}
}

func mergeConfiguredProviders(spec *core.AgentRuntimeSpec) []core.ProviderConfig {
	if spec == nil || len(spec.Providers) == 0 {
		return nil
	}
	out := make([]core.ProviderConfig, 0, len(spec.Providers))
	seen := map[string]struct{}{}
	for _, provider := range spec.Providers {
		if strings.TrimSpace(provider.ID) == "" {
			continue
		}
		if _, ok := seen[provider.ID]; ok {
			continue
		}
		seen[provider.ID] = struct{}{}
		out = append(out, provider)
	}
	return out
}

func isAlreadyRegistered(err error) bool {
	return err != nil && strings.Contains(err.Error(), "already registered")
}

func stringConfigValue(values map[string]any, key string) (string, bool) {
	if values == nil {
		return "", false
	}
	raw, ok := values[key]
	if !ok || raw == nil {
		return "", false
	}
	switch typed := raw.(type) {
	case string:
		return typed, true
	default:
		return fmt.Sprint(typed), true
	}
}

func stringSliceConfigValue(values map[string]any, key string) []string {
	if values == nil {
		return nil
	}
	raw, ok := values[key]
	if !ok || raw == nil {
		return nil
	}
	switch typed := raw.(type) {
	case []string:
		return append([]string(nil), typed...)
	case []any:
		out := make([]string, 0, len(typed))
		for _, item := range typed {
			if item == nil {
				continue
			}
			out = append(out, fmt.Sprint(item))
		}
		return out
	default:
		return []string{fmt.Sprint(typed)}
	}
}

func boolConfigValue(values map[string]any, key string) bool {
	if values == nil {
		return false
	}
	raw, ok := values[key]
	if !ok || raw == nil {
		return false
	}
	switch typed := raw.(type) {
	case bool:
		return typed
	case string:
		return strings.EqualFold(strings.TrimSpace(typed), "true") || strings.TrimSpace(typed) == "1"
	default:
		return strings.EqualFold(strings.TrimSpace(fmt.Sprint(typed)), "true")
	}
}

func intConfigValue(values map[string]any, key string) int {
	if values == nil {
		return 0
	}
	raw, ok := values[key]
	if !ok || raw == nil {
		return 0
	}
	switch typed := raw.(type) {
	case int:
		return typed
	case int32:
		return int(typed)
	case int64:
		return int(typed)
	case float64:
		return int(typed)
	case string:
		n, err := strconv.Atoi(strings.TrimSpace(typed))
		if err == nil {
			return n
		}
	}
	return 0
}

func coordinationExecutionModesFromConfig(values map[string]any) []core.CoordinationExecutionMode {
	rawModes := stringSliceConfigValue(values, "execution_modes")
	if len(rawModes) == 0 {
		return []core.CoordinationExecutionMode{core.CoordinationExecutionModeSync}
	}
	out := make([]core.CoordinationExecutionMode, 0, len(rawModes))
	for _, mode := range rawModes {
		switch normalized := strings.TrimSpace(strings.ToLower(mode)); normalized {
		case string(core.CoordinationExecutionModeSync):
			out = append(out, core.CoordinationExecutionModeSync)
		case string(core.CoordinationExecutionModeSessionBacked):
			out = append(out, core.CoordinationExecutionModeSessionBacked)
		case string(core.CoordinationExecutionModeBackgroundAgent):
			out = append(out, core.CoordinationExecutionModeBackgroundAgent)
		}
	}
	if len(out) == 0 {
		return []core.CoordinationExecutionMode{core.CoordinationExecutionModeSync}
	}
	return out
}

func containsBackgroundExecutionModes(modes []core.CoordinationExecutionMode) bool {
	for _, mode := range modes {
		switch mode {
		case core.CoordinationExecutionModeBackgroundAgent, core.CoordinationExecutionModeSessionBacked:
			return true
		}
	}
	return false
}

func supportsRemoteResourceSubscriptions(capabilities map[string]any) bool {
	if capabilities == nil {
		return false
	}
	raw, ok := capabilities["resources"]
	if !ok || raw == nil {
		return false
	}
	switch typed := raw.(type) {
	case map[string]any:
		value, ok := typed["subscribe"]
		if !ok {
			return false
		}
		switch sub := value.(type) {
		case bool:
			return sub
		default:
			return strings.EqualFold(strings.TrimSpace(fmt.Sprint(sub)), "true")
		}
	default:
		return false
	}
}

func importedResourceSubscriptionDescriptor(providerID, sessionID, revision string, resource protocol.Resource, trust core.TrustClass) core.CapabilityDescriptor {
	name := strings.TrimSpace(resource.Name)
	if name == "" {
		name = resource.URI
	}
	return core.NormalizeCapabilityDescriptor(core.CapabilityDescriptor{
		ID:            "subscription:" + providerID + ":" + sessionID + ":" + sanitizeCapabilityComponent(name),
		Kind:          core.CapabilityKindSubscription,
		RuntimeFamily: core.CapabilityRuntimeFamilyProvider,
		Name:          name + ".subscription",
		Version:       revision,
		Description:   "MCP resource subscription for " + name,
		Category:      "mcp",
		Tags:          []string{"mcp", "subscription", "resource"},
		Source: core.CapabilitySource{
			ProviderID: providerID,
			Scope:      core.CapabilityScopeProvider,
			SessionID:  sessionID,
		},
		TrustClass:      trust,
		SessionAffinity: sessionID,
		InputSchema: &core.Schema{
			Type: "object",
			Properties: map[string]*core.Schema{
				"action": {
					Type:        "string",
					Description: "subscribe or unsubscribe",
					Enum:        []any{"subscribe", "unsubscribe"},
					Default:     "subscribe",
				},
			},
		},
		Availability: core.AvailabilitySpec{Available: true},
		Annotations: map[string]any{
			"mcp_uri": resource.URI,
		},
	})
}

func sanitizeCapabilityComponent(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" {
		return "resource"
	}
	replacer := strings.NewReplacer(" ", "-", "/", "-", ":", "-", ".", "-", "\\", "-")
	value = replacer.Replace(value)
	for strings.Contains(value, "--") {
		value = strings.ReplaceAll(value, "--", "-")
	}
	return strings.Trim(value, "-")
}

func firstConfiguredNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

var _ RuntimeProvider = (*mcpClientProvider)(nil)
var _ DescribedRuntimeProvider = (*mcpClientProvider)(nil)
var _ SessionManagedProvider = (*mcpClientProvider)(nil)
var _ core.ProviderSnapshotter = (*mcpClientProvider)(nil)
var _ core.ProviderSessionSnapshotter = (*mcpClientProvider)(nil)

var _ RuntimeProvider = (*mcpServerProvider)(nil)
var _ DescribedRuntimeProvider = (*mcpServerProvider)(nil)
var _ SessionManagedProvider = (*mcpServerProvider)(nil)
var _ core.ProviderSnapshotter = (*mcpServerProvider)(nil)
var _ core.ProviderSessionSnapshotter = (*mcpServerProvider)(nil)
