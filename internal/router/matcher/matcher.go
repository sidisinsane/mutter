// Package matcher provides cosine similarity matching for semantic routing.
package matcher

import (
	"math"
	"sort"
)

// Match represents a single routing match with its confidence score.
type Match struct {
	// Index is the index of the matched entry in the candidates slice.
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
	return &Matcher{threshold: threshold}
}

// FindBest finds the single best matching entry for a query embedding.
// Returns nil if no candidate meets the threshold.
func (m *Matcher) FindBest(query []float32, candidates [][]float32) *Match {
	var best *Match
	bestScore := 0.0

	for i, candidate := range candidates {
		score := cosineSimilarity(query, candidate)
		if score >= m.threshold && score > bestScore {
			best = &Match{Index: i, Score: score}
			bestScore = score
		}
	}

	return best
}

// FindAll returns all candidates that meet the confidence threshold, sorted
// by score in descending order (highest confidence first).
func (m *Matcher) FindAll(query []float32, candidates [][]float32) []Match {
	var matches []Match

	for i, candidate := range candidates {
		score := cosineSimilarity(query, candidate)
		if score >= m.threshold {
			matches = append(matches, Match{Index: i, Score: score})
		}
	}

	sort.Slice(matches, func(i, j int) bool {
		return matches[i].Score > matches[j].Score
	})

	return matches
}

// cosineSimilarity computes the cosine similarity between two vectors.
// Returns 0.0 for zero-norm or mismatched-dimension vectors.
func cosineSimilarity(a, b []float32) float64 {
	if len(a) != len(b) {
		return 0.0
	}

	var dotProduct, normA, normB float64
	for i := range a {
		dotProduct += float64(a[i]) * float64(b[i])
		normA += float64(a[i]) * float64(a[i])
		normB += float64(b[i]) * float64(b[i])
	}

	if normA == 0 || normB == 0 {
		return 0.0
	}

	return dotProduct / (math.Sqrt(normA) * math.Sqrt(normB))
}
