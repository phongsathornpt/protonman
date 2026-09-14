package style

import (
	"fmt"
	"strings"
)

// IconSet is the semantic icon catalog used by the TUI. Prefix glyphs include
// their trailing space so every profile preserves the same layout contract.
// Brand is the only bare mark because callers compose its spacing.
type IconSet struct {
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

// IconMode selects the terminal glyph capability profile.
type IconMode string

const (
	IconModeAuto    IconMode = "auto"
	IconModeNerd    IconMode = "nerd"
	IconModeUnicode IconMode = "unicode"
	IconModeASCII   IconMode = "ascii"
)

// Unicode profile preserves Protonman's terminal-safe glyphs.
const (
	UnicodePrompt      = "› "
	UnicodeMark        = "› "
	UnicodeTool        = "$ "
	UnicodeToolSuccess = "✓ "
	UnicodeToolError   = "× "
	UnicodeToolDenied  = "! "
	UnicodeWeb         = "↗ "
	UnicodeRead        = "≡ "
	UnicodeDir         = "▸ "
	UnicodeSearch      = "? "
	UnicodeExec        = "$ "
	UnicodeEdit        = "+ "
	UnicodeSkill       = "* "
	UnicodeAgent       = "→ "
	UnicodeGit         = "⌥ "
	UnicodeGeneric     = "· "
	UnicodeTodoPending = "○ "
	UnicodeTodoActive  = "● "
	UnicodeBrand       = "◆"
)

// ASCII profile is safe for dumb terminals, redirected output, and logs.
const (
	ASCIIPrompt      = "> "
	ASCIIMark        = "> "
	ASCIITool        = "$ "
	ASCIIToolSuccess = "+ "
	ASCIIToolError   = "x "
	ASCIIToolDenied  = "! "
	ASCIIWeb         = "> "
	ASCIIRead        = "= "
	ASCIIDir         = "> "
	ASCIISearch      = "? "
	ASCIIExec        = "$ "
	ASCIIEdit        = "+ "
	ASCIISkill       = "* "
	ASCIIAgent       = "> "
	ASCIIGit         = "@ "
	ASCIIGeneric     = ". "
	ASCIITodoPending = "o "
	ASCIITodoActive  = "* "
	ASCIIBrand       = "*"
)

// Nerd Font profile uses Codicons from Nerd Fonts v3. Protonman supports the
// Nerd Font Mono terminal variant so these PUA glyphs retain a one-cell width.
const (
	NerdPrompt      = "\ueab6 " // nf-cod-chevron_right
	NerdMark        = "\ueab6 " // nf-cod-chevron_right
	NerdTool        = "\ueb6d " // nf-cod-tools
	NerdToolSuccess = "\ueab2 " // nf-cod-check
	NerdToolError   = "\uea87 " // nf-cod-error
	NerdToolDenied  = "\uea6c " // nf-cod-warning
	NerdWeb         = "\ueb01 " // nf-cod-globe
	NerdRead        = "\uea7b " // nf-cod-file
	NerdDir         = "\uea83 " // nf-cod-folder
	NerdSearch      = "\uea6d " // nf-cod-search
	NerdExec        = "\uea85 " // nf-cod-terminal
	NerdEdit        = "\uea73 " // nf-cod-edit
	NerdSkill       = "\uec10 " // nf-cod-sparkle
	NerdAgent       = "\uec67 " // nf-cod-agent
	NerdGit         = "\uea68 " // nf-cod-source_control
	NerdGeneric     = "\ueabc " // nf-cod-circle
	NerdTodoPending = "\ueabc " // nf-cod-circle
	NerdTodoActive  = "\uea71 " // nf-cod-circle_filled
	NerdBrand       = "\ueb44"  // nf-cod-rocket
)

var (
	UnicodeIcons = IconSet{
		Prompt: UnicodePrompt, Mark: UnicodeMark, Tool: UnicodeTool,
		ToolSuccess: UnicodeToolSuccess, ToolError: UnicodeToolError, ToolDenied: UnicodeToolDenied,
		Web: UnicodeWeb, Read: UnicodeRead, Dir: UnicodeDir, Search: UnicodeSearch,
		Exec: UnicodeExec, Edit: UnicodeEdit, Skill: UnicodeSkill, Agent: UnicodeAgent, Git: UnicodeGit,
		Generic: UnicodeGeneric, TodoPending: UnicodeTodoPending, TodoActive: UnicodeTodoActive, Brand: UnicodeBrand,
	}
	ASCIIIcons = IconSet{
		Prompt: ASCIIPrompt, Mark: ASCIIMark, Tool: ASCIITool,
		ToolSuccess: ASCIIToolSuccess, ToolError: ASCIIToolError, ToolDenied: ASCIIToolDenied,
		Web: ASCIIWeb, Read: ASCIIRead, Dir: ASCIIDir, Search: ASCIISearch,
		Exec: ASCIIExec, Edit: ASCIIEdit, Skill: ASCIISkill, Agent: ASCIIAgent, Git: ASCIIGit,
		Generic: ASCIIGeneric, TodoPending: ASCIITodoPending, TodoActive: ASCIITodoActive, Brand: ASCIIBrand,
	}
	NerdIcons = IconSet{
		Prompt: NerdPrompt, Mark: NerdMark, Tool: NerdTool,
		ToolSuccess: NerdToolSuccess, ToolError: NerdToolError, ToolDenied: NerdToolDenied,
		Web: NerdWeb, Read: NerdRead, Dir: NerdDir, Search: NerdSearch,
		Exec: NerdExec, Edit: NerdEdit, Skill: NerdSkill, Agent: NerdAgent, Git: NerdGit,
		Generic: NerdGeneric, TodoPending: NerdTodoPending, TodoActive: NerdTodoActive, Brand: NerdBrand,
	}
)

// ParseIconMode validates an icon mode from user-controlled environment state.
func ParseIconMode(value string) (IconMode, error) {
	mode := IconMode(strings.ToLower(strings.TrimSpace(value)))
	if mode == "" {
		return IconModeAuto, nil
	}
	switch mode {
	case IconModeAuto, IconModeNerd, IconModeUnicode, IconModeASCII:
		return mode, nil
	default:
		return "", fmt.Errorf("unsupported icon mode %q", value)
	}
}

// ResolveIcons returns a deterministic icon set. Auto deliberately does not
// guess the terminal font because font selection is not reliably observable,
// especially over SSH, tmux, or remote IDE terminals.
func ResolveIcons(mode IconMode, interactive bool) IconSet {
	switch mode {
	case IconModeNerd:
		return NerdIcons
	case IconModeASCII:
		return ASCIIIcons
	case IconModeUnicode:
		return UnicodeIcons
	case IconModeAuto, "":
		if !interactive {
			return ASCIIIcons
		}
		return UnicodeIcons
	default:
		return UnicodeIcons
	}
}

// OrUnicodeIcons returns the supplied profile, or the compatibility Unicode
// profile when an optional icon field was left zero-valued.
func OrUnicodeIcons(icons IconSet) IconSet {
	if icons == (IconSet{}) {
		return UnicodeIcons
	}
	return icons
}
