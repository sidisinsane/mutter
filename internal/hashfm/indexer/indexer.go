// Package indexer traverses the filesystem to discover hashfm-annotated scripts
// and compiles an index of their metadata for the router.
package indexer

import (
	"context"
	"fmt"
	"log"
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
	// Embedding is the vector representation of the script's description,
	// used for semantic similarity matching. Populated by the embedder.
	// Only the description is embedded — not the usage — to maximise
	// cosine similarity between natural language queries and indexed scripts.
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
		path := filepath.Join(cfg.WorkspaceRoot, root)
		log.Printf("indexer: walking path %s", path)

		err := filepath.Walk(path, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				log.Printf("indexer: skipping inaccessible path %s: %v", path, err)
				return nil
			}

			if info.IsDir() {
				if path != filepath.Join(cfg.WorkspaceRoot, root) && !cfg.Discovery.Recursive {
					log.Printf("indexer: skipping directory %s (not recursive)", path)
					return filepath.SkipDir
				}
				return nil
			}

			ext := strings.TrimPrefix(filepath.Ext(path), ".")
			if !hasSupportedExtension(ext, cfg.Extensions) {
				return nil
			}

			log.Printf("indexer: parsing hashfm block from %s", path)

			block, err := parser.ParseFile(path, cfg)
			if err != nil {
				log.Printf("indexer: failed to parse %s: %v", path, err)
				return nil
			}

			log.Printf("indexer: successfully parsed %s", path)

			entry := &Entry{Block: block}

			if emb != nil && len(block.Commands) > 0 {
				// Embed the description only — not the usage string.
				// Mixing usage syntax into the embedding dilutes semantic
				// similarity against natural language queries.
				text := block.Commands[0].Description
				embedding, err := emb.Embed(ctx, text)
				if err != nil {
					log.Printf("indexer: failed to embed %s: %v", path, err)
				} else {
					entry.Embedding = embedding
					log.Printf("indexer: generated embedding for %s", path)
				}
			}

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
