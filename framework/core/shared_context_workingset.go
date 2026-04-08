package core

import (
	"fmt"
	"os"
	"sort"
	"sync"
	"time"
)

// DetailLevel controls how much of a file is retained inside the working set.
type DetailLevel int

const (
	DetailFull DetailLevel = iota
	DetailBodyOnly
	DetailSignature
	DetailSummary
)

// FileContext keeps metadata about a tracked file or snippet.
type FileContext struct {
	Path         string
	Language     string
	Content      string
	RawContent   string
	Summary      string
	Level        DetailLevel
	LastAccessed time.Time
	Version      string
	Pinned       bool
}

func (fc *FileContext) tokens() int {
	if fc.Content != "" && fc.Level <= DetailBodyOnly {
		return estimateCodeTokens(fc.Content)
	}
	return estimateTokens(fc.getSummaryFallback())
}

func (fc *FileContext) getSummaryFallback() string {
	if fc.Summary != "" {
		return fc.Summary
	}
	if fc.Content != "" {
		if len(fc.Content) > 256 {
			return fc.Content[:256]
		}
		return fc.Content
	}
	return ""
}

// EvictionPolicy dictates how the working set ejects files.
type EvictionPolicy int

const (
	EvictLRU EvictionPolicy = iota
	EvictByRelevance
)

// EvictionRecord tracks a file eviction event.
type EvictionRecord struct {
	Path          string      `json:"path"`
	EvictedAt     time.Time   `json:"evicted_at"`
	EvictionScore float64     `json:"eviction_score"`
	DetailLevel   DetailLevel `json:"detail_level"`
}

// WorkingSet retains the active file contexts for an agent session.
type WorkingSet struct {
	mu             sync.RWMutex
	files          map[string]*FileContext
	maxSize        int
	evictionPolicy EvictionPolicy
	evictionLog    []EvictionRecord
}

// NewWorkingSet returns a working set with a bounded capacity.
func NewWorkingSet(maxSize int, policy EvictionPolicy) *WorkingSet {
	if maxSize <= 0 {
		maxSize = 12
	}
	return &WorkingSet{
		files:          make(map[string]*FileContext),
		maxSize:        maxSize,
		evictionPolicy: policy,
		evictionLog:    make([]EvictionRecord, 0, 64),
	}
}

// Add registers a file and evicts older entries if necessary.
func (ws *WorkingSet) Add(fc *FileContext) (evicted []string) {
	ws.mu.Lock()
	defer ws.mu.Unlock()
	ws.files[fc.Path] = fc
	if len(ws.files) <= ws.maxSize {
		return nil
	}
	return ws.evictLocked(len(ws.files) - ws.maxSize)
}

// Get retrieves a file context if present.
func (ws *WorkingSet) Get(path string) (*FileContext, bool) {
	ws.mu.RLock()
	defer ws.mu.RUnlock()
	fc, ok := ws.files[path]
	return fc, ok
}

// List returns copies of tracked files.
func (ws *WorkingSet) List() []*FileContext {
	ws.mu.RLock()
	defer ws.mu.RUnlock()
	out := make([]*FileContext, 0, len(ws.files))
	for _, fc := range ws.files {
		out = append(out, fc)
	}
	return out
}

// Remove deletes a file from the working set.
func (ws *WorkingSet) Remove(path string) {
	ws.mu.Lock()
	defer ws.mu.Unlock()
	delete(ws.files, path)
}

// SetMaxSize updates the capacity and evicts extra files immediately.
func (ws *WorkingSet) SetMaxSize(max int) []string {
	ws.mu.Lock()
	defer ws.mu.Unlock()
	if max <= 0 {
		max = 12
	}
	ws.maxSize = max
	if len(ws.files) <= ws.maxSize {
		return nil
	}
	return ws.evictLocked(len(ws.files) - ws.maxSize)
}

// EvictIfNeeded enforces the current limit without inserting new files.
func (ws *WorkingSet) EvictIfNeeded() []string {
	ws.mu.Lock()
	defer ws.mu.Unlock()
	if len(ws.files) <= ws.maxSize {
		return nil
	}
	return ws.evictLocked(len(ws.files) - ws.maxSize)
}

func (ws *WorkingSet) evictLocked(count int) []string {
	if count <= 0 {
		return nil
	}
	candidates := make([]*FileContext, 0, len(ws.files))
	for _, fc := range ws.files {
		if fc.Pinned {
			continue
		}
		candidates = append(candidates, fc)
	}
	if len(candidates) == 0 {
		return nil
	}
	switch ws.evictionPolicy {
	case EvictByRelevance:
		sort.Slice(candidates, func(i, j int) bool {
			return candidates[i].LastAccessed.Before(candidates[j].LastAccessed)
		})
	default:
		sort.Slice(candidates, func(i, j int) bool {
			return candidates[i].LastAccessed.Before(candidates[j].LastAccessed)
		})
	}
	evicted := make([]string, 0, count)
	for _, fc := range candidates {
		if count == 0 {
			break
		}
		record := EvictionRecord{
			Path:          fc.Path,
			EvictedAt:     time.Now().UTC(),
			EvictionScore: 0.0,
			DetailLevel:   fc.Level,
		}
		if len(ws.evictionLog) >= 64 {
			ws.evictionLog = ws.evictionLog[1:]
		}
		ws.evictionLog = append(ws.evictionLog, record)
		delete(ws.files, fc.Path)
		evicted = append(evicted, fc.Path)
		count--
	}
	return evicted
}

// EvictionHistory returns a copy of the eviction log.
func (ws *WorkingSet) EvictionHistory() []EvictionRecord {
	ws.mu.RLock()
	defer ws.mu.RUnlock()
	if len(ws.evictionLog) == 0 {
		return nil
	}
	history := make([]EvictionRecord, len(ws.evictionLog))
	copy(history, ws.evictionLog)
	return history
}

// ContextTokenUsage exposes aggregated token consumption by category.
type ContextTokenUsage struct {
	Total     int
	BySection map[string]int
}

// StateMutation records a mutation to shared context state.
type StateMutation struct {
	Key        string           `json:"key"`
	Operation  string           `json:"operation"` // "set", "delete", "merge_overwrite"
	AgentID    string           `json:"agent_id"`
	Timestamp  time.Time        `json:"timestamp"`
	Derivation *DerivationChain `json:"derivation,omitempty"`
}

func ensureFileRawContent(path string, fc *FileContext) (string, error) {
	if fc == nil {
		return "", os.ErrNotExist
	}
	info, err := os.Stat(path)
	if err != nil {
		if fc.RawContent != "" {
			return fc.RawContent, nil
		}
		return "", err
	}
	version := fmt.Sprintf("%d:%d", info.ModTime().UTC().UnixNano(), info.Size())
	if fc.RawContent != "" && fc.Version == version {
		return fc.RawContent, nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if fc.RawContent != "" {
			return fc.RawContent, nil
		}
		return "", err
	}
	fc.RawContent = string(data)
	fc.Version = version
	return fc.RawContent, nil
}

func linesAround(content string, maxLines int) string {
	if maxLines <= 0 {
		maxLines = 20
	}
	lines := splitLines(content)
	if len(lines) <= maxLines {
		return content
	}
	return joinLines(lines[:maxLines])
}

func splitLines(content string) []string {
	if content == "" {
		return nil
	}
	lines := make([]string, 0, 8)
	start := 0
	for i, ch := range content {
		if ch != '\n' {
			continue
		}
		lines = append(lines, content[start:i])
		start = i + 1
	}
	if start < len(content) {
		lines = append(lines, content[start:])
	}
	return lines
}

func joinLines(lines []string) string {
	if len(lines) == 0 {
		return ""
	}
	out := lines[0]
	for _, line := range lines[1:] {
		out += "\n" + line
	}
	return out
}

type fileBudgetItem struct {
	file         *FileContext
	summarizer   Summarizer
	cachedID     string
	cachedTokens int
}

func newFileBudgetItem(fc *FileContext, summarizer Summarizer) *fileBudgetItem {
	item := &fileBudgetItem{
		file:       fc,
		summarizer: summarizer,
		cachedID:   fc.Path,
	}
	item.refresh()
	return item
}

func (f *fileBudgetItem) refresh() {
	if f == nil || f.file == nil {
		return
	}
	f.cachedTokens = f.file.tokens()
	f.file.LastAccessed = time.Now().UTC()
}

func (f *fileBudgetItem) GetID() string {
	return f.cachedID
}

func (f *fileBudgetItem) GetTokenCount() int {
	if f == nil {
		return 0
	}
	return f.cachedTokens
}

func (f *fileBudgetItem) GetPriority() int {
	if f == nil || f.file == nil {
		return 0
	}
	recency := int(time.Since(f.file.LastAccessed).Minutes())
	if recency < 0 {
		recency = 0
	}
	return recency + int(f.file.Level)*10
}

func (f *fileBudgetItem) CanCompress() bool {
	return f != nil && f.file != nil && f.file.Level < DetailSummary && !f.file.Pinned
}

func (f *fileBudgetItem) Compress() (BudgetItem, error) {
	if !f.CanCompress() {
		return f, nil
	}
	if f.summarizer != nil && f.file.Content != "" {
		if summary, err := f.summarizer.Summarize(f.file.Content, SummaryMinimal); err == nil {
			f.file.Summary = summary
		}
	}
	f.file.Content = ""
	f.file.Level = DetailSummary
	f.refresh()
	return f, nil
}

func (f *fileBudgetItem) CanEvict() bool {
	return f != nil && f.file != nil && !f.file.Pinned
}
