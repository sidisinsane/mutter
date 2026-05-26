// Package embedder provides semantic embedding generation using ONNX sentence
// transformer models for script matching.
package embedder

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"

	ort "github.com/yalue/onnxruntime_go"
)

const (
	// onnxruntimeVersion is the version of the onnxruntime shared library
	// required by onnxruntime_go v1.30.1.
	onnxruntimeVersion = "1.25.0"

	// DefaultModelURL is the HuggingFace URL for the all-MiniLM-L6-v2 ONNX model.
	DefaultModelURL = "https://huggingface.co/sentence-transformers/all-MiniLM-L6-v2/resolve/main/onnx/model.onnx"

	// maxTokens is the maximum sequence length for all-MiniLM-L6-v2.
	maxTokens = 128
)

// ortOnce ensures the ONNX runtime environment is initialised exactly once.
var ortOnce sync.Once

// initORT initialises the ONNX runtime environment on first call.
// Panics if initialisation fails — a broken runtime is not recoverable.
func initORT() {
	ortOnce.Do(func() {
		if err := ort.InitializeEnvironment(); err != nil {
			panic(fmt.Sprintf("initialise onnxruntime environment: %v", err))
		}
	})
}

// ONNXEmbedder implements the Embedder interface using an ONNX sentence
// transformer model. It produces L2-normalised embeddings via mean pooling
// over the token embeddings output by the model.
type ONNXEmbedder struct {
	modelPath  string
	dimensions int
	vocab      map[string]int32
}

// NewONNXEmbedder creates a new ONNX-based embedder. It ensures both the
// onnxruntime shared library and the ONNX model are present, downloading them
// on first run if necessary. libraryPath overrides the default install location
// (~/.mutter/lib/) when non-empty.
func NewONNXEmbedder(modelPath, libraryPath string, expectedDims int) (*ONNXEmbedder, error) {
	modelPath = expandHome(modelPath)

	resolvedLib, err := ensureLibrary(libraryPath)
	if err != nil {
		return nil, fmt.Errorf("ensure onnxruntime library: %w", err)
	}
	ort.SetSharedLibraryPath(resolvedLib)

	if err := ensureModel(modelPath); err != nil {
		return nil, fmt.Errorf("ensure model: %w", err)
	}

	vocabPath := filepath.Join(filepath.Dir(modelPath), "vocab.txt")
	if err := ensureVocab(vocabPath); err != nil {
		return nil, fmt.Errorf("ensure vocab: %w", err)
	}

	vocab, err := loadVocab(vocabPath)
	if err != nil {
		return nil, fmt.Errorf("load vocab: %w", err)
	}

	initORT()

	return &ONNXEmbedder{
		modelPath:  modelPath,
		dimensions: expectedDims,
		vocab:      vocab,
	}, nil
}

// Embed generates an L2-normalised embedding vector for the given text using
// the all-MiniLM-L6-v2 ONNX model.
//
// The model accepts input_ids, attention_mask, and token_type_ids and outputs
// last_hidden_state [batch, seq, 384]. We mean pool over the sequence
// dimension (weighted by attention_mask) and L2-normalise the result.
func (e *ONNXEmbedder) Embed(_ context.Context, text string) ([]float32, error) {
	inputIDs, attentionMask, tokenTypeIDs, err := e.tokenize(text)
	if err != nil {
		return nil, fmt.Errorf("tokenize: %w", err)
	}

	seqLen := int64(len(inputIDs))

	inputIDsTensor, err := ort.NewTensor(ort.NewShape(1, seqLen), inputIDs)
	if err != nil {
		return nil, fmt.Errorf("create input_ids tensor: %w", err)
	}
	defer inputIDsTensor.Destroy()

	attentionMaskTensor, err := ort.NewTensor(ort.NewShape(1, seqLen), attentionMask)
	if err != nil {
		return nil, fmt.Errorf("create attention_mask tensor: %w", err)
	}
	defer attentionMaskTensor.Destroy()

	tokenTypeIDsTensor, err := ort.NewTensor(ort.NewShape(1, seqLen), tokenTypeIDs)
	if err != nil {
		return nil, fmt.Errorf("create token_type_ids tensor: %w", err)
	}
	defer tokenTypeIDsTensor.Destroy()

	// last_hidden_state: [1, seqLen, dimensions]
	outputData := make([]float32, seqLen*int64(e.dimensions))
	outputTensor, err := ort.NewTensor(ort.NewShape(1, seqLen, int64(e.dimensions)), outputData)
	if err != nil {
		return nil, fmt.Errorf("create output tensor: %w", err)
	}
	defer outputTensor.Destroy()

	session, err := ort.NewAdvancedSession(
		e.modelPath,
		[]string{"input_ids", "attention_mask", "token_type_ids"},
		[]string{"last_hidden_state"},
		[]ort.ArbitraryTensor{inputIDsTensor, attentionMaskTensor, tokenTypeIDsTensor},
		[]ort.ArbitraryTensor{outputTensor},
		nil,
	)
	if err != nil {
		return nil, fmt.Errorf("create onnx session: %w", err)
	}
	defer session.Destroy()

	if err := session.Run(); err != nil {
		return nil, fmt.Errorf("run onnx session: %w", err)
	}

	embedding := meanPool(outputTensor.GetData(), attentionMask, int(seqLen), e.dimensions)
	normalise(embedding)

	return embedding, nil
}

// Dimensions returns the expected output dimensionality of the embedding vectors.
func (e *ONNXEmbedder) Dimensions() int {
	return e.dimensions
}

// Close releases any resources held by the embedder. Safe to call multiple times.
func (e *ONNXEmbedder) Close() error {
	return nil
}

// ensureLibrary returns the path to the onnxruntime shared library, downloading
// it from the official Microsoft GitHub release if necessary.
// If libraryPath is non-empty it is used directly (after ~ expansion) and
// existence is verified without downloading.
func ensureLibrary(libraryPath string) (string, error) {
	if libraryPath != "" {
		p := expandHome(libraryPath)
		if _, err := os.Stat(p); err != nil {
			return "", fmt.Errorf("configured library_path %q not found: %w", p, err)
		}
		return p, nil
	}

	destPath, err := defaultLibraryPath()
	if err != nil {
		return "", err
	}

	if _, err := os.Stat(destPath); err == nil {
		return destPath, nil
	}

	archiveURL, libPathInArchive, err := libraryArchiveURL()
	if err != nil {
		return "", err
	}

	if err := os.MkdirAll(filepath.Dir(destPath), 0o755); err != nil {
		return "", fmt.Errorf("create library directory: %w", err)
	}

	fmt.Printf("downloading onnxruntime library from %s\n", archiveURL)
	fmt.Printf("  extracting: %s\n", libPathInArchive)
	if err := downloadAndExtract(archiveURL, libPathInArchive, destPath); err != nil {
		return "", fmt.Errorf("download library: %w", err)
	}
	fmt.Println("onnxruntime library downloaded successfully")

	return destPath, nil
}

// defaultLibraryPath returns the default install path for the onnxruntime
// shared library on the current platform.
func defaultLibraryPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("home directory: %w", err)
	}
	filename, err := libraryFilename()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".mutter", "lib", filename), nil
}

// libraryFilename returns the versioned shared library filename for the
// current platform.
func libraryFilename() (string, error) {
	switch runtime.GOOS {
	case "darwin":
		return fmt.Sprintf("libonnxruntime.%s.dylib", onnxruntimeVersion), nil
	case "linux":
		return fmt.Sprintf("libonnxruntime.so.%s", onnxruntimeVersion), nil
	default:
		return "", fmt.Errorf("unsupported OS: %s", runtime.GOOS)
	}
}

// libraryArchiveURL returns the GitHub release archive URL and the path of the
// shared library within that archive for the current platform and architecture.
func libraryArchiveURL() (archiveURL, libPathInArchive string, err error) {
	const base = "https://github.com/microsoft/onnxruntime/releases/download"
	ver := onnxruntimeVersion

	var osName, archName, libName string

	switch runtime.GOOS {
	case "darwin":
		osName = "osx"
		libName = fmt.Sprintf("libonnxruntime.%s.dylib", ver)
	case "linux":
		osName = "linux"
		libName = fmt.Sprintf("libonnxruntime.so.%s", ver)
	default:
		return "", "", fmt.Errorf("unsupported OS: %s", runtime.GOOS)
	}

	switch runtime.GOARCH {
	case "arm64", "aarch64":
		if runtime.GOOS == "linux" {
			archName = "aarch64"
		} else {
			archName = "arm64"
		}
	case "amd64":
		if runtime.GOOS == "linux" {
			archName = "x64"
		} else {
			archName = "x86_64"
		}
	default:
		return "", "", fmt.Errorf("unsupported architecture: %s", runtime.GOARCH)
	}

	archiveName := fmt.Sprintf("onnxruntime-%s-%s-%s.tgz", osName, archName, ver)
	archiveURL = fmt.Sprintf("%s/v%s/%s", base, ver, archiveName)
	libPathInArchive = fmt.Sprintf("./onnxruntime-%s-%s-%s/lib/%s", osName, archName, ver, libName)

	return archiveURL, libPathInArchive, nil
}

// downloadAndExtract downloads a .tgz archive from url to a temporary file,
// then extracts the entry at targetPath within the archive to destPath.
// Using a temp file avoids partial writes if the connection drops mid-stream.
func downloadAndExtract(url, targetPath, destPath string) error {
	// Download archive to a temp file first.
	tmp, err := os.CreateTemp(filepath.Dir(destPath), "onnxruntime-*.tgz")
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)

	resp, err := http.Get(url) //nolint:gosec
	if err != nil {
		tmp.Close()
		return fmt.Errorf("http get: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		tmp.Close()
		return fmt.Errorf("unexpected http status %d for %s", resp.StatusCode, url)
	}

	if _, err := io.Copy(tmp, resp.Body); err != nil {
		tmp.Close()
		return fmt.Errorf("download archive: %w", err)
	}
	tmp.Close()

	// Extract the target file from the downloaded archive.
	f, err := os.Open(tmpPath)
	if err != nil {
		return fmt.Errorf("open archive: %w", err)
	}
	defer f.Close()

	gzr, err := gzip.NewReader(f)
	if err != nil {
		return fmt.Errorf("gzip reader: %w", err)
	}
	defer gzr.Close()

	tr := tar.NewReader(gzr)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("tar read: %w", err)
		}

		// Normalise entry name — some archives use a leading "./" prefix.
		entryName := strings.TrimPrefix(hdr.Name, "./")
		targetName := strings.TrimPrefix(targetPath, "./")
		if entryName != targetName {
			continue
		}

		out, err := os.OpenFile(destPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o755)
		if err != nil {
			return fmt.Errorf("create dest file: %w", err)
		}
		if _, err := io.Copy(out, tr); err != nil {
			out.Close()
			return fmt.Errorf("write library: %w", err)
		}
		return out.Close()
	}

	return fmt.Errorf("library %q not found in archive", targetPath)
}

// tokenize converts text into input_ids, attention_mask, and token_type_ids
// slices suitable for all-MiniLM-L6-v2. Sequences are truncated to maxTokens.
func (e *ONNXEmbedder) tokenize(text string) (inputIDs, attentionMask, tokenTypeIDs []int64, err error) {
	clsID := int64(e.vocab["[CLS]"])
	sepID := int64(e.vocab["[SEP]"])
	unkID := int64(e.vocab["[UNK]"])

	tokens := wordPieceTokenize(text, e.vocab, unkID, maxTokens-2)

	inputIDs = make([]int64, 0, len(tokens)+2)
	inputIDs = append(inputIDs, clsID)
	inputIDs = append(inputIDs, tokens...)
	inputIDs = append(inputIDs, sepID)

	seqLen := len(inputIDs)
	attentionMask = make([]int64, seqLen)
	tokenTypeIDs = make([]int64, seqLen)
	for i := range attentionMask {
		attentionMask[i] = 1
	}

	return inputIDs, attentionMask, tokenTypeIDs, nil
}

// wordPieceTokenize converts text to token IDs using WordPiece, truncated to maxLen.
func wordPieceTokenize(text string, vocab map[string]int32, unkID int64, maxLen int) []int64 {
	text = strings.ToLower(strings.TrimSpace(text))
	words := strings.Fields(text)

	var ids []int64
	for _, word := range words {
		if len(ids) >= maxLen {
			break
		}
		wordIDs := tokenizeWord(word, vocab, unkID)
		remaining := maxLen - len(ids)
		if len(wordIDs) > remaining {
			wordIDs = wordIDs[:remaining]
		}
		ids = append(ids, wordIDs...)
	}
	return ids
}

// tokenizeWord applies WordPiece to a single word, returning token IDs.
func tokenizeWord(word string, vocab map[string]int32, unkID int64) []int64 {
	if id, ok := vocab[word]; ok {
		return []int64{int64(id)}
	}

	var ids []int64
	start := 0
	for start < len(word) {
		end := len(word)
		found := false
		for end > start {
			substr := word[start:end]
			if start > 0 {
				substr = "##" + substr
			}
			if id, ok := vocab[substr]; ok {
				ids = append(ids, int64(id))
				start = end
				found = true
				break
			}
			end--
		}
		if !found {
			return []int64{unkID}
		}
	}
	return ids
}

// meanPool computes the mean of token embeddings weighted by attentionMask.
func meanPool(tokenEmbeddings []float32, attentionMask []int64, seqLen, dims int) []float32 {
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

// normalise divides each element of v by the L2 norm of v in place.
func normalise(v []float32) {
	var norm float64
	for _, x := range v {
		norm += float64(x) * float64(x)
	}
	norm = math.Sqrt(norm)
	if norm == 0 {
		return
	}
	for i := range v {
		v[i] = float32(float64(v[i]) / norm)
	}
}

// ensureModel checks if the model file exists and downloads it if not.
func ensureModel(modelPath string) error {
	if _, err := os.Stat(modelPath); err == nil {
		return nil
	}

	if err := os.MkdirAll(filepath.Dir(modelPath), 0o755); err != nil {
		return fmt.Errorf("create model directory: %w", err)
	}

	fmt.Printf("downloading model from %s\n", DefaultModelURL)
	if err := downloadFile(DefaultModelURL, modelPath); err != nil {
		return fmt.Errorf("download model: %w", err)
	}
	fmt.Println("model downloaded successfully")
	return nil
}

// ensureVocab checks if the vocabulary file exists and downloads it if not.
func ensureVocab(vocabPath string) error {
	if _, err := os.Stat(vocabPath); err == nil {
		return nil
	}

	const vocabURL = "https://huggingface.co/sentence-transformers/all-MiniLM-L6-v2/resolve/main/vocab.txt"
	fmt.Printf("downloading vocabulary from %s\n", vocabURL)
	if err := downloadFile(vocabURL, vocabPath); err != nil {
		return fmt.Errorf("download vocab: %w", err)
	}
	fmt.Println("vocabulary downloaded successfully")
	return nil
}

// loadVocab reads a vocab.txt file and returns a map from token to ID.
func loadVocab(vocabPath string) (map[string]int32, error) {
	data, err := os.ReadFile(vocabPath)
	if err != nil {
		return nil, fmt.Errorf("read vocab: %w", err)
	}

	vocab := make(map[string]int32)
	for i, line := range strings.Split(string(data), "\n") {
		if token := strings.TrimSpace(line); token != "" {
			vocab[token] = int32(i)
		}
	}

	if len(vocab) == 0 {
		return nil, fmt.Errorf("vocab file is empty: %s", vocabPath)
	}
	return vocab, nil
}

// downloadFile downloads url to destPath, streaming directly to disk.
func downloadFile(url, destPath string) error {
	resp, err := http.Get(url) //nolint:gosec
	if err != nil {
		return fmt.Errorf("http get: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected http status %d", resp.StatusCode)
	}

	out, err := os.Create(destPath)
	if err != nil {
		return fmt.Errorf("create file: %w", err)
	}
	defer out.Close()

	if _, err := io.Copy(out, resp.Body); err != nil {
		return fmt.Errorf("write file: %w", err)
	}
	return nil
}

// verifyChecksum verifies the SHA256 checksum of a file.
func verifyChecksum(filePath, expectedChecksum string) error {
	f, err := os.Open(filePath)
	if err != nil {
		return fmt.Errorf("open file: %w", err)
	}
	defer f.Close()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return fmt.Errorf("hash file: %w", err)
	}

	actual := hex.EncodeToString(h.Sum(nil))
	if actual != expectedChecksum {
		return fmt.Errorf("checksum mismatch: expected %s, got %s", expectedChecksum, actual)
	}
	return nil
}

// expandHome expands a leading ~ to the current user's home directory.
func expandHome(path string) string {
	if !strings.HasPrefix(path, "~/") && path != "~" {
		return path
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return path
	}
	return filepath.Join(home, path[2:])
}
