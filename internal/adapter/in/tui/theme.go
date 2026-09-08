package tui

import (
	tuistyle "github.com/phongsathornpt/protonman/internal/adapter/in/tui/style"
	"github.com/phongsathornpt/protonman/internal/base/buildinfo"
)

const (
	glyphPrompt      = tuistyle.GlyphPrompt
	glyphMark        = tuistyle.GlyphMark
	glyphTool        = tuistyle.GlyphTool
	glyphSep         = tuistyle.GlyphSep
	glyphToolSuccess = tuistyle.GlyphToolSuccess
	glyphToolError   = tuistyle.GlyphToolError
	glyphToolDenied  = tuistyle.GlyphToolDenied
	glyphWeb         = tuistyle.GlyphWeb
	glyphRead        = tuistyle.GlyphRead
	glyphDir         = tuistyle.GlyphDir
	glyphSearch      = tuistyle.GlyphSearch
	glyphExec        = tuistyle.GlyphExec
	glyphEdit        = tuistyle.GlyphEdit
	glyphSkill       = tuistyle.GlyphSkill
	glyphAgent       = tuistyle.GlyphAgent
	glyphGeneric     = tuistyle.GlyphGeneric
	glyphTodoPending = tuistyle.GlyphTodoPending
	glyphTodoActive  = tuistyle.GlyphTodoActive
	glyphBrand       = tuistyle.GlyphBrand
)

var (
	appVersion      = buildinfo.Version()
	accentAssistant = tuistyle.AccentAssistant
	accentUser      = tuistyle.AccentUser
	accentTool      = tuistyle.AccentTool
	accentSystem    = tuistyle.AccentSystem
	accentPlan      = tuistyle.AccentPlan
	accentError     = tuistyle.AccentError
	accentSuccess   = tuistyle.AccentSuccess
	commandColor    = tuistyle.CommandColor
	warningColor    = tuistyle.WarningColor
	promptBorder    = tuistyle.PromptBorder

	brandStyle       = tuistyle.BrandStyle
	brandMarkStyle   = tuistyle.BrandMarkStyle
	userStyle        = tuistyle.UserStyle
	assistantStyle   = tuistyle.AssistantStyle
	toolStyle        = tuistyle.ToolStyle
	systemStyle      = tuistyle.SystemStyle
	mutedStyle       = tuistyle.MutedStyle
	statusStyle      = tuistyle.StatusStyle
	warningStyle     = tuistyle.WarningStyle
	successStyle     = tuistyle.SuccessStyle
	errorStyle       = tuistyle.ErrorStyle
	planStyle        = tuistyle.PlanStyle
	commandStyle     = tuistyle.CommandStyle
	toolTargetStyle  = tuistyle.ToolTargetStyle
	toolDirStyle     = tuistyle.ToolDirStyle
	toolSummaryStyle = tuistyle.ToolSummaryStyle
	fileBadgeStyle   = tuistyle.FileBadgeStyle
	toolExcerptStyle = tuistyle.ToolExcerptStyle
	toolFoldStyle    = tuistyle.ToolFoldStyle
	heroLabelStyle   = tuistyle.HeroLabelStyle
	heroKeyStyle     = tuistyle.HeroKeyStyle
	diffAddStyle     = tuistyle.DiffAddStyle
	diffDeleteStyle  = tuistyle.DiffDeleteStyle
	diffHunkStyle    = tuistyle.DiffHunkStyle
	bodyStyle        = tuistyle.BodyStyle
	modalStyle       = tuistyle.ModalStyle
)
