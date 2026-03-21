package gateway

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/framework/event"
)

const (
	// broadcastQueueDepth is the number of frames that can be buffered per
	// connected client before it is considered too slow and disconnected.
	broadcastQueueDepth = 64
	// broadcastWriteTimeout is the per-write deadline applied to each client.
	broadcastWriteTimeout = 10 * time.Second
	// maxInboundMessageSize caps inbound WebSocket frame size to prevent
	// memory exhaustion from oversized payloads. 4 MiB is generous for any
	// legitimate capability invocation or outbound message.
	maxInboundMessageSize = 4 * 1024 * 1024 // 4 MiB
)

// broadcastClient couples a WebSocket connection with a bounded send queue and
// a dedicated write goroutine so that one slow client never blocks others.
type broadcastClient struct {
	conn      *websocket.Conn
	queue     chan any
	principal ConnectionPrincipal
	mu        sync.Mutex
	done      bool
}

// send enqueues a frame for the client. Returns false and closes the queue if
// the buffer is full, signalling the write goroutine to exit.
func (bc *broadcastClient) send(frame any) bool {
	bc.mu.Lock()
	defer bc.mu.Unlock()
	if bc.done {
		return false
	}
	select {
	case bc.queue <- frame:
		return true
	default:
		bc.closeQueueLocked()
		return false
	}
}

func (bc *broadcastClient) closeQueue() {
	bc.mu.Lock()
	defer bc.mu.Unlock()
	bc.closeQueueLocked()
}

func (bc *broadcastClient) closeQueueLocked() {
	if bc.done {
		return
	}
	bc.done = true
	close(bc.queue)
}

type Server struct {
	Log                          event.Log
	Partition                    string
	Upgrader                     websocket.Upgrader
	FMPTransportPolicy           *FMPTransportPolicy
	Capabilities                 []core.CapabilityDescriptor
	ListCapabilities             func() []core.CapabilityDescriptor
	ListCapabilitiesForPrincipal func(principal ConnectionPrincipal) []core.CapabilityDescriptor
	InvokeCapability             func(ctx context.Context, principal ConnectionPrincipal, sessionKey, capabilityID string, args map[string]any) (*core.CapabilityExecutionResult, error)
	HandleOutboundMessage        func(ctx context.Context, principal ConnectionPrincipal, sessionKey, content string) error
	VerifyNodeConnection         func(ctx context.Context, principal ConnectionPrincipal, info NodeConnectInfo, conn *websocket.Conn) error
	HandleNodeConnection         func(ctx context.Context, principal ConnectionPrincipal, info NodeConnectInfo, conn *websocket.Conn) error
	AdminSnapshot                func(ctx context.Context, principal ConnectionPrincipal) (map[string]any, error)
	// PrincipalResolver resolves a bearer token into the authenticated gateway
	// principal. When set, gateway identity comes from the resolved principal
	// rather than any user-supplied connect-frame actor fields.
	PrincipalResolver func(ctx context.Context, token string) (ConnectionPrincipal, error)
	// SessionTenantResolver resolves opaque session IDs to their owning tenant so
	// scoped event delivery can filter session-derived events.
	SessionTenantResolver func(ctx context.Context, sessionID string) (string, error)
	// SessionEventAuthorizer decides whether a principal may observe events
	// associated with a specific session. Tenant-wide admins may bypass this via
	// canDeliverEvent, but runtime principals are gated through it.
	SessionEventAuthorizer func(ctx context.Context, principal ConnectionPrincipal, sessionID string) (bool, error)

	mu      sync.RWMutex
	clients map[*websocket.Conn]*broadcastClient
	once    sync.Once
}

type connectFrame struct {
	Type                    string                      `json:"type"`
	Version                 string                      `json:"version"`
	Role                    string                      `json:"role"`
	FeedScope               string                      `json:"feed_scope,omitempty"`
	ActorID                 string                      `json:"actor_id,omitempty"`
	LastSeenSeq             uint64                      `json:"last_seen_seq"`
	NodeID                  string                      `json:"node_id,omitempty"`
	NodeName                string                      `json:"node_name,omitempty"`
	NodePlatform            string                      `json:"node_platform,omitempty"`
	TrustDomain             string                      `json:"trust_domain,omitempty"`
	RuntimeID               string                      `json:"runtime_id,omitempty"`
	RuntimeVersion          string                      `json:"runtime_version,omitempty"`
	CompatibilityClass      string                      `json:"compatibility_class,omitempty"`
	SupportedContextClasses []string                    `json:"supported_context_classes,omitempty"`
	TransportProfile        string                      `json:"transport_profile,omitempty"`
	SessionNonce            string                      `json:"session_nonce,omitempty"`
	SessionIssuedAt         time.Time                   `json:"session_issued_at,omitempty"`
	SessionExpiresAt        time.Time                   `json:"session_expires_at,omitempty"`
	PeerKeyID               string                      `json:"peer_key_id,omitempty"`
	Capabilities            []core.CapabilityDescriptor `json:"capabilities,omitempty"`
}

type connectedFrame struct {
	Type             string                      `json:"type"`
	SessionID        string                      `json:"session_id"`
	FeedScope        connectionFeedScope         `json:"feed_scope,omitempty"`
	ServerSeq        uint64                      `json:"server_seq"`
	ReplayFrom       uint64                      `json:"replay_from"`
	Capabilities     []core.CapabilityDescriptor `json:"capabilities,omitempty"`
	TransportProfile string                      `json:"transport_profile,omitempty"`
	SessionExpiresAt time.Time                   `json:"session_expires_at,omitempty"`
}

type NodeConnectInfo struct {
	NodeID                  string
	NodeName                string
	NodePlatform            string
	TrustClass              string
	TrustDomain             string
	RuntimeID               string
	RuntimeVersion          string
	CompatibilityClass      string
	SupportedContextClasses []string
	TransportProfile        string
	SessionNonce            string
	SessionIssuedAt         time.Time
	SessionExpiresAt        time.Time
	PeerKeyID               string
	Capabilities            []core.CapabilityDescriptor
}

type ConnectionPrincipal struct {
	Role          string
	Actor         core.EventActor
	Authenticated bool
	Principal     *core.AuthenticatedPrincipal
	FeedScope     connectionFeedScope
}

type connectionFeedScope string

const (
	feedScopeRuntime     connectionFeedScope = "runtime"
	feedScopeTenantAdmin connectionFeedScope = "tenant_admin"
	feedScopeGlobalAdmin connectionFeedScope = "global_admin"
)

func (s *Server) Handler() http.Handler {
	upgrader := s.Upgrader
	if upgrader.CheckOrigin == nil {
		upgrader.CheckOrigin = defaultCheckOrigin
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		token := extractBearerToken(r.Header.Get("Authorization"))
		principal, err := s.resolvePrincipal(r.Context(), token)
		if err != nil {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		defer conn.Close()
		_ = s.handleConnection(r.Context(), conn, principal, r.TLS != nil)
	})
	return mux
}

func (s *Server) resolvePrincipal(ctx context.Context, token string) (ConnectionPrincipal, error) {
	if s != nil && s.PrincipalResolver != nil {
		return s.PrincipalResolver(ctx, token)
	}
	return ConnectionPrincipal{}, fmt.Errorf("gateway principal resolver required")
}

func extractBearerToken(header string) string {
	const prefix = "Bearer "
	if len(header) > len(prefix) && header[:len(prefix)] == prefix {
		return header[len(prefix):]
	}
	return ""
}

func (s *Server) Start(ctx context.Context) error {
	if s == nil || s.Log == nil {
		<-ctx.Done()
		return ctx.Err()
	}
	var runErr error
	s.once.Do(func() {
		lastSeq, err := s.Log.LastSeq(ctx, s.partition())
		if err != nil {
			runErr = err
			return
		}
		go s.broadcastLoop(ctx, lastSeq)
	})
	return runErr
}

func (s *Server) handleConnection(ctx context.Context, conn *websocket.Conn, resolvedPrincipal ConnectionPrincipal, tlsActive bool) error {
	conn.SetReadLimit(maxInboundMessageSize)
	_, data, err := conn.ReadMessage()
	if err != nil {
		return err
	}
	frame, err := parseConnectFrame(data)
	if err != nil {
		_ = conn.WriteControl(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.ClosePolicyViolation, "first frame must be connect"), time.Now().Add(time.Second))
		return err
	}
	principal, err := validateAndBindPrincipal(frame, resolvedPrincipal)
	if err != nil {
		_ = conn.WriteControl(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.ClosePolicyViolation, err.Error()), time.Now().Add(time.Second))
		return err
	}
	principal, err = bindConnectionSessionID(principal)
	if err != nil {
		_ = conn.WriteControl(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseInternalServerErr, "session initialization failed"), time.Now().Add(time.Second))
		return err
	}
	if err := s.validateTransport(ctx, frame, tlsActive); err != nil {
		_ = conn.WriteControl(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.ClosePolicyViolation, err.Error()), time.Now().Add(time.Second))
		return err
	}
	response := s.connectedResponse(ctx, frame, principal)
	if err := conn.WriteJSON(response); err != nil {
		return err
	}
	if frame.LastSeenSeq > 0 {
		frames, err := s.replayFrames(ctx, principal, frame.LastSeenSeq)
		if err != nil {
			return err
		}
		for _, replay := range frames {
			if err := conn.WriteJSON(replay); err != nil {
				return err
			}
		}
	}
	if frame.Role == "node" && s.HandleNodeConnection != nil {
		info := nodeConnectInfoForFrame(frame, principal)
		if s.VerifyNodeConnection != nil {
			if err := s.VerifyNodeConnection(ctx, principal, info, conn); err != nil {
				_ = conn.WriteControl(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.ClosePolicyViolation, err.Error()), time.Now().Add(time.Second))
				return err
			}
		}
		return s.HandleNodeConnection(ctx, principal, info, conn)
	}
	bc := s.register(conn, principal)
	defer s.unregister(conn)
	for {
		_, data, err := conn.ReadMessage()
		if err != nil {
			return nil
		}
		if err := s.handleClientFrame(ctx, bc, frame, principal, data); err != nil {
			if err == io.EOF {
				return nil
			}
			return err
		}
	}
}

func parseConnectFrame(data []byte) (connectFrame, error) {
	var frame connectFrame
	if err := json.Unmarshal(data, &frame); err != nil || frame.Type != "connect" || frame.Role == "" {
		if err == nil {
			err = websocket.ErrBadHandshake
		}
		return connectFrame{}, err
	}
	return frame, nil
}

func (s *Server) connectedResponse(ctx context.Context, frame connectFrame, principal ConnectionPrincipal) connectedFrame {
	lastSeq := uint64(0)
	if s.Log != nil {
		partition := s.partition()
		payload, _ := json.Marshal(frame)
		actor := actorForLog(principal, frame.Role)
		seqs, _ := s.Log.Append(ctx, partition, []core.FrameworkEvent{{
			Timestamp: time.Now().UTC(),
			Type:      core.FrameworkEventSessionCreated,
			Payload:   payload,
			Actor:     actor,
			Partition: partition,
		}})
		if len(seqs) > 0 {
			lastSeq = seqs[len(seqs)-1]
		}
	}
	capabilities := append([]core.CapabilityDescriptor(nil), s.Capabilities...)
	if s.ListCapabilitiesForPrincipal != nil {
		capabilities = append([]core.CapabilityDescriptor(nil), s.ListCapabilitiesForPrincipal(principal)...)
	} else if s.ListCapabilities != nil {
		capabilities = append([]core.CapabilityDescriptor(nil), s.ListCapabilities()...)
	}
	return connectedFrame{
		Type:             "connected",
		SessionID:        connectionSessionID(principal),
		FeedScope:        connectionFeed(principal),
		ServerSeq:        lastSeq,
		ReplayFrom:       frame.LastSeenSeq,
		Capabilities:     capabilities,
		TransportProfile: frame.TransportProfile,
		SessionExpiresAt: frame.SessionExpiresAt,
	}
}

func nodeConnectInfoForFrame(frame connectFrame, principal ConnectionPrincipal) NodeConnectInfo {
	return NodeConnectInfo{
		NodeID:                  nodeIDForConnection(frame, principal),
		NodeName:                frame.NodeName,
		NodePlatform:            frame.NodePlatform,
		TrustClass:              nodeTrustClassForConnection(principal),
		TrustDomain:             strings.TrimSpace(frame.TrustDomain),
		RuntimeID:               strings.TrimSpace(frame.RuntimeID),
		RuntimeVersion:          strings.TrimSpace(frame.RuntimeVersion),
		CompatibilityClass:      strings.TrimSpace(frame.CompatibilityClass),
		SupportedContextClasses: append([]string(nil), frame.SupportedContextClasses...),
		TransportProfile:        strings.TrimSpace(frame.TransportProfile),
		SessionNonce:            strings.TrimSpace(frame.SessionNonce),
		SessionIssuedAt:         frame.SessionIssuedAt.UTC(),
		SessionExpiresAt:        frame.SessionExpiresAt.UTC(),
		PeerKeyID:               strings.TrimSpace(frame.PeerKeyID),
		Capabilities:            append([]core.CapabilityDescriptor(nil), frame.Capabilities...),
	}
}

func (s *Server) validateTransport(ctx context.Context, frame connectFrame, tlsActive bool) error {
	if s == nil || s.FMPTransportPolicy == nil {
		return nil
	}
	return s.FMPTransportPolicy.validateNodeConnect(ctx, frame, tlsActive)
}

func (s *Server) handleClientFrame(ctx context.Context, bc *broadcastClient, connect connectFrame, principal ConnectionPrincipal, data []byte) error {
	var envelope struct {
		Type string `json:"type"`
	}
	if err := json.Unmarshal(data, &envelope); err != nil {
		return s.sendError(bc, "invalid_frame", err.Error(), "")
	}
	switch envelope.Type {
	case "ping":
		return bc.sendOrError(map[string]any{"type": "pong"})
	case "session.close":
		_ = bc.send(map[string]any{"type": "session.closed", "session_id": connectionSessionID(principal)})
		return io.EOF
	case "admin.snapshot":
		if !isAdminPrincipal(principal) {
			return s.sendError(bc, "unauthorized", "admin scope required", envelope.Type)
		}
		if s.AdminSnapshot == nil {
			return s.sendError(bc, "unsupported", "admin snapshot unavailable", envelope.Type)
		}
		snapshot, err := s.AdminSnapshot(ctx, principal)
		if err != nil {
			return s.sendError(bc, "snapshot_failed", err.Error(), envelope.Type)
		}
		return bc.sendOrError(map[string]any{"type": "admin.snapshot", "snapshot": snapshot})
	case "message.outbound":
		return s.recordOutboundMessage(ctx, connect, principal, data)
	case "capability.invoke":
		return s.invokeCapability(ctx, bc, principal, data)
	default:
		return s.sendError(bc, "unsupported_frame", envelope.Type, envelope.Type)
	}
}

func (bc *broadcastClient) sendOrError(frame any) error {
	if !bc.send(frame) {
		return fmt.Errorf("client queue unavailable")
	}
	return nil
}

func (s *Server) sendError(bc *broadcastClient, code, message, correlationID string) error {
	frame := map[string]any{
		"type":    "error",
		"code":    code,
		"message": message,
	}
	if correlationID != "" {
		frame["correlation_id"] = correlationID
	}
	return bc.sendOrError(frame)
}

func (s *Server) recordOutboundMessage(ctx context.Context, connect connectFrame, principal ConnectionPrincipal, data []byte) error {
	var request struct {
		Type       string         `json:"type"`
		SessionKey string         `json:"session_key"`
		Content    map[string]any `json:"content"`
	}
	if err := json.Unmarshal(data, &request); err != nil {
		return err
	}
	dispatched := false
	if s.HandleOutboundMessage != nil && request.SessionKey != "" {
		if text, _ := request.Content["text"].(string); text != "" {
			if err := s.HandleOutboundMessage(ctx, principal, request.SessionKey, text); err != nil {
				return err
			}
			dispatched = true
		}
	}
	if !dispatched || s == nil || s.Log == nil {
		return nil
	}
	partition := s.partition()
	_, err := s.Log.Append(ctx, partition, []core.FrameworkEvent{{
		Timestamp: time.Now().UTC(),
		Type:      core.FrameworkEventMessageOutbound,
		Payload:   append([]byte(nil), data...),
		Actor:     actorForLog(principal, connect.Role),
		Partition: partition,
	}})
	return err
}

func (s *Server) invokeCapability(ctx context.Context, bc *broadcastClient, principal ConnectionPrincipal, data []byte) error {
	var request struct {
		Type          string         `json:"type"`
		CorrelationID string         `json:"correlation_id"`
		SessionKey    string         `json:"session_key"`
		CapabilityID  string         `json:"capability_id"`
		Args          map[string]any `json:"args"`
	}
	if err := json.Unmarshal(data, &request); err != nil {
		return err
	}
	if s.Log != nil {
		partition := s.partition()
		actor := actorForLog(principal, "agent")
		_, _ = s.Log.Append(ctx, partition, []core.FrameworkEvent{{
			Timestamp: time.Now().UTC(),
			Type:      core.FrameworkEventCapabilityInvoked,
			Payload:   append([]byte(nil), data...),
			Actor:     actor,
			Partition: partition,
		}})
	}
	result := &core.CapabilityExecutionResult{Success: false}
	if strings.TrimSpace(request.SessionKey) == "" {
		result.Error = "capability invocation requires session_key"
	} else if s.InvokeCapability != nil {
		execResult, err := s.InvokeCapability(ctx, principal, strings.TrimSpace(request.SessionKey), request.CapabilityID, request.Args)
		if err != nil {
			result.Error = err.Error()
		} else if execResult != nil {
			result = execResult
		}
	} else {
		result.Error = fmt.Sprintf("capability %s unavailable", request.CapabilityID)
	}
	response := map[string]any{
		"type":           "capability.result",
		"correlation_id": request.CorrelationID,
		"result":         result,
	}
	if !bc.send(response) {
		return fmt.Errorf("capability result delivery failed: client queue unavailable")
	}
	if s.Log != nil {
		partition := s.partition()
		payload, _ := json.Marshal(response)
		_, _ = s.Log.Append(ctx, partition, []core.FrameworkEvent{{
			Timestamp: time.Now().UTC(),
			Type:      core.FrameworkEventCapabilityResult,
			Payload:   payload,
			Actor:     actorForLog(principal, "agent"),
			Partition: partition,
		}})
	}
	return nil
}

func validateAndBindPrincipal(frame connectFrame, resolved ConnectionPrincipal) (ConnectionPrincipal, error) {
	role := strings.ToLower(strings.TrimSpace(frame.Role))
	if role == "" {
		return ConnectionPrincipal{}, fmt.Errorf("role required")
	}
	if !resolved.Authenticated {
		return ConnectionPrincipal{}, fmt.Errorf("authenticated principal required for role %s", role)
	}
	principal := resolved
	if principal.Role != "" && !strings.EqualFold(principal.Role, role) {
		return ConnectionPrincipal{}, fmt.Errorf("principal role %s cannot connect as %s", principal.Role, role)
	}
	if principal.Role == "" {
		principal.Role = role
	}
	if principal.Actor.ID == "" {
		if principal.Principal != nil && strings.TrimSpace(principal.Principal.Subject.ID) != "" {
			principal.Actor.ID = strings.TrimSpace(principal.Principal.Subject.ID)
		}
		if principal.Actor.ID == "" {
			return ConnectionPrincipal{}, fmt.Errorf("authenticated principal missing actor id")
		}
	}
	if role == "node" && principal.Actor.SubjectKind != "" && principal.Actor.SubjectKind != core.SubjectKindNode {
		return ConnectionPrincipal{}, fmt.Errorf("node connections require node subject")
	}
	feedScope, err := requestedFeedScope(frame, principal)
	if err != nil {
		return ConnectionPrincipal{}, err
	}
	principal.FeedScope = feedScope
	return principal, nil
}

func requestedFeedScope(frame connectFrame, principal ConnectionPrincipal) (connectionFeedScope, error) {
	requested := connectionFeedScope(strings.ToLower(strings.TrimSpace(frame.FeedScope)))
	if requested == "" {
		return connectionFeed(principal), nil
	}
	switch requested {
	case feedScopeRuntime:
		return requested, nil
	case feedScopeTenantAdmin:
		if !isAdminPrincipal(principal) {
			return "", fmt.Errorf("feed_scope %s requires admin scope", requested)
		}
		return requested, nil
	case feedScopeGlobalAdmin:
		if !hasGlobalAdminScope(principal) {
			return "", fmt.Errorf("feed_scope %s requires global admin scope", requested)
		}
		return requested, nil
	default:
		return "", fmt.Errorf("feed_scope %s invalid", requested)
	}
}

func bindConnectionSessionID(principal ConnectionPrincipal) (ConnectionPrincipal, error) {
	if principal.Principal != nil && strings.TrimSpace(principal.Principal.SessionID) != "" {
		principal.Principal = clonePrincipal(principal.Principal)
		principal.Principal.SessionID = strings.TrimSpace(principal.Principal.SessionID)
		return principal, nil
	}
	sessionID, err := generateGatewaySessionID()
	if err != nil {
		return ConnectionPrincipal{}, err
	}
	if principal.Principal == nil {
		principal.Principal = &core.AuthenticatedPrincipal{}
	} else {
		principal.Principal = clonePrincipal(principal.Principal)
	}
	principal.Principal.SessionID = sessionID
	return principal, nil
}

func connectionSessionID(principal ConnectionPrincipal) string {
	if principal.Principal == nil {
		return ""
	}
	return strings.TrimSpace(principal.Principal.SessionID)
}

func clonePrincipal(principal *core.AuthenticatedPrincipal) *core.AuthenticatedPrincipal {
	if principal == nil {
		return nil
	}
	clone := *principal
	clone.Scopes = append([]string(nil), principal.Scopes...)
	return &clone
}

func actorForLog(principal ConnectionPrincipal, fallbackKind string) core.EventActor {
	actor := principal.Actor
	if strings.TrimSpace(actor.Kind) == "" {
		actor.Kind = connectionRole(principal, fallbackKind)
	}
	if strings.TrimSpace(actor.ID) == "" {
		actor.ID = connectionSessionID(principal)
	}
	return actor
}

func connectionRole(principal ConnectionPrincipal, fallback string) string {
	if strings.TrimSpace(principal.Role) != "" {
		return strings.TrimSpace(principal.Role)
	}
	return fallback
}

func generateGatewaySessionID() (string, error) {
	buf := make([]byte, 18)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return "gw_" + base64.RawURLEncoding.EncodeToString(buf), nil
}

func (s *Server) partition() string {
	if s == nil || strings.TrimSpace(s.Partition) == "" {
		return "local"
	}
	return strings.TrimSpace(s.Partition)
}

func nodeIDForConnection(frame connectFrame, principal ConnectionPrincipal) string {
	if principal.Authenticated && principal.Actor.ID != "" {
		return principal.Actor.ID
	}
	if strings.TrimSpace(frame.NodeID) != "" {
		return frame.NodeID
	}
	return "node-session"
}

func nodeTrustClassForConnection(principal ConnectionPrincipal) string {
	if principal.Authenticated {
		return string(core.TrustClassRemoteApproved)
	}
	return ""
}

func defaultCheckOrigin(r *http.Request) bool {
	origin := strings.TrimSpace(r.Header.Get("Origin"))
	if origin == "" {
		return true
	}
	parsed, err := http.NewRequest(http.MethodGet, origin, nil)
	if err != nil || parsed.URL == nil {
		return false
	}
	return strings.EqualFold(parsed.URL.Host, r.Host)
}

func (s *Server) broadcastLoop(ctx context.Context, afterSeq uint64) {
	partition := s.partition()
	for {
		events, err := s.Log.Read(ctx, partition, afterSeq, 256, true)
		if err != nil {
			return
		}
		for _, ev := range events {
			afterSeq = ev.Seq
			s.broadcastEvent(ctx, ev)
		}
	}
}

func (s *Server) register(conn *websocket.Conn, principal ConnectionPrincipal) *broadcastClient {
	bc := &broadcastClient{
		conn:      conn,
		queue:     make(chan any, broadcastQueueDepth),
		principal: principal,
	}
	s.mu.Lock()
	if s.clients == nil {
		s.clients = map[*websocket.Conn]*broadcastClient{}
	}
	s.clients[conn] = bc
	s.mu.Unlock()
	go s.writeLoop(bc)
	return bc
}

func (s *Server) unregister(conn *websocket.Conn) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.clients, conn)
}

// writeLoop drains a client's queue with a per-write deadline. It exits when
// the queue is closed (slow-client eviction) or a write fails.
func (s *Server) writeLoop(bc *broadcastClient) {
	defer func() {
		_ = bc.conn.Close()
		s.unregister(bc.conn)
	}()
	for frame := range bc.queue {
		_ = bc.conn.SetWriteDeadline(time.Now().Add(broadcastWriteTimeout))
		if err := bc.conn.WriteJSON(frame); err != nil {
			bc.closeQueue()
			return
		}
	}
}

func (s *Server) broadcastEvent(ctx context.Context, ev core.FrameworkEvent) {
	s.mu.RLock()
	clients := make([]*broadcastClient, 0, len(s.clients))
	for _, bc := range s.clients {
		clients = append(clients, bc)
	}
	s.mu.RUnlock()
	for _, bc := range clients {
		allowed, err := s.canDeliverEvent(ctx, bc.principal, ev)
		if err != nil || !allowed {
			continue
		}
		if !bc.send(replayEventFrame{
			Type:   "event",
			Replay: false,
			Event:  ev,
		}) {
			// Queue was full — client evicted; clean up the registry entry.
			s.unregister(bc.conn)
		}
	}
}

func (s *Server) canDeliverEvent(ctx context.Context, principal ConnectionPrincipal, ev core.FrameworkEvent) (bool, error) {
	switch connectionFeed(principal) {
	case feedScopeGlobalAdmin:
		return true, nil
	case feedScopeTenantAdmin:
		return s.canDeliverTenantAdminEvent(ctx, principal, ev)
	default:
		return s.canDeliverRuntimeEvent(ctx, principal, ev)
	}
}

func (s *Server) canDeliverTenantAdminEvent(ctx context.Context, principal ConnectionPrincipal, ev core.FrameworkEvent) (bool, error) {
	tenantID, err := s.eventTenantID(ctx, ev)
	if err != nil {
		return false, err
	}
	if tenantID == "" {
		return false, nil
	}
	return strings.EqualFold(tenantID, principal.Actor.TenantID), nil
}

func (s *Server) canDeliverRuntimeEvent(ctx context.Context, principal ConnectionPrincipal, ev core.FrameworkEvent) (bool, error) {
	tenantID, err := s.eventTenantID(ctx, ev)
	if err != nil {
		return false, err
	}
	if tenantID == "" || !strings.EqualFold(tenantID, principal.Actor.TenantID) {
		return false, nil
	}
	sessionID, ok := s.eventSessionID(ev)
	if !ok || s.SessionEventAuthorizer == nil {
		return false, nil
	}
	return s.SessionEventAuthorizer(ctx, principal, sessionID)
}

func (s *Server) eventTenantID(ctx context.Context, ev core.FrameworkEvent) (string, error) {
	if strings.TrimSpace(ev.Actor.TenantID) != "" {
		return strings.TrimSpace(ev.Actor.TenantID), nil
	}
	if s == nil || s.SessionTenantResolver == nil {
		return "", nil
	}
	var payload map[string]any
	if len(ev.Payload) == 0 {
		return "", nil
	}
	if err := json.Unmarshal(ev.Payload, &payload); err != nil {
		return "", nil
	}
	if sessionID, ok := eventSessionIDFromPayload(payload); ok {
		return s.SessionTenantResolver(ctx, sessionID)
	}
	return "", nil
}

func (s *Server) eventSessionID(ev core.FrameworkEvent) (string, bool) {
	if strings.EqualFold(ev.Type, core.FrameworkEventSessionMessage) && strings.TrimSpace(ev.Actor.ID) != "" {
		return strings.TrimSpace(ev.Actor.ID), true
	}
	if len(ev.Payload) == 0 {
		return "", false
	}
	var payload map[string]any
	if err := json.Unmarshal(ev.Payload, &payload); err != nil {
		return "", false
	}
	return eventSessionIDFromPayload(payload)
}

func eventSessionIDFromPayload(payload map[string]any) (string, bool) {
	for _, key := range []string{"session_key", "session_id"} {
		if sessionID, ok := payload[key].(string); ok && strings.TrimSpace(sessionID) != "" {
			return strings.TrimSpace(sessionID), true
		}
	}
	return "", false
}

func isAdminPrincipal(principal ConnectionPrincipal) bool {
	if !principal.Authenticated || principal.Principal == nil {
		return false
	}
	for _, scope := range principal.Principal.Scopes {
		switch strings.ToLower(strings.TrimSpace(scope)) {
		case "admin", "operator", "gateway:admin", "nexus:admin":
			return true
		}
	}
	return false
}

func hasGlobalAdminScope(principal ConnectionPrincipal) bool {
	if !isAdminPrincipal(principal) || principal.Principal == nil {
		return false
	}
	for _, scope := range principal.Principal.Scopes {
		switch strings.ToLower(strings.TrimSpace(scope)) {
		case "nexus:admin:global", "gateway:admin:global", "admin:global":
			return true
		}
	}
	return false
}

func connectionFeed(principal ConnectionPrincipal) connectionFeedScope {
	if principal.FeedScope != "" {
		return principal.FeedScope
	}
	if hasGlobalAdminScope(principal) {
		return feedScopeGlobalAdmin
	}
	if isAdminPrincipal(principal) {
		return feedScopeTenantAdmin
	}
	return feedScopeRuntime
}
