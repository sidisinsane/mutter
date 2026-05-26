package indexer_test

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/sidisinsane/mutter/internal/config"
	"github.com/sidisinsane/mutter/internal/hashfm/indexer"
)

// repoRoot returns the absolute path to the repository root.
func repoRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("could not determine test file path")
	}
	// internal/hashfm/indexer/ -> repo root (three levels up)
	return filepath.Join(filepath.Dir(file), "..", "..", "..")
}

// testConfig builds a config pointing at the given directory with default
// extension mappings and no embedder.
func testConfig(t *testing.T, dir string, recursive bool) *config.Config {
	t.Helper()
	return &config.Config{
		WorkspaceRoot: dir,
		Discovery: config.DiscoveryConfig{
			Paths:     []string{"."},
			Recursive: recursive,
		},
		Extensions: map[string][]string{
			"#":  {"sh", "rb", "py"},
			"//": {"go", "js", "ts"},
		},
	}
}

// TestBuild_ExamplesDir_IndexesBothScripts verifies that the indexer discovers
// both example scripts and populates the index correctly.
func TestBuild_ExamplesDir_IndexesBothScripts(t *testing.T) {
	examplesDir := filepath.Join(repoRoot(t), "examples")
	if _, err := os.Stat(examplesDir); err != nil {
		t.Skipf("examples/ directory not found: %v", err)
	}

	cfg := testConfig(t, examplesDir, false)
	idx, err := indexer.Build(cfg)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}

	if len(idx.Entries) != 2 {
		t.Errorf("expected 2 entries, got %d", len(idx.Entries))
	}

	// Verify both scripts are present by name
	names := make(map[string]bool)
	for _, entry := range idx.Entries {
		names[entry.Block.Name] = true
	}
	for _, expected := range []string{"hello.sh", "convert-video.sh"} {
		if !names[expected] {
			t.Errorf("expected %q in index, not found", expected)
		}
	}
}

// TestBuild_ExamplesDir_ParsedMetadataCorrect verifies the parsed content of
// the indexed hello.sh entry.
func TestBuild_ExamplesDir_ParsedMetadataCorrect(t *testing.T) {
	examplesDir := filepath.Join(repoRoot(t), "examples")
	if _, err := os.Stat(examplesDir); err != nil {
		t.Skipf("examples/ directory not found: %v", err)
	}

	cfg := testConfig(t, examplesDir, false)
	idx, err := indexer.Build(cfg)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}

	// Find hello.sh entry
	var helloEntry *indexer.Entry
	for _, entry := range idx.Entries {
		if entry.Block.Name == "hello.sh" {
			helloEntry = entry
			break
		}
	}
	if helloEntry == nil {
		t.Fatal("hello.sh not found in index")
	}

	cmd := helloEntry.Block.Commands[0]
	if cmd.Description != "Print a hello message" {
		t.Errorf("unexpected description: %q", cmd.Description)
	}
	if cmd.Type == nil {
		t.Error("expected type to be indexed")
	}
}

// TestBuild_EmptyDir_ReturnsEmptyIndex verifies that a directory with no
// supported scripts produces an empty index without error.
func TestBuild_EmptyDir_ReturnsEmptyIndex(t *testing.T) {
	dir := t.TempDir()
	// Write an unsupported file
	os.WriteFile(filepath.Join(dir, "readme.txt"), []byte("hello"), 0o644)

	cfg := testConfig(t, dir, false)
	idx, err := indexer.Build(cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(idx.Entries) != 0 {
		t.Errorf("expected empty index, got %d entries", len(idx.Entries))
	}
}

// TestBuild_ScriptWithoutHashfmBlock_Skipped verifies that scripts without a
// hashfm block are silently skipped.
func TestBuild_ScriptWithoutHashfmBlock_Skipped(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "noblk.sh"), []byte("#!/usr/bin/env bash\necho hi\n"), 0o755)

	cfg := testConfig(t, dir, false)
	idx, err := indexer.Build(cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(idx.Entries) != 0 {
		t.Errorf("expected script without block to be skipped, got %d entries", len(idx.Entries))
	}
}

// TestBuild_Recursive_FindsNestedScripts verifies that recursive discovery
// descends into subdirectories.
func TestBuild_Recursive_FindsNestedScripts(t *testing.T) {
	dir := t.TempDir()
	subdir := filepath.Join(dir, "sub")
	os.MkdirAll(subdir, 0o755)

	script := `#!/usr/bin/env bash
# ---
# description: Nested script
# usage: nested.sh
# exits:
#   0: success
# ---
echo "nested"
`
	os.WriteFile(filepath.Join(subdir, "nested.sh"), []byte(script), 0o755)

	cfg := testConfig(t, dir, true)
	idx, err := indexer.Build(cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(idx.Entries) != 1 {
		t.Errorf("expected 1 entry from recursive scan, got %d", len(idx.Entries))
	}
}

// TestBuild_NonRecursive_IgnoresNestedScripts verifies that non-recursive
// discovery does not descend into subdirectories.
func TestBuild_NonRecursive_IgnoresNestedScripts(t *testing.T) {
	dir := t.TempDir()
	subdir := filepath.Join(dir, "sub")
	os.MkdirAll(subdir, 0o755)

	script := `#!/usr/bin/env bash
# ---
# description: Nested script
# usage: nested.sh
# exits:
#   0: success
# ---
echo "nested"
`
	os.WriteFile(filepath.Join(subdir, "nested.sh"), []byte(script), 0o755)

	cfg := testConfig(t, dir, false)
	idx, err := indexer.Build(cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(idx.Entries) != 0 {
		t.Errorf("expected 0 entries for non-recursive scan, got %d", len(idx.Entries))
	}
}

// TestBuild_WithMockEmbedder_PopulatesEmbeddings verifies that when an
// embedder is provided, index entries are populated with non-nil embeddings.
func TestBuild_WithMockEmbedder_PopulatesEmbeddings(t *testing.T) {
	examplesDir := filepath.Join(repoRoot(t), "examples")
	if _, err := os.Stat(examplesDir); err != nil {
		t.Skipf("examples/ directory not found: %v", err)
	}

	cfg := testConfig(t, examplesDir, false)
	idx, err := indexer.BuildWithEmbedder(cfg, &mockEmbedder{dims: 4})
	if err != nil {
		t.Fatalf("BuildWithEmbedder: %v", err)
	}

	for path, entry := range idx.Entries {
		if entry.Embedding == nil {
			t.Errorf("expected embedding for %s, got nil", path)
		}
		if len(entry.Embedding) != 4 {
			t.Errorf("expected embedding dims=4 for %s, got %d", path, len(entry.Embedding))
		}
	}
}

// mockEmbedder implements embedder.Embedder with a fixed-dimension zero vector.
type mockEmbedder struct {
	dims int
}

func (m *mockEmbedder) Embed(_ context.Context, _ string) ([]float32, error) {
	return make([]float32, m.dims), nil
}

func (m *mockEmbedder) Dimensions() int  { return m.dims }
func (m *mockEmbedder) Close() error     { return nil }
