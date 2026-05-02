package orchestrate

import (
	"context"

	"codeburg.org/lexbit/relurpify/framework/agentgraph"
	"codeburg.org/lexbit/relurpify/framework/contextdata"
	"codeburg.org/lexbit/relurpify/framework/core"
	"codeburg.org/lexbit/relurpify/named/euclo/intake"
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
func (n *IngestionNode) Execute(ctx context.Context, env *contextdata.Envelope) (*core.Result, error) {
	_ = ctx
	result := &core.Result{
		NodeID:  n.id,
		Success: true,
		Data: map[string]any{
			"user_files_ingested":   0,
			"session_pins_ingested": 0,
			"skipped":               true,
		},
	}

	// Get task envelope from working memory.
	taskEnvelopeVal, ok := env.GetWorkingValue("euclo.task.envelope")
	if !ok {
		return result, nil
	}

	taskEnvelope, ok := taskEnvelopeVal.(*intake.TaskEnvelope)
	if !ok {
		return result, nil
	}
	result.Data["skipped"] = false

	// Ingest user files
	userFiles := taskEnvelope.UserFiles
	for _, filePath := range userFiles {
		ingestedContent := "stub_ingested_content_for_" + filePath
		env.SetWorkingValue("euclo.ingested.file."+filePath, ingestedContent, contextdata.MemoryClassTask)
	}

	// Ingest session pins
	sessionPins := taskEnvelope.SessionPins
	for _, filePath := range sessionPins {
		ingestedContent := "stub_ingested_content_for_" + filePath
		env.SetWorkingValue("euclo.ingested.pin."+filePath, ingestedContent, contextdata.MemoryClassTask)
	}

	// Write ingestion metadata
	env.SetWorkingValue("euclo.ingestion.user_files_count", len(userFiles), contextdata.MemoryClassTask)
	env.SetWorkingValue("euclo.ingestion.session_pins_count", len(sessionPins), contextdata.MemoryClassTask)

	result.Data["user_files_ingested"] = len(userFiles)
	result.Data["session_pins_ingested"] = len(sessionPins)
	return result, nil
}
