package presentation

import (
	"strings"

	"github.com/charmbracelet/x/ansi"
	tuistyle "github.com/phongsathornpt/protonman/internal/adapter/in/tui/view/style"
)

const (
	fullSessionHeaderWidth  = 40
	sessionHeaderTextColumn = 11
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
		return tuistyle.BrandStyle.Render(ansi.Truncate(tuistyle.ProductName, width, ""))
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
	right := []string{tuistyle.ProductName, sessionHeaderMeta(model), "", context}
	lines := make([]string, len(logo))
	for i, mark := range logo {
		lines[i] = tuistyle.BrandMarkStyle.Render(mark)
		if right[i] == "" {
			continue
		}
		gap := max(0, sessionHeaderTextColumn-ansi.StringWidth(mark))
		available := max(1, width-sessionHeaderTextColumn)
		text := ansi.Truncate(right[i], available, "")
		if i == 0 {
			lines[i] += strings.Repeat(" ", gap) + tuistyle.BrandStyle.Render(text)
			continue
		}
		lines[i] += strings.Repeat(" ", gap) + tuistyle.MutedStyle.Render(text)
	}
	return strings.Join(lines, "\n")
}

func renderCompactSessionHeader(model SessionHeaderModel) string {
	brand := tuistyle.BrandStyle.Render(ansi.Truncate(tuistyle.ProductName, model.Width, ""))
	meta := sessionHeaderMeta(model)
	if meta == "" || model.Width < 12 {
		return brand
	}
	return brand + "\n" + tuistyle.MutedStyle.Render(ansi.Truncate(meta, model.Width, ""))
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
