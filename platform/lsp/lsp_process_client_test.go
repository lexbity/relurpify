package lsp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/sourcegraph/jsonrpc2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.lsp.dev/protocol"

	"codeburg.org/lexbit/relurpify/capability/ports"
	"codeburg.org/lexbit/relurpify/capability/sandbox"
)

// routingFiles are the file names the fake language server dispatches on.
// They must exist on disk because the client opens them before querying.
var routingFiles = []string{
	"main.go", "nodef.go", "missingtarget.go", "docsym.go", "rawinfo.go",
	"garbage.go", "fmtnone.go", "fmtfull.go", "fmtmany.go", "diag.go", "quiet.go",
	"decl.go", "use.go",
}

// TestMain doubles as the fake language-server entrypoint. When the test
// binary is re-executed by the fake-lsp wrapper script (see newFakeClient),
// it speaks LSP over stdio instead of running tests.
func TestMain(m *testing.M) {
	if os.Getenv("RELURPIFY_FAKE_LSP") == "1" {
		os.Exit(runFakeLSPServer(findFlagArgs()))
	}
	os.Exit(m.Run())
}

func findFlagArgs() []string {
	for i, arg := range os.Args {
		if arg == "--" {
			return os.Args[i+1:]
		}
	}
	return nil
}

// TestFakeLSPServerProcess is never executed by m.Run in helper mode; it only
// gives -test.run a harmless target for the re-executed binary.
func TestFakeLSPServerProcess(t *testing.T) {
	t.Skip("helper process only")
}

func runFakeLSPServer(args []string) int {
	if len(args) < 2 {
		fmt.Fprintln(os.Stderr, "fake-lsp: expected <mode> <root> args")
		return 2
	}
	root := args[1]

	handler := jsonrpc2.HandlerWithError(func(ctx context.Context, conn *jsonrpc2.Conn, req *jsonrpc2.Request) (any, error) {
		if req.Notif {
			return handleFakeNotification(ctx, conn, req)
		}
		return handleFakeRequest(req, root)
	})

	conn := jsonrpc2.NewConn(context.Background(),
		jsonrpc2.NewBufferedStream(&stdioReadWriteCloser{reader: os.Stdin, writer: os.Stdout},
			jsonrpc2.VSCodeObjectCodec{}),
		handler)
	<-conn.DisconnectNotify()
	_ = conn.Close()
	return 0
}

func handleFakeNotification(ctx context.Context, conn *jsonrpc2.Conn, req *jsonrpc2.Request) (any, error) {
	switch req.Method {
	case "initialized":
		// One malformed publishDiagnostics notification exercises the
		// client-side JSON decode failure branch.
		_ = conn.Notify(ctx, "textDocument/publishDiagnostics", json.RawMessage(`"not-an-object"`))
		return nil, nil
	case "textDocument/didOpen":
		var params struct {
			TextDocument struct {
				URI string `json:"uri"`
			} `json:"textDocument"`
		}
		if err := json.Unmarshal(*req.Params, &params); err != nil {
			return nil, err
		}
		if strings.Contains(params.TextDocument.URI, "diag.go") {
			publish := map[string]any{
				"uri": params.TextDocument.URI,
				"diagnostics": []map[string]any{
					{
						"range":    protocolRange(0, 0, 0, 5),
						"severity": 1,
						"message":  "fake error",
						"source":   "fake-lsp",
					},
					{
						"range":    protocolRange(3, 2, 3, 9),
						"severity": 2,
						"message":  "fake warning",
						"source":   "fake-lsp",
					},
				},
			}
			_ = conn.Notify(ctx, "textDocument/publishDiagnostics", publish)
		}
		return nil, nil
	default:
		return nil, nil
	}
}

func handleFakeRequest(req *jsonrpc2.Request, root string) (any, error) {
	type docPos struct {
		TextDocument struct {
			URI string `json:"uri"`
		} `json:"textDocument"`
	}
	uriOf := func(name string) protocol.DocumentURI {
		return protocol.DocumentURI("file://" + filepath.Join(root, name))
	}
	switch req.Method {
	case "initialize":
		return protocol.InitializeResult{Capabilities: protocol.ServerCapabilities{}}, nil
	case "textDocument/definition":
		var p docPos
		if err := json.Unmarshal(*req.Params, &p); err != nil {
			return nil, err
		}
		switch {
		case strings.Contains(p.TextDocument.URI, "nodef.go"):
			return []protocol.Location{}, nil
		case strings.Contains(p.TextDocument.URI, "missingtarget.go"):
			return []protocol.Location{{
				URI:   "file:///definitely/not/on/disk.go",
				Range: protocol.Range{Start: protocol.Position{Line: 0}, End: protocol.Position{Line: 0, Character: 3}},
			}}, nil
		default:
			return []protocol.Location{{
				URI:   uriOf("decl.go"),
				Range: protocol.Range{Start: protocol.Position{Line: 0}, End: protocol.Position{Line: 0, Character: 10}},
			}}, nil
		}
	case "textDocument/references":
		return []protocol.Location{
			{URI: uriOf("decl.go"), Range: protocol.Range{Start: protocol.Position{Line: 1}, End: protocol.Position{Line: 1, Character: 4}}},
			{URI: uriOf("use.go"), Range: protocol.Range{Start: protocol.Position{Line: 7, Character: 2}, End: protocol.Position{Line: 7, Character: 6}}},
		}, nil
	case "textDocument/hover":
		return protocol.Hover{Contents: protocol.MarkupContent{Kind: protocol.Markdown, Value: "fake hover docs"}}, nil
	case "workspace/symbol":
		return []protocol.SymbolInformation{{
			Name: "FakeSymbol",
			Kind: 12,
			Location: protocol.Location{
				URI:   uriOf("decl.go"),
				Range: protocol.Range{Start: protocol.Position{Line: 2}, End: protocol.Position{Line: 2, Character: 10}},
			},
		}}, nil
	case "textDocument/documentSymbol":
		var p docPos
		if err := json.Unmarshal(*req.Params, &p); err != nil {
			return nil, err
		}
		switch {
		case strings.Contains(p.TextDocument.URI, "rawinfo.go"):
			return []protocol.SymbolInformation{{
				Name: "InfoSym",
				Kind: 6,
				Location: protocol.Location{
					URI:   uriOf("rawinfo.go"),
					Range: protocol.Range{Start: protocol.Position{Line: 4}, End: protocol.Position{Line: 4, Character: 8}},
				},
			}}, nil
		case strings.Contains(p.TextDocument.URI, "garbage.go"):
			return json.RawMessage(`{"unexpected":"shape"}`), nil
		default:
			return []protocol.DocumentSymbol{{
				Name: "Parent",
				Kind: 5,
				Range: protocol.Range{
					Start: protocol.Position{Line: 0},
					End:   protocol.Position{Line: 9},
				},
				Children: []protocol.DocumentSymbol{{
					Name: "Child",
					Kind: 6,
					Range: protocol.Range{
						Start: protocol.Position{Line: 1},
						End:   protocol.Position{Line: 2},
					},
				}},
			}}, nil
		}
	case "textDocument/formatting":
		var p docPos
		if err := json.Unmarshal(*req.Params, &p); err != nil {
			return nil, err
		}
		switch {
		case strings.Contains(p.TextDocument.URI, "fmtnone.go"):
			return nil, nil
		case strings.Contains(p.TextDocument.URI, "fmtfull.go"):
			return []protocol.TextEdit{{
				Range:   protocol.Range{Start: protocol.Position{Line: 0}, End: protocol.Position{Line: 500}},
				NewText: "package formatted\n",
			}}, nil
		default:
			return []protocol.TextEdit{
				{Range: protocol.Range{Start: protocol.Position{Line: 0}}, NewText: "EDIT1"},
				{Range: protocol.Range{Start: protocol.Position{Line: 1}}, NewText: "EDIT2"},
			}, nil
		}
	default:
		return nil, fmt.Errorf("fake-lsp: unhandled method %q", req.Method)
	}
}

func protocolRange(sl, sc, el, ec int) map[string]any {
	return map[string]any{
		"start": map[string]any{"line": sl, "character": sc},
		"end":   map[string]any{"line": el, "character": ec},
	}
}

// newFakeClient launches the process LSP client against the in-repo fake
// language server. The server is the test binary re-executed through a
// wrapper script, so no network or system language server is involved.
// It returns the client and the fake workspace root holding the routing files.
func newFakeClient(t *testing.T, mode string) (*processLSPClient, string) {
	t.Helper()

	testBin, err := os.Executable()
	require.NoError(t, err)

	binDir := t.TempDir()
	script := filepath.Join(binDir, "fake-lsp")
	root := t.TempDir()
	for _, name := range routingFiles {
		require.NoError(t, os.WriteFile(filepath.Join(root, name), []byte("func Fake() {}\n"), 0o600))
	}
	body := fmt.Sprintf("#!/bin/sh\necho 'fake-lsp stderr line for %s' >&2\nRELURPIFY_FAKE_LSP=1 exec '%s' -test.run=TestFakeLSPServerProcess -- %s '%s'\n",
		mode, testBin, mode, root)
	require.NoError(t, os.WriteFile(script, []byte(body), 0o755))

	clientAny, err := newProcessLSPClientInternal(ProcessLSPConfig{
		Command:    script,
		LanguageID: "go",
		RootDir:    root,
	}, nil)
	require.NoError(t, err)

	concrete, ok := clientAny.(*processLSPClient)
	require.True(t, ok)
	t.Cleanup(func() { _ = concrete.Close() })
	return concrete, root
}

func TestNewProcessLSPClient_RejectsMissingCommand(t *testing.T) {
	_, err := NewProcessLSPClient(ProcessLSPConfig{LanguageID: "go"})
	require.Error(t, err)
	require.Contains(t, err.Error(), "command is required")
}

func TestNewProcessLSPClient_RejectsMissingLanguageID(t *testing.T) {
	_, err := NewProcessLSPClient(ProcessLSPConfig{Command: "fake-lsp"})
	require.Error(t, err)
	require.Contains(t, err.Error(), "language id is required")
}

func TestNewProcessLSPClient_CommandNotFound(t *testing.T) {
	_, err := NewProcessLSPClient(ProcessLSPConfig{
		Command:    "definitely-not-a-real-lsp-binary-9x7",
		LanguageID: "go",
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "not found")
}

func TestNewProcessLSPClientWithPermissions_PolicyDenial(t *testing.T) {
	denied := errors.New("policy denies lsp")
	_, err := NewProcessLSPClientWithPermissions(ProcessLSPConfig{
		Command:    "fake-lsp",
		LanguageID: "go",
	}, sandbox.CommandPolicyFunc(func(_ context.Context, _ ports.CommandRequest) error {
		return denied
	}))
	require.ErrorIs(t, err, denied)
}

func TestNewProcessLSPClientWithPermissions_PolicySeesCommandArgs(t *testing.T) {
	_, root := newFakeClient(t, "ok")
	testBin, err := os.Executable()
	require.NoError(t, err)
	binDir := t.TempDir()
	script := filepath.Join(binDir, "fake-lsp")
	require.NoError(t, os.WriteFile(script,
		[]byte(fmt.Sprintf("#!/bin/sh\nRELURPIFY_FAKE_LSP=1 exec '%s' -test.run=TestFakeLSPServerProcess -- ok '%s'\n", testBin, root)),
		0o755))

	var seen []string
	policyClient, err := NewProcessLSPClientWithPermissions(ProcessLSPConfig{
		Command:    script,
		LanguageID: "go",
		RootDir:    root,
	}, sandbox.CommandPolicyFunc(func(_ context.Context, req ports.CommandRequest) error {
		seen = append([]string(nil), req.Args...)
		return nil
	}))
	require.NoError(t, err)
	t.Cleanup(func() {
		if closer, ok := policyClient.(interface{ Close() error }); ok {
			_ = closer.Close()
		}
	})

	require.NotEmpty(t, seen)
	require.Equal(t, script, seen[0], "policy must observe the resolved command path")

	res, err := policyClient.GetHover(context.Background(), HoverRequest{
		File:     filepath.Join(root, "main.go"),
		Position: Position{Line: 0, Character: 0},
	})
	require.NoError(t, err)
	require.Equal(t, "fake hover docs", res.TypeInfo)
}

func TestNewProcessLSPClient_InitializeFailure(t *testing.T) {
	binDir := t.TempDir()
	script := filepath.Join(binDir, "fake-lsp-exit")
	require.NoError(t, os.WriteFile(script, []byte("#!/bin/sh\nexit 3\n"), 0o755))

	_, err := NewProcessLSPClient(ProcessLSPConfig{Command: script, LanguageID: "go"})
	require.Error(t, err, "a server that exits before initialize must fail the handshake")
}

func TestProcessClient_GetDefinition(t *testing.T) {
	client, root := newFakeClient(t, "ok")

	def, err := client.GetDefinition(context.Background(), DefinitionRequest{
		File:     filepath.Join(root, "main.go"),
		Symbol:   "Fake",
		Position: Position{Line: 0, Character: 5},
	})
	require.NoError(t, err)
	require.Contains(t, def.Location.URI, "decl.go")
	require.Equal(t, int64(0), def.Location.Range[0])
	require.Equal(t, int64(0), def.Location.Range[1])
	require.NotEmpty(t, def.Snippet, "snippet must be read from the definition target")
	require.Equal(t, def.Snippet, def.Signature)

	_, err = client.GetDefinition(context.Background(), DefinitionRequest{
		File: filepath.Join(root, "nodef.go"),
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "definition not found")

	// The server points at a URI that does not exist on disk; the snippet
	// read fails but the definition itself is still returned.
	def, err = client.GetDefinition(context.Background(), DefinitionRequest{
		File: filepath.Join(root, "missingtarget.go"),
	})
	require.NoError(t, err)
	require.Empty(t, def.Snippet)
}

func TestProcessClient_GetReferences(t *testing.T) {
	client, root := newFakeClient(t, "ok")
	refs, err := client.GetReferences(context.Background(), ReferencesRequest{
		File:     filepath.Join(root, "main.go"),
		Symbol:   "Fake",
		Position: Position{Line: 0, Character: 5},
	})
	require.NoError(t, err)
	require.Len(t, refs, 2)
	require.Equal(t, int64(1), refs[0].Range[0])
	require.Equal(t, int64(7), refs[1].Range[0])
}

func TestProcessClient_GetDocumentSymbols(t *testing.T) {
	client, root := newFakeClient(t, "ok")

	symbols, err := client.GetDocumentSymbols(context.Background(), filepath.Join(root, "docsym.go"))
	require.NoError(t, err)
	require.Len(t, symbols, 2, "nested document symbols must be flattened")
	require.Equal(t, "Parent", symbols[0].Name)
	require.Equal(t, "Child", symbols[1].Name)
	require.Contains(t, symbols[0].Location, "docsym.go")

	symbols, err = client.GetDocumentSymbols(context.Background(), filepath.Join(root, "rawinfo.go"))
	require.NoError(t, err)
	require.Len(t, symbols, 1)
	require.Equal(t, "InfoSym", symbols[0].Name)

	_, err = client.GetDocumentSymbols(context.Background(), filepath.Join(root, "garbage.go"))
	require.Error(t, err)
	require.Contains(t, err.Error(), "not understood")
}

func TestProcessClient_SearchSymbols(t *testing.T) {
	client, _ := newFakeClient(t, "ok")
	symbols, err := client.SearchSymbols(context.Background(), "Fake")
	require.NoError(t, err)
	require.Len(t, symbols, 1)
	require.Equal(t, "FakeSymbol", symbols[0].Name)
	require.Equal(t, "12", symbols[0].Kind)
	require.Contains(t, symbols[0].Location, "decl.go")
}

func TestProcessClient_Format(t *testing.T) {
	client, root := newFakeClient(t, "ok")

	out, err := client.Format(context.Background(), FormatRequest{File: filepath.Join(root, "fmtnone.go"), Code: "as-is"})
	require.NoError(t, err)
	require.Equal(t, "as-is", out, "no edits means code is returned unchanged")

	out, err = client.Format(context.Background(), FormatRequest{File: filepath.Join(root, "fmtfull.go"), Code: "raw"})
	require.NoError(t, err)
	require.Equal(t, "package formatted\n", out, "full-document replacement wins")

	out, err = client.Format(context.Background(), FormatRequest{File: filepath.Join(root, "fmtmany.go"), Code: "raw"})
	require.NoError(t, err)
	require.Equal(t, "EDIT2", out, "multi-edit responses take the last edit text")
}

func TestProcessClient_GetDiagnostics(t *testing.T) {
	client, root := newFakeClient(t, "ok")

	diags, err := client.GetDiagnostics(context.Background(), filepath.Join(root, "diag.go"))
	require.NoError(t, err)
	require.Len(t, diags, 2)
	require.Equal(t, "1", diags[0].Severity)
	require.Equal(t, "fake error", diags[0].Message)
	require.Equal(t, "fake-lsp", diags[0].Source)
	require.Equal(t, 0, diags[0].Line)
	require.Equal(t, "2", diags[1].Severity)
	require.Equal(t, 3, diags[1].Line)
}

func TestProcessClient_GetDiagnosticsTimesOut(t *testing.T) {
	client, root := newFakeClient(t, "ok")

	start := time.Now()
	_, err := client.GetDiagnostics(context.Background(), filepath.Join(root, "quiet.go"))
	require.Error(t, err)
	require.Contains(t, err.Error(), "diagnostics timeout")
	require.GreaterOrEqual(t, time.Since(start), 2*time.Second,
		"timeout must honor the diagnostics wait window")

	// The file is already open at this point, so a canceled context reaches
	// the wait loop and surfaces the context error.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = client.GetDiagnostics(ctx, filepath.Join(root, "quiet.go"))
	require.ErrorIs(t, err, context.Canceled)
}

func TestProcessClient_EnsureOpenMissingFile(t *testing.T) {
	client, _ := newFakeClient(t, "ok")
	_, err := client.GetDefinition(context.Background(), DefinitionRequest{
		File: filepath.Join(t.TempDir(), "does-not-exist.go"),
	})
	require.Error(t, err, "didOpen of a missing file must fail the call")
}

func TestProcessClient_ProcessMetadataAndLogs(t *testing.T) {
	client, _ := newFakeClient(t, "ok")

	meta := client.ProcessMetadata()
	assert.Equal(t, "fake-lsp", filepath.Base(meta.Command))
	require.NotZero(t, meta.PID)
	require.False(t, meta.Started.IsZero())

	select {
	case line, ok := <-client.Logs():
		require.True(t, ok, "log channel must be open")
		require.Contains(t, line, "fake-lsp stderr line")
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for server stderr log line")
	}
}

func TestProcessClient_CloseAndNilReceivers(t *testing.T) {
	var nilClient *processLSPClient
	require.NoError(t, nilClient.Close())
	require.Nil(t, nilClient.Logs())

	client, _ := newFakeClient(t, "ok")
	require.NoError(t, client.Close(), "close must be safe and return nil")
	require.NoError(t, client.Close())
}

func TestKnownServerConstructors_FailWhenBinaryMissing(t *testing.T) {
	t.Setenv("PATH", t.TempDir())

	root := t.TempDir()
	for name, ctor := range map[string]func(string) (LSPClient, error){
		"rust-analyzer":  NewRustAnalyzerClient,
		"gopls":          NewGoplsClient,
		"clangd":         NewClangdClient,
		"haskell-lsp":    NewHaskellClient,
		"typescript-lsp": NewTypeScriptClient,
		"lua-lsp":        NewLuaClient,
		"python-lsp":     NewPythonLSPClient,
	} {
		_, err := ctor(root)
		require.Error(t, err, "%s constructor must fail when the binary is missing", name)
		require.Contains(t, err.Error(), "not found", "%s: %v", name, err)
	}
}

func TestSafeConverters(t *testing.T) {
	require.Equal(t, int32(12345), safePID(12345))
	require.Equal(t, int32(0), safePID(math.MaxInt32+1))
	require.Equal(t, int32(0), safePID(math.MinInt32-1))

	require.Equal(t, uint32(7), safeUint32Line(7))
	require.Equal(t, uint32(0), safeUint32Line(-1))
	require.Equal(t, uint32(math.MaxUint32), safeUint32Line(math.MaxUint32), "MaxUint32 is the inclusive upper bound")

	require.Equal(t, uint32(9), safeUint32Char(9))
	require.Equal(t, uint32(0), safeUint32Char(-5))
	require.Equal(t, uint32(math.MaxUint32), safeUint32Char(math.MaxUint32), "MaxUint32 is the inclusive upper bound")
}

func TestPathToURI(t *testing.T) {
	require.Equal(t, "file:///", pathToURI("."))
	require.Equal(t, "file:///a/b.go", pathToURI("/a/b.go"))
	require.Equal(t, "file:///rel.go", pathToURI("rel.go"))
}

func TestURIToPath(t *testing.T) {
	require.Equal(t, "/a/b.go", uriToPath("file:///a/b.go"))
	require.Equal(t, "/a:b.go", uriToPath("file:///a%3Ab.go"))
	require.Equal(t, "plain/path", uriToPath("plain/path"))
}

func TestConvertDiagnostics(t *testing.T) {
	require.Empty(t, convertDiagnostics(nil))
}

func TestCollectDocumentSymbolsNested(t *testing.T) {
	var out []SymbolInformation
	collectDocumentSymbols(&out, "f.go", []protocol.DocumentSymbol{
		{Name: "A", Kind: 5, Children: []protocol.DocumentSymbol{{Name: "B", Kind: 6}}},
		{Name: "C", Kind: 12},
	})
	require.Len(t, out, 3)
	require.Equal(t, "A", out[0].Name)
	require.Equal(t, "B", out[1].Name)
	require.Equal(t, "C", out[2].Name)
}

func FuzzURIToPath(f *testing.F) {
	f.Add("file:///a/b.go")
	f.Add("file:///x%3Ay")
	f.Add("")
	f.Add("file://")
	f.Add("notauri")
	f.Add("file:///C:%3A/back%5Cslash")

	f.Fuzz(func(t *testing.T, uri string) {
		path := uriToPath(uri)
		// uriToPath must never panic; when it recognizes the scheme at all,
		// the prefix is stripped and %-escapes the server sent are decoded.
		if strings.HasPrefix(uri, "file://") {
			require.False(t, strings.HasPrefix(path, "file://"), "scheme must be stripped")
			require.NotContains(t, path, "%3A", "percent escapes must be decoded")
		}
	})
}
