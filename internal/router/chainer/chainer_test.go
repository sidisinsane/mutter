package chainer_test

import (
	"errors"
	"testing"

	"github.com/sidisinsane/mutter/internal/hashfm"
	"github.com/sidisinsane/mutter/internal/router/chainer"
)

// makeBlock is a test helper that builds a minimal hashfm.Block.
func makeBlock(name string, input, output []hashfm.TypeCategory) *hashfm.Block {
	var t *hashfm.Type
	if input != nil || output != nil {
		t = &hashfm.Type{Input: input, Output: output}
	}
	return &hashfm.Block{
		Name: name,
		Commands: []hashfm.Command{
			{Description: name, Usage: name, Type: t},
		},
	}
}

func TestBuild_SingleBlock_NoValidation(t *testing.T) {
	c := chainer.New()
	block := makeBlock("hello", nil, []hashfm.TypeCategory{hashfm.TypeCategoryText})

	chain, err := c.Build([]*hashfm.Block{block})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(chain.Commands) != 1 {
		t.Errorf("expected 1 command, got %d", len(chain.Commands))
	}
}

func TestBuild_EmptyBlocks_ReturnsError(t *testing.T) {
	c := chainer.New()
	_, err := c.Build(nil)
	if err == nil {
		t.Error("expected error for empty blocks, got nil")
	}
}

func TestBuild_BlockWithNoCommands_ReturnsError(t *testing.T) {
	c := chainer.New()
	block := &hashfm.Block{Name: "empty"}
	_, err := c.Build([]*hashfm.Block{block})
	if err == nil {
		t.Error("expected error for block with no commands, got nil")
	}
}

func TestBuild_CompatibleChain_Succeeds(t *testing.T) {
	c := chainer.New()
	downloader := makeBlock("downloader",
		nil,
		[]hashfm.TypeCategory{hashfm.TypeCategoryVideo},
	)
	converter := makeBlock("converter",
		[]hashfm.TypeCategory{hashfm.TypeCategoryVideo},
		[]hashfm.TypeCategory{hashfm.TypeCategoryAudio},
	)

	chain, err := c.Build([]*hashfm.Block{downloader, converter})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(chain.Commands) != 2 {
		t.Errorf("expected 2 commands, got %d", len(chain.Commands))
	}
}

func TestBuild_IncompatibleTypes_ReturnsValidationError(t *testing.T) {
	c := chainer.New()
	videoOut := makeBlock("video-tool",
		nil,
		[]hashfm.TypeCategory{hashfm.TypeCategoryVideo},
	)
	textIn := makeBlock("text-tool",
		[]hashfm.TypeCategory{hashfm.TypeCategoryText},
		nil,
	)

	_, err := c.Build([]*hashfm.Block{videoOut, textIn})
	if err == nil {
		t.Error("expected validation error for incompatible types, got nil")
	}

	var valErr *chainer.ValidationError
	if !errors.As(err, &valErr) {
		t.Errorf("expected *chainer.ValidationError, got %T: %v", err, err)
	}
}

func TestBuild_NoTypeDeclarations_SkipsValidation(t *testing.T) {
	c := chainer.New()
	a := makeBlock("a", nil, nil)
	b := makeBlock("b", nil, nil)

	_, err := c.Build([]*hashfm.Block{a, b})
	if err != nil {
		t.Errorf("expected no error when types are omitted, got: %v", err)
	}
}

func TestBuild_MultipleCategories_IntersectionSuffices(t *testing.T) {
	c := chainer.New()
	multi := makeBlock("multi-out",
		nil,
		[]hashfm.TypeCategory{hashfm.TypeCategoryVideo, hashfm.TypeCategoryAudio},
	)
	audioIn := makeBlock("audio-in",
		[]hashfm.TypeCategory{hashfm.TypeCategoryAudio},
		nil,
	)

	_, err := c.Build([]*hashfm.Block{multi, audioIn})
	if err != nil {
		t.Errorf("expected no error for overlapping categories, got: %v", err)
	}
}
