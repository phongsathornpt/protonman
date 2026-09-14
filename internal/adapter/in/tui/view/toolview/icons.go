package toolview

import (
	"path/filepath"
	"strings"

	tuistyle "github.com/phongsathornpt/protonman/internal/adapter/in/tui/view/style"
	"github.com/phongsathornpt/protonman/internal/core/tool"
)

// KindGlyphWithIcons returns the semantic glyph for a tool using the supplied
// terminal icon profile. Existing KindGlyph remains the Unicode compatibility
// path while callers migrate incrementally.
func KindGlyphWithIcons(icons tuistyle.IconSet, kind tool.Kind, name string) string {
	icons = tuistyle.OrUnicodeIcons(icons)
	switch kind {
	case tool.KindWeb:
		return icons.Web
	case tool.KindRead:
		trimmedName := strings.TrimSpace(name)
		if trimmedName == tool.NameLS {
			return icons.Dir
		}
		if trimmedName == "image" || trimmedName == "img2llm" || trimmedName == "image2llm" {
			if icons.Image != "" {
				return icons.Image
			}
			return tuistyle.ASCIIImage
		}
		return icons.Read
	case tool.KindGrep:
		return icons.Search
	case tool.KindGit:
		return icons.Git
	case tool.KindBash:
		return icons.Exec
	case tool.KindEdit:
		return icons.Edit
	case tool.KindTask:
		return icons.TodoActive
	case tool.KindAgent:
		return icons.Agent
	}
	if strings.TrimSpace(name) == tool.NameSkill {
		return icons.Skill
	}
	return icons.Generic
}

// KindGlyphWithTarget returns the semantic glyph for a tool using the supplied
// terminal icon profile, taking the target artifact kind into consideration.
func KindGlyphWithTarget(icons tuistyle.IconSet, kind tool.Kind, name string, target string) string {
	icons = tuistyle.OrUnicodeIcons(icons)
	if (kind == tool.KindRead || strings.TrimSpace(name) == tool.NameRead) && isImageArtifactPath(target) {
		if icons.Image != "" {
			return icons.Image
		}
		return tuistyle.ASCIIImage
	}
	return KindGlyphWithIcons(icons, kind, name)
}

func isImageArtifactPath(target string) bool {
	clean := strings.TrimSpace(target)
	clean = strings.Trim(clean, "\"'`")
	if clean == "" {
		return false
	}
	switch strings.ToLower(filepath.Ext(clean)) {
	case ".png", ".jpg", ".jpeg", ".gif", ".webp", ".bmp", ".svg", ".ico", ".tiff", ".tif":
		return true
	default:
		return false
	}
}
