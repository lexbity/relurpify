package server

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/lexcodex/relurpify/framework/core"
	fwgateway "github.com/lexcodex/relurpify/framework/middleware/gateway"
	fwnode "github.com/lexcodex/relurpify/framework/middleware/node"
	"github.com/lexcodex/relurpify/framework/middleware/session"
)

const NodeDisconnectTimeout = 5 * time.Second

type websocketRPCConn struct {
	conn *websocket.Conn
	mu   sync.Mutex
}

func (c *websocketRPCConn) WriteJSON(v any) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.conn.WriteJSON(v)
}

func (c *websocketRPCConn) ReadJSON(v any) error {
	return c.conn.ReadJSON(v)
}

func (c *websocketRPCConn) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.conn.Close()
}

func HandleGatewayNodeConnection(ctx context.Context, manager *fwnode.Manager, principal fwgateway.ConnectionPrincipal, frame fwgateway.NodeConnectInfo, conn *websocket.Conn) error {
	if manager == nil {
		return fmt.Errorf("node manager unavailable")
	}
	nodeDesc := core.NodeDescriptor{
		ID:         frame.NodeID,
		TenantID:   principal.Actor.TenantID,
		Name:       frame.NodeName,
		Platform:   core.NodePlatform(frame.NodePlatform),
		TrustClass: core.TrustClass(frame.TrustClass),
	}
	if nodeDesc.ID == "" {
		nodeDesc.ID = "node-session"
	}
	if nodeDesc.Name == "" {
		nodeDesc.Name = nodeDesc.ID
	}
	if nodeDesc.Platform == "" {
		nodeDesc.Platform = core.NodePlatformHeadless
	}
	if nodeDesc.TrustClass == "" {
		nodeDesc.TrustClass = core.TrustClassWorkspaceTrusted
	}
	if principal.Actor.ID != "" {
		nodeDesc.Owner = core.SubjectRef{
			TenantID: principal.Actor.TenantID,
			Kind:     principal.Actor.SubjectKind,
			ID:       principal.Actor.ID,
		}
	}
	wsConn := &fwnode.WSConnection{
		Conn:          &websocketRPCConn{conn: conn},
		Descriptor:    nodeDesc,
		CapabilitySet: append([]core.CapabilityDescriptor(nil), frame.Capabilities...),
		HealthState: core.NodeHealth{
			Online:     true,
			Foreground: true,
		},
	}
	if err := manager.HandleConnect(ctx, wsConn); err != nil {
		return err
	}
	defer func() {
		disconnectCtx, cancel := NewNodeDisconnectContext(ctx)
		defer cancel()
		_ = manager.HandleDisconnect(disconnectCtx, nodeDesc.ID, "websocket closed")
	}()
	return wsConn.ReadLoop(ctx)
}

func NewNodeDisconnectContext(ctx context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.WithoutCancel(ctx), NodeDisconnectTimeout)
}

func ListNodeCapabilities(manager *fwnode.Manager, principal fwgateway.ConnectionPrincipal) []core.CapabilityDescriptor {
	if manager == nil {
		return nil
	}
	return manager.ListCapabilitiesForTenant(NormalizeTenantID(principal.Actor.TenantID))
}

func InvokeNodeCapability(ctx context.Context, manager *fwnode.Manager, principal fwgateway.ConnectionPrincipal, capabilityID string, args map[string]any) (*core.CapabilityExecutionResult, error) {
	if manager == nil {
		return nil, fmt.Errorf("node manager unavailable")
	}
	return manager.InvokeCapabilityForTenant(ctx, NormalizeTenantID(principal.Actor.TenantID), capabilityID, args)
}

func InvokeAuthorizedNodeCapability(ctx context.Context, router session.Router, store session.Store, manager *fwnode.Manager, principal fwgateway.ConnectionPrincipal, sessionKey, capabilityID string, args map[string]any) (*core.CapabilityExecutionResult, error) {
	if store == nil {
		return nil, fmt.Errorf("session store unavailable")
	}
	boundary, err := store.GetBoundaryBySessionID(ctx, sessionKey)
	if err != nil {
		return nil, err
	}
	if router == nil {
		return nil, fmt.Errorf("session router unavailable")
	}
	if err := router.Authorize(ctx, session.AuthorizationRequest{
		Actor:         principal.Actor,
		Authenticated: principal.Authenticated,
		Operation:     core.SessionOperationInvoke,
		Boundary:      boundary,
	}); err != nil {
		return nil, err
	}
	return InvokeNodeCapability(ctx, manager, principal, capabilityID, args)
}

func NormalizeTenantID(tenantID string) string {
	if tenantID == "" {
		return DefaultTenantID
	}
	return tenantID
}
