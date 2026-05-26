// Package chainer builds command DAGs and validates type intersections for
// chained command execution.
package chainer

import (
	"fmt"

	"github.com/sidisinsane/mutter/internal/hashfm"
)

// Chain represents a validated chain of commands to be executed in sequence.
type Chain struct {
	// Commands is the ordered list of commands in the chain.
	Commands []*hashfm.Command
	// Blocks is the ordered list of source hashfm blocks for each command.
	Blocks []*hashfm.Block
}

// ValidationError indicates a type compatibility error between chained commands.
type ValidationError struct {
	// FromIndex is the index of the upstream command.
	FromIndex int
	// ToIndex is the index of the downstream command.
	ToIndex int
	// Reason describes the type mismatch.
	Reason string
}

// Error returns a human-readable description of the validation error.
func (e *ValidationError) Error() string {
	return fmt.Sprintf("chain validation failed between commands %d and %d: %s",
		e.FromIndex, e.ToIndex, e.Reason)
}

// Chainer validates and builds command chains based on type compatibility.
type Chainer struct{}

// New creates a new Chainer.
func New() *Chainer {
	return &Chainer{}
}

// Build validates a sequence of blocks and returns an executable Chain if all
// adjacent type pairs are compatible. The output types of each command must
// intersect with the input types of the next command.
func (c *Chainer) Build(blocks []*hashfm.Block) (*Chain, error) {
	if len(blocks) == 0 {
		return nil, fmt.Errorf("empty command chain")
	}

	for _, block := range blocks {
		if len(block.Commands) == 0 {
			return nil, fmt.Errorf("block %q has no commands", block.Name)
		}
	}

	chain := &Chain{
		Blocks:   blocks,
		Commands: make([]*hashfm.Command, 0, len(blocks)),
	}

	for i, block := range blocks {
		cmd := &block.Commands[0]
		if i > 0 {
			prev := &blocks[i-1].Commands[0]
			if err := validateTypeCompatibility(prev, cmd, i-1, i); err != nil {
				return nil, err
			}
		}
		chain.Commands = append(chain.Commands, cmd)
	}

	return chain, nil
}

// validateTypeCompatibility checks that the output types of the upstream
// command intersect with the input types of the downstream command.
// If either command omits type declarations, validation is skipped.
func validateTypeCompatibility(from, to *hashfm.Command, fromIdx, toIdx int) error {
	if from.Type == nil || to.Type == nil {
		return nil
	}

	if len(from.Type.Output) == 0 || len(to.Type.Input) == 0 {
		return &ValidationError{
			FromIndex: fromIdx,
			ToIndex:   toIdx,
			Reason:    "missing type declarations for chaining",
		}
	}

	if !hasIntersection(from.Type.Output, to.Type.Input) {
		return &ValidationError{
			FromIndex: fromIdx,
			ToIndex:   toIdx,
			Reason: fmt.Sprintf("type mismatch: %v output doesn't intersect with %v input",
				from.Type.Output, to.Type.Input),
		}
	}

	return nil
}

// hasIntersection reports whether two type category slices share any element.
func hasIntersection(a, b []hashfm.TypeCategory) bool {
	for _, x := range a {
		for _, y := range b {
			if x == y {
				return true
			}
		}
	}
	return false
}
