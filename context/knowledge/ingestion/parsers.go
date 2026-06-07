package ingestion

import (
	"bufio"
	"context"
	"fmt"
	"path/filepath"
	"strings"
)

// selectParser returns an appropriate parser for the MIME type or file path.
func selectParser(mimeType string, filePath string) ContentParser {
	// Try path-based selection first
	if filePath != "" {
		ext := strings.ToLower(filepath.Ext(filePath))
		switch ext {
		case ".go":
			return &GoParser{}
		case ".py":
			return &PythonParser{}
		case ".rs":
			return &RustParser{}
		case ".js", ".ts", ".tsx":
			return &JSParser{}
		case ".md":
			return &MarkdownParser{}
		case ".txt":
			return &TextParser{}
		case ".json":
			return &JSONParser{}
		case ".yaml", ".yml":
			return &YAMLParser{}
		}
	}

	// Fall back to MIME type
	switch mimeType {
	case "text/x-go":
		return &GoParser{}
	case "text/x-python":
		return &PythonParser{}
	case "text/x-rust":
		return &RustParser{}
	case "text/x-javascript", "text/x-typescript":
		return &JSParser{}
	case "text/markdown":
		return &MarkdownParser{}
	case "text/plain":
		return &TextParser{}
	case "application/json":
		return &JSONParser{}
	case "application/yaml":
		return &YAMLParser{}
	}

	// Default to text parser
	return &TextParser{}
}

// GoParser parses Go source files.
type GoParser struct{}

func (p *GoParser) CanParse(mimeType string, filePath string) bool {
	return mimeType == "text/x-go" || filepath.Ext(filePath) == ".go"
}

func (p *GoParser) Parse(ctx context.Context, raw RawIngestion) (*TypedIngestion, error) {
	content := string(raw.Content)
	lines := strings.Split(content, "\n")

	var boundaries []ChunkBoundary
	var currentChunk int

	// Simple line-based chunking for functions and types
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)

		// Detect function or type declarations
		if strings.HasPrefix(trimmed, "func ") || strings.HasPrefix(trimmed, "type ") {
			if currentChunk < i {
				// Save previous chunk if there's content
				if currentChunk < i-1 {
					// Find a good name for the chunk
					name := extractGoChunkName(lines[currentChunk])
					boundaries = append(boundaries, ChunkBoundary{
						Start: currentChunk,
						End:   i - 1,
						Type:  detectGoChunkType(name),
						Name:  name,
					})
				}
				currentChunk = i
			}
		}

		// Detect package declaration (file-level chunk)
		if strings.HasPrefix(trimmed, "package ") && len(boundaries) == 0 {
			currentChunk = i
		}
	}

	// Add final chunk
	if currentChunk < len(lines) {
		name := extractGoChunkName(lines[currentChunk])
		boundaries = append(boundaries, ChunkBoundary{
			Start: currentChunk,
			End:   len(lines),
			Type:  detectGoChunkType(name),
			Name:  name,
		})
	}

	return &TypedIngestion{
		Content:           raw.Content,
		ContentType:       "go",
		ChunkBoundaries:   boundaries,
		Metadata:          map[string]any{"lines": len(lines)},
		SourcePrincipal:   raw.SourcePrincipal,
		AcquisitionMethod: raw.AcquisitionMethod,
		AcquiredAt:        raw.AcquiredAt,
	}, nil
}

func extractGoChunkName(line string) string {
	trimmed := strings.TrimSpace(line)
	parts := strings.Fields(trimmed)
	if len(parts) >= 2 {
		// func Name(...) or type Name ...
		return parts[1]
	}
	return "unnamed"
}

func detectGoChunkType(name string) string {
	if strings.Contains(name, "(") {
		return "function"
	}
	return "type"
}

// PythonParser parses Python source files.
type PythonParser struct{}

func (p *PythonParser) CanParse(mimeType string, filePath string) bool {
	return mimeType == "text/x-python" || filepath.Ext(filePath) == ".py"
}

func (p *PythonParser) Parse(ctx context.Context, raw RawIngestion) (*TypedIngestion, error) {
	content := string(raw.Content)
	lines := strings.Split(content, "\n")

	var boundaries []ChunkBoundary
	var currentChunk int

	for i, line := range lines {
		trimmed := strings.TrimSpace(line)

		// Detect function or class definitions
		if strings.HasPrefix(trimmed, "def ") || strings.HasPrefix(trimmed, "class ") {
			if currentChunk < i && i > 0 {
				name := extractPythonChunkName(lines[currentChunk])
				boundaries = append(boundaries, ChunkBoundary{
					Start: currentChunk,
					End:   i,
					Type:  detectPythonChunkType(lines[currentChunk]),
					Name:  name,
				})
			}
			currentChunk = i
		}
	}

	// Add final chunk
	if currentChunk < len(lines) {
		name := extractPythonChunkName(lines[currentChunk])
		boundaries = append(boundaries, ChunkBoundary{
			Start: currentChunk,
			End:   len(lines),
			Type:  detectPythonChunkType(lines[currentChunk]),
			Name:  name,
		})
	}

	return &TypedIngestion{
		Content:           raw.Content,
		ContentType:       "python",
		ChunkBoundaries:   boundaries,
		Metadata:          map[string]any{"lines": len(lines)},
		SourcePrincipal:   raw.SourcePrincipal,
		AcquisitionMethod: raw.AcquisitionMethod,
		AcquiredAt:        raw.AcquiredAt,
	}, nil
}

func extractPythonChunkName(line string) string {
	trimmed := strings.TrimSpace(line)
	parts := strings.FieldsFunc(trimmed, func(r rune) bool {
		return r == ' ' || r == '(' || r == ':'
	})
	if len(parts) >= 2 {
		return parts[1]
	}
	return "unnamed"
}

func detectPythonChunkType(line string) string {
	if strings.HasPrefix(strings.TrimSpace(line), "def ") {
		return "function"
	}
	if strings.HasPrefix(strings.TrimSpace(line), "class ") {
		return "class"
	}
	return "module"
}

// RustParser parses Rust source files.
type RustParser struct{}

func (p *RustParser) CanParse(mimeType string, filePath string) bool {
	return mimeType == "text/x-rust" || filepath.Ext(filePath) == ".rs"
}

func (p *RustParser) Parse(ctx context.Context, raw RawIngestion) (*TypedIngestion, error) {
	// Simplified Rust parsing - similar to Go
	return &TypedIngestion{
		Content:           raw.Content,
		ContentType:       "rust",
		ChunkBoundaries:   []ChunkBoundary{{Start: 0, End: len(strings.Split(string(raw.Content), "\n")), Type: "file", Name: "file"}},
		Metadata:          map[string]any{},
		SourcePrincipal:   raw.SourcePrincipal,
		AcquisitionMethod: raw.AcquisitionMethod,
		AcquiredAt:        raw.AcquiredAt,
	}, nil
}

// JSParser parses JavaScript/TypeScript files.
type JSParser struct{}

func (p *JSParser) CanParse(mimeType string, filePath string) bool {
	ext := filepath.Ext(filePath)
	return mimeType == "text/x-javascript" || mimeType == "text/x-typescript" ||
		ext == ".js" || ext == ".ts" || ext == ".tsx"
}

func (p *JSParser) Parse(ctx context.Context, raw RawIngestion) (*TypedIngestion, error) {
	return &TypedIngestion{
		Content:           raw.Content,
		ContentType:       "javascript",
		ChunkBoundaries:   []ChunkBoundary{{Start: 0, End: len(strings.Split(string(raw.Content), "\n")), Type: "file", Name: "file"}},
		Metadata:          map[string]any{},
		SourcePrincipal:   raw.SourcePrincipal,
		AcquisitionMethod: raw.AcquisitionMethod,
		AcquiredAt:        raw.AcquiredAt,
	}, nil
}

// MarkdownParser parses Markdown files.
type MarkdownParser struct{}

func (p *MarkdownParser) CanParse(mimeType string, filePath string) bool {
	return mimeType == "text/markdown" || filepath.Ext(filePath) == ".md"
}

func (p *MarkdownParser) Parse(ctx context.Context, raw RawIngestion) (*TypedIngestion, error) {
	content := string(raw.Content)
	lines := strings.Split(content, "\n")

	var boundaries []ChunkBoundary
	var currentChunk int
	var currentLevel int

	for i, line := range lines {
		trimmed := strings.TrimSpace(line)

		// Detect headings
		if strings.HasPrefix(trimmed, "#") {
			level := 0
			for _, ch := range trimmed {
				if ch == '#' {
					level++
				} else {
					break
				}
			}

			// Close previous chunk
			if currentChunk < i {
				title := extractMarkdownHeading(lines[currentChunk])
				boundaries = append(boundaries, ChunkBoundary{
					Start: currentChunk,
					End:   i,
					Type:  fmt.Sprintf("heading_%d", currentLevel),
					Name:  title,
				})
			}

			currentChunk = i
			currentLevel = level
		}
	}

	// Add final chunk
	if currentChunk < len(lines) {
		title := ""
		if len(boundaries) > 0 {
			title = extractMarkdownHeading(lines[currentChunk])
		} else {
			title = "document"
		}
		boundaries = append(boundaries, ChunkBoundary{
			Start: currentChunk,
			End:   len(lines),
			Type:  fmt.Sprintf("heading_%d", currentLevel),
			Name:  title,
		})
	}

	// If no headings found, treat as single chunk
	if len(boundaries) == 0 {
		boundaries = append(boundaries, ChunkBoundary{
			Start: 0,
			End:   len(lines),
			Type:  "document",
			Name:  "document",
		})
	}

	return &TypedIngestion{
		Content:           raw.Content,
		ContentType:       "markdown",
		ChunkBoundaries:   boundaries,
		Metadata:          map[string]any{"lines": len(lines)},
		SourcePrincipal:   raw.SourcePrincipal,
		AcquisitionMethod: raw.AcquisitionMethod,
		AcquiredAt:        raw.AcquiredAt,
	}, nil
}

func extractMarkdownHeading(line string) string {
	trimmed := strings.TrimSpace(line)
	// Remove leading #s
	for strings.HasPrefix(trimmed, "#") {
		trimmed = strings.TrimSpace(trimmed[1:])
	}
	return trimmed
}

// TextParser parses plain text files.
type TextParser struct {
	WindowSize int
	Overlap    int
}

func (p *TextParser) CanParse(mimeType string, filePath string) bool {
	return mimeType == "text/plain" || filepath.Ext(filePath) == ".txt"
}

func (p *TextParser) Parse(ctx context.Context, raw RawIngestion) (*TypedIngestion, error) {
	if p.WindowSize == 0 {
		p.WindowSize = 50 // lines per chunk
		p.Overlap = 5     // lines of overlap
	}

	content := string(raw.Content)
	lines := strings.Split(content, "\n")

	var boundaries []ChunkBoundary
	for i := 0; i < len(lines); i += p.WindowSize - p.Overlap {
		end := i + p.WindowSize
		if end > len(lines) {
			end = len(lines)
		}

		boundaries = append(boundaries, ChunkBoundary{
			Start: i,
			End:   end,
			Type:  "window",
			Name:  fmt.Sprintf("window_%d", len(boundaries)),
		})

		if end == len(lines) {
			break
		}
	}

	if len(boundaries) == 0 {
		boundaries = append(boundaries, ChunkBoundary{
			Start: 0,
			End:   len(lines),
			Type:  "document",
			Name:  "document",
		})
	}

	return &TypedIngestion{
		Content:           raw.Content,
		ContentType:       "text",
		ChunkBoundaries:   boundaries,
		Metadata:          map[string]any{"lines": len(lines), "window_size": p.WindowSize, "overlap": p.Overlap},
		SourcePrincipal:   raw.SourcePrincipal,
		AcquisitionMethod: raw.AcquisitionMethod,
		AcquiredAt:        raw.AcquiredAt,
	}, nil
}

// JSONParser parses JSON files.
type JSONParser struct{}

func (p *JSONParser) CanParse(mimeType string, filePath string) bool {
	return mimeType == "application/json" || filepath.Ext(filePath) == ".json"
}

func (p *JSONParser) Parse(ctx context.Context, raw RawIngestion) (*TypedIngestion, error) {
	return &TypedIngestion{
		Content:           raw.Content,
		ContentType:       "json",
		ChunkBoundaries:   []ChunkBoundary{{Start: 0, End: 1, Type: "file", Name: "file"}},
		Metadata:          map[string]any{},
		SourcePrincipal:   raw.SourcePrincipal,
		AcquisitionMethod: raw.AcquisitionMethod,
		AcquiredAt:        raw.AcquiredAt,
	}, nil
}

// YAMLParser parses YAML files.
type YAMLParser struct{}

func (p *YAMLParser) CanParse(mimeType string, filePath string) bool {
	ext := filepath.Ext(filePath)
	return mimeType == "application/yaml" || ext == ".yaml" || ext == ".yml"
}

func (p *YAMLParser) Parse(ctx context.Context, raw RawIngestion) (*TypedIngestion, error) {
	return &TypedIngestion{
		Content:           raw.Content,
		ContentType:       "yaml",
		ChunkBoundaries:   []ChunkBoundary{{Start: 0, End: 1, Type: "file", Name: "file"}},
		Metadata:          map[string]any{},
		SourcePrincipal:   raw.SourcePrincipal,
		AcquisitionMethod: raw.AcquisitionMethod,
		AcquiredAt:        raw.AcquiredAt,
	}, nil
}

// BufioScannerLineIterator provides a buffered line-by-line scanner
// that can be used for large files without loading them entirely into memory.
type BufioScannerLineIterator struct {
	scanner *bufio.Scanner
	lineNum int
}

func NewLineIterator(content []byte) *BufioScannerLineIterator {
	return &BufioScannerLineIterator{
		scanner: bufio.NewScanner(strings.NewReader(string(content))),
		lineNum: 0,
	}
}

func (it *BufioScannerLineIterator) Next() (string, bool) {
	if it.scanner.Scan() {
		it.lineNum++
		return it.scanner.Text(), true
	}
	return "", false
}

func (it *BufioScannerLineIterator) LineNum() int {
	return it.lineNum
}

func (it *BufioScannerLineIterator) Err() error {
	return it.scanner.Err()
}
