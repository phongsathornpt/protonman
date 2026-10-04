//go:build desktop || desktop_gio

package conversation

import (
	"strings"

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
