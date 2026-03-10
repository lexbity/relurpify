package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gorilla/websocket"
	"github.com/lexcodex/relurpify/app/nexus/adapters/discord"
	"github.com/lexcodex/relurpify/app/nexus/adapters/telegram"
	"github.com/lexcodex/relurpify/app/nexus/adapters/webchat"
	nexusadmin "github.com/lexcodex/relurpify/app/nexus/admin"
	nexuscfg "github.com/lexcodex/relurpify/app/nexus/config"
	nexusgateway "github.com/lexcodex/relurpify/app/nexus/gateway"
	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/framework/event"
	"github.com/lexcodex/relurpify/framework/identity"
	"github.com/lexcodex/relurpify/framework/middleware/channel"
	fwgateway "github.com/lexcodex/relurpify/framework/middleware/gateway"
	mcpprotocol "github.com/lexcodex/relurpify/framework/middleware/mcp/protocol"
	mcpserver "github.com/lexcodex/relurpify/framework/middleware/mcp/server"
	fwnode "github.com/lexcodex/relurpify/framework/middleware/node"
	"github.com/lexcodex/relurpify/framework/middleware/session"
)

const DefaultTenantID = "local"

// NexusApp wires the Nexus gateway stack around already-open stores so tests can
// construct the full handler without filesystem config or bound ports.
type NexusApp struct {
	EventLog      event.Log
	SessionStore  session.Store
	IdentityStore identity.Store
	NodeStore     fwnode.NodeStore
	TokenStore    nexusadmin.TokenStore
	PolicyStore   nexusadmin.PolicyRuleStore
	Config        nexuscfg.Config
	Partition     string

	ChannelAdapters []channel.Adapter
	WebchatAdapter  *webchat.Adapter

	NodeManager          *fwnode.Manager
	ChannelManager       *channel.Manager
	StateMaterializer    *nexusgateway.StateMaterializer
	StartedAt            time.Time
	PrincipalResolver    func(context.Context, string) (fwgateway.ConnectionPrincipal, error)
	VerifyNodeConnection func(context.Context, identity.Store, fwgateway.ConnectionPrincipal, fwgateway.NodeConnectInfo, *websocket.Conn) error
}

func (a *NexusApp) Handler(ctx context.Context) (http.Handler, error) {
	if a == nil {
		return nil, fmt.Errorf("nexus app required")
	}
	if a.EventLog == nil {
		return nil, fmt.Errorf("event log required")
	}
	if a.SessionStore == nil {
		return nil, fmt.Errorf("session store required")
	}
	if a.IdentityStore == nil {
		return nil, fmt.Errorf("identity store required")
	}
	if a.NodeStore == nil {
		return nil, fmt.Errorf("node store required")
	}

	nodeManager := a.NodeManager
	if nodeManager == nil {
		nodeManager = &fwnode.Manager{
			Store: a.NodeStore,
			Log:   a.EventLog,
			Pairing: fwnode.PairingConfig{
				AutoApproveLocal: a.Config.Nodes.AutoApproveLocal,
				PairingCodeTTL:   a.Config.Nodes.PairingCodeTTL,
			},
		}
	}
	a.NodeManager = nodeManager
	router := &session.DefaultRouter{
		Store: a.SessionStore,
		Log:   a.EventLog,
		Scope: core.SessionScopePerChannelPeer,
	}
	sink := &session.SessionSink{
		Log:       a.EventLog,
		Partition: a.partition(),
		Router:    router,
		Resolver:  identity.StoreResolver{Store: a.IdentityStore, DefaultTenantID: DefaultTenantID},
	}
	manager := a.ChannelManager
	if manager == nil {
		manager = channel.NewManager(a.EventLog, sink)
	}
	a.ChannelManager = manager

	webchatAdapter := a.WebchatAdapter
	if webchatAdapter == nil {
		webchatAdapter = &webchat.Adapter{}
	}
	if len(a.ChannelAdapters) > 0 {
		for _, adapter := range a.ChannelAdapters {
			if err := manager.Register(adapter); err != nil {
				return nil, err
			}
		}
	} else {
		if err := registerConfiguredAdapters(manager, a.Config, webchatAdapter); err != nil {
			return nil, err
		}
	}
	if err := manager.Start(ctx, channelConfigs(a.Config)); err != nil {
		return nil, err
	}
	go func() {
		<-ctx.Done()
		_ = manager.Stop(context.Background())
	}()

	srv := &fwgateway.Server{
		Log:       a.EventLog,
		Partition: a.partition(),
		ListCapabilitiesForPrincipal: func(principal fwgateway.ConnectionPrincipal) []core.CapabilityDescriptor {
			return ListNodeCapabilities(nodeManager, principal)
		},
		PrincipalResolver: a.PrincipalResolver,
		VerifyNodeConnection: func(ctx context.Context, principal fwgateway.ConnectionPrincipal, info fwgateway.NodeConnectInfo, conn *websocket.Conn) error {
			if a.VerifyNodeConnection == nil {
				return nil
			}
			return a.VerifyNodeConnection(ctx, a.IdentityStore, principal, info, conn)
		},
		SessionTenantResolver: func(ctx context.Context, sessionID string) (string, error) {
			boundary, err := a.SessionStore.GetBoundaryBySessionID(ctx, sessionID)
			if err != nil || boundary == nil {
				return "", err
			}
			return boundary.TenantID, nil
		},
		InvokeCapability: func(ctx context.Context, principal fwgateway.ConnectionPrincipal, sessionKey, capabilityID string, args map[string]any) (*core.CapabilityExecutionResult, error) {
			return InvokeAuthorizedNodeCapability(ctx, router, a.SessionStore, nodeManager, principal, sessionKey, capabilityID, args)
		},
		HandleOutboundMessage: func(ctx context.Context, principal fwgateway.ConnectionPrincipal, sessionKey, content string) error {
			boundary, err := a.SessionStore.GetBoundaryBySessionID(ctx, sessionKey)
			if err != nil {
				return err
			}
			if err := router.Authorize(ctx, session.AuthorizationRequest{
				Actor:         principal.Actor,
				Authenticated: principal.Authenticated,
				Operation:     core.SessionOperationSend,
				Boundary:      boundary,
			}); err != nil {
				return err
			}
			msg, err := session.OutboundMessageForSession(boundary, content)
			if err != nil {
				return err
			}
			return manager.Send(ctx, msg)
		},
		HandleNodeConnection: func(ctx context.Context, principal fwgateway.ConnectionPrincipal, info fwgateway.NodeConnectInfo, conn *websocket.Conn) error {
			return HandleGatewayNodeConnection(ctx, nodeManager, principal, info, conn)
		},
		AdminSnapshot: func(ctx context.Context, principal fwgateway.ConnectionPrincipal) (map[string]any, error) {
			if a.StateMaterializer == nil {
				return map[string]any{}, nil
			}
			return map[string]any{
				"last_seq":         a.StateMaterializer.State().LastSeq,
				"active_sessions":  a.StateMaterializer.State().ActiveSessions,
				"channel_activity": a.StateMaterializer.State().ChannelActivity,
				"event_counts":     a.StateMaterializer.State().EventTypeCounts,
			}, nil
		},
	}
	if err := srv.Start(ctx); err != nil {
		return nil, err
	}

	adminSvc := nexusadmin.NewService(nexusadmin.ServiceConfig{
		Nodes:        a.NodeStore,
		NodeManager:  nodeManager,
		Sessions:     a.SessionStore,
		Identities:   a.IdentityStore,
		Tokens:       a.TokenStore,
		Policies:     a.PolicyStore,
		Events:       a.EventLog,
		Materializer: a.StateMaterializer,
		Channels:     manager,
		Partition:    a.partition(),
		Config:       a.Config,
		StartedAt:    a.StartedAt,
	})
	adminExporter := nexusadmin.NewMCPExporter(adminSvc)
	adminMCPSvc := mcpserver.New(
		mcpprotocol.PeerInfo{Name: "nexus-admin", Version: nexusadmin.APIVersionV1Alpha1},
		adminExporter,
		mcpserver.Hooks{},
	)

	mux := http.NewServeMux()
	mux.Handle(a.gatewayPath(), srv.Handler())
	mux.Handle("/admin/mcp", adminAuthMiddleware(a.PrincipalResolver, http.HandlerFunc(adminMCPSvc.ServeHTTP)))
	if len(a.ChannelAdapters) == 0 {
		mux.Handle("/webchat", webchatAdapter.Handler())
	}
	return mux, nil
}

func (a *NexusApp) partition() string {
	if a == nil || a.Partition == "" {
		return "local"
	}
	return a.Partition
}

func (a *NexusApp) gatewayPath() string {
	if a == nil || a.Config.Gateway.Path == "" {
		return "/gateway"
	}
	return a.Config.Gateway.Path
}

func channelConfigs(cfg nexuscfg.Config) map[string]json.RawMessage {
	if len(cfg.Channels) == 0 {
		return nil
	}
	out := make(map[string]json.RawMessage, len(cfg.Channels))
	for name, config := range cfg.Channels {
		data, err := json.Marshal(config)
		if err != nil {
			continue
		}
		out[name] = data
	}
	return out
}

func registerConfiguredAdapters(manager *channel.Manager, cfg nexuscfg.Config, webchatAdapter *webchat.Adapter) error {
	if enabled(cfg.Channels, "webchat", true) {
		if err := manager.Register(webchatAdapter); err != nil {
			return err
		}
	}
	if enabled(cfg.Channels, "telegram", false) {
		if err := manager.Register(&telegram.Adapter{}); err != nil {
			return err
		}
	}
	if enabled(cfg.Channels, "discord", false) {
		if err := manager.Register(&discord.Adapter{}); err != nil {
			return err
		}
	}
	return nil
}

func enabled(channels map[string]map[string]any, name string, defaultValue bool) bool {
	config, ok := channels[name]
	if !ok {
		return defaultValue
	}
	value, ok := config["enabled"]
	if !ok {
		return defaultValue
	}
	enabled, ok := value.(bool)
	if !ok {
		return defaultValue
	}
	return enabled
}

func adminAuthMiddleware(
	resolver func(context.Context, string) (fwgateway.ConnectionPrincipal, error),
	next http.Handler,
) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if resolver == nil {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		principal, err := resolver(r.Context(), bearerToken(r.Header.Get("Authorization")))
		if err != nil || principal.Principal == nil || !isAdminOrOperator(*principal.Principal) {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r.WithContext(nexusadmin.WithPrincipal(r.Context(), *principal.Principal)))
	})
}

func bearerToken(header string) string {
	const prefix = "Bearer "
	if strings.HasPrefix(header, prefix) {
		return strings.TrimSpace(strings.TrimPrefix(header, prefix))
	}
	return ""
}

func isAdminOrOperator(principal core.AuthenticatedPrincipal) bool {
	for _, scope := range principal.Scopes {
		switch strings.ToLower(strings.TrimSpace(scope)) {
		case "gateway:admin", "nexus:admin", "nexus:operator", "admin", "operator":
			return true
		}
	}
	return false
}
