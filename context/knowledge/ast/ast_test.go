package ast

import (
	"errors"
	"testing"
	"time"
)

func TestLanguageDetector(t *testing.T) {
	detector := NewLanguageDetector()
	if lang := detector.Detect("main.go"); lang != "go" {
		t.Fatalf("expected go, got %s", lang)
	}
	if lang := detector.Detect("README.md"); lang != "markdown" {
		t.Fatalf("expected markdown, got %s", lang)
	}
	if cat := detector.DetectCategory("yaml"); cat != CategoryConfig {
		t.Fatalf("expected config category, got %s", cat)
	}
	if cat := detector.DetectCategory("unknown-lang"); cat != CategoryDoc {
		t.Fatalf("expected doc category fallback, got %s", cat)
	}
}

type stubParser struct {
	language string
}

func (s *stubParser) Parse(content string, path string) (*ParseResult, error) {
	return &ParseResult{
		Nodes: []*Node{},
		Edges: []*Edge{},
		Metadata: &FileMetadata{
			ID:          GenerateFileID(path),
			Path:        path,
			Language:    s.language,
			Category:    CategoryDoc,
			ContentHash: HashContent(content),
			IndexedAt:   time.Now(),
		},
	}, nil
}

func (s *stubParser) ParseIncremental(_ *ParseResult, _ []ContentChange) (*ParseResult, error) {
	return nil, errors.New("stub: not implemented")
}

func (s *stubParser) Language() string          { return s.language }
func (s *stubParser) Category() Category        { return CategoryDoc }
func (s *stubParser) SupportsIncremental() bool { return false }

func TestParserRegistry(t *testing.T) {
	registry := NewParserRegistry()
	parser := &stubParser{language: "custom"}
	registry.Register(parser)
	if _, ok := registry.GetParser("custom"); !ok {
		t.Fatal("expected parser to be registered")
	}
	supported := registry.SupportedLanguages()
	if len(supported) != 1 || supported[0] != "custom" {
		t.Fatalf("unexpected supported languages: %v", supported)
	}
}

func TestGoParserParse(t *testing.T) {
	source := `package sample
import "fmt"
func Hello(name string) string {
	return fmt.Sprintf("hi %s", name)
}`
	parser := NewGoParser()
	result, err := parser.Parse(source, "sample.go")
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	if result.Metadata == nil || result.Metadata.Language != "go" {
		t.Fatalf("metadata not populated: %#v", result.Metadata)
	}
	if len(result.Nodes) < 3 {
		t.Fatalf("expected several nodes, got %d", len(result.Nodes))
	}
	if result.RootNode == nil || result.RootNode.Type != NodeTypePackage {
		t.Fatalf("root node incorrect: %#v", result.RootNode)
	}
	if len(result.Edges) == 0 {
		t.Fatalf("expected import edges, got %d", len(result.Edges))
	}
}

func TestMarkdownParserParse(t *testing.T) {
	content := "# Title\n\nSome text.\n\n## Section\n\n```go\nfmt.Println(\"hi\")\n```\n"
	parser := NewMarkdownParser()
	result, err := parser.Parse(content, "doc.md")
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	if result.RootNode == nil || result.RootNode.Type != NodeTypeDocument {
		t.Fatalf("expected document root, got %#v", result.RootNode)
	}
	if len(result.Nodes) < 3 {
		t.Fatalf("expected heading and code nodes, got %d", len(result.Nodes))
	}
}
