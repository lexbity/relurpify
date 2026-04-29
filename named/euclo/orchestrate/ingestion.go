package orchestrate

import (
	"context"

	"codeburg.org/lexbit/relurpify/framework/contextdata"
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
func (n *IngestionNode) Type() string {
	return "ingestion"
}

// Execute performs file ingestion.
func (n *IngestionNode) Execute(ctx context.Context, env *contextdata.Envelope) (map[string]any, error) {
	// Get task envelope from working memory
	taskEnvelopeVal, ok := env.GetWorkingValue("euclo.task.envelope")
	if !ok {
		return nil, nil // No task envelope, nothing to ingest
	}

	taskEnvelope, ok := taskEnvelopeVal.(*intake.TaskEnvelope)
	if !ok {
		return nil, nil // Invalid task envelope type
	}

	// Ingest user files
	userFiles := taskEnvelope.UserFiles
	for _, filePath := range userFiles {
		// Phase 9: stub ingestion - in production, this would call the framework ingestion service
		ingestedContent := n.stubIngestFile(filePath)
		env.SetWorkingValue("euclo.ingested.file."+filePath, ingestedContent, contextdata.MemoryClassTask)
	}

	// Ingest session pins
	sessionPins := taskEnvelope.SessionPins
	for _, filePath := range sessionPins {
		// Phase 9: stub ingestion - in production, this would call the framework ingestion service
		ingestedContent := n.stubIngestFile(filePath)
		env.SetWorkingValue("euclo.ingested.pin."+filePath, ingestedContent, contextdata.MemoryClassTask)
	}

	// Write ingestion metadata
	env.SetWorkingValue("euclo.ingestion.user_files_count", len(userFiles), contextdata.MemoryClassTask)
	env.SetWorkingValue("euclo.ingestion.session_pins_count", len(sessionPins), contextdata.MemoryClassTask)

	return map[string]any{
		"user_files_ingested": len(userFiles),
		"session_pins_ingested": len(sessionPins),
	}, nil
}

// stubIngestFile is a stub for file ingestion.
// In production, this would call the framework ingestion service.
func (n *IngestionNode) stubIngestFile(filePath string) string {
	return "stub_ingested_content_for_" + filePath
}
