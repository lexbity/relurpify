package indexing

import (
	"fmt"
	"sync"
	"time"
)

// IndexedCodeSnippet represents a code snippet indexed from link evaluation.
type IndexedCodeSnippet struct {
	// Identification
	ID        string    // Unique identifier
	TaskID    string    // Task that generated this
	LinkName  string    // Link that evaluated it
	Timestamp time.Time // When indexed

	// Content
	Source       string // Original source code
	FilePath     string // File path (if known)
	Language     string // Programming language
	StartLine    int    // Start line in source (if applicable)
	EndLine      int    // End line in source (if applicable)

	// Extracted Symbols
	Symbols     []string // Function/type/constant names found
	Imports     []string // Package/module imports
	Dependencies []string // Referenced symbols from other sources

	// Metadata
	IsError      bool   // Whether this is error code
	Analysis     string // Result of analysis (if any)
	ErrorMessage string // Error details (if IsError)
}

// CodeIndex stores and retrieves indexed code snippets.
type CodeIndex struct {
	mu       sync.RWMutex
	snippets map[string]*IndexedCodeSnippet
	byTask   map[string][]*IndexedCodeSnippet
	byLink   map[string][]*IndexedCodeSnippet
	byPath   map[string][]*IndexedCodeSnippet
	bySymbol map[string][]*IndexedCodeSnippet
}

// NewCodeIndex creates a new code index.
func NewCodeIndex() *CodeIndex {
	return &CodeIndex{
		snippets: make(map[string]*IndexedCodeSnippet),
		byTask:   make(map[string][]*IndexedCodeSnippet),
		byLink:   make(map[string][]*IndexedCodeSnippet),
		byPath:   make(map[string][]*IndexedCodeSnippet),
		bySymbol: make(map[string][]*IndexedCodeSnippet),
	}
}

// Index stores a code snippet in the index.
func (ci *CodeIndex) Index(snippet *IndexedCodeSnippet) error {
	if ci == nil {
		return fmt.Errorf("code index not initialized")
	}

	if snippet == nil {
		return fmt.Errorf("cannot index nil snippet")
	}

	if snippet.ID == "" {
		return fmt.Errorf("snippet must have an ID")
	}

	ci.mu.Lock()
	defer ci.mu.Unlock()

	// Store main snippet
	ci.snippets[snippet.ID] = snippet

	// Index by task
	ci.byTask[snippet.TaskID] = append(ci.byTask[snippet.TaskID], snippet)

	// Index by link
	ci.byLink[snippet.LinkName] = append(ci.byLink[snippet.LinkName], snippet)

	// Index by path
	if snippet.FilePath != "" {
		ci.byPath[snippet.FilePath] = append(ci.byPath[snippet.FilePath], snippet)
	}

	// Index by symbols
	for _, sym := range snippet.Symbols {
		ci.bySymbol[sym] = append(ci.bySymbol[sym], snippet)
	}

	return nil
}

// Get retrieves a snippet by ID.
func (ci *CodeIndex) Get(id string) *IndexedCodeSnippet {
	if ci == nil {
		return nil
	}

	ci.mu.RLock()
	defer ci.mu.RUnlock()

	return ci.snippets[id]
}

// ByTask returns all snippets from a task.
func (ci *CodeIndex) ByTask(taskID string) []*IndexedCodeSnippet {
	if ci == nil {
		return nil
	}

	ci.mu.RLock()
	defer ci.mu.RUnlock()

	snippets := ci.byTask[taskID]
	// Return copy to prevent external mutation
	result := make([]*IndexedCodeSnippet, len(snippets))
	copy(result, snippets)
	return result
}

// ByLink returns all snippets evaluated by a link.
func (ci *CodeIndex) ByLink(linkName string) []*IndexedCodeSnippet {
	if ci == nil {
		return nil
	}

	ci.mu.RLock()
	defer ci.mu.RUnlock()

	snippets := ci.byLink[linkName]
	// Return copy to prevent external mutation
	result := make([]*IndexedCodeSnippet, len(snippets))
	copy(result, snippets)
	return result
}

// ByPath returns all snippets from a file path.
func (ci *CodeIndex) ByPath(filePath string) []*IndexedCodeSnippet {
	if ci == nil {
		return nil
	}

	ci.mu.RLock()
	defer ci.mu.RUnlock()

	snippets := ci.byPath[filePath]
	// Return copy to prevent external mutation
	result := make([]*IndexedCodeSnippet, len(snippets))
	copy(result, snippets)
	return result
}

// BySymbol returns all snippets containing a symbol.
func (ci *CodeIndex) BySymbol(symbolName string) []*IndexedCodeSnippet {
	if ci == nil {
		return nil
	}

	ci.mu.RLock()
	defer ci.mu.RUnlock()

	snippets := ci.bySymbol[symbolName]
	// Return copy to prevent external mutation
	result := make([]*IndexedCodeSnippet, len(snippets))
	copy(result, snippets)
	return result
}

// AllSnippets returns all indexed snippets.
func (ci *CodeIndex) AllSnippets() []*IndexedCodeSnippet {
	if ci == nil {
		return nil
	}

	ci.mu.RLock()
	defer ci.mu.RUnlock()

	var result []*IndexedCodeSnippet
	for _, snippet := range ci.snippets {
		result = append(result, snippet)
	}
	return result
}

// Count returns the number of indexed snippets.
func (ci *CodeIndex) Count() int {
	if ci == nil {
		return 0
	}

	ci.mu.RLock()
	defer ci.mu.RUnlock()

	return len(ci.snippets)
}

// Clear removes all snippets.
func (ci *CodeIndex) Clear() {
	if ci == nil {
		return
	}

	ci.mu.Lock()
	defer ci.mu.Unlock()

	ci.snippets = make(map[string]*IndexedCodeSnippet)
	ci.byTask = make(map[string][]*IndexedCodeSnippet)
	ci.byLink = make(map[string][]*IndexedCodeSnippet)
	ci.byPath = make(map[string][]*IndexedCodeSnippet)
	ci.bySymbol = make(map[string][]*IndexedCodeSnippet)
}

// Delete removes a snippet by ID.
func (ci *CodeIndex) Delete(id string) {
	if ci == nil {
		return
	}

	ci.mu.Lock()
	defer ci.mu.Unlock()

	snippet, exists := ci.snippets[id]
	if !exists {
		return
	}

	// Remove from main store
	delete(ci.snippets, id)

	// Remove from task index
	if snippets, ok := ci.byTask[snippet.TaskID]; ok {
		filtered := make([]*IndexedCodeSnippet, 0)
		for _, s := range snippets {
			if s.ID != id {
				filtered = append(filtered, s)
			}
		}
		if len(filtered) > 0 {
			ci.byTask[snippet.TaskID] = filtered
		} else {
			delete(ci.byTask, snippet.TaskID)
		}
	}

	// Remove from link index
	if snippets, ok := ci.byLink[snippet.LinkName]; ok {
		filtered := make([]*IndexedCodeSnippet, 0)
		for _, s := range snippets {
			if s.ID != id {
				filtered = append(filtered, s)
			}
		}
		if len(filtered) > 0 {
			ci.byLink[snippet.LinkName] = filtered
		} else {
			delete(ci.byLink, snippet.LinkName)
		}
	}

	// Remove from path index
	if snippet.FilePath != "" {
		if snippets, ok := ci.byPath[snippet.FilePath]; ok {
			filtered := make([]*IndexedCodeSnippet, 0)
			for _, s := range snippets {
				if s.ID != id {
					filtered = append(filtered, s)
				}
			}
			if len(filtered) > 0 {
				ci.byPath[snippet.FilePath] = filtered
			} else {
				delete(ci.byPath, snippet.FilePath)
			}
		}
	}

	// Remove from symbol indices
	for _, sym := range snippet.Symbols {
		if snippets, ok := ci.bySymbol[sym]; ok {
			filtered := make([]*IndexedCodeSnippet, 0)
			for _, s := range snippets {
				if s.ID != id {
					filtered = append(filtered, s)
				}
			}
			if len(filtered) > 0 {
				ci.bySymbol[sym] = filtered
			} else {
				delete(ci.bySymbol, sym)
			}
		}
	}
}
