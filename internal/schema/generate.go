//go:build ignore

package main

import (
	"fmt"
	"os"
	"path/filepath"
)

func main() {
	srcDir := filepath.Join("..", "..", "schema")
	files := []string{
		"hashfm-mutter.schema.json",
		"mutter-config.schema.json",
	}
	for _, name := range files {
		src := filepath.Join(srcDir, name)
		data, err := os.ReadFile(src)
		if err != nil {
			fmt.Fprintf(os.Stderr, "read %s: %v\n", src, err)
			os.Exit(1)
		}
		if err := os.WriteFile(name, data, 0o644); err != nil {
			fmt.Fprintf(os.Stderr, "write %s: %v\n", name, err)
			os.Exit(1)
		}
		fmt.Println("generated", name)
	}
}
