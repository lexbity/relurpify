package core

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

type WorkingSetReference struct {
	ID       string      `json:"id,omitempty" yaml:"id,omitempty"`
	URI      string      `json:"uri,omitempty" yaml:"uri,omitempty"`
	Kind     string      `json:"kind,omitempty" yaml:"kind,omitempty"`
	Language string      `json:"language,omitempty" yaml:"language,omitempty"`
	Level    DetailLevel `json:"level,omitempty" yaml:"level,omitempty"`
	Summary  string      `json:"summary,omitempty" yaml:"summary,omitempty"`
}

type MutationRecord struct {
	Key       string         `json:"key,omitempty" yaml:"key,omitempty"`
	Action    string         `json:"action,omitempty" yaml:"action,omitempty"`
	Actor     string         `json:"actor,omitempty" yaml:"actor,omitempty"`
	Metadata  map[string]any `json:"metadata,omitempty" yaml:"metadata,omitempty"`
	Timestamp time.Time      `json:"timestamp,omitempty" yaml:"timestamp,omitempty"`
}

type SharedContextTokenUsage struct {
	Total     int            `json:"total,omitempty" yaml:"total,omitempty"`
	BySection map[string]int `json:"by_section,omitempty" yaml:"by_section,omitempty"`
}

type SharedContext struct {
	Context    *Context
	Budget     *ArtifactBudget
	Summarizer Summarizer

	mu                  sync.RWMutex
	files               map[string]*FileContext
	workingSet          map[string]WorkingSetReference
	mutations           []MutationRecord
	conversationSummary string
}

func NewSharedContext(ctx *Context, budget *ArtifactBudget, summarizer Summarizer) *SharedContext {
	if ctx == nil {
		ctx = NewContext()
	}
	return &SharedContext{
		Context:    ctx,
		Budget:     budget,
		Summarizer: summarizer,
		files:      map[string]*FileContext{},
		workingSet: map[string]WorkingSetReference{},
	}
}

func (s *SharedContext) AddFile(path, content, language string, level DetailLevel) (*FileContext, error) {
	if s == nil {
		return nil, fmt.Errorf("shared context required")
	}
	path = filepath.Clean(strings.TrimSpace(path))
	if path == "" {
		return nil, fmt.Errorf("file path required")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now().UTC()
	raw := content
	summary := ""
	if s.Summarizer != nil && (level == DetailSummary || level == DetailMinimal || level == DetailMetadata || level == DetailBodyOnly) {
		if text, err := s.Summarizer.Summarize(content, SummaryConcise); err == nil {
			summary = text
		}
	}
	if summary == "" && content != "" {
		summary = truncateParagraph(content, 160)
	}
	fc := &FileContext{
		Path:       path,
		Language:   strings.TrimSpace(language),
		Level:      level,
		RawContent: raw,
		Summary:    summary,
		UpdatedAt:  now,
	}
	switch level {
	case DetailFull, DetailBodyOnly:
		fc.Content = raw
	case DetailSummary:
		fc.Content = summary
	case DetailMetadata, DetailMinimal:
		fc.Content = ""
	default:
		fc.Content = raw
	}
	if fc.Metadata == nil {
		fc.Metadata = map[string]any{}
	}
	fc.Metadata["path"] = path
	fc.Metadata["language"] = fc.Language
	s.files[path] = fc
	s.workingSet[path] = WorkingSetReference{
		ID:       path,
		URI:      path,
		Kind:     "file",
		Language: fc.Language,
		Level:    fc.Level,
		Summary:  fc.Summary,
	}
	return fc, nil
}

func (s *SharedContext) GetFile(path string) (*FileContext, bool) {
	if s == nil {
		return nil, false
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	fc, ok := s.files[filepath.Clean(strings.TrimSpace(path))]
	return fc, ok
}

func (s *SharedContext) OnBudgetWarning(_ float64) {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, fc := range s.files {
		if fc == nil || fc.Level != DetailFull {
			continue
		}
		fc.Level = DetailSummary
		fc.Content = ""
		fc.UpdatedAt = time.Now().UTC()
		if fc.Summary == "" && s.Summarizer != nil {
			if text, err := s.Summarizer.Summarize(fc.RawContent, SummaryConcise); err == nil {
				fc.Summary = text
			}
		}
		if ref, ok := s.workingSet[fc.Path]; ok {
			ref.Level = fc.Level
			ref.Summary = fc.Summary
			s.workingSet[fc.Path] = ref
		}
	}
}

func (s *SharedContext) OnBudgetExceeded(_ string, _, _ int) {}

func (s *SharedContext) OnCompression(_ string, _ int) {}

func (s *SharedContext) RefreshConversationSummary() {
	if s == nil {
		return
	}
	history := s.Context.History()
	lines := make([]string, 0, len(history))
	for _, item := range history {
		if text := strings.TrimSpace(item.Content); text != "" {
			lines = append(lines, fmt.Sprintf("%s: %s", item.Role, text))
		}
	}
	raw := strings.Join(lines, "\n")
	if s.Summarizer != nil {
		if summary, err := s.Summarizer.Summarize(raw, SummaryConcise); err == nil {
			raw = summary
		}
	}
	s.mu.Lock()
	s.conversationSummary = strings.TrimSpace(raw)
	s.mu.Unlock()
}

func (s *SharedContext) GetConversationSummary() string {
	if s == nil {
		return ""
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.conversationSummary
}

func (s *SharedContext) DowngradeOldFiles(level DetailLevel, keep int) error {
	if s == nil {
		return nil
	}
	if keep < 0 {
		keep = 0
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.files) == 0 {
		return nil
	}
	type entry struct {
		path string
		fc   *FileContext
	}
	entries := make([]entry, 0, len(s.files))
	for path, fc := range s.files {
		entries = append(entries, entry{path: path, fc: fc})
	}
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].fc.UpdatedAt.After(entries[j].fc.UpdatedAt)
	})
	for idx, item := range entries {
		if idx < keep || item.fc == nil {
			continue
		}
		item.fc.Level = level
		item.fc.Content = ""
		item.fc.UpdatedAt = time.Now().UTC()
		if ref, ok := s.workingSet[item.path]; ok {
			ref.Level = level
			s.workingSet[item.path] = ref
		}
	}
	return nil
}

func (s *SharedContext) EnsureFileLevel(path string, level DetailLevel) (*FileContext, error) {
	if s == nil {
		return nil, fmt.Errorf("shared context required")
	}
	path = filepath.Clean(strings.TrimSpace(path))
	s.mu.Lock()
	defer s.mu.Unlock()
	fc, ok := s.files[path]
	if !ok || fc == nil {
		return nil, fmt.Errorf("file %s not found", path)
	}
	switch level {
	case DetailFull:
		if fc.RawContent != "" {
			fc.Content = fc.RawContent
		} else if fc.Content == "" {
			fc.Content = fc.Summary
		}
		fc.Level = DetailFull
	case DetailSummary:
		if fc.Summary == "" && fc.RawContent != "" && s.Summarizer != nil {
			if text, err := s.Summarizer.Summarize(fc.RawContent, SummaryConcise); err == nil {
				fc.Summary = text
			}
		}
		fc.Content = fc.Summary
		fc.Level = DetailSummary
	case DetailBodyOnly:
		if fc.RawContent != "" {
			fc.Content = fc.RawContent
		}
		fc.Level = DetailBodyOnly
	default:
		fc.Level = level
	}
	fc.UpdatedAt = time.Now().UTC()
	if ref, ok := s.workingSet[path]; ok {
		ref.Level = fc.Level
		ref.Summary = fc.Summary
		s.workingSet[path] = ref
	}
	return fc, nil
}

func (s *SharedContext) RecordMutation(key, action, actor string, metadata map[string]any) {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	rec := MutationRecord{
		Key:       strings.TrimSpace(key),
		Action:    strings.TrimSpace(action),
		Actor:     strings.TrimSpace(actor),
		Metadata:  cloneAnyMap(metadata),
		Timestamp: time.Now().UTC(),
	}
	s.mutations = append(s.mutations, rec)
}

func (s *SharedContext) RecentMutations(limit int) []MutationRecord {
	if s == nil {
		return nil
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	if limit <= 0 || len(s.mutations) == 0 {
		return nil
	}
	start := len(s.mutations) - limit
	if start < 0 {
		start = 0
	}
	out := make([]MutationRecord, len(s.mutations[start:]))
	copy(out, s.mutations[start:])
	return out
}

func (s *SharedContext) WorkingSetReferences() []WorkingSetReference {
	if s == nil {
		return nil
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	keys := make([]string, 0, len(s.workingSet))
	for key := range s.workingSet {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	out := make([]WorkingSetReference, 0, len(keys))
	for _, key := range keys {
		out = append(out, s.workingSet[key])
	}
	return out
}

func (s *SharedContext) GetTokenUsage() *SharedContextTokenUsage {
	if s == nil {
		return nil
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	usage := &SharedContextTokenUsage{BySection: map[string]int{}}
	for _, fc := range s.files {
		if fc == nil {
			continue
		}
		tokens := estimateTokens(firstNonEmpty(fc.Content, fc.RawContent, fc.Summary))
		usage.BySection["files"] += tokens
		usage.Total += tokens
	}
	historyTokens := estimateTokens(s.Context.History())
	usage.BySection["history"] = historyTokens
	usage.Total += historyTokens
	mutationTokens := 0
	for _, mutation := range s.mutations {
		mutationTokens += estimateTokens(mutation.Key + " " + mutation.Action + " " + mutation.Actor)
	}
	usage.BySection["mutations"] = mutationTokens
	usage.Total += mutationTokens
	return usage
}
