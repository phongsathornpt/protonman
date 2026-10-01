//go:build desktop || desktop_gio

package conversation

import (
	"fmt"
	"strings"

	"gioui.org/font"
	"gioui.org/layout"
	"gioui.org/unit"
	"gioui.org/widget"

	"github.com/phongsathornpt/protonman/internal/adapter/in/desktop/gioui/component/uikit"
	"github.com/phongsathornpt/protonman/internal/base/diffutil"
)

type ToolDiffCacheEntry struct {
	Source    string
	IsDiff    bool
	Filename  string
	Additions int
	Deletions int
	Preview   []string
	Omitted   int
}

func IsDiffText(text string) bool {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return false
	}
	if strings.HasPrefix(trimmed, "diff --git ") {
		return true
	}
	lines := strings.Split(text, "\n")
	hasHunk := false
	hasAddDel := false
	for _, l := range lines {
		tl := strings.TrimSpace(l)
		if strings.HasPrefix(tl, "diff --git ") {
			return true
		}
		if strings.HasPrefix(tl, "@@") && strings.Contains(tl[2:], "@@") {
			hasHunk = true
		}
		if strings.HasPrefix(l, "+") && !strings.HasPrefix(l, "+++") {
			hasAddDel = true
		}
		if strings.HasPrefix(l, "-") && !strings.HasPrefix(l, "---") {
			hasAddDel = true
		}
	}
	return hasHunk && hasAddDel
}

func ExtractDiffFilename(text string) string {
	for _, line := range strings.Split(text, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "diff --git ") {
			parts := strings.Fields(trimmed)
			if len(parts) >= 4 {
				return strings.TrimPrefix(parts[3], "b/")
			}
		}
		if strings.HasPrefix(trimmed, "+++ b/") {
			return strings.TrimPrefix(trimmed, "+++ b/")
		}
	}
	return ""
}

func ToolDiffInfo(cache *uikit.BoundedCache[string, ToolDiffCacheEntry], toolKey, text string) ToolDiffCacheEntry {
	if cache != nil {
		if cached, ok := cache.Get(toolKey); ok && cached.Source == text {
			return cached
		}
	}
	entry := ToolDiffCacheEntry{Source: text}
	if IsDiffText(text) {
		entry.IsDiff = true
		entry.Filename = ExtractDiffFilename(text)
		entry.Additions, entry.Deletions = diffutil.DiffStats(text)
		entry.Preview, entry.Omitted = diffutil.ExtractPreview(text, 32)
	}
	if cache != nil {
		cache.Put(toolKey, entry)
	}
	return entry
}

func ToolCategoryIcon(title string) string {
	lower := strings.ToLower(title)
	switch {
	case strings.Contains(lower, "bash") || strings.Contains(lower, "exec") || strings.Contains(lower, "command") || strings.Contains(lower, "terminal"):
		return "terminal"
	case strings.Contains(lower, "read") || strings.Contains(lower, "file") || strings.Contains(lower, "dir") || strings.Contains(lower, "ls"):
		return "folder"
	case strings.Contains(lower, "grep") || strings.Contains(lower, "find") || strings.Contains(lower, "search"):
		return "search"
	case strings.Contains(lower, "edit") || strings.Contains(lower, "write") || strings.Contains(lower, "patch") || strings.Contains(lower, "replace"):
		return "compose"
	default:
		return "settings"
	}
}

type DiffToolItemInput struct {
	Button      *widget.Clickable
	Expanded    bool
	Title       string
	Diff        ToolDiffCacheEntry
	Description string
	Chrome      Chrome
	OnToggle    func()
}

func LayoutDiffToolItem(gtx layout.Context, input DiffToolItemInput) layout.Dimensions {
	chrome := input.Chrome
	adds, dels := input.Diff.Additions, input.Diff.Deletions
	displayTitle := input.Title
	if input.Diff.Filename != "" {
		displayTitle = input.Diff.Filename
	}

	btn := input.Button
	if btn == nil {
		btn = new(widget.Clickable)
	}
	if btn.Clicked(gtx) && input.OnToggle != nil {
		input.OnToggle()
	}

	chevron := uikit.IconChevronRight
	if input.Expanded {
		chevron = uikit.IconChevronDown
	}

	headerBg := chrome.Colors.SurfaceContainerHigh
	headerBorder := chrome.Colors.OutlineVariant
	if btn.Hovered() {
		headerBg = chrome.Colors.SurfaceContainerHighest
		headerBorder = chrome.Colors.Primary
	}

	previewLines, omitted := input.Diff.Preview, input.Diff.Omitted

	return chrome.Inset(gtx, layout.Inset{Top: 4, Bottom: 4, Left: 16, Right: 16}, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return btn.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					gtx.Constraints.Min.Y = gtx.Dp(36)
					return chrome.BorderSurface(gtx, unit.Dp(8), headerBg, headerBorder, 1, func(gtx layout.Context) layout.Dimensions {
						return chrome.Inset(gtx, layout.Inset{Top: 8, Bottom: 8, Left: 12, Right: 12}, func(gtx layout.Context) layout.Dimensions {
							return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
								layout.Rigid(func(gtx layout.Context) layout.Dimensions {
									return chrome.ActionIcon(gtx, uikit.IconCompose, unit.Dp(16), chrome.Colors.Primary)
								}),
								layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
									return chrome.Inset(gtx, layout.Inset{Left: 8, Right: 8}, func(gtx layout.Context) layout.Dimensions {
										return chrome.Label(gtx, displayTitle, unit.Sp(14), font.SemiBold, chrome.Colors.OnSurface, 1)
									})
								}),
								layout.Rigid(func(gtx layout.Context) layout.Dimensions {
									if adds == 0 {
										return layout.Dimensions{}
									}
									return chrome.Inset(gtx, layout.Inset{Right: 6}, func(gtx layout.Context) layout.Dimensions {
										return chrome.RoundedSurface(gtx, unit.Dp(4), chrome.Colors.DiffAddedContainer, func(gtx layout.Context) layout.Dimensions {
											return chrome.Inset(gtx, layout.Inset{Top: 2, Bottom: 2, Left: 6, Right: 6}, func(gtx layout.Context) layout.Dimensions {
												return chrome.Label(gtx, fmt.Sprintf("+%d", adds), unit.Sp(11), font.Bold, chrome.Colors.DiffAdded, 1)
											})
										})
									})
								}),
								layout.Rigid(func(gtx layout.Context) layout.Dimensions {
									if dels == 0 {
										return layout.Dimensions{}
									}
									return chrome.Inset(gtx, layout.Inset{Right: 6}, func(gtx layout.Context) layout.Dimensions {
										return chrome.RoundedSurface(gtx, unit.Dp(4), chrome.Colors.DiffDeletedContainer, func(gtx layout.Context) layout.Dimensions {
											return chrome.Inset(gtx, layout.Inset{Top: 2, Bottom: 2, Left: 6, Right: 6}, func(gtx layout.Context) layout.Dimensions {
												return chrome.Label(gtx, fmt.Sprintf("-%d", dels), unit.Sp(11), font.Bold, chrome.Colors.DiffDeleted, 1)
											})
										})
									})
								}),
								layout.Rigid(func(gtx layout.Context) layout.Dimensions {
									return chrome.ActionIcon(gtx, chevron, unit.Dp(14), chrome.Colors.OnSurfaceVariant)
								}),
							)
						})
					})
				})
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				if !input.Expanded || len(previewLines) == 0 {
					return layout.Dimensions{}
				}
				return chrome.Inset(gtx, layout.Inset{Top: 4}, func(gtx layout.Context) layout.Dimensions {
					return chrome.BorderSurface(gtx, unit.Dp(4), chrome.Colors.SurfaceContainerLowest, chrome.Colors.OutlineVariant, 1, func(gtx layout.Context) layout.Dimensions {
						return chrome.Inset(gtx, layout.Inset{Top: 8, Bottom: 8, Left: 12, Right: 12}, func(gtx layout.Context) layout.Dimensions {
							lines := make([]layout.FlexChild, 0, len(previewLines)+1)
							for _, l := range previewLines {
								lineStr := l
								lines = append(lines, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
									return LayoutDiffLine(gtx, lineStr, chrome)
								}))
							}
							if omitted > 0 {
								lines = append(lines, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
									return chrome.Inset(gtx, layout.Inset{Top: 4}, func(gtx layout.Context) layout.Dimensions {
										return chrome.Label(gtx, fmt.Sprintf("… and %d more lines", omitted), unit.Sp(11), font.Normal, chrome.Colors.Outline, 1)
									})
								}))
							}
							return layout.Flex{Axis: layout.Vertical}.Layout(gtx, lines...)
						})
					})
				})
			}),
		)
	})
}

func LayoutDiffLine(gtx layout.Context, line string, chrome Chrome) layout.Dimensions {
	colorVal := chrome.Colors.OnSurface
	bgVal := chrome.Colors.SurfaceContainerLowest
	hasBg := false

	if strings.HasPrefix(line, "+") {
		colorVal = chrome.Colors.DiffAdded
		bgVal = chrome.Colors.DiffAddedContainer
		hasBg = true
	} else if strings.HasPrefix(line, "-") {
		colorVal = chrome.Colors.DiffDeleted
		bgVal = chrome.Colors.DiffDeletedContainer
		hasBg = true
	} else if strings.HasPrefix(line, "@") {
		colorVal = chrome.Colors.Outline
	}

	content := func(gtx layout.Context) layout.Dimensions {
		return chrome.Inset(gtx, layout.Inset{Left: 4, Right: 4}, func(gtx layout.Context) layout.Dimensions {
			return chrome.Label(gtx, line, unit.Sp(12), font.Normal, colorVal, 1)
		})
	}

	if hasBg {
		return chrome.RoundedSurface(gtx, unit.Dp(2), bgVal, content)
	}
	return content(gtx)
}
