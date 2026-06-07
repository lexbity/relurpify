package orchestrate

import (
	"context"

	"codeburg.org/lexbit/relurpify/context/contextdata"
	execution "codeburg.org/lexbit/relurpify/execution"
	"codeburg.org/lexbit/relurpify/execution/agentgraph"
	"codeburg.org/lexbit/relurpify/named/euclo/intake"
	euclostate "codeburg.org/lexbit/relurpify/named/euclo/state"
)

// IngestionNode ingests user files and session pins into the envelope.
type IngestionNode struct {
	id string
}

// NewIngestionNode creates a new ingestion node.
func NewIngestionNode(id string) *IngestionNode {
	return &IngestionNode{
		id: id,
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

// Execute performs file ingestion.
func (n *IngestionNode) Execute(ctx context.Context, env *contextdata.Envelope) (*execution.Result, error) {
	_ = ctx
	result := &execution.Result{
		NodeID:  n.id,
		Success: true,
		Data: execution.NewToolResultPayload(map[string]any{
			"user_files_ingested":   0,
			"session_pins_ingested": 0,
			"skipped":               true,
		}),
	}

	// Get task envelope from working memory.
	taskEnvelopeVal, ok := env.GetWorkingValue(euclostate.KeyTaskEnvelope)
	if !ok {
		return result, nil
	}

	taskEnvelope, ok := taskEnvelopeVal.(*intake.TaskEnvelope)
	if !ok {
		return result, nil
	}
	fields := execution.ResultFields(result.Data)
	fields["skipped"] = false

	// Ingest user files
	userFiles := taskEnvelope.UserFiles
	for _, filePath := range userFiles {
		euclostate.SetIngestedFile(env, filePath, "stub_ingested_content_for_"+filePath)
	}

	// Ingest session pins
	sessionPins := taskEnvelope.SessionPins
	for _, filePath := range sessionPins {
		euclostate.SetIngestedPin(env, filePath, "stub_ingested_content_for_"+filePath)
	}

	// Write ingestion metadata
	euclostate.SetIngestionUserFilesCount(env, len(userFiles))
	euclostate.SetIngestionSessionPinsCount(env, len(sessionPins))

	fields["user_files_ingested"] = len(userFiles)
	fields["session_pins_ingested"] = len(sessionPins)
	return result, nil
}
