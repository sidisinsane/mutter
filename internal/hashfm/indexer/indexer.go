// Package indexer traverses the filesystem to discover hashfm-annotated scripts
// and compiles an index of their metadata for the router.
package indexer

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/sidisinsane/mutter/internal/config"
	"github.com/sidisinsane/mutter/internal/hashfm"
	"github.com/sidisinsane/mutter/internal/hashfm/parser"
	"github.com/sidisinsane/mutter/internal/router/embedder"
)

// Entry represents a single indexed script with its metadata and embedding vector.
type Entry struct {
	// Block is the parsed hashfm metadata block from the script.
	Block *hashfm.Block
	// Embedding is the vector representation of the script's description/usage.
	// Populated by the embedder after indexing.
	Embedding []float32
}

// Index holds all discovered and parsed hashfm script entries.
type Index struct {
	// Entries maps script paths to their index entries.
	Entries map[string]*Entry
}

// NewIndex creates a new empty index.
func NewIndex() *Index {
	return &Index{
		Entries: make(map[string]*Entry),
	}
}

// Build traverses the configured paths and compiles an index of all
// hashfm-annotated scripts found.
func Build(cfg *config.Config) (*Index, error) {
	return BuildWithEmbedder(cfg, nil)
}

// BuildWithEmbedder traverses the configured paths and compiles an index of all
// hashfm-annotated scripts found, generating embeddings if an embedder is provided.
func BuildWithEmbedder(cfg *config.Config, emb embedder.Embedder) (*Index, error) {
	idx := NewIndex()
	ctx := context.Background()

	for _, root := range cfg.Discovery.Paths {
		// Resolve path relative to workspace root
		path := filepath.Join(cfg.WorkspaceRoot, root)
		fmt.Printf("indexer: walking path %s\n", path)

		err := filepath.Walk(path, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				fmt.Printf("indexer: skipping inaccessible path %s: %v\n", path, err)
				return nil // Skip inaccessible paths
			}

			// Skip directories unless we're doing recursive scan
			if info.IsDir() {
				if path != filepath.Join(cfg.WorkspaceRoot, root) && !cfg.Discovery.Recursive {
					fmt.Printf("indexer: skipping directory %s (not recursive)\n", path)
					return filepath.SkipDir
				}
				return nil
			}

			fmt.Printf("indexer: checking file %s\n", path)

			// Check if file has a supported extension
			ext := strings.TrimPrefix(filepath.Ext(path), ".")
			fmt.Printf("indexer: file extension is '%s'\n", ext)
			if !hasSupportedExtension(ext, cfg.Extensions) {
				fmt.Printf("indexer: extension '%s' not supported\n", ext)
				return nil
			}

			fmt.Printf("indexer: parsing hashfm block from %s\n", path)

			// Parse hashfm block
			block, err := parser.ParseFile(path, cfg)
			if err != nil {
				fmt.Printf("indexer: failed to parse %s: %v\n", path, err)
				// Skip files without valid hashfm blocks
				return nil
			}

			fmt.Printf("indexer: successfully parsed %s\n", path)

			// Create entry
			entry := &Entry{
				Block: block,
			}

			// Generate embedding if embedder is provided
			if emb != nil && len(block.Commands) > 0 {
				text := block.Commands[0].Description + " " + block.Commands[0].Usage
				embedding, err := emb.Embed(ctx, text)
				if err != nil {
					fmt.Printf("indexer: failed to embed %s: %v\n", path, err)
				} else {
					entry.Embedding = embedding
					fmt.Printf("indexer: generated embedding for %s\n", path)
				}
			}

			// Add to index
			idx.Entries[path] = entry

			return nil
		})

		if err != nil {
			return nil, fmt.Errorf("discover scripts in %s: %w", root, err)
		}
	}

	return idx, nil
}

// hasSupportedExtension checks if the given extension is mapped in the config.
func hasSupportedExtension(ext string, extensions map[string][]string) bool {
	for _, exts := range extensions {
		for _, e := range exts {
			if e == ext {
				return true
			}
		}
	}
	return false
}
