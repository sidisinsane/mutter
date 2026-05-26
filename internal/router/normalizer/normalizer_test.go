package normalizer_test

import (
	"testing"

	"github.com/sidisinsane/mutter/internal/router/normalizer"
)

func TestNormalize_SingleCommand_NoChain(t *testing.T) {
	n := normalizer.New()
	out, hasChain := n.Normalize("convert the video file")
	if hasChain {
		t.Error("expected hasChain=false for single command")
	}
	if out != "convert the video file" {
		t.Errorf("unexpected output: %q", out)
	}
}

func TestNormalize_AndThen(t *testing.T) {
	n := normalizer.New()
	out, hasChain := n.Normalize("download the video and then convert it")
	if !hasChain {
		t.Error("expected hasChain=true")
	}
	if out != "download the video | convert it" {
		t.Errorf("unexpected output: %q", out)
	}
}

func TestNormalize_And(t *testing.T) {
	n := normalizer.New()
	out, hasChain := n.Normalize("download the video and convert it")
	if !hasChain {
		t.Error("expected hasChain=true")
	}
	if out != "download the video | convert it" {
		t.Errorf("unexpected output: %q", out)
	}
}

func TestNormalize_Then(t *testing.T) {
	n := normalizer.New()
	out, hasChain := n.Normalize("download the video then convert it")
	if !hasChain {
		t.Error("expected hasChain=true")
	}
	if out != "download the video | convert it" {
		t.Errorf("unexpected output: %q", out)
	}
}

func TestNormalize_ExistingPipe(t *testing.T) {
	n := normalizer.New()
	out, hasChain := n.Normalize("download|convert")
	if !hasChain {
		t.Error("expected hasChain=true for existing pipe")
	}
	if out != "download | convert" {
		t.Errorf("unexpected spacing normalisation: %q", out)
	}
}

func TestNormalize_CaseInsensitive(t *testing.T) {
	n := normalizer.New()
	_, hasChain := n.Normalize("download AND THEN convert")
	if !hasChain {
		t.Error("expected hasChain=true for uppercase AND THEN")
	}
}

func TestNormalize_TrimsWhitespace(t *testing.T) {
	n := normalizer.New()
	out, _ := n.Normalize("  convert the video  ")
	if out != "convert the video" {
		t.Errorf("expected trimmed output, got %q", out)
	}
}

func TestSplitChains_SplitsOnPipe(t *testing.T) {
	n := normalizer.New()
	segments := n.SplitChains("download the video | convert it | upload it")
	if len(segments) != 3 {
		t.Fatalf("expected 3 segments, got %d", len(segments))
	}
	expected := []string{"download the video", "convert it", "upload it"}
	for i, seg := range segments {
		if seg != expected[i] {
			t.Errorf("segment[%d]: expected %q, got %q", i, expected[i], seg)
		}
	}
}

func TestSplitChains_EmptySegmentsIgnored(t *testing.T) {
	n := normalizer.New()
	segments := n.SplitChains("download | | convert")
	if len(segments) != 2 {
		t.Errorf("expected 2 segments (empty ignored), got %d", len(segments))
	}
}
