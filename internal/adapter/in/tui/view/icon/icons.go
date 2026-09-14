// Package icon defines terminal presentation glyph sets independently from
// styling so renderers can switch capabilities without scattering raw glyphs.
package icon

// Set is the semantic icon catalog used by the TUI. Glyphs that prefix text
// include their trailing space so all profiles preserve the same layout
// contract. Brand is the only bare mark because callers compose its spacing.
type Set struct {
	Prompt      string
	Mark        string
	Tool        string
	ToolSuccess string
	ToolError   string
	ToolDenied  string
	Web         string
	Read        string
	Dir         string
	Search      string
	Exec        string
	Edit        string
	Skill       string
	Agent       string
	Git         string
	Generic     string
	TodoPending string
	TodoActive  string
	Brand       string
}
