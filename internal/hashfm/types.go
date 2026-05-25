// Package hashfm implements mutter's extended hashfm specification for
// script metadata blocks, including parsing, indexing, and validation.
package hashfm

// TypeCategory represents a broad data category for chain compatibility validation.
// Valid values are defined in the hashfm schema: video, audio, image, text, binary.
type TypeCategory string

const (
	TypeCategoryVideo  TypeCategory = "video"
	TypeCategoryAudio  TypeCategory = "audio"
	TypeCategoryImage  TypeCategory = "image"
	TypeCategoryText   TypeCategory = "text"
	TypeCategoryBinary TypeCategory = "binary"
)

// Type declares the data categories a command consumes and produces for chain validation.
type Type struct {
	// Input is the list of data categories this command accepts as input.
	// A chain is valid when this intersects with the upstream command's output.
	Input []TypeCategory `yaml:"input"`
	// Output is the list of data categories this command produces as output.
	// A chain is valid when this intersects with the downstream command's input.
	Output []TypeCategory `yaml:"output"`
}

// Argument describes a single named argument extracted from user input.
type Argument struct {
	// Pattern is a regular expression to extract this argument's value from a natural language prompt.
	Pattern string `yaml:"pattern"`
	// Description is a short plain-language description of the argument.
	Description string `yaml:"description"`
}

// Command represents a single command entry in a hashfm block (either a single-command
// script or one subcommand in a multi-command script).
type Command struct {
	// Description is a single-line imperative description of the command.
	Description string `yaml:"description"`
	// Usage shows the invocation syntax with {{argument}} placeholders.
	Usage string `yaml:"usage"`
	// Type declares input/output data categories for chain validation.
	Type *Type `yaml:"type"`
	// Arguments maps argument names to their descriptors. Keys must match {{placeholders}} in Usage.
	Arguments map[string]Argument `yaml:"arguments"`
	// Exits maps exit codes to plain-language descriptions.
	Exits map[int]string `yaml:"exits"`
}

// Block represents a parsed hashfm metadata block from a script.
// Name and Path are derived from the filesystem, not the block itself.
type Block struct {
	// Name is the base name of the script file (e.g., "convert-video.sh").
	Name string
	// Path is the absolute path to the script file.
	Path string
	// Commands contains either a single command (single-command form) or multiple commands (multi-command form).
	Commands []Command
	// IsMultiCommand indicates whether the block uses the multi-command form.
	IsMultiCommand bool
}
