package retrieval

import (
	"context"
	"database/sql"
	"errors"
	"sort"
	"strings"

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
	DocID         string `json:"doc_id"`
	ChunkID       string `json:"chunk_id"`
	VersionID     string `json:"version_id"`
	CanonicalURI  string `json:"canonical_uri"`
	SourceType    string `json:"source_type"`
	StructuralKey string `json:"structural_key,omitempty"`
	StartOffset   int    `json:"start_offset"`
	EndOffset     int    `json:"end_offset"`
}

// PackingResult captures packed blocks plus the packing decisions made.
type PackingResult struct {
	Blocks         []core.ContentBlock
	InjectedChunks []string
	DroppedChunks  map[string]string
	TokensUsed     int
	TokenBudget    int
}

// ContextPacker converts ranked retrieval candidates into bounded content blocks.
type ContextPacker struct {
	db *sql.DB
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
}

// NewContextPacker constructs a context packer over the retrieval SQLite store.
func NewContextPacker(db *sql.DB) *ContextPacker {
	return &ContextPacker{db: db}
}

// Pack converts ranked candidates into bounded content blocks with citation metadata.
func (p *ContextPacker) Pack(ctx context.Context, candidates []RankedCandidate, opts PackingOptions) (*PackingResult, error) {
	if p == nil || p.db == nil {
		return nil, errors.New("context packer db required")
	}
	if err := EnsureSchema(ctx, p.db); err != nil {
		return nil, err
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
		block := packBlockFromChunk(*chunk, i)
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
		result.Blocks = append(result.Blocks, core.StructuredContentBlock{
			Data: map[string]any{
				"type":      "retrieval_evidence",
				"text":      block.text,
				"citations": block.citations,
			},
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

func packBlockFromChunk(chunk packableChunk, rank int) *packBlock {
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
	return true
}

func repackTokens(blocks []*packBlock) int {
	total := 0
	for _, block := range blocks {
		total += block.tokenCount
	}
	return total
}
