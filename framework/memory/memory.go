// Package memory provides a hybrid in-memory and disk-backed storage system for agents.
// It supports session, project, and global scopes with Remember, Recall, Search, Forget,
// and Summarise operations, persisting items as JSON files for durability across restarts.
package memory

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
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

// HybridMemory combines in-memory caching with JSON persistence on disk. The
// design keeps session data transient (great for experiments) while persisting
// project/global scopes across runs for longer-term recall.
type HybridMemory struct {
	mu          sync.RWMutex
	cache       map[MemoryScope]map[string]MemoryRecord
	basePath    string
	vectorStore VectorStore
}

// NewHybridMemory creates a new memory store.
func NewHybridMemory(basePath string) (*HybridMemory, error) {
	if basePath == "" {
		basePath = ".memory"
	}
	if err := os.MkdirAll(basePath, 0o755); err != nil {
		return nil, err
	}
	store := &HybridMemory{
		cache: map[MemoryScope]map[string]MemoryRecord{
			MemoryScopeSession: {},
			MemoryScopeProject: {},
			MemoryScopeGlobal:  {},
		},
		basePath: basePath,
	}
	if err := store.loadFromDisk(); err != nil {
		return nil, err
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

// loadFromDisk hydrates the in-memory cache from JSON files previously written
// to disk. Missing files are ignored so the store can start empty on first run.
func (m *HybridMemory) loadFromDisk() error {
	for scope := range m.cache {
		path := m.scopePath(scope)
		data, err := os.ReadFile(path)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				continue
			}
			return err
		}
		var records []MemoryRecord
		if err := json.Unmarshal(data, &records); err != nil {
			return err
		}
		for _, r := range records {
			m.cache[scope][r.Key] = r
		}
	}
	if m.vectorStore != nil {
		m.warmVectorStore(context.Background(), m.vectorStore, m.snapshot())
	}
	return nil
}

// persist writes the cached records for a scope back to disk so that project
// and global memories survive process restarts.
func (m *HybridMemory) persist(scope MemoryScope) error {
	records := make([]MemoryRecord, 0, len(m.cache[scope]))
	for _, r := range m.cache[scope] {
		records = append(records, r)
	}
	data, err := json.MarshalIndent(records, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(m.scopePath(scope), data, 0o644)
}

// scopePath resolves the JSON file associated with a scope so all persistence
// logic shares the same directory layout.
func (m *HybridMemory) scopePath(scope MemoryScope) string {
	filename := string(scope) + ".json"
	return filepath.Join(m.basePath, filename)
}

// Remember stores data for a given scope. Session-scoped memories stay in RAM
// to avoid excessive disk churn during fast agent loops, while project/global
// scopes are flushed to JSON for durability.
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
	if scope == MemoryScopeSession {
		m.mu.Unlock()
		if vectorStore != nil {
			if err := vectorStore.Upsert(ctx, memoryDocument(record)); err != nil {
				return err
			}
		}
		return nil
	}
	if err := m.persist(scope); err != nil {
		m.mu.Unlock()
		return err
	}
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

// Search executes a naive semantic search by substring match. It is purposely
// simple so that the memory subsystem feels deterministic and debuggable; you
// can later replace it with a vector store without touching agent code.
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
	if scope == MemoryScopeSession {
		m.mu.Unlock()
		if vectorStore != nil {
			if err := vectorStore.Delete(ctx, memoryDocumentID(scope, key)); err != nil {
				return err
			}
		}
		return nil
	}
	if err := m.persist(scope); err != nil {
		m.mu.Unlock()
		return err
	}
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

func (m *HybridMemory) warmVectorStore(ctx context.Context, vs VectorStore, records []MemoryRecord) {
	for _, record := range records {
		_ = vs.Upsert(ctx, memoryDocument(record))
	}
}

func memoryDocument(record MemoryRecord) Document {
	return Document{
		ID:      memoryDocumentID(record.Scope, record.Key),
		Content: record.Key + " " + flattenValue(record.Value),
		Metadata: map[string]interface{}{
			"key":   record.Key,
			"scope": string(record.Scope),
		},
	}
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

func flattenValue(value map[string]interface{}) string {
	data, err := json.Marshal(value)
	if err != nil {
		return ""
	}
	return string(data)
}
