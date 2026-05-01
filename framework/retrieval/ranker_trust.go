package retrieval

import (
	"context"
	"strings"

	"codeburg.org/lexbit/relurpify/framework/agentspec"
	"codeburg.org/lexbit/relurpify/framework/knowledge"
)

// TrustRanker prefers higher-trust chunks.
type TrustRanker struct{}

func (r *TrustRanker) Name() string { return "trust" }

func (r *TrustRanker) Rank(ctx context.Context, query RetrievalQuery, store *knowledge.ChunkStore) ([]knowledge.ChunkID, error) {
	_ = ctx
	_ = query
	if r == nil || store == nil {
		return nil, nil
	}
	chunks, err := loadRankerChunks(store)
	if err != nil || len(chunks) == 0 {
		return nil, err
	}

	scores := make(map[knowledge.ChunkID]float64, len(chunks))
	for _, chunk := range chunks {
		scores[chunk.ID] = trustMultiplier(chunk.TrustClass)
	}

	ids := sortRankedIDs(scores, func(a, b knowledge.ChunkID) bool {
		left, _ := chunkByID(chunks, a)
		right, _ := chunkByID(chunks, b)
		if trustMultiplier(left.TrustClass) == trustMultiplier(right.TrustClass) {
			return strings.Compare(string(a), string(b)) < 0
		}
		return trustMultiplier(left.TrustClass) > trustMultiplier(right.TrustClass)
	})
	return ids, nil
}

func trustMultiplier(class agentspec.TrustClass) float64 {
	switch class {
	case agentspec.TrustClassBuiltinTrusted, agentspec.TrustClassWorkspaceTrusted:
		return 1.0
	case agentspec.TrustClassToolResult:
		return 0.9
	case agentspec.TrustClassLLMGenerated:
		return 0.8
	case agentspec.TrustClassProviderLocalUntrusted:
		return 0.6
	case agentspec.TrustClassRemoteDeclared:
		return 0.5
	case agentspec.TrustClassRemoteApproved:
		return 0.7
	default:
		return 0.5
	}
}
