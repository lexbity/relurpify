package retrieval

import (
	"math"
	"sort"

	"codeburg.org/lexbit/relurpify/framework/knowledge"
)

// RRF implements Reciprocal Rank Fusion.
// Given N ranked lists and weights, produces a single merged ranked list with RRF scores.
// Formula: RRF_score(d) = sum(weight_i / (k + rank_i(d)))
// where k is a constant (default 60) and rank_i(d) is the rank of document d in list i.
func RRF(rankedLists [][]knowledge.ChunkID, weights []float64, k float64) []RankedChunk {
	if k == 0 {
		k = 60.0 // Default RRF constant
	}

	// Collect all unique chunks and their scores
	chunkScores := make(map[knowledge.ChunkID]float64)
	chunkSources := make(map[knowledge.ChunkID][]string)

	for i, list := range rankedLists {
		weight := 1.0
		if i < len(weights) {
			weight = weights[i]
		}

		for rank, chunkID := range list {
			// RRF formula: weight / (k + rank)
			// rank is 0-indexed, so add 1 for 1-indexed ranking
			rrfScore := weight / (k + float64(rank+1))
			chunkScores[chunkID] += rrfScore

			// Track which rankers contributed
			source := "unknown"
			if i < len(weights) {
				source = "ranker"
			}
			chunkSources[chunkID] = append(chunkSources[chunkID], source)
		}
	}

	// Convert to slice for sorting
	result := make([]RankedChunk, 0, len(chunkScores))
	for chunkID, score := range chunkScores {
		result = append(result, RankedChunk{
			ChunkID: chunkID,
			Score:   score,
			Source:  "rrf",
		})
	}

	// Sort by score descending
	sort.Slice(result, func(i, j int) bool {
		return result[i].Score > result[j].Score
	})

	// Assign ranks after sorting
	for i := range result {
		result[i].Rank = i + 1
	}

	return result
}

// SimpleRRF is a convenience function that uses equal weights for all lists.
func SimpleRRF(rankedLists [][]knowledge.ChunkID) []RankedChunk {
	if len(rankedLists) == 0 {
		return nil
	}
	weights := make([]float64, len(rankedLists))
	for i := range weights {
		weights[i] = 1.0
	}
	return RRF(rankedLists, weights, 60.0)
}

// WeightedRRF uses custom weights for each ranked list.
func WeightedRRF(rankedLists [][]knowledge.ChunkID, weights []float64) []RankedChunk {
	return RRF(rankedLists, weights, 60.0)
}

// NormalizedRRF normalizes scores from different rankers before fusion.
func NormalizedRRF(rankedChunks [][]RankedChunk, weights []float64, k float64) []RankedChunk {
	if k == 0 {
		k = 60.0
	}

	chunkScores := make(map[knowledge.ChunkID]float64)

	for i, list := range rankedChunks {
		weight := 1.0
		if i < len(weights) {
			weight = weights[i]
		}

		// Find max score in this list for normalization
		maxScore := 0.0
		for _, chunk := range list {
			if chunk.Score > maxScore {
				maxScore = chunk.Score
			}
		}

		for rank, chunk := range list {
			normalizedWeight := weight
			if maxScore > 0 {
				normalizedWeight = weight * (chunk.Score / maxScore)
			}

			rrfScore := normalizedWeight / (k + float64(rank+1))
			chunkScores[chunk.ChunkID] += rrfScore
		}
	}

	result := make([]RankedChunk, 0, len(chunkScores))
	for chunkID, score := range chunkScores {
		result = append(result, RankedChunk{
			ChunkID: chunkID,
			Score:   score,
			Source:  "normalized_rrf",
		})
	}

	sort.Slice(result, func(i, j int) bool {
		return result[i].Score > result[j].Score
	})

	for i := range result {
		result[i].Rank = i + 1
	}

	return result
}

// ComputeRRFScore calculates the RRF score for a document given its ranks across multiple lists.
func ComputeRRFScore(ranks []int, weights []float64, k float64) float64 {
	if k == 0 {
		k = 60.0
	}

	score := 0.0
	for i, rank := range ranks {
		weight := 1.0
		if i < len(weights) {
			weight = weights[i]
		}

		if rank > 0 { // rank of 0 means not present in this list
			score += weight / (k + float64(rank))
		}
	}

	return score
}

// LogarithmicRRF uses logarithmic scaling to reduce the impact of top ranks.
func LogarithmicRRF(rankedLists [][]knowledge.ChunkID, weights []float64) []RankedChunk {
	chunkScores := make(map[knowledge.ChunkID]float64)

	for i, list := range rankedLists {
		weight := 1.0
		if i < len(weights) {
			weight = weights[i]
		}

		for rank, chunkID := range list {
			// Logarithmic damping
			logRank := math.Log1p(float64(rank + 1))
			rrfScore := weight / (1.0 + logRank)
			chunkScores[chunkID] += rrfScore
		}
	}

	result := make([]RankedChunk, 0, len(chunkScores))
	for chunkID, score := range chunkScores {
		result = append(result, RankedChunk{
			ChunkID: chunkID,
			Score:   score,
			Source:  "log_rrf",
		})
	}

	sort.Slice(result, func(i, j int) bool {
		return result[i].Score > result[j].Score
	})

	for i := range result {
		result[i].Rank = i + 1
	}

	return result
}
