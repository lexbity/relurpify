package server

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"codeburg.org/lexbit/relurpify/framework/core"
	"codeburg.org/lexbit/relurpify/framework/identity"
	rexnexus "codeburg.org/lexbit/relurpify/named/rex/nexus"
	fwfmp "codeburg.org/lexbit/relurpify/relurpnet/fmp"
	fwgateway "codeburg.org/lexbit/relurpify/relurpnet/gateway"
	fwnode "codeburg.org/lexbit/relurpify/relurpnet/node"
	"codeburg.org/lexbit/relurpify/relurpnet/session"
	"github.com/gorilla/websocket"
)

const NodeDisconnectTimeout = 5 * time.Second

type rexRuntimeView interface {
	RuntimeProjection() rexnexus.Projection
	RuntimeDescriptor(context.Context) (core.RuntimeDescriptor, error)
}

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

func HandleGatewayNodeConnection(ctx context.Context, manager *fwnode.Manager, identities identity.Store, mesh *fwfmp.Service, principal fwgateway.ConnectionPrincipal, frame fwgateway.NodeConnectInfo, conn *websocket.Conn, rexRuntime rexRuntimeView) error {
	if manager == nil {
		return fmt.Errorf("node manager unavailable")
	}
	nodeDesc, err := connectedNodeDescriptor(ctx, manager, identities, principal, frame)
	if err != nil {
		return err
	}
	rpcConn := nodeRPCConnForTransport(principal, frame, &websocketRPCConn{conn: conn})
	wsConn := &fwnode.WSConnection{
		Conn:          rpcConn,
		Descriptor:    nodeDesc,
		CapabilitySet: connectedNodeCapabilities(ctx, manager, nodeDesc.ID),
		HealthState: core.NodeHealth{
			Online:     true,
			Foreground: true,
		},
	}
	if mesh != nil && strings.TrimSpace(frame.TransportProfile) != "" {
		wsConn.FrameHandler = meshTransportFrameHandler(mesh, frame, rexRuntime)
	}
	if err := manager.HandleConnect(ctx, wsConn); err != nil {
		return err
	}
	if mesh != nil {
		if err := advertiseConnectedNodeToFMP(ctx, mesh, nodeDesc, frame, rexRuntime); err != nil {
			return err
		}
	}
	defer func() {
		disconnectCtx, cancel := NewNodeDisconnectContext(ctx)
		defer cancel()
		_ = manager.HandleDisconnect(disconnectCtx, nodeDesc.ID, "websocket closed")
	}()
	return wsConn.ReadLoop(ctx)
}

func meshTransportFrameHandler(mesh *fwfmp.Service, connectInfo fwgateway.NodeConnectInfo, rexRuntime rexRuntimeView) func(context.Context, *fwnode.WSConnection, map[string]json.RawMessage) error {
	return func(ctx context.Context, conn *fwnode.WSConnection, frame map[string]json.RawMessage) error {
		if mesh == nil {
			return nil
		}
		var frameType string
		if raw, ok := frame["type"]; ok {
			if err := json.Unmarshal(raw, &frameType); err != nil {
				return err
			}
		}
		switch frameType {
		case "fmp.runtime.register":
			var req struct {
				Type        string                 `json:"type"`
				TrustDomain string                 `json:"trust_domain"`
				Runtime     core.RuntimeDescriptor `json:"runtime"`
				ExpiresAt   time.Time              `json:"expires_at,omitempty"`
				Signature   string                 `json:"signature,omitempty"`
			}
			if err := remarshalNodeFrame(frame, &req); err != nil {
				return err
			}
			if strings.TrimSpace(req.TrustDomain) == "" {
				req.TrustDomain = strings.TrimSpace(connectInfo.TrustDomain)
			}
			req.Runtime.NodeID = conn.Descriptor.ID
			if strings.TrimSpace(req.Runtime.RuntimeID) == "" {
				req.Runtime.RuntimeID = fallbackNodeRuntimeID(conn.Descriptor, connectInfo)
			}
			if strings.TrimSpace(req.Runtime.TrustDomain) == "" {
				req.Runtime.TrustDomain = req.TrustDomain
			}
			if strings.TrimSpace(req.Runtime.RuntimeVersion) == "" {
				req.Runtime.RuntimeVersion = fallbackRuntimeVersion(connectInfo)
			}
			if strings.TrimSpace(req.Runtime.CompatibilityClass) == "" {
				req.Runtime.CompatibilityClass = fallbackCompatibilityClass(connectInfo)
			}
			if len(req.Runtime.SupportedContextClasses) == 0 {
				req.Runtime.SupportedContextClasses = append([]string(nil), connectInfo.SupportedContextClasses...)
			}
			if err := mesh.RegisterRuntime(ctx, fwfmp.RuntimeRegistrationRequest{
				TrustDomain: req.TrustDomain,
				Node:        conn.Descriptor,
				Runtime:     req.Runtime,
				ExpiresAt:   req.ExpiresAt,
				Signature:   req.Signature,
			}); err != nil {
				return conn.SendJSON(map[string]any{"type": "fmp.runtime.error", "operation": "register", "error": err.Error()})
			}
			return conn.SendJSON(map[string]any{"type": "fmp.runtime.registered", "runtime_id": req.Runtime.RuntimeID, "trust_domain": req.TrustDomain})
		case "fmp.export.advertise":
			var req struct {
				Type        string                `json:"type"`
				TrustDomain string                `json:"trust_domain"`
				RuntimeID   string                `json:"runtime_id"`
				Export      core.ExportDescriptor `json:"export"`
				ExpiresAt   time.Time             `json:"expires_at,omitempty"`
				Signature   string                `json:"signature,omitempty"`
			}
			if err := remarshalNodeFrame(frame, &req); err != nil {
				return err
			}
			if strings.TrimSpace(req.TrustDomain) == "" {
				req.TrustDomain = strings.TrimSpace(connectInfo.TrustDomain)
			}
			if strings.TrimSpace(req.RuntimeID) == "" {
				req.RuntimeID = fallbackNodeRuntimeID(conn.Descriptor, connectInfo)
			}
			exportAd := core.ExportAdvertisement{
				TrustDomain: req.TrustDomain,
				RuntimeID:   req.RuntimeID,
				NodeID:      conn.Descriptor.ID,
				Export:      req.Export,
				ExpiresAt:   req.ExpiresAt,
				Signature:   req.Signature,
			}

			// Phase 7.3: Include DR metadata from runtime projection for federation health visibility
			if rexRuntime != nil {
				projection := rexRuntime.RuntimeProjection()
				exportAd.FailoverReady = projection.FailoverReady
				exportAd.RecoveryState = projection.RecoveryState
				exportAd.RuntimeVersion = projection.RuntimeVersion
				exportAd.LastCheckpoint = projection.LastCheckpoint
			}

			if err := mesh.AdvertiseExport(ctx, exportAd); err != nil {
				return conn.SendJSON(map[string]any{"type": "fmp.export.error", "operation": "advertise", "error": err.Error()})
			}
			return conn.SendJSON(map[string]any{"type": "fmp.export.advertised", "runtime_id": req.RuntimeID, "export_name": req.Export.ExportName, "trust_domain": req.TrustDomain})
		case "fmp.chunk.open":
			var req struct {
				Type      string               `json:"type"`
				LineageID string               `json:"lineage_id"`
				Manifest  core.ContextManifest `json:"manifest"`
				Sealed    core.SealedContext   `json:"sealed"`
			}
			if err := remarshalNodeFrame(frame, &req); err != nil {
				return err
			}
			session, err := mesh.OpenChunkTransfer(ctx, req.LineageID, req.Manifest, req.Sealed)
			if err != nil {
				return conn.SendJSON(map[string]any{"type": "fmp.chunk.error", "operation": "open", "error": err.Error()})
			}
			return conn.SendJSON(map[string]any{"type": "fmp.chunk.opened", "session": session})
		case "fmp.chunk.read":
			var req struct {
				Type       string `json:"type"`
				TransferID string `json:"transfer_id"`
				MaxChunks  int    `json:"max_chunks,omitempty"`
			}
			if err := remarshalNodeFrame(frame, &req); err != nil {
				return err
			}
			chunks, control, err := mesh.ReadChunkTransfer(ctx, req.TransferID, req.MaxChunks)
			if err != nil {
				return conn.SendJSON(map[string]any{"type": "fmp.chunk.error", "operation": "read", "transfer_id": req.TransferID, "error": err.Error()})
			}
			return conn.SendJSON(map[string]any{"type": "fmp.chunk.data", "transfer_id": req.TransferID, "chunks": chunks, "flow_control": control})
		case "fmp.chunk.ack":
			var req struct {
				Type       string `json:"type"`
				LineageID  string `json:"lineage_id"`
				TransferID string `json:"transfer_id"`
				AckedIndex int    `json:"acked_index"`
				WindowSize int    `json:"window_size,omitempty"`
			}
			if err := remarshalNodeFrame(frame, &req); err != nil {
				return err
			}
			control, err := mesh.AckChunkTransfer(ctx, req.LineageID, fwfmp.ChunkAck{
				TransferID: req.TransferID,
				AckedIndex: req.AckedIndex,
				WindowSize: req.WindowSize,
			})
			if err != nil {
				return conn.SendJSON(map[string]any{"type": "fmp.chunk.error", "operation": "ack", "transfer_id": req.TransferID, "error": err.Error()})
			}
			return conn.SendJSON(map[string]any{"type": "fmp.chunk.acked", "transfer_id": req.TransferID, "flow_control": control})
		case "fmp.chunk.cancel":
			var req struct {
				Type       string `json:"type"`
				LineageID  string `json:"lineage_id"`
				TransferID string `json:"transfer_id"`
				Reason     string `json:"reason,omitempty"`
			}
			if err := remarshalNodeFrame(frame, &req); err != nil {
				return err
			}
			if err := mesh.CancelChunkTransfer(ctx, req.LineageID, req.TransferID, req.Reason); err != nil {
				return conn.SendJSON(map[string]any{"type": "fmp.chunk.error", "operation": "cancel", "transfer_id": req.TransferID, "error": err.Error()})
			}
			return conn.SendJSON(map[string]any{"type": "fmp.chunk.cancelled", "transfer_id": req.TransferID, "reason": req.Reason})
		case "fmp.resume.execute":
			var req struct {
				Type        string                `json:"type"`
				Actor       core.SubjectRef       `json:"actor"`
				RuntimeID   string                `json:"runtime_id,omitempty"`
				Offer       core.HandoffOffer     `json:"offer"`
				Destination core.ExportDescriptor `json:"destination"`
				Manifest    core.ContextManifest  `json:"manifest"`
				Sealed      core.SealedContext    `json:"sealed"`
			}
			if err := remarshalNodeFrame(frame, &req); err != nil {
				return err
			}
			runtimeID := strings.TrimSpace(req.RuntimeID)
			if runtimeID == "" {
				runtimeID = fallbackNodeRuntimeID(conn.Descriptor, connectInfo)
			}
			executed, commit, authorized, refusal, err := mesh.ResumeHandoffForNode(ctx, req.Offer, req.Destination, runtimeID, conn.Descriptor.ID, req.Actor, req.Manifest, req.Sealed)
			if err != nil {
				return conn.SendJSON(map[string]any{"type": "fmp.resume.error", "operation": "execute", "error": err.Error()})
			}
			if refusal != nil {
				return conn.SendJSON(map[string]any{"type": "fmp.resume.refused", "operation": "execute", "refusal": refusal})
			}
			return conn.SendJSON(map[string]any{
				"type":       "fmp.resume.executed",
				"authorized": authorized,
				"executed":   executed,
				"commit":     commit,
			})
		default:
			return nil
		}
	}
}

func remarshalNodeFrame(input any, out any) error {
	data, err := json.Marshal(input)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, out)
}

func ChunkTransportFrameHandlerForTest(mesh *fwfmp.Service) func(context.Context, *fwnode.WSConnection, map[string]json.RawMessage) error {
	return meshTransportFrameHandler(mesh, fwgateway.NodeConnectInfo{}, nil)
}

func MeshTransportFrameHandlerForTest(mesh *fwfmp.Service, connectInfo fwgateway.NodeConnectInfo) func(context.Context, *fwnode.WSConnection, map[string]json.RawMessage) error {
	return meshTransportFrameHandler(mesh, connectInfo, nil)
}

func MeshTransportFrameHandlerWithRuntimeForTest(mesh *fwfmp.Service, connectInfo fwgateway.NodeConnectInfo, rexRuntime rexRuntimeView) func(context.Context, *fwnode.WSConnection, map[string]json.RawMessage) error {
	return meshTransportFrameHandler(mesh, connectInfo, rexRuntime)
}

func nodeRPCConnForTransport(principal fwgateway.ConnectionPrincipal, frame fwgateway.NodeConnectInfo, base interface {
	WriteJSON(v any) error
	ReadJSON(v any) error
	Close() error
}) interface {
	WriteJSON(v any) error
	ReadJSON(v any) error
	Close() error
} {
	if strings.TrimSpace(frame.TransportProfile) == "" {
		return base
	}
	sessionID := frame.NodeID
	if principal.Principal != nil && strings.TrimSpace(principal.Principal.SessionID) != "" {
		sessionID = strings.TrimSpace(principal.Principal.SessionID)
	}
	if strings.TrimSpace(sessionID) == "" {
		sessionID = frame.RuntimeID
	}
	return fwnode.NewFramedRPCConn(base, sessionID)
}

func NodeRPCConnForTransportForTest(principal fwgateway.ConnectionPrincipal, frame fwgateway.NodeConnectInfo, base interface {
	WriteJSON(v any) error
	ReadJSON(v any) error
	Close() error
}) interface {
	WriteJSON(v any) error
	ReadJSON(v any) error
	Close() error
} {
	return nodeRPCConnForTransport(principal, frame, base)
}

func AdvertiseConnectedNodeToFMP(ctx context.Context, mesh *fwfmp.Service, nodeDesc core.NodeDescriptor, frame fwgateway.NodeConnectInfo) error {
	return advertiseConnectedNodeToFMP(ctx, mesh, nodeDesc, frame, nil)
}

func advertiseConnectedNodeToFMP(ctx context.Context, mesh *fwfmp.Service, nodeDesc core.NodeDescriptor, frame fwgateway.NodeConnectInfo, rexRuntime rexRuntimeView) error {
	if mesh == nil {
		return nil
	}
	runtime := advertisedRuntimeDescriptor(nodeDesc, frame, rexRuntime)
	trustDomain := runtime.TrustDomain
	if trustDomain == "" {
		trustDomain = "local"
		runtime.TrustDomain = trustDomain
	}
	expiresAt := time.Now().UTC().Add(5 * time.Minute)
	signature := runtimeRegistrationSignatureFromValues(nodeDesc.ID, runtime.RuntimeID, runtime.RuntimeVersion, strings.TrimSpace(frame.PeerKeyID), strings.TrimSpace(frame.TransportProfile))
	runtime.ExpiresAt = expiresAt
	runtime.Signature = signature
	return mesh.RegisterRuntime(ctx, fwfmp.RuntimeRegistrationRequest{
		TrustDomain: trustDomain,
		Node:        nodeDesc,
		ExpiresAt:   expiresAt,
		Signature:   signature,
		Runtime:     runtime,
	})
}

func advertisedRuntimeDescriptor(nodeDesc core.NodeDescriptor, frame fwgateway.NodeConnectInfo, rexRuntime rexRuntimeView) core.RuntimeDescriptor {
	runtime := core.RuntimeDescriptor{
		RuntimeID:               fallbackNodeRuntimeID(nodeDesc, frame),
		NodeID:                  nodeDesc.ID,
		TrustDomain:             strings.TrimSpace(frame.TrustDomain),
		RuntimeVersion:          fallbackRuntimeVersion(frame),
		SupportedContextClasses: append([]string(nil), frame.SupportedContextClasses...),
		CompatibilityClass:      fallbackCompatibilityClass(frame),
		AttestationProfile:      "nexus.node_enrollment.v1",
		AttestationClaims: map[string]string{
			"node_id":         nodeDesc.ID,
			"tenant_id":       nodeDesc.TenantID,
			"trust_class":     string(nodeDesc.TrustClass),
			"peer_key_id":     strings.TrimSpace(frame.PeerKeyID),
			"transport":       strings.TrimSpace(frame.TransportProfile),
			"runtime_version": fallbackRuntimeVersion(frame),
		},
	}
	if rexRuntime != nil {
		if descriptor, err := rexRuntime.RuntimeDescriptor(context.Background()); err == nil {
			if strings.TrimSpace(descriptor.RuntimeID) != "" {
				runtime.RuntimeID = strings.TrimSpace(descriptor.RuntimeID)
			}
			if strings.TrimSpace(descriptor.TrustDomain) != "" {
				runtime.TrustDomain = strings.TrimSpace(descriptor.TrustDomain)
			}
			if strings.TrimSpace(descriptor.RuntimeVersion) != "" {
				runtime.RuntimeVersion = strings.TrimSpace(descriptor.RuntimeVersion)
			}
			if len(descriptor.SupportedContextClasses) > 0 {
				runtime.SupportedContextClasses = append([]string(nil), descriptor.SupportedContextClasses...)
			}
			if strings.TrimSpace(descriptor.CompatibilityClass) != "" {
				runtime.CompatibilityClass = strings.TrimSpace(descriptor.CompatibilityClass)
			}
			runtime.AttestationClaims["runtime_version"] = runtime.RuntimeVersion
		}
	}
	return runtime
}

func fallbackNodeRuntimeID(nodeDesc core.NodeDescriptor, frame fwgateway.NodeConnectInfo) string {
	if strings.TrimSpace(frame.RuntimeID) != "" {
		return strings.TrimSpace(frame.RuntimeID)
	}
	return nodeDesc.ID + ":default"
}

func fallbackRuntimeVersion(frame fwgateway.NodeConnectInfo) string {
	if strings.TrimSpace(frame.RuntimeVersion) != "" {
		return strings.TrimSpace(frame.RuntimeVersion)
	}
	return "0.0.0"
}

func fallbackCompatibilityClass(frame fwgateway.NodeConnectInfo) string {
	if strings.TrimSpace(frame.CompatibilityClass) != "" {
		return strings.TrimSpace(frame.CompatibilityClass)
	}
	return "default"
}

func runtimeRegistrationSignature(nodeDesc core.NodeDescriptor, frame fwgateway.NodeConnectInfo) string {
	return runtimeRegistrationSignatureFromValues(nodeDesc.ID, strings.TrimSpace(frame.RuntimeID), strings.TrimSpace(frame.RuntimeVersion), strings.TrimSpace(frame.PeerKeyID), strings.TrimSpace(frame.TransportProfile))
}

func runtimeRegistrationSignatureFromValues(nodeID, runtimeID, runtimeVersion, peerKeyID, transportProfile string) string {
	parts := []string{
		"nexus-register",
		nodeID,
		runtimeID,
		runtimeVersion,
		peerKeyID,
		transportProfile,
	}
	return strings.Join(parts, ":")
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

func AdvertiseConnectedNodeToFMPWithRuntimeForTest(ctx context.Context, mesh *fwfmp.Service, nodeDesc core.NodeDescriptor, frame fwgateway.NodeConnectInfo, rexRuntime rexRuntimeView) error {
	return advertiseConnectedNodeToFMP(ctx, mesh, nodeDesc, frame, rexRuntime)
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
