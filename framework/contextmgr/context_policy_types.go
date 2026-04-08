package contextmgr

import (
	"fmt"
	"github.com/lexcodex/relurpify/framework/ast"
	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/framework/memory"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// ContextStrategy defines how an agent manages context.
type ContextStrategy interface {
	// SelectContext determines what context to load initially.
	SelectContext(task *core.Task, budget *core.ContextBudget) (*ContextRequest, error)

	// ShouldCompress decides when to compress history.
	ShouldCompress(ctx *core.SharedContext) bool

	// DetermineDetailLevel chooses appropriate detail for content.
	DetermineDetailLevel(file string, relevance float64) DetailLevel

	// ShouldExpandContext decides if more context is needed.
	ShouldExpandContext(ctx *core.SharedContext, lastResult *core.Result) bool

	// PrioritizeContext ranks context items by importance.
	PrioritizeContext(items []core.ContextItem) []core.ContextItem
}

// ChunkLoader is an optional extension point for strategies that can seed
// context from a semantic chunk sequence before normal file/AST/memory loading.
type ChunkLoader interface {
	LoadChunks(task *core.Task, budget *core.ContextBudget) ([]ContextChunk, error)
}

// ContextRequest describes what context to load.
type ContextRequest struct {
	Files         []FileRequest
	ASTQueries    []ASTQuery
	MemoryQueries []MemoryQuery
	SearchQueries []SearchQuery
	ChunkSequence []ContextChunk
	MaxTokens     int
}

// ContextChunk is a framework-generic semantic chunk payload.
type ContextChunk struct {
	ID            string
	Content       string
	TokenEstimate int
	Metadata      map[string]string
}

// FileRequest specifies how to load a file.
type FileRequest struct {
	Path        string
	DetailLevel DetailLevel
	Priority    int
	Pinned      bool
}

// DetailLevel controls content granularity.
type DetailLevel int

const (
	DetailFull DetailLevel = iota
	DetailDetailed
	DetailConcise
	DetailMinimal
	DetailSignatureOnly
)

func (dl DetailLevel) String() string {
	switch dl {
	case DetailFull:
		return "full"
	case DetailDetailed:
		return "detailed"
	case DetailConcise:
		return "concise"
	case DetailMinimal:
		return "minimal"
	case DetailSignatureOnly:
		return "signature_only"
	default:
		return "unknown"
	}
}

// ASTQuery requests structured code information.
type ASTQuery struct {
	Type   ASTQueryType
	Symbol string
	Filter ASTFilter
}

// ASTQueryType enumerates supported AST operations.
type ASTQueryType string

const (
	ASTQueryListSymbols     ASTQueryType = "list_symbols"
	ASTQueryGetSignature    ASTQueryType = "get_signature"
	ASTQueryFindCallers     ASTQueryType = "find_callers"
	ASTQueryFindCallees     ASTQueryType = "find_callees"
	ASTQueryGetDependencies ASTQueryType = "get_dependencies"
)

// ASTFilter narrows down AST responses.
type ASTFilter struct {
	Types        []ast.NodeType
	Categories   []ast.Category
	ExportedOnly bool
}

// MemoryQuery requests information from memory stores.
type MemoryQuery struct {
	Scope      memory.MemoryScope
	Query      string
	MaxResults int
}

// ContextLoadEvent tracks context loading decisions.
type ContextLoadEvent struct {
	Timestamp   time.Time
	Trigger     string
	RequestType string
	ItemsLoaded int
	TokensAdded int
	Success     bool
	Reason      string
}

var (
	fileReferenceRegex   = regexp.MustCompile(`[\w./-]+\.[\w]+`)
	symbolReferenceRegex = regexp.MustCompile(`([A-Za-z_][A-Za-z0-9_]+)\(`)
)

// ExtractFileReferences scans text for file-like tokens.
func ExtractFileReferences(text string) []string {
	matches := fileReferenceRegex.FindAllString(text, -1)
	unique := make(map[string]struct{})
	refs := make([]string, 0, len(matches))
	for _, match := range matches {
		clean := filepath.Clean(match)
		if _, ok := unique[clean]; ok {
			continue
		}
		unique[clean] = struct{}{}
		refs = append(refs, clean)
	}
	return refs
}

// ExtractContextFiles returns explicit file paths provided by the caller.
func ExtractContextFiles(task *core.Task) []string {
	if task == nil || task.Context == nil {
		return nil
	}
	raw, ok := task.Context["context_files"]
	if !ok || raw == nil {
		return nil
	}
	paths := make([]string, 0)
	switch v := raw.(type) {
	case []string:
		paths = append(paths, v...)
	case []interface{}:
		for _, item := range v {
			if s, ok := item.(string); ok {
				paths = append(paths, s)
			}
		}
	case string:
		paths = append(paths, v)
	}
	unique := make(map[string]struct{}, len(paths))
	out := make([]string, 0, len(paths))
	for _, path := range paths {
		clean := filepath.Clean(strings.TrimSpace(path))
		if clean == "" {
			continue
		}
		if _, ok := unique[clean]; ok {
			continue
		}
		unique[clean] = struct{}{}
		out = append(out, clean)
	}
	return out
}

// ResolveContextRequestPaths maps relative file requests into the active
// workspace so initial context loads use the derived workspace, not process cwd.
func ResolveContextRequestPaths(request *ContextRequest, task *core.Task) {
	if request == nil || task == nil || task.Context == nil {
		return
	}
	root := strings.TrimSpace(stringValue(task.Context["workspace"]))
	if root == "" || root == "." {
		return
	}
	root = filepath.Clean(root)
	for i := range request.Files {
		path := strings.TrimSpace(request.Files[i].Path)
		if path == "" || filepath.IsAbs(path) {
			continue
		}
		request.Files[i].Path = filepath.Join(root, filepath.FromSlash(path))
	}
}

func stringValue(v any) string {
	if v == nil {
		return ""
	}
	if s, ok := v.(string); ok {
		return s
	}
	return strings.TrimSpace(fmt.Sprint(v))
}

// AppendContextFiles injects explicit file requests into an existing request.
func AppendContextFiles(request *ContextRequest, task *core.Task, level DetailLevel) {
	if request == nil {
		return
	}
	paths := ExtractContextFiles(task)
	if len(paths) == 0 {
		return
	}
	existing := make(map[string]struct{}, len(request.Files))
	for _, req := range request.Files {
		if req.Path == "" {
			continue
		}
		existing[req.Path] = struct{}{}
	}
	for _, path := range paths {
		if _, ok := existing[path]; ok {
			continue
		}
		request.Files = append(request.Files, FileRequest{
			Path:        path,
			DetailLevel: level,
			Priority:    -1,
			Pinned:      true,
		})
	}
}

// ExtractSymbolReferences returns probable symbol names mentioned in the text.
func ExtractSymbolReferences(text string) []string {
	matches := symbolReferenceRegex.FindAllStringSubmatch(text, -1)
	unique := make(map[string]struct{})
	symbols := make([]string, 0, len(matches))
	for _, match := range matches {
		if len(match) < 2 {
			continue
		}
		name := match[1]
		if _, ok := unique[name]; ok {
			continue
		}
		unique[name] = struct{}{}
		symbols = append(symbols, name)
	}
	return symbols
}

// ExtractKeywords returns a truncated keyword string for search queries.
func ExtractKeywords(text string) string {
	words := strings.Fields(text)
	if len(words) > 10 {
		words = words[:10]
	}
	return strings.Join(words, " ")
}

// ContainsInsensitive checks if substr appears in text ignoring case.
func ContainsInsensitive(text, substr string) bool {
	return strings.Contains(strings.ToLower(text), strings.ToLower(substr))
}

func countKeywords(text string, keywords []string) int {
	count := 0
	lower := strings.ToLower(text)
	for _, kw := range keywords {
		if strings.Contains(lower, strings.ToLower(kw)) {
			count++
		}
	}
	return count
}
