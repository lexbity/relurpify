// Package memory provides an in-memory scratchpad for agents.
// It supports session, project, and global scopes with Remember, Recall, Search,
// Forget, and Summarise operations.
package memory

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"
)

// MemoryScope determines where data is persisted.
type MemoryScope string

const (
	MemoryScopeSession MemoryScope = "session"
	MemoryScopeProject MemoryScope = "project"
	MemoryScopeGlobal  MemoryScope = "global"
)

// MemoryRecord represents a stored memory item. Value is intentionally
// unstructured JSON so agents can stash anything from LLM responses to plan
// summaries without evolving the schema.
type MemoryRecord struct {
	Key       string                 `json:"key"`
	Value     map[string]interface{} `json:"value"`
	Scope     MemoryScope            `json:"scope"`
	Timestamp time.Time              `json:"timestamp"`
	Tags      []string               `json:"tags,omitempty"`
}

// MemoryStore describes the memory system operations.
type MemoryStore interface {
	Remember(ctx context.Context, key string, value map[string]interface{}, scope MemoryScope) error
	Recall(ctx context.Context, key string, scope MemoryScope) (*MemoryRecord, bool, error)
	Search(ctx context.Context, query string, scope MemoryScope) ([]MemoryRecord, error)
	Forget(ctx context.Context, key string, scope MemoryScope) error
	Summarize(ctx context.Context, scope MemoryScope) (string, error)
}

// HybridMemory combines in-memory caching with optional semantic indexing.
type HybridMemory struct {
	mu          sync.RWMutex
	cache       map[MemoryScope]map[string]MemoryRecord
	vectorStore VectorStore
}

// NewHybridMemory creates a new transient memory store.
func NewHybridMemory(basePath string) (*HybridMemory, error) {
	store := &HybridMemory{
		cache: map[MemoryScope]map[string]MemoryRecord{
			MemoryScopeSession: {},
			MemoryScopeProject: {},
			MemoryScopeGlobal:  {},
		},
	}
	return store, nil
}

// WithVectorStore enables semantic indexing for remembered values while keeping
// the nil default safe for existing callers.
func (m *HybridMemory) WithVectorStore(vs VectorStore) *HybridMemory {
	if m == nil {
		return nil
	}
	m.mu.Lock()
	m.vectorStore = vs
	records := m.snapshotLocked()
	m.mu.Unlock()
	if vs != nil {
		m.warmVectorStore(context.Background(), vs, records)
	}
	return m
}

// Remember stores data for a given scope.
func (m *HybridMemory) Remember(ctx context.Context, key string, value map[string]interface{}, scope MemoryScope) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}
	m.mu.Lock()
	record := MemoryRecord{
		Key:       key,
		Value:     value,
		Scope:     scope,
		Timestamp: time.Now().UTC(),
	}
	m.cache[scope][key] = record
	vectorStore := m.vectorStore
	m.mu.Unlock()
	if vectorStore != nil {
		if err := vectorStore.Upsert(ctx, memoryDocument(record)); err != nil {
			return err
		}
	}
	return nil
}

// Recall retrieves a memory record.
func (m *HybridMemory) Recall(ctx context.Context, key string, scope MemoryScope) (*MemoryRecord, bool, error) {
	select {
	case <-ctx.Done():
		return nil, false, ctx.Err()
	default:
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	record, ok := m.cache[scope][key]
	if !ok {
		return nil, false, nil
	}
	return &record, true, nil
}

// Search executes a naive semantic search by substring match.
func (m *HybridMemory) Search(ctx context.Context, query string, scope MemoryScope) ([]MemoryRecord, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}
	m.mu.RLock()
	vectorStore := m.vectorStore
	m.mu.RUnlock()
	if vectorStore != nil {
		vectorResults, err := vectorStore.Query(ctx, query, 10)
		if err != nil {
			return nil, err
		}
		if len(vectorResults) > 0 {
			results := m.recordsForVectorResults(scope, vectorResults)
			if len(results) > 0 {
				return results, nil
			}
		}
	}
	lower := strings.ToLower(query)
	m.mu.RLock()
	defer m.mu.RUnlock()

	var results []MemoryRecord
	for _, record := range m.cache[scope] {
		data, _ := json.Marshal(record.Value)
		if strings.Contains(strings.ToLower(string(data)), lower) {
			results = append(results, record)
		}
	}
	return results, nil
}

// Forget removes a stored memory entry.
func (m *HybridMemory) Forget(ctx context.Context, key string, scope MemoryScope) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}
	m.mu.Lock()
	delete(m.cache[scope], key)
	vectorStore := m.vectorStore
	m.mu.Unlock()
	if vectorStore != nil {
		if err := vectorStore.Delete(ctx, memoryDocumentID(scope, key)); err != nil {
			return err
		}
	}
	return nil
}

// KnowledgeStore is an interface for storing and retrieving knowledge items
// such as patterns, tensions, decisions, and interactions.
type KnowledgeStore interface {
	// StoreKnowledge stores a knowledge item
	StoreKnowledge(ctx context.Context, item KnowledgeItem) error
	// RetrieveKnowledge retrieves knowledge items matching the query
	RetrieveKnowledge(ctx context.Context, query KnowledgeQuery) ([]KnowledgeItem, error)
	// DeleteKnowledge removes a knowledge item by ID
	DeleteKnowledge(ctx context.Context, id string) error
}

// KnowledgeItem represents a single piece of knowledge in the store
type KnowledgeItem struct {
	ID        string                 `json:"id"`
	Kind      string                 `json:"kind"` // pattern, tension, decision, interaction
	Title     string                 `json:"title"`
	Content   map[string]interface{} `json:"content"`
	Metadata  map[string]interface{} `json:"metadata"`
	CreatedAt time.Time              `json:"created_at"`
	UpdatedAt time.Time              `json:"updated_at"`
}

// KnowledgeQuery defines parameters for querying knowledge items
type KnowledgeQuery struct {
	Kind      string   `json:"kind,omitempty"`
	TextQuery string   `json:"text_query,omitempty"`
	Tags      []string `json:"tags,omitempty"`
	Limit     int      `json:"limit,omitempty"`
	Offset    int      `json:"offset,omitempty"`
}

// NewInMemoryKnowledgeStore creates a simple in-memory knowledge store
func NewInMemoryKnowledgeStore() KnowledgeStore {
	return &inMemoryKnowledgeStore{
		items: make(map[string]KnowledgeItem),
	}
}

type inMemoryKnowledgeStore struct {
	mu    sync.RWMutex
	items map[string]KnowledgeItem
}

func (s *inMemoryKnowledgeStore) StoreKnowledge(ctx context.Context, item KnowledgeItem) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if item.ID == "" {
		item.ID = fmt.Sprintf("knowledge_%d", time.Now().UnixNano())
	}
	if item.CreatedAt.IsZero() {
		item.CreatedAt = time.Now().UTC()
	}
	item.UpdatedAt = time.Now().UTC()

	s.items[item.ID] = item
	return nil
}

func (s *inMemoryKnowledgeStore) RetrieveKnowledge(ctx context.Context, query KnowledgeQuery) ([]KnowledgeItem, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var results []KnowledgeItem
	for _, item := range s.items {
		// Simple filtering by kind
		if query.Kind != "" && item.Kind != query.Kind {
			continue
		}
		// Simple text search in title
		if query.TextQuery != "" {
			if !strings.Contains(strings.ToLower(item.Title), strings.ToLower(query.TextQuery)) {
				continue
			}
		}
		results = append(results, item)

		if query.Limit > 0 && len(results) >= query.Limit {
			break
		}
	}
	return results, nil
}

func (s *inMemoryKnowledgeStore) DeleteKnowledge(ctx context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	delete(s.items, id)
	return nil
}

// Summarize compresses older records into a textual summary. Teams often call
// this before persisting workflows so they can log “what just happened” without
// storing entire transcripts.
func (m *HybridMemory) Summarize(ctx context.Context, scope MemoryScope) (string, error) {
	select {
	case <-ctx.Done():
		return "", ctx.Err()
	default:
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	var builder strings.Builder
	builder.WriteString("Summary for scope ")
	builder.WriteString(string(scope))
	builder.WriteString(":\n")
	for _, record := range m.cache[scope] {
		builder.WriteString("- ")
		builder.WriteString(record.Key)
		builder.WriteString(": ")
		data, _ := json.Marshal(record.Value)
		builder.Write(data)
		builder.WriteRune('\n')
	}
	return builder.String(), nil
}

func (m *HybridMemory) recordsForVectorResults(scope MemoryScope, vectorResults []SearchResult) []MemoryRecord {
	m.mu.RLock()
	defer m.mu.RUnlock()
	results := make([]MemoryRecord, 0, len(vectorResults))
	for _, result := range vectorResults {
		key, resultScope, ok := memoryDocumentRef(result.Document)
		if !ok || resultScope != scope {
			continue
		}
		record, exists := m.cache[scope][key]
		if !exists {
			continue
		}
		results = append(results, record)
	}
	return results
}

func (m *HybridMemory) snapshot() []MemoryRecord {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.snapshotLocked()
}

func (m *HybridMemory) snapshotLocked() []MemoryRecord {
	count := 0
	for _, scoped := range m.cache {
		count += len(scoped)
	}
	records := make([]MemoryRecord, 0, count)
	for _, scoped := range m.cache {
		for _, record := range scoped {
			records = append(records, record)
		}
	}
	return records
}

func memoryDocumentID(scope MemoryScope, key string) string {
	return fmt.Sprintf("%s:%s", scope, key)
}

func memoryDocumentRef(doc Document) (string, MemoryScope, bool) {
	if doc.Metadata != nil {
		key, keyOK := doc.Metadata["key"].(string)
		scopeRaw, scopeOK := doc.Metadata["scope"].(string)
		if keyOK && scopeOK {
			return key, MemoryScope(scopeRaw), true
		}
	}
	parts := strings.SplitN(doc.ID, ":", 2)
	if len(parts) != 2 {
		return "", "", false
	}
	return parts[1], MemoryScope(parts[0]), true
}
