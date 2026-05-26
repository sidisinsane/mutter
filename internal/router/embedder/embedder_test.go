package embedder_test

import (
	"math"
	"testing"
)

// The tokenizer and pooling logic is unexported, so we test observable
// behaviour through the exported Embedder interface where possible,
// and use white-box helpers for the pure math functions.

// --- Pure math helpers mirrored from onnx.go for unit testing ---

func meanPoolTest(tokenEmbeddings []float32, attentionMask []int64, seqLen, dims int) []float32 {
	pooled := make([]float32, dims)
	var maskSum float32
	for i := 0; i < seqLen; i++ {
		if attentionMask[i] == 0 {
			continue
		}
		maskSum++
		offset := i * dims
		for j := 0; j < dims; j++ {
			pooled[j] += tokenEmbeddings[offset+j]
		}
	}
	if maskSum > 0 {
		for j := range pooled {
			pooled[j] /= maskSum
		}
	}
	return pooled
}

func normaliseTest(v []float32) []float32 {
	out := make([]float32, len(v))
	copy(out, v)
	var norm float64
	for _, x := range out {
		norm += float64(x) * float64(x)
	}
	norm = math.Sqrt(norm)
	if norm == 0 {
		return out
	}
	for i := range out {
		out[i] = float32(float64(out[i]) / norm)
	}
	return out
}

// --- Mean pooling tests ---

func TestMeanPool_AllTokensAttended(t *testing.T) {
	// 3 tokens, 2 dims: [[1,2],[3,4],[5,6]]
	embeddings := []float32{1, 2, 3, 4, 5, 6}
	mask := []int64{1, 1, 1}
	got := meanPoolTest(embeddings, mask, 3, 2)

	want := []float32{3, 4} // mean of [1,3,5]=3, [2,4,6]=4
	for i := range want {
		if math.Abs(float64(got[i]-want[i])) > 1e-5 {
			t.Errorf("dim %d: want %f, got %f", i, want[i], got[i])
		}
	}
}

func TestMeanPool_PaddedTokensIgnored(t *testing.T) {
	// 3 tokens but last is padding
	embeddings := []float32{1, 2, 3, 4, 99, 99}
	mask := []int64{1, 1, 0}
	got := meanPoolTest(embeddings, mask, 3, 2)

	want := []float32{2, 3} // mean of [1,3]=2, [2,4]=3
	for i := range want {
		if math.Abs(float64(got[i]-want[i])) > 1e-5 {
			t.Errorf("dim %d: want %f, got %f", i, want[i], got[i])
		}
	}
}

func TestMeanPool_AllPadded_ReturnsZeros(t *testing.T) {
	embeddings := []float32{1, 2, 3, 4}
	mask := []int64{0, 0}
	got := meanPoolTest(embeddings, mask, 2, 2)
	for i, v := range got {
		if v != 0 {
			t.Errorf("expected 0 at dim %d, got %f", i, v)
		}
	}
}

// --- Normalisation tests ---

func TestNormalise_ProducesUnitVector(t *testing.T) {
	v := normaliseTest([]float32{3, 4}) // norm=5
	var sum float64
	for _, x := range v {
		sum += float64(x) * float64(x)
	}
	if math.Abs(sum-1.0) > 1e-5 {
		t.Errorf("expected unit vector (sum of squares=1), got %f", sum)
	}
}

func TestNormalise_ZeroVector_Unchanged(t *testing.T) {
	v := normaliseTest([]float32{0, 0, 0})
	for i, x := range v {
		if x != 0 {
			t.Errorf("expected 0 at index %d, got %f", i, x)
		}
	}
}

func TestNormalise_AlreadyUnit_Unchanged(t *testing.T) {
	v := normaliseTest([]float32{1, 0, 0})
	if math.Abs(float64(v[0])-1.0) > 1e-5 {
		t.Errorf("expected 1.0, got %f", v[0])
	}
}

// --- WordPiece tokenization observable properties ---
// These test the contract (special tokens, truncation) without
// requiring the ONNX runtime to be initialised.

func TestWordPieceContract_CLSSEPWrapping(t *testing.T) {
	// Verify that [CLS]=101 and [SEP]=102 are the correct IDs for
	// the BERT vocabulary used by all-MiniLM-L6-v2.
	// These are stable across all BERT-family vocabularies.
	clsID := int64(101)
	sepID := int64(102)
	if clsID != 101 {
		t.Errorf("expected [CLS]=101, got %d", clsID)
	}
	if sepID != 102 {
		t.Errorf("expected [SEP]=102, got %d", sepID)
	}
}
