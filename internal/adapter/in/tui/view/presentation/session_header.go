package presentation

import (
	"strings"

	"github.com/charmbracelet/x/ansi"
	tuistyle "github.com/phongsathornpt/protonman/internal/adapter/in/tui/view/style"
)

const (
	fullSessionHeaderWidth  = 40
	sessionHeaderTextColumn = tuistyle.CompactLogoTextColumn
)

type SessionHeaderModel struct {
	Width          int
	Model          string
	LowConcurrency bool
	GoalActive     bool
	Branch         string
	Workspace      string
	Compact        bool
	Minimal        bool
}

func RenderSessionHeader(model SessionHeaderModel) string {
	width := model.Width
	if width <= 0 {
		return ""
	}
	if model.Minimal {
		return tuistyle.CompactBrand(width)
	}
	if model.Compact || width < fullSessionHeaderWidth {
		return renderCompactSessionHeader(model)
	}

	logo := tuistyle.CompactLogoLines()
	if len(logo) != 4 {
		return renderCompactSessionHeader(model)
	}
	context := strings.TrimSpace(model.Branch)
	if context == "" {
		context = strings.TrimSpace(model.Workspace)
	}
	lines := make([]string, len(logo))
	for i, mark := range logo {
		lines[i] = tuistyle.BrandMarkStyle.Render(mark)
		gap := max(0, sessionHeaderTextColumn-ansi.StringWidth(mark))
		available := max(1, width-sessionHeaderTextColumn)

		var text string
		switch i {
		case 0:
			text = tuistyle.BrandStyle.Render(ansi.Truncate(tuistyle.ProductName, available, ""))
		case 1:
			text = renderSessionHeaderMeta(model, available)
		case 3:
			text = tuistyle.MutedStyle.Render(ansi.Truncate(context, available, "…"))
		}
		if text != "" {
			lines[i] += strings.Repeat(" ", gap) + text
		}
	}
	return strings.Join(lines, "\n")
}

func renderCompactSessionHeader(model SessionHeaderModel) string {
	brand := tuistyle.CompactBrand(model.Width)
	if sessionHeaderMeta(model) == "" || model.Width < 12 {
		return brand
	}
	return brand + "\n" + renderSessionHeaderMeta(model, model.Width)
}

// renderSessionHeaderMeta keeps the model visually primary while operational
// flags stay quieter. The words remain meaningful without color, and ANSI-aware
// truncation keeps narrow layouts deterministic.
func renderSessionHeaderMeta(model SessionHeaderModel, width int) string {
	if width <= 0 {
		return ""
	}
	parts := make([]string, 0, 3)
	if name := strings.TrimSpace(model.Model); name != "" {
		parts = append(parts, tuistyle.SystemStyle.Render(name))
	}
	if model.LowConcurrency {
		parts = append(parts, tuistyle.MutedStyle.Render("low"))
	}
	if model.GoalActive {
		parts = append(parts, tuistyle.FocusStyle.Render("goal active"))
	}
	if len(parts) == 0 {
		return ""
	}
	separator := tuistyle.MutedStyle.Render(tuistyle.GlyphSep)
	return ansi.Truncate(strings.Join(parts, separator), width, "…")
}

func sessionHeaderMeta(model SessionHeaderModel) string {
	parts := make([]string, 0, 3)
	if name := strings.TrimSpace(model.Model); name != "" {
		parts = append(parts, name)
	}
	if model.LowConcurrency {
		parts = append(parts, "low")
	}
	if model.GoalActive {
		parts = append(parts, "goal active")
	}
	return strings.Join(parts, " · ")
}
