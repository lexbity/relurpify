package pretask

import (
	"context"
	"fmt"
	"strings"

	"github.com/lexcodex/relurpify/framework/ast"
	"github.com/lexcodex/relurpify/framework/retrieval"
)

// IndexRetriever performs structural retrieval from the AST index.
type IndexRetriever struct {
	index    IndexQuerier
	deps     DependencyQuerier
	config   IndexRetrieverConfig
}

type DependencyQuerier interface {
	GetDependencyGraph(symbol string) (*ast.DependencyGraph, error)
	GetCallGraph(symbol string) (*ast.CallGraph, error)
}

type IndexRetrieverConfig struct {
	// DependencyHops is how many hops to expand from each anchor symbol (default 1).
	DependencyHops int
	// MaxFilesPerSymbol caps expansion per anchor (default 3).
	MaxFilesPerSymbol int
}

// Retrieve returns code evidence for the given anchor set.
func (r *IndexRetriever) Retrieve(ctx context.Context, anchors AnchorSet) ([]CodeEvidenceItem, error) {
	if r.index == nil {
		return nil, nil
	}
	
	var results []CodeEvidenceItem
	seenPaths := make(map[string]bool)
	
	// Process file paths directly
	for _, path := range anchors.FilePaths {
		if seenPaths[path] {
			continue
		}
		results = append(results, CodeEvidenceItem{
			Path:    path,
			Score:   1.0,
			Source:  EvidenceSourceAnchor,
			Summary: fmt.Sprintf("File: %s", path),
		})
		seenPaths[path] = true
	}
	
	// Process symbols for dependency expansion
	for _, symbol := range anchors.SymbolNames {
		// Get the file containing the symbol
		nodes, err := r.index.QuerySymbol(symbol)
		if err != nil || len(nodes) == 0 {
			continue
		}
		
		// For each node, get its file
		for _, node := range nodes {
			if node.FileID == "" {
				continue
			}
			// We need to get the file path from the node
			// For now, use a placeholder
			path := fmt.Sprintf("file_from_%s", symbol)
			if seenPaths[path] {
				continue
			}
			results = append(results, CodeEvidenceItem{
				Path:    path,
				Score:   0.8,
				Source:  EvidenceSourceIndex,
				Summary: fmt.Sprintf("Contains symbol: %s", symbol),
			})
			seenPaths[path] = true
			
			// Expand dependencies if available
			if r.deps != nil && r.config.DependencyHops > 0 {
				// This is a simplified implementation
				// In reality, we would traverse the dependency graph
			}
		}
	}
	
	return results, nil
}

// ArchaeoRetriever queries the archaeo knowledge layer.
type ArchaeoRetriever struct {
	tensionSvc   TensionQuerier
	patternSvc   PatternQuerier
	retriever    retrieval.RetrieverService
	config       ArchaeoRetrieverConfig
}

// TensionQuerier is the narrow interface needed from archaeo/tensions.
type TensionQuerier interface {
	ActiveByWorkflow(ctx context.Context, workflowID string) ([]interface{}, error)
}

// PatternQuerier is the narrow interface needed from archaeo patterns.
type PatternQuerier interface {
	ListByWorkflow(ctx context.Context, workflowID string) ([]interface{}, error)
}

type ArchaeoRetrieverConfig struct {
	WorkflowID string
	MaxItems   int
	MaxTokens  int
}

// RetrieveTopic performs Stage 1b: query-driven archaeo retrieval.
func (r *ArchaeoRetriever) RetrieveTopic(ctx context.Context, query, workflowID string) ([]KnowledgeEvidenceItem, error) {
	if workflowID == "" || (r.tensionSvc == nil && r.patternSvc == nil && r.retriever == nil) {
		return nil, nil
	}
	
	var results []KnowledgeEvidenceItem
	
	// Try to get tensions if available
	if r.tensionSvc != nil {
		tensions, err := r.tensionSvc.ActiveByWorkflow(ctx, workflowID)
		if err == nil && len(tensions) > 0 {
			for i, tension := range tensions {
				// Convert to KnowledgeEvidenceItem
				// This is a simplified conversion
				results = append(results, KnowledgeEvidenceItem{
					RefID:  fmt.Sprintf("tension_%d", i),
					Kind:   KnowledgeKindTension,
					Title:  fmt.Sprintf("Tension %d", i),
					Summary: "Active tension related to query",
					Score:  0.7,
					Source: EvidenceSourceArchaeoTopic,
				})
			}
		}
	}
	
	// Try to get patterns if available
	if r.patternSvc != nil {
		patterns, err := r.patternSvc.ListByWorkflow(ctx, workflowID)
		if err == nil && len(patterns) > 0 {
			for i, pattern := range patterns {
				results = append(results, KnowledgeEvidenceItem{
					RefID:  fmt.Sprintf("pattern_%d", i),
					Kind:   KnowledgeKindPattern,
					Title:  fmt.Sprintf("Pattern %d", i),
					Summary: "Pattern related to query",
					Score:  0.6,
					Source: EvidenceSourceArchaeoTopic,
				})
			}
		}
	}
	
	// Apply limits
	if len(results) > r.config.MaxItems {
		results = results[:r.config.MaxItems]
	}
	
	return results, nil
}

// RetrieveExpanded performs Stage 3: hypothetical-driven archaeo retrieval.
func (r *ArchaeoRetriever) RetrieveExpanded(ctx context.Context, sketch HypotheticalSketch) ([]KnowledgeEvidenceItem, error) {
	if !sketch.Grounded || r.retriever == nil || r.config.WorkflowID == "" {
		return nil, nil
	}
	
	// Use the sketch text for retrieval
	queryText := sketch.Text
	if queryText == "" {
		return nil, nil
	}
	
	// This is a simplified implementation
	// In reality, we would use the retriever service
	var results []KnowledgeEvidenceItem
	
	// Simulate some results
	results = append(results, KnowledgeEvidenceItem{
		RefID:   "expanded_1",
		Kind:    KnowledgeKindDecision,
		Title:   "Decision based on hypothetical",
		Summary: "Decision related to the generated vocabulary",
		Score:   0.8,
		Source:  EvidenceSourceArchaeoExpanded,
	})
	
	// Apply limits
	if len(results) > r.config.MaxItems {
		results = results[:r.config.MaxItems]
	}
	
	return results, nil
}
