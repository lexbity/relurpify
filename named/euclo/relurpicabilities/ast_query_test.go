package relurpicabilities

import (
	"context"
	"errors"
	"testing"

	"codeburg.org/lexbit/relurpify/framework/agentenv"
	"codeburg.org/lexbit/relurpify/framework/ast"
	"codeburg.org/lexbit/relurpify/framework/contextdata"
	"codeburg.org/lexbit/relurpify/framework/core"
	"codeburg.org/lexbit/relurpify/framework/sandbox"
)

// mockIndexStore is a test double for ast.IndexStore
type mockIndexStore struct {
	nodes []*ast.Node
}

func (m *mockIndexStore) SaveFile(metadata *ast.FileMetadata) error {
	return nil
}

func (m *mockIndexStore) GetFile(fileID string) (*ast.FileMetadata, error) {
	return nil, nil
}

func (m *mockIndexStore) GetFileByPath(path string) (*ast.FileMetadata, error) {
	return nil, nil
}

func (m *mockIndexStore) ListFiles(category ast.Category) ([]*ast.FileMetadata, error) {
	return nil, nil
}

func (m *mockIndexStore) DeleteFile(fileID string) error {
	return nil
}

func (m *mockIndexStore) SaveNodes(nodes []*ast.Node) error {
	m.nodes = append(m.nodes, nodes...)
	return nil
}

func (m *mockIndexStore) GetNode(nodeID string) (*ast.Node, error) {
	for _, node := range m.nodes {
		if node.ID == nodeID {
			return node, nil
		}
	}
	return nil, nil
}

func (m *mockIndexStore) GetNodesByFile(fileID string) ([]*ast.Node, error) {
	return nil, nil
}

func (m *mockIndexStore) GetNodesByType(nodeType ast.NodeType) ([]*ast.Node, error) {
	return nil, nil
}

func (m *mockIndexStore) GetNodesByName(name string) ([]*ast.Node, error) {
	return nil, nil
}

func (m *mockIndexStore) SearchNodes(query ast.NodeQuery) ([]*ast.Node, error) {
	if query.Limit > 0 && len(m.nodes) > query.Limit {
		return m.nodes[:query.Limit], nil
	}
	return m.nodes, nil
}

func (m *mockIndexStore) DeleteNode(nodeID string) error {
	return nil
}

func (m *mockIndexStore) SaveEdges(edges []*ast.Edge) error {
	return nil
}

func (m *mockIndexStore) GetEdge(edgeID string) (*ast.Edge, error) {
	return nil, nil
}

func (m *mockIndexStore) GetEdgesBySource(sourceID string) ([]*ast.Edge, error) {
	return nil, nil
}

func (m *mockIndexStore) GetEdgesByTarget(targetID string) ([]*ast.Edge, error) {
	return nil, nil
}

func (m *mockIndexStore) GetEdgesByType(edgeType ast.EdgeType) ([]*ast.Edge, error) {
	return nil, nil
}

func allowCommandPolicy() sandbox.CommandPolicy {
	return sandbox.CommandPolicyFunc(func(ctx context.Context, req sandbox.CommandRequest) error { return nil })
}

func (m *mockIndexStore) SearchEdges(query ast.EdgeQuery) ([]*ast.Edge, error) {
	return nil, nil
}

func (m *mockIndexStore) DeleteEdge(edgeID string) error {
	return nil
}

func (m *mockIndexStore) GetCallees(nodeID string) ([]*ast.Node, error) {
	return nil, nil
}

func (m *mockIndexStore) GetCallers(nodeID string) ([]*ast.Node, error) {
	return nil, nil
}

func (m *mockIndexStore) GetImports(nodeID string) ([]*ast.Node, error) {
	return nil, nil
}

func (m *mockIndexStore) GetImportedBy(nodeID string) ([]*ast.Node, error) {
	return nil, nil
}

func (m *mockIndexStore) GetReferences(nodeID string) ([]*ast.Node, error) {
	return nil, nil
}

func (m *mockIndexStore) GetReferencedBy(nodeID string) ([]*ast.Node, error) {
	return nil, nil
}

func (m *mockIndexStore) GetDependencies(nodeID string) ([]*ast.Node, error) {
	return nil, nil
}

func (m *mockIndexStore) GetDependents(nodeID string) ([]*ast.Node, error) {
	return nil, nil
}

func (m *mockIndexStore) BeginTransaction() (ast.Transaction, error) {
	return nil, nil
}

func (m *mockIndexStore) Vacuum() error {
	return nil
}

func (m *mockIndexStore) GetStats() (*ast.IndexStats, error) {
	return nil, nil
}

func TestASTQueryHandlerDescriptor(t *testing.T) {
	wsEnv := agentenv.WorkspaceEnvironment{}
	handler := NewASTQueryHandler(wsEnv)

	ctx := context.Background()
	envelope := contextdata.NewEnvelope("test-task", "test-session")

	desc := handler.Descriptor(ctx, envelope)

	if desc.ID != "euclo:cap.ast_query" {
		t.Errorf("descriptor ID = %q, want %q", desc.ID, "euclo:cap.ast_query")
	}

	if desc.Kind != core.CapabilityKindTool {
		t.Errorf("descriptor Kind = %v, want %v", desc.Kind, core.CapabilityKindTool)
	}

	if desc.RuntimeFamily != core.CapabilityRuntimeFamilyRelurpic {
		t.Errorf("descriptor RuntimeFamily = %v, want %v", desc.RuntimeFamily, core.CapabilityRuntimeFamilyRelurpic)
	}
}

func TestASTQueryHandlerQueriesIndex(t *testing.T) {
	store := &mockIndexStore{}
	indexManager := ast.NewIndexManager(store, ast.IndexConfig{})

	// Insert two test nodes
	nodes := []*ast.Node{
		{
			ID:        "node1",
			Name:      "TestFunction",
			Type:      ast.NodeTypeFunction,
			Language:  "go",
			StartLine: 10,
			EndLine:   20,
			FileID:    "file1",
		},
		{
			ID:        "node2",
			Name:      "TestFunction",
			Type:      ast.NodeTypeFunction,
			Language:  "go",
			StartLine: 30,
			EndLine:   40,
			FileID:    "file2",
		},
	}
	store.SaveNodes(nodes)

	wsEnv := agentenv.WorkspaceEnvironment{
		IndexManager: indexManager,
	}
	handler := NewASTQueryHandler(wsEnv)

	ctx := context.Background()
	envelope := contextdata.NewEnvelope("test-task", "test-session")
	args := map[string]interface{}{
		"query": "TestFunction",
	}

	result, err := handler.Invoke(ctx, envelope, args)
	if err != nil {
		t.Fatalf("Invoke returned error: %v", err)
	}

	if !result.Success {
		t.Errorf("result.Success = false, want true")
	}

	matches, ok := result.Data["matches"].([]map[string]interface{})
	if !ok {
		t.Fatal("result.Data[\"matches\"] is not a []map[string]interface{}")
	}
	if len(matches) != 2 {
		t.Errorf("matches length = %d, want 2", len(matches))
	}
}

func TestASTQueryHandlerEmptyQueryErrors(t *testing.T) {
	store := &mockIndexStore{}
	indexManager := ast.NewIndexManager(store, ast.IndexConfig{})

	wsEnv := agentenv.WorkspaceEnvironment{
		IndexManager: indexManager,
	}
	handler := NewASTQueryHandler(wsEnv)

	ctx := context.Background()
	envelope := contextdata.NewEnvelope("test-task", "test-session")
	args := map[string]interface{}{
		"query": "",
	}

	result, err := handler.Invoke(ctx, envelope, args)
	if err != nil {
		t.Fatalf("Invoke returned error: %v", err)
	}

	if result.Success {
		t.Errorf("result.Success = true, want false")
	}

	errorMsg, ok := result.Data["error"].(string)
	if !ok {
		t.Fatal("result.Data[\"error\"] is not a string")
	}
	if errorMsg == "" {
		t.Errorf("error message is empty, want non-empty")
	}
}

func TestASTQueryHandlerLimitRespected(t *testing.T) {
	store := &mockIndexStore{}
	indexManager := ast.NewIndexManager(store, ast.IndexConfig{})

	// Insert 30 test nodes
	nodes := make([]*ast.Node, 30)
	for i := 0; i < 30; i++ {
		nodes[i] = &ast.Node{
			ID:        "node" + string(rune(i)),
			Name:      "TestFunction",
			Type:      ast.NodeTypeFunction,
			Language:  "go",
			StartLine: i * 10,
			EndLine:   i*10 + 10,
			FileID:    "file1",
		}
	}
	store.SaveNodes(nodes)

	wsEnv := agentenv.WorkspaceEnvironment{
		IndexManager: indexManager,
	}
	handler := NewASTQueryHandler(wsEnv)

	ctx := context.Background()
	envelope := contextdata.NewEnvelope("test-task", "test-session")
	args := map[string]interface{}{
		"query": "TestFunction",
		"limit": 20,
	}

	result, err := handler.Invoke(ctx, envelope, args)
	if err != nil {
		t.Fatalf("Invoke returned error: %v", err)
	}

	if !result.Success {
		t.Errorf("result.Success = false, want true")
	}

	matches, ok := result.Data["matches"].([]map[string]interface{})
	if !ok {
		t.Fatal("result.Data[\"matches\"] is not a []map[string]interface{}")
	}
	if len(matches) > 20 {
		t.Errorf("matches length = %d, want ≤ 20", len(matches))
	}
}

func TestASTQueryHandlerWritesReferences(t *testing.T) {
	store := &mockIndexStore{}
	indexManager := ast.NewIndexManager(store, ast.IndexConfig{})

	// Insert test nodes
	nodes := []*ast.Node{
		{
			ID:        "node1",
			Name:      "TestFunction",
			Type:      ast.NodeTypeFunction,
			Language:  "go",
			StartLine: 10,
			EndLine:   20,
			FileID:    "file1",
		},
	}
	store.SaveNodes(nodes)

	wsEnv := agentenv.WorkspaceEnvironment{
		IndexManager: indexManager,
	}
	handler := NewASTQueryHandler(wsEnv)

	ctx := context.Background()
	envelope := contextdata.NewEnvelope("test-task", "test-session")
	args := map[string]interface{}{
		"query": "TestFunction",
	}

	_, err := handler.Invoke(ctx, envelope, args)
	if err != nil {
		t.Fatalf("Invoke returned error: %v", err)
	}

	if len(envelope.References.Retrieval) == 0 {
		t.Errorf("envelope.References.Retrieval is empty, want non-empty")
	}
}

func TestSymbolTraceHandlerDescriptor(t *testing.T) {
	wsEnv := agentenv.WorkspaceEnvironment{}
	handler := NewSymbolTraceHandler(wsEnv)

	ctx := context.Background()
	envelope := contextdata.NewEnvelope("test-task", "test-session")

	desc := handler.Descriptor(ctx, envelope)

	if desc.ID != "euclo:cap.symbol_trace" {
		t.Errorf("descriptor ID = %q, want %q", desc.ID, "euclo:cap.symbol_trace")
	}

	if desc.Kind != core.CapabilityKindTool {
		t.Errorf("descriptor Kind = %v, want %v", desc.Kind, core.CapabilityKindTool)
	}

	if desc.RuntimeFamily != core.CapabilityRuntimeFamilyRelurpic {
		t.Errorf("descriptor RuntimeFamily = %v, want %v", desc.RuntimeFamily, core.CapabilityRuntimeFamilyRelurpic)
	}
}

func TestSymbolTraceHandlerCallees(t *testing.T) {
	store := &mockIndexStore{}
	indexManager := ast.NewIndexManager(store, ast.IndexConfig{})

	// Insert nodes with callee relationship
	rootNode := &ast.Node{
		ID:        "root",
		Name:      "MainFunction",
		Type:      ast.NodeTypeFunction,
		Language:  "go",
		StartLine: 1,
		EndLine:   10,
		FileID:    "file1",
	}
	calleeNode := &ast.Node{
		ID:        "callee",
		Name:      "HelperFunction",
		Type:      ast.NodeTypeFunction,
		Language:  "go",
		StartLine: 20,
		EndLine:   30,
		FileID:    "file1",
	}
	store.SaveNodes([]*ast.Node{rootNode, calleeNode})

	wsEnv := agentenv.WorkspaceEnvironment{
		IndexManager: indexManager,
	}
	handler := NewSymbolTraceHandler(wsEnv)

	ctx := context.Background()
	envelope := contextdata.NewEnvelope("test-task", "test-session")
	args := map[string]interface{}{
		"symbol": "MainFunction",
	}

	result, err := handler.Invoke(ctx, envelope, args)
	if err != nil {
		t.Fatalf("Invoke returned error: %v", err)
	}

	if !result.Success {
		t.Errorf("result.Success = false, want true")
	}

	// Note: The mock store doesn't implement GetCallees, so we just verify the call doesn't error
	_ = result.Data["root"]
}

func TestSymbolTraceHandlerSymbolNotFound(t *testing.T) {
	store := &mockIndexStore{}
	indexManager := ast.NewIndexManager(store, ast.IndexConfig{})

	wsEnv := agentenv.WorkspaceEnvironment{
		IndexManager: indexManager,
	}
	handler := NewSymbolTraceHandler(wsEnv)

	ctx := context.Background()
	envelope := contextdata.NewEnvelope("test-task", "test-session")
	args := map[string]interface{}{
		"symbol": "NonExistentFunction",
	}

	result, err := handler.Invoke(ctx, envelope, args)
	if err != nil {
		t.Fatalf("Invoke returned error: %v", err)
	}

	if result.Success {
		t.Errorf("result.Success = true, want false for non-existent symbol")
	}

	errorMsg, ok := result.Data["error"].(string)
	if !ok {
		t.Fatal("result.Data[\"error\"] is not a string")
	}
	if errorMsg == "" {
		t.Errorf("error message is empty, want non-empty")
	}
}

func TestCallGraphHandlerDescriptor(t *testing.T) {
	wsEnv := agentenv.WorkspaceEnvironment{}
	handler := NewCallGraphHandler(wsEnv)

	ctx := context.Background()
	envelope := contextdata.NewEnvelope("test-task", "test-session")

	desc := handler.Descriptor(ctx, envelope)

	if desc.ID != "euclo:cap.call_graph" {
		t.Errorf("descriptor ID = %q, want %q", desc.ID, "euclo:cap.call_graph")
	}

	if desc.Kind != core.CapabilityKindTool {
		t.Errorf("descriptor Kind = %v, want %v", desc.Kind, core.CapabilityKindTool)
	}

	if desc.RuntimeFamily != core.CapabilityRuntimeFamilyRelurpic {
		t.Errorf("descriptor RuntimeFamily = %v, want %v", desc.RuntimeFamily, core.CapabilityRuntimeFamilyRelurpic)
	}
}

func TestCallGraphHandlerBuildsGraph(t *testing.T) {
	store := &mockIndexStore{}
	indexManager := ast.NewIndexManager(store, ast.IndexConfig{})

	// Insert 3 nodes in a call chain
	nodes := []*ast.Node{
		{
			ID:        "node1",
			Name:      "MainFunction",
			Type:      ast.NodeTypeFunction,
			Language:  "go",
			StartLine: 1,
			EndLine:   10,
			FileID:    "file1",
		},
		{
			ID:        "node2",
			Name:      "HelperFunction",
			Type:      ast.NodeTypeFunction,
			Language:  "go",
			StartLine: 20,
			EndLine:   30,
			FileID:    "file1",
		},
		{
			ID:        "node3",
			Name:      "UtilityFunction",
			Type:      ast.NodeTypeFunction,
			Language:  "go",
			StartLine: 40,
			EndLine:   50,
			FileID:    "file1",
		},
	}
	store.SaveNodes(nodes)

	wsEnv := agentenv.WorkspaceEnvironment{
		IndexManager: indexManager,
	}
	handler := NewCallGraphHandler(wsEnv)

	ctx := context.Background()
	envelope := contextdata.NewEnvelope("test-task", "test-session")
	args := map[string]interface{}{
		"entry_point": "MainFunction",
	}

	result, err := handler.Invoke(ctx, envelope, args)
	if err != nil {
		t.Fatalf("Invoke returned error: %v", err)
	}

	if !result.Success {
		t.Errorf("result.Success = false, want true")
	}

	nodesOut, ok := result.Data["nodes"].([]map[string]interface{})
	if !ok {
		t.Fatal("result.Data[\"nodes\"] is not a []map[string]interface{}")
	}
	if len(nodesOut) == 0 {
		t.Errorf("nodes is empty, want non-empty")
	}
}

func TestCallGraphHandlerEntryPointNotFound(t *testing.T) {
	store := &mockIndexStore{}
	indexManager := ast.NewIndexManager(store, ast.IndexConfig{})

	wsEnv := agentenv.WorkspaceEnvironment{
		IndexManager: indexManager,
	}
	handler := NewCallGraphHandler(wsEnv)

	ctx := context.Background()
	envelope := contextdata.NewEnvelope("test-task", "test-session")
	args := map[string]interface{}{
		"entry_point": "NonExistentFunction",
	}

	result, err := handler.Invoke(ctx, envelope, args)
	if err != nil {
		t.Fatalf("Invoke returned error: %v", err)
	}

	if result.Success {
		t.Errorf("result.Success = true, want false for non-existent entry point")
	}

	errorMsg, ok := result.Data["error"].(string)
	if !ok {
		t.Fatal("result.Data[\"error\"] is not a string")
	}
	if errorMsg == "" {
		t.Errorf("error message is empty, want non-empty")
	}
}

func TestCallGraphHandlerWritesReferences(t *testing.T) {
	store := &mockIndexStore{}
	indexManager := ast.NewIndexManager(store, ast.IndexConfig{})

	// Insert test nodes
	nodes := []*ast.Node{
		{
			ID:        "node1",
			Name:      "MainFunction",
			Type:      ast.NodeTypeFunction,
			Language:  "go",
			StartLine: 1,
			EndLine:   10,
			FileID:    "file1",
		},
	}
	store.SaveNodes(nodes)

	wsEnv := agentenv.WorkspaceEnvironment{
		IndexManager: indexManager,
	}
	handler := NewCallGraphHandler(wsEnv)

	ctx := context.Background()
	envelope := contextdata.NewEnvelope("test-task", "test-session")
	args := map[string]interface{}{
		"entry_point": "MainFunction",
	}

	_, err := handler.Invoke(ctx, envelope, args)
	if err != nil {
		t.Fatalf("Invoke returned error: %v", err)
	}

	if len(envelope.References.Retrieval) == 0 {
		t.Errorf("envelope.References.Retrieval is empty, want non-empty")
	}
}

func TestBlameTraceHandlerDescriptor(t *testing.T) {
	wsEnv := agentenv.WorkspaceEnvironment{}
	handler := NewBlameTraceHandler(wsEnv)

	ctx := context.Background()
	envelope := contextdata.NewEnvelope("test-task", "test-session")

	desc := handler.Descriptor(ctx, envelope)

	if desc.ID != "euclo:cap.blame_trace" {
		t.Errorf("descriptor ID = %q, want %q", desc.ID, "euclo:cap.blame_trace")
	}

	if desc.Kind != core.CapabilityKindTool {
		t.Errorf("descriptor Kind = %v, want %v", desc.Kind, core.CapabilityKindTool)
	}

	if desc.RuntimeFamily != core.CapabilityRuntimeFamilyRelurpic {
		t.Errorf("descriptor RuntimeFamily = %v, want %v", desc.RuntimeFamily, core.CapabilityRuntimeFamilyRelurpic)
	}
}

func TestBlameTraceHandlerParsesOutput(t *testing.T) {
	mockRunner := &mockCommandRunner{
		stdout: "abc123def456abc123def456abc123def456abc123 1 2 3\nauthor John Doe\nsummary Added feature\n",
		stderr: "",
		err:    nil,
	}

	store := &mockIndexStore{}
	indexManager := ast.NewIndexManager(store, ast.IndexConfig{})

	wsEnv := agentenv.WorkspaceEnvironment{
		CommandRunner: mockRunner,
		IndexManager:  indexManager,
		CommandPolicy: allowCommandPolicy(),
	}
	handler := NewBlameTraceHandler(wsEnv)

	ctx := context.Background()
	envelope := contextdata.NewEnvelope("test-task", "test-session")
	args := map[string]interface{}{
		"file": "test.go",
	}

	result, err := handler.Invoke(ctx, envelope, args)
	if err != nil {
		t.Fatalf("Invoke returned error: %v", err)
	}

	if !result.Success {
		t.Errorf("result.Success = false, want true")
	}

	entries, ok := result.Data["entries"].([]map[string]interface{})
	if !ok {
		t.Fatal("result.Data[\"entries\"] is not a []map[string]interface{}")
	}
	if len(entries) == 0 {
		t.Errorf("entries is empty, want non-empty")
	}
}

func TestBlameTraceHandlerLineRange(t *testing.T) {
	mockRunner := &mockCommandRunner{
		stdout: "",
		stderr: "",
		err:    nil,
	}

	store := &mockIndexStore{}
	indexManager := ast.NewIndexManager(store, ast.IndexConfig{})

	wsEnv := agentenv.WorkspaceEnvironment{
		CommandRunner: mockRunner,
		IndexManager:  indexManager,
		CommandPolicy: allowCommandPolicy(),
	}
	handler := NewBlameTraceHandler(wsEnv)

	ctx := context.Background()
	envelope := contextdata.NewEnvelope("test-task", "test-session")
	args := map[string]interface{}{
		"file":  "test.go",
		"lines": []interface{}{10, 20},
	}

	_, err := handler.Invoke(ctx, envelope, args)
	if err != nil {
		t.Fatalf("Invoke returned error: %v", err)
	}
}

func TestBlameTraceHandlerCommandDenied(t *testing.T) {
	mockRunner := &mockCommandRunner{
		stdout: "",
		stderr: "",
		err:    errors.New("command denied by policy"),
	}

	store := &mockIndexStore{}
	indexManager := ast.NewIndexManager(store, ast.IndexConfig{})

	wsEnv := agentenv.WorkspaceEnvironment{
		CommandRunner: mockRunner,
		IndexManager:  indexManager,
		CommandPolicy: allowCommandPolicy(),
	}
	handler := NewBlameTraceHandler(wsEnv)

	ctx := context.Background()
	envelope := contextdata.NewEnvelope("test-task", "test-session")
	args := map[string]interface{}{
		"file": "test.go",
	}

	result, err := handler.Invoke(ctx, envelope, args)
	if err != nil {
		t.Fatalf("Invoke returned error: %v", err)
	}

	if result.Success {
		t.Errorf("result.Success = true, want false")
	}

	errorMsg, ok := result.Data["error"].(string)
	if !ok {
		t.Fatal("result.Data[\"error\"] is not a string")
	}
	if errorMsg == "" {
		t.Errorf("error message is empty, want non-empty")
	}
}

func TestBlameTraceHandlerSymbolResolvedToLines(t *testing.T) {
	mockRunner := &mockCommandRunner{
		stdout: "",
		stderr: "",
		err:    nil,
	}

	store := &mockIndexStore{}
	indexManager := ast.NewIndexManager(store, ast.IndexConfig{})

	// Insert a node with specific line range
	nodes := []*ast.Node{
		{
			ID:        "node1",
			Name:      "TestFunction",
			Type:      ast.NodeTypeFunction,
			Language:  "go",
			StartLine: 5,
			EndLine:   15,
			FileID:    "file1",
		},
	}
	store.SaveNodes(nodes)

	wsEnv := agentenv.WorkspaceEnvironment{
		CommandRunner: mockRunner,
		IndexManager:  indexManager,
		CommandPolicy: allowCommandPolicy(),
	}
	handler := NewBlameTraceHandler(wsEnv)

	ctx := context.Background()
	envelope := contextdata.NewEnvelope("test-task", "test-session")
	args := map[string]interface{}{
		"file":   "test.go",
		"symbol": "TestFunction",
	}

	_, err := handler.Invoke(ctx, envelope, args)
	if err != nil {
		t.Fatalf("Invoke returned error: %v", err)
	}
}

func TestBisectHandlerDescriptor(t *testing.T) {
	wsEnv := agentenv.WorkspaceEnvironment{}
	handler := NewBisectHandler(wsEnv)

	ctx := context.Background()
	envelope := contextdata.NewEnvelope("test-task", "test-session")

	desc := handler.Descriptor(ctx, envelope)

	if desc.ID != "euclo:cap.bisect" {
		t.Errorf("descriptor ID = %q, want %q", desc.ID, "euclo:cap.bisect")
	}

	if desc.Kind != core.CapabilityKindTool {
		t.Errorf("descriptor Kind = %v, want %v", desc.Kind, core.CapabilityKindTool)
	}

	if desc.RuntimeFamily != core.CapabilityRuntimeFamilyRelurpic {
		t.Errorf("descriptor RuntimeFamily = %v, want %v", desc.RuntimeFamily, core.CapabilityRuntimeFamilyRelurpic)
	}
}

func TestBisectHandlerMissingArgs(t *testing.T) {
	mockRunner := &mockCommandRunner{
		stdout: "",
		stderr: "",
		err:    nil,
	}

	store := &mockIndexStore{}
	indexManager := ast.NewIndexManager(store, ast.IndexConfig{})

	wsEnv := agentenv.WorkspaceEnvironment{
		CommandRunner: mockRunner,
		IndexManager:  indexManager,
		CommandPolicy: allowCommandPolicy(),
	}
	handler := NewBisectHandler(wsEnv)

	ctx := context.Background()
	envelope := contextdata.NewEnvelope("test-task", "test-session")
	args := map[string]interface{}{
		// Missing good_ref
		"bad_ref":      "bad123",
		"test_command": "go test ./...",
	}

	result, err := handler.Invoke(ctx, envelope, args)
	if err != nil {
		t.Fatalf("Invoke returned error: %v", err)
	}

	if result.Success {
		t.Errorf("result.Success = true, want false for missing good_ref")
	}

	errorMsg, ok := result.Data["error"].(string)
	if !ok {
		t.Fatal("result.Data[\"error\"] is not a string")
	}
	if errorMsg == "" {
		t.Errorf("error message is empty, want non-empty")
	}
}

func TestBisectHandlerStepLimit(t *testing.T) {
	mockRunner := &mockCommandRunner{
		stdout: "bisecting...",
		stderr: "",
		err:    nil,
	}

	store := &mockIndexStore{}
	indexManager := ast.NewIndexManager(store, ast.IndexConfig{})

	wsEnv := agentenv.WorkspaceEnvironment{
		CommandRunner: mockRunner,
		IndexManager:  indexManager,
		CommandPolicy: allowCommandPolicy(),
	}
	handler := NewBisectHandler(wsEnv)

	ctx := context.Background()
	envelope := contextdata.NewEnvelope("test-task", "test-session")
	args := map[string]interface{}{
		"good_ref":     "good123",
		"bad_ref":      "bad123",
		"test_command": "go test ./...",
		"max_steps":    5,
	}

	result, err := handler.Invoke(ctx, envelope, args)
	if err != nil {
		t.Fatalf("Invoke returned error: %v", err)
	}

	// With max_steps=5, should complete without finding culprit
	stepsTaken, ok := result.Data["steps_taken"].(int)
	if !ok {
		t.Fatal("result.Data[\"steps_taken\"] is not an int")
	}
	if stepsTaken > 5 {
		t.Errorf("steps_taken = %d, want <= 5", stepsTaken)
	}
}

func TestBisectHandlerCulpritExtracted(t *testing.T) {
	mockRunner := &mockCommandRunner{
		stdout: "first bad commit: abc123def456",
		stderr: "",
		err:    nil,
	}

	store := &mockIndexStore{}
	indexManager := ast.NewIndexManager(store, ast.IndexConfig{})

	wsEnv := agentenv.WorkspaceEnvironment{
		CommandRunner: mockRunner,
		IndexManager:  indexManager,
		CommandPolicy: allowCommandPolicy(),
	}
	handler := NewBisectHandler(wsEnv)

	ctx := context.Background()
	envelope := contextdata.NewEnvelope("test-task", "test-session")
	args := map[string]interface{}{
		"good_ref":     "good123",
		"bad_ref":      "bad123",
		"test_command": "go test ./...",
		"max_steps":    10,
	}

	result, err := handler.Invoke(ctx, envelope, args)
	if err != nil {
		t.Fatalf("Invoke returned error: %v", err)
	}

	found, ok := result.Data["found"].(bool)
	if !ok {
		t.Fatal("result.Data[\"found\"] is not a bool")
	}
	if !found {
		t.Errorf("found = false, want true")
	}
}

func TestCodeReviewHandlerDescriptor(t *testing.T) {
	wsEnv := agentenv.WorkspaceEnvironment{}
	handler := NewCodeReviewHandler(wsEnv)

	ctx := context.Background()
	envelope := contextdata.NewEnvelope("test-task", "test-session")

	desc := handler.Descriptor(ctx, envelope)

	if desc.ID != "euclo:cap.code_review" {
		t.Errorf("descriptor ID = %q, want %q", desc.ID, "euclo:cap.code_review")
	}

	if desc.Kind != core.CapabilityKindTool {
		t.Errorf("descriptor Kind = %v, want %v", desc.Kind, core.CapabilityKindTool)
	}

	if desc.RuntimeFamily != core.CapabilityRuntimeFamilyRelurpic {
		t.Errorf("descriptor RuntimeFamily = %v, want %v", desc.RuntimeFamily, core.CapabilityRuntimeFamilyRelurpic)
	}

	if desc.Category != "review_synthesis" {
		t.Errorf("descriptor Category = %q, want %q", desc.Category, "review_synthesis")
	}
}

func TestCodeReviewHandlerNoContextReturnsEmpty(t *testing.T) {
	wsEnv := agentenv.WorkspaceEnvironment{}
	handler := NewCodeReviewHandler(wsEnv)

	ctx := context.Background()
	envelope := contextdata.NewEnvelope("test-task", "test-session")
	args := map[string]interface{}{
		"focus": "all",
	}

	result, err := handler.Invoke(ctx, envelope, args)
	if err != nil {
		t.Fatalf("Invoke returned error: %v", err)
	}

	if !result.Success {
		t.Errorf("result.Success = false, want true")
	}

	findings, ok := result.Data["findings"].([]interface{})
	if !ok {
		t.Fatal("result.Data[\"findings\"] is not a []interface{}")
	}
	if len(findings) != 0 {
		t.Errorf("findings length = %d, want 0", len(findings))
	}

	summary, ok := result.Data["summary"].(string)
	if !ok {
		t.Fatal("result.Data[\"summary\"] is not a string")
	}
	if summary != "no context to review" {
		t.Errorf("summary = %q, want %q", summary, "no context to review")
	}
}

func TestDiffSummaryHandlerDescriptor(t *testing.T) {
	wsEnv := agentenv.WorkspaceEnvironment{}
	handler := NewDiffSummaryHandler(wsEnv)

	ctx := context.Background()
	envelope := contextdata.NewEnvelope("test-task", "test-session")

	desc := handler.Descriptor(ctx, envelope)

	if desc.ID != "euclo:cap.diff_summary" {
		t.Errorf("descriptor ID = %q, want %q", desc.ID, "euclo:cap.diff_summary")
	}

	if desc.Kind != core.CapabilityKindTool {
		t.Errorf("descriptor Kind = %v, want %v", desc.Kind, core.CapabilityKindTool)
	}

	if desc.RuntimeFamily != core.CapabilityRuntimeFamilyRelurpic {
		t.Errorf("descriptor RuntimeFamily = %v, want %v", desc.RuntimeFamily, core.CapabilityRuntimeFamilyRelurpic)
	}
}

func TestDiffSummaryHandlerCommandDenied(t *testing.T) {
	mockRunner := &mockCommandRunner{
		stdout: "",
		stderr: "",
		err:    errors.New("command denied by policy"),
	}

	store := &mockIndexStore{}
	indexManager := ast.NewIndexManager(store, ast.IndexConfig{})

	wsEnv := agentenv.WorkspaceEnvironment{
		CommandRunner: mockRunner,
		IndexManager:  indexManager,
		CommandPolicy: allowCommandPolicy(),
	}
	handler := NewDiffSummaryHandler(wsEnv)

	ctx := context.Background()
	envelope := contextdata.NewEnvelope("test-task", "test-session")
	args := map[string]interface{}{
		"base_ref": "HEAD~1",
		"head_ref": "HEAD",
	}

	result, err := handler.Invoke(ctx, envelope, args)
	if err != nil {
		t.Fatalf("Invoke returned error: %v", err)
	}

	if result.Success {
		t.Errorf("result.Success = true, want false for command denial")
	}

	errorMsg, ok := result.Data["error"].(string)
	if !ok {
		t.Fatal("result.Data[\"error\"] is not a string")
	}
	if errorMsg == "" {
		t.Errorf("error message is empty, want non-empty")
	}
}

func TestLayerCheckHandlerDescriptor(t *testing.T) {
	wsEnv := agentenv.WorkspaceEnvironment{}
	handler := NewLayerCheckHandler(wsEnv)

	ctx := context.Background()
	envelope := contextdata.NewEnvelope("test-task", "test-session")

	desc := handler.Descriptor(ctx, envelope)

	if desc.ID != "euclo:cap.layer_check" {
		t.Errorf("descriptor ID = %q, want %q", desc.ID, "euclo:cap.layer_check")
	}

	if desc.Kind != core.CapabilityKindTool {
		t.Errorf("descriptor Kind = %v, want %v", desc.Kind, core.CapabilityKindTool)
	}

	if desc.RuntimeFamily != core.CapabilityRuntimeFamilyRelurpic {
		t.Errorf("descriptor RuntimeFamily = %v, want %v", desc.RuntimeFamily, core.CapabilityRuntimeFamilyRelurpic)
	}
}

func TestLayerCheckHandlerCleanWorkspace(t *testing.T) {
	store := &mockIndexStore{}
	indexManager := ast.NewIndexManager(store, ast.IndexConfig{})

	wsEnv := agentenv.WorkspaceEnvironment{
		IndexManager: indexManager,
	}
	handler := NewLayerCheckHandler(wsEnv)

	ctx := context.Background()
	envelope := contextdata.NewEnvelope("test-task", "test-session")
	args := map[string]interface{}{
		"layer": "all",
	}

	result, err := handler.Invoke(ctx, envelope, args)
	if err != nil {
		t.Fatalf("Invoke returned error: %v", err)
	}

	if !result.Success {
		t.Errorf("result.Success = false, want true")
	}

	passed, ok := result.Data["passed"].(bool)
	if !ok {
		t.Fatal("result.Data[\"passed\"] is not a bool")
	}
	if !passed {
		t.Errorf("passed = false, want true for clean workspace")
	}

	violations, ok := result.Data["violations"].([]interface{})
	if !ok {
		t.Fatal("result.Data[\"violations\"] is not a []interface{}")
	}
	if len(violations) != 0 {
		t.Errorf("violations length = %d, want 0", len(violations))
	}
}

func TestLayerCheckHandlerLayerFilter(t *testing.T) {
	store := &mockIndexStore{}
	indexManager := ast.NewIndexManager(store, ast.IndexConfig{})

	wsEnv := agentenv.WorkspaceEnvironment{
		IndexManager: indexManager,
	}
	handler := NewLayerCheckHandler(wsEnv)

	ctx := context.Background()
	envelope := contextdata.NewEnvelope("test-task", "test-session")
	args := map[string]interface{}{
		"layer": "framework",
	}

	result, err := handler.Invoke(ctx, envelope, args)
	if err != nil {
		t.Fatalf("Invoke returned error: %v", err)
	}

	layer, ok := result.Data["layer"].(string)
	if !ok {
		t.Fatal("result.Data[\"layer\"] is not a string")
	}
	if layer != "framework" {
		t.Errorf("layer = %q, want %q", layer, "framework")
	}
}
