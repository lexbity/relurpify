package pretask

import (
	"context"
)

// IndexRetriever performs structural retrieval from the AST index.
type IndexRetriever struct{}

// Retrieve returns code evidence for the given anchor set.
func (r *IndexRetriever) Retrieve(ctx context.Context, anchors AnchorSet) ([]CodeEvidenceItem, error) {
	// Stub implementation for Phase 1.
	return nil, nil
}

// ArchaeoRetriever queries the archaeo knowledge layer.
type ArchaeoRetriever struct{}

// RetrieveTopic performs Stage 1b: query-driven archaeo retrieval.
func (r *ArchaeoRetriever) RetrieveTopic(ctx context.Context, query, workflowID string) ([]KnowledgeEvidenceItem, error) {
	// Stub implementation for Phase 1.
	return nil, nil
}

// RetrieveExpanded performs Stage 3: hypothetical-driven archaeo retrieval.
func (r *ArchaeoRetriever) RetrieveExpanded(ctx context.Context, sketch HypotheticalSketch) ([]KnowledgeEvidenceItem, error) {
	// Stub implementation for Phase 1.
	return nil, nil
}
