package ingestion

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"codeburg.org/lexbit/relurpify/governance/identity"
)

var parserTestPrincipal = identity.SubjectRef{ID: "parser-tester"}

func rawForParser(content []byte) RawIngestion {
	return RawIngestion{
		Content:           content,
		SourcePrincipal:   parserTestPrincipal,
		AcquisitionMethod: "file_read",
		AcquiredAt:        time.Now().UTC(),
	}
}

func TestSelectParserPrefersFilePathExtension(t *testing.T) {
	cases := []struct {
		path string
		want string
	}{
		{"main.go", "*ingestion.GoParser"},
		{"MAIN.GO", "*ingestion.GoParser"},
		{"script.py", "*ingestion.PythonParser"},
		{"lib.rs", "*ingestion.RustParser"},
		{"app.js", "*ingestion.JSParser"},
		{"app.ts", "*ingestion.JSParser"},
		{"comp.tsx", "*ingestion.JSParser"},
		{"README.md", "*ingestion.MarkdownParser"},
		{"notes.txt", "*ingestion.TextParser"},
		{"data.json", "*ingestion.JSONParser"},
		{"conf.yaml", "*ingestion.YAMLParser"},
		{"conf.yml", "*ingestion.YAMLParser"},
	}
	for _, tc := range cases {
		parser := selectParser("", tc.path)
		require.Equal(t, tc.want, fmt.Sprintf("%T", parser), "path %q", tc.path)
	}
}

func TestSelectParserFallsBackToMimeType(t *testing.T) {
	cases := []struct {
		mimeType string
		want     string
	}{
		{"text/x-go", "*ingestion.GoParser"},
		{"text/x-python", "*ingestion.PythonParser"},
		{"text/x-rust", "*ingestion.RustParser"},
		{"text/x-javascript", "*ingestion.JSParser"},
		{"text/x-typescript", "*ingestion.JSParser"},
		{"text/markdown", "*ingestion.MarkdownParser"},
		{"text/plain", "*ingestion.TextParser"},
		{"application/json", "*ingestion.JSONParser"},
		{"application/yaml", "*ingestion.YAMLParser"},
	}
	for _, tc := range cases {
		parser := selectParser(tc.mimeType, "")
		require.Equal(t, tc.want, fmt.Sprintf("%T", parser), "mime %q", tc.mimeType)
	}
}

func TestSelectParserDefaultsToTextParser(t *testing.T) {
	for _, mimeType := range []string{"application/octet-stream", "image/png", ""} {
		parser := selectParser(mimeType, "archive.bin")
		require.Equal(t, "*ingestion.TextParser", fmt.Sprintf("%T", parser), "mime %q", mimeType)
	}
}

func TestGoParserCanParse(t *testing.T) {
	parser := &GoParser{}
	require.True(t, parser.CanParse("text/x-go", ""))
	require.True(t, parser.CanParse("", "main.go"))
	require.False(t, parser.CanParse("text/plain", "main.md"))
}

func TestGoParserChunksDeclarations(t *testing.T) {
	source := "package main\n\nimport \"fmt\"\n\nfunc Alpha() int {\n\treturn 1\n}\n\ntype Thing struct {\n\tX int\n}\n\nfunc Beta() {}\n"
	typed, err := (&GoParser{}).Parse(context.Background(), rawForParser([]byte(source)))
	require.NoError(t, err)
	require.Equal(t, "go", typed.ContentType)
	require.Equal(t, []byte(source), typed.Content)
	require.Equal(t, parserTestPrincipal, typed.SourcePrincipal)
	require.Equal(t, "file_read", typed.AcquisitionMethod)
	require.Equal(t, 14, typed.Metadata["lines"])
	require.Equal(t, []ChunkBoundary{
		{Start: 0, End: 3, Type: "type", Name: "main"},
		{Start: 4, End: 7, Type: "function", Name: "Alpha()"},
		{Start: 8, End: 11, Type: "type", Name: "Thing"},
		{Start: 12, End: 14, Type: "function", Name: "Beta()"},
	}, typed.ChunkBoundaries)
}

func TestGoParserSingleChunkForContentWithoutDeclarations(t *testing.T) {
	typed, err := (&GoParser{}).Parse(context.Background(), rawForParser([]byte("just prose here\n")))
	require.NoError(t, err)
	require.Equal(t, "go", typed.ContentType)
	require.Equal(t, []ChunkBoundary{{Start: 0, End: 2, Type: "type", Name: "prose"}}, typed.ChunkBoundaries)
}

func TestGoParserHelpersFallbacks(t *testing.T) {
	require.Equal(t, "unnamed", extractGoChunkName("func"))
	require.Equal(t, "unnamed", extractGoChunkName(""))
	require.Equal(t, "unnamed", extractGoChunkName("   "))
	require.Equal(t, "function", detectGoChunkType("Alpha()"))
	require.Equal(t, "type", detectGoChunkType("Thing"))
}

func TestPythonParserCanParse(t *testing.T) {
	parser := &PythonParser{}
	require.True(t, parser.CanParse("text/x-python", ""))
	require.True(t, parser.CanParse("", "app.py"))
	require.False(t, parser.CanParse("text/plain", "app.js"))
}

func TestPythonParserChunksDefinitions(t *testing.T) {
	source := "import os\n\ndef one():\n    pass\n\nclass Two:\n    pass\n\ndef three():\n    pass\n"
	typed, err := (&PythonParser{}).Parse(context.Background(), rawForParser([]byte(source)))
	require.NoError(t, err)
	require.Equal(t, "python", typed.ContentType)
	require.Equal(t, 11, typed.Metadata["lines"])
	require.Equal(t, []ChunkBoundary{
		{Start: 0, End: 2, Type: "module", Name: "os"},
		{Start: 2, End: 5, Type: "function", Name: "one"},
		{Start: 5, End: 8, Type: "class", Name: "Two"},
		{Start: 8, End: 11, Type: "function", Name: "three"},
	}, typed.ChunkBoundaries)
}

func TestPythonParserHelpersFallbacks(t *testing.T) {
	require.Equal(t, "unnamed", extractPythonChunkName("def"))
	require.Equal(t, "unnamed", extractPythonChunkName(""))
	require.Equal(t, "function", detectPythonChunkType("def one():"))
	require.Equal(t, "class", detectPythonChunkType("class Two:"))
	require.Equal(t, "module", detectPythonChunkType("import os"))
}

func TestRustParserTreatsFileAsSingleChunk(t *testing.T) {
	source := "fn main() {}\nfn other() {}\n"
	parser := &RustParser{}
	require.True(t, parser.CanParse("text/x-rust", ""))
	require.True(t, parser.CanParse("", "lib.rs"))
	require.False(t, parser.CanParse("text/plain", "lib.go"))

	typed, err := parser.Parse(context.Background(), rawForParser([]byte(source)))
	require.NoError(t, err)
	require.Equal(t, "rust", typed.ContentType)
	require.Empty(t, typed.Metadata)
	require.Equal(t, []ChunkBoundary{{Start: 0, End: 3, Type: "file", Name: "file"}}, typed.ChunkBoundaries)
}

func TestJSParserCanParse(t *testing.T) {
	parser := &JSParser{}
	require.True(t, parser.CanParse("text/x-javascript", ""))
	require.True(t, parser.CanParse("text/x-typescript", ""))
	require.True(t, parser.CanParse("", "app.js"))
	require.True(t, parser.CanParse("", "app.ts"))
	require.True(t, parser.CanParse("", "app.tsx"))
	require.True(t, parser.CanParse("text/plain", "app.js"), "a matching extension accepts the file regardless of mime hint")
	require.False(t, parser.CanParse("", "app.jsx"))
	require.False(t, parser.CanParse("text/plain", "app.jsx"))
}

func TestJSParserTreatsFileAsSingleChunk(t *testing.T) {
	typed, err := (&JSParser{}).Parse(context.Background(), rawForParser([]byte("function hi() {}\n")))
	require.NoError(t, err)
	require.Equal(t, "javascript", typed.ContentType)
	require.Empty(t, typed.Metadata)
	require.Equal(t, []ChunkBoundary{{Start: 0, End: 2, Type: "file", Name: "file"}}, typed.ChunkBoundaries)
}

func TestMarkdownParserChunksHeadings(t *testing.T) {
	source := "# Title\nintro text\n## Sub\nmore\n### Deep\nend\n"
	typed, err := (&MarkdownParser{}).Parse(context.Background(), rawForParser([]byte(source)))
	require.NoError(t, err)
	require.Equal(t, "markdown", typed.ContentType)
	require.Equal(t, 7, typed.Metadata["lines"])
	require.Equal(t, []ChunkBoundary{
		{Start: 0, End: 2, Type: "heading_1", Name: "Title"},
		{Start: 2, End: 4, Type: "heading_2", Name: "Sub"},
		{Start: 4, End: 7, Type: "heading_3", Name: "Deep"},
	}, typed.ChunkBoundaries)
}

func TestMarkdownParserSingleChunkWithoutHeadings(t *testing.T) {
	typed, err := (&MarkdownParser{}).Parse(context.Background(), rawForParser([]byte("plain text\nsecond\n")))
	require.NoError(t, err)
	require.Equal(t, "markdown", typed.ContentType)
	require.Equal(t, []ChunkBoundary{{Start: 0, End: 3, Type: "heading_0", Name: "document"}}, typed.ChunkBoundaries)
}

func TestMarkdownParserCanParse(t *testing.T) {
	parser := &MarkdownParser{}
	require.True(t, parser.CanParse("text/markdown", ""))
	require.True(t, parser.CanParse("", "doc.md"))
	require.False(t, parser.CanParse("text/plain", "doc.txt"))
}

func TestExtractMarkdownHeadingStripsLeadingHashes(t *testing.T) {
	require.Equal(t, "Deep", extractMarkdownHeading("###   Deep   "))
	require.Equal(t, "A", extractMarkdownHeading("#A"))
	require.Equal(t, "", extractMarkdownHeading("####"))
	require.Equal(t, "no heading", extractMarkdownHeading("no heading"))
}

func TestTextParserWindowsLongContent(t *testing.T) {
	content := strings.Repeat("row\n", 120) // 121 lines including the trailing empty one
	typed, err := (&TextParser{}).Parse(context.Background(), rawForParser([]byte(content)))
	require.NoError(t, err)
	require.Equal(t, "text", typed.ContentType)
	require.Equal(t, 121, typed.Metadata["lines"])
	require.Equal(t, 50, typed.Metadata["window_size"])
	require.Equal(t, 5, typed.Metadata["overlap"])
	require.Equal(t, []ChunkBoundary{
		{Start: 0, End: 50, Type: "window", Name: "window_0"},
		{Start: 45, End: 95, Type: "window", Name: "window_1"},
		{Start: 90, End: 121, Type: "window", Name: "window_2"},
	}, typed.ChunkBoundaries)
	require.Less(t, typed.ChunkBoundaries[1].Start, typed.ChunkBoundaries[0].End, "windows overlap")
}

func TestTextParserHonorsCustomWindowAndOverlap(t *testing.T) {
	parser := &TextParser{WindowSize: 3, Overlap: 1}
	typed, err := parser.Parse(context.Background(), rawForParser([]byte("a\nb\nc\nd\ne\n")))
	require.NoError(t, err)
	require.Equal(t, 3, typed.Metadata["window_size"])
	require.Equal(t, 1, typed.Metadata["overlap"])
	require.Equal(t, []ChunkBoundary{
		{Start: 0, End: 3, Type: "window", Name: "window_0"},
		{Start: 2, End: 5, Type: "window", Name: "window_1"},
		{Start: 4, End: 6, Type: "window", Name: "window_2"},
	}, typed.ChunkBoundaries)
}

func TestTextParserSingleWindowForShortContent(t *testing.T) {
	parser := &TextParser{}
	require.True(t, parser.CanParse("text/plain", ""))
	require.True(t, parser.CanParse("", "notes.txt"))
	require.False(t, parser.CanParse("text/markdown", "notes.md"))

	typed, err := parser.Parse(context.Background(), rawForParser([]byte("hello\nworld\n")))
	require.NoError(t, err)
	require.Equal(t, []ChunkBoundary{{Start: 0, End: 3, Type: "window", Name: "window_0"}}, typed.ChunkBoundaries)
}

func TestJSONAndYAMLParserFileChunks(t *testing.T) {
	jsonParser := &JSONParser{}
	require.True(t, jsonParser.CanParse("application/json", ""))
	require.True(t, jsonParser.CanParse("", "data.json"))
	require.False(t, jsonParser.CanParse("", "data.yaml"))

	typed, err := jsonParser.Parse(context.Background(), rawForParser([]byte(`{"k": "v"}`)))
	require.NoError(t, err)
	require.Equal(t, "json", typed.ContentType)
	require.Empty(t, typed.Metadata)
	require.Equal(t, []ChunkBoundary{{Start: 0, End: 1, Type: "file", Name: "file"}}, typed.ChunkBoundaries)

	yamlParser := &YAMLParser{}
	require.True(t, yamlParser.CanParse("application/yaml", ""))
	require.True(t, yamlParser.CanParse("", "conf.yaml"))
	require.True(t, yamlParser.CanParse("", "conf.yml"))
	require.False(t, yamlParser.CanParse("", "conf.json"))

	typed, err = yamlParser.Parse(context.Background(), rawForParser([]byte("k: v\n")))
	require.NoError(t, err)
	require.Equal(t, "yaml", typed.ContentType)
	require.Empty(t, typed.Metadata)
	require.Equal(t, []ChunkBoundary{{Start: 0, End: 1, Type: "file", Name: "file"}}, typed.ChunkBoundaries)
}

func TestLineIteratorWalksLines(t *testing.T) {
	it := NewLineIterator([]byte("alpha\nbeta\ngamma"))
	collected := make([]string, 0, 3)
	for {
		line, ok := it.Next()
		if !ok {
			break
		}
		collected = append(collected, line)
		require.Equal(t, len(collected), it.LineNum())
	}
	require.Equal(t, []string{"alpha", "beta", "gamma"}, collected)
	require.NoError(t, it.Err())
	_, ok := it.Next()
	require.False(t, ok, "iterator must be exhausted")
	require.Equal(t, 3, it.LineNum())
}

func TestLineIteratorOnEmptyContent(t *testing.T) {
	it := NewLineIterator(nil)
	line, ok := it.Next()
	require.False(t, ok)
	require.Empty(t, line)
	require.Equal(t, 0, it.LineNum())
	require.NoError(t, it.Err())
}

// FuzzParsers exercises every content parser against arbitrary untrusted
// bytes. Invariants: parsing never fails, propagates the raw content, and
// produces boundaries that stay within the line range of the input.
func FuzzParsers(f *testing.F) {
	f.Add([]byte("package main\n\nfunc Alpha() int {\n\treturn 1\n}\n"))
	f.Add([]byte("import os\n\ndef one():\n    pass\n\nclass Two:\n    pass\n"))
	f.Add([]byte("fn main() {}\n"))
	f.Add([]byte("function hi() {}\n"))
	f.Add([]byte("# Title\nbody\n## Sub\nmore\n"))
	f.Add([]byte(strings.Repeat("line\n", 120)))
	f.Add([]byte(`{"key": "value"}`))
	f.Add([]byte("key: value\nlist:\n  - a\n"))
	f.Add([]byte(""))
	f.Add([]byte{0x00, 0x01, 0x02})
	f.Add([]byte("héllo wörld\n# ünïcode heading\n"))

	f.Fuzz(func(t *testing.T, data []byte) {
		ctx := context.Background()
		parsers := []ContentParser{
			&GoParser{}, &PythonParser{}, &RustParser{}, &JSParser{},
			&MarkdownParser{}, &JSONParser{}, &YAMLParser{}, &TextParser{},
		}
		lineCount := len(strings.Split(string(data), "\n"))
		for _, parser := range parsers {
			typed, err := parser.Parse(ctx, rawForParser(data))
			require.NoError(t, err)
			require.NotNil(t, typed)
			require.Equal(t, data, typed.Content)
			require.NotEmpty(t, typed.ContentType)
			require.Equal(t, parserTestPrincipal, typed.SourcePrincipal)
			if lines, ok := typed.Metadata["lines"]; ok {
				require.Equal(t, lineCount, lines)
			}
			for _, boundary := range typed.ChunkBoundaries {
				require.GreaterOrEqual(t, boundary.Start, 0)
				require.LessOrEqual(t, boundary.Start, boundary.End)
				require.LessOrEqual(t, boundary.End, lineCount)
			}
		}

		parser := selectParser(string(data), "fuzz.txt")
		require.NotNil(t, parser)
	})
}

// FuzzDetectMIMETypeAndBinaryContent checks that MIME sniffing and the binary
// heuristic are total functions over arbitrary untrusted bytes.
//
// History: the ELF magic check used to index content[2]/content[3] under a
// len>1 guard and panicked on inputs like {0x7f, 'E'} (REVISIT-issues R-7,
// found by this fuzz target); the guard now requires len>3 and the seeds
// below pin those inputs to the octet-stream fallback.
func FuzzDetectMIMETypeAndBinaryContent(f *testing.F) {
	f.Add("", []byte("plain text\n"))
	f.Add("main.go", []byte(nil))
	f.Add("", []byte{0x7f, 'E'})
	f.Add("", []byte{0x7f, 'E', 'L'})
	f.Add("", []byte{0x7f, 'E', 'L', 'F', 0x02})
	f.Add("", []byte{0x89, 'P', 'N', 'G'})
	f.Add("x.unknownext", []byte("whatever"))
	f.Add("", []byte{0x00})

	f.Fuzz(func(t *testing.T, path string, content []byte) {
		mime := detectMIMEType(path, content)
		require.NotEmpty(t, mime)
		binary := isBinaryContent(content)
		if len(content) == 0 {
			require.False(t, binary)
		}
	})
}
