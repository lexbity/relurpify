package server

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"sync"

	"github.com/lexcodex/relurpify/framework/middleware/mcp/protocol"
	"github.com/lexcodex/relurpify/framework/middleware/mcp/versioning"
)

type Exporter interface {
	ListTools(ctx context.Context) ([]protocol.Tool, error)
	CallTool(ctx context.Context, name string, args map[string]any) (*protocol.CallToolResult, error)
	ListPrompts(ctx context.Context) ([]protocol.Prompt, error)
	GetPrompt(ctx context.Context, name string, args map[string]any) (*protocol.GetPromptResult, error)
	ListResources(ctx context.Context) ([]protocol.Resource, error)
	ReadResource(ctx context.Context, uri string) (*protocol.ReadResourceResult, error)
}

type Hooks struct {
	OnSessionOpen        func(sessionID string, requestedVersion string)
	OnSessionInitialized func(sessionID string, negotiatedVersion string, client protocol.PeerInfo)
	OnSessionClosed      func(sessionID string, err error)
}

type Service struct {
	peerInfo protocol.PeerInfo
	exporter Exporter
	matrix   versioning.SupportMatrix
	hooks    Hooks
	mu       sync.Mutex
	sessions map[string]httpSessionState
	streams  map[string]streamSessionState
	nextID   uint64
}

type httpSessionState struct {
	clientInfo        protocol.PeerInfo
	negotiatedVersion string
	initialized       bool
}

type streamSessionState struct {
	clientInfo        protocol.PeerInfo
	negotiatedVersion string
	initialized       bool
	send              func(rpcEnvelope) error
	subscriptions     map[string]struct{}
}

type rpcEnvelope struct {
	JSONRPC string          `json:"jsonrpc,omitempty"`
	ID      string          `json:"id,omitempty"`
	Method  string          `json:"method,omitempty"`
	Params  json.RawMessage `json:"params,omitempty"`
	Result  any             `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func New(peer protocol.PeerInfo, exporter Exporter, hooks Hooks) *Service {
	return &Service{
		peerInfo: peer,
		exporter: exporter,
		matrix:   versioning.DefaultSupportMatrix(),
		hooks:    hooks,
		sessions: make(map[string]httpSessionState),
		streams:  make(map[string]streamSessionState),
	}
}

func (s *Service) ServeConn(ctx context.Context, sessionID string, conn io.ReadWriteCloser) error {
	if s == nil || s.exporter == nil {
		return fmt.Errorf("server unavailable")
	}
	if strings.TrimSpace(sessionID) == "" {
		return fmt.Errorf("session id required")
	}
	defer conn.Close()
	decoder := json.NewDecoder(bufio.NewReader(conn))
	encoder := json.NewEncoder(conn)
	var (
		initialized       bool
		clientInfo        protocol.PeerInfo
		negotiatedVersion string
		writeMu           sync.Mutex
	)
	send := func(env rpcEnvelope) error {
		writeMu.Lock()
		defer writeMu.Unlock()
		return encoder.Encode(env)
	}
	s.registerStreamSession(sessionID, streamSessionState{send: send, subscriptions: map[string]struct{}{}})
	for {
		var envelope rpcEnvelope
		if err := decoder.Decode(&envelope); err != nil {
			s.mu.Lock()
			delete(s.streams, sessionID)
			s.mu.Unlock()
			if s.hooks.OnSessionClosed != nil {
				s.hooks.OnSessionClosed(sessionID, err)
			}
			return err
		}
		switch envelope.Method {
		case "initialize":
			var req protocol.InitializeRequest
			if err := json.Unmarshal(envelope.Params, &req); err != nil {
				_ = send(rpcEnvelope{JSONRPC: "2.0", ID: envelope.ID, Error: &rpcError{Code: -32602, Message: err.Error()}})
				continue
			}
			clientInfo = req.ClientInfo
			s.updateStreamSession(sessionID, func(state *streamSessionState) {
				state.clientInfo = req.ClientInfo
			})
			if s.hooks.OnSessionOpen != nil {
				s.hooks.OnSessionOpen(sessionID, req.ProtocolVersion)
			}
			negotiated, err := s.matrix.Negotiate(req.ProtocolVersion, []string{req.ProtocolVersion})
			if err != nil {
				_ = send(rpcEnvelope{JSONRPC: "2.0", ID: envelope.ID, Error: &rpcError{Code: -32001, Message: err.Error()}})
				continue
			}
			result := protocol.InitializeResult{
				ProtocolVersion: negotiated.Negotiated,
				ServerInfo:      s.peerInfo,
				Capabilities: map[string]any{
					"tools":     map[string]any{"listChanged": true},
					"prompts":   map[string]any{"listChanged": true},
					"resources": map[string]any{"listChanged": true},
				},
			}
			negotiatedVersion = negotiated.Negotiated
			s.updateStreamSession(sessionID, func(state *streamSessionState) {
				state.negotiatedVersion = negotiatedVersion
			})
			_ = send(rpcEnvelope{JSONRPC: "2.0", ID: envelope.ID, Result: result})
		case "notifications/initialized":
			initialized = true
			s.updateStreamSession(sessionID, func(state *streamSessionState) {
				state.initialized = true
			})
			if s.hooks.OnSessionInitialized != nil {
				s.hooks.OnSessionInitialized(sessionID, negotiatedVersion, clientInfo)
			}
		default:
			if !initialized {
				_ = send(rpcEnvelope{JSONRPC: "2.0", ID: envelope.ID, Error: &rpcError{Code: -32002, Message: "session not initialized"}})
				continue
			}
			if err := s.handleMethod(ctx, sessionID, send, envelope); err != nil {
				_ = send(rpcEnvelope{JSONRPC: "2.0", ID: envelope.ID, Error: &rpcError{Code: -32003, Message: err.Error()}})
			}
		}
	}
}

func (s *Service) handleMethod(ctx context.Context, sessionID string, send func(rpcEnvelope) error, envelope rpcEnvelope) error {
	switch envelope.Method {
	case "tools/list":
		tools, err := s.exporter.ListTools(ctx)
		if err != nil {
			return err
		}
		return send(rpcEnvelope{JSONRPC: "2.0", ID: envelope.ID, Result: protocol.ListToolsResult{Tools: tools}})
	case "tools/call":
		var params protocol.CallToolParams
		if err := json.Unmarshal(envelope.Params, &params); err != nil {
			return err
		}
		result, err := s.exporter.CallTool(ctx, params.Name, params.Arguments)
		if err != nil {
			return err
		}
		return send(rpcEnvelope{JSONRPC: "2.0", ID: envelope.ID, Result: result})
	case "prompts/list":
		prompts, err := s.exporter.ListPrompts(ctx)
		if err != nil {
			return err
		}
		return send(rpcEnvelope{JSONRPC: "2.0", ID: envelope.ID, Result: protocol.ListPromptsResult{Prompts: prompts}})
	case "prompts/get":
		var params protocol.GetPromptParams
		if err := json.Unmarshal(envelope.Params, &params); err != nil {
			return err
		}
		result, err := s.exporter.GetPrompt(ctx, params.Name, params.Arguments)
		if err != nil {
			return err
		}
		return send(rpcEnvelope{JSONRPC: "2.0", ID: envelope.ID, Result: result})
	case "resources/list":
		resources, err := s.exporter.ListResources(ctx)
		if err != nil {
			return err
		}
		return send(rpcEnvelope{JSONRPC: "2.0", ID: envelope.ID, Result: protocol.ListResourcesResult{Resources: resources}})
	case "resources/read":
		var params protocol.ReadResourceParams
		if err := json.Unmarshal(envelope.Params, &params); err != nil {
			return err
		}
		result, err := s.exporter.ReadResource(ctx, params.URI)
		if err != nil {
			return err
		}
		return send(rpcEnvelope{JSONRPC: "2.0", ID: envelope.ID, Result: result})
	case "resources/subscribe":
		var params protocol.ResourceSubscribeParams
		if err := json.Unmarshal(envelope.Params, &params); err != nil {
			return err
		}
		s.subscribeResource(sessionID, params.URI)
		return send(rpcEnvelope{JSONRPC: "2.0", ID: envelope.ID, Result: map[string]any{}})
	case "resources/unsubscribe":
		var params protocol.ResourceSubscribeParams
		if err := json.Unmarshal(envelope.Params, &params); err != nil {
			return err
		}
		s.unsubscribeResource(sessionID, params.URI)
		return send(rpcEnvelope{JSONRPC: "2.0", ID: envelope.ID, Result: map[string]any{}})
	default:
		return fmt.Errorf("method %s not supported", envelope.Method)
	}
}

func (s *Service) registerStreamSession(sessionID string, state streamSessionState) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.streams[sessionID] = state
}

func (s *Service) updateStreamSession(sessionID string, update func(*streamSessionState)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	state, ok := s.streams[sessionID]
	if !ok {
		return
	}
	update(&state)
	s.streams[sessionID] = state
}

func (s *Service) subscribeResource(sessionID, uri string) {
	s.updateStreamSession(sessionID, func(state *streamSessionState) {
		if state.subscriptions == nil {
			state.subscriptions = map[string]struct{}{}
		}
		state.subscriptions[uri] = struct{}{}
	})
}

func (s *Service) unsubscribeResource(sessionID, uri string) {
	s.updateStreamSession(sessionID, func(state *streamSessionState) {
		delete(state.subscriptions, uri)
	})
}

func (s *Service) NotifyResourceUpdated(uri string) error {
	s.mu.Lock()
	sessions := make([]streamSessionState, 0, len(s.streams))
	for _, state := range s.streams {
		if _, ok := state.subscriptions[uri]; ok && state.send != nil && state.initialized {
			sessions = append(sessions, state)
		}
	}
	s.mu.Unlock()
	for _, state := range sessions {
		if err := state.send(rpcEnvelope{
			JSONRPC: "2.0",
			Method:  "notifications/resources/updated",
			Params:  mustMarshal(protocol.ResourceUpdatedParams{URI: uri}),
		}); err != nil {
			return err
		}
	}
	return nil
}

func mustMarshal(value any) json.RawMessage {
	data, _ := json.Marshal(value)
	return data
}
