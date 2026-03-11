package runtime

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/lexcodex/relurpify/framework/ast"
	fauthorization "github.com/lexcodex/relurpify/framework/authorization"
	"github.com/lexcodex/relurpify/framework/capability"
	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/framework/manifest"
	"github.com/lexcodex/relurpify/framework/memory"
	"github.com/lexcodex/relurpify/framework/memory/db"
	"github.com/lexcodex/relurpify/framework/middleware/mcp/protocol"
	mstdio "github.com/lexcodex/relurpify/framework/middleware/mcp/transport/stdio"
	"github.com/stretchr/testify/require"
)

type testProvider struct {
	initCalls  int
	closeCalls int
	initErr    error
	closeErr   error
	runtime    *Runtime
	desc       core.ProviderDescriptor
}

type testSessionProvider struct {
	testProvider
	closedSessions []string
}

type testSnapshotProvider struct {
	testProvider
	providerSnapshot []core.ProviderSnapshot
	sessionSnapshot  []core.ProviderSessionSnapshot
}

type fixtureMCPProcess struct {
	stdinW  *io.PipeWriter
	stdoutR *io.PipeReader
	stderrR *io.PipeReader
	waitCh  chan error
	pid     int
}

func (p *fixtureMCPProcess) Stdin() io.WriteCloser { return p.stdinW }
func (p *fixtureMCPProcess) Stdout() io.ReadCloser { return p.stdoutR }
func (p *fixtureMCPProcess) Stderr() io.ReadCloser { return p.stderrR }
func (p *fixtureMCPProcess) PID() int              { return p.pid }
func (p *fixtureMCPProcess) Wait() error           { return <-p.waitCh }
func (p *fixtureMCPProcess) Kill() error {
	select {
	case p.waitCh <- context.Canceled:
	default:
	}
	return nil
}

type fixtureMCPLauncher struct {
	process mstdio.Process
}

func (l fixtureMCPLauncher) Launch(context.Context, mstdio.Config) (mstdio.Process, error) {
	return l.process, nil
}

type fixtureMCPServer struct {
	mu        sync.Mutex
	encoder   *json.Encoder
	tools     []protocol.Tool
	prompts   []protocol.Prompt
	resources []protocol.Resource
	reqID     int
}

func newFixtureMCPServer() (*fixtureMCPServer, mstdio.Launcher) {
	clientToServerR, clientToServerW := io.Pipe()
	serverToClientR, serverToClientW := io.Pipe()
	stderrR, stderrW := io.Pipe()
	_ = stderrW.Close()
	server := &fixtureMCPServer{
		encoder: json.NewEncoder(serverToClientW),
		tools: []protocol.Tool{{
			Name:        "remote.echo",
			Description: "echo from fixture",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"message": map[string]any{"type": "string"},
				},
			},
		}},
		prompts:   []protocol.Prompt{{Name: "draft.summary", Description: "summary prompt"}},
		resources: []protocol.Resource{{URI: "file:///tmp/catalog.json", Name: "catalog"}},
	}
	go server.serve(clientToServerR)
	process := &fixtureMCPProcess{
		stdinW:  clientToServerW,
		stdoutR: serverToClientR,
		stderrR: stderrR,
		waitCh:  make(chan error, 1),
		pid:     2001,
	}
	return server, fixtureMCPLauncher{process: process}
}

func (s *fixtureMCPServer) serve(reader io.Reader) {
	decoder := json.NewDecoder(bufio.NewReader(reader))
	for {
		var envelope map[string]json.RawMessage
		if err := decoder.Decode(&envelope); err != nil {
			return
		}
		method := decodeString(envelope["method"])
		id := decodeString(envelope["id"])
		switch method {
		case "initialize":
			_ = s.encoder.Encode(map[string]any{
				"jsonrpc": "2.0",
				"id":      id,
				"result": protocol.InitializeResult{
					ProtocolVersion: protocol.Revision20250618,
					ServerInfo:      protocol.PeerInfo{Name: "fixture-mcp", Version: "1.0.0"},
					Capabilities: map[string]any{
						"tools":     map[string]any{"listChanged": true},
						"resources": map[string]any{"listChanged": true, "subscribe": true},
					},
				},
			})
		case "tools/list":
			s.mu.Lock()
			tools := append([]protocol.Tool(nil), s.tools...)
			s.mu.Unlock()
			_ = s.encoder.Encode(map[string]any{"jsonrpc": "2.0", "id": id, "result": protocol.ListToolsResult{Tools: tools}})
		case "prompts/list":
			s.mu.Lock()
			prompts := append([]protocol.Prompt(nil), s.prompts...)
			s.mu.Unlock()
			_ = s.encoder.Encode(map[string]any{"jsonrpc": "2.0", "id": id, "result": protocol.ListPromptsResult{Prompts: prompts}})
		case "resources/list":
			s.mu.Lock()
			resources := append([]protocol.Resource(nil), s.resources...)
			s.mu.Unlock()
			_ = s.encoder.Encode(map[string]any{"jsonrpc": "2.0", "id": id, "result": protocol.ListResourcesResult{Resources: resources}})
		case "tools/call":
			var params protocol.CallToolParams
			_ = json.Unmarshal(envelope["params"], &params)
			_ = s.encoder.Encode(map[string]any{
				"jsonrpc": "2.0",
				"id":      id,
				"result": protocol.CallToolResult{
					StructuredContent: map[string]any{"echo": params.Arguments["message"]},
					Content:           []protocol.ContentBlock{{Type: "text", Text: fmt.Sprint(params.Arguments["message"])}},
				},
			})
		case "resources/subscribe", "resources/unsubscribe":
			_ = s.encoder.Encode(map[string]any{"jsonrpc": "2.0", "id": id, "result": map[string]any{}})
		}
	}
}

func (s *fixtureMCPServer) setTools(tools []protocol.Tool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.tools = append([]protocol.Tool(nil), tools...)
}

func (s *fixtureMCPServer) notify(method string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.encoder.Encode(map[string]any{
		"jsonrpc": "2.0",
		"method":  method,
		"params":  map[string]any{},
	})
}

func (s *fixtureMCPServer) request(method string, params any) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.reqID++
	return s.encoder.Encode(map[string]any{
		"jsonrpc": "2.0",
		"id":      fmt.Sprintf("srv-%d", s.reqID),
		"method":  method,
		"params":  params,
	})
}

func decodeString(raw json.RawMessage) string {
	var value string
	_ = json.Unmarshal(raw, &value)
	return value
}

func encodeJSONBody(t *testing.T, payload map[string]any) io.Reader {
	t.Helper()
	data, err := json.Marshal(payload)
	require.NoError(t, err)
	return strings.NewReader(string(data))
}

func (p *testProvider) Initialize(_ context.Context, rt *Runtime) error {
	p.initCalls++
	p.runtime = rt
	return p.initErr
}

func (p *testProvider) Close() error {
	p.closeCalls++
	return p.closeErr
}

func (p *testProvider) Descriptor() core.ProviderDescriptor {
	return p.desc
}

func (p *testSessionProvider) CloseSession(_ context.Context, sessionID string) error {
	for _, existing := range p.closedSessions {
		if existing == sessionID {
			return nil
		}
	}
	if sessionID != "session-1" {
		return ErrSessionNotManaged
	}
	p.closedSessions = append(p.closedSessions, sessionID)
	return nil
}

func (p *testSnapshotProvider) SnapshotProvider(_ context.Context) (*core.ProviderSnapshot, error) {
	if len(p.providerSnapshot) == 0 {
		return nil, nil
	}
	snapshot := p.providerSnapshot[0]
	return &snapshot, nil
}

func (p *testSnapshotProvider) SnapshotSessions(_ context.Context) ([]core.ProviderSessionSnapshot, error) {
	return append([]core.ProviderSessionSnapshot(nil), p.sessionSnapshot...), nil
}

type runtimeProviderScopedTool struct {
	name      string
	provider  string
	sessionID string
}

func (t runtimeProviderScopedTool) Name() string        { return t.name }
func (t runtimeProviderScopedTool) Description() string { return "provider-scoped tool" }
func (t runtimeProviderScopedTool) Category() string    { return "testing" }
func (t runtimeProviderScopedTool) Parameters() []core.ToolParameter {
	return nil
}
func (t runtimeProviderScopedTool) Execute(context.Context, *core.Context, map[string]interface{}) (*core.ToolResult, error) {
	return &core.ToolResult{Success: true, Data: map[string]interface{}{"session_id": t.sessionID}}, nil
}
func (t runtimeProviderScopedTool) IsAvailable(context.Context, *core.Context) bool { return true }
func (t runtimeProviderScopedTool) Permissions() core.ToolPermissions {
	return core.ToolPermissions{Permissions: &core.PermissionSet{}}
}
func (t runtimeProviderScopedTool) Tags() []string { return nil }
func (t runtimeProviderScopedTool) CapabilitySource() core.CapabilitySource {
	return core.CapabilitySource{
		ProviderID: t.provider,
		Scope:      core.CapabilityScopeProvider,
		SessionID:  t.sessionID,
	}
}

type runtimeProviderScopedCapability struct {
	name      string
	provider  string
	sessionID string
}

func (c runtimeProviderScopedCapability) Descriptor(context.Context, *core.Context) core.CapabilityDescriptor {
	return core.NormalizeCapabilityDescriptor(core.CapabilityDescriptor{
		ID:            "provider:" + c.name,
		Kind:          core.CapabilityKindTool,
		Name:          c.name,
		RuntimeFamily: core.CapabilityRuntimeFamilyProvider,
		Description:   "provider-scoped capability",
		Source: core.CapabilitySource{
			ProviderID: c.provider,
			Scope:      core.CapabilityScopeProvider,
			SessionID:  c.sessionID,
		},
		TrustClass:   core.TrustClassProviderLocalUntrusted,
		Availability: core.AvailabilitySpec{Available: true},
	})
}

func (c runtimeProviderScopedCapability) Invoke(context.Context, *core.Context, map[string]interface{}) (*core.ToolResult, error) {
	return &core.ToolResult{Success: true, Data: map[string]interface{}{"session_id": c.sessionID}}, nil
}

type exportableInvocableCapability struct {
	desc   core.CapabilityDescriptor
	result *core.ToolResult
}

type exportablePromptCapability struct {
	desc   core.CapabilityDescriptor
	result *core.PromptRenderResult
}

type exportableResourceCapability struct {
	desc   core.CapabilityDescriptor
	result *core.ResourceReadResult
}

type mcpSamplingStubModel struct{}

func (mcpSamplingStubModel) Generate(context.Context, string, *core.LLMOptions) (*core.LLMResponse, error) {
	return &core.LLMResponse{Text: "generated"}, nil
}

func (mcpSamplingStubModel) GenerateStream(context.Context, string, *core.LLMOptions) (<-chan string, error) {
	ch := make(chan string)
	close(ch)
	return ch, nil
}

func (mcpSamplingStubModel) Chat(context.Context, []core.Message, *core.LLMOptions) (*core.LLMResponse, error) {
	return &core.LLMResponse{Text: "sampled-response", FinishReason: "stop"}, nil
}

func (mcpSamplingStubModel) ChatWithTools(context.Context, []core.Message, []core.LLMToolSpec, *core.LLMOptions) (*core.LLMResponse, error) {
	return &core.LLMResponse{Text: "sampled-response", FinishReason: "stop"}, nil
}

type mcpElicitationHandlerStub struct {
	result *protocol.ElicitationResult
}

func (h mcpElicitationHandlerStub) HandleMCPElicitation(context.Context, protocol.ElicitationParams) (*protocol.ElicitationResult, error) {
	return h.result, nil
}

func (c exportableInvocableCapability) Descriptor(context.Context, *core.Context) core.CapabilityDescriptor {
	return c.desc
}

func (c exportableInvocableCapability) Invoke(context.Context, *core.Context, map[string]interface{}) (*core.ToolResult, error) {
	return c.result, nil
}

func (c exportablePromptCapability) Descriptor(context.Context, *core.Context) core.CapabilityDescriptor {
	return c.desc
}

func (c exportablePromptCapability) RenderPrompt(context.Context, *core.Context, map[string]interface{}) (*core.PromptRenderResult, error) {
	return c.result, nil
}

func (c exportableResourceCapability) Descriptor(context.Context, *core.Context) core.CapabilityDescriptor {
	return c.desc
}

func (c exportableResourceCapability) ReadResource(context.Context, *core.Context) (*core.ResourceReadResult, error) {
	return c.result, nil
}

type testRuntimeCloser struct {
	closeCalls int
	err        error
}

func (c *testRuntimeCloser) Close() error {
	c.closeCalls++
	return c.err
}

type testIndexStore struct {
	closeCalls int
	err        error
}

func (s *testIndexStore) SaveFile(*ast.FileMetadata) error                    { return nil }
func (s *testIndexStore) GetFile(string) (*ast.FileMetadata, error)           { return nil, nil }
func (s *testIndexStore) GetFileByPath(string) (*ast.FileMetadata, error)     { return nil, nil }
func (s *testIndexStore) ListFiles(ast.Category) ([]*ast.FileMetadata, error) { return nil, nil }
func (s *testIndexStore) DeleteFile(string) error                             { return nil }
func (s *testIndexStore) SaveNodes([]*ast.Node) error                         { return nil }
func (s *testIndexStore) GetNode(string) (*ast.Node, error)                   { return nil, nil }
func (s *testIndexStore) GetNodesByFile(string) ([]*ast.Node, error)          { return nil, nil }
func (s *testIndexStore) GetNodesByType(ast.NodeType) ([]*ast.Node, error)    { return nil, nil }
func (s *testIndexStore) GetNodesByName(string) ([]*ast.Node, error)          { return nil, nil }
func (s *testIndexStore) SearchNodes(ast.NodeQuery) ([]*ast.Node, error)      { return nil, nil }
func (s *testIndexStore) DeleteNode(string) error                             { return nil }
func (s *testIndexStore) SaveEdges([]*ast.Edge) error                         { return nil }
func (s *testIndexStore) GetEdge(string) (*ast.Edge, error)                   { return nil, nil }
func (s *testIndexStore) GetEdgesBySource(string) ([]*ast.Edge, error)        { return nil, nil }
func (s *testIndexStore) GetEdgesByTarget(string) ([]*ast.Edge, error)        { return nil, nil }
func (s *testIndexStore) GetEdgesByType(ast.EdgeType) ([]*ast.Edge, error)    { return nil, nil }
func (s *testIndexStore) SearchEdges(ast.EdgeQuery) ([]*ast.Edge, error)      { return nil, nil }
func (s *testIndexStore) DeleteEdge(string) error                             { return nil }
func (s *testIndexStore) GetCallees(string) ([]*ast.Node, error)              { return nil, nil }
func (s *testIndexStore) GetCallers(string) ([]*ast.Node, error)              { return nil, nil }
func (s *testIndexStore) GetImports(string) ([]*ast.Node, error)              { return nil, nil }
func (s *testIndexStore) GetImportedBy(string) ([]*ast.Node, error)           { return nil, nil }
func (s *testIndexStore) GetReferences(string) ([]*ast.Node, error)           { return nil, nil }
func (s *testIndexStore) GetReferencedBy(string) ([]*ast.Node, error)         { return nil, nil }
func (s *testIndexStore) GetDependencies(string) ([]*ast.Node, error)         { return nil, nil }
func (s *testIndexStore) GetDependents(string) ([]*ast.Node, error)           { return nil, nil }
func (s *testIndexStore) BeginTransaction() (ast.Transaction, error)          { return nil, nil }
func (s *testIndexStore) Vacuum() error                                       { return nil }
func (s *testIndexStore) GetStats() (*ast.IndexStats, error)                  { return nil, nil }
func (s *testIndexStore) Close() error {
	s.closeCalls++
	return s.err
}

func TestRuntimeRegisterProviderInitializesAndStoresProvider(t *testing.T) {
	rt := &Runtime{}
	provider := &testProvider{}

	err := rt.RegisterProvider(context.Background(), provider)

	require.NoError(t, err)
	require.Equal(t, 1, provider.initCalls)
	require.Same(t, rt, provider.runtime)
	require.Len(t, rt.registeredProviders(), 1)
}

func TestRuntimeRegisterProviderDoesNotStoreFailedProvider(t *testing.T) {
	rt := &Runtime{}
	provider := &testProvider{initErr: errors.New("init failed")}

	err := rt.RegisterProvider(context.Background(), provider)

	require.ErrorContains(t, err, "init failed")
	require.Equal(t, 1, provider.initCalls)
	require.Empty(t, rt.registeredProviders())
}

func TestRuntimeCloseClosesProvidersRegistryIndexAndLog(t *testing.T) {
	logCloser := &testRuntimeCloser{}
	registryCloser := &testRuntimeCloser{}
	indexStore := &testIndexStore{}
	rt := &Runtime{
		Context:      core.NewContext(),
		IndexManager: ast.NewIndexManager(indexStore, ast.IndexConfig{}),
		logFile:      logCloser,
	}
	rt.Context.SetHandleScoped("browser.session", registryCloser, "task-1")
	first := &testProvider{}
	second := &testProvider{}
	require.NoError(t, rt.RegisterProvider(context.Background(), first))
	require.NoError(t, rt.RegisterProvider(context.Background(), second))

	err := rt.Close()

	require.NoError(t, err)
	require.Equal(t, 1, second.closeCalls)
	require.Equal(t, 1, first.closeCalls)
	require.Equal(t, 1, registryCloser.closeCalls)
	require.Equal(t, 1, indexStore.closeCalls)
	require.Equal(t, 1, logCloser.closeCalls)
}

func TestRuntimeCloseJoinsErrors(t *testing.T) {
	providerErr := errors.New("provider close failed")
	registryErr := errors.New("registry close failed")
	logErr := errors.New("log close failed")
	rt := &Runtime{
		Context: core.NewContext(),
		logFile: &testRuntimeCloser{err: logErr},
	}
	rt.Context.SetHandleScoped("browser.session", &testRuntimeCloser{err: registryErr}, "task-2")
	require.NoError(t, rt.RegisterProvider(context.Background(), &testProvider{closeErr: providerErr}))

	err := rt.Close()

	require.Error(t, err)
	require.ErrorContains(t, err, providerErr.Error())
	require.ErrorContains(t, err, registryErr.Error())
	require.ErrorContains(t, err, logErr.Error())
}

func TestRuntimeRegisterProviderHonorsProviderActivationPolicy(t *testing.T) {
	hitl := fauthorization.NewHITLBroker(time.Second)
	perms, err := fauthorization.NewPermissionManager(t.TempDir(), &core.PermissionSet{
		FileSystem: []core.FileSystemPermission{{Action: core.FileSystemRead, Path: "**"}},
	}, core.NewInMemoryAuditLogger(10), hitl)
	require.NoError(t, err)

	rt := &Runtime{
		AgentSpec: &core.AgentRuntimeSpec{
			ProviderPolicies: map[string]core.ProviderPolicy{
				"remote-mcp": {Activate: core.AgentPermissionDeny},
			},
		},
		Registration: &fauthorization.AgentRegistration{
			ID:          "agent-1",
			Permissions: perms,
			HITL:        hitl,
		},
	}
	provider := &testProvider{
		desc: core.ProviderDescriptor{
			ID:            "remote-mcp",
			Kind:          core.ProviderKindMCPClient,
			TrustBaseline: core.TrustClassRemoteDeclared,
			Security: core.ProviderSecurityProfile{
				Origin:                     core.ProviderOriginRemote,
				RequiresFrameworkMediation: true,
			},
		},
	}

	err = rt.RegisterProvider(context.Background(), provider)

	require.ErrorContains(t, err, "activation denied by policy")
	require.Zero(t, provider.initCalls)
}

func TestRuntimeRegisterProviderCanBeApprovedThroughHITL(t *testing.T) {
	hitl := fauthorization.NewHITLBroker(2 * time.Second)
	perms, err := fauthorization.NewPermissionManager(t.TempDir(), &core.PermissionSet{
		FileSystem: []core.FileSystemPermission{{Action: core.FileSystemRead, Path: "**"}},
	}, core.NewInMemoryAuditLogger(10), hitl)
	require.NoError(t, err)

	rt := &Runtime{
		Registration: &fauthorization.AgentRegistration{
			ID:          "agent-1",
			Permissions: perms,
			HITL:        hitl,
		},
	}
	provider := &testProvider{
		desc: core.ProviderDescriptor{
			ID:            "remote-mcp",
			Kind:          core.ProviderKindMCPClient,
			TrustBaseline: core.TrustClassRemoteDeclared,
			Security: core.ProviderSecurityProfile{
				Origin:                     core.ProviderOriginRemote,
				RequiresFrameworkMediation: true,
			},
		},
	}

	go func() {
		for {
			pending := hitl.PendingRequests()
			if len(pending) == 0 {
				time.Sleep(10 * time.Millisecond)
				continue
			}
			_ = hitl.Approve(fauthorization.PermissionDecision{
				RequestID:  pending[0].ID,
				Approved:   true,
				ApprovedBy: "tester",
				Scope:      fauthorization.GrantScopeSession,
			})
			return
		}
	}()

	err = rt.RegisterProvider(context.Background(), provider)

	require.NoError(t, err)
	require.Equal(t, 1, provider.initCalls)
}

func TestRuntimeRegisterProviderPolicyEnginePreservesRemoteApprovalFallback(t *testing.T) {
	hitl := fauthorization.NewHITLBroker(2 * time.Second)
	perms, err := fauthorization.NewPermissionManager(t.TempDir(), &core.PermissionSet{
		FileSystem: []core.FileSystemPermission{{Action: core.FileSystemRead, Path: "**"}},
	}, core.NewInMemoryAuditLogger(10), hitl)
	require.NoError(t, err)
	engine, err := fauthorization.FromManifestWithConfig(&manifest.AgentManifest{
		Metadata: manifest.ManifestMetadata{Name: "agent-1"},
		Spec: manifest.ManifestSpec{
			Agent: &core.AgentRuntimeSpec{
				Mode: core.AgentModePrimary,
				Model: core.AgentModelConfig{
					Provider: "ollama",
					Name:     "test",
				},
			},
		},
	}, "agent-1", perms)
	require.NoError(t, err)

	rt := &Runtime{
		Registration: &fauthorization.AgentRegistration{
			ID:          "agent-1",
			Permissions: perms,
			Policy:      engine,
			HITL:        hitl,
		},
	}
	provider := &testProvider{
		desc: core.ProviderDescriptor{
			ID:            "remote-mcp",
			Kind:          core.ProviderKindMCPClient,
			TrustBaseline: core.TrustClassRemoteDeclared,
			Security: core.ProviderSecurityProfile{
				Origin:                     core.ProviderOriginRemote,
				RequiresFrameworkMediation: true,
			},
		},
	}

	go func() {
		for {
			pending := hitl.PendingRequests()
			if len(pending) == 0 {
				time.Sleep(10 * time.Millisecond)
				continue
			}
			_ = hitl.Approve(fauthorization.PermissionDecision{
				RequestID:  pending[0].ID,
				Approved:   true,
				ApprovedBy: "tester",
				Scope:      fauthorization.GrantScopeSession,
			})
			return
		}
	}()

	err = rt.RegisterProvider(context.Background(), provider)

	require.NoError(t, err)
	require.Equal(t, 1, provider.initCalls)
}

func TestRuntimeCaptureAndPersistProviderSnapshots(t *testing.T) {
	rt := &Runtime{}
	provider := &testSnapshotProvider{
		testProvider: testProvider{
			desc: core.ProviderDescriptor{
				ID:                 "browser",
				Kind:               core.ProviderKindAgentRuntime,
				TrustBaseline:      core.TrustClassProviderLocalUntrusted,
				RecoverabilityMode: core.RecoverabilityInProcess,
				Security:           core.ProviderSecurityProfile{Origin: core.ProviderOriginLocal},
			},
		},
		providerSnapshot: []core.ProviderSnapshot{{
			ProviderID:     "browser",
			Recoverability: core.RecoverabilityInProcess,
			Descriptor: core.ProviderDescriptor{
				ID:                 "browser",
				Kind:               core.ProviderKindAgentRuntime,
				TrustBaseline:      core.TrustClassProviderLocalUntrusted,
				RecoverabilityMode: core.RecoverabilityInProcess,
				Security:           core.ProviderSecurityProfile{Origin: core.ProviderOriginLocal},
			},
			Metadata:   map[string]any{"token": "secret"},
			State:      map[string]any{"backend": "cdp"},
			CapturedAt: time.Now().UTC().Format(time.RFC3339Nano),
		}},
		sessionSnapshot: []core.ProviderSessionSnapshot{{
			Session: core.ProviderSession{
				ID:             "session-1",
				ProviderID:     "browser",
				Recoverability: core.RecoverabilityInProcess,
				WorkflowID:     "wf-1",
			},
			State:      map[string]any{"url": "https://example.com"},
			CapturedAt: time.Now().UTC().Format(time.RFC3339Nano),
		}},
	}

	require.NoError(t, rt.RegisterProvider(context.Background(), provider))

	providers, sessions, err := rt.CaptureProviderSnapshots(context.Background())
	require.NoError(t, err)
	require.Len(t, providers, 1)
	require.Len(t, sessions, 1)
	require.Equal(t, "browser", providers[0].ProviderID)
	require.Equal(t, "session-1", sessions[0].Session.ID)

	store, err := db.NewSQLiteWorkflowStateStore(filepath.Join(t.TempDir(), "workflow.db"))
	require.NoError(t, err)
	defer store.Close()
	ctx := context.Background()
	require.NoError(t, store.CreateWorkflow(ctx, memory.WorkflowRecord{
		WorkflowID:  "wf-1",
		TaskID:      "task-1",
		TaskType:    core.TaskTypeCodeModification,
		Instruction: "persist provider snapshots",
	}))
	require.NoError(t, store.CreateRun(ctx, memory.WorkflowRunRecord{
		RunID:      "run-1",
		WorkflowID: "wf-1",
		Status:     memory.WorkflowRunStatusRunning,
	}))

	require.NoError(t, rt.PersistProviderSnapshots(ctx, store, "wf-1", "run-1"))

	persistedProviders, err := store.ListProviderSnapshots(ctx, "wf-1", "run-1")
	require.NoError(t, err)
	require.Len(t, persistedProviders, 1)
	require.Equal(t, "[REDACTED]", persistedProviders[0].Metadata["token"])

	persistedSessions, err := store.ListProviderSessionSnapshots(ctx, "wf-1", "run-1")
	require.NoError(t, err)
	require.Len(t, persistedSessions, 1)
	require.Equal(t, "session-1", persistedSessions[0].Session.ID)
}

func TestRegisterBuiltinProvidersRegistersConfiguredMCPProviders(t *testing.T) {
	server, launcher := newFixtureMCPServer()
	prevFactory := mcpClientLauncherFactory
	mcpClientLauncherFactory = func(core.ProviderConfig) mstdio.Launcher { return launcher }
	defer func() { mcpClientLauncherFactory = prevFactory }()

	registry := capability.NewRegistry()
	rt := &Runtime{
		Tools: registry,
		AgentSpec: &core.AgentRuntimeSpec{
			ProviderPolicies: map[string]core.ProviderPolicy{
				"remote-mcp":       {Activate: core.AgentPermissionAllow},
				"local-mcp-export": {Activate: core.AgentPermissionAllow},
			},
			Providers: []core.ProviderConfig{
				{
					ID:             "remote-mcp",
					Kind:           core.ProviderKindMCPClient,
					Enabled:        true,
					Target:         "stdio://fixture",
					Recoverability: core.RecoverabilityPersistedRestore,
					Config: map[string]any{
						"command": "fixture-mcp",
					},
				},
				{
					ID:             "local-mcp-export",
					Kind:           core.ProviderKindMCPServer,
					Enabled:        true,
					Target:         "stdio://local",
					Recoverability: core.RecoverabilityPersistedRestore,
				},
			},
		},
	}

	require.NoError(t, RegisterBuiltinProviders(context.Background(), rt))

	providers := rt.registeredProviders()
	require.Len(t, providers, 2)
	require.True(t, registry.HasCapability("resource:remote-mcp:catalog"))
	require.True(t, registry.HasCapability("resource:local-mcp-export:exports"))
	require.True(t, registry.HasCapability("session:remote-mcp:primary"))
	require.True(t, registry.HasCapability("mcp:remote-mcp:tool:remote.echo"))
	require.True(t, registry.HasCapability("mcp:remote-mcp:prompt:draft.summary"))
	require.True(t, registry.HasCapability("mcp:remote-mcp:resource:file____tmp_catalog_json"))

	_ = server
}

func TestMCPProviderSnapshotsExposeConfiguredState(t *testing.T) {
	config := core.ProviderConfig{
		ID:             "remote-mcp",
		Kind:           core.ProviderKindMCPClient,
		Enabled:        true,
		Target:         "stdio://fixture",
		Recoverability: core.RecoverabilityPersistedRestore,
		Config: map[string]any{
			"command":          "fixture-mcp",
			"protocol_version": protocol.Revision20250618,
		},
	}
	provider, err := providerFromConfig(config)
	require.NoError(t, err)
	server, launcher := newFixtureMCPServer()
	clientProvider, ok := provider.(*mcpClientProvider)
	require.True(t, ok)
	clientProvider.launcher = launcher

	rt := &Runtime{
		Tools: capability.NewRegistry(),
		AgentSpec: &core.AgentRuntimeSpec{
			ProviderPolicies: map[string]core.ProviderPolicy{
				"remote-mcp": {Activate: core.AgentPermissionAllow},
			},
		},
	}
	require.NoError(t, rt.RegisterProvider(context.Background(), provider))

	snapshots, sessions, err := rt.CaptureProviderSnapshots(context.Background())
	require.NoError(t, err)
	require.Len(t, snapshots, 1)
	require.Equal(t, "remote-mcp", snapshots[0].ProviderID)
	require.Equal(t, "stdio://fixture", snapshots[0].Metadata["target"])
	require.Len(t, sessions, 1)
	require.Equal(t, "remote-mcp:primary", sessions[0].Session.ID)
	require.Equal(t, protocol.Revision20250618, sessions[0].Session.Metadata["protocol_version"])
	require.Equal(t, "fixture-mcp", sessions[0].Session.Metadata["remote_peer_name"])
	_ = server
}

func TestMCPClientProviderCallableExposureAndInvocation(t *testing.T) {
	server, launcher := newFixtureMCPServer()
	prevFactory := mcpClientLauncherFactory
	mcpClientLauncherFactory = func(core.ProviderConfig) mstdio.Launcher { return launcher }
	defer func() { mcpClientLauncherFactory = prevFactory }()

	provider, err := providerFromConfig(core.ProviderConfig{
		ID:             "remote-mcp",
		Kind:           core.ProviderKindMCPClient,
		Enabled:        true,
		Target:         "stdio://fixture",
		Recoverability: core.RecoverabilityPersistedRestore,
		Config: map[string]any{
			"command": "fixture-mcp",
		},
	})
	require.NoError(t, err)

	registry := capability.NewRegistry()
	registry.UseAgentSpec("agent", &core.AgentRuntimeSpec{
		ProviderPolicies: map[string]core.ProviderPolicy{
			"remote-mcp": {Activate: core.AgentPermissionAllow},
		},
		ExposurePolicies: []core.CapabilityExposurePolicy{{
			Selector: core.CapabilitySelector{
				ID: "mcp:remote-mcp:tool:remote.echo",
			},
			Access: core.CapabilityExposureCallable,
		}},
	})
	rt := &Runtime{
		Tools: registry,
		AgentSpec: &core.AgentRuntimeSpec{
			ProviderPolicies: map[string]core.ProviderPolicy{
				"remote-mcp": {Activate: core.AgentPermissionAllow},
			},
			ExposurePolicies: []core.CapabilityExposurePolicy{{
				Selector: core.CapabilitySelector{
					ID: "mcp:remote-mcp:tool:remote.echo",
				},
				Access: core.CapabilityExposureCallable,
			}},
		},
	}

	require.NoError(t, rt.RegisterProvider(context.Background(), provider))
	require.True(t, registry.HasCapability("mcp:remote-mcp:tool:remote.echo"))

	result, err := registry.InvokeCapability(context.Background(), core.NewContext(), "mcp:remote-mcp:tool:remote.echo", map[string]interface{}{"message": "hello"})
	require.NoError(t, err)
	require.Equal(t, "hello", result.Data["echo"])
	_ = server
}

func TestMCPClientProviderResyncsOnListChangedNotification(t *testing.T) {
	server, launcher := newFixtureMCPServer()
	prevFactory := mcpClientLauncherFactory
	mcpClientLauncherFactory = func(core.ProviderConfig) mstdio.Launcher { return launcher }
	defer func() { mcpClientLauncherFactory = prevFactory }()

	provider, err := providerFromConfig(core.ProviderConfig{
		ID:             "remote-mcp",
		Kind:           core.ProviderKindMCPClient,
		Enabled:        true,
		Target:         "stdio://fixture",
		Recoverability: core.RecoverabilityPersistedRestore,
		Config: map[string]any{
			"command": "fixture-mcp",
		},
	})
	require.NoError(t, err)

	registry := capability.NewRegistry()
	rt := &Runtime{
		Tools: registry,
		AgentSpec: &core.AgentRuntimeSpec{
			ProviderPolicies: map[string]core.ProviderPolicy{
				"remote-mcp": {Activate: core.AgentPermissionAllow},
			},
		},
	}
	require.NoError(t, rt.RegisterProvider(context.Background(), provider))
	require.False(t, registry.HasCapability("mcp:remote-mcp:tool:remote.search"))

	server.setTools([]protocol.Tool{
		{Name: "remote.echo", Description: "echo"},
		{Name: "remote.search", Description: "search"},
	})
	require.NoError(t, server.notify("notifications/tools/list_changed"))
	require.Eventually(t, func() bool {
		return registry.HasCapability("mcp:remote-mcp:tool:remote.search")
	}, time.Second, 10*time.Millisecond)
}

func TestMCPClientProviderImportsConfiguredCoordinationService(t *testing.T) {
	server, launcher := newFixtureMCPServer()
	prevFactory := mcpClientLauncherFactory
	mcpClientLauncherFactory = func(core.ProviderConfig) mstdio.Launcher { return launcher }
	defer func() { mcpClientLauncherFactory = prevFactory }()

	provider, err := providerFromConfig(core.ProviderConfig{
		ID:             "remote-mcp",
		Kind:           core.ProviderKindMCPClient,
		Enabled:        true,
		Target:         "stdio://fixture",
		Recoverability: core.RecoverabilityPersistedRestore,
		Config: map[string]any{
			"command": "fixture-mcp",
			"coordination_tools": map[string]any{
				"remote.echo": map[string]any{
					"role":       "reviewer",
					"task_types": []any{"review"},
				},
			},
		},
	})
	require.NoError(t, err)

	registry := capability.NewRegistry()
	rt := &Runtime{
		Tools: registry,
		AgentSpec: &core.AgentRuntimeSpec{
			ProviderPolicies: map[string]core.ProviderPolicy{
				"remote-mcp": {Activate: core.AgentPermissionAllow},
			},
		},
	}
	require.NoError(t, rt.RegisterProvider(context.Background(), provider))

	target, ok := registry.GetCoordinationTarget("remote.echo")
	require.True(t, ok)
	require.Equal(t, core.CoordinationRoleReviewer, target.Coordination.Role)
	require.Equal(t, []string{"review"}, target.Coordination.TaskTypes)
	require.Equal(t, core.CapabilityExposureInspectable, registry.EffectiveExposure(target))
	_ = server
}

func TestMCPClientProviderImportsResourceSubscriptionCapability(t *testing.T) {
	server, launcher := newFixtureMCPServer()
	prevFactory := mcpClientLauncherFactory
	mcpClientLauncherFactory = func(core.ProviderConfig) mstdio.Launcher { return launcher }
	defer func() { mcpClientLauncherFactory = prevFactory }()

	provider, err := providerFromConfig(core.ProviderConfig{
		ID:      "remote-mcp",
		Kind:    core.ProviderKindMCPClient,
		Enabled: true,
		Target:  "stdio://fixture",
		Config: map[string]any{
			"command":                       "fixture-mcp",
			"enable_resource_subscriptions": true,
		},
	})
	require.NoError(t, err)

	registry := capability.NewRegistry()
	rt := &Runtime{
		Tools: registry,
		AgentSpec: &core.AgentRuntimeSpec{
			ProviderPolicies: map[string]core.ProviderPolicy{
				"remote-mcp": {Activate: core.AgentPermissionAllow},
			},
		},
	}
	require.NoError(t, rt.RegisterProvider(context.Background(), provider))

	var subscriptionID string
	for _, capability := range registry.AllCapabilities() {
		if capability.Kind == core.CapabilityKindSubscription {
			subscriptionID = capability.ID
			break
		}
	}
	require.NotEmpty(t, subscriptionID)

	_, err = registry.InvokeCapability(context.Background(), core.NewContext(), subscriptionID, map[string]interface{}{"action": "subscribe"})
	require.NoError(t, err)

	snapshots, err := provider.(core.ProviderSessionSnapshotter).SnapshotSessions(context.Background())
	require.NoError(t, err)
	require.Len(t, snapshots, 1)
	state := snapshots[0].State.(map[string]any)
	require.Contains(t, state["active_subscriptions"], "file:///tmp/catalog.json")
	_ = server
}

func TestMCPClientProviderHandlesSamplingAndElicitationRequests(t *testing.T) {
	server, launcher := newFixtureMCPServer()
	prevFactory := mcpClientLauncherFactory
	mcpClientLauncherFactory = func(core.ProviderConfig) mstdio.Launcher { return launcher }
	defer func() { mcpClientLauncherFactory = prevFactory }()

	provider, err := providerFromConfig(core.ProviderConfig{
		ID:      "remote-mcp",
		Kind:    core.ProviderKindMCPClient,
		Enabled: true,
		Target:  "stdio://fixture",
		Config: map[string]any{
			"command":            "fixture-mcp",
			"enable_sampling":    true,
			"enable_elicitation": true,
		},
	})
	require.NoError(t, err)

	rt := &Runtime{
		Config: Config{OllamaModel: "test-model"},
		Tools:  capability.NewRegistry(),
		Model:  mcpSamplingStubModel{},
		AgentSpec: &core.AgentRuntimeSpec{
			ProviderPolicies: map[string]core.ProviderPolicy{
				"remote-mcp": {Activate: core.AgentPermissionAllow},
			},
		},
	}
	rt.SetMCPElicitationHandler(mcpElicitationHandlerStub{
		result: &protocol.ElicitationResult{Action: "accept", Content: map[string]any{"topic": "MCP"}},
	})
	require.NoError(t, rt.RegisterProvider(context.Background(), provider))

	require.NoError(t, server.request("sampling/createMessage", protocol.CreateMessageParams{
		Messages: []protocol.SamplingMessage{{Role: "user", Content: protocol.ContentBlock{Type: "text", Text: "hello"}}},
	}))
	require.NoError(t, server.request("elicitation/create", protocol.ElicitationParams{
		Message: "Need more detail",
	}))
}

func TestMCPServerProviderExportsSelectedCapabilitiesAndTracksPeerSessions(t *testing.T) {
	registry := capability.NewRegistry()
	require.NoError(t, registry.RegisterInvocableCapability(exportableInvocableCapability{
		desc: core.CapabilityDescriptor{
			ID:            "relurpic:echo",
			Kind:          core.CapabilityKindTool,
			RuntimeFamily: core.CapabilityRuntimeFamilyRelurpic,
			Name:          "echo",
			Description:   "Echo tool",
			InputSchema: &core.Schema{
				Type: "object",
				Properties: map[string]*core.Schema{
					"message": {Type: "string", Description: "Message"},
				},
				Required: []string{"message"},
			},
			Availability: core.AvailabilitySpec{Available: true},
			TrustClass:   core.TrustClassBuiltinTrusted,
		},
		result: &core.ToolResult{Success: true, Data: map[string]interface{}{"echo": "hello"}},
	}))
	require.NoError(t, registry.RegisterPromptCapability(exportablePromptCapability{
		desc: core.CapabilityDescriptor{
			ID:            "prompt:draft.summary",
			Kind:          core.CapabilityKindPrompt,
			RuntimeFamily: core.CapabilityRuntimeFamilyRelurpic,
			Name:          "draft.summary",
			Description:   "Summary prompt",
			InputSchema: &core.Schema{
				Type: "object",
				Properties: map[string]*core.Schema{
					"topic": {Type: "string"},
				},
			},
			TrustClass:   core.TrustClassBuiltinTrusted,
			Availability: core.AvailabilitySpec{Available: true},
		},
		result: &core.PromptRenderResult{
			Description: "Summary prompt",
			Messages: []core.PromptMessage{{
				Content: []core.ContentBlock{core.TextContentBlock{Text: "Summarize MCP"}},
			}},
		},
	}))
	require.NoError(t, registry.RegisterResourceCapability(exportableResourceCapability{
		desc: core.CapabilityDescriptor{
			ID:            "resource:docs",
			Kind:          core.CapabilityKindResource,
			RuntimeFamily: core.CapabilityRuntimeFamilyRelurpic,
			Name:          "docs",
			Description:   "Documentation",
			TrustClass:    core.TrustClassBuiltinTrusted,
			Availability:  core.AvailabilitySpec{Available: true},
			Annotations: map[string]any{
				"mcp_uri":   "file:///docs/guide.md",
				"mime_type": "text/markdown",
			},
		},
		result: &core.ResourceReadResult{
			Contents: []core.ContentBlock{core.TextContentBlock{Text: "guide contents"}},
		},
	}))

	provider, err := providerFromConfig(core.ProviderConfig{
		ID:             "local-mcp-export",
		Kind:           core.ProviderKindMCPServer,
		Enabled:        true,
		Target:         "stdio://local",
		Recoverability: core.RecoverabilityPersistedRestore,
		Config: map[string]any{
			"export_tools":     []any{"echo"},
			"export_prompts":   []any{"draft.summary"},
			"export_resources": []any{"docs"},
		},
	})
	require.NoError(t, err)
	rt := &Runtime{
		Tools: registry,
		AgentSpec: &core.AgentRuntimeSpec{
			ProviderPolicies: map[string]core.ProviderPolicy{
				"local-mcp-export": {Activate: core.AgentPermissionAllow},
			},
		},
	}
	require.NoError(t, rt.RegisterProvider(context.Background(), provider))
	serverProvider, ok := provider.(*mcpServerProvider)
	require.True(t, ok)

	conn, peerID, err := serverProvider.openLoopbackSession(context.Background())
	require.NoError(t, err)
	defer conn.Close()
	enc := json.NewEncoder(conn)
	dec := json.NewDecoder(bufio.NewReader(conn))

	require.NoError(t, enc.Encode(map[string]any{
		"jsonrpc": "2.0", "id": "1", "method": "initialize",
		"params": protocol.InitializeRequest{ProtocolVersion: protocol.Revision20250618},
	}))
	var initResp map[string]any
	require.NoError(t, dec.Decode(&initResp))

	require.NoError(t, enc.Encode(map[string]any{"jsonrpc": "2.0", "method": "notifications/initialized", "params": map[string]any{}}))
	require.NoError(t, enc.Encode(map[string]any{"jsonrpc": "2.0", "id": "2", "method": "tools/list", "params": map[string]any{}}))
	var toolsResp struct {
		ID     string                   `json:"id"`
		Result protocol.ListToolsResult `json:"result"`
	}
	require.NoError(t, dec.Decode(&toolsResp))
	require.Len(t, toolsResp.Result.Tools, 1)
	require.Equal(t, "echo", toolsResp.Result.Tools[0].Name)

	require.NoError(t, enc.Encode(map[string]any{
		"jsonrpc": "2.0", "id": "3", "method": "tools/call",
		"params": protocol.CallToolParams{Name: "echo", Arguments: map[string]any{"message": "ignored"}},
	}))
	var callResp struct {
		ID     string                  `json:"id"`
		Result protocol.CallToolResult `json:"result"`
	}
	require.NoError(t, dec.Decode(&callResp))
	require.Equal(t, "hello", callResp.Result.StructuredContent["echo"])

	require.NoError(t, enc.Encode(map[string]any{"jsonrpc": "2.0", "id": "4", "method": "prompts/list", "params": map[string]any{}}))
	var promptsResp struct {
		ID     string                     `json:"id"`
		Result protocol.ListPromptsResult `json:"result"`
	}
	require.NoError(t, dec.Decode(&promptsResp))
	require.Len(t, promptsResp.Result.Prompts, 1)
	require.Equal(t, "draft.summary", promptsResp.Result.Prompts[0].Name)

	require.NoError(t, enc.Encode(map[string]any{
		"jsonrpc": "2.0", "id": "5", "method": "prompts/get",
		"params": protocol.GetPromptParams{Name: "draft.summary", Arguments: map[string]any{"topic": "MCP"}},
	}))
	var promptResp struct {
		ID     string                   `json:"id"`
		Result protocol.GetPromptResult `json:"result"`
	}
	require.NoError(t, dec.Decode(&promptResp))
	require.Equal(t, "Summarize MCP", promptResp.Result.Messages[0].Text)

	require.NoError(t, enc.Encode(map[string]any{"jsonrpc": "2.0", "id": "6", "method": "resources/list", "params": map[string]any{}}))
	var resourcesResp struct {
		ID     string                       `json:"id"`
		Result protocol.ListResourcesResult `json:"result"`
	}
	require.NoError(t, dec.Decode(&resourcesResp))
	require.Len(t, resourcesResp.Result.Resources, 1)
	require.Equal(t, "file:///docs/guide.md", resourcesResp.Result.Resources[0].URI)

	require.NoError(t, enc.Encode(map[string]any{
		"jsonrpc": "2.0", "id": "7", "method": "resources/read",
		"params": protocol.ReadResourceParams{URI: "file:///docs/guide.md"},
	}))
	var resourceResp struct {
		ID     string                      `json:"id"`
		Result protocol.ReadResourceResult `json:"result"`
	}
	require.NoError(t, dec.Decode(&resourceResp))
	require.Equal(t, "guide contents", resourceResp.Result.Contents[0].Text)

	sessions, err := serverProvider.ListSessions(context.Background())
	require.NoError(t, err)
	require.Len(t, sessions, 2)
	found := false
	for _, session := range sessions {
		if session.ID == peerID {
			found = true
			require.Equal(t, "initialized", session.Health)
			require.Equal(t, protocol.Revision20250618, session.Metadata["protocol_version"])
		}
	}
	require.True(t, found)
}

func TestMCPServerProviderExportsSelectorsFromManifestConfig(t *testing.T) {
	registry := capability.NewRegistry()
	require.NoError(t, registry.RegisterInvocableCapability(exportableInvocableCapability{
		desc: core.CapabilityDescriptor{
			ID:            "relurpic:echo",
			Kind:          core.CapabilityKindTool,
			RuntimeFamily: core.CapabilityRuntimeFamilyRelurpic,
			Name:          "echo",
			Description:   "Echo tool",
			Category:      "testing",
			TrustClass:    core.TrustClassBuiltinTrusted,
			Availability:  core.AvailabilitySpec{Available: true},
		},
		result: &core.ToolResult{Success: true, Data: map[string]interface{}{"echo": "hello"}},
	}))
	require.NoError(t, registry.RegisterPromptCapability(exportablePromptCapability{
		desc: core.CapabilityDescriptor{
			ID:            "prompt:draft.summary",
			Kind:          core.CapabilityKindPrompt,
			RuntimeFamily: core.CapabilityRuntimeFamilyRelurpic,
			Name:          "draft.summary",
			TrustClass:    core.TrustClassBuiltinTrusted,
			Availability:  core.AvailabilitySpec{Available: true},
		},
		result: &core.PromptRenderResult{
			Messages: []core.PromptMessage{{Content: []core.ContentBlock{core.TextContentBlock{Text: "summary"}}}},
		},
	}))
	require.NoError(t, registry.RegisterResourceCapability(exportableResourceCapability{
		desc: core.CapabilityDescriptor{
			ID:            "resource:docs",
			Kind:          core.CapabilityKindResource,
			RuntimeFamily: core.CapabilityRuntimeFamilyRelurpic,
			Name:          "docs",
			TrustClass:    core.TrustClassBuiltinTrusted,
			Availability:  core.AvailabilitySpec{Available: true},
		},
		result: &core.ResourceReadResult{
			Contents: []core.ContentBlock{core.TextContentBlock{Text: "guide"}},
		},
	}))

	provider, err := providerFromConfig(core.ProviderConfig{
		ID:      "selector-mcp-export",
		Kind:    core.ProviderKindMCPServer,
		Enabled: true,
		Target:  "stdio://local",
		Config: map[string]any{
			"export_tool_selectors": []any{
				map[string]any{"kind": string(core.CapabilityKindTool), "name": "echo"},
			},
			"export_prompt_selectors": []any{
				map[string]any{"kind": string(core.CapabilityKindPrompt), "name": "draft.summary"},
			},
			"export_resource_selectors": []any{
				map[string]any{"kind": string(core.CapabilityKindResource), "name": "docs"},
			},
		},
	})
	require.NoError(t, err)

	rt := &Runtime{
		Tools: registry,
		AgentSpec: &core.AgentRuntimeSpec{
			ProviderPolicies: map[string]core.ProviderPolicy{
				"selector-mcp-export": {Activate: core.AgentPermissionAllow},
			},
		},
	}
	require.NoError(t, rt.RegisterProvider(context.Background(), provider))
	serverProvider := provider.(*mcpServerProvider)
	tools, err := serverProvider.exportableDescriptors(context.Background(), core.CapabilityKindTool, true)
	require.NoError(t, err)
	require.Len(t, tools, 1)
	require.Equal(t, "echo", tools[0].Name)
	prompts, err := serverProvider.exportableDescriptors(context.Background(), core.CapabilityKindPrompt, false)
	require.NoError(t, err)
	require.Len(t, prompts, 1)
	require.Equal(t, "draft.summary", prompts[0].Name)
	resources, err := serverProvider.exportableDescriptors(context.Background(), core.CapabilityKindResource, false)
	require.NoError(t, err)
	require.Len(t, resources, 1)
	require.Equal(t, "docs", resources[0].Name)
}

func TestMCPServerProviderDefaultDenyExportsNothingWithoutConfig(t *testing.T) {
	registry := capability.NewRegistry()
	require.NoError(t, registry.RegisterInvocableCapability(exportableInvocableCapability{
		desc: core.CapabilityDescriptor{
			ID:            "relurpic:echo",
			Kind:          core.CapabilityKindTool,
			RuntimeFamily: core.CapabilityRuntimeFamilyRelurpic,
			Name:          "echo",
			TrustClass:    core.TrustClassBuiltinTrusted,
			Availability:  core.AvailabilitySpec{Available: true},
		},
		result: &core.ToolResult{Success: true},
	}))
	provider, err := providerFromConfig(core.ProviderConfig{
		ID:             "local-mcp-export",
		Kind:           core.ProviderKindMCPServer,
		Enabled:        true,
		Target:         "stdio://local",
		Recoverability: core.RecoverabilityPersistedRestore,
	})
	require.NoError(t, err)
	rt := &Runtime{
		Tools: registry,
		AgentSpec: &core.AgentRuntimeSpec{
			ProviderPolicies: map[string]core.ProviderPolicy{
				"local-mcp-export": {Activate: core.AgentPermissionAllow},
			},
		},
	}
	require.NoError(t, rt.RegisterProvider(context.Background(), provider))
	serverProvider := provider.(*mcpServerProvider)
	conn, _, err := serverProvider.openLoopbackSession(context.Background())
	require.NoError(t, err)
	defer conn.Close()
	enc := json.NewEncoder(conn)
	dec := json.NewDecoder(bufio.NewReader(conn))
	require.NoError(t, enc.Encode(map[string]any{
		"jsonrpc": "2.0", "id": "1", "method": "initialize",
		"params": protocol.InitializeRequest{ProtocolVersion: protocol.Revision20250618},
	}))
	var initResp map[string]any
	require.NoError(t, dec.Decode(&initResp))
	require.NoError(t, enc.Encode(map[string]any{"jsonrpc": "2.0", "method": "notifications/initialized", "params": map[string]any{}}))
	require.NoError(t, enc.Encode(map[string]any{"jsonrpc": "2.0", "id": "2", "method": "tools/list", "params": map[string]any{}}))
	var toolsResp struct {
		ID     string                   `json:"id"`
		Result protocol.ListToolsResult `json:"result"`
	}
	require.NoError(t, dec.Decode(&toolsResp))
	require.Empty(t, toolsResp.Result.Tools)
}

func TestMCPServerProviderExposesHTTPHandler(t *testing.T) {
	registry := capability.NewRegistry()
	require.NoError(t, registry.RegisterInvocableCapability(exportableInvocableCapability{
		desc: core.CapabilityDescriptor{
			ID:            "relurpic:echo",
			Kind:          core.CapabilityKindTool,
			RuntimeFamily: core.CapabilityRuntimeFamilyRelurpic,
			Name:          "echo",
			TrustClass:    core.TrustClassBuiltinTrusted,
			Availability:  core.AvailabilitySpec{Available: true},
		},
		result: &core.ToolResult{Success: true, Data: map[string]interface{}{"echo": "hello"}},
	}))
	provider, err := providerFromConfig(core.ProviderConfig{
		ID:      "http-mcp-export",
		Kind:    core.ProviderKindMCPServer,
		Enabled: true,
		Target:  "http://127.0.0.1:0/mcp",
		Config: map[string]any{
			"export_tool_selectors": []any{
				map[string]any{"kind": string(core.CapabilityKindTool), "name": "echo"},
			},
		},
	})
	require.NoError(t, err)
	rt := &Runtime{
		Tools: registry,
		AgentSpec: &core.AgentRuntimeSpec{
			ProviderPolicies: map[string]core.ProviderPolicy{
				"http-mcp-export": {Activate: core.AgentPermissionAllow},
			},
		},
	}
	require.NoError(t, rt.RegisterProvider(context.Background(), provider))
	serverProvider := provider.(*mcpServerProvider)
	handler := serverProvider.HTTPHandler()
	require.NotNil(t, handler)

	initReq := httptest.NewRequest(http.MethodPost, "/mcp", encodeJSONBody(t, map[string]any{
		"jsonrpc": "2.0",
		"id":      "1",
		"method":  "initialize",
		"params": protocol.InitializeRequest{
			ProtocolVersion: protocol.Revision20250618,
		},
	}))
	initResp := httptest.NewRecorder()
	handler.ServeHTTP(initResp, initReq)
	require.Equal(t, http.StatusOK, initResp.Code)
	sessionID := initResp.Header().Get("Mcp-Session-Id")
	require.NotEmpty(t, sessionID)
}

func TestRuntimeQuarantineProviderClosesLiveProviderAndRevokesExecution(t *testing.T) {
	registry := capability.NewRegistry()
	require.NoError(t, registry.RegisterInvocableCapability(runtimeProviderScopedCapability{name: "browser", provider: "remote-mcp"}))
	rt := &Runtime{
		Tools: registry,
		AgentSpec: &core.AgentRuntimeSpec{
			ProviderPolicies: map[string]core.ProviderPolicy{
				"remote-mcp": {Activate: core.AgentPermissionAllow},
			},
		},
	}
	provider := &testProvider{
		desc: core.ProviderDescriptor{
			ID:   "remote-mcp",
			Kind: core.ProviderKindMCPClient,
			Security: core.ProviderSecurityProfile{
				Origin: core.ProviderOriginRemote,
			},
		},
	}
	require.NoError(t, rt.RegisterProvider(context.Background(), provider))

	require.NoError(t, rt.QuarantineProvider(context.Background(), "remote-mcp", "manual quarantine"))
	require.Equal(t, 1, provider.closeCalls)

	_, err := registry.InvokeCapability(context.Background(), core.NewContext(), "browser", nil)
	require.ErrorContains(t, err, "provider remote-mcp revoked")
}

func TestRuntimeRevokeSessionClosesManagedSessionAndRevokesExecution(t *testing.T) {
	registry := capability.NewRegistry()
	require.NoError(t, registry.RegisterInvocableCapability(runtimeProviderScopedCapability{
		name:      "browser",
		provider:  "browser",
		sessionID: "session-1",
	}))
	rt := &Runtime{Tools: registry}
	provider := &testSessionProvider{
		testProvider: testProvider{
			desc: core.ProviderDescriptor{
				ID:   "browser",
				Kind: core.ProviderKindAgentRuntime,
				Security: core.ProviderSecurityProfile{
					Origin: core.ProviderOriginLocal,
				},
			},
		},
	}
	require.NoError(t, rt.RegisterProvider(context.Background(), provider))

	require.NoError(t, rt.RevokeSession(context.Background(), "session-1", "forced shutdown"))
	require.Equal(t, []string{"session-1"}, provider.closedSessions)

	_, err := registry.InvokeCapability(context.Background(), core.NewContext(), "browser", nil)
	require.ErrorContains(t, err, "session session-1 revoked")
}
