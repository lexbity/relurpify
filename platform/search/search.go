package search

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

		"codeburg.org/lexbit/relurpify/platform/contracts"
)

// GrepTool implements plain text search.
type GrepTool struct {
	BasePath string
}

func (t *GrepTool) Name() string        { return "search_grep" }
func (t *GrepTool) Description() string { return "Searches files using substring matching." }
func (t *GrepTool) Category() string    { return "search" }
func (t *GrepTool) Parameters() []contracts.ToolParameter {
	return []contracts.ToolParameter{
		{Name: "pattern", Type: "string", Required: true},
		{Name: "directory", Type: "string", Required: false, Default: "."},
	}
}
func (t *GrepTool) Execute(ctx context.Context, args map[string]interface{}) (*contracts.ToolResult, error) {
	root := fmt.Sprint(args["directory"])
	if root == "" {
		root = "."
	}
	root = preparePath(t.BasePath, root)
	pattern := strings.ToLower(fmt.Sprint(args["pattern"]))
	type match struct {
		File    string `json:"file"`
		Line    int    `json:"line"`
		Content string `json:"content"`
	}
	var matches []match
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			if shouldSkipGeneratedDir(info.Name()) {
				return filepath.SkipDir
			}
			return nil
		}
		file, err := os.Open(path)
		if err != nil {
			return nil // skip unreadable files
		}
		defer file.Close()
		scanner := bufio.NewScanner(file)
		scanner.Buffer(make([]byte, scanChunkSize), scanChunkSize)
		scanner.Split(scanLinesOrChunks(scanChunkSize))
		line := 1
		for scanner.Scan() {
			text := scanner.Text()
			if strings.Contains(strings.ToLower(text), pattern) {
				matches = append(matches, match{File: path, Line: line, Content: text})
			}
			line++
		}
		// Skip files with I/O errors (e.g. permission denied mid-read).
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &contracts.ToolResult{Success: true, Data: map[string]interface{}{"matches": matches}}, nil
}
func (t *GrepTool) IsAvailable(ctx context.Context) bool { return true }

func (t *GrepTool) Permissions() contracts.ToolPermissions {
	return contracts.ToolPermissions{Permissions: &contracts.PermissionSet{}}
}
func (t *GrepTool) Tags() []string { return []string{contracts.TagReadOnly, "search", "recovery"} }

// SimilarityTool finds similar snippets using a naive approach.
type SimilarityTool struct {
	BasePath string
}

func (t *SimilarityTool) Name() string        { return "search_find_similar" }
func (t *SimilarityTool) Description() string { return "Finds structurally similar code snippets." }
func (t *SimilarityTool) Category() string    { return "search" }
func (t *SimilarityTool) Parameters() []contracts.ToolParameter {
	return []contracts.ToolParameter{
		{Name: "snippet", Type: "string", Required: true},
		{Name: "directory", Type: "string", Required: false, Default: "."},
	}
}
func (t *SimilarityTool) Execute(ctx context.Context, args map[string]interface{}) (*contracts.ToolResult, error) {
	dirArg, _ := args["directory"].(string)
	root := preparePath(t.BasePath, dirArg)
	target := sanitizeSnippet(fmt.Sprint(args["snippet"]))
	terms := semanticTerms(fmt.Sprint(args["snippet"]))
	type match struct {
		File     string  `json:"file"`
		Score    float64 `json:"score"`
		Fragment string  `json:"fragment"`
	}
	var matches []match
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			if err == nil && info.IsDir() && shouldSkipGeneratedDir(info.Name()) {
				return filepath.SkipDir
			}
			return err
		}
		if !isSimilarityCandidate(path) {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		content := string(data)
		score := semanticScore(terms, strings.ToLower(content))
		if score == 0 {
			score = jaccard(target, sanitizeSnippet(content))
		}
		if score > 0.3 {
			matches = append(matches, match{File: path, Score: score, Fragment: summarize(content)})
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Slice(matches, func(i, j int) bool { return matches[i].Score > matches[j].Score })
	return &contracts.ToolResult{Success: true, Data: map[string]interface{}{"matches": matches}}, nil
}
func (t *SimilarityTool) IsAvailable(ctx context.Context) bool { return true }

func (t *SimilarityTool) Permissions() contracts.ToolPermissions {
	return contracts.ToolPermissions{Permissions: &contracts.PermissionSet{}}
}
func (t *SimilarityTool) Tags() []string { return []string{contracts.TagReadOnly, "search"} }

// SemanticSearchTool uses a vector-like heuristic (currently substring).
type SemanticSearchTool struct {
	BasePath string
}

func (t *SemanticSearchTool) Name() string { return "search_semantic" }
func (t *SemanticSearchTool) Description() string {
	return "Performs semantic search using heuristic embeddings."
}
func (t *SemanticSearchTool) Category() string { return "search" }
func (t *SemanticSearchTool) Parameters() []contracts.ToolParameter {
	return []contracts.ToolParameter{{Name: "query", Type: "string", Required: true}}
}
func (t *SemanticSearchTool) Execute(ctx context.Context, args map[string]interface{}) (*contracts.ToolResult, error) {
	query := strings.ToLower(fmt.Sprint(args["query"]))
	terms := semanticTerms(query)
	var hits []map[string]interface{}
	err := filepath.Walk(t.BasePath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			if shouldSkipGeneratedDir(info.Name()) {
				return filepath.SkipDir
			}
			return nil
		}
		if !isSemanticCandidate(path) {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		content := strings.ToLower(string(data))
		score := semanticScore(terms, content)
		if score > 0 {
			hits = append(hits, map[string]interface{}{
				"file":    path,
				"score":   score,
				"snippet": summarize(string(data)),
			})
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Slice(hits, func(i, j int) bool {
		left, _ := hits[i]["score"].(float64)
		right, _ := hits[j]["score"].(float64)
		return left > right
	})
	return &contracts.ToolResult{Success: true, Data: map[string]interface{}{"results": hits}}, nil
}
func (t *SemanticSearchTool) IsAvailable(ctx context.Context) bool {
	return true
}

func (t *SemanticSearchTool) Permissions() contracts.ToolPermissions {
	return contracts.ToolPermissions{Permissions: &contracts.PermissionSet{}}
}
func (t *SemanticSearchTool) Tags() []string { return []string{contracts.TagReadOnly, "search"} }

func sanitizeSnippet(snippet string) string {
	return strings.ToLower(strings.ReplaceAll(snippet, " ", ""))
}

func isSimilarityCandidate(path string) bool {
	if shouldSkipSearchPath(path) {
		return false
	}
	switch strings.ToLower(filepath.Ext(path)) {
	case ".go", ".rs", ".py", ".js", ".ts", ".tsx", ".jsx", ".sql":
		return true
	default:
		return false
	}
}

func isSemanticCandidate(path string) bool {
	if shouldSkipSearchPath(path) {
		return false
	}
	switch strings.ToLower(filepath.Ext(path)) {
	case ".go", ".md", ".txt", ".sql", ".rs", ".py", ".js", ".ts", ".tsx", ".jsx":
		return true
	default:
		return false
	}
}

func shouldSkipSearchPath(path string) bool {
	path = filepath.ToSlash(path)
	if strings.Contains(path, "/testsuite/agenttests/") {
		return true
	}
	base := filepath.Base(path)
	return strings.HasPrefix(base, ".")
}

func semanticTerms(query string) []string {
	fields := strings.FieldsFunc(strings.ToLower(query), func(r rune) bool {
		return !(r >= 'a' && r <= 'z' || r >= '0' && r <= '9')
	})
	seen := map[string]struct{}{}
	out := make([]string, 0, len(fields))
	for _, field := range fields {
		if len(field) < 3 {
			continue
		}
		if _, ok := seen[field]; ok {
			continue
		}
		seen[field] = struct{}{}
		out = append(out, field)
	}
	return out
}

func semanticScore(terms []string, content string) float64 {
	if len(terms) == 0 {
		return 0
	}
	matches := 0
	for _, term := range terms {
		if strings.Contains(content, term) {
			matches++
		}
	}
	return float64(matches) / float64(len(terms))
}

func jaccard(a, b string) float64 {
	setA := make(map[rune]bool)
	for _, r := range a {
		setA[r] = true
	}
	setB := make(map[rune]bool)
	for _, r := range b {
		setB[r] = true
	}
	intersection := 0
	for r := range setA {
		if setB[r] {
			intersection++
		}
	}
	union := len(setA) + len(setB) - intersection
	if union == 0 {
		return 0
	}
	return float64(intersection) / float64(union)
}

func summarize(content string) string {
	lines := strings.Split(content, "\n")
	if len(lines) > 5 {
		return strings.Join(lines[:5], "\n")
	}
	return content
}
