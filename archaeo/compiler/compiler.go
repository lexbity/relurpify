package compiler

import (
	"fmt"
	"time"

	biknowledgecc "codeburg.org/lexbit/relurpify/framework/biknowledgecc"
)

type CompilerInputKind = biknowledgecc.CompilerInputKind
type PatternConfirmationInput = biknowledgecc.PatternConfirmationInput
type AnchorConfirmationInput = biknowledgecc.AnchorConfirmationInput
type ASTIndexEntryInput = biknowledgecc.ASTIndexEntryInput
type UserStatementInput = biknowledgecc.UserStatementInput
type CompilerInput = biknowledgecc.CompilerInput
type CompileResult = biknowledgecc.CompileResult

// Compiler keeps the archaeology-local compile helpers that layer on top of
// the framework-native substrate.
type Compiler struct {
	Store *ChunkStore
	Now   func() time.Time
}

func (c *Compiler) saveCompiledChunk(chunk KnowledgeChunk, related []ChunkID, amplifies []ChunkID, writeCodeStateEdge bool) (*CompileResult, error) {
	if c == nil || c.Store == nil {
		return nil, fmt.Errorf("compiler: chunk store required")
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
	if writeCodeStateEdge && firstNonEmpty(saved.Provenance.CodeStateRef) != "" {
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
	if len(sources) == 0 || c == nil || c.Store == nil {
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
