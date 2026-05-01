package retrieval

import (
	"context"
	"math"

	"codeburg.org/lexbit/relurpify/framework/knowledge"
)

// KeywordRanker scores chunks with a BM25-style term model.
type KeywordRanker struct {
	K1 float64
	B  float64
}

func (r *KeywordRanker) Name() string { return "keyword" }

func (r *KeywordRanker) Rank(ctx context.Context, query RetrievalQuery, store *knowledge.ChunkStore) ([]knowledge.ChunkID, error) {
	_ = ctx
	if r == nil || store == nil {
		return nil, nil
	}
	queryTerms := tokenizeRankerText(query.Text)
	if len(queryTerms) == 0 {
		return nil, nil
	}
	chunks, err := loadRankerChunks(store)
	if err != nil || len(chunks) == 0 {
		return nil, err
	}

	k1 := r.K1
	if k1 <= 0 {
		k1 = 1.2
	}
	b := r.B
	if b <= 0 {
		b = 0.75
	}

	docTerms := make([][]string, 0, len(chunks))
	lengths := make([]float64, 0, len(chunks))
	termDocs := make(map[string]int)
	for _, chunk := range chunks {
		terms := tokenizeRankerText(chunk.Body.Raw)
		docTerms = append(docTerms, terms)
		lengths = append(lengths, float64(len(terms)))
		seen := make(map[string]struct{})
		for _, term := range terms {
			if _, ok := seen[term]; ok {
				continue
			}
			seen[term] = struct{}{}
			termDocs[term]++
		}
	}

	avgLen := 0.0
	for _, length := range lengths {
		avgLen += length
	}
	if len(lengths) > 0 {
		avgLen /= float64(len(lengths))
	}
	if avgLen <= 0 {
		avgLen = 1.0
	}

	scores := make(map[knowledge.ChunkID]float64, len(chunks))
	for i, chunk := range chunks {
		doc := docTerms[i]
		if len(doc) == 0 {
			continue
		}
		tf := make(map[string]int)
		for _, term := range doc {
			tf[term]++
		}
		score := 0.0
		for _, term := range queryTerms {
			freq := tf[term]
			if freq == 0 {
				continue
			}
			df := termDocs[term]
			idf := math.Log(1.0 + ((float64(len(chunks)) - float64(df) + 0.5) / (float64(df) + 0.5)))
			denom := float64(freq) + k1*(1.0-b+b*(float64(len(doc))/avgLen))
			score += idf * (float64(freq) * (k1 + 1.0) / denom)
		}
		if score > 0 {
			scores[chunk.ID] = score
		}
	}

	ids := sortRankedIDs(scores, func(a, b knowledge.ChunkID) bool { return a < b })
	return ids, nil
}
