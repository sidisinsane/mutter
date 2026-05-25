// Package schema provides JSON Schema validation for mutter workspace
// configuration and hashfm script blocks.
//
// Both schemas are embedded at compile time from the project's schema/
// directory. The go:generate directive keeps the embedded copies in sync
// with the canonical sources.
package schema

import _ "embed"

// ConfigSchema holds the embedded JSON Schema for mutter workspace
// configuration (mutter-config.schema.json).
//
//go:embed mutter-config.schema.json
var ConfigSchema []byte

// HashfmSchema holds the embedded JSON Schema for hashfm script blocks
// (hashfm-mutter.schema.json).
//
//go:embed hashfm-mutter.schema.json
var HashfmSchema []byte

//go:generate go run generate.go
