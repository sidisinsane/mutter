// Package executor handles command execution with template rendering and
// stdout/stdin piping for chained commands.
package executor

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"text/template"
)

// ExecutionResult holds the result of a command execution.
type ExecutionResult struct {
	// ExitCode is the exit code of the command.
	ExitCode int
	// Stdout is the captured stdout output.
	Stdout string
	// Stderr is the captured stderr output.
	Stderr string
	// Error is any execution error (not the same as non-zero exit code).
	Error error
}

// Executor runs commands with template rendering and piping support.
type Executor struct {
	// shell is the shell to use for script execution.
	shell string
}

// New creates a new Executor with the default shell.
func New() *Executor {
	return &Executor{
		shell: detectShell(),
	}
}

// Execute runs a single command with the given arguments.
// The scriptPath is the path to the hashfm-annotated script.
func (e *Executor) Execute(ctx context.Context, scriptPath string, args map[string]string) (*ExecutionResult, error) {
	// Read the script
	script, err := os.ReadFile(scriptPath)
	if err != nil {
		return nil, fmt.Errorf("read script: %w", err)
	}

	// Render templates in the script with provided arguments
	rendered, err := renderTemplate(string(script), args)
	if err != nil {
		return nil, fmt.Errorf("render template: %w", err)
	}

	// Execute the script
	cmd := exec.CommandContext(ctx, e.shell, "-c", rendered)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err = cmd.Run()

	result := &ExecutionResult{
		Stdout: stdout.String(),
		Stderr: stderr.String(),
	}

	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			result.ExitCode = exitErr.ExitCode()
		} else {
			result.Error = err
		}
	}

	return result, nil
}

// ExecuteChain runs a chain of commands with piping between them.
// The output of each command is piped to the input of the next.
func (e *Executor) ExecuteChain(ctx context.Context, chain []string, argsList []map[string]string) ([]*ExecutionResult, error) {
	if len(chain) == 0 {
		return nil, fmt.Errorf("empty chain")
	}

	results := make([]*ExecutionResult, len(chain))

	// For a single command, just execute it
	if len(chain) == 1 {
		result, err := e.Execute(ctx, chain[0], argsList[0])
		if err != nil {
			return nil, err
		}
		results[0] = result
		return results, nil
	}

	// For chained commands, pipe output between them
	var prevStdout string
	for i, scriptPath := range chain {
		args := argsList[i]
		if i > 0 {
			// Add previous stdout as input to template
			args["stdin"] = prevStdout
		}

		result, err := e.Execute(ctx, scriptPath, args)
		if err != nil {
			return nil, fmt.Errorf("execute chain[%d]: %w", i, err)
		}
		results[i] = result

		if result.ExitCode != 0 {
			// Stop chain on non-zero exit
			break
		}

		prevStdout = result.Stdout
	}

	return results, nil
}

// renderTemplate renders a script template with the provided arguments.
func renderTemplate(script string, args map[string]string) (string, error) {
	tmpl, err := template.New("script").Parse(script)
	if err != nil {
		return "", fmt.Errorf("parse template: %w", err)
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, args); err != nil {
		return "", fmt.Errorf("execute template: %w", err)
	}

	return buf.String(), nil
}

// detectShell returns the path to the default shell.
func detectShell() string {
	// Check for SHELL environment variable
	if shell := os.Getenv("SHELL"); shell != "" {
		return shell
	}

	// Fall back to /bin/sh
	return "/bin/sh"
}
