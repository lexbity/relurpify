package ayenitd

import (
	"codeburg.org/lexbit/relurpify/context/knowledge"
	"codeburg.org/lexbit/relurpify/context/knowledge/graphdb"
)

// openKnowledgeStore creates a new ChunkStore backed by the graphdb engine.
func openKnowledgeStore(engine *graphdb.Engine) (*knowledge.ChunkStore, error) {
	return &knowledge.ChunkStore{Graph: engine}, nil
}
