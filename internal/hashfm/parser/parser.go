// Package parser extracts and parses hashfm metadata blocks from scripts.
package parser

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/sidisinsane/mutter/internal/config"
	"github.com/sidisinsane/mutter/internal/hashfm"
	"gopkg.in/yaml.v3"
)

// ErrNoHashfmBlock is returned when no hashfm block is found in a script.
var ErrNoHashfmBlock = errors.New("no hashfm block found in script")

// ErrUnsupportedExtension is returned when a file's extension is not mapped to a comment delimiter.
var ErrUnsupportedExtension = errors.New("unsupported file extension")

// ParseFile reads a script file and returns its parsed hashfm metadata block.
// The config is used to map file extensions to comment delimiters.
func ParseFile(filePath string, cfg *config.Config) (*hashfm.Block, error) {
	// Get file extension and resolve comment delimiter
	ext := strings.TrimPrefix(filepath.Ext(filePath), ".")
	delimiter, err := resolveDelimiter(ext, cfg.Extensions)
	if err != nil {
		return nil, err
	}

	// Read file contents
	lines, err := readLines(filePath)
	if err != nil {
		return nil, fmt.Errorf("read file: %w", err)
	}

	// Find hashfm block boundaries
	start, end, err := findBlockBoundaries(lines, delimiter)
	if err != nil {
		return nil, err
	}

	// Extract and clean YAML content from block
	rawYAML, err := extractYAML(lines[start+1:end], delimiter)
	if err != nil {
		return nil, fmt.Errorf("extract yaml: %w", err)
	}

	// Parse YAML into hashfm block
	block, err := parseYAML(rawYAML, filePath)
	if err != nil {
		return nil, fmt.Errorf("parse yaml: %w", err)
	}

	return block, nil
}

// resolveDelimiter maps a file extension to its comment delimiter using the config.
func resolveDelimiter(ext string, extensions map[string][]string) (string, error) {
	for delim, exts := range extensions {
		for _, e := range exts {
			if e == ext {
				return delim, nil
			}
		}
	}
	return "", fmt.Errorf("%w: .%s", ErrUnsupportedExtension, ext)
}

// readLines reads all lines from a file into a slice.
func readLines(filePath string) ([]string, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var lines []string
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}
	return lines, scanner.Err()
}

// findBlockBoundaries locates the start and end line indices of the hashfm block.
// Skips the shebang line if present.
func findBlockBoundaries(lines []string, delimiter string) (int, int, error) {
	marker := delimiter + " ---"
	start := -1

	// Skip shebang line if present
	offset := 0
	if len(lines) > 0 && strings.HasPrefix(lines[0], "#!") {
		offset = 1
	}

	// Find start marker
	for i := offset; i < len(lines); i++ {
		if strings.TrimSpace(lines[i]) == marker {
			start = i
			break
		}
	}
	if start == -1 {
		return 0, 0, ErrNoHashfmBlock
	}

	// Find end marker
	end := -1
	for i := start + 1; i < len(lines); i++ {
		if strings.TrimSpace(lines[i]) == marker {
			end = i
			break
		}
	}
	if end == -1 {
		return 0, 0, fmt.Errorf("unclosed hashfm block: missing closing %s", marker)
	}

	return start, end, nil
}

// extractYAML strips comment delimiters from block lines to produce raw YAML.
func extractYAML(lines []string, delimiter string) (string, error) {
	prefix := delimiter + " "
	var yamlLines []string
	for _, line := range lines {
		trimmed := strings.TrimPrefix(line, prefix)
		yamlLines = append(yamlLines, trimmed)
	}
	return strings.Join(yamlLines, "\n"), nil
}

// parseYAML unmarshals raw YAML into a hashfm.Block, detecting single vs multi-command form.
func parseYAML(rawYAML string, filePath string) (*hashfm.Block, error) {
	block := &hashfm.Block{
		Name: filepath.Base(filePath),
		Path: filePath,
	}

	// Try multi-command form first (array of commands)
	var multiCmd []hashfm.Command
	if err := yaml.Unmarshal([]byte(rawYAML), &multiCmd); err == nil && len(multiCmd) > 0 {
		block.Commands = multiCmd
		block.IsMultiCommand = true
		return block, nil
	}

	// Fall back to single-command form
	var singleCmd hashfm.Command
	if err := yaml.Unmarshal([]byte(rawYAML), &singleCmd); err != nil {
		return nil, fmt.Errorf("invalid hashfm block: %w", err)
	}
	block.Commands = []hashfm.Command{singleCmd}
	block.IsMultiCommand = false
	return block, nil
}
