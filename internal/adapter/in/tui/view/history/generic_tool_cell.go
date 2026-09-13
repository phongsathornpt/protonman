package history

import (
	"fmt"
	"strings"

	tuistyle "github.com/phongsathornpt/protonman/internal/adapter/in/tui/view/style"
	"github.com/phongsathornpt/protonman/internal/adapter/in/tui/view/toolview"
	"github.com/phongsathornpt/protonman/internal/core/tool"
)

// ToolCell is the generic representation for a tool that has no specialized
// presentation model.
type ToolCell struct {
	CallID          string
	Name            string
	Body            string
	Stdout          string
	Stderr          string
	Running         bool
	ExitCode        *int
	Truncated       bool
	StdoutTruncated bool
	StderrTruncated bool
	Denied          bool
	FailureCode     tool.ErrorCode
	Spinner         string
	Target          string
	ToolKind        tool.Kind
	Summary         string
	ShowDetail      bool
}

func (ToolCell) Kind() HistoryCellKind { return HistoryCellTool }
func (c ToolCell) RenderWidth(width int) []string {
	summary := c.Summary
	if summary == "" && c.Body != "" && !c.Running && !c.Denied && c.FailureCode == "" {
		summary = toolview.SummarizeOutput(c.Name, c.ToolKind, c.Target, c.Body, c.ExitCode, c.Truncated)
	}

	label := ""
	target := c.Target
	if isSkillTool(c.Name) && !c.Running && !c.Denied && c.FailureCode == "" {
		label = "Activated skill"
		if target == "" {
			if skillName := toolview.ExtractSkillContentName(c.Body); skillName != "" {
				target = fmt.Sprintf("%q", skillName)
			}
		}
		summary = ""
	}

	header := toolview.ProjectHeader(toolview.HeaderInput{
		Name:        c.Name,
		Kind:        c.ToolKind,
		Label:       label,
		Target:      target,
		Summary:     summary,
		Running:     c.Running,
		Denied:      c.Denied,
		FailureCode: c.FailureCode,
		Spinner:     c.Spinner,
	})
	headerLine := toolview.RenderHeader(header, width)

	out := make([]string, 0, 1)
	for _, line := range wrapStyledLines(headerLine, max(1, width)) {
		out = append(out, line)
	}

	showDetail := !c.Running && (c.ShowDetail || c.Denied || c.FailureCode != "")
	if showDetail {
		if c.ShowDetail && strings.TrimSpace(c.Name) == tool.NameRead && !c.Denied && c.FailureCode == "" && c.Body != "" {
			if excerpt := toolview.ExtractReadFileExcerpt(c.Body); excerpt != "" {
				out = append(out, tuistyle.ToolExcerptStyle.Render("  ↳ "+excerpt))
			}
		}
		if c.ToolKind == tool.KindGrep || c.Name == "grep" {
			for _, line := range toolview.FormatGrepView(c.bodyLines(), c.Target, width) {
				out = append(out, "  "+line)
			}
		} else {
			bodyLines := c.bodyLines()
			if len(bodyLines) > 0 {
				folded := toolview.FormatOutputFold(bodyLines, 3)
				for _, line := range folded {
					for _, wrapped := range wrapStyledLines(line, max(1, width-2)) {
						out = append(out, tuistyle.BodyStyle.Render("  "+wrapped))
					}
				}
			}
		}
	}

	return out
}

func routineToolAggregationKey(cell HistoryCell) string {
	c, ok := cell.(*ToolCell)
	if !ok || c.Running || c.Denied || c.FailureCode != "" || c.ShowDetail {
		return ""
	}
	switch strings.ToLower(strings.TrimSpace(c.Name)) {
	case "read":
		return "read"
	case "ls", "list":
		return "list"
	case "find", "grep", "search":
		return "search"
	default:
		return ""
	}
}

func renderRoutineToolAggregate(key string, count, width int) []string {
	if count < 2 {
		return nil
	}
	label := "Operations"
	switch key {
	case "read":
		label = fmt.Sprintf("Read %d files", count)
	case "list":
		label = fmt.Sprintf("Listed %d locations", count)
	case "search":
		label = fmt.Sprintf("Search %d queries", count)
	}
	line := tuistyle.SuccessStyle.Render(tuistyle.GlyphToolSuccess) + tuistyle.MutedStyle.Render(label)
	return wrapStyledLines(line, max(1, width))
}

func (c ToolCell) RawLines() []string {
	header := sanitizeBubbleText(tool.DisplayName(c.Name))
	if c.Target != "" {
		header += " " + sanitizeBubbleText(c.Target)
	}
	out := []string{header}
	out = append(out, c.bodyLines()...)
	return out
}
func (c ToolCell) LineCount() int           { return len(c.RawLines()) }
func (c ToolCell) historyToolID() string    { return c.CallID }
func (c ToolCell) historyToolName() string  { return c.Name }
func (c ToolCell) historyToolRunning() bool { return c.Running }
func (c ToolCell) bodyLines() []string {
	if isSkillTool(c.Name) {
		return formatSkillToolBody(c.Body, c.ExitCode, c.Truncated, c.Denied, c.FailureCode)
	}
	return resultBodyLines(c.Body, c.ExitCode, c.Truncated, c.Denied, c.FailureCode)
}

func isSkillTool(name string) bool {
	return strings.TrimSpace(name) == tool.NameSkill
}

func formatSkillToolBody(body string, exitCode *int, truncated bool, denied bool, failureCode tool.ErrorCode) []string {
	if denied {
		return []string{"denied"}
	}
	if failureCode != "" {
		return []string{"failure: " + string(failureCode)}
	}
	var lines []string
	if dir := extractSkillDirectory(body); dir != "" {
		lines = append(lines, "directory: "+dir)
	}
	if res := extractSkillResources(body); len(res) > 0 {
		if len(res) <= 3 {
			lines = append(lines, fmt.Sprintf("resources: %s", strings.Join(res, ", ")))
		} else {
			lines = append(lines, fmt.Sprintf("resources: %d files (%s, …)", len(res), strings.Join(res[:2], ", ")))
		}
	}
	if len(lines) > 0 {
		return lines
	}
	if skillName := toolview.ExtractSkillContentName(body); skillName != "" {
		return []string{fmt.Sprintf("instructions loaded for %q", skillName)}
	}
	return resultBodyLines(body, exitCode, truncated, denied, failureCode)
}

func extractSkillDirectory(body string) string {
	const marker = "Skill directory: "
	if idx := strings.Index(body, marker); idx != -1 {
		rest := body[idx+len(marker):]
		if end := strings.IndexByte(rest, '\n'); end != -1 {
			return strings.TrimSpace(rest[:end])
		}
		return strings.TrimSpace(rest)
	}
	return ""
}

func extractSkillResources(body string) []string {
	var res []string
	startTag := "<file>"
	endTag := "</file>"
	cur := body
	for {
		s := strings.Index(cur, startTag)
		if s == -1 {
			break
		}
		e := strings.Index(cur[s:], endTag)
		if e == -1 {
			break
		}
		item := strings.TrimSpace(cur[s+len(startTag) : s+e])
		if item != "" {
			res = append(res, item)
		}
		cur = cur[s+e+len(endTag):]
	}
	if len(res) > 0 {
		return res
	}
	if idx := strings.Index(body, "Resources:\n"); idx != -1 {
		lines := strings.Split(body[idx:], "\n")
		for _, line := range lines[1:] {
			line = strings.TrimSpace(line)
			if strings.HasPrefix(line, "- ") {
				res = append(res, strings.TrimPrefix(line, "- "))
			} else if line != "" && !strings.HasPrefix(line, "-") {
				break
			}
		}
	}
	return res
}
