package retrieval

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/lexcodex/relurpify/framework/core"
)

const (
	defaultPackingMaxChunks      = 8
	defaultPackingMaxPerDocument = 3
	defaultAdjacencyGapBytes     = 128
)

// PackingOptions controls context packing limits.
type PackingOptions struct {
	MaxTokens      int
	MaxChunks      int
	MaxPerDocument int
}

// PackedCitation is the stable citation payload attached to packed evidence blocks.
type PackedCitation struct {
	DocID         string      `json:"doc_id"`
	ChunkID       string      `json:"chunk_id"`
	VersionID     string      `json:"version_id"`
	CanonicalURI  string      `json:"canonical_uri"`
	SourceType    string      `json:"source_type"`
	StructuralKey string      `json:"structural_key,omitempty"`
	StartOffset   int         `json:"start_offset"`
	EndOffset     int         `json:"end_offset"`
	Anchors       []AnchorRef `json:"anchors,omitempty"` // Active anchors for this citation
}

// PackedAnchorMetadata represents semantic anchor information in packed blocks.
type PackedAnchorMetadata struct {
	AnchorID   string    `json:"anchor_id"`
	Term       string    `json:"term"`
	Definition string    `json:"definition"`
	Class      string    `json:"class"`
	Status     string    `json:"status"` // "fresh" | "drifted" | "superseded"
	CreatedAt  time.Time `json:"created_at"`
}

// PackingResult captures packed blocks plus the packing decisions made.
type PackingResult struct {
	Blocks         []core.ContentBlock
	InjectedChunks []string
	DroppedChunks  map[string]string
	TokensUsed     int
	TokenBudget    int
}

// ContextItems converts packed retrieval blocks into reference-capable context items.
func (r *PackingResult) ContextItems() []core.ContextItem {
	if r == nil || len(r.Blocks) == 0 {
		return nil
	}
	items := make([]core.ContextItem, 0, len(r.Blocks))
	now := time.Now().UTC()
	for _, block := range r.Blocks {
		structured, ok := block.(core.StructuredContentBlock)
		if !ok {
			continue
		}
		payload, ok := structured.Data.(map[string]any)
		if !ok {
			continue
		}
		if strings.TrimSpace(toString(payload["type"])) != "retrieval_evidence" {
			continue
		}
		reference := retrievalReferenceFromPayload(payload)
		items = append(items, &core.RetrievalContextItem{
			Source:       toString(payload["type"]),
			Content:      toString(payload["text"]),
			Summary:      toString(payload["summary"]),
			Reference:    reference,
			LastAccessed: now,
			Relevance:    0.9,
			PriorityVal:  1,
		})
	}
	if len(items) == 0 {
		return nil
	}
	return items
}

// ContextPacker converts ranked retrieval candidates into bounded content blocks.
type ContextPacker struct {
	db        *sql.DB
	schemaErr error
}

type packableChunk struct {
	ChunkID       string
	DocID         string
	VersionID     string
	CanonicalURI  string
	SourceType    string
	StructuralKey string
	Text          string
	StartOffset   int
	EndOffset     int
	Derivation    *core.DerivationChain
}

type packBlock struct {
	docID        string
	versionID    string
	text         string
	startOffset  int
	endOffset    int
	citations    []PackedCitation
	tokenCount   int
	injectedIDs  []string
	structuralID string
	firstRank    int
	derivation   *core.DerivationChain
}

// NewContextPacker constructs a context packer over the retrieval SQLite store.
func NewContextPacker(db *sql.DB) *ContextPacker {
	return &ContextPacker{
		db:        db,
		schemaErr: ensureRuntimeSchema(context.Background(), db),
	}
}

// Pack converts ranked candidates into bounded content blocks with citation metadata.
func (p *ContextPacker) Pack(ctx context.Context, candidates []RankedCandidate, opts PackingOptions) (*PackingResult, error) {
	if p == nil || p.db == nil {
		return nil, errors.New("context packer db required")
	}
	if p.schemaErr != nil {
		return nil, p.schemaErr
	}
	limitChunks := opts.MaxChunks
	if limitChunks <= 0 {
		limitChunks = defaultPackingMaxChunks
	}
	maxPerDoc := opts.MaxPerDocument
	if maxPerDoc <= 0 {
		maxPerDoc = defaultPackingMaxPerDocument
	}
	result := &PackingResult{
		Blocks:        make([]core.ContentBlock, 0),
		DroppedChunks: make(map[string]string),
		TokenBudget:   opts.MaxTokens,
	}
	seen := make(map[string]struct{}, len(candidates))
	selectedByDoc := make(map[string]int)
	blocks := make([]*packBlock, 0)

	for i, candidate := range candidates {
		if len(result.InjectedChunks) >= limitChunks {
			result.DroppedChunks[candidate.ChunkID] = "chunk_limit"
			continue
		}
		if _, ok := seen[candidate.ChunkID]; ok {
			result.DroppedChunks[candidate.ChunkID] = "deduped"
			continue
		}
		chunk, err := p.loadPackableChunk(ctx, candidate.ChunkID, candidate.VersionID)
		if err != nil {
			return nil, err
		}
		if chunk == nil {
			result.DroppedChunks[candidate.ChunkID] = "missing"
			continue
		}
		if selectedByDoc[chunk.DocID] >= maxPerDoc {
			result.DroppedChunks[candidate.ChunkID] = "per_document_cap"
			continue
		}
		block := packBlockFromChunk(*chunk, i, candidate.Derivation)
		if merged := tryMergeAdjacent(blocks, block, opts.MaxTokens, result.TokensUsed); merged {
			seen[candidate.ChunkID] = struct{}{}
			selectedByDoc[chunk.DocID]++
			result.InjectedChunks = append(result.InjectedChunks, candidate.ChunkID)
			result.TokensUsed = repackTokens(blocks)
			continue
		}
		if opts.MaxTokens > 0 && result.TokensUsed+block.tokenCount > opts.MaxTokens {
			result.DroppedChunks[candidate.ChunkID] = "budget_overflow"
			continue
		}
		blocks = append(blocks, block)
		result.TokensUsed += block.tokenCount
		result.InjectedChunks = append(result.InjectedChunks, candidate.ChunkID)
		selectedByDoc[chunk.DocID]++
		seen[candidate.ChunkID] = struct{}{}
	}

	sort.Slice(blocks, func(i, j int) bool {
		if blocks[i].firstRank == blocks[j].firstRank {
			if blocks[i].docID == blocks[j].docID {
				return blocks[i].startOffset < blocks[j].startOffset
			}
			return blocks[i].docID < blocks[j].docID
		}
		return blocks[i].firstRank < blocks[j].firstRank
	})
	for _, block := range blocks {
		ref := blockReference(block)
		blockData := map[string]any{
			"type":      "retrieval_evidence",
			"text":      block.text,
			"summary":   truncateBlockText(block.text, 240),
			"citations": block.citations,
			"reference": ref,
		}
		// Include derivation summary if available
		if block.derivation != nil {
			blockData["derivation"] = block.derivation.Summary()
		}
		// Include anchor metadata if available
		if len(block.injectedIDs) > 0 {
			anchors, err := p.loadAnchorMetadataForChunks(ctx, block.injectedIDs)
			if err == nil && len(anchors) > 0 {
				blockData["anchors"] = anchors
				// Also attach anchors to citations
				p.attachAnchorsToCitations(block.citations, anchors)
			}
		}
		result.Blocks = append(result.Blocks, core.StructuredContentBlock{
			Data: blockData,
		})
	}
	return result, nil
}

func (p *ContextPacker) loadPackableChunk(ctx context.Context, chunkID, versionID string) (*packableChunk, error) {
	row := p.db.QueryRowContext(ctx, `SELECT cv.chunk_id, cv.doc_id, cv.version_id, d.canonical_uri, d.source_type, cv.structural_key, cv.text, cv.start_offset, cv.end_offset
		FROM retrieval_chunk_versions cv
		JOIN retrieval_chunks c
			ON c.chunk_id = cv.chunk_id AND c.active_version_id = cv.version_id
		JOIN retrieval_documents d
			ON d.doc_id = cv.doc_id
		WHERE cv.chunk_id = ?
		  AND cv.version_id = ?
		  AND c.tombstoned = 0
		  AND cv.tombstoned = 0`, chunkID, versionID)
	var chunk packableChunk
	if err := row.Scan(&chunk.ChunkID, &chunk.DocID, &chunk.VersionID, &chunk.CanonicalURI, &chunk.SourceType, &chunk.StructuralKey, &chunk.Text, &chunk.StartOffset, &chunk.EndOffset); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return &chunk, nil
}

func packBlockFromChunk(chunk packableChunk, rank int, candidateDerivation *core.DerivationChain) *packBlock {
	text := strings.TrimSpace(chunk.Text)
	citation := PackedCitation{
		DocID:         chunk.DocID,
		ChunkID:       chunk.ChunkID,
		VersionID:     chunk.VersionID,
		CanonicalURI:  chunk.CanonicalURI,
		SourceType:    chunk.SourceType,
		StructuralKey: chunk.StructuralKey,
		StartOffset:   chunk.StartOffset,
		EndOffset:     chunk.EndOffset,
	}

	// Stamp pack derivation
	var derivation *core.DerivationChain
	if candidateDerivation != nil {
		derivation = candidateDerivation
	} else {
		origin := core.OriginDerivation("retrieval")
		derivation = &origin
	}
	// Pack derivation has loss 0.0 for standalone chunks
	packed := derivation.Derive("pack", "retrieval", 0.0, "standalone chunk")
	derivation = &packed

	return &packBlock{
		docID:       chunk.DocID,
		versionID:   chunk.VersionID,
		text:        text,
		startOffset: chunk.StartOffset,
		endOffset:   chunk.EndOffset,
		citations:   []PackedCitation{citation},
		tokenCount:  core.EstimateTextTokens(text),
		injectedIDs: []string{chunk.ChunkID},
		firstRank:   rank,
		derivation:  derivation,
	}
}

func blockReference(block *packBlock) map[string]any {
	if block == nil {
		return nil
	}
	chunkIDs := append([]string(nil), block.injectedIDs...)
	return map[string]any{
		"kind":       string(core.ContextReferenceRetrievalEvidence),
		"id":         strings.Join(chunkIDs, ","),
		"uri":        block.docID,
		"version":    block.versionID,
		"detail":     "packed",
		"chunk_ids":  chunkIDs,
		"structural": block.structuralID,
	}
}

func tryMergeAdjacent(blocks []*packBlock, next *packBlock, maxTokens int, currentTokens int) bool {
	if len(blocks) == 0 || next == nil {
		return false
	}
	last := blocks[len(blocks)-1]
	if last.docID != next.docID || last.versionID != next.versionID {
		return false
	}
	if next.startOffset > last.endOffset+defaultAdjacencyGapBytes {
		return false
	}
	mergedText := strings.TrimSpace(last.text + "\n\n" + next.text)
	mergedTokens := core.EstimateTextTokens(mergedText)
	projectedTokens := currentTokens - last.tokenCount + mergedTokens
	if maxTokens > 0 && projectedTokens > maxTokens {
		return false
	}
	last.text = mergedText
	last.endOffset = next.endOffset
	last.citations = append(last.citations, next.citations...)
	last.injectedIDs = append(last.injectedIDs, next.injectedIDs...)
	last.tokenCount = mergedTokens
	if next.firstRank < last.firstRank {
		last.firstRank = next.firstRank
	}

	// Update derivation for merged chunk (loss 0.1 for adjacent merge)
	if next.derivation != nil {
		// Remove the pack step from next and re-add with merged loss
		if next.derivation.Depth() > 0 && next.derivation.LastTransform() == "pack" {
			steps := next.derivation.Steps
			if len(steps) > 1 {
				// Reconstruct chain without last pack step
				newChain := core.DerivationChain{Steps: steps[:len(steps)-1]}
				next.derivation = &newChain
			}
		}
		if next.derivation != nil {
			merged := next.derivation.Derive("pack", "retrieval", 0.1, "adjacent merge")
			last.derivation = &merged
		}
	}

	return true
}

func retrievalReferenceFromPayload(payload map[string]any) *core.ContextReference {
	raw, ok := payload["reference"].(map[string]any)
	if !ok || len(raw) == 0 {
		return nil
	}
	ref := &core.ContextReference{
		Kind:    core.ContextReferenceKind(toString(raw["kind"])),
		ID:      toString(raw["id"]),
		URI:     toString(raw["uri"]),
		Version: toString(raw["version"]),
		Detail:  toString(raw["detail"]),
	}
	metadata := map[string]string{}
	if structural := toString(raw["structural"]); structural != "" {
		metadata["structural"] = structural
	}
	if chunkIDs, ok := raw["chunk_ids"].([]string); ok && len(chunkIDs) > 0 {
		metadata["chunk_ids"] = strings.Join(chunkIDs, ",")
	}
	if chunkAny, ok := raw["chunk_ids"].([]any); ok && len(chunkAny) > 0 {
		ids := make([]string, 0, len(chunkAny))
		for _, v := range chunkAny {
			if id := toString(v); id != "" {
				ids = append(ids, id)
			}
		}
		if len(ids) > 0 {
			metadata["chunk_ids"] = strings.Join(ids, ",")
		}
	}
	if len(metadata) > 0 {
		ref.Metadata = metadata
	}
	return ref
}

func truncateBlockText(text string, max int) string {
	text = strings.TrimSpace(text)
	if max <= 0 || len(text) <= max {
		return text
	}
	return text[:max] + "..."
}

func toString(value any) string {
	if value == nil {
		return ""
	}
	switch typed := value.(type) {
	case string:
		return typed
	default:
		return strings.TrimSpace(fmt.Sprint(value))
	}
}

func repackTokens(blocks []*packBlock) int {
	total := 0
	for _, block := range blocks {
		total += block.tokenCount
	}
	return total
}

// loadAnchorMetadataForChunks retrieves active anchors for given chunk IDs and their freshness status.
func (p *ContextPacker) loadAnchorMetadataForChunks(ctx context.Context, chunkIDs []string) ([]PackedAnchorMetadata, error) {
	if p == nil || p.db == nil || len(chunkIDs) == 0 {
		return nil, nil
	}

	var metadata []PackedAnchorMetadata

	// Query active anchors for the chunk IDs
	placeholders := make([]string, len(chunkIDs))
	args := make([]any, len(chunkIDs))
	for i, id := range chunkIDs {
		placeholders[i] = "?"
		args[i] = id
	}

	query := fmt.Sprintf(`
		SELECT a.anchor_id, a.term, a.definition, a.anchor_class, a.created_at
		FROM retrieval_semantic_anchors a
		WHERE a.source_chunk_id IN (%s)
		  AND a.superseded_by IS NULL
		  AND a.invalidated_at IS NULL
		ORDER BY a.created_at DESC
	`, strings.Join(placeholders, ","))

	rows, err := p.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	// Map to track drift status for each anchor
	driftStatus := make(map[string]bool)

	for rows.Next() {
		var anchorID, term, definition, class string
		var createdAtStr string

		if err := rows.Scan(&anchorID, &term, &definition, &class, &createdAtStr); err != nil {
			return nil, err
		}

		var createdAt time.Time
		if createdAtStr != "" {
			t, err := time.Parse(time.RFC3339, createdAtStr)
			if err != nil {
				createdAt = time.Now().UTC()
			} else {
				createdAt = t
			}
		} else {
			createdAt = time.Now().UTC()
		}

		// Determine freshness status by checking for drift events
		status := "fresh"
		if _, hasDrift := driftStatus[anchorID]; !hasDrift {
			// Query for unresolved drift events for this anchor
			var driftCount int
			err := p.db.QueryRowContext(ctx, `
				SELECT COUNT(*)
				FROM retrieval_anchor_events
				WHERE anchor_id = ?
				  AND event_type = 'drift_detected'
				  AND resolved_at IS NULL
			`, anchorID).Scan(&driftCount)

			if err == nil && driftCount > 0 {
				status = "drifted"
				driftStatus[anchorID] = true
			} else {
				driftStatus[anchorID] = false
			}
		} else if hasDrift {
			status = "drifted"
		}

		metadata = append(metadata, PackedAnchorMetadata{
			AnchorID:   anchorID,
			Term:       term,
			Definition: definition,
			Class:      class,
			Status:     status,
			CreatedAt:  createdAt,
		})
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	if len(metadata) == 0 {
		return nil, nil
	}

	return metadata, nil
}

// attachAnchorsToCitations converts PackedAnchorMetadata to AnchorRef and attaches to citations.
func (p *ContextPacker) attachAnchorsToCitations(citations []PackedCitation, anchors []PackedAnchorMetadata) {
	if len(citations) == 0 || len(anchors) == 0 {
		return
	}

	// Convert metadata to AnchorRef
	anchorRefs := make([]AnchorRef, len(anchors))
	for i, meta := range anchors {
		anchorRefs[i] = AnchorRef{
			AnchorID:   meta.AnchorID,
			Term:       meta.Term,
			Definition: meta.Definition,
			Class:      meta.Class,
			Active:     meta.Status != "superseded",
			CreatedAt:  meta.CreatedAt.Format(time.RFC3339),
		}
	}

	// Attach to all citations
	for i := range citations {
		citations[i].Anchors = anchorRefs
	}
}
