package matcher_test

import (
	"math"
	"testing"

	"github.com/sidisinsane/mutter/internal/router/matcher"
)

// normalised returns a unit-length copy of v.
func normalised(v []float32) []float32 {
	var sum float64
	for _, x := range v {
		sum += float64(x) * float64(x)
	}
	norm := float32(math.Sqrt(sum))
	out := make([]float32, len(v))
	for i, x := range v {
		out[i] = x / norm
	}
	return out
}

func TestFindBest_ReturnsHighestScoreAboveThreshold(t *testing.T) {
	m := matcher.New(0.5)

	query := normalised([]float32{1, 0, 0})
	candidates := [][]float32{
		normalised([]float32{1, 0, 0}),   // identical — score 1.0
		normalised([]float32{0, 1, 0}),   // orthogonal — score 0.0
		normalised([]float32{0.9, 0.1, 0}), // close — score ~0.99
	}

	best := m.FindBest(query, candidates)
	if best == nil {
		t.Fatal("expected a match, got nil")
	}
	if best.Index != 0 {
		t.Errorf("expected index 0 (identical vector), got %d", best.Index)
	}
	if math.Abs(best.Score-1.0) > 1e-5 {
		t.Errorf("expected score ~1.0, got %f", best.Score)
	}
}

func TestFindBest_ReturnsNilWhenNoCandidatesAboveThreshold(t *testing.T) {
	m := matcher.New(0.9)

	query := normalised([]float32{1, 0, 0})
	candidates := [][]float32{
		normalised([]float32{0, 1, 0}), // score 0.0
		normalised([]float32{0, 0, 1}), // score 0.0
	}

	if best := m.FindBest(query, candidates); best != nil {
		t.Errorf("expected nil, got match with score %f", best.Score)
	}
}

func TestFindAll_ReturnsSortedByScore(t *testing.T) {
	m := matcher.New(0.0)

	query := normalised([]float32{1, 0, 0})
	candidates := [][]float32{
		normalised([]float32{0.5, 0.5, 0}), // moderate
		normalised([]float32{1, 0, 0}),     // identical
		normalised([]float32{0.8, 0.2, 0}), // high
	}

	matches := m.FindAll(query, candidates)
	if len(matches) != 3 {
		t.Fatalf("expected 3 matches, got %d", len(matches))
	}

	// Verify descending order
	for i := 1; i < len(matches); i++ {
		if matches[i].Score > matches[i-1].Score {
			t.Errorf("matches not sorted: [%d].Score=%f > [%d].Score=%f",
				i, matches[i].Score, i-1, matches[i-1].Score)
		}
	}
}

func TestFindAll_FiltersOnThreshold(t *testing.T) {
	m := matcher.New(0.8)

	query := normalised([]float32{1, 0, 0})
	candidates := [][]float32{
		normalised([]float32{1, 0, 0}),     // score 1.0 — above
		normalised([]float32{0.5, 0.5, 0}), // score ~0.71 — below
	}

	matches := m.FindAll(query, candidates)
	if len(matches) != 1 {
		t.Errorf("expected 1 match above threshold 0.8, got %d", len(matches))
	}
}

func TestFindAll_EmptyCandidates(t *testing.T) {
	m := matcher.New(0.5)
	query := normalised([]float32{1, 0, 0})

	matches := m.FindAll(query, nil)
	if len(matches) != 0 {
		t.Errorf("expected 0 matches for empty candidates, got %d", len(matches))
	}
}

func TestFindBest_DimensionMismatchReturnsNil(t *testing.T) {
	m := matcher.New(0.0)
	query := []float32{1, 0}
	candidates := [][]float32{{1, 0, 0}} // wrong dimensions

	if best := m.FindBest(query, candidates); best != nil {
		t.Errorf("expected nil for dimension mismatch, got score %f", best.Score)
	}
}
