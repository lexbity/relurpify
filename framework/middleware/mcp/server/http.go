package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/lexcodex/relurpify/framework/middleware/mcp/protocol"
)

const SessionHeader = "Mcp-Session-Id"

// ServeHTTP exposes the MCP server facade over a basic session-bound HTTP
// transport. Resumability and long-lived streaming are deferred; this covers
// initialize, initialized, request/response, and explicit session close.
func (s *Service) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if s == nil || s.exporter == nil {
		http.Error(w, "server unavailable", http.StatusServiceUnavailable)
		return
	}
	switch r.Method {
	case http.MethodPost:
		s.handleHTTPPost(w, r)
	case http.MethodDelete:
		s.handleHTTPDelete(w, r)
	default:
		w.Header().Set("Allow", "POST, DELETE")
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Service) handleHTTPPost(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()
	var envelope rpcEnvelope
	if err := json.NewDecoder(r.Body).Decode(&envelope); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	sessionID := strings.TrimSpace(r.Header.Get(SessionHeader))
	w.Header().Set("Content-Type", "application/json")
	if envelope.Method == "initialize" {
		if sessionID == "" {
			sessionID = s.allocateHTTPSessionID()
		}
		w.Header().Set(SessionHeader, sessionID)
		s.handleHTTPInitialize(w, sessionID, envelope)
		return
	}
	if sessionID == "" {
		s.writeHTTPEnvelope(w, rpcEnvelope{
			JSONRPC: "2.0",
			ID:      envelope.ID,
			Error:   &rpcError{Code: -32002, Message: "session id required"},
		})
		return
	}
	w.Header().Set(SessionHeader, sessionID)
	if envelope.Method == "notifications/initialized" {
		if err := s.markHTTPSessionInitialized(sessionID); err != nil {
			s.writeHTTPEnvelope(w, rpcEnvelope{
				JSONRPC: "2.0",
				ID:      envelope.ID,
				Error:   &rpcError{Code: -32002, Message: err.Error()},
			})
			return
		}
		w.WriteHeader(http.StatusAccepted)
		return
	}
	if !s.httpSessionInitialized(sessionID) {
		s.writeHTTPEnvelope(w, rpcEnvelope{
			JSONRPC: "2.0",
			ID:      envelope.ID,
			Error:   &rpcError{Code: -32002, Message: "session not initialized"},
		})
		return
	}
	if err := s.handleMethod(r.Context(), sessionID, func(env rpcEnvelope) error {
		return s.writeHTTPEnvelope(w, env)
	}, envelope); err != nil {
		_ = s.writeHTTPEnvelope(w, rpcEnvelope{
			JSONRPC: "2.0",
			ID:      envelope.ID,
			Error:   &rpcError{Code: -32003, Message: err.Error()},
		})
	}
}

func (s *Service) handleHTTPDelete(w http.ResponseWriter, r *http.Request) {
	sessionID := strings.TrimSpace(r.Header.Get(SessionHeader))
	if sessionID == "" {
		http.Error(w, "session id required", http.StatusBadRequest)
		return
	}
	if !s.closeHTTPSession(sessionID, nil) {
		http.Error(w, "session not found", http.StatusNotFound)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Service) handleHTTPInitialize(w http.ResponseWriter, sessionID string, envelope rpcEnvelope) {
	var req protocol.InitializeRequest
	if err := json.Unmarshal(envelope.Params, &req); err != nil {
		_ = s.writeHTTPEnvelope(w, rpcEnvelope{
			JSONRPC: "2.0",
			ID:      envelope.ID,
			Error:   &rpcError{Code: -32602, Message: err.Error()},
		})
		return
	}
	if s.hooks.OnSessionOpen != nil {
		s.hooks.OnSessionOpen(sessionID, req.ProtocolVersion)
	}
	negotiated, err := s.matrix.Negotiate(req.ProtocolVersion, []string{req.ProtocolVersion})
	if err != nil {
		_ = s.writeHTTPEnvelope(w, rpcEnvelope{
			JSONRPC: "2.0",
			ID:      envelope.ID,
			Error:   &rpcError{Code: -32001, Message: err.Error()},
		})
		return
	}
	s.storeHTTPSession(sessionID, httpSessionState{
		clientInfo:        req.ClientInfo,
		negotiatedVersion: negotiated.Negotiated,
		initialized:       false,
	})
	_ = s.writeHTTPEnvelope(w, rpcEnvelope{
		JSONRPC: "2.0",
		ID:      envelope.ID,
		Result: protocol.InitializeResult{
			ProtocolVersion: negotiated.Negotiated,
			ServerInfo:      s.peerInfo,
			Capabilities: map[string]any{
				"tools":     map[string]any{"listChanged": true},
				"prompts":   map[string]any{"listChanged": true},
				"resources": map[string]any{"listChanged": true},
			},
		},
	})
}

func (s *Service) writeHTTPEnvelope(w http.ResponseWriter, env rpcEnvelope) error {
	return json.NewEncoder(w).Encode(env)
}

func (s *Service) allocateHTTPSessionID() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.nextID++
	return fmt.Sprintf("http-%d", s.nextID)
}

func (s *Service) storeHTTPSession(sessionID string, state httpSessionState) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sessions[sessionID] = state
}

func (s *Service) httpSessionInitialized(sessionID string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	state, ok := s.sessions[sessionID]
	return ok && state.initialized
}

func (s *Service) markHTTPSessionInitialized(sessionID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	state, ok := s.sessions[sessionID]
	if !ok {
		return fmt.Errorf("session not found")
	}
	state.initialized = true
	s.sessions[sessionID] = state
	if s.hooks.OnSessionInitialized != nil {
		s.hooks.OnSessionInitialized(sessionID, state.negotiatedVersion, state.clientInfo)
	}
	return nil
}

func (s *Service) closeHTTPSession(sessionID string, err error) bool {
	s.mu.Lock()
	_, ok := s.sessions[sessionID]
	if ok {
		delete(s.sessions, sessionID)
	}
	s.mu.Unlock()
	if ok && s.hooks.OnSessionClosed != nil {
		s.hooks.OnSessionClosed(sessionID, err)
	}
	return ok
}
