package retrieval

import (
	"math"
	"sort"
	"strings"
	"unicode"

	"codeburg.org/lexbit/relurpify/framework/knowledge"
)

func tokenizeRankerText(text string) []string {
	text = strings.ToLower(strings.TrimSpace(text))
	if text == "" {
		return nil
	}
	return strings.FieldsFunc(text, func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsNumber(r)
	})
}

func loadRankerChunks(store *knowledge.ChunkStore) ([]knowledge.KnowledgeChunk, error) {
	if store == nil {
		return nil, nil
	}
	return store.FindAll()
}

func sortRankedIDs(scores map[knowledge.ChunkID]float64, tiebreak func(a, b knowledge.ChunkID) bool) []knowledge.ChunkID {
	if len(scores) == 0 {
		return nil
	}
	ids := make([]knowledge.ChunkID, 0, len(scores))
	for id := range scores {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool {
		if scores[ids[i]] == scores[ids[j]] {
			if tiebreak != nil {
				return tiebreak(ids[i], ids[j])
			}
			return ids[i] < ids[j]
		}
		return scores[ids[i]] > scores[ids[j]]
	})
	return ids
}

func clampFloat(v, min, max float64) float64 {
	return math.Max(min, math.Min(max, v))
}
