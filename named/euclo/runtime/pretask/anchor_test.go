package pretask

import (
	"testing"

	"github.com/lexcodex/relurpify/framework/ast"
)

// stubIndexQuerier returns a pre-configured set of matching symbol names.
type stubIndexQuerier struct {
	symbols map[string][]*ast.Node
}

func (s *stubIndexQuerier) QuerySymbol(pattern string) ([]*ast.Node, error) {
	if nodes, ok := s.symbols[pattern]; ok {
		return nodes, nil
	}
	return nil, nil
}

func (s *stubIndexQuerier) SearchNodes(query ast.NodeQuery) ([]*ast.Node, error) {
	return nil, nil
}

func TestAnchorExtract_CurrentTurnFilesIncluded(t *testing.T) {
	extractor := &AnchorExtractor{
		index: &dummyIndexQuerier{},
		config: AnchorConfig{
			MinSymbolLength: 3,
			MaxSymbols:      12,
		},
	}
	input := AnchorExtractInput{
		CurrentTurnFiles: []string{"a.go"},
	}
	anchors := extractor.Extract(input)
	if len(anchors.FilePaths) != 1 || anchors.FilePaths[0] != "a.go" {
		t.Errorf("Expected a.go in FilePaths, got %v", anchors.FilePaths)
	}
}

func TestAnchorExtract_SessionPinsIncluded(t *testing.T) {
	extractor := &AnchorExtractor{
		index: &dummyIndexQuerier{},
		config: AnchorConfig{
			MinSymbolLength: 3,
			MaxSymbols:      12,
		},
	}
	input := AnchorExtractInput{
		SessionPins: []string{"pinned.go"},
	}
	anchors := extractor.Extract(input)
	if len(anchors.FilePaths) != 1 || anchors.FilePaths[0] != "pinned.go" {
		t.Errorf("Expected pinned.go in FilePaths, got %v", anchors.FilePaths)
	}
}

func TestAnchorExtract_AtMentionExtraction(t *testing.T) {
	extractor := &AnchorExtractor{
		index: &dummyIndexQuerier{},
		config: AnchorConfig{
			MinSymbolLength: 3,
			MaxSymbols:      12,
		},
	}
	input := AnchorExtractInput{
		Query: "look at @cmd/main.go for details",
	}
	anchors := extractor.Extract(input)
	if len(anchors.FilePaths) != 1 || anchors.FilePaths[0] != "cmd/main.go" {
		t.Errorf("Expected cmd/main.go in FilePaths, got %v", anchors.FilePaths)
	}
}

func TestAnchorExtract_CamelCaseConfirmedByIndex(t *testing.T) {
	stub := &stubIndexQuerier{
		symbols: map[string][]*ast.Node{
			"MyHandler": {{}},
		},
	}
	extractor := &AnchorExtractor{
		index: stub,
		config: AnchorConfig{
			MinSymbolLength: 3,
			MaxSymbols:      12,
		},
	}
	input := AnchorExtractInput{
		Query: "fix MyHandler bug",
	}
	anchors := extractor.Extract(input)
	if len(anchors.SymbolNames) != 1 || anchors.SymbolNames[0] != "MyHandler" {
		t.Errorf("Expected MyHandler in SymbolNames, got %v", anchors.SymbolNames)
	}
}

func TestAnchorExtract_CamelCaseFilteredByIndex(t *testing.T) {
	stub := &stubIndexQuerier{
		symbols: map[string][]*ast.Node{}, // returns nil for every symbol
	}
	extractor := &AnchorExtractor{
		index: stub,
		config: AnchorConfig{
			MinSymbolLength: 3,
			MaxSymbols:      12,
		},
	}
	input := AnchorExtractInput{
		Query: "fix MyHandler bug",
	}
	anchors := extractor.Extract(input)
	if len(anchors.SymbolNames) != 0 {
		t.Errorf("Expected empty SymbolNames, got %v", anchors.SymbolNames)
	}
}

func TestAnchorExtract_EmptyInput(t *testing.T) {
	extractor := &AnchorExtractor{
		index: &dummyIndexQuerier{},
		config: AnchorConfig{
			MinSymbolLength: 3,
			MaxSymbols:      12,
		},
	}
	input := AnchorExtractInput{}
	anchors := extractor.Extract(input)
	if len(anchors.FilePaths) != 0 || len(anchors.SymbolNames) != 0 || len(anchors.PackageRefs) != 0 {
		t.Errorf("Expected empty AnchorSet, got %+v", anchors)
	}
}
