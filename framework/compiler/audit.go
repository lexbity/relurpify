package compiler

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"codeburg.org/lexbit/relurpify/framework/knowledge"
)

// CompilationAuditor inspects persisted compilation records and their chunk provenance.
type CompilationAuditor struct {
	Compiler *Compiler
	Store    *knowledge.ChunkStore
}

// CompilationAuditReport summarizes a compilation record and its provenance chain.
type CompilationAuditReport struct {
	RecordID            string
	RequestID           string
	DigestMatch         bool
	ExpectedDigest      string
	RecomputedDigest    string
	LoadedChunkIDs      []knowledge.ChunkID
	MissingDependencies []knowledge.ChunkID
	ProvenanceLines     []string
	Text                string
}

// NewCompilationAuditor creates an auditor for the provided compiler.
func NewCompilationAuditor(compiler *Compiler) *CompilationAuditor {
	var store *knowledge.ChunkStore
	if compiler != nil {
		store = compiler.chunkStore
	}
	return &CompilationAuditor{Compiler: compiler, Store: store}
}

// LoadCompilationRecord loads a compilation record by ID.
func (a *CompilationAuditor) LoadCompilationRecord(ctx context.Context, compilationID string) (*CompilationRecord, error) {
	if a != nil && a.Compiler != nil {
		return a.Compiler.LoadCompilationRecord(ctx, compilationID)
	}
	if a == nil || a.Store == nil {
		return nil, fmt.Errorf("compiler auditor: chunk store not configured")
	}
	chunks, err := a.Store.FindAll()
	if err != nil {
		return nil, err
	}
	for _, chunk := range chunks {
		if chunk.SourceOrigin != "compilation_record" {
			continue
		}
		var record CompilationRecord
		var content any
		if chunk.Body.Fields != nil {
			content, _ = chunk.Body.Fields["content"]
		}
		if content == nil {
			content = chunk.Body.Raw
		}
		data, ok := contentBytes(content)
		if !ok {
			continue
		}
		if err := json.Unmarshal(data, &record); err != nil {
			continue
		}
		if record.RequestID == compilationID {
			return &record, nil
		}
	}
	return nil, fmt.Errorf("compilation record not found: %s", compilationID)
}

// Audit loads a record, reconstructs the chunk set, and renders a provenance report.
func (a *CompilationAuditor) Audit(ctx context.Context, compilationID string) (*CompilationAuditReport, error) {
	record, err := a.LoadCompilationRecord(ctx, compilationID)
	if err != nil {
		return nil, err
	}
	chunks, missing, err := a.reconstructChunks(record)
	if err != nil {
		return nil, err
	}
	lines := a.renderProvenance(record, chunks, missing)
	expected := record.DeterministicDigest
	recomputed := compilationDigest(record)
	report := &CompilationAuditReport{
		RecordID:            record.AssemblyMetadata.CompilationID,
		RequestID:           record.RequestID,
		DigestMatch:         expected == recomputed,
		ExpectedDigest:      expected,
		RecomputedDigest:    recomputed,
		LoadedChunkIDs:      chunkIDs(chunks),
		MissingDependencies: missing,
		ProvenanceLines:     lines,
	}
	report.Text = strings.Join(lines, "\n")
	return report, nil
}

func (a *CompilationAuditor) reconstructChunks(record *CompilationRecord) ([]knowledge.KnowledgeChunk, []knowledge.ChunkID, error) {
	if record == nil {
		return nil, nil, fmt.Errorf("compiler auditor: compilation record required")
	}
	if a == nil || a.Store == nil || len(record.Dependencies) == 0 {
		return nil, append([]knowledge.ChunkID(nil), record.Dependencies...), nil
	}
	loaded, err := a.Store.LoadMany(record.Dependencies)
	if err != nil {
		return nil, nil, err
	}
	found := make(map[knowledge.ChunkID]struct{}, len(loaded))
	for _, chunk := range loaded {
		found[chunk.ID] = struct{}{}
	}
	missing := make([]knowledge.ChunkID, 0)
	for _, id := range record.Dependencies {
		if _, ok := found[id]; !ok {
			missing = append(missing, id)
		}
	}
	return loaded, missing, nil
}

func (a *CompilationAuditor) renderProvenance(record *CompilationRecord, chunks []knowledge.KnowledgeChunk, missing []knowledge.ChunkID) []string {
	lines := []string{
		"Compilation audit report",
		fmt.Sprintf("Record ID: %s", record.AssemblyMetadata.CompilationID),
		fmt.Sprintf("Request ID: %s", record.RequestID),
		fmt.Sprintf("Query: %s", record.Request.Query.Text),
		fmt.Sprintf("Event log seq: %d", record.EventLogSeq),
		fmt.Sprintf("Digest verified: %t", record.DeterministicDigest == compilationDigest(record)),
	}
	if len(record.RankersUsed) > 0 {
		lines = append(lines, "Rankers: "+strings.Join(record.RankersUsed, ", "))
	}
	if len(record.Dependencies) > 0 {
		lines = append(lines, "Dependencies:")
		for _, id := range record.Dependencies {
			lines = append(lines, "  - "+string(id))
		}
	}
	if len(missing) > 0 {
		lines = append(lines, "Missing dependencies:")
		for _, id := range missing {
			lines = append(lines, "  - "+string(id))
		}
	}
	if len(chunks) > 0 {
		lines = append(lines, "Chunk provenance:")
		seen := make(map[knowledge.ChunkID]struct{})
		for _, chunk := range chunks {
			lines = append(lines, a.renderChunkProvenance(chunk, 1, seen)...)
		}
	}
	return lines
}

func (a *CompilationAuditor) renderChunkProvenance(chunk knowledge.KnowledgeChunk, depth int, seen map[knowledge.ChunkID]struct{}) []string {
	indent := strings.Repeat("  ", depth)
	lines := []string{
		fmt.Sprintf("%s- %s origin=%s freshness=%s trust=%s", indent, chunk.ID, chunk.SourceOrigin, chunk.Freshness, chunk.TrustClass),
	}
	if len(chunk.DerivedFrom) > 0 {
		lines = append(lines, fmt.Sprintf("%s  derived_from=%s", indent, joinChunkIDs(chunk.DerivedFrom)))
	}
	if len(chunk.Provenance.Sources) > 0 {
		sources := make([]string, 0, len(chunk.Provenance.Sources))
		for _, source := range chunk.Provenance.Sources {
			sources = append(sources, fmt.Sprintf("%s:%s", source.Kind, source.Ref))
		}
		lines = append(lines, fmt.Sprintf("%s  provenance_sources=%s", indent, strings.Join(sources, ", ")))
	}
	if a == nil || a.Store == nil {
		return lines
	}
	if _, ok := seen[chunk.ID]; ok {
		lines = append(lines, fmt.Sprintf("%s  (cycle detected)", indent))
		return lines
	}
	seen[chunk.ID] = struct{}{}
	for _, parentID := range chunk.DerivedFrom {
		parent, ok, err := a.Store.Load(parentID)
		if err != nil {
			lines = append(lines, fmt.Sprintf("%s  parent_load_error=%v", indent, err))
			continue
		}
		if !ok || parent == nil {
			lines = append(lines, fmt.Sprintf("%s  parent_missing=%s", indent, parentID))
			continue
		}
		lines = append(lines, a.renderChunkProvenance(*parent, depth+1, seen)...)
	}
	return lines
}

func chunkIDs(chunks []knowledge.KnowledgeChunk) []knowledge.ChunkID {
	if len(chunks) == 0 {
		return nil
	}
	out := make([]knowledge.ChunkID, 0, len(chunks))
	for _, chunk := range chunks {
		out = append(out, chunk.ID)
	}
	return out
}

func joinChunkIDs(ids []knowledge.ChunkID) string {
	if len(ids) == 0 {
		return ""
	}
	parts := make([]string, 0, len(ids))
	for _, id := range ids {
		parts = append(parts, string(id))
	}
	return strings.Join(parts, ", ")
}

func contentBytes(content any) ([]byte, bool) {
	switch v := content.(type) {
	case string:
		return []byte(v), true
	case []byte:
		return v, true
	default:
		return nil, false
	}
}
