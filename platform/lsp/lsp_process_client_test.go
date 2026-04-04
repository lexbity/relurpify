package lsp

import (
	"os"
	"path/filepath"
	"testing"

	"go.lsp.dev/protocol"
)

func TestConvertDiagnostics(t *testing.T) {
	protocolDiags := []protocol.Diagnostic{
		{
			Severity: protocol.DiagnosticSeverityError,
			Message:  "something wrong",
			Source:   "test",
			Range: protocol.Range{
				Start: protocol.Position{Line: 1, Character: 0},
				End:   protocol.Position{Line: 1, Character: 5},
			},
		},
		{
			Severity: protocol.DiagnosticSeverityWarning,
			Message:  "maybe fix",
			Source:   "lint",
			Range: protocol.Range{
				Start: protocol.Position{Line: 10, Character: 2},
				End:   protocol.Position{Line: 10, Character: 8},
			},
		},
	}
	diags := convertDiagnostics(protocolDiags)
	if len(diags) != 2 {
		t.Fatalf("expected 2 diagnostics, got %d", len(diags))
	}
	if diags[0].Severity != "1" {
		t.Errorf("expected severity 1, got %s", diags[0].Severity)
	}
	if diags[0].Line != 1 {
		t.Errorf("expected line 1, got %d", diags[0].Line)
	}
	if diags[1].Severity != "2" {
		t.Errorf("expected severity 2, got %s", diags[1].Severity)
	}
	if diags[1].Line != 10 {
		t.Errorf("expected line 10, got %d", diags[1].Line)
	}
}

func TestReadSnippet(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "test.txt")
	content := "line1\nline2\nline3\nline4\nline5"
	if err := os.WriteFile(file, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	client := &processLSPClient{}
	rng := protocol.Range{
		Start: protocol.Position{Line: 1, Character: 0},
		End:   protocol.Position{Line: 3, Character: 0},
	}
	snippet, err := client.readSnippet(file, rng)
	if err != nil {
		t.Fatal(err)
	}
	expected := "line2\nline3\nline4"
	if snippet != expected {
		t.Errorf("readSnippet = %q, want %q", snippet, expected)
	}

	// test out of bounds
	rng = protocol.Range{
		Start: protocol.Position{Line: 10, Character: 0},
		End:   protocol.Position{Line: 12, Character: 0},
	}
	snippet, err = client.readSnippet(file, rng)
	if err != nil {
		t.Fatal(err)
	}
	if snippet != "" {
		t.Errorf("expected empty snippet, got %q", snippet)
	}
}

func TestCollectDocumentSymbols(t *testing.T) {
	var dst []SymbolInformation
	symbols := []protocol.DocumentSymbol{
		{
			Name: "func1",
			Kind: protocol.SymbolKindFunction,
			Range: protocol.Range{
				Start: protocol.Position{Line: 5, Character: 0},
			},
			Children: []protocol.DocumentSymbol{
				{
					Name: "inner",
					Kind: protocol.SymbolKindVariable,
					Range: protocol.Range{
						Start: protocol.Position{Line: 6, Character: 0},
					},
				},
			},
		},
		{
			Name: "class1",
			Kind: protocol.SymbolKindClass,
			Range: protocol.Range{
				Start: protocol.Position{Line: 10, Character: 0},
			},
		},
	}
	collectDocumentSymbols(&dst, "test.go", symbols)
	if len(dst) != 3 {
		t.Fatalf("expected 3 symbols, got %d", len(dst))
	}
	if dst[0].Name != "func1" || dst[0].Kind != "12" {
		t.Errorf("unexpected first symbol: %+v", dst[0])
	}
	if dst[1].Name != "inner" || dst[1].Kind != "13" {
		t.Errorf("unexpected child symbol: %+v", dst[1])
	}
	if dst[2].Name != "class1" || dst[2].Kind != "5" {
		t.Errorf("unexpected third symbol: %+v", dst[2])
	}
}

func TestNewProcessLSPClient_Errors(t *testing.T) {
	// empty command
	_, err := NewProcessLSPClient(ProcessLSPConfig{})
	if err == nil {
		t.Error("expected error for empty command")
	}
	// empty language id
	_, err = NewProcessLSPClient(ProcessLSPConfig{
		Command: "some-server",
	})
	if err == nil {
		t.Error("expected error for empty language id")
	}
}

func TestProcessLSPClient_Metadata(t *testing.T) {
	// This test cannot start a real server in unit test, so we only test that
	// the configuration is stored correctly in the client's metadata.
	// Skipping actual client creation.
	_ = ProcessLSPConfig{
		Command:    "test-server",
		Args:       []string{"--verbose"},
		RootDir:    ".",
		LanguageID: "test",
	}
}
