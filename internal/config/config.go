package config

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/sidisinsane/mutter/internal/schema"
)

//go:embed mutter.defaults.json
var defaultsJSON []byte

// configFileCandidates lists config file names in discovery-preference order.
// The first match found wins.
var configFileCandidates = []string{
	"mutter.yml",
	"mutter.yaml",
	"mutter.json",
}

// userConfigDirs returns the ordered list of directories to search for a
// user-level config file. The first directory that contains a matching file
// is used.
func userConfigDirs() []string {
	dirs := []string{}

	if xdg := os.Getenv("XDG_CONFIG_HOME"); xdg != "" {
		dirs = append(dirs, filepath.Join(xdg, "mutter"))
	}

	if home, err := os.UserHomeDir(); err == nil {
		dirs = append(dirs,
			filepath.Join(home, ".config", "mutter"),
			filepath.Join(home, ".mutter"),
		)
	}

	return dirs
}

// Load reads, validates, and merges workspace configuration for the given
// workspace root directory.
//
// Configuration is resolved in three layers (last wins):
//
//  1. Embedded application defaults (mutter.defaults.json)
//  2. User-level config (~/.config/mutter/mutter.yml → .yaml → .json)
//  3. Project-level config (workspaceRoot/mutter.yml → .yaml → .json)
//
// A missing project-level config file is a hard error — mutter requires a
// config file at the workspace root to identify the workspace.
//
// A missing user-level config file is silently ignored.
func Load(workspaceRoot string) (*Config, error) {
	// 1. Read application defaults.
	var defaults map[string]any
	if err := json.Unmarshal(defaultsJSON, &defaults); err != nil {
		return nil, fmt.Errorf("unmarshal defaults: %w", err)
	}

	// 2. Read user-level config (optional).
	userRaw, err := readFirstConfig(userConfigDirs())
	if err != nil {
		return nil, fmt.Errorf("read user config: %w", err)
	}

	// 3. Read project-level config (required).
	projectRaw, err := readFirstConfig([]string{workspaceRoot})
	if err != nil {
		return nil, fmt.Errorf("read project config: %w", err)
	}
	if projectRaw == nil {
		return nil, fmt.Errorf(
			"no mutter config file found in %s (mutter.yml, mutter.yaml, or mutter.json)\n"+
				"Make sure you are running 'mutter' from your workspace root",
			workspaceRoot,
		)
	}

	// 4. Validate each user-supplied layer against the schema before merging.
	if userRaw != nil {
		if violations := schema.ValidateConfig(userRaw); len(violations) > 0 {
			return nil, fmt.Errorf("user config invalid:\n%s", joinViolations(violations))
		}
	}
	if violations := schema.ValidateConfig(projectRaw); len(violations) > 0 {
		return nil, fmt.Errorf("project config invalid:\n%s", joinViolations(violations))
	}

	// 5. Deep-merge: defaults → user → project.
	merged := defaults
	if userRaw != nil {
		merged = deepMerge(merged, userRaw)
	}
	merged = deepMerge(merged, projectRaw)

	// 6. Populate Config from merged map.
	cfg, err := buildConfig(merged)
	if err != nil {
		return nil, fmt.Errorf("build config: %w", err)
	}

	cfg.WorkspaceRoot = workspaceRoot
	cfg.mutterDir = filepath.Join(workspaceRoot, ".mutter")

	return cfg, nil
}

// readFirstConfig searches each directory in dirs for a config file, returning
// the parsed contents of the first match. Returns nil without error when no
// file is found in any of the directories.
func readFirstConfig(dirs []string) (map[string]any, error) {
	for _, dir := range dirs {
		for _, name := range configFileCandidates {
			p := filepath.Join(dir, name)
			data, err := os.ReadFile(p)
			if os.IsNotExist(err) {
				continue
			}
			if err != nil {
				return nil, fmt.Errorf("read %s: %w", p, err)
			}

			raw, err := parseConfig(data, filepath.Ext(name))
			if err != nil {
				return nil, fmt.Errorf("parse %s: %w", p, err)
			}

			return raw, nil
		}
	}
	return nil, nil
}

// parseConfig deserialises raw config bytes based on the file extension.
// ext must be ".json", ".yml", or ".yaml".
func parseConfig(data []byte, ext string) (map[string]any, error) {
	var raw map[string]any
	var err error

	if ext == ".json" {
		err = json.Unmarshal(data, &raw)
	} else {
		err = yaml.Unmarshal(data, &raw)
	}
	if err != nil {
		return nil, err
	}

	normaliseYAML(raw)
	return raw, nil
}

// buildConfig populates a [Config] from the fully merged raw map.
func buildConfig(raw map[string]any) (*Config, error) {
	cfg := &Config{
		SchemaVersion:       stringValue(raw, "schema_version"),
		Confirmation:        boolValue(raw, "confirmation"),
		ConfidenceThreshold: floatValue(raw, "confidence_threshold"),
	}

	if s, ok := raw["session"].(map[string]any); ok {
		cfg.Session = SessionConfig{
			BufferSize: intValue(s, "buffer_size"),
		}
	}

	if m, ok := raw["model"].(map[string]any); ok {
		cfg.Model = ModelConfig{
			Path:        expandHome(stringValue(m, "path")),
			Dimensions:  intValue(m, "dimensions"),
			LibraryPath: expandHome(stringValue(m, "library_path")),
		}
	}

	if d, ok := raw["discovery"].(map[string]any); ok {
		cfg.Discovery = DiscoveryConfig{
			Paths:     stringSliceValue(d, "paths"),
			Recursive: boolValue(d, "recursive"),
		}
	}

	if e, ok := raw["extensions"].(map[string]any); ok {
		cfg.Extensions = make(map[string][]string, len(e))
		for delimiter, v := range e {
			exts, ok := v.([]any)
			if !ok {
				continue
			}
			ss := make([]string, 0, len(exts))
			for _, item := range exts {
				if s, ok := item.(string); ok {
					ss = append(ss, s)
				}
			}
			cfg.Extensions[delimiter] = ss
		}
	}

	return cfg, nil
}

// normaliseYAML recursively converts time.Time values (produced by the YAML
// parser for date literals) to their "2006-01-02" string representation, and
// converts map[any]any to map[string]any for downstream JSON-schema validation.
func normaliseYAML(m map[string]any) {
	for k, v := range m {
		switch x := v.(type) {
		case time.Time:
			m[k] = x.Format("2006-01-02")
		case map[string]any:
			normaliseYAML(x)
		case map[any]any:
			// gopkg.in/yaml.v3 occasionally produces map[any]any for nested maps.
			converted := make(map[string]any, len(x))
			for mk, mv := range x {
				converted[fmt.Sprintf("%v", mk)] = mv
			}
			normaliseYAML(converted)
			m[k] = converted
		}
	}
}

// deepMerge returns a new map containing all keys from base, with values from
// override taking precedence. Nested maps are merged recursively. Slice values
// from override replace those in base entirely.
func deepMerge(base, override map[string]any) map[string]any {
	result := make(map[string]any, len(base))
	for k, v := range base {
		result[k] = deepCopy(v)
	}
	for k, v := range override {
		if bm, ok := result[k].(map[string]any); ok {
			if vm, ok := v.(map[string]any); ok {
				result[k] = deepMerge(bm, vm)
				continue
			}
		}
		result[k] = deepCopy(v)
	}
	return result
}

// deepCopy returns a deep copy of v. Maps and slices are copied recursively;
// all other values are returned as-is (they are immutable scalar types).
func deepCopy(v any) any {
	switch x := v.(type) {
	case map[string]any:
		m := make(map[string]any, len(x))
		for k, v2 := range x {
			m[k] = deepCopy(v2)
		}
		return m
	case []any:
		s := make([]any, len(x))
		for i, v2 := range x {
			s[i] = deepCopy(v2)
		}
		return s
	default:
		return v
	}
}

// joinViolations formats a slice of schema violation strings as a single
// indented block suitable for inclusion in an error message.
func joinViolations(violations []string) string {
	out := ""
	for _, v := range violations {
		out += "  - " + v + "\n"
	}
	return out
}

// intValue extracts an integer from a raw map by key, handling the float64
// representation used by both the JSON and YAML decoders.
func intValue(m map[string]any, key string) int {
	v, ok := m[key]
	if !ok {
		return 0
	}
	switch x := v.(type) {
	case int:
		return x
	case int64:
		return int(x)
	case float64:
		return int(x)
	default:
		return 0
	}
}

// floatValue extracts a float64 from a raw map by key.
func floatValue(m map[string]any, key string) float64 {
	v, ok := m[key]
	if !ok {
		return 0
	}
	switch x := v.(type) {
	case float64:
		return x
	case float32:
		return float64(x)
	case int:
		return float64(x)
	case int64:
		return float64(x)
	default:
		return 0
	}
}

// stringValue extracts a string from a raw map by key.
func stringValue(m map[string]any, key string) string {
	v, ok := m[key]
	if !ok {
		return ""
	}
	s, _ := v.(string)
	return s
}

// boolValue extracts a bool from a raw map by key.
func boolValue(m map[string]any, key string) bool {
	v, ok := m[key]
	if !ok {
		return false
	}
	b, _ := v.(bool)
	return b
}

// stringSliceValue extracts a []string from a raw map by key, tolerating the
// []any representation produced by JSON and YAML decoders.
func stringSliceValue(m map[string]any, key string) []string {
	v, ok := m[key]
	if !ok {
		return nil
	}
	s, ok := v.([]any)
	if !ok {
		return nil
	}
	out := make([]string, 0, len(s))
	for _, item := range s {
		if str, ok := item.(string); ok {
			out = append(out, str)
		}
	}
	return out
}

// expandHome expands ~ to the user's home directory in a path string.
func expandHome(path string) string {
	if strings.HasPrefix(path, "~/") || path == "~" {
		home, err := os.UserHomeDir()
		if err != nil {
			return path
		}
		return filepath.Join(home, path[2:])
	}
	return path
}