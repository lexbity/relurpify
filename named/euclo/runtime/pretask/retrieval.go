package pretask

import (
	"context"
	"fmt"

	"github.com/lexcodex/relurpify/framework/ast"
)

// IndexRetriever performs structural retrieval from the AST index.
type IndexRetriever struct {
	index  IndexQuerier
	deps   DependencyQuerier
	config IndexRetrieverConfig
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
		return []CodeEvidenceItem{}, nil
	}

	results := make([]CodeEvidenceItem, 0)
	seenPaths := make(map[string]bool)

	// Process file paths directly (these are already confirmed by the user)
	for _, path := range anchors.FilePaths {
		if seenPaths[path] {
			continue
		}
		results = append(results, CodeEvidenceItem{
			Path:       path,
			Score:      1.0,
			Source:     EvidenceSourceAnchor,
			TrustClass: "workspace-trusted",
			Summary:    fmt.Sprintf("File: %s", path),
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

		// Placeholder path for the symbol
		path := fmt.Sprintf("symbol:%s", symbol)
		if seenPaths[path] {
			continue
		}
		results = append(results, CodeEvidenceItem{
			Path:       path,
			Score:      0.8,
			Source:     EvidenceSourceIndex,
			TrustClass: "builtin-trusted",
			Summary:    fmt.Sprintf("Contains symbol: %s", symbol),
		})
		seenPaths[path] = true

		// Expand dependencies if available
		if r.deps != nil && r.config.DependencyHops > 0 {
			// Try to get dependency graph
			depGraph, err := r.deps.GetDependencyGraph(symbol)
			if err == nil && depGraph != nil {
				for _, dep := range depGraph.Dependencies {
					depPath := fmt.Sprintf("dep:%s", dep.Name)
					if !seenPaths[depPath] {
						results = append(results, CodeEvidenceItem{
							Path:       depPath,
							Score:      0.6,
							Source:     EvidenceSourceIndex,
							TrustClass: "builtin-trusted",
							Summary:    fmt.Sprintf("Dependency of %s", symbol),
						})
						seenPaths[depPath] = true
					}
				}
			}
		}
	}

	return results, nil
}

// ArchaeoRetriever queries the archaeo knowledge layer.
type ArchaeoRetriever struct {
	tensionSvc TensionQuerier
	patternSvc PatternQuerier
	retriever  ExpandedKnowledgeRetriever
	config     ArchaeoRetrieverConfig
}

// ExpandedKnowledgeRetriever optionally provides topic-specific expanded
// retrieval instead of the built-in stub fallback.
type ExpandedKnowledgeRetriever interface {
	RetrieveExpanded(context.Context, HypotheticalSketch) ([]KnowledgeEvidenceItem, error)
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
	if workflowID == "" {
		return []KnowledgeEvidenceItem{}, nil
	}

	results := make([]KnowledgeEvidenceItem, 0)

	// Try to get tensions if available
	if r.tensionSvc != nil {
		tensions, err := r.tensionSvc.ActiveByWorkflow(ctx, workflowID)
		if err == nil && tensions != nil {
			for i := range tensions {
				results = append(results, KnowledgeEvidenceItem{
					RefID:      fmt.Sprintf("tension_%d", i),
					Kind:       KnowledgeKindTension,
					Title:      fmt.Sprintf("Tension %d", i),
					Summary:    fmt.Sprintf("Active tension in workflow %s", workflowID),
					Score:      0.7,
					Source:     EvidenceSourceArchaeoTopic,
					TrustClass: "builtin-trusted",
				})
			}
		}
	}

	// Try to get patterns if available
	if r.patternSvc != nil {
		patterns, err := r.patternSvc.ListByWorkflow(ctx, workflowID)
		if err == nil && patterns != nil {
			for i := range patterns {
				results = append(results, KnowledgeEvidenceItem{
					RefID:      fmt.Sprintf("pattern_%d", i),
					Kind:       KnowledgeKindPattern,
					Title:      fmt.Sprintf("Pattern %d", i),
					Summary:    fmt.Sprintf("Pattern in workflow %s", workflowID),
					Score:      0.6,
					Source:     EvidenceSourceArchaeoTopic,
					TrustClass: "builtin-trusted",
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
	if !sketch.Grounded || r.config.WorkflowID == "" {
		return []KnowledgeEvidenceItem{}, nil
	}

	// Use the sketch text for retrieval
	queryText := sketch.Text
	if queryText == "" {
		return []KnowledgeEvidenceItem{}, nil
	}

	results := make([]KnowledgeEvidenceItem, 0)

	// Try to use the retriever service if available.
	if r.retriever != nil {
		results, err := r.retriever.RetrieveExpanded(ctx, sketch)
		if err == nil && len(results) > 0 {
			if len(results) > r.config.MaxItems {
				return results[:r.config.MaxItems], nil
			}
			return results, nil
		}
	}

	// Fallback to stub results
	results = append(results, KnowledgeEvidenceItem{
		RefID:      "expanded_1",
		Kind:       KnowledgeKindDecision,
		Title:      "Architectural decision",
		Summary:    fmt.Sprintf("Decision related to: %s", queryText),
		Score:      0.8,
		Source:     EvidenceSourceArchaeoExpanded,
		TrustClass: "builtin-trusted",
	})

	results = append(results, KnowledgeEvidenceItem{
		RefID:      "expanded_2",
		Kind:       KnowledgeKindInteraction,
		Title:      "Previous interaction",
		Summary:    "Similar interaction from workflow history",
		Score:      0.7,
		Source:     EvidenceSourceArchaeoExpanded,
		TrustClass: "builtin-trusted",
	})

	// Apply limits
	if len(results) > r.config.MaxItems {
		results = results[:r.config.MaxItems]
	}

	return results, nil
}
