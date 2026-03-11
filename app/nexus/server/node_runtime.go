package server

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/framework/identity"
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

func HandleGatewayNodeConnection(ctx context.Context, manager *fwnode.Manager, identities identity.Store, principal fwgateway.ConnectionPrincipal, frame fwgateway.NodeConnectInfo, conn *websocket.Conn) error {
	if manager == nil {
		return fmt.Errorf("node manager unavailable")
	}
	nodeDesc, err := connectedNodeDescriptor(ctx, manager, identities, principal, frame)
	if err != nil {
		return err
	}
	wsConn := &fwnode.WSConnection{
		Conn:          &websocketRPCConn{conn: conn},
		Descriptor:    nodeDesc,
		CapabilitySet: connectedNodeCapabilities(ctx, manager, nodeDesc.ID),
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

func connectedNodeDescriptor(ctx context.Context, manager *fwnode.Manager, identities identity.Store, principal fwgateway.ConnectionPrincipal, frame fwgateway.NodeConnectInfo) (core.NodeDescriptor, error) {
	tenantID := NormalizeTenantID(principal.Actor.TenantID)
	nodeID := frame.NodeID
	if nodeID == "" {
		nodeID = principal.Actor.ID
	}
	if nodeID == "" {
		return core.NodeDescriptor{}, fmt.Errorf("node id required")
	}

	var enrollment *core.NodeEnrollment
	var err error
	if identities != nil {
		enrollment, err = identities.GetNodeEnrollment(ctx, tenantID, nodeID)
		if err != nil {
			return core.NodeDescriptor{}, err
		}
	}
	if enrollment == nil {
		return core.NodeDescriptor{}, fmt.Errorf("node enrollment not found")
	}

	nodeDesc := core.NodeDescriptor{
		ID:         enrollment.NodeID,
		TenantID:   enrollment.TenantID,
		Name:       enrollment.NodeID,
		Platform:   core.NodePlatformHeadless,
		TrustClass: enrollment.TrustClass,
		PairedAt:   enrollment.PairedAt,
		Owner:      enrollment.Owner,
	}
	if manager != nil && manager.Store != nil {
		storedNode, err := manager.Store.GetNode(ctx, enrollment.NodeID)
		if err != nil {
			return core.NodeDescriptor{}, err
		}
		if storedNode != nil {
			if storedNode.Name != "" {
				nodeDesc.Name = storedNode.Name
			}
			if storedNode.Platform != "" {
				nodeDesc.Platform = storedNode.Platform
			}
			if len(storedNode.Tags) > 0 {
				nodeDesc.Tags = copyNodeTags(storedNode.Tags)
			}
		}
	}
	return nodeDesc, nil
}

func ConnectedNodeDescriptorForTest(ctx context.Context, manager *fwnode.Manager, identities identity.Store, principal fwgateway.ConnectionPrincipal, frame fwgateway.NodeConnectInfo) (core.NodeDescriptor, error) {
	return connectedNodeDescriptor(ctx, manager, identities, principal, frame)
}

func ConnectedNodeCapabilitiesForTest(ctx context.Context, manager *fwnode.Manager, nodeID string) []core.CapabilityDescriptor {
	return connectedNodeCapabilities(ctx, manager, nodeID)
}

func copyNodeTags(in map[string]string) map[string]string {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]string, len(in))
	for key, value := range in {
		out[key] = value
	}
	return out
}

func connectedNodeCapabilities(ctx context.Context, manager *fwnode.Manager, nodeID string) []core.CapabilityDescriptor {
	if manager == nil || manager.Store == nil || nodeID == "" {
		return nil
	}
	storedNode, err := manager.Store.GetNode(ctx, nodeID)
	if err != nil || storedNode == nil {
		return nil
	}
	if len(storedNode.ApprovedCapabilities) == 0 {
		return nil
	}
	out := make([]core.CapabilityDescriptor, len(storedNode.ApprovedCapabilities))
	copy(out, storedNode.ApprovedCapabilities)
	return out
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
