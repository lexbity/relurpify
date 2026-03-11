package gateway

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/gorilla/websocket"
	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/framework/memory/db"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGatewayHandshakeAcceptsConnectFrame(t *testing.T) {
	log, err := db.NewSQLiteEventLog(t.TempDir() + "/events.db")
	require.NoError(t, err)
	defer log.Close()

	server := &Server{Log: log, Partition: "tenant-a"}
	data, err := json.Marshal(map[string]any{
		"type":          "connect",
		"version":       "1.0",
		"role":          "agent",
		"last_seen_seq": 0,
	})
	require.NoError(t, err)

	frame, err := parseConnectFrame(data)
	require.NoError(t, err)
	principal, err := bindConnectionSessionID(ConnectionPrincipal{})
	require.NoError(t, err)
	response := server.connectedResponse(context.Background(), frame, principal)
	require.Equal(t, "connected", response.Type)
	require.NotEmpty(t, response.SessionID)
	require.NotEqual(t, "agent-session", response.SessionID)
	require.Equal(t, uint64(1), response.ServerSeq)

	events, err := log.Read(context.Background(), "tenant-a", 0, 10, false)
	require.NoError(t, err)
	require.NotEmpty(t, events)
	require.Equal(t, "tenant-a", events[0].Partition)
}

func TestGatewayHandshakeIncludesDynamicCapabilities(t *testing.T) {
	log, err := db.NewSQLiteEventLog(t.TempDir() + "/events.db")
	require.NoError(t, err)
	defer log.Close()

	server := &Server{
		Log: log,
		ListCapabilitiesForPrincipal: func(principal ConnectionPrincipal) []core.CapabilityDescriptor {
			require.Equal(t, "tenant-1", principal.Actor.TenantID)
			return []core.CapabilityDescriptor{{
				ID:   "camera.capture",
				Name: "camera.capture",
				Kind: core.CapabilityKindTool,
			}}
		},
	}
	frame := connectFrame{Type: "connect", Role: "agent"}
	response := server.connectedResponse(context.Background(), frame, ConnectionPrincipal{
		Authenticated: true,
		Actor:         core.EventActor{Kind: "agent", ID: "svc-1", TenantID: "tenant-1"},
	})
	require.Len(t, response.Capabilities, 1)
	require.Equal(t, "camera.capture", response.Capabilities[0].ID)
	require.Equal(t, feedScopeRuntime, response.FeedScope)
}

func TestGatewayRejectsBadFirstFrame(t *testing.T) {
	_, err := parseConnectFrame([]byte(`{"type":"wrong"}`))
	require.Error(t, err)
}

func TestGatewayPrincipalResolverRejectsUnknownToken(t *testing.T) {
	server := &Server{
		PrincipalResolver: func(_ context.Context, token string) (ConnectionPrincipal, error) {
			if token != "valid-token" {
				return ConnectionPrincipal{}, errors.New("unknown token")
			}
			return ConnectionPrincipal{
				Authenticated: true,
				Actor:         core.EventActor{Kind: "agent", ID: "svc-1", TenantID: "tenant-1"},
			}, nil
		},
	}
	ts := httptest.NewServer(server.Handler())
	defer ts.Close()

	wsURL := "ws" + strings.TrimPrefix(ts.URL, "http")
	_, resp, err := websocket.DefaultDialer.Dial(wsURL, http.Header{
		"Authorization": []string{"Bearer bad-token"},
	})
	require.Error(t, err)
	require.Equal(t, http.StatusUnauthorized, resp.StatusCode)
}

func TestGatewayPrincipalResolverRejectsAnonymousAgent(t *testing.T) {
	log, err := db.NewSQLiteEventLog(t.TempDir() + "/events.db")
	require.NoError(t, err)
	defer log.Close()

	server := &Server{
		Log: log,
		PrincipalResolver: func(_ context.Context, token string) (ConnectionPrincipal, error) {
			if token == "" {
				return ConnectionPrincipal{}, nil
			}
			return ConnectionPrincipal{}, errors.New("unexpected token")
		},
	}
	ts := httptest.NewServer(server.Handler())
	defer ts.Close()

	wsURL := "ws" + strings.TrimPrefix(ts.URL, "http")
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	require.NoError(t, err)
	defer conn.Close()

	connectMsg, _ := json.Marshal(map[string]any{
		"type": "connect", "version": "1.0", "role": "agent", "last_seen_seq": 0,
	})
	require.NoError(t, conn.WriteMessage(websocket.TextMessage, connectMsg))
	_, _, err = conn.ReadMessage()
	require.Error(t, err)
}

func TestGatewayWithoutAuthConfigurationRejectsConnection(t *testing.T) {
	log, err := db.NewSQLiteEventLog(t.TempDir() + "/events.db")
	require.NoError(t, err)
	defer log.Close()

	server := &Server{Log: log}
	ts := httptest.NewServer(server.Handler())
	defer ts.Close()

	wsURL := "ws" + strings.TrimPrefix(ts.URL, "http")
	_, resp, err := websocket.DefaultDialer.Dial(wsURL, nil)
	require.Error(t, err)
	require.Equal(t, http.StatusUnauthorized, resp.StatusCode)
}

func TestGatewayPrincipalResolverUsesResolvedActor(t *testing.T) {
	log, err := db.NewSQLiteEventLog(t.TempDir() + "/events.db")
	require.NoError(t, err)
	defer log.Close()

	server := &Server{
		Log: log,
		PrincipalResolver: func(_ context.Context, token string) (ConnectionPrincipal, error) {
			require.Equal(t, "valid-token", token)
			principal := core.AuthenticatedPrincipal{
				TenantID:      "tenant-1",
				AuthMethod:    core.AuthMethodBearerToken,
				Authenticated: true,
				Subject: core.SubjectRef{
					TenantID: "tenant-1",
					Kind:     core.SubjectKindServiceAccount,
					ID:       "svc-1",
				},
			}
			return ConnectionPrincipal{
				Authenticated: true,
				Principal:     &principal,
				Actor: core.EventActor{
					Kind:        "agent",
					ID:          "svc-1",
					TenantID:    "tenant-1",
					SubjectKind: core.SubjectKindServiceAccount,
				},
			}, nil
		},
	}
	ts := httptest.NewServer(server.Handler())
	defer ts.Close()

	wsURL := "ws" + strings.TrimPrefix(ts.URL, "http")
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, http.Header{
		"Authorization": []string{"Bearer valid-token"},
	})
	require.NoError(t, err)
	defer conn.Close()

	connectMsg, _ := json.Marshal(map[string]any{
		"type": "connect", "version": "1.0", "role": "agent", "actor_id": "spoofed", "last_seen_seq": 0,
	})
	require.NoError(t, conn.WriteMessage(websocket.TextMessage, connectMsg))
	_, _, err = conn.ReadMessage()
	require.NoError(t, err)

	events, err := log.Read(context.Background(), "local", 0, 10, false)
	require.NoError(t, err)
	require.NotEmpty(t, events)
	require.Equal(t, "svc-1", events[0].Actor.ID)
}

func TestValidateAndBindPrincipalFallsBackToResolvedSubjectID(t *testing.T) {
	principal, err := validateAndBindPrincipal(connectFrame{Role: "agent", ActorID: "spoofed"}, ConnectionPrincipal{
		Authenticated: true,
		Principal: &core.AuthenticatedPrincipal{
			Authenticated: true,
			Subject: core.SubjectRef{
				TenantID: "tenant-1",
				Kind:     core.SubjectKindServiceAccount,
				ID:       "svc-1",
			},
		},
	})
	require.NoError(t, err)
	require.Equal(t, "agent", principal.Role)
	require.Equal(t, "svc-1", principal.Actor.ID)
	require.Equal(t, feedScopeRuntime, principal.FeedScope)
}

func TestValidateAndBindPrincipalHonorsExplicitRuntimeFeedForAdmin(t *testing.T) {
	principal, err := validateAndBindPrincipal(connectFrame{Role: "admin", FeedScope: "runtime"}, ConnectionPrincipal{
		Authenticated: true,
		Actor:         core.EventActor{Kind: "admin", ID: "admin-a", TenantID: "tenant-a"},
		Principal: &core.AuthenticatedPrincipal{
			Authenticated: true,
			Scopes:        []string{"gateway:admin"},
			Subject:       core.SubjectRef{TenantID: "tenant-a", Kind: core.SubjectKindServiceAccount, ID: "admin-a"},
		},
	})
	require.NoError(t, err)
	require.Equal(t, feedScopeRuntime, principal.FeedScope)
}

func TestValidateAndBindPrincipalRejectsUnauthorizedFeedScope(t *testing.T) {
	_, err := validateAndBindPrincipal(connectFrame{Role: "agent", FeedScope: "tenant_admin"}, ConnectionPrincipal{
		Authenticated: true,
		Actor:         core.EventActor{Kind: "agent", ID: "svc-a", TenantID: "tenant-a"},
		Principal: &core.AuthenticatedPrincipal{
			Authenticated: true,
			Subject:       core.SubjectRef{TenantID: "tenant-a", Kind: core.SubjectKindServiceAccount, ID: "svc-a"},
		},
	})
	require.ErrorContains(t, err, "requires admin scope")
}

func TestValidateAndBindPrincipalRejectsGlobalFeedWithoutGlobalScope(t *testing.T) {
	_, err := validateAndBindPrincipal(connectFrame{Role: "admin", FeedScope: "global_admin"}, ConnectionPrincipal{
		Authenticated: true,
		Actor:         core.EventActor{Kind: "admin", ID: "admin-a", TenantID: "tenant-a"},
		Principal: &core.AuthenticatedPrincipal{
			Authenticated: true,
			Scopes:        []string{"gateway:admin"},
			Subject:       core.SubjectRef{TenantID: "tenant-a", Kind: core.SubjectKindServiceAccount, ID: "admin-a"},
		},
	})
	require.ErrorContains(t, err, "requires global admin scope")
}

func TestValidateAndBindPrincipalRejectsAuthenticatedPrincipalWithoutActorIdentity(t *testing.T) {
	_, err := validateAndBindPrincipal(connectFrame{Role: "agent", ActorID: "spoofed"}, ConnectionPrincipal{
		Authenticated: true,
		Principal: &core.AuthenticatedPrincipal{
			Authenticated: true,
			Subject: core.SubjectRef{
				TenantID: "tenant-1",
				Kind:     core.SubjectKindServiceAccount,
			},
		},
	})
	require.ErrorContains(t, err, "missing actor id")
}

func TestValidateAndBindPrincipalRejectsAuthenticatedPrincipalWithoutResolvedPrincipal(t *testing.T) {
	_, err := validateAndBindPrincipal(connectFrame{Role: "agent", ActorID: "spoofed"}, ConnectionPrincipal{
		Authenticated: true,
	})
	require.ErrorContains(t, err, "missing actor id")
}

func TestResolvePrincipalRequiresPrincipalResolver(t *testing.T) {
	server := &Server{}

	principal, err := server.resolvePrincipal(context.Background(), "valid-token")
	require.ErrorContains(t, err, "principal resolver required")
	require.Equal(t, ConnectionPrincipal{}, principal)
}

func TestExtractBearerToken(t *testing.T) {
	require.Equal(t, "abc123", extractBearerToken("Bearer abc123"))
	require.Equal(t, "", extractBearerToken("Basic abc123"))
	require.Equal(t, "", extractBearerToken(""))
	require.Equal(t, "", extractBearerToken("Bearer"))
}

func TestGatewayRecordOutboundDelegatesToHandler(t *testing.T) {
	log, err := db.NewSQLiteEventLog(t.TempDir() + "/events.db")
	require.NoError(t, err)
	defer log.Close()

	var sessionKey string
	var content string
	var principal ConnectionPrincipal
	server := &Server{
		Log: log,
		HandleOutboundMessage: func(_ context.Context, actor ConnectionPrincipal, key, body string) error {
			principal = actor
			sessionKey = key
			content = body
			return nil
		},
	}
	require.NoError(t, server.recordOutboundMessage(context.Background(), connectFrame{Role: "agent", ActorID: "agent-1"}, ConnectionPrincipal{
		Actor:         core.EventActor{Kind: "agent", ID: "agent-1"},
		Authenticated: true,
	}, []byte(`{"type":"message.outbound","session_key":"sess-1","content":{"text":"hello"}}`)))
	require.Equal(t, "sess-1", sessionKey)
	require.Equal(t, "hello", content)
	require.Equal(t, "agent-1", principal.Actor.ID)
	require.True(t, principal.Authenticated)

	events, err := log.Read(context.Background(), "local", 0, 10, false)
	require.NoError(t, err)
	require.Len(t, events, 1)
	require.Equal(t, core.FrameworkEventMessageOutbound, events[0].Type)
}

func TestGatewayRecordOutboundDoesNotLogWithoutHandler(t *testing.T) {
	log, err := db.NewSQLiteEventLog(t.TempDir() + "/events.db")
	require.NoError(t, err)
	defer log.Close()

	server := &Server{Log: log}
	require.NoError(t, server.recordOutboundMessage(context.Background(), connectFrame{Role: "agent", ActorID: "agent-1"}, ConnectionPrincipal{
		Actor:         core.EventActor{Kind: "agent", ID: "agent-1"},
		Authenticated: true,
	}, []byte(`{"type":"message.outbound","session_key":"sess-1","content":{"text":"hello"}}`)))

	events, err := log.Read(context.Background(), "local", 0, 10, false)
	require.NoError(t, err)
	require.Empty(t, events)
}

func TestGatewayRecordOutboundDoesNotLogWithoutSessionKey(t *testing.T) {
	log, err := db.NewSQLiteEventLog(t.TempDir() + "/events.db")
	require.NoError(t, err)
	defer log.Close()

	server := &Server{
		Log: log,
		HandleOutboundMessage: func(_ context.Context, _ ConnectionPrincipal, _ string, _ string) error {
			t.Fatal("handler should not be called without session key")
			return nil
		},
	}
	require.NoError(t, server.recordOutboundMessage(context.Background(), connectFrame{Role: "agent", ActorID: "agent-1"}, ConnectionPrincipal{
		Actor:         core.EventActor{Kind: "agent", ID: "agent-1"},
		Authenticated: true,
	}, []byte(`{"type":"message.outbound","content":{"text":"hello"}}`)))

	events, err := log.Read(context.Background(), "local", 0, 10, false)
	require.NoError(t, err)
	require.Empty(t, events)
}

func TestValidateAndBindPrincipalRejectsRoleMismatch(t *testing.T) {
	_, err := validateAndBindPrincipal(connectFrame{Role: "node"}, ConnectionPrincipal{
		Role:          "agent",
		Authenticated: true,
		Actor:         core.EventActor{Kind: "service_account", ID: "svc-1"},
	})
	require.Error(t, err)
}

func TestBindConnectionSessionIDGeneratesOpaqueSessionIDs(t *testing.T) {
	first, err := bindConnectionSessionID(ConnectionPrincipal{})
	require.NoError(t, err)
	second, err := bindConnectionSessionID(ConnectionPrincipal{})
	require.NoError(t, err)

	require.NotEmpty(t, connectionSessionID(first))
	require.NotEmpty(t, connectionSessionID(second))
	require.NotEqual(t, connectionSessionID(first), connectionSessionID(second))
	require.NotEqual(t, "agent-session", connectionSessionID(first))
}

func TestIsAdminPrincipalRequiresAdminScope(t *testing.T) {
	require.False(t, isAdminPrincipal(ConnectionPrincipal{
		Authenticated: true,
		Actor:         core.EventActor{Kind: "operator", ID: "svc-1"},
		Principal: &core.AuthenticatedPrincipal{
			Authenticated: true,
			Scopes:        []string{"session:send"},
		},
	}))

	require.True(t, isAdminPrincipal(ConnectionPrincipal{
		Authenticated: true,
		Actor:         core.EventActor{Kind: "agent", ID: "svc-1"},
		Principal: &core.AuthenticatedPrincipal{
			Authenticated: true,
			Scopes:        []string{"gateway:admin"},
		},
	}))
}

func TestConnectionFeedClassification(t *testing.T) {
	require.Equal(t, feedScopeRuntime, connectionFeed(ConnectionPrincipal{
		Authenticated: true,
		Actor:         core.EventActor{Kind: "agent", ID: "svc-1", TenantID: "tenant-a"},
	}))

	require.Equal(t, feedScopeTenantAdmin, connectionFeed(ConnectionPrincipal{
		Authenticated: true,
		Actor:         core.EventActor{Kind: "admin", ID: "admin-a", TenantID: "tenant-a"},
		Principal: &core.AuthenticatedPrincipal{
			Authenticated: true,
			Scopes:        []string{"gateway:admin"},
		},
	}))

	require.Equal(t, feedScopeGlobalAdmin, connectionFeed(ConnectionPrincipal{
		Authenticated: true,
		Actor:         core.EventActor{Kind: "admin", ID: "admin-a", TenantID: "tenant-a"},
		Principal: &core.AuthenticatedPrincipal{
			Authenticated: true,
			Scopes:        []string{"gateway:admin", "gateway:admin:global"},
		},
	}))
}

func TestDefaultCheckOrigin(t *testing.T) {
	req, err := http.NewRequest(http.MethodGet, "http://gateway.test", nil)
	require.NoError(t, err)
	req.Host = "gateway.test"
	req.Header.Set("Origin", "http://gateway.test")
	require.True(t, defaultCheckOrigin(req))

	req.Header.Set("Origin", "http://evil.test")
	require.False(t, defaultCheckOrigin(req))
}

func TestBroadcastClientSendAfterCloseReturnsFalse(t *testing.T) {
	bc := &broadcastClient{queue: make(chan any, 1)}
	bc.closeQueue()
	assert.NotPanics(t, func() {
		require.False(t, bc.send(map[string]any{"type": "event"}))
	})
}

func TestGatewayInvokeCapabilityFailsWhenClientQueueUnavailable(t *testing.T) {
	bc := &broadcastClient{queue: make(chan any, 1)}
	bc.queue <- map[string]any{"type": "already-buffered"}

	server := &Server{
		InvokeCapability: func(_ context.Context, principal ConnectionPrincipal, sessionKey, capabilityID string, args map[string]any) (*core.CapabilityExecutionResult, error) {
			require.Equal(t, "tenant-1", principal.Actor.TenantID)
			require.Equal(t, "sess-1", sessionKey)
			require.Equal(t, "remote.echo", capabilityID)
			require.Equal(t, "hi", args["text"])
			return &core.CapabilityExecutionResult{Success: true}, nil
		},
	}

	err := server.invokeCapability(context.Background(), bc, ConnectionPrincipal{Actor: core.EventActor{Kind: "agent", ID: "svc-1", TenantID: "tenant-1"}, Authenticated: true}, []byte(`{"type":"capability.invoke","correlation_id":"corr-1","session_key":"sess-1","capability_id":"remote.echo","args":{"text":"hi"}}`))
	require.Error(t, err)
	require.ErrorContains(t, err, "client queue unavailable")
}

func TestGatewayInvokeCapabilityRequiresSessionKey(t *testing.T) {
	bc := &broadcastClient{queue: make(chan any, 1)}
	server := &Server{
		InvokeCapability: func(_ context.Context, _ ConnectionPrincipal, _ string, _ string, _ map[string]any) (*core.CapabilityExecutionResult, error) {
			t.Fatal("invoke should not be called without session key")
			return nil, nil
		},
	}

	require.NoError(t, server.invokeCapability(context.Background(), bc, ConnectionPrincipal{
		Actor:         core.EventActor{Kind: "agent", ID: "svc-1", TenantID: "tenant-1"},
		Authenticated: true,
	}, []byte(`{"type":"capability.invoke","correlation_id":"corr-1","capability_id":"remote.echo","args":{"text":"hi"}}`)))

	response := <-bc.queue
	frame, ok := response.(map[string]any)
	require.True(t, ok)
	require.Equal(t, "capability.result", frame["type"])
	result, ok := frame["result"].(*core.CapabilityExecutionResult)
	require.True(t, ok)
	require.Equal(t, "capability invocation requires session_key", result.Error)
}

func TestGatewayHandleClientFrameRespondsToPing(t *testing.T) {
	bc := &broadcastClient{queue: make(chan any, 1)}
	server := &Server{}

	require.NoError(t, server.handleClientFrame(context.Background(), bc, connectFrame{Role: "agent"}, ConnectionPrincipal{
		Actor:         core.EventActor{Kind: "agent", ID: "svc-1"},
		Authenticated: true,
	}, []byte(`{"type":"ping"}`)))

	frame := (<-bc.queue).(map[string]any)
	require.Equal(t, "pong", frame["type"])
}

func TestGatewayHandleClientFrameSessionCloseReturnsEOF(t *testing.T) {
	bc := &broadcastClient{queue: make(chan any, 1)}
	server := &Server{}

	err := server.handleClientFrame(context.Background(), bc, connectFrame{Role: "agent"}, ConnectionPrincipal{
		Actor:         core.EventActor{Kind: "agent", ID: "svc-1"},
		Authenticated: true,
		Principal:     &core.AuthenticatedPrincipal{SessionID: "gw-1"},
	}, []byte(`{"type":"session.close"}`))
	require.ErrorIs(t, err, io.EOF)

	frame := (<-bc.queue).(map[string]any)
	require.Equal(t, "session.closed", frame["type"])
	require.Equal(t, "gw-1", frame["session_id"])
}

func TestBroadcastEventEvictsFullQueueClient(t *testing.T) {
	log, err := db.NewSQLiteEventLog(t.TempDir() + "/events.db")
	require.NoError(t, err)
	defer log.Close()

	server := &Server{Log: log}

	// Build a client whose queue is already full.
	bc := &broadcastClient{
		queue: make(chan any, broadcastQueueDepth),
		principal: ConnectionPrincipal{
			Authenticated: true,
			Actor:         core.EventActor{Kind: "agent", ID: "svc-1", TenantID: "tenant-1"},
			Principal:     &core.AuthenticatedPrincipal{Authenticated: true, Scopes: []string{"admin", "admin:global"}},
		},
	}
	for i := 0; i < broadcastQueueDepth; i++ {
		bc.queue <- map[string]any{"type": "dummy"}
	}

	server.mu.Lock()
	server.clients = map[*websocket.Conn]*broadcastClient{nil: bc}
	server.mu.Unlock()

	ev := core.FrameworkEvent{
		Seq:  1,
		Type: core.FrameworkEventSessionCreated,
	}
	server.broadcastEvent(context.Background(), ev)

	// The full client should have been evicted.
	server.mu.RLock()
	_, stillRegistered := server.clients[nil]
	server.mu.RUnlock()
	require.False(t, stillRegistered, "slow client should be evicted when its queue is full")
}

func TestBroadcastClientSendDropsWhenQueueFull(t *testing.T) {
	bc := &broadcastClient{queue: make(chan any, 1)}
	require.True(t, bc.send(map[string]any{"type": "first"}))
	// Second send should fail since queue is full (depth=1).
	require.False(t, bc.send(map[string]any{"type": "second"}))
	// Queue should now be closed (done=true).
	require.True(t, bc.done)
}

func TestBroadcastClientConcurrentCloseAndSendDoesNotPanic(t *testing.T) {
	for i := 0; i < 64; i++ {
		bc := &broadcastClient{queue: make(chan any, 1)}
		panicCh := make(chan any, 2)
		var wg sync.WaitGroup
		wg.Add(2)

		go func() {
			defer wg.Done()
			defer func() {
				if r := recover(); r != nil {
					panicCh <- r
				}
			}()
			bc.closeQueue()
		}()

		go func() {
			defer wg.Done()
			defer func() {
				if r := recover(); r != nil {
					panicCh <- r
				}
			}()
			_ = bc.send(map[string]any{"type": "event", "iteration": i})
		}()

		wg.Wait()
		select {
		case panicValue := <-panicCh:
			t.Fatalf("unexpected panic during concurrent close/send: %v", panicValue)
		default:
		}
	}
}
