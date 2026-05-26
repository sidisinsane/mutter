// Package executor handles command execution with template rendering and
// stdout/stdin piping for chained commands.
package executor

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
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
	// Error is any execution error (not the same as a non-zero exit code).
	Error error
}

// Executor runs commands with template rendering and piping support.
type Executor struct {
	// shell is the shell used to execute scripts.
	shell string
}

// New creates a new Executor using the shell resolved by detectShell.
func New() *Executor {
	return &Executor{
		shell: detectShell(),
	}
}

// NewWithShell creates a new Executor using the given shell path.
func NewWithShell(shell string) *Executor {
	return &Executor{shell: shell}
}

// Execute runs a single script with the given arguments. Arguments are
// rendered into the script body via Go text/template before execution.
func (e *Executor) Execute(ctx context.Context, scriptPath string, args map[string]string) (*ExecutionResult, error) {
	script, err := os.ReadFile(scriptPath)
	if err != nil {
		return nil, fmt.Errorf("read script: %w", err)
	}

	rendered, err := renderTemplate(string(script), args)
	if err != nil {
		return nil, fmt.Errorf("render template: %w", err)
	}

	cmd := exec.CommandContext(ctx, e.shell, "-c", rendered) //nolint:gosec

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	runErr := cmd.Run()

	result := &ExecutionResult{
		Stdout: stdout.String(),
		Stderr: stderr.String(),
	}

	if runErr != nil {
		if exitErr, ok := runErr.(*exec.ExitError); ok {
			result.ExitCode = exitErr.ExitCode()
		} else {
			result.Error = runErr
		}
	}

	return result, nil
}

// ExecuteChain runs a sequence of scripts with stdout piped between them.
// Each script receives the previous script's stdout via the "stdin" template
// argument. The chain stops on the first non-zero exit code.
func (e *Executor) ExecuteChain(ctx context.Context, chain []string, argsList []map[string]string) ([]*ExecutionResult, error) {
	if len(chain) == 0 {
		return nil, fmt.Errorf("empty chain")
	}

	results := make([]*ExecutionResult, len(chain))
	var prevStdout string

	for i, scriptPath := range chain {
		args := argsList[i]
		if args == nil {
			args = make(map[string]string)
		}
		if i > 0 {
			args["stdin"] = prevStdout
		}

		result, err := e.Execute(ctx, scriptPath, args)
		if err != nil {
			return nil, fmt.Errorf("execute chain[%d]: %w", i, err)
		}
		results[i] = result

		if result.ExitCode != 0 {
			break
		}
		prevStdout = result.Stdout
	}

	return results, nil
}

// renderTemplate renders a script body using Go text/template with the given
// argument map. Returns the rendered string or an error.
func renderTemplate(script string, args map[string]string) (string, error) {
	if len(args) == 0 {
		return script, nil
	}

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

// detectShell returns the shell to use for execution. Prefers the SHELL
// environment variable, falling back to /bin/sh.
func detectShell() string {
	if shell := os.Getenv("SHELL"); shell != "" {
		return shell
	}
	return "/bin/sh"
}
