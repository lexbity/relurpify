package ingestion

import (
	"context"
	"fmt"

	"codeburg.org/lexbit/relurpify/framework/agentgraph"
	"codeburg.org/lexbit/relurpify/framework/contextdata"
)

// IngestionNode is an agentgraph.Node that runs the ingestion pipeline.
type IngestionNode struct {
	id   string
	spec IngestionSpec
}

// NewIngestionNode creates a new IngestionNode.
func NewIngestionNode(id string, spec IngestionSpec) *IngestionNode {
	return &IngestionNode{
		id:   id,
		spec: spec,
	}
}

// ID returns the node ID.
func (n *IngestionNode) ID() string {
	return n.id
}

// Type returns the node type.
func (n *IngestionNode) Type() agentgraph.NodeType {
	return agentgraph.NodeTypeTool
}

// Contract returns the node contract.
func (n *IngestionNode) Contract() agentgraph.NodeContract {
	return agentgraph.NodeContract{
		SideEffectClass: agentgraph.SideEffectLocal,
		Idempotency:     agentgraph.IdempotencyReplaySafe,
	}
}

// Execute runs the ingestion pipeline.
func (n *IngestionNode) Execute(ctx context.Context, env *contextdata.Envelope) (*agentgraph.Result, error) {
	result := &IngestionResult{
		Mode:        n.spec.Mode,
		CompletedAt: 0, // Set after completion
	}

	// If no files to ingest and mode is files_only, skip
	if n.spec.Mode == IngestionModeFilesOnly && len(n.spec.ExplicitFiles) == 0 {
		return &agentgraph.Result{
			NodeID:  n.id,
			Success: true,
			Data: map[string]any{
				"ingestion_result": result,
			},
		}, nil
	}

	// Execute ingestion based on mode
	switch n.spec.Mode {
	case IngestionModeFilesOnly:
		if err := n.ingestFiles(ctx, env, result); err != nil {
			result.Error = err.Error()
			return &agentgraph.Result{
				NodeID:  n.id,
				Success: false,
				Data: map[string]any{
					"ingestion_result": result,
					"error":            err.Error(),
				},
			}, err
		}

	case IngestionModeIncremental:
		if err := n.ingestIncremental(ctx, env, result); err != nil {
			result.Error = err.Error()
			return &agentgraph.Result{
				NodeID:  n.id,
				Success: false,
				Data: map[string]any{
					"ingestion_result": result,
					"error":            err.Error(),
				},
			}, err
		}

	case IngestionModeFull:
		if err := n.ingestFull(ctx, env, result); err != nil {
			result.Error = err.Error()
			return &agentgraph.Result{
				NodeID:  n.id,
				Success: false,
				Data: map[string]any{
					"ingestion_result": result,
					"error":            err.Error(),
				},
			}, err
		}

	default:
		err := fmt.Errorf("unknown ingestion mode: %s", n.spec.Mode)
		return &agentgraph.Result{
			NodeID:  n.id,
			Success: false,
			Data: map[string]any{
				"error": err.Error(),
			},
		}, err
	}

	// Write result to envelope
	env.SetWorkingValue("euclo.ingestion_result", result, contextdata.MemoryClassTask)

	return &agentgraph.Result{
		NodeID:  n.id,
		Success: true,
		Data: map[string]any{
			"ingestion_result": result,
		},
	}, nil
}

// ingestFiles ingests explicit files using the ingestion pipeline.
func (n *IngestionNode) ingestFiles(ctx context.Context, env *contextdata.Envelope, result *IngestionResult) error {
	// Phase 9: Stub implementation
	// Full implementation would:
	// 1. For each file in n.spec.ExplicitFiles, call ingestion.PipelineFactory
	// 2. Use ingestion.AcquireFromFile to build the pipeline
	// 3. Run the pipeline and collect IngestResult
	// 4. Write chunks to knowledge.ChunkStore
	// 5. Track file records in result.Records

	for _, filePath := range n.spec.ExplicitFiles {
		record := FileIngestionRecord{
			Path:        filePath,
			ChunkCount:  1, // Stub
			SizeBytes:   0, // Stub
			ContentHash: "stub_hash",
		}
		result.Records = append(result.Records, record)
		result.ChunkCount += record.ChunkCount
	}
	result.FileCount = len(result.Records)
	return nil
}

// ingestIncremental performs incremental workspace scanning.
func (n *IngestionNode) ingestIncremental(ctx context.Context, env *contextdata.Envelope, result *IngestionResult) error {
	// Phase 9: Stub implementation
	// Full implementation would use ingestion.WorkspaceScanner.ScanIncremental
	return fmt.Errorf("incremental ingestion not yet implemented")
}

// ingestFull performs full workspace scanning.
func (n *IngestionNode) ingestFull(ctx context.Context, env *contextdata.Envelope, result *IngestionResult) error {
	// Phase 9: Stub implementation
	// Full implementation would use ingestion.WorkspaceScanner.Scan
	return fmt.Errorf("full ingestion not yet implemented")
}
