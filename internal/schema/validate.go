package schema

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sync"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

var (
	compiledSchemas     map[string]*jsonschema.Schema
	compiledSchemasOnce sync.Once
)

// initCompiledSchemas compiles all embedded schemas exactly once. Panics on
// any compilation failure — a broken embedded schema is a programming error,
// not a runtime condition.
func initCompiledSchemas() {
	compiledSchemasOnce.Do(func() {
		sources := map[string][]byte{
			"config": ConfigSchema,
			"hashfm": HashfmSchema,
		}

		compiler := jsonschema.NewCompiler()
		compiledSchemas = make(map[string]*jsonschema.Schema, len(sources))

		for name, data := range sources {
			uri := "resource://" + name + ".json"
			s, err := jsonschema.UnmarshalJSON(bytes.NewReader(data))
			if err != nil {
				panic(fmt.Sprintf("unmarshal %s schema: %v", name, err))
			}
			if err := compiler.AddResource(uri, s); err != nil {
				panic(fmt.Sprintf("add %s schema: %v", name, err))
			}
			compiled, err := compiler.Compile(uri)
			if err != nil {
				panic(fmt.Sprintf("compile %s schema: %v", name, err))
			}
			compiledSchemas[name] = compiled
		}
	})
}

// ValidateConfig validates a raw config map against the mutter workspace
// configuration schema and returns all violation messages. An empty slice
// means the config is valid.
func ValidateConfig(raw map[string]any) []string {
	initCompiledSchemas()
	return validateAgainst(raw, compiledSchemas["config"])
}

// ValidateHashfm validates a raw hashfm block map against the hashfm-mutter
// schema and returns all violation messages. An empty slice means the block
// is valid.
func ValidateHashfm(raw map[string]any) []string {
	initCompiledSchemas()
	return validateAgainst(raw, compiledSchemas["hashfm"])
}

// validateAgainst marshals raw to JSON and validates it against the given
// compiled schema. Returns all leaf violation messages.
func validateAgainst(raw map[string]any, s *jsonschema.Schema) []string {
	if s == nil {
		return nil
	}
	b, err := json.Marshal(raw)
	if err != nil {
		return []string{fmt.Sprintf("marshal: %v", err)}
	}
	inst, err := jsonschema.UnmarshalJSON(bytes.NewReader(b))
	if err != nil {
		return []string{fmt.Sprintf("unmarshal: %v", err)}
	}
	verr, ok := s.Validate(inst).(*jsonschema.ValidationError)
	if !ok || verr == nil {
		return nil
	}
	return collectMessages(verr)
}

// collectMessages recursively extracts leaf error messages from a
// ValidationError tree, skipping intermediate wrapper nodes that carry no
// message of their own.
func collectMessages(err *jsonschema.ValidationError) []string {
	if len(err.Causes) == 0 {
		return []string{err.Error()}
	}
	var msgs []string
	for _, c := range err.Causes {
		msgs = append(msgs, collectMessages(c)...)
	}
	return msgs
}
