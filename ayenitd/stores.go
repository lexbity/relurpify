package ayenitd

import (
	"codeburg.org/lexbit/relurpify/framework/graphdb"
	"codeburg.org/lexbit/relurpify/framework/knowledge"
)

// openKnowledgeStore creates a new ChunkStore backed by the graphdb engine.
func openKnowledgeStore(engine *graphdb.Engine) (*knowledge.ChunkStore, error) {
	return &knowledge.ChunkStore{Graph: engine}, nil
}
