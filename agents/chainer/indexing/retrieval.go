package indexing

import (
	"fmt"
	"strings"
	"time"
)

// RetrievalQuery specifies criteria for retrieving indexed code snippets.
type RetrievalQuery struct {
	// Symbol-based retrieval
	SymbolNames []string

	// Path-based retrieval
	FilePath        string
	FilePathPattern string

	// Content-based retrieval
	KeywordPattern string

	// Link-based retrieval
	LinkName string

	// Task-based retrieval
	TaskID string

	// Time-based retrieval
	Since time.Time

	// Filters
	Language      string
	ExcludeErrors bool

	// Limits
	Limit  int
	Offset int
}

// Retriever provides retrieval of indexed code snippets.
type Retriever struct {
	index *CodeIndex
}

// NewRetriever creates a retriever for a code index.
func NewRetriever(index *CodeIndex) *Retriever {
	return &Retriever{
		index: index,
	}
}

// Retrieve executes a retrieval query against the index.
func (r *Retriever) Retrieve(query *RetrievalQuery) []*IndexedCodeSnippet {
	if r == nil || r.index == nil || query == nil {
		return nil
	}

	candidates := r.index.AllSnippets()
	if len(candidates) == 0 {
		return []*IndexedCodeSnippet{}
	}

	// Apply filters
	candidates = filterBySymbols(candidates, query.SymbolNames)
	candidates = filterByPath(candidates, query.FilePath, query.FilePathPattern)
	candidates = filterByKeyword(candidates, query.KeywordPattern)
	candidates = filterByLink(candidates, query.LinkName)
	candidates = filterByTask(candidates, query.TaskID)
	candidates = filterBySince(candidates, query.Since)
	candidates = filterByLanguage(candidates, query.Language)
	candidates = filterErrors(candidates, query.ExcludeErrors)

	// Apply pagination
	if query.Offset > 0 && query.Offset < len(candidates) {
		candidates = candidates[query.Offset:]
	}

	if query.Limit > 0 && len(candidates) > query.Limit {
		candidates = candidates[:query.Limit]
	}
	if candidates == nil {
		return []*IndexedCodeSnippet{}
	}

	return candidates
}

// RetrieveSimilar finds snippets similar to given criteria.
//
// This is a simplified similarity matching that looks for:
// - Overlapping symbols
// - Same file path
// - Similar language
func (r *Retriever) RetrieveSimilar(snippet *IndexedCodeSnippet, limit int) []*IndexedCodeSnippet {
	if r == nil || r.index == nil || snippet == nil {
		return nil
	}

	query := &RetrievalQuery{
		SymbolNames: snippet.Symbols,
		FilePath:    snippet.FilePath,
		Language:    snippet.Language,
		Limit:       limit,
	}

	return r.Retrieve(query)
}

// RetrieveByLanguage finds all snippets of a given language.
func (r *Retriever) RetrieveByLanguage(language string, limit int) []*IndexedCodeSnippet {
	if r == nil || r.index == nil {
		return nil
	}

	query := &RetrievalQuery{
		Language: language,
		Limit:    limit,
	}

	return r.Retrieve(query)
}

// RetrieveRecent retrieves snippets from the last duration.
func (r *Retriever) RetrieveRecent(duration time.Duration, limit int) []*IndexedCodeSnippet {
	if r == nil || r.index == nil {
		return nil
	}

	query := &RetrievalQuery{
		Since: time.Now().Add(-duration),
		Limit: limit,
	}

	return r.Retrieve(query)
}

// Statistics returns statistics about indexed code.
type Statistics struct {
	TotalSnippets   int
	UniqueLanguages int
	UniquePaths     int
	UniqueSymbols   int
	ErrorSnippets   int
	SuccessSnippets int
	Languages       map[string]int
	TopSymbols      map[string]int
	TopFiles        map[string]int
}

// Stats generates statistics about the index.
func (r *Retriever) Stats() *Statistics {
	if r == nil || r.index == nil {
		return &Statistics{}
	}

	all := r.index.AllSnippets()
	if len(all) == 0 {
		return &Statistics{}
	}

	stats := &Statistics{
		TotalSnippets: len(all),
		Languages:     make(map[string]int),
		TopSymbols:    make(map[string]int),
		TopFiles:      make(map[string]int),
	}

	uniqueLangs := make(map[string]bool)
	uniquePaths := make(map[string]bool)
	uniqueSymbols := make(map[string]bool)

	for _, snippet := range all {
		if snippet.IsError {
			stats.ErrorSnippets++
		} else {
			stats.SuccessSnippets++
		}

		if snippet.Language != "" {
			uniqueLangs[snippet.Language] = true
			stats.Languages[snippet.Language]++
		}

		if snippet.FilePath != "" {
			uniquePaths[snippet.FilePath] = true
			stats.TopFiles[snippet.FilePath]++
		}

		for _, sym := range snippet.Symbols {
			uniqueSymbols[sym] = true
			stats.TopSymbols[sym]++
		}
	}

	stats.UniqueLanguages = len(uniqueLangs)
	stats.UniquePaths = len(uniquePaths)
	stats.UniqueSymbols = len(uniqueSymbols)

	return stats
}

// Filter functions

func filterBySymbols(snippets []*IndexedCodeSnippet, symbols []string) []*IndexedCodeSnippet {
	if len(symbols) == 0 {
		return snippets
	}

	var result []*IndexedCodeSnippet
	for _, snippet := range snippets {
		if hasAnySymbol(snippet.Symbols, symbols) {
			result = append(result, snippet)
		}
	}
	return result
}

func filterByPath(snippets []*IndexedCodeSnippet, path, pattern string) []*IndexedCodeSnippet {
	if path == "" && pattern == "" {
		return snippets
	}

	var result []*IndexedCodeSnippet
	for _, snippet := range snippets {
		if path != "" && snippet.FilePath == path {
			result = append(result, snippet)
		} else if pattern != "" && strings.Contains(snippet.FilePath, pattern) {
			result = append(result, snippet)
		}
	}
	return result
}

func filterByKeyword(snippets []*IndexedCodeSnippet, keyword string) []*IndexedCodeSnippet {
	if keyword == "" {
		return snippets
	}

	var result []*IndexedCodeSnippet
	keyword = strings.ToLower(keyword)
	for _, snippet := range snippets {
		if strings.Contains(strings.ToLower(snippet.Source), keyword) {
			result = append(result, snippet)
		}
	}
	return result
}

func filterByLink(snippets []*IndexedCodeSnippet, linkName string) []*IndexedCodeSnippet {
	if linkName == "" {
		return snippets
	}

	var result []*IndexedCodeSnippet
	for _, snippet := range snippets {
		if snippet.LinkName == linkName {
			result = append(result, snippet)
		}
	}
	return result
}

func filterByTask(snippets []*IndexedCodeSnippet, taskID string) []*IndexedCodeSnippet {
	if taskID == "" {
		return snippets
	}

	var result []*IndexedCodeSnippet
	for _, snippet := range snippets {
		if snippet.TaskID == taskID {
			result = append(result, snippet)
		}
	}
	return result
}

func filterBySince(snippets []*IndexedCodeSnippet, since time.Time) []*IndexedCodeSnippet {
	if since.IsZero() {
		return snippets
	}

	var result []*IndexedCodeSnippet
	for _, snippet := range snippets {
		if snippet.Timestamp.After(since) || snippet.Timestamp.Equal(since) {
			result = append(result, snippet)
		}
	}
	return result
}

func filterByLanguage(snippets []*IndexedCodeSnippet, language string) []*IndexedCodeSnippet {
	if language == "" {
		return snippets
	}

	var result []*IndexedCodeSnippet
	for _, snippet := range snippets {
		if snippet.Language == language {
			result = append(result, snippet)
		}
	}
	return result
}

func filterErrors(snippets []*IndexedCodeSnippet, excludeErrors bool) []*IndexedCodeSnippet {
	if !excludeErrors {
		return snippets
	}

	var result []*IndexedCodeSnippet
	for _, snippet := range snippets {
		if !snippet.IsError {
			result = append(result, snippet)
		}
	}
	return result
}

func hasAnySymbol(haystack, needles []string) bool {
	if len(needles) == 0 {
		return true
	}

	for _, needle := range needles {
		for _, hay := range haystack {
			if hay == needle {
				return true
			}
		}
	}
	return false
}

// RetrievalSummary provides summary of retrieval results.
type RetrievalSummary struct {
	QueryDescription string
	ResultCount      int
	LanguagesFound   []string
	FilesFound       []string
	SymbolsFound     []string
}

// Summary generates a summary of retrieval results.
func (r *Retriever) Summary(results []*IndexedCodeSnippet, query *RetrievalQuery) *RetrievalSummary {
	if r == nil {
		return &RetrievalSummary{}
	}

	summary := &RetrievalSummary{
		QueryDescription: describeQuery(query),
		ResultCount:      len(results),
		LanguagesFound:   make([]string, 0),
		FilesFound:       make([]string, 0),
		SymbolsFound:     make([]string, 0),
	}

	langSet := make(map[string]bool)
	fileSet := make(map[string]bool)
	symSet := make(map[string]bool)

	for _, result := range results {
		if result.Language != "" {
			langSet[result.Language] = true
		}
		if result.FilePath != "" {
			fileSet[result.FilePath] = true
		}
		for _, sym := range result.Symbols {
			symSet[sym] = true
		}
	}

	for lang := range langSet {
		summary.LanguagesFound = append(summary.LanguagesFound, lang)
	}
	for file := range fileSet {
		summary.FilesFound = append(summary.FilesFound, file)
	}
	for sym := range symSet {
		summary.SymbolsFound = append(summary.SymbolsFound, sym)
	}

	return summary
}

func describeQuery(query *RetrievalQuery) string {
	if query == nil {
		return "empty query"
	}

	parts := make([]string, 0)

	if len(query.SymbolNames) > 0 {
		parts = append(parts, fmt.Sprintf("symbols:%v", query.SymbolNames))
	}
	if query.FilePath != "" {
		parts = append(parts, fmt.Sprintf("path:%s", query.FilePath))
	}
	if query.FilePathPattern != "" {
		parts = append(parts, fmt.Sprintf("pattern:%s", query.FilePathPattern))
	}
	if query.KeywordPattern != "" {
		parts = append(parts, fmt.Sprintf("keyword:%s", query.KeywordPattern))
	}
	if query.LinkName != "" {
		parts = append(parts, fmt.Sprintf("link:%s", query.LinkName))
	}
	if query.TaskID != "" {
		parts = append(parts, fmt.Sprintf("task:%s", query.TaskID))
	}
	if query.Language != "" {
		parts = append(parts, fmt.Sprintf("lang:%s", query.Language))
	}
	if query.ExcludeErrors {
		parts = append(parts, "exclude-errors")
	}

	if len(parts) == 0 {
		return "all snippets"
	}

	return strings.Join(parts, ", ")
}
