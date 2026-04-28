package agentgraph

import (
	"context"
	"fmt"

	"codeburg.org/lexbit/relurpify/framework/core"
	"codeburg.org/lexbit/relurpify/framework/retrieval"
)

// RetrievalNode is a graph node that performs knowledge retrieval.
// It uses the Retriever interface to scatter-gather results and writes them to graph state.
type RetrievalNode struct {
	id        string
	retriever *retrieval.Retriever
	queryKey  string // Key in context where query text is stored
	resultKey string // Key in context where results will be stored
}

// NewRetrievalNode creates a new retrieval node.
func NewRetrievalNode(id string, retriever *retrieval.Retriever, queryKey, resultKey string) *RetrievalNode {
	return &RetrievalNode{
		id:        id,
		retriever: retriever,
		queryKey:  queryKey,
		resultKey: resultKey,
	}
}

// ID returns the node ID.
func (n *RetrievalNode) ID() string {
	return n.id
}

// Type returns the node type.
func (n *RetrievalNode) Type() NodeType {
	return NodeTypeSystem
}

// Contract returns the execution contract for the retrieval node.
// Retrieval is a read-only external operation with no graph state side effects.
func (n *RetrievalNode) Contract() NodeContract {
	return NodeContract{
		SideEffectClass: SideEffectNone,
		Idempotency:     IdempotencyReplaySafe,
		ContextPolicy: core.StateBoundaryPolicy{
			ReadKeys:                 []string{"task.*", "retrieval.*", n.queryKey},
			WriteKeys:                []string{"retrieval.*", n.resultKey},
			AllowedMemoryClasses:     []core.MemoryClass{core.MemoryClassWorking},
			AllowedDataClasses:       []core.StateDataClass{core.StateDataClassTaskMetadata, core.StateDataClassStructuredState},
			MaxStateEntryBytes:       8192,
			MaxInlineCollectionItems: 100,
		},
	}
}

// Execute performs the retrieval operation.
func (n *RetrievalNode) Execute(ctx context.Context, state *Context) (*Result, error) {
	if n.retriever == nil {
		return &Result{
			NodeID:  n.id,
			Success: false,
			Error:   "retriever not configured",
		}, nil
	}

	// Get query text from state
	queryText := ""
	if n.queryKey != "" {
		if val, ok := (*state)[n.queryKey]; ok {
			queryText = fmt.Sprint(val)
		}
	}

	// Build retrieval query
	query := retrieval.RetrievalQuery{
		Text:  queryText,
		Scope: "workflow", // Default scope
		Limit: 10,         // Default limit
	}

	// Execute retrieval
	result, err := n.retriever.Retrieve(ctx, query)
	if err != nil {
		return &Result{
			NodeID:  n.id,
			Success: false,
			Error:   fmt.Sprintf("retrieval failed: %v", err),
		}, nil
	}

	// Store results in state
	if n.resultKey != "" {
		(*state)[n.resultKey] = result
	}

	// Also store ranked chunk IDs for easy access
	chunkIDs := make([]string, 0, len(result.Ranked))
	for _, rc := range result.Ranked {
		chunkIDs = append(chunkIDs, string(rc.ChunkID))
	}
	(*state)[n.resultKey+"_chunks"] = chunkIDs

	return &Result{
		NodeID:  n.id,
		Success: true,
		Data:    map[string]any{"result": result},
	}, nil
}

// RetrievalNodeConfig provides configuration for building retrieval nodes.
type RetrievalNodeConfig struct {
	ID         string
	Retriever  *retrieval.Retriever
	QueryKey   string
	ResultKey  string
	Scope      string
	Limit      int
	SourceType string
}

// BuildRetrievalNode creates a retrieval node from config.
func BuildRetrievalNode(config RetrievalNodeConfig) (*RetrievalNode, error) {
	if config.ID == "" {
		return nil, fmt.Errorf("retrieval node ID is required")
	}
	if config.Retriever == nil {
		return nil, fmt.Errorf("retriever is required")
	}

	node := NewRetrievalNode(config.ID, config.Retriever, config.QueryKey, config.ResultKey)
	return node, nil
}
