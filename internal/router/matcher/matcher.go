// Package matcher provides cosine similarity matching for semantic routing.
package matcher

import (
	"math"
)

// Match represents a single routing match with its confidence score.
type Match struct {
	// Index is the index of the matched entry in the index.
	Index int
	// Score is the cosine similarity score (0.0 to 1.0).
	Score float64
}

// Matcher performs cosine similarity matching between query embeddings and
// indexed script embeddings.
type Matcher struct {
	// threshold is the minimum confidence score required for a match.
	threshold float64
}

// New creates a new Matcher with the given confidence threshold.
func New(threshold float64) *Matcher {
	return &Matcher{
		threshold: threshold,
	}
}

// FindBest finds the best matching entry for a query embedding against
// a set of candidate embeddings. Returns nil if no match meets the threshold.
func (m *Matcher) FindBest(query []float32, candidates [][]float32) *Match {
	var best *Match
	bestScore := 0.0

	for i, candidate := range candidates {
		score := cosineSimilarity(query, candidate)
		if score >= m.threshold && score > bestScore {
			best = &Match{
				Index: i,
				Score: score,
			}
			bestScore = score
		}
	}

	return best
}

// FindAll finds all matches that meet the confidence threshold, sorted by
// score in descending order.
func (m *Matcher) FindAll(query []float32, candidates [][]float32) []Match {
	var matches []Match

	for i, candidate := range candidates {
		score := cosineSimilarity(query, candidate)
		if score >= m.threshold {
			matches = append(matches, Match{
				Index: i,
				Score: score,
			})
		}
	}

	return matches
}

// cosineSimilarity computes the cosine similarity between two vectors.
// Returns a value between 0.0 and 1.0 for normalized vectors.
func cosineSimilarity(a, b []float32) float64 {
	if len(a) != len(b) {
		return 0.0
	}

	var dotProduct, normA, normB float64
	for i := 0; i < len(a); i++ {
		dotProduct += float64(a[i]) * float64(b[i])
		normA += float64(a[i]) * float64(a[i])
		normB += float64(b[i]) * float64(b[i])
	}

	if normA == 0 || normB == 0 {
		return 0.0
	}

	return dotProduct / (math.Sqrt(normA) * math.Sqrt(normB))
}
