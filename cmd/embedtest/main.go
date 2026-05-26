//go:build ignore

package main

import (
	"context"
	"fmt"
	"log"
	"math"

	"github.com/sidisinsane/mutter/internal/router/embedder"
)

func cosine(a, b []float32) float64 {
	var dot, n1, n2 float64
	for i := range a {
		dot += float64(a[i]) * float64(b[i])
		n1 += float64(a[i]) * float64(a[i])
		n2 += float64(b[i]) * float64(b[i])
	}
	return dot / (math.Sqrt(n1) * math.Sqrt(n2))
}

func main() {
	emb, err := embedder.New(embedder.Config{
		ModelPath:          "~/.mutter/models/all-MiniLM-L6-v2.onnx",
		LibraryPath:        "",
		ExpectedDimensions: 384,
	})
	if err != nil {
		log.Fatalf("create embedder: %v", err)
	}
	defer emb.Close()

	ctx := context.Background()
	query := "print a hello message"

	// What the indexer currently embeds (description + usage)
	descAndUsage := "Print a hello message hello.sh --name {{name}}"
	// Description only
	descOnly := "Print a hello message"
	// Unrelated
	unrelated := "Convert video files to different formats using ffmpeg"

	vQuery, _ := emb.Embed(ctx, query)
	vDescAndUsage, _ := emb.Embed(ctx, descAndUsage)
	vDescOnly, _ := emb.Embed(ctx, descOnly)
	vUnrelated, _ := emb.Embed(ctx, unrelated)

	fmt.Printf("query vs description+usage : %.4f\n", cosine(vQuery, vDescAndUsage))
	fmt.Printf("query vs description only  : %.4f\n", cosine(vQuery, vDescOnly))
	fmt.Printf("query vs unrelated         : %.4f\n", cosine(vQuery, vUnrelated))
}
