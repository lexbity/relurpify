package lsp

import (
	"context"
	"testing"
	"time"
)

// mockLSPClient implements LSPClient for testing.
type mockLSPClient struct {
	defResult    DefinitionResult
	defErr       error
	refsResult   []Location
	refsErr      error
	hoverResult  HoverResult
	hoverErr     error
	diagResult   []Diagnostic
	diagErr      error
	searchResult []SymbolInformation
	searchErr    error
	docSymResult []SymbolInformation
	docSymErr    error
	formatResult string
	formatErr    error
	calledDef    bool
	calledRefs   bool
	calledHover  bool
	calledDiag   bool
	calledSearch bool
	calledDocSym bool
	calledFormat bool
}

func (m *mockLSPClient) GetDefinition(ctx context.Context, req DefinitionRequest) (DefinitionResult, error) {
	m.calledDef = true
	return m.defResult, m.defErr
}
func (m *mockLSPClient) GetReferences(ctx context.Context, req ReferencesRequest) ([]Location, error) {
	m.calledRefs = true
	return m.refsResult, m.refsErr
}
func (m *mockLSPClient) GetHover(ctx context.Context, req HoverRequest) (HoverResult, error) {
	m.calledHover = true
	return m.hoverResult, m.hoverErr
}
func (m *mockLSPClient) GetDiagnostics(ctx context.Context, file string) ([]Diagnostic, error) {
	m.calledDiag = true
	return m.diagResult, m.diagErr
}
func (m *mockLSPClient) SearchSymbols(ctx context.Context, query string) ([]SymbolInformation, error) {
	m.calledSearch = true
	return m.searchResult, m.searchErr
}
func (m *mockLSPClient) GetDocumentSymbols(ctx context.Context, file string) ([]SymbolInformation, error) {
	m.calledDocSym = true
	return m.docSymResult, m.docSymErr
}
func (m *mockLSPClient) Format(ctx context.Context, req FormatRequest) (string, error) {
	m.calledFormat = true
	return m.formatResult, m.formatErr
}

func TestProxy_Register_clientForFile(t *testing.T) {
	proxy := NewProxy(time.Minute)
	mock := &mockLSPClient{}
	proxy.Register("go", mock)

	client, err := proxy.clientForFile("test.go")
	if err != nil {
		t.Fatalf("expected client, got error: %v", err)
	}
	if client != mock {
		t.Error("client mismatch")
	}

	_, err = proxy.clientForFile("test.rs")
	if err == nil {
		t.Error("expected error for unknown extension")
	}
}

func TestProxy_cached(t *testing.T) {
	proxy := NewProxy(100 * time.Millisecond)
	calls := 0
	fetch := func() (interface{}, error) {
		calls++
		return calls, nil
	}
	val, err := proxy.cached("key", fetch)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if val != 1 {
		t.Errorf("expected 1, got %v", val)
	}
	// second call within TTL should return cached value
	val, err = proxy.cached("key", fetch)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if val != 1 {
		t.Errorf("expected cached 1, got %v", val)
	}
	if calls != 1 {
		t.Errorf("fetch called %d times, expected 1", calls)
	}
	// after expiry, fetch again
	time.Sleep(150 * time.Millisecond)
	val, err = proxy.cached("key", fetch)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if val != 2 {
		t.Errorf("expected 2 after expiry, got %v", val)
	}
	if calls != 2 {
		t.Errorf("fetch called %d times, expected 2", calls)
	}
}

func TestDefinitionTool_Execute(t *testing.T) {
	proxy := NewProxy(time.Minute)
	mock := &mockLSPClient{
		defResult: DefinitionResult{
			Location:  Location{URI: "file:///test.go", Range: [2]int64{10, 12}},
			Snippet:   "func foo() {}",
			Signature: "func foo()",
		},
	}
	proxy.Register("go", mock)

	tool := &DefinitionTool{Proxy: proxy}
	ctx := context.Background()
	result, err := tool.Execute(ctx, nil, map[string]interface{}{
		"file":      "test.go",
		"symbol":    "foo",
		"line":      5,
		"character": 3,
	})
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if !result.Success {
		t.Error("expected success")
	}
	loc, ok := result.Data["location"].(Location)
	if !ok {
		t.Fatal("location missing")
	}
	if loc.URI != "file:///test.go" {
		t.Errorf("unexpected URI: %s", loc.URI)
	}
	if !mock.calledDef {
		t.Error("GetDefinition not called")
	}
}

func TestReferencesTool_Execute(t *testing.T) {
	proxy := NewProxy(time.Minute)
	mock := &mockLSPClient{
		refsResult: []Location{{URI: "file:///a.go", Range: [2]int64{1, 2}}},
	}
	proxy.Register("go", mock)

	tool := &ReferencesTool{Proxy: proxy}
	ctx := context.Background()
	result, err := tool.Execute(ctx, nil, map[string]interface{}{
		"file":      "test.go",
		"symbol":    "Foo",
		"line":      10,
		"character": 0,
	})
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if !result.Success {
		t.Error("expected success")
	}
	locs, ok := result.Data["locations"].([]Location)
	if !ok || len(locs) != 1 {
		t.Fatal("locations missing")
	}
	if !mock.calledRefs {
		t.Error("GetReferences not called")
	}
}

func TestHoverTool_Execute(t *testing.T) {
	proxy := NewProxy(time.Minute)
	mock := &mockLSPClient{
		hoverResult: HoverResult{
			TypeInfo: "int",
			Docs:     "returns an integer",
		},
	}
	proxy.Register("go", mock)

	tool := &HoverTool{Proxy: proxy}
	ctx := context.Background()
	result, err := tool.Execute(ctx, nil, map[string]interface{}{
		"file":      "test.go",
		"line":      7,
		"character": 4,
	})
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if !result.Success {
		t.Error("expected success")
	}
	ti, ok := result.Data["type"].(string)
	if !ok || ti != "int" {
		t.Errorf("unexpected type: %v", ti)
	}
	if !mock.calledHover {
		t.Error("GetHover not called")
	}
}

func TestDiagnosticsTool_Execute(t *testing.T) {
	proxy := NewProxy(time.Minute)
	mock := &mockLSPClient{
		diagResult: []Diagnostic{
			{Severity: "1", Message: "error", Source: "test", Line: 5},
		},
	}
	proxy.Register("go", mock)

	tool := &DiagnosticsTool{Proxy: proxy}
	ctx := context.Background()
	result, err := tool.Execute(ctx, nil, map[string]interface{}{
		"file": "test.go",
	})
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if !result.Success {
		t.Error("expected success")
	}
	diags, ok := result.Data["diagnostics"].([]Diagnostic)
	if !ok || len(diags) != 1 {
		t.Fatal("diagnostics missing")
	}
	if diags[0].Line != 5 {
		t.Errorf("unexpected line: %d", diags[0].Line)
	}
	if !mock.calledDiag {
		t.Error("GetDiagnostics not called")
	}
}

func TestFormatTool_Execute(t *testing.T) {
	proxy := NewProxy(time.Minute)
	mock := &mockLSPClient{
		formatResult: "formatted code",
	}
	proxy.Register("go", mock)

	tool := &FormatTool{Proxy: proxy}
	ctx := context.Background()
	result, err := tool.Execute(ctx, nil, map[string]interface{}{
		"file": "test.go",
		"code": "bad code",
	})
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if !result.Success {
		t.Error("expected success")
	}
	code, ok := result.Data["code"].(string)
	if !ok || code != "formatted code" {
		t.Errorf("unexpected formatted code: %v", code)
	}
	if !mock.calledFormat {
		t.Error("Format not called")
	}
}
