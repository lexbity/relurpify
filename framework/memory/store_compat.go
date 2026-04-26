package memory

import (
	"codeburg.org/lexbit/relurpify/framework/knowledge"
)

type KnowledgeStore = *knowledge.ChunkStore

func NewInMemoryKnowledgeStore() KnowledgeStore {
	return &knowledge.ChunkStore{}
}
