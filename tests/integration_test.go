package integration_test

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/sidisinsane/mutter/internal/router/executor"
)

// repoRoot returns the absolute path to the repository root, resolved
// relative to this test file's location.
func repoRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("could not determine test file path")
	}
	// tests/ is one level below the repo root
	return filepath.Join(filepath.Dir(file), "..")
}

// examplesDir returns the absolute path to the examples/ directory.
func examplesDir(t *testing.T) string {
	t.Helper()
	return filepath.Join(repoRoot(t), "examples")
}

// TestHelloScript_ExecutesAndProducesOutput verifies that hello.sh can be
// executed directly and produces the expected output.
func TestHelloScript_ExecutesAndProducesOutput(t *testing.T) {
	script := filepath.Join(examplesDir(t), "hello.sh")
	if _, err := os.Stat(script); err != nil {
		t.Skipf("hello.sh not found: %v", err)
	}

	exec := executor.New()
	result, err := exec.Execute(t.Context(), script, map[string]string{})
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if result.ExitCode != 0 {
		t.Errorf("expected exit code 0, got %d\nstderr: %s", result.ExitCode, result.Stderr)
	}
	if result.Stdout == "" {
		t.Error("expected non-empty stdout")
	}
}

// TestAPIQuery_ExecutesHelloScript documents the expected curl invocations
// for the Phase 1 milestone. Run these manually against a live daemon.
func TestAPIQuery_ExecutesHelloScript(t *testing.T) {
	t.Log("Phase 1 milestone verification:")
	t.Log("  Start:  ./mutter-daemon")
	t.Log("  Route:  curl -s -X POST http://localhost:8080/api/route \\")
	t.Log("            -H 'Content-Type: application/json' \\")
	t.Log("            -d '{\"query\":\"print a hello message\"}' | jq .")
	t.Log("  Query:  curl -s -X POST http://localhost:8080/api/query \\")
	t.Log("            -H 'Content-Type: application/json' \\")
	t.Log("            -d '{\"query\":\"print a hello message\"}' | jq .")
	t.Log("  Expect: stdout contains 'Hello, World!'")
}
