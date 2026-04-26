package biknowledgecc

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"sync"
	"time"

	frameworkpatterns "codeburg.org/lexbit/relurpify/framework/patterns"
	frameworkretrieval "codeburg.org/lexbit/relurpify/framework/retrieval"
)

type CompilerInputKind string

const (
	CompilerInputPatternConfirmation CompilerInputKind = "pattern_confirmation"
	CompilerInputAnchorConfirmation  CompilerInputKind = "anchor_confirmation"
	CompilerInputASTIndexEntry       CompilerInputKind = "ast_index_entry"
	CompilerInputUserStatement       CompilerInputKind = "user_statement"
)

type PatternConfirmationInput struct {
	WorkspaceID     string
	WorkflowID      string
	Pattern         frameworkpatterns.PatternRecord
	BasedOnRevision string
	RelatedChunkIDs []ChunkID
	AmplifyChunkIDs []ChunkID
	SessionID       string
}

type AnchorConfirmationInput struct {
	WorkspaceID     string
	WorkflowID      string
	Anchor          frameworkretrieval.AnchorRecord
	RelatedChunkIDs []ChunkID
	AmplifyChunkIDs []ChunkID
	SessionID       string
}

type ASTIndexEntryInput struct {
	WorkspaceID     string
	WorkflowID      string
	EntryID         string
	FilePath        string
	SymbolID        string
	Summary         string
	Kind            string
	BasedOnRevision string
	RelatedChunkIDs []ChunkID
	AmplifyChunkIDs []ChunkID
	SessionID       string
}

type UserStatementInput struct {
	WorkspaceID     string
	WorkflowID      string
	Interaction     any
	Statement       string
	BasedOnRevision string
	AmplifyChunkIDs []ChunkID
	SessionID       string
}

type CompilerInput struct {
	Kind               CompilerInputKind         `json:"kind"`
	PatternConfirmed   *PatternConfirmationInput `json:"pattern_confirmed,omitempty"`
	AnchorConfirmed    *AnchorConfirmationInput  `json:"anchor_confirmed,omitempty"`
	IndexEntryProduced *ASTIndexEntryInput       `json:"index_entry_produced,omitempty"`
	UserStatement      *UserStatementInput       `json:"user_statement,omitempty"`
}

type CompileResult struct {
	ChunkIDs []ChunkID `json:"chunk_ids,omitempty"`
	EdgeIDs  []EdgeID  `json:"edge_ids,omitempty"`
}

// Compiler performs deterministic chunk compilation from knowledge records.
type Compiler struct {
	Store    *ChunkStore
	EventBus *EventBus
	Now      func() time.Time

	mu      sync.Mutex
	cancel  func()
	running bool
	modules []Module
}

// AttachModule registers a module hook for future expansion.
func (c *Compiler) AttachModule(module Module) {
	if c == nil || module == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.modules = append(c.modules, module)
	module.Attach(c)
}

func (c *Compiler) Compile(ctx context.Context, input CompilerInput) (*CompileResult, error) {
	switch input.Kind {
	case CompilerInputPatternConfirmation:
		if input.PatternConfirmed == nil {
			return nil, fmt.Errorf("biknowledgecc: pattern confirmation payload required")
		}
		return c.fromPatternConfirmation(*input.PatternConfirmed)
	case CompilerInputAnchorConfirmation:
		if input.AnchorConfirmed == nil {
			return nil, fmt.Errorf("biknowledgecc: anchor confirmation payload required")
		}
		return c.fromAnchorConfirmation(*input.AnchorConfirmed)
	case CompilerInputASTIndexEntry:
		if input.IndexEntryProduced == nil {
			return nil, fmt.Errorf("biknowledgecc: index entry payload required")
		}
		return c.fromASTIndexEntry(*input.IndexEntryProduced)
	case CompilerInputUserStatement:
		if input.UserStatement == nil {
			return nil, fmt.Errorf("biknowledgecc: user statement payload required")
		}
		return c.fromUserStatement(*input.UserStatement)
	default:
		return nil, fmt.Errorf("biknowledgecc: unknown compiler input kind %q", input.Kind)
	}
}

func (c *Compiler) Start(ctx context.Context) error {
	if c == nil || c.EventBus == nil {
		return nil
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.running {
		return nil
	}
	ch, unsub := c.EventBus.Subscribe(32)
	runCtx, cancel := context.WithCancel(ctx)
	c.cancel = func() {
		cancel()
		unsub()
	}
	c.running = true
	go func() {
		defer func() {
			c.mu.Lock()
			c.running = false
			c.cancel = nil
			c.mu.Unlock()
		}()
		for {
			select {
			case <-runCtx.Done():
				return
			case event, ok := <-ch:
				if !ok {
					return
				}
				input, ok := event.Payload.(CompilerInput)
				if !ok {
					continue
				}
				_, _ = c.Compile(runCtx, input)
			}
		}
	}()
	return nil
}

func (c *Compiler) Stop() error {
	if c == nil {
		return nil
	}
	c.mu.Lock()
	cancel := c.cancel
	c.cancel = nil
	c.running = false
	c.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	return nil
}

func (c *Compiler) fromPatternConfirmation(input PatternConfirmationInput) (*CompileResult, error) {
	record := input.Pattern
	if strings.TrimSpace(record.ID) == "" {
		return nil, fmt.Errorf("biknowledgecc: pattern id required")
	}
	bodyFields := map[string]any{
		"pattern_id":    record.ID,
		"kind":          record.Kind,
		"title":         record.Title,
		"description":   record.Description,
		"status":        record.Status,
		"instances":     record.Instances,
		"anchor_refs":   record.AnchorRefs,
		"corpus_scope":  record.CorpusScope,
		"corpus_source": record.CorpusSource,
		"confirmed_by":  record.ConfirmedBy,
		"confirmed_at":  record.ConfirmedAt,
		"comment_ids":   record.CommentIDs,
		"confidence":    record.Confidence,
		"superseded_by": record.SupersededBy,
	}
	raw, _ := json.Marshal(bodyFields)
	chunk := KnowledgeChunk{
		ID:            deterministicChunkID("pattern_confirmation", record.ID),
		WorkspaceID:   strings.TrimSpace(input.WorkspaceID),
		ContentHash:   hashStrings(record.ID, string(raw), input.BasedOnRevision),
		TokenEstimate: estimateTokens(string(raw)),
		Provenance: ChunkProvenance{
			Sources: []ProvenanceSource{{
				Kind: "pattern_confirmation",
				Ref:  record.ID,
			}},
			SessionID:    strings.TrimSpace(input.SessionID),
			WorkflowID:   strings.TrimSpace(input.WorkflowID),
			CodeStateRef: strings.TrimSpace(input.BasedOnRevision),
			CompiledBy:   CompilerDeterministic,
			Timestamp:    c.Now(),
		},
		Freshness: FreshnessValid,
		Body: ChunkBody{
			Raw:    string(raw),
			Fields: bodyFields,
		},
	}
	return c.saveCompiledChunk(chunk, input.RelatedChunkIDs, input.AmplifyChunkIDs, true)
}

func (c *Compiler) fromAnchorConfirmation(input AnchorConfirmationInput) (*CompileResult, error) {
	record := input.Anchor
	if strings.TrimSpace(record.AnchorID) == "" {
		return nil, fmt.Errorf("biknowledgecc: anchor id required")
	}
	bodyFields := map[string]any{
		"anchor_id":          record.AnchorID,
		"term":               record.Term,
		"definition":         record.Definition,
		"anchor_class":       record.AnchorClass,
		"trust_class":        record.TrustClass,
		"context_summary":    record.ContextSummary,
		"scope":              record.Scope,
		"source_chunk_id":    record.SourceChunkID,
		"source_version_id":  record.SourceVersionID,
		"source_doc_id":      record.SourceDocID,
		"corpus_scope":       record.CorpusScope,
		"policy_snapshot_id": record.PolicySnapshotID,
	}
	raw, _ := json.Marshal(bodyFields)
	chunk := KnowledgeChunk{
		ID:            deterministicChunkID("anchor_confirmation", record.AnchorID),
		WorkspaceID:   strings.TrimSpace(input.WorkspaceID),
		ContentHash:   hashStrings(record.AnchorID, string(raw), record.SourceVersionID),
		TokenEstimate: estimateTokens(string(raw)),
		Provenance: ChunkProvenance{
			Sources: []ProvenanceSource{{
				Kind: "anchor_confirmation",
				Ref:  record.AnchorID,
			}},
			SessionID:    strings.TrimSpace(input.SessionID),
			WorkflowID:   strings.TrimSpace(input.WorkflowID),
			CodeStateRef: strings.TrimSpace(record.SourceVersionID),
			CompiledBy:   CompilerDeterministic,
			Timestamp:    c.Now(),
		},
		Freshness: FreshnessValid,
		Body: ChunkBody{
			Raw:    string(raw),
			Fields: bodyFields,
		},
	}
	return c.saveCompiledChunk(chunk, input.RelatedChunkIDs, input.AmplifyChunkIDs, true)
}

func (c *Compiler) fromASTIndexEntry(input ASTIndexEntryInput) (*CompileResult, error) {
	ref := strings.TrimSpace(input.EntryID)
	if ref == "" {
		ref = firstNonEmpty(strings.TrimSpace(input.SymbolID), strings.TrimSpace(input.FilePath))
	}
	if ref == "" {
		return nil, fmt.Errorf("biknowledgecc: ast index entry requires entry id, symbol id, or file path")
	}
	bodyFields := map[string]any{
		"entry_id":          input.EntryID,
		"file_path":         input.FilePath,
		"symbol_id":         input.SymbolID,
		"summary":           input.Summary,
		"kind":              input.Kind,
		"based_on_revision": input.BasedOnRevision,
	}
	raw, _ := json.Marshal(bodyFields)
	chunk := KnowledgeChunk{
		ID:            deterministicChunkID("ast_index", ref),
		WorkspaceID:   strings.TrimSpace(input.WorkspaceID),
		ContentHash:   hashStrings(ref, string(raw), input.BasedOnRevision),
		TokenEstimate: estimateTokens(string(raw)),
		Provenance: ChunkProvenance{
			Sources: []ProvenanceSource{{
				Kind: "ast_index",
				Ref:  ref,
			}},
			SessionID:    strings.TrimSpace(input.SessionID),
			WorkflowID:   strings.TrimSpace(input.WorkflowID),
			CodeStateRef: strings.TrimSpace(input.BasedOnRevision),
			CompiledBy:   CompilerDeterministic,
			Timestamp:    c.Now(),
		},
		Freshness: FreshnessValid,
		Body: ChunkBody{
			Raw:    string(raw),
			Fields: bodyFields,
		},
	}
	return c.saveCompiledChunk(chunk, input.RelatedChunkIDs, input.AmplifyChunkIDs, true)
}

func (c *Compiler) fromUserStatement(input UserStatementInput) (*CompileResult, error) {
	bodyFields := userStatementFields(input.Interaction)
	if bodyFields == nil {
		bodyFields = map[string]any{"payload": input.Interaction}
	}
	if strings.TrimSpace(input.Statement) != "" {
		bodyFields["statement"] = input.Statement
	}
	ref := stringValue(bodyFields["interaction_id"])
	if ref == "" {
		ref = stringValue(bodyFields["id"])
	}
	if ref == "" {
		ref = strings.TrimSpace(input.Statement)
	}
	if ref == "" {
		return nil, fmt.Errorf("biknowledgecc: user statement requires interaction id or statement")
	}
	raw, _ := json.Marshal(bodyFields)
	chunk := KnowledgeChunk{
		ID:            deterministicChunkID("user_statement", ref),
		WorkspaceID:   strings.TrimSpace(input.WorkspaceID),
		ContentHash:   hashStrings(ref, string(raw), input.BasedOnRevision),
		TokenEstimate: estimateTokens(string(raw)),
		Provenance: ChunkProvenance{
			Sources: []ProvenanceSource{{
				Kind: "user_statement",
				Ref:  ref,
			}},
			SessionID:    strings.TrimSpace(input.SessionID),
			WorkflowID:   strings.TrimSpace(input.WorkflowID),
			CodeStateRef: strings.TrimSpace(input.BasedOnRevision),
			CompiledBy:   CompilerDeterministic,
			Timestamp:    c.Now(),
		},
		Freshness: FreshnessValid,
		Body: ChunkBody{
			Raw:    string(raw),
			Fields: bodyFields,
		},
	}
	return c.saveCompiledChunk(chunk, nil, input.AmplifyChunkIDs, false)
}

func (c *Compiler) saveCompiledChunk(chunk KnowledgeChunk, related []ChunkID, amplifies []ChunkID, writeCodeStateEdge bool) (*CompileResult, error) {
	if c == nil || c.Store == nil {
		return nil, fmt.Errorf("biknowledgecc: chunk store required")
	}
	result := &CompileResult{}
	previous, found, err := c.findLatestBySource(chunk.WorkspaceID, chunk.Provenance.Sources)
	if err != nil {
		return nil, err
	}
	saved, err := c.Store.Save(chunk)
	if err != nil {
		return nil, err
	}
	result.ChunkIDs = append(result.ChunkIDs, saved.ID)
	if found && previous != nil {
		edge, err := c.Store.SaveEdge(ChunkEdge{
			FromChunk:  saved.ID,
			ToChunk:    previous.ID,
			Kind:       EdgeKindSupersedes,
			Provenance: saved.Provenance,
		})
		if err != nil {
			return nil, err
		}
		result.EdgeIDs = append(result.EdgeIDs, edge.ID)
	}
	for _, source := range saved.Provenance.Sources {
		edge, err := c.Store.SaveEdge(ChunkEdge{
			FromChunk: saved.ID,
			Kind:      EdgeKindDerivesFrom,
			Meta: map[string]any{
				"source_kind": source.Kind,
				"source_ref":  source.Ref,
			},
			Provenance: saved.Provenance,
		})
		if err != nil {
			return nil, err
		}
		result.EdgeIDs = append(result.EdgeIDs, edge.ID)
	}
	if writeCodeStateEdge && strings.TrimSpace(saved.Provenance.CodeStateRef) != "" {
		edge, err := c.Store.SaveEdge(ChunkEdge{
			FromChunk: saved.ID,
			Kind:      EdgeKindDependsOnCodeState,
			Meta: map[string]any{
				"code_state_ref": saved.Provenance.CodeStateRef,
			},
			Provenance: saved.Provenance,
		})
		if err != nil {
			return nil, err
		}
		result.EdgeIDs = append(result.EdgeIDs, edge.ID)
	}
	for _, relatedID := range related {
		if relatedID == "" {
			continue
		}
		edge, err := c.Store.SaveEdge(ChunkEdge{
			FromChunk:  saved.ID,
			ToChunk:    relatedID,
			Kind:       EdgeKindRequiresContext,
			Weight:     1.0,
			Provenance: saved.Provenance,
		})
		if err != nil {
			return nil, err
		}
		result.EdgeIDs = append(result.EdgeIDs, edge.ID)
	}
	for i, amplifyID := range amplifies {
		if amplifyID == "" {
			continue
		}
		weight := 0.9 - float64(i)*0.1
		if weight < 0.1 {
			weight = 0.1
		}
		edge, err := c.Store.SaveEdge(ChunkEdge{
			FromChunk:  saved.ID,
			ToChunk:    amplifyID,
			Kind:       EdgeKindAmplifies,
			Weight:     weight,
			Provenance: saved.Provenance,
		})
		if err != nil {
			return nil, err
		}
		result.EdgeIDs = append(result.EdgeIDs, edge.ID)
	}
	return result, nil
}

func (c *Compiler) findLatestBySource(workspaceID string, sources []ProvenanceSource) (*KnowledgeChunk, bool, error) {
	if len(sources) == 0 {
		return nil, false, nil
	}
	chunks, err := c.Store.FindByWorkspace(workspaceID)
	if err != nil {
		return nil, false, err
	}
	var latest *KnowledgeChunk
	for i := range chunks {
		chunk := chunks[i]
		if !sameSourceSet(chunk.Provenance.Sources, sources) {
			continue
		}
		if latest == nil || chunk.Version > latest.Version {
			copy := chunk
			latest = &copy
		}
	}
	return latest, latest != nil, nil
}

func sameSourceSet(a, b []ProvenanceSource) bool {
	if len(a) != len(b) {
		return false
	}
	counts := make(map[string]int, len(a))
	for _, source := range a {
		counts[source.Kind+"\x00"+source.Ref]++
	}
	for _, source := range b {
		key := source.Kind + "\x00" + source.Ref
		counts[key]--
		if counts[key] < 0 {
			return false
		}
	}
	for _, value := range counts {
		if value != 0 {
			return false
		}
	}
	return true
}

func deterministicChunkID(kind, ref string) ChunkID {
	return ChunkID(fmt.Sprintf("chunk:%s:%s", kind, hashStrings(kind, ref)))
}

func hashStrings(values ...string) string {
	h := sha1.New()
	for _, value := range values {
		_, _ = h.Write([]byte(value))
		_, _ = h.Write([]byte{'\n'})
	}
	return hex.EncodeToString(h.Sum(nil))[:16]
}

func estimateTokens(raw string) int {
	if raw == "" {
		return 0
	}
	return max(1, len(raw)/4)
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func stringValue(v any) string {
	if s, ok := v.(string); ok {
		return strings.TrimSpace(s)
	}
	return strings.TrimSpace(fmt.Sprint(v))
}

func userStatementFields(v any) map[string]any {
	if v == nil {
		return nil
	}
	if m, ok := v.(map[string]any); ok {
		return cloneMap(m)
	}
	rv := reflect.ValueOf(v)
	if rv.Kind() == reflect.Pointer {
		if rv.IsNil() {
			return nil
		}
		rv = rv.Elem()
	}
	if rv.Kind() != reflect.Struct {
		return nil
	}
	rt := rv.Type()
	out := make(map[string]any, rt.NumField())
	for i := 0; i < rt.NumField(); i++ {
		field := rt.Field(i)
		if !field.IsExported() {
			continue
		}
		out[strings.ToLower(field.Name)] = rv.Field(i).Interface()
	}
	// Normalize common field names used by archaeology learning interactions.
	if id, ok := out["id"]; ok {
		out["interaction_id"] = id
	}
	if subjectType, ok := out["subjecttype"]; ok {
		out["subject_type"] = subjectType
	}
	if subjectID, ok := out["subjectid"]; ok {
		out["subject_id"] = subjectID
	}
	return out
}

func cloneMap(in map[string]any) map[string]any {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]any, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}
