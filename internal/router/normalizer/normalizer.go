// Package normalizer converts natural language chain expressions into
// executable pipe syntax for command chaining.
package normalizer

import (
	"regexp"
	"strings"
)

// Normalizer converts natural language chain terms to pipe syntax.
type Normalizer struct {
	// chainPatterns maps natural language terms to pipe syntax.
	// The keys are regex patterns to match in the input.
	chainPatterns []*regexp.Regexp
}

// New creates a new Normalizer with default chain patterns.
func New() *Normalizer {
	return &Normalizer{
		chainPatterns: []*regexp.Regexp{
			// Match "X and then Y", "X and Y", "X then Y"
			regexp.MustCompile(`(?i)\s+and\s+(then\s+)?`),
			regexp.MustCompile(`(?i)\s+then\s+`),
			// Match "X | Y" (already pipe syntax, but normalize spacing)
			regexp.MustCompile(`\s*\|\s*`),
		},
	}
}

// Normalize converts a natural language prompt into a normalized form
// with pipe syntax for chaining. Returns the normalized prompt and whether
// it contains chained commands.
func (n *Normalizer) Normalize(prompt string) (string, bool) {
	normalized := prompt
	hasChain := false

	// Replace natural language chain terms with pipe symbol
	for _, pattern := range n.chainPatterns {
		if pattern.MatchString(normalized) {
			hasChain = true
			// Don't replace if it's already a pipe pattern
			if pattern == n.chainPatterns[len(n.chainPatterns)-1] {
				// Normalize pipe spacing
				normalized = pattern.ReplaceAllString(normalized, " | ")
			} else {
				normalized = pattern.ReplaceAllString(normalized, " | ")
			}
		}
	}

	return strings.TrimSpace(normalized), hasChain
}

// SplitChains splits a normalized prompt into individual command segments
// based on pipe syntax.
func (n *Normalizer) SplitChains(normalized string) []string {
	parts := strings.Split(normalized, "|")
	segments := make([]string, 0, len(parts))

	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed != "" {
			segments = append(segments, trimmed)
		}
	}

	return segments
}
