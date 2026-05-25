// Package embedder provides semantic embedding generation using ONNX sentence
// transformer models for script matching.
package embedder

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/yalue/onnxruntime_go"
)

// Model URLs
const (
	// DefaultModelURL is the HuggingFace URL for the all-MiniLM-L6-v2 ONNX model
	DefaultModelURL = "https://huggingface.co/sentence-transformers/all-MiniLM-L6-v2/resolve/main/onnx/model.onnx"
)

// ONNXEmbedder implements the Embedder interface using an ONNX sentence transformer model.
type ONNXEmbedder struct {
	modelPath   string
	dimensions  int
	session     *onnxruntime_go.AdvancedSession
	initialized bool
}

// NewONNXEmbedder creates a new ONNX-based embedder.
func NewONNXEmbedder(modelPath string, expectedDims int) (*ONNXEmbedder, error) {
	// Expand home directory in path
	modelPath = expandHome(modelPath)

	// Ensure model exists
	if err := ensureModel(modelPath); err != nil {
		return nil, fmt.Errorf("ensure model: %w", err)
	}

	emb := &ONNXEmbedder{
		modelPath:  modelPath,
		dimensions: expectedDims,
	}

	// TODO: Initialize ONNX session for actual inference
	// For now, using simple embedding approach

	return emb, nil
}

// Embed generates an embedding vector for the given text.
// This implementation uses a simple character n-gram approach for now.
// TODO: Implement actual ONNX inference with tokenization.
func (e *ONNXEmbedder) Embed(ctx context.Context, text string) ([]float32, error) {
	// Simple character n-gram embedding for demonstration
	// This creates a reproducible embedding based on character bigrams
	embedding := make([]float32, e.dimensions)

	// Normalize text
	text = strings.ToLower(text)

	// Create character bigram features
	bigrams := make(map[string]int)
	for i := 0; i < len(text)-1; i++ {
		bigram := text[i : i+2]
		bigrams[bigram]++
	}

	// Hash bigrams to embedding dimensions
	for bigram, count := range bigrams {
		hash := sha256.Sum256([]byte(bigram))
		for j := 0; j < len(hash) && j < e.dimensions; j++ {
			val := float32(hash[j]) * float32(count) / 255.0
			embedding[j%e.dimensions] += val
		}
	}

	// Normalize the embedding
	var norm float32
	for _, v := range embedding {
		norm += v * v
	}
	norm = sqrt(norm)

	if norm > 0 {
		for i := range embedding {
			embedding[i] /= norm
		}
	}

	return embedding, nil
}

// Dimensions returns the dimensionality of the embedding vectors.
func (e *ONNXEmbedder) Dimensions() int {
	return e.dimensions
}

// Close releases the ONNX session resources.
func (e *ONNXEmbedder) Close() error {
	if e.session != nil {
		return e.session.Destroy()
	}
	return nil
}

// sqrt computes the square root of a float32 value.
func sqrt(x float32) float32 {
	// Simple Newton's method for sqrt
	if x <= 0 {
		return 0
	}
	z := x
	for i := 0; i < 10; i++ {
		z = 0.5 * (z + x/z)
	}
	return z
}

// ensureModel checks if the model file exists, downloads it if not.
func ensureModel(modelPath string) error {
	if _, err := os.Stat(modelPath); err == nil {
		return nil // Model exists
	}

	// Create directory if needed
	dir := filepath.Dir(modelPath)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create model directory: %w", err)
	}

	// Download model
	fmt.Printf("downloading model from %s to %s...\n", DefaultModelURL, modelPath)
	if err := downloadFile(DefaultModelURL, modelPath); err != nil {
		return fmt.Errorf("download model: %w", err)
	}

	fmt.Println("model downloaded successfully")
	return nil
}

// downloadFile downloads a file from URL to the given path.
func downloadFile(url, destPath string) error {
	resp, err := http.Get(url)
	if err != nil {
		return fmt.Errorf("http get: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("http status %d", resp.StatusCode)
	}

	out, err := os.Create(destPath)
	if err != nil {
		return fmt.Errorf("create file: %w", err)
	}
	defer out.Close()

	_, err = io.Copy(out, resp.Body)
	return err
}

// verifyChecksum verifies the SHA256 checksum of a file.
func verifyChecksum(filePath, expectedChecksum string) error {
	file, err := os.Open(filePath)
	if err != nil {
		return fmt.Errorf("open file: %w", err)
	}
	defer file.Close()

	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return fmt.Errorf("hash file: %w", err)
	}

	actual := hex.EncodeToString(hash.Sum(nil))
	if actual != expectedChecksum {
		return fmt.Errorf("checksum mismatch: expected %s, got %s", expectedChecksum, actual)
	}

	return nil
}

// expandHome expands ~ to the user's home directory in a path string.
func expandHome(path string) string {
	if strings.HasPrefix(path, "~/") || path == "~" {
		home, err := os.UserHomeDir()
		if err != nil {
			return path
		}
		return filepath.Join(home, path[2:])
	}
	return path
}
