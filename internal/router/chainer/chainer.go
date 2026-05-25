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
	// Blocks maps command indices to their source hashfm blocks.
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

// Build validates a chain of commands and returns an executable Chain if valid.
// Commands are validated for type compatibility: the output types of each
// command must intersect with the input types of the next command.
func (c *Chainer) Build(blocks []*hashfm.Block) (*Chain, error) {
	if len(blocks) == 0 {
		return nil, fmt.Errorf("empty command chain")
	}

	// For single commands, just validate the block has at least one command
	if len(blocks) == 1 {
		block := blocks[0]
		if len(block.Commands) == 0 {
			return nil, fmt.Errorf("block has no commands")
		}
		return &Chain{
			Commands: block.Commands,
			Blocks:   blocks,
		}, nil
	}

	// For chained commands, validate type compatibility
	chain := &Chain{
		Blocks: blocks,
	}

	for i := 0; i < len(blocks)-1; i++ {
		from := blocks[i]
		to := blocks[i+1]

		// Get the primary command from each block (first command for multi-command)
		fromCmd := from.Commands[0]
		toCmd := to.Commands[0]

		if err := validateTypeCompatibility(&fromCmd, &toCmd, i, i+1); err != nil {
			return nil, err
		}

		chain.Commands = append(chain.Commands, &fromCmd)
	}

	// Add the last command
	lastCmd := blocks[len(blocks)-1].Commands[0]
	chain.Commands = append(chain.Commands, &lastCmd)

	return chain, nil
}

// validateTypeCompatibility checks if the output types of the upstream command
// intersect with the input types of the downstream command.
func validateTypeCompatibility(from, to *hashfm.Command, fromIdx, toIdx int) error {
	// If either command doesn't declare types, skip validation
	if from.Type == nil || to.Type == nil {
		return nil
	}

	// If from has no output or to has no input, they can't be chained
	if len(from.Type.Output) == 0 || len(to.Type.Input) == 0 {
		return &ValidationError{
			FromIndex: fromIdx,
			ToIndex:   toIdx,
			Reason:    "missing type declarations for chaining",
		}
	}

	// Check if there's an intersection between from's output and to's input
	if !hasIntersection(from.Type.Output, to.Type.Input) {
		return &ValidationError{
			FromIndex: fromIdx,
			ToIndex:   toIdx,
			Reason:    fmt.Sprintf("type mismatch: %v output doesn't intersect with %v input",
				from.Type.Output, to.Type.Input),
		}
	}

	return nil
}

// hasIntersection checks if two type category slices have any common elements.
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
