package core

import (
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"
)

// SharedContext wraps Context with richer memory primitives (working set,
// summaries, compression) geared toward large-repo workflows.
type SharedContext struct {
	*Context

	contextBudget *ContextBudget
	workingSet    *WorkingSet
	summarizer    Summarizer

	mu                  sync.RWMutex
	conversationSummary string
	changeLogSummary    string
	fileItems           map[string]*fileBudgetItem
	mutationLog         []StateMutation // capped ring buffer of recent state mutations
}

// NewSharedContext constructs a shared context with optional helpers. Passing
// nil uses sensible defaults for ad-hoc sessions.
func NewSharedContext(base *Context, budget *ContextBudget, summarizer Summarizer) *SharedContext {
	if base == nil {
		base = NewContext()
	}
	if budget == nil {
		budget = NewContextBudget(8192)
	}
	sc := &SharedContext{
		Context:       base,
		contextBudget: budget,
		workingSet:    NewWorkingSet(12, EvictLRU),
		summarizer:    summarizer,
		fileItems:     make(map[string]*fileBudgetItem),
	}
	if budget != nil {
		budget.AddListener(sc)
	}
	return sc
}

// AddFile inserts or updates a file in the working set and budget.
func (sc *SharedContext) AddFile(path, content, language string, level DetailLevel) (*FileContext, error) {
	if path == "" {
		return nil, fmt.Errorf("file path required")
	}
	fc := &FileContext{
		Path:         path,
		Language:     language,
		Content:      content,
		RawContent:   content,
		Level:        level,
		LastAccessed: time.Now().UTC(),
	}
	if fc.Summary == "" && sc.summarizer != nil {
		if summary, err := sc.summarizer.Summarize(content, SummaryConcise); err == nil {
			fc.Summary = summary
		}
	}
	sc.workingSet.Add(fc)
	item := newFileBudgetItem(fc, sc.summarizer)
	sc.fileItems[path] = item
	if sc.contextBudget != nil {
		_ = sc.contextBudget.Allocate("immediate", item.GetTokenCount(), item)
	}
	return fc, nil
}

// GetFile returns a tracked file if available.
func (sc *SharedContext) GetFile(path string) (*FileContext, bool) {
	return sc.workingSet.Get(path)
}

// FileReference returns the stable reference for a tracked file, if present.
func (sc *SharedContext) FileReference(path string) (ContextReference, bool) {
	fc, ok := sc.GetFile(path)
	if !ok || fc == nil {
		return ContextReference{}, false
	}
	return ContextReference{
		Kind:    ContextReferenceFile,
		ID:      fc.Path,
		URI:     fc.Path,
		Version: fc.Version,
		Detail:  detailLevelName(fc.Level),
		Metadata: map[string]string{
			"language": fc.Language,
		},
	}, true
}

// HydrateFileReference ensures the referenced file is available at the desired detail level.
func (sc *SharedContext) HydrateFileReference(ref ContextReference, desired DetailLevel) (*FileContext, error) {
	if sc == nil {
		return nil, fmt.Errorf("shared context unavailable")
	}
	if ref.Kind != ContextReferenceFile {
		return nil, fmt.Errorf("unsupported context reference kind %q", ref.Kind)
	}
	path := strings.TrimSpace(ref.URI)
	if path == "" {
		path = strings.TrimSpace(ref.ID)
	}
	if path == "" {
		return nil, fmt.Errorf("file reference missing path")
	}
	return sc.EnsureFileLevel(path, desired)
}

// WorkingSetReferences returns the current tracked-file references without hydrating content.
func (sc *SharedContext) WorkingSetReferences() []ContextReference {
	if sc == nil || sc.workingSet == nil {
		return nil
	}
	files := sc.workingSet.List()
	refs := make([]ContextReference, 0, len(files))
	for _, fc := range files {
		if fc == nil {
			continue
		}
		ref, ok := sc.FileReference(fc.Path)
		if ok {
			refs = append(refs, ref)
		}
	}
	return refs
}

// EnsureFileLevel upgrades or downgrades a file to the desired detail level.
func (sc *SharedContext) EnsureFileLevel(path string, desired DetailLevel) (*FileContext, error) {
	fc, ok := sc.workingSet.Get(path)
	if !ok {
		return nil, fmt.Errorf("file %s not tracked", path)
	}
	if fc.Level <= desired {
		return fc, nil
	}
	switch desired {
	case DetailFull, DetailBodyOnly:
		content, err := ensureFileRawContent(path, fc)
		if err != nil {
			return nil, err
		}
		fc.Content = content
		fc.Level = DetailFull
	case DetailSignature:
		if fc.RawContent == "" {
			if _, err := ensureFileRawContent(path, fc); err != nil {
				return nil, err
			}
		}
		lines := linesAround(fc.RawContent, 40)
		fc.Content = lines
		fc.Level = DetailSignature
	default:
		if fc.RawContent == "" && fc.Content != "" {
			fc.RawContent = fc.Content
		}
		if sc.summarizer != nil && fc.RawContent != "" {
			if summary, err := sc.summarizer.Summarize(fc.RawContent, SummaryMinimal); err == nil {
				fc.Summary = summary
			}
		}
		fc.Content = ""
		fc.Level = DetailSummary
	}
	if item, ok := sc.fileItems[path]; ok {
		item.refresh()
	}
	return fc, nil
}

func detailLevelName(level DetailLevel) string {
	switch level {
	case DetailFull:
		return "full"
	case DetailBodyOnly:
		return "body"
	case DetailSignature:
		return "signature"
	case DetailSummary:
		return "summary"
	default:
		return "unknown"
	}
}

// TouchFile bumps the recency metadata to keep the file pinned in the working set.
func (sc *SharedContext) TouchFile(path string) {
	if fc, ok := sc.workingSet.Get(path); ok {
		fc.LastAccessed = time.Now().UTC()
	}
}

// DowngradeOldFiles aggressively compresses files older than the target.
func (sc *SharedContext) DowngradeOldFiles(target DetailLevel, maxTokens int) error {
	files := sc.workingSet.List()
	sort.Slice(files, func(i, j int) bool {
		return files[i].LastAccessed.Before(files[j].LastAccessed)
	})
	saved := 0
	for _, fc := range files {
		if fc.Pinned || fc.Level >= target {
			continue
		}
		if fc.RawContent == "" && fc.Content != "" {
			fc.RawContent = fc.Content
		}
		if sc.summarizer != nil && fc.RawContent != "" {
			if summary, err := sc.summarizer.Summarize(fc.RawContent, SummaryMinimal); err == nil {
				fc.Summary = summary
			}
		}
		fc.Content = ""
		fc.Level = target
		if item, ok := sc.fileItems[fc.Path]; ok {
			before := item.cachedTokens
			item.refresh()
			saved += before - item.cachedTokens
		}
		if maxTokens > 0 && saved >= maxTokens {
			break
		}
	}
	if sc.contextBudget != nil && saved > 0 {
		sc.contextBudget.Free("immediate", saved, "")
	}
	return nil
}

// GetConversationSummary returns the cached history summary.
func (sc *SharedContext) GetConversationSummary() string {
	sc.mu.RLock()
	defer sc.mu.RUnlock()
	return sc.conversationSummary
}

// GetChangeLogSummary returns a coarse summary of recent modifications.
func (sc *SharedContext) GetChangeLogSummary() string {
	sc.mu.RLock()
	defer sc.mu.RUnlock()
	if sc.changeLogSummary != "" {
		return sc.changeLogSummary
	}
	return sc.conversationSummary
}

// SetChangeLogSummary lets external workflows store a higher fidelity change log.
func (sc *SharedContext) SetChangeLogSummary(summary string) {
	sc.mu.Lock()
	defer sc.mu.Unlock()
	sc.changeLogSummary = summary
}

// RefreshConversationSummary rebuilds the summary using either the latest
// compressed chunk or a fresh summarization of recent history.
func (sc *SharedContext) RefreshConversationSummary() {
	sc.mu.Lock()
	defer sc.mu.Unlock()
	if len(sc.compressedHistory) > 0 {
		latest := sc.compressedHistory[len(sc.compressedHistory)-1]
		sc.conversationSummary = latest.Summary
		return
	}
	if sc.summarizer != nil && len(sc.history) > 0 {
		var builder strings.Builder
		for _, interaction := range sc.history {
			builder.WriteString(fmt.Sprintf("[%s] %s\n", interaction.Role, interaction.Content))
		}
		if summary, err := sc.summarizer.Summarize(builder.String(), SummaryConcise); err == nil {
			sc.conversationSummary = summary
			return
		}
	}
	sc.conversationSummary = ""
}

// GetTokenUsage estimates tokens consumed by files and history.
func (sc *SharedContext) GetTokenUsage() *ContextTokenUsage {
	files := sc.workingSet.List()
	usage := &ContextTokenUsage{
		BySection: make(map[string]int),
	}
	fileTokens := 0
	for _, fc := range files {
		fileTokens += fc.tokens()
	}
	historyTokens := 0
	for _, interaction := range sc.history {
		historyTokens += estimateTokens(interaction.Content)
	}
	usage.BySection["files"] = fileTokens
	usage.BySection["history"] = historyTokens
	usage.Total = fileTokens + historyTokens
	return usage
}

// OnBudgetWarning responds to context budget pressure by downgrading older
// files to summaries so higher-priority snippets can fit in the prompt.
func (sc *SharedContext) OnBudgetWarning(_ float64) {
	_ = sc.DowngradeOldFiles(DetailSummary, 256)
}

// OnBudgetExceeded satisfies the BudgetListener interface; no-op.
func (sc *SharedContext) OnBudgetExceeded(string, int, int) {}

// OnCompression records that compression happened; no-op for shared context.
func (sc *SharedContext) OnCompression(string, int) {}

// RecordMutation logs a state mutation to the mutation history buffer.
// Maintains a capped ring buffer (default 100 entries).
func (sc *SharedContext) RecordMutation(key, operation, agentID string, derivation *DerivationChain) {
	if sc == nil {
		return
	}

	sc.mu.Lock()
	defer sc.mu.Unlock()

	mutation := StateMutation{
		Key:        key,
		Operation:  operation,
		AgentID:    agentID,
		Timestamp:  time.Now().UTC(),
		Derivation: derivation,
	}

	// Maintain capped ring buffer (max 100 entries)
	const maxMutations = 100
	if len(sc.mutationLog) >= maxMutations {
		// Remove oldest entry
		sc.mutationLog = sc.mutationLog[1:]
	}
	sc.mutationLog = append(sc.mutationLog, mutation)
}

// MutationHistory returns all mutations for a given key.
func (sc *SharedContext) MutationHistory(key string) []StateMutation {
	if sc == nil {
		return nil
	}

	sc.mu.RLock()
	defer sc.mu.RUnlock()

	var result []StateMutation
	for _, mutation := range sc.mutationLog {
		if mutation.Key == key {
			result = append(result, mutation)
		}
	}
	return result
}

// RecentMutations returns the last N mutations across all keys.
func (sc *SharedContext) RecentMutations(limit int) []StateMutation {
	if sc == nil || limit <= 0 {
		return nil
	}

	sc.mu.RLock()
	defer sc.mu.RUnlock()

	if limit > len(sc.mutationLog) {
		limit = len(sc.mutationLog)
	}

	// Return the most recent 'limit' entries
	result := make([]StateMutation, limit)
	copy(result, sc.mutationLog[len(sc.mutationLog)-limit:])
	return result
}
