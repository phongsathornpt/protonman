//go:build desktop || desktop_gio

package shell

import (
	"gioui.org/font"
	"gioui.org/x/richtext"

	conversationcomponent "github.com/phongsathornpt/protonman/internal/adapter/in/desktop/gioui/component/conversation"
)

const (
	markdownBlockText = conversationcomponent.MarkdownBlockText
	markdownBlockCode = conversationcomponent.MarkdownBlockCode
)

type markdownBlock = conversationcomponent.MarkdownBlock

// splitMarkdownCodeBlocks re-exported for the markdown layout call sites.
var splitMarkdownCodeBlocks = conversationcomponent.SplitMarkdownCodeBlocks

func highlightCodeSpans(t *theme, lang, code string) []richtext.SpanStyle {
	return conversationcomponent.HighlightCodeSpans(conversationcomponent.SyntaxTheme{
		Font:      t.MonoFont(font.Normal),
		FontSize:  textBodySmall,
		Plain:     t.Colors.OnSurface,
		Keyword:   t.Colors.Primary,
		TypeIdent: t.Colors.Tertiary,
		String:    t.Colors.Strength,
		Comment:   t.Colors.Secondary,
		Number:    t.Colors.Agility,
		Operator:  t.Colors.OnSurfaceVariant,
	}, lang, code)
}
