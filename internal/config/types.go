// Package config reads, validates, and merges workspace configuration for mutter.
//
// Configuration is resolved by merging three layers in order, with later layers
// taking precedence over earlier ones:
//
//  1. Application defaults (embedded mutter.defaults.json)
//  2. User-level config (~/.config/mutter/mutter.yml → .yaml → .json)
//  3. Project-level config (./mutter.yml → .yaml → .json)
//
// The merged result is validated against the mutter-config JSON schema before
// being returned as a [Config].
package config

// Config holds the fully merged and validated mutter workspace configuration.
type Config struct {
	// SchemaVersion is the semver string declared in the config file, e.g. "0.1.0".
	SchemaVersion string

	// Confirmation controls whether mutter presents the planned execution and
	// waits for explicit user confirmation before running anything. When false,
	// commands that meet the confidence threshold execute immediately. Commands
	// below the threshold always require confirmation regardless of this setting.
	Confirmation bool

	// ConfidenceThreshold is the minimum cosine similarity score (0.0–1.0)
	// required for a routing match to execute without confirmation.
	ConfidenceThreshold float64

	// Session holds configuration for the pending execution buffer.
	Session SessionConfig

	// Model holds configuration for the ONNX sentence embedding model.
	Model ModelConfig

	// Discovery holds configuration for script discovery.
	Discovery DiscoveryConfig

	// Extensions maps comment delimiters to the file extensions that use them.
	// Valid keys are "#" and "//". Values are slices of extensions without a
	// leading dot, e.g. ["sh", "rb", "py"].
	Extensions map[string][]string

	// WorkspaceRoot is the absolute path of the directory from which mutter
	// was invoked. It is derived at runtime and never read from the config file.
	WorkspaceRoot string

	// mutterDir is the absolute path of the .mutter directory within the
	// workspace. It is derived from WorkspaceRoot at load time.
	mutterDir string
}

// SessionConfig holds configuration for the pending execution buffer.
//
// The buffer preserves unexecuted execution IDs for recovery purposes.
// Executed commands are never stored. When the buffer is full, the oldest
// entry is dropped (FIFO).
type SessionConfig struct {
	// BufferSize is the maximum number of unexecuted execution IDs to preserve.
	BufferSize int
}

// ModelConfig holds configuration for the ONNX sentence embedding model used
// for semantic routing.
//
// The model and the compiled index are coupled — swapping models requires a
// full reindex.
type ModelConfig struct {
	// Path is the absolute path to the ONNX model file. Can be overridden at
	// runtime via the --model-path flag.
	Path string

	// Dimensions is the expected output dimensionality of the embedding model.
	// Validated at startup; a mismatch is a hard error.
	Dimensions int
}

// DiscoveryConfig holds configuration for script discovery.
type DiscoveryConfig struct {
	// Paths is the list of directories to scan for scripts. Paths are resolved
	// relative to the workspace root.
	Paths []string

	// Recursive controls whether each path is scanned recursively.
	Recursive bool
}

// MutterDir returns the absolute path of the .mutter directory within the
// workspace. This directory is used for derived artefacts and is never
// committed to version control.
func (c *Config) MutterDir() string {
	return c.mutterDir
}
