package parser_test

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/sidisinsane/mutter/internal/config"
	"github.com/sidisinsane/mutter/internal/hashfm"
	"github.com/sidisinsane/mutter/internal/hashfm/parser"
)

// testConfig returns a minimal config with the default extension map.
func testConfig() *config.Config {
	return &config.Config{
		Extensions: map[string][]string{
			"#":  {"sh", "rb", "py"},
			"//": {"go", "js", "ts"},
		},
	}
}

// writeScript writes content to a temp file with the given extension and
// returns its path.
func writeScript(t *testing.T, ext, content string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "script."+ext)
	if err := os.WriteFile(path, []byte(content), 0o755); err != nil {
		t.Fatalf("write script: %v", err)
	}
	return path
}

// --- Hash delimiter scripts (#) ---

func TestParseFile_HashDelimiter_SingleCommand(t *testing.T) {
	script := `#!/usr/bin/env bash
# ---
# description: Print a hello message
# usage: hello.sh --name {{name}}
# exits:
#   0: success
# ---
echo "hello"
`
	path := writeScript(t, "sh", script)
	block, err := parser.ParseFile(path, testConfig())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if block.Name != "script.sh" {
		t.Errorf("expected name 'script.sh', got %q", block.Name)
	}
	if block.Path != path {
		t.Errorf("expected path %q, got %q", path, block.Path)
	}
	if block.IsMultiCommand {
		t.Error("expected IsMultiCommand=false")
	}
	if len(block.Commands) != 1 {
		t.Fatalf("expected 1 command, got %d", len(block.Commands))
	}

	cmd := block.Commands[0]
	if cmd.Description != "Print a hello message" {
		t.Errorf("unexpected description: %q", cmd.Description)
	}
	if cmd.Usage != "hello.sh --name {{name}}" {
		t.Errorf("unexpected usage: %q", cmd.Usage)
	}
	if _, ok := cmd.Exits[0]; !ok {
		t.Error("expected exit code 0 to be present")
	}
}

func TestParseFile_HashDelimiter_WithTypeAndArguments(t *testing.T) {
	script := `#!/usr/bin/env bash
# ---
# description: Convert video files to different formats using ffmpeg
# usage: convert-video.sh --input {{input}} --output {{output}}
# type:
#   input: [video]
#   output: [video]
# arguments:
#   input:
#     pattern: '(?i)(?:convert|input)\s+(\S+)'
#     description: Input video file path
# exits:
#   0: success
#   1: ffmpeg not found
# ---
echo "convert"
`
	path := writeScript(t, "sh", script)
	block, err := parser.ParseFile(path, testConfig())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	cmd := block.Commands[0]

	if cmd.Type == nil {
		t.Fatal("expected Type to be set")
	}
	if len(cmd.Type.Input) != 1 || cmd.Type.Input[0] != hashfm.TypeCategoryVideo {
		t.Errorf("unexpected type.input: %v", cmd.Type.Input)
	}
	if len(cmd.Type.Output) != 1 || cmd.Type.Output[0] != hashfm.TypeCategoryVideo {
		t.Errorf("unexpected type.output: %v", cmd.Type.Output)
	}

	arg, ok := cmd.Arguments["input"]
	if !ok {
		t.Fatal("expected 'input' argument to be present")
	}
	if arg.Pattern == "" {
		t.Error("expected non-empty argument pattern")
	}
}

func TestParseFile_HashDelimiter_MultiCommand(t *testing.T) {
	script := `#!/usr/bin/env bash
# ---
# - description: Create a feature branch
#   usage: git-tools.sh feature
#   exits:
#     0: success
# - description: Prune merged branches
#   usage: git-tools.sh cleanup
#   exits:
#     0: success
# ---
echo "git tools"
`
	path := writeScript(t, "sh", script)
	block, err := parser.ParseFile(path, testConfig())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !block.IsMultiCommand {
		t.Error("expected IsMultiCommand=true")
	}
	if len(block.Commands) != 2 {
		t.Fatalf("expected 2 commands, got %d", len(block.Commands))
	}
	if block.Commands[0].Description != "Create a feature branch" {
		t.Errorf("unexpected first command description: %q", block.Commands[0].Description)
	}
	if block.Commands[1].Description != "Prune merged branches" {
		t.Errorf("unexpected second command description: %q", block.Commands[1].Description)
	}
}

// --- Double-slash delimiter scripts (//) ---

func TestParseFile_SlashDelimiter_SingleCommand(t *testing.T) {
	script := `// ---
// description: Compile and run a Go file
// usage: run.go
// exits:
//   0: success
// ---
package main
`
	path := writeScript(t, "go", script)
	block, err := parser.ParseFile(path, testConfig())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(block.Commands) != 1 {
		t.Fatalf("expected 1 command, got %d", len(block.Commands))
	}
	if block.Commands[0].Description != "Compile and run a Go file" {
		t.Errorf("unexpected description: %q", block.Commands[0].Description)
	}
}

// --- Error cases ---

func TestParseFile_NoHashfmBlock_ReturnsError(t *testing.T) {
	script := `#!/usr/bin/env bash
echo "no block here"
`
	path := writeScript(t, "sh", script)
	_, err := parser.ParseFile(path, testConfig())
	if !errors.Is(err, parser.ErrNoHashfmBlock) {
		t.Errorf("expected ErrNoHashfmBlock, got %v", err)
	}
}

func TestParseFile_UnclosedBlock_ReturnsError(t *testing.T) {
	script := `#!/usr/bin/env bash
# ---
# description: Missing closing marker
# usage: script.sh
echo "body"
`
	path := writeScript(t, "sh", script)
	_, err := parser.ParseFile(path, testConfig())
	if err == nil {
		t.Error("expected error for unclosed block, got nil")
	}
}

func TestParseFile_UnsupportedExtension_ReturnsError(t *testing.T) {
	path := writeScript(t, "lua", "-- some lua script")
	_, err := parser.ParseFile(path, testConfig())
	if !errors.Is(err, parser.ErrUnsupportedExtension) {
		t.Errorf("expected ErrUnsupportedExtension, got %v", err)
	}
}

func TestParseFile_NoShebang_StillParsed(t *testing.T) {
	// Scripts without a shebang line should still parse correctly.
	script := `# ---
# description: A script without a shebang
# usage: no-shebang.sh
# exits:
#   0: success
# ---
echo "hello"
`
	path := writeScript(t, "sh", script)
	block, err := parser.ParseFile(path, testConfig())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if block.Commands[0].Description != "A script without a shebang" {
		t.Errorf("unexpected description: %q", block.Commands[0].Description)
	}
}

// --- Example scripts ---

func TestParseFile_HelloSh_RealFile(t *testing.T) {
	// Resolve examples/hello.sh relative to this test file.
	path := filepath.Join("..", "..", "..", "examples", "hello.sh")
	if _, err := os.Stat(path); err != nil {
		t.Skipf("examples/hello.sh not found: %v", err)
	}

	block, err := parser.ParseFile(path, testConfig())
	if err != nil {
		t.Fatalf("unexpected error parsing hello.sh: %v", err)
	}

	if block.Name != "hello.sh" {
		t.Errorf("expected name 'hello.sh', got %q", block.Name)
	}
	if len(block.Commands) != 1 {
		t.Fatalf("expected 1 command, got %d", len(block.Commands))
	}
	cmd := block.Commands[0]
	if cmd.Description != "Print a hello message" {
		t.Errorf("unexpected description: %q", cmd.Description)
	}
	if cmd.Type == nil {
		t.Error("expected type declarations to be parsed")
	}
}

func TestParseFile_ConvertVideoSh_RealFile(t *testing.T) {
	path := filepath.Join("..", "..", "..", "examples", "convert-video.sh")
	if _, err := os.Stat(path); err != nil {
		t.Skipf("examples/convert-video.sh not found: %v", err)
	}

	block, err := parser.ParseFile(path, testConfig())
	if err != nil {
		t.Fatalf("unexpected error parsing convert-video.sh: %v", err)
	}

	cmd := block.Commands[0]
	if cmd.Type == nil {
		t.Fatal("expected type to be parsed")
	}
	if len(cmd.Arguments) == 0 {
		t.Error("expected arguments to be parsed")
	}
	if len(cmd.Exits) < 2 {
		t.Errorf("expected at least 2 exit codes, got %d", len(cmd.Exits))
	}
}
