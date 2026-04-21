package lsp

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"codeburg.org/lexbit/relurpify/framework/authorization"
	"codeburg.org/lexbit/relurpify/framework/core"
	"github.com/sourcegraph/jsonrpc2"
	"github.com/stretchr/testify/require"
	"go.lsp.dev/protocol"
)

type bufferWriteCloser struct{ bytes.Buffer }

func (b *bufferWriteCloser) Close() error { return nil }

type flakyReader struct {
	count int
}

func (r *flakyReader) Read(p []byte) (int, error) {
	if r.count == 0 {
		r.count++
		copy(p, []byte("one\n"))
		return 4, nil
	}
	return 0, errors.New("read failed")
}

func TestMain(m *testing.M) {
	if os.Getenv("LSP_FAKE_SERVER") == "1" {
		runFakeProcessLSPServer()
		return
	}
	os.Exit(m.Run())
}

type toolFakeClient struct {
	definitionCalls      int
	referencesCalls      int
	hoverCalls           int
	diagnosticsCalls     int
	searchSymbolsCalls   int
	documentSymbolsCalls int
	formatCalls          int

	definitionResult      DefinitionResult
	referencesResult      []Location
	hoverResult           HoverResult
	diagnosticsResult     []Diagnostic
	searchSymbolsResult   []SymbolInformation
	documentSymbolsResult []SymbolInformation
	formatResult          string
}

func (f *toolFakeClient) GetDefinition(context.Context, DefinitionRequest) (DefinitionResult, error) {
	f.definitionCalls++
	return f.definitionResult, nil
}
func (f *toolFakeClient) GetReferences(context.Context, ReferencesRequest) ([]Location, error) {
	f.referencesCalls++
	return append([]Location(nil), f.referencesResult...), nil
}
func (f *toolFakeClient) GetHover(context.Context, HoverRequest) (HoverResult, error) {
	f.hoverCalls++
	return f.hoverResult, nil
}
func (f *toolFakeClient) GetDiagnostics(context.Context, string) ([]Diagnostic, error) {
	f.diagnosticsCalls++
	return append([]Diagnostic(nil), f.diagnosticsResult...), nil
}
func (f *toolFakeClient) SearchSymbols(context.Context, string) ([]SymbolInformation, error) {
	f.searchSymbolsCalls++
	return append([]SymbolInformation(nil), f.searchSymbolsResult...), nil
}
func (f *toolFakeClient) GetDocumentSymbols(context.Context, string) ([]SymbolInformation, error) {
	f.documentSymbolsCalls++
	return append([]SymbolInformation(nil), f.documentSymbolsResult...), nil
}
func (f *toolFakeClient) Format(context.Context, FormatRequest) (string, error) {
	f.formatCalls++
	return f.formatResult, nil
}

func TestLSPHelpersAndProxyCoverage(t *testing.T) {
	require.Equal(t, "file:///tmp/example.go", pathToURI("/tmp/example.go"))
	require.Equal(t, "file:///example.go", pathToURI("example.go"))
	require.Equal(t, "file:///", pathToURI("."))
	require.Equal(t, "file:///", pathToURI(""))
	require.Equal(t, "/tmp/example.go", uriToPath("file:///tmp/example.go"))
	require.Equal(t, 7, toInt(int64(7)))
	require.Equal(t, 9, toInt(int32(9)))
	require.Equal(t, 4, toInt(float64(4)))
	require.Equal(t, 0, toInt("abc"))

	proxy := NewProxy(0)
	client := &toolFakeClient{definitionResult: DefinitionResult{Snippet: "snippet", Signature: "func Hello()"}}
	proxy.Register("go", client)
	got, err := proxy.clientForFile("main.go")
	require.NoError(t, err)
	require.NotNil(t, got)

	defTool := &DefinitionTool{Proxy: proxy}
	res, err := defTool.Execute(context.Background(), core.NewContext(), map[string]interface{}{
		"file":      "main.go",
		"symbol":    "Hello",
		"line":      1,
		"character": 0,
	})
	require.NoError(t, err)
	require.True(t, res.Success)
	require.Equal(t, 1, client.definitionCalls)

	cacheCalls := 0
	val, err := proxy.cached("key", func() (interface{}, error) {
		cacheCalls++
		return "value", nil
	})
	require.NoError(t, err)
	require.Equal(t, "value", val)
	val, err = proxy.cached("key", func() (interface{}, error) {
		cacheCalls++
		return "other", nil
	})
	require.NoError(t, err)
	require.Equal(t, "value", val)
	require.Equal(t, 1, cacheCalls)
}

func TestLSPToolParametersAndAvailability(t *testing.T) {
	toolCases := []struct {
		tool        core.Tool
		expectedLen int
	}{
		{tool: &DefinitionTool{}, expectedLen: 4},
		{tool: &ReferencesTool{}, expectedLen: 4},
		{tool: &HoverTool{}, expectedLen: 3},
		{tool: &DiagnosticsTool{}, expectedLen: 1},
		{tool: &SearchSymbolsTool{}, expectedLen: 1},
		{tool: &DocumentSymbolsTool{}, expectedLen: 1},
		{tool: &FormatTool{}, expectedLen: 2},
	}
	for _, tc := range toolCases {
		require.Len(t, tc.tool.Parameters(), tc.expectedLen)
		require.False(t, tc.tool.IsAvailable(context.Background(), core.NewContext()))
	}
}

func TestLSPToolMetadataAndExecution(t *testing.T) {
	workspace := t.TempDir()
	file := filepath.Join(workspace, "main.go")
	require.NoError(t, os.WriteFile(file, []byte("package main\n\nfunc main() {}\n"), 0o600))

	pm, err := authorization.NewPermissionManager(workspace, core.NewFileSystemPermissionSet(workspace, core.FileSystemRead), nil, nil)
	require.NoError(t, err)

	proxy := NewProxy(time.Minute)
	client := &toolFakeClient{
		definitionResult: DefinitionResult{
			Location:  Location{URI: "file:///tmp/main.go", Range: [2]int64{1, 3}},
			Snippet:   "definition snippet",
			Signature: "func Hello()",
		},
		referencesResult: []Location{
			{URI: "file:///tmp/main.go", Range: [2]int64{2, 4}},
		},
		hoverResult: HoverResult{TypeInfo: "string", Docs: "docs"},
		diagnosticsResult: []Diagnostic{
			{Severity: "1", Message: "oops", Source: "test", Line: 9},
		},
		searchSymbolsResult: []SymbolInformation{
			{Name: "sym", Kind: "12", Location: "file:///tmp/main.go:5"},
		},
		documentSymbolsResult: []SymbolInformation{
			{Name: "doc", Kind: "5", Location: "file:///tmp/main.go:7"},
		},
		formatResult: "formatted code",
	}
	proxy.Register("go", client)
	proxy.Register("txt", client)

	tools := []struct {
		tool     core.Tool
		name     string
		desc     string
		category string
		tags     []string
		execute  func(*testing.T)
	}{
		{
			tool:     &DefinitionTool{Proxy: proxy},
			name:     "lsp_get_definition",
			desc:     "Finds the definition for a symbol.",
			category: "lsp",
			tags:     []string{core.TagReadOnly},
			execute: func(t *testing.T) {
				tool := &DefinitionTool{Proxy: proxy}
				tool.SetPermissionManager(pm, "agent")
				res, err := tool.Execute(context.Background(), core.NewContext(), map[string]interface{}{
					"file":      file,
					"symbol":    "Hello",
					"line":      int64(2),
					"character": float64(1),
				})
				require.NoError(t, err)
				require.True(t, res.Success)
				require.Equal(t, "definition snippet", res.Data["snippet"])
				require.Equal(t, "func Hello()", res.Data["signature"])
			},
		},
		{
			tool:     &ReferencesTool{Proxy: proxy},
			name:     "lsp_get_references",
			desc:     "Lists references for a symbol.",
			category: "lsp",
			tags:     []string{core.TagReadOnly},
			execute: func(t *testing.T) {
				tool := &ReferencesTool{Proxy: proxy}
				tool.SetPermissionManager(pm, "agent")
				res, err := tool.Execute(context.Background(), core.NewContext(), map[string]interface{}{
					"file":      file,
					"symbol":    "Hello",
					"line":      2,
					"character": 1,
				})
				require.NoError(t, err)
				require.True(t, res.Success)
				require.Len(t, res.Data["locations"].([]Location), 1)
			},
		},
		{
			tool:     &HoverTool{Proxy: proxy},
			name:     "lsp_get_hover",
			desc:     "Retrieves type information for a position.",
			category: "lsp",
			tags:     []string{core.TagReadOnly},
			execute: func(t *testing.T) {
				tool := &HoverTool{Proxy: proxy}
				tool.SetPermissionManager(pm, "agent")
				res, err := tool.Execute(context.Background(), core.NewContext(), map[string]interface{}{
					"file":      file,
					"line":      2,
					"character": 1,
				})
				require.NoError(t, err)
				require.True(t, res.Success)
				require.Equal(t, "string", res.Data["type"])
			},
		},
		{
			tool:     &DiagnosticsTool{Proxy: proxy},
			name:     "lsp_get_diagnostics",
			desc:     "Retrieves diagnostics for a file.",
			category: "lsp",
			tags:     []string{core.TagReadOnly},
			execute: func(t *testing.T) {
				tool := &DiagnosticsTool{Proxy: proxy}
				tool.SetPermissionManager(pm, "agent")
				res, err := tool.Execute(context.Background(), core.NewContext(), map[string]interface{}{
					"file": file,
				})
				require.NoError(t, err)
				require.True(t, res.Success)
				require.Len(t, res.Data["diagnostics"].([]Diagnostic), 1)
			},
		},
		{
			tool:     &SearchSymbolsTool{Proxy: proxy},
			name:     "lsp_search_symbols",
			desc:     "Searches workspace symbols.",
			category: "lsp",
			tags:     []string{core.TagReadOnly},
			execute: func(t *testing.T) {
				tool := &SearchSymbolsTool{Proxy: proxy}
				res, err := tool.Execute(context.Background(), core.NewContext(), map[string]interface{}{
					"query": "Hello",
				})
				require.NoError(t, err)
				require.True(t, res.Success)
				require.Len(t, res.Data["symbols"].([]SymbolInformation), 2)
			},
		},
		{
			tool:     &DocumentSymbolsTool{Proxy: proxy},
			name:     "lsp_document_symbols",
			desc:     "Lists symbols in a document.",
			category: "lsp",
			tags:     []string{core.TagReadOnly},
			execute: func(t *testing.T) {
				tool := &DocumentSymbolsTool{Proxy: proxy}
				tool.SetPermissionManager(pm, "agent")
				res, err := tool.Execute(context.Background(), core.NewContext(), map[string]interface{}{
					"file": file,
				})
				require.NoError(t, err)
				require.True(t, res.Success)
				require.Len(t, res.Data["symbols"].([]SymbolInformation), 1)
			},
		},
		{
			tool:     &FormatTool{Proxy: proxy},
			name:     "lsp_format",
			desc:     "Formats code using the language server.",
			category: "lsp",
			tags:     []string{core.TagDestructive},
			execute: func(t *testing.T) {
				tool := &FormatTool{Proxy: proxy}
				tool.SetPermissionManager(pm, "agent")
				res, err := tool.Execute(context.Background(), core.NewContext(), map[string]interface{}{
					"file": file,
					"code": "code",
				})
				require.NoError(t, err)
				require.True(t, res.Success)
				require.Equal(t, "formatted code", res.Data["code"])
			},
		},
	}

	for _, tc := range tools {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.name, tc.tool.Name())
			require.Equal(t, tc.desc, tc.tool.Description())
			require.Equal(t, tc.category, tc.tool.Category())
			require.NoError(t, tc.tool.Permissions().Validate())
			require.Equal(t, tc.tags, tc.tool.Tags())
			require.True(t, tc.tool.IsAvailable(context.Background(), core.NewContext()))
			tc.execute(t)
		})
	}

	for _, tool := range []core.Tool{&DefinitionTool{}, &ReferencesTool{}, &HoverTool{}, &DiagnosticsTool{}, &SearchSymbolsTool{}, &DocumentSymbolsTool{}, &FormatTool{}} {
		require.False(t, tool.IsAvailable(context.Background(), core.NewContext()))
	}
}

func TestLSPToolSetPermissionManagerAndNilAvailability(t *testing.T) {
	tools := []interface {
		SetPermissionManager(*authorization.PermissionManager, string)
		IsAvailable(context.Context, *core.Context) bool
	}{
		&DefinitionTool{},
		&ReferencesTool{},
		&HoverTool{},
		&DiagnosticsTool{},
		&SearchSymbolsTool{},
		&DocumentSymbolsTool{},
		&FormatTool{},
	}
	for _, tool := range tools {
		tool.SetPermissionManager(nil, "agent")
		require.False(t, tool.IsAvailable(context.Background(), core.NewContext()))
	}
}

func TestProcessLSPClientInMemory(t *testing.T) {
	client, cleanup := newInMemoryProcessClient(t, fakeProcessServerConfig{
		documentSymbols: []protocol.DocumentSymbol{
			{
				Name:  "func1",
				Kind:  protocol.SymbolKindFunction,
				Range: protocol.Range{Start: protocol.Position{Line: 0, Character: 0}},
				Children: []protocol.DocumentSymbol{
					{Name: "inner", Kind: protocol.SymbolKindVariable, Range: protocol.Range{Start: protocol.Position{Line: 2, Character: 0}}},
				},
			},
		},
		workspaceSymbols: []protocol.SymbolInformation{
			{Name: "search-sym", Kind: protocol.SymbolKindFunction, Location: protocol.Location{Range: protocol.Range{Start: protocol.Position{Line: 3, Character: 0}}}},
		},
		definitions: []protocol.Location{
			{Range: protocol.Range{Start: protocol.Position{Line: 1, Character: 0}, End: protocol.Position{Line: 2, Character: 0}}},
		},
		references: []protocol.Location{
			{Range: protocol.Range{Start: protocol.Position{Line: 0, Character: 0}, End: protocol.Position{Line: 0, Character: 0}}},
		},
		hover: protocol.Hover{Contents: protocol.MarkupContent{Kind: protocol.PlainText, Value: "hover docs"}},
		formatEdits: []protocol.TextEdit{
			{Range: protocol.Range{Start: protocol.Position{Line: 0, Character: 0}}, NewText: "formatted-by-server"},
		},
	})
	defer cleanup.close()

	pm, err := authorization.NewPermissionManager(cleanup.root, core.NewFileSystemPermissionSet(cleanup.root, core.FileSystemRead), nil, nil)
	require.NoError(t, err)
	client.manager = pm

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	require.NoError(t, client.initialize(ctx, cleanup.root))
	client.diagnostics[protocol.DocumentURI(cleanup.fileURI)] = []protocol.Diagnostic{
		{
			Severity: protocol.DiagnosticSeverityError,
			Message:  "problem",
			Source:   "server",
			Range: protocol.Range{
				Start: protocol.Position{Line: 1, Character: 0},
				End:   protocol.Position{Line: 1, Character: 4},
			},
		},
	}

	meta := client.ProcessMetadata()
	require.Equal(t, "demo", meta.Command)
	require.Equal(t, []string{"--serve"}, meta.Args)
	require.Greater(t, meta.PID, 0)

	require.NotNil(t, client.Logs())

	def, err := client.GetDefinition(ctx, DefinitionRequest{File: cleanup.file, Symbol: "Hello", Position: Position{Line: 1, Character: 0}})
	require.NoError(t, err)
	require.Equal(t, "line1\nline2", def.Snippet)
	require.Equal(t, cleanup.fileURI, def.Location.URI)
	def2, err := client.GetDefinition(ctx, DefinitionRequest{File: cleanup.file, Symbol: "Hello", Position: Position{Line: 1, Character: 0}})
	require.NoError(t, err)
	require.Equal(t, def.Snippet, def2.Snippet)

	refs, err := client.GetReferences(ctx, ReferencesRequest{File: cleanup.file, Symbol: "Hello", Position: Position{Line: 1, Character: 0}})
	require.NoError(t, err)
	require.Len(t, refs, 1)

	hover, err := client.GetHover(ctx, HoverRequest{File: cleanup.file, Position: Position{Line: 1, Character: 0}})
	require.NoError(t, err)
	require.Equal(t, "hover docs", hover.TypeInfo)

	diags, err := client.GetDiagnostics(ctx, cleanup.file)
	require.NoError(t, err)
	require.Len(t, diags, 1)
	require.Equal(t, "1", diags[0].Severity)

	syms, err := client.SearchSymbols(ctx, "Hello")
	require.NoError(t, err)
	require.Len(t, syms, 1)

	docSyms, err := client.GetDocumentSymbols(ctx, cleanup.file)
	require.NoError(t, err)
	require.Len(t, docSyms, 2)
	require.Equal(t, "func1", docSyms[0].Name)
	require.Equal(t, "inner", docSyms[1].Name)
	docSyms2, err := client.GetDocumentSymbols(ctx, cleanup.file)
	require.NoError(t, err)
	require.Len(t, docSyms2, 2)

	formatted, err := client.Format(ctx, FormatRequest{File: cleanup.file, Code: "original"})
	require.NoError(t, err)
	require.Equal(t, "formatted-by-server", formatted)

	require.NoError(t, client.Close())
}

func TestNewProcessLSPClientWithPermissionsFakeServer(t *testing.T) {
	root := t.TempDir()
	file := filepath.Join(root, "main.go")
	require.NoError(t, os.WriteFile(file, []byte("line0\nline1\nline2\nline3\n"), 0o600))

	t.Setenv("LSP_FAKE_SERVER", "1")
	client, err := NewProcessLSPClientWithPermissions(ProcessLSPConfig{
		Command:    os.Args[0],
		Args:       []string{"-test.run=^$"},
		RootDir:    root,
		LanguageID: "go",
	}, nil, "", nil)
	require.NoError(t, err)

	pc, ok := client.(*processLSPClient)
	require.True(t, ok)
	require.NotNil(t, pc.Logs())

	pm, err := authorization.NewPermissionManager(root, core.NewFileSystemPermissionSet(root, core.FileSystemRead), nil, nil)
	require.NoError(t, err)
	pc.manager = pm

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	require.Equal(t, "line1\nline2", mustDefinitionSnippet(t, pc, ctx, file))
	require.Equal(t, "line1\nline2", mustDefinitionSnippet(t, pc, ctx, file))
	require.Len(t, mustReferences(t, pc, ctx, file), 1)
	require.Equal(t, "hover docs", mustHover(t, pc, ctx, file))
	require.Len(t, mustDiagnostics(t, pc, ctx, file), 1)
	require.Len(t, mustSymbols(t, pc, ctx, file), 2)
	require.Len(t, mustSymbols(t, pc, ctx, file), 2)
	require.Equal(t, "formatted-by-server", mustFormat(t, pc, ctx, file))

	require.NoError(t, pc.Close())
}

func TestNewProcessLSPClientWithPermissionsInitializeError(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(root, "main.go"), []byte("line0\nline1\n"), 0o600))
	client, err := NewProcessLSPClientWithPermissions(ProcessLSPConfig{
		Command:    "sh",
		Args:       []string{"-c", "exit 0"},
		RootDir:    root,
		LanguageID: "go",
	}, nil, "", nil)
	require.Error(t, err)
	require.Nil(t, client)
}

func TestNewProcessLSPClientWithPermissionsStartError(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(root, "main.go"), []byte("line0\nline1\n"), 0o600))

	client, err := NewProcessLSPClientWithPermissions(ProcessLSPConfig{
		Command:    "/dev/null",
		RootDir:    root,
		LanguageID: "go",
	}, nil, "", nil)
	require.Error(t, err)
	require.Nil(t, client)
}

func mustDefinitionSnippet(t *testing.T, pc *processLSPClient, ctx context.Context, file string) string {
	t.Helper()
	def, err := pc.GetDefinition(ctx, DefinitionRequest{File: file, Symbol: "Hello", Position: Position{Line: 1, Character: 0}})
	require.NoError(t, err)
	return def.Snippet
}

func mustReferences(t *testing.T, pc *processLSPClient, ctx context.Context, file string) []Location {
	t.Helper()
	refs, err := pc.GetReferences(ctx, ReferencesRequest{File: file, Symbol: "Hello", Position: Position{Line: 1, Character: 0}})
	require.NoError(t, err)
	return refs
}

func mustHover(t *testing.T, pc *processLSPClient, ctx context.Context, file string) string {
	t.Helper()
	hover, err := pc.GetHover(ctx, HoverRequest{File: file, Position: Position{Line: 1, Character: 0}})
	require.NoError(t, err)
	return hover.TypeInfo
}

func mustDiagnostics(t *testing.T, pc *processLSPClient, ctx context.Context, file string) []Diagnostic {
	t.Helper()
	pc.diagnostics[protocol.DocumentURI(pathToURI(file))] = []protocol.Diagnostic{{
		Severity: protocol.DiagnosticSeverityError,
		Message:  "problem",
		Source:   "server",
		Range: protocol.Range{
			Start: protocol.Position{Line: 1, Character: 0},
			End:   protocol.Position{Line: 1, Character: 4},
		},
	}}
	diags, err := pc.GetDiagnostics(ctx, file)
	require.NoError(t, err)
	return diags
}

func mustSymbols(t *testing.T, pc *processLSPClient, ctx context.Context, file string) []SymbolInformation {
	t.Helper()
	syms, err := pc.GetDocumentSymbols(ctx, file)
	require.NoError(t, err)
	return syms
}

func mustFormat(t *testing.T, pc *processLSPClient, ctx context.Context, file string) string {
	t.Helper()
	formatted, err := pc.Format(ctx, FormatRequest{File: file, Code: "original"})
	require.NoError(t, err)
	return formatted
}

func TestProcessLSPClientConsumeLogs(t *testing.T) {
	c := &processLSPClient{cfg: ProcessLSPConfig{LanguageID: "go"}, logCh: make(chan string, 4)}
	c.consumeLogs(bytes.NewBufferString("line-one\nline-two\n"))
	require.Equal(t, "line-one", <-c.logCh)
	require.Equal(t, "line-two", <-c.logCh)
	_, ok := <-c.logCh
	require.False(t, ok)
}

func TestProcessLSPClientConsumeLogsDropAndError(t *testing.T) {
	c := &processLSPClient{cfg: ProcessLSPConfig{LanguageID: "go"}, logCh: make(chan string, 1)}
	c.logCh <- "busy"
	c.consumeLogs(&flakyReader{})
	_, ok := <-c.logCh
	require.True(t, ok)
}

func TestProcessLSPClientLogsNilReceiver(t *testing.T) {
	var c *processLSPClient
	require.Nil(t, c.Logs())
}

func TestProcessLSPClientGetDiagnosticsCanceled(t *testing.T) {
	client, cleanup := newInMemoryProcessClient(t, fakeProcessServerConfig{
		documentSymbols: []protocol.DocumentSymbol{},
	})
	defer cleanup.close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	require.NoError(t, client.initialize(context.Background(), cleanup.root))
	_, err := client.GetDiagnostics(ctx, cleanup.file)
	require.Error(t, err)
}

func TestProcessLSPClientGetDocumentSymbolsInfoFallbackAndFormatEmpty(t *testing.T) {
	client, cleanup := newInMemoryProcessClient(t, fakeProcessServerConfig{
		documentSymbolInfos: []protocol.SymbolInformation{
			{Name: "sym-info", Kind: protocol.SymbolKindClass, Location: protocol.Location{Range: protocol.Range{Start: protocol.Position{Line: 5, Character: 0}}}},
		},
		formatEdits: nil,
	})
	defer cleanup.close()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	require.NoError(t, client.initialize(ctx, cleanup.root))

	syms, err := client.GetDocumentSymbols(ctx, cleanup.file)
	require.NoError(t, err)
	require.Len(t, syms, 1)
	require.Equal(t, "sym-info", syms[0].Name)

	formatted, err := client.Format(ctx, FormatRequest{File: cleanup.file, Code: "original"})
	require.NoError(t, err)
	require.Equal(t, "original", formatted)
}

func TestProcessLSPClientGetDefinitionErrorAndDocumentSymbolsError(t *testing.T) {
	client, cleanup := newInMemoryProcessClient(t, fakeProcessServerConfig{
		definitions: []protocol.Location{},
	})
	defer cleanup.close()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	require.NoError(t, client.initialize(ctx, cleanup.root))

	_, err := client.GetDefinition(ctx, DefinitionRequest{File: cleanup.file, Symbol: "Hello", Position: Position{Line: 1, Character: 0}})
	require.Error(t, err)
}

func TestProcessLSPClientEnsureOpenManagerError(t *testing.T) {
	client, cleanup := newInMemoryProcessClient(t, fakeProcessServerConfig{})
	defer cleanup.close()

	client.manager = &authorization.PermissionManager{}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	err := client.ensureOpen(ctx, cleanup.file)
	require.Error(t, err)
}

func TestProcessLSPClientMethodManagerErrors(t *testing.T) {
	client, cleanup := newInMemoryProcessClient(t, fakeProcessServerConfig{})
	defer cleanup.close()

	client.manager = &authorization.PermissionManager{}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	reset := func() {
		client.openedFiles = make(map[protocol.DocumentURI]bool)
	}

	_, err := client.GetDefinition(ctx, DefinitionRequest{File: cleanup.file, Symbol: "Hello", Position: Position{Line: 1, Character: 0}})
	require.Error(t, err)
	reset()
	_, err = client.GetReferences(ctx, ReferencesRequest{File: cleanup.file, Symbol: "Hello", Position: Position{Line: 1, Character: 0}})
	require.Error(t, err)
	reset()
	_, err = client.GetHover(ctx, HoverRequest{File: cleanup.file, Position: Position{Line: 1, Character: 0}})
	require.Error(t, err)
	reset()
	_, err = client.GetDiagnostics(ctx, cleanup.file)
	require.Error(t, err)
	reset()
	_, err = client.GetDocumentSymbols(ctx, cleanup.file)
	require.Error(t, err)
	reset()
	_, err = client.Format(ctx, FormatRequest{File: cleanup.file, Code: "original"})
	require.Error(t, err)
}

func TestProcessLSPClientCloseNilReceiver(t *testing.T) {
	var client *processLSPClient
	require.NoError(t, client.Close())
}

func TestProcessLSPClientDiagnosticsTimeout(t *testing.T) {
	client, cleanup := newInMemoryProcessClient(t, fakeProcessServerConfig{})
	defer cleanup.close()

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	oldWait := diagnosticsWait
	diagnosticsWait = time.Nanosecond
	defer func() { diagnosticsWait = oldWait }()

	require.NoError(t, client.initialize(context.Background(), cleanup.root))
	_, err := client.GetDiagnostics(ctx, cleanup.file)
	require.Error(t, err)
}

func TestProcessLSPClientDiagnosticsTicker(t *testing.T) {
	client, cleanup := newInMemoryProcessClient(t, fakeProcessServerConfig{})
	defer cleanup.close()

	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
	defer cancel()
	oldWait := diagnosticsWait
	diagnosticsWait = 5 * time.Second
	defer func() { diagnosticsWait = oldWait }()

	require.NoError(t, client.initialize(context.Background(), cleanup.root))
	_, err := client.GetDiagnostics(ctx, cleanup.file)
	require.Error(t, err)
}

func TestProcessLSPClientDefinitionRPCError(t *testing.T) {
	client, cleanup := newInMemoryProcessClient(t, fakeProcessServerConfig{
		definitionError: "definition failed",
	})
	defer cleanup.close()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	require.NoError(t, client.initialize(ctx, cleanup.root))
	_, err := client.GetDefinition(ctx, DefinitionRequest{File: cleanup.file, Symbol: "Hello", Position: Position{Line: 1, Character: 0}})
	require.Error(t, err)
}

func TestProcessLSPClientErrorBranches(t *testing.T) {
	client, cleanup := newInMemoryProcessClient(t, fakeProcessServerConfig{
		hoverError:           "hover failed",
		formatError:          "format failed",
		documentSymbolsRaw:   json.RawMessage("123"),
		referencesError:      "references failed",
		searchError:          "search failed",
		documentSymbolsError: "document symbols failed",
	})
	defer cleanup.close()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	require.NoError(t, client.initialize(ctx, cleanup.root))

	_, err := client.GetHover(ctx, HoverRequest{File: cleanup.file, Position: Position{Line: 1, Character: 0}})
	require.Error(t, err)

	_, err = client.GetDocumentSymbols(ctx, cleanup.file)
	require.Error(t, err)

	_, err = client.Format(ctx, FormatRequest{File: cleanup.file, Code: "original"})
	require.Error(t, err)

	_, err = client.GetReferences(ctx, ReferencesRequest{File: cleanup.file, Symbol: "Hello", Position: Position{Line: 1, Character: 0}})
	require.Error(t, err)

	_, err = client.SearchSymbols(ctx, "Hello")
	require.Error(t, err)

	_, err = client.GetDocumentSymbols(ctx, cleanup.file)
	require.Error(t, err)
}

func TestProcessLSPClientInitializeError(t *testing.T) {
	client, cleanup := newInMemoryProcessClient(t, fakeProcessServerConfig{
		initError: "initialize failed",
	})
	defer cleanup.close()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	err := client.initialize(ctx, cleanup.root)
	require.Error(t, err)
}

func TestProcessLSPClientFormatEditLoop(t *testing.T) {
	client, cleanup := newInMemoryProcessClient(t, fakeProcessServerConfig{
		formatEdits: []protocol.TextEdit{
			{
				Range:   protocol.Range{Start: protocol.Position{Line: 1, Character: 0}},
				NewText: "first",
			},
			{
				Range:   protocol.Range{Start: protocol.Position{Line: 2, Character: 0}},
				NewText: "second",
			},
		},
	})
	defer cleanup.close()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	require.NoError(t, client.initialize(ctx, cleanup.root))

	formatted, err := client.Format(ctx, FormatRequest{File: cleanup.file, Code: "original"})
	require.NoError(t, err)
	require.Equal(t, "second", formatted)
}

func TestProcessLSPClientReadSnippetPermissionManagerError(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "snippet.txt")
	require.NoError(t, os.WriteFile(file, []byte("line1\nline2\n"), 0o600))

	client := &processLSPClient{
		manager: &authorization.PermissionManager{},
		agentID: "agent",
	}
	_, err := client.readSnippet(file, protocol.Range{Start: protocol.Position{Line: 0, Character: 0}, End: protocol.Position{Line: 0, Character: 0}})
	require.Error(t, err)
}

func TestProcessLSPClientReadSnippetMissingFile(t *testing.T) {
	client := &processLSPClient{}
	_, err := client.readSnippet(filepath.Join(t.TempDir(), "missing.txt"), protocol.Range{Start: protocol.Position{Line: 0, Character: 0}, End: protocol.Position{Line: 0, Character: 0}})
	require.Error(t, err)
}

func TestProcessLSPClientReadWriteClose(t *testing.T) {
	rwc := &stdioReadWriteCloser{reader: io.NopCloser(bytes.NewBufferString("abc")), writer: &bufferWriteCloser{}}
	buf := make([]byte, 3)
	n, err := rwc.Read(buf)
	require.NoError(t, err)
	require.Equal(t, 3, n)
	require.Equal(t, "abc", string(buf))
	n, err = rwc.Write([]byte("xyz"))
	require.NoError(t, err)
	require.Equal(t, 3, n)
	require.NoError(t, rwc.Close())
}

func TestLanguageServerWrappers(t *testing.T) {
	root := t.TempDir()
	t.Setenv("PATH", t.TempDir())
	constructors := []func(string) (LSPClient, error){
		NewRustAnalyzerClient,
		NewGoplsClient,
		NewClangdClient,
		NewHaskellClient,
		NewTypeScriptClient,
		NewLuaClient,
		NewPythonLSPClient,
	}
	for _, ctor := range constructors {
		client, err := ctor(root)
		require.Error(t, err)
		require.Nil(t, client)
	}
}

type fakeProcessServerConfig struct {
	diagnostics          []protocol.Diagnostic
	documentSymbols      []protocol.DocumentSymbol
	documentSymbolInfos  []protocol.SymbolInformation
	workspaceSymbols     []protocol.SymbolInformation
	definitions          []protocol.Location
	references           []protocol.Location
	hover                protocol.Hover
	formatEdits          []protocol.TextEdit
	hoverError           string
	formatError          string
	documentSymbolsRaw   json.RawMessage
	initError            string
	referencesError      string
	searchError          string
	documentSymbolsError string
	definitionError      string
}

type inMemoryProcessCleanup struct {
	root    string
	file    string
	fileURI string
	close   func()
}

func newInMemoryProcessClient(t *testing.T, cfg fakeProcessServerConfig) (*processLSPClient, inMemoryProcessCleanup) {
	t.Helper()

	root := t.TempDir()
	file := filepath.Join(root, "main.go")
	content := "line0\nline1\nline2\nline3\nline4\n"
	require.NoError(t, os.WriteFile(file, []byte(content), 0o600))
	fileURI := pathToURI(file)

	for i := range cfg.definitions {
		if cfg.definitions[i].URI == "" {
			cfg.definitions[i].URI = protocol.DocumentURI(fileURI)
		}
	}
	for i := range cfg.references {
		if cfg.references[i].URI == "" {
			cfg.references[i].URI = protocol.DocumentURI(fileURI)
		}
	}
	for i := range cfg.workspaceSymbols {
		if cfg.workspaceSymbols[i].Location.URI == "" {
			cfg.workspaceSymbols[i].Location.URI = protocol.DocumentURI(fileURI)
		}
	}
	for i := range cfg.documentSymbolInfos {
		if cfg.documentSymbolInfos[i].Location.URI == "" {
			cfg.documentSymbolInfos[i].Location.URI = protocol.DocumentURI(fileURI)
		}
	}

	clientConn, serverConn, cancel := newInMemoryJSONRPCPair(t, cfg, fileURI)
	_ = serverConn

	client := &processLSPClient{
		cfg:         ProcessLSPConfig{Command: "demo", Args: []string{"--serve"}, RootDir: root, LanguageID: "go"},
		conn:        clientConn,
		startedAt:   time.Unix(1700000000, 0).UTC(),
		openedFiles: make(map[protocol.DocumentURI]bool),
		diagnostics: make(map[protocol.DocumentURI][]protocol.Diagnostic),
		logCh:       make(chan string, 8),
	}

	sleepCmd := exec.Command("sleep", "60")
	require.NoError(t, sleepCmd.Start())
	client.cmd = sleepCmd

	return client, inMemoryProcessCleanup{
		root:    root,
		file:    file,
		fileURI: fileURI,
		close: func() {
			_ = client.Close()
			cancel()
		},
	}
}

func newInMemoryJSONRPCPair(t *testing.T, cfg fakeProcessServerConfig, fileURI string) (*jsonrpc2.Conn, *jsonrpc2.Conn, context.CancelFunc) {
	t.Helper()

	clientSide, serverSide := net.Pipe()
	ctx, cancel := context.WithCancel(context.Background())

	var serverConn *jsonrpc2.Conn
	handler := jsonrpc2.HandlerWithError(func(ctx context.Context, conn *jsonrpc2.Conn, req *jsonrpc2.Request) (interface{}, error) {
		switch req.Method {
		case "initialize":
			if cfg.initError != "" {
				return nil, errors.New(cfg.initError)
			}
			return protocol.InitializeResult{}, nil
		case "initialized":
			return nil, nil
		case "textDocument/didOpen":
			if len(cfg.diagnostics) > 0 {
				go func() {
					_ = serverConn.Notify(context.Background(), "textDocument/publishDiagnostics", protocol.PublishDiagnosticsParams{
						URI:         protocol.DocumentURI(fileURI),
						Diagnostics: cfg.diagnostics,
						Version:     1,
					})
				}()
			}
			return nil, nil
		case "textDocument/definition":
			if cfg.definitionError != "" {
				return nil, errors.New(cfg.definitionError)
			}
			return cfg.definitions, nil
		case "textDocument/references":
			if cfg.referencesError != "" {
				return nil, errors.New(cfg.referencesError)
			}
			return cfg.references, nil
		case "textDocument/hover":
			if cfg.hoverError != "" {
				return nil, errors.New(cfg.hoverError)
			}
			return cfg.hover, nil
		case "workspace/symbol":
			if cfg.searchError != "" {
				return nil, errors.New(cfg.searchError)
			}
			return cfg.workspaceSymbols, nil
		case "textDocument/documentSymbol":
			if cfg.documentSymbolsError != "" {
				return nil, errors.New(cfg.documentSymbolsError)
			}
			if cfg.documentSymbolsRaw != nil {
				return cfg.documentSymbolsRaw, nil
			}
			if len(cfg.documentSymbols) > 0 {
				var raw json.RawMessage
				b, _ := json.Marshal(cfg.documentSymbols)
				raw = b
				return raw, nil
			}
			b, _ := json.Marshal(cfg.documentSymbolInfos)
			return json.RawMessage(b), nil
		case "textDocument/formatting":
			if cfg.formatError != "" {
				return nil, errors.New(cfg.formatError)
			}
			return cfg.formatEdits, nil
		default:
			return nil, &jsonrpc2.Error{Code: jsonrpc2.CodeMethodNotFound, Message: "unknown method"}
		}
	})

	serverConn = jsonrpc2.NewConn(ctx, jsonrpc2.NewBufferedStream(serverSide, jsonrpc2.VSCodeObjectCodec{}), handler)
	clientConn := jsonrpc2.NewConn(ctx, jsonrpc2.NewBufferedStream(clientSide, jsonrpc2.VSCodeObjectCodec{}), jsonrpc2.HandlerWithError(func(context.Context, *jsonrpc2.Conn, *jsonrpc2.Request) (interface{}, error) {
		return nil, nil
	}))

	return clientConn, serverConn, cancel
}

func runFakeProcessLSPServer() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	stream := jsonrpc2.NewBufferedStream(&stdioReadWriteCloser{reader: os.Stdin, writer: os.Stdout}, jsonrpc2.VSCodeObjectCodec{})
	var serverConn *jsonrpc2.Conn
	handler := jsonrpc2.HandlerWithError(func(ctx context.Context, conn *jsonrpc2.Conn, req *jsonrpc2.Request) (interface{}, error) {
		switch req.Method {
		case "initialize":
			fmt.Fprintln(os.Stderr, "fake lsp server starting")
			return protocol.InitializeResult{}, nil
		case "initialized":
			return nil, nil
		case "textDocument/didOpen":
			var params protocol.DidOpenTextDocumentParams
			if req.Params != nil {
				_ = json.Unmarshal(*req.Params, &params)
				go func(uri protocol.DocumentURI) {
					_ = serverConn.Notify(context.Background(), "textDocument/publishDiagnostics", protocol.PublishDiagnosticsParams{
						URI: uri,
						Diagnostics: []protocol.Diagnostic{{
							Severity: protocol.DiagnosticSeverityError,
							Message:  "problem",
							Source:   "server",
							Range: protocol.Range{
								Start: protocol.Position{Line: 1, Character: 0},
								End:   protocol.Position{Line: 1, Character: 4},
							},
						}},
						Version: 1,
					})
				}(params.TextDocument.URI)
			}
			return nil, nil
		case "textDocument/definition":
			var params protocol.DefinitionParams
			_ = json.Unmarshal(*req.Params, &params)
			return []protocol.Location{{
				URI: params.TextDocument.URI,
				Range: protocol.Range{
					Start: protocol.Position{Line: 1, Character: 0},
					End:   protocol.Position{Line: 2, Character: 0},
				},
			}}, nil
		case "textDocument/references":
			var params protocol.ReferenceParams
			_ = json.Unmarshal(*req.Params, &params)
			return []protocol.Location{{
				URI: params.TextDocument.URI,
				Range: protocol.Range{
					Start: protocol.Position{Line: 0, Character: 0},
					End:   protocol.Position{Line: 0, Character: 0},
				},
			}}, nil
		case "textDocument/hover":
			return protocol.Hover{Contents: protocol.MarkupContent{Kind: protocol.PlainText, Value: "hover docs"}}, nil
		case "workspace/symbol":
			return []protocol.SymbolInformation{{
				Name: "search-sym",
				Kind: protocol.SymbolKindFunction,
				Location: protocol.Location{
					URI: protocol.DocumentURI("file:///tmp/main.go"),
					Range: protocol.Range{
						Start: protocol.Position{Line: 3, Character: 0},
					},
				},
			}}, nil
		case "textDocument/documentSymbol":
			return []protocol.DocumentSymbol{{
				Name: "func1",
				Kind: protocol.SymbolKindFunction,
				Range: protocol.Range{
					Start: protocol.Position{Line: 0, Character: 0},
				},
				Children: []protocol.DocumentSymbol{{
					Name:  "inner",
					Kind:  protocol.SymbolKindVariable,
					Range: protocol.Range{Start: protocol.Position{Line: 2, Character: 0}},
				}},
			}}, nil
		case "textDocument/formatting":
			return []protocol.TextEdit{{
				Range:   protocol.Range{Start: protocol.Position{Line: 0, Character: 0}},
				NewText: "formatted-by-server",
			}}, nil
		default:
			return nil, &jsonrpc2.Error{Code: jsonrpc2.CodeMethodNotFound, Message: "unknown method"}
		}
	})
	serverConn = jsonrpc2.NewConn(ctx, stream, handler)
	<-serverConn.DisconnectNotify()
}
