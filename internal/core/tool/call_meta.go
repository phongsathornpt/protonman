package tool

import (
	"encoding/json"
	"strings"
	"unicode/utf8"

	"github.com/phongsathornpt/protonman/internal/base/strutil"
)

// ArgumentsMap decodes the raw JSON arguments into a generic map.
// If arguments are empty or malformed JSON, an empty map is returned.
func (c Call) ArgumentsMap() map[string]any {
	if len(c.Arguments) == 0 {
		return make(map[string]any)
	}
	args := map[string]any{}
	if err := json.Unmarshal(c.Arguments, &args); err != nil || args == nil {
		return make(map[string]any)
	}
	return args
}

// Kind returns the classified tool.Kind for this call based on its Name.
func (c Call) Kind() Kind {
	return KindForName(c.Name)
}

// Title produces a human-readable title describing what the tool call is doing.
func (c Call) Title() string {
	canonical := c
	if spec, ok := metadataForName(canonical.Name); ok && spec.title != nil {
		return spec.title(canonical.ArgumentsMap())
	}
	return c.Name
}

// Target inspects the tool call and returns a human-facing target
// string (e.g. URL, filepath, pattern, command, subagent ID).
func (c Call) Target() string {
	canonical := c
	args := canonical.ArgumentsMap()
	if spec, ok := metadataForName(canonical.Name); ok && spec.target != nil {
		return spec.target(args)
	}
	// Heuristic fallback for arbitrary MCP and custom tools.
	return ExtractString(args, "url", "file_path", "path", "file", "query", "pattern", "command", "target", "task", "name")
}

// DisplayName returns a clean, human-readable action label for a tool name.
func DisplayName(name string) string {
	if metadata, ok := MetadataForName(name); ok {
		return metadata.DisplayName
	}
	clean := strings.TrimPrefix(name, "mcp.")
	if idx := strings.LastIndex(clean, "."); idx != -1 {
		clean = clean[idx+1:]
	}
	return clean
}

// DisplayName returns the human-readable action label for the tool call.
func (c Call) DisplayName() string {
	return DisplayName(c.Name)
}

// DisplayName returns the human-readable action label for the tool definition.
func (d Definition) DisplayName() string {
	return DisplayName(d.Name)
}

// AffectedPaths returns all file paths affected or accessed by the tool call.
func (c Call) AffectedPaths() []string {
	canonical := c
	args := canonical.ArgumentsMap()
	if spec, ok := metadataForName(canonical.Name); ok && spec.affectedPaths != nil {
		return spec.affectedPaths(args)
	}
	for _, key := range []string{"patch", "diff", "input"} {
		if patch := ExtractString(args, key); patch != "" {
			if paths := ParsePatchPaths(patch); len(paths) > 0 {
				return paths
			}
		}
	}
	if path := ExtractString(args, "file_path", "path", "file", "filename", "target", "destination", "move_path"); path != "" {
		if kind := KindForName(canonical.Name); kind == KindEdit || kind == KindRead {
			return []string{path}
		}
	}
	return nil
}

// KindForName returns the canonical tool.Kind for a given tool name.
func KindForName(name string) Kind {
	if metadata, ok := MetadataForName(name); ok {
		return metadata.Kind
	}
	return ""
}

// ExtractString retrieves the first non-empty string among the given keys from args.
func ExtractString(args map[string]any, keys ...string) string {
	for _, key := range keys {
		if val, ok := args[key].(string); ok {
			trimmed := strings.TrimSpace(val)
			if trimmed != "" {
				return trimmed
			}
		}
	}
	return ""
}

// TruncateRunes safely truncates a string to at most maxRunes, avoiding multi-byte
// UTF-8 splitting, and appends a typographic ellipsis '…'.
func TruncateRunes(s string, maxRunes int) string {
	s = strings.TrimSpace(s)
	if maxRunes <= 0 {
		return ""
	}
	if maxRunes == 1 {
		runes := []rune(s)
		if len(runes) == 0 {
			return ""
		}
		return string(runes[:1])
	}
	return strutil.TruncateRunesWithEllipsis(s, maxRunes)
}

// ParsePatchPaths extracts affected file paths from Codex-style patches and unified diffs.
func ParsePatchPaths(patch string) []string {
	if strings.TrimSpace(patch) == "" {
		return nil
	}
	paths := []string{}
	seen := make(map[string]struct{})
	addPath := func(p string) {
		p = strings.TrimSpace(p)
		p = strings.Trim(p, `"'`)
		if p == "" || p == "/dev/null" {
			return
		}
		if _, exists := seen[p]; !exists {
			seen[p] = struct{}{}
			paths = append(paths, p)
		}
	}

	lines := strings.Split(patch, "\n")
	for _, rawLine := range lines {
		line := strings.TrimSpace(rawLine)
		switch {
		case strings.HasPrefix(line, "*** Add File: "):
			addPath(strings.TrimPrefix(line, "*** Add File: "))
		case strings.HasPrefix(line, "*** Delete File: "):
			addPath(strings.TrimPrefix(line, "*** Delete File: "))
		case strings.HasPrefix(line, "*** Update File: "):
			addPath(strings.TrimPrefix(line, "*** Update File: "))
		case strings.HasPrefix(line, "*** Move to: "):
			addPath(strings.TrimPrefix(line, "*** Move to: "))
		case strings.HasPrefix(line, "+++ b/"):
			addPath(strings.TrimPrefix(line, "+++ b/"))
		case strings.HasPrefix(line, "--- a/"):
			addPath(strings.TrimPrefix(line, "--- a/"))
		}
	}
	return paths
}

// Ensure utf8 package is referenced if needed, or suppress unused import.
var _ = utf8.ValidString
