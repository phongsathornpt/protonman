package tool

import (
	"encoding/json"
	"fmt"
	"strings"
	"unicode/utf8"
)

// ArgumentsMap decodes the raw JSON arguments into a generic map.
// If arguments are empty or malformed JSON, an empty map is returned.
func (c Call) ArgumentsMap() map[string]any {
	if len(c.Arguments) == 0 {
		return make(map[string]any)
	}
	var args map[string]any
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
	args := c.ArgumentsMap()

	switch c.Name {
	case "read_file":
		if path := ExtractString(args, "path", "file_path", "file"); path != "" {
			return fmt.Sprintf("Read %s", path)
		}
		return "Read file"
	case "write_file":
		if path := ExtractString(args, "file_path", "path", "file"); path != "" {
			return fmt.Sprintf("Write %s", path)
		}
		return "Write file"
	case "search_replace":
		if path := ExtractString(args, "file_path", "path", "file"); path != "" {
			return fmt.Sprintf("Edit %s", path)
		}
		return "Search and replace"
	case "apply_patch":
		if path := ExtractString(args, "path", "file_path", "file"); path != "" {
			return fmt.Sprintf("Patch %s", path)
		}
		if patch := ExtractString(args, "patch"); patch != "" {
			paths := ParsePatchPaths(patch)
			if len(paths) == 1 {
				return fmt.Sprintf("Patch %s", paths[0])
			} else if len(paths) > 1 {
				return fmt.Sprintf("Patch %s (+%d files)", paths[0], len(paths)-1)
			}
		}
		return "Apply patch"
	case "list_dir":
		if path := ExtractString(args, "path", "dir_path", "directory", "dir"); path != "" {
			return fmt.Sprintf("List %s", path)
		}
		return "List directory"
	case "grep":
		pattern := ExtractString(args, "pattern", "query")
		path := ExtractString(args, "path", "dir_path", "directory")
		if pattern != "" {
			if path != "" && path != "." {
				return fmt.Sprintf("Search %q in %s", TruncateRunes(pattern, 30), path)
			}
			return fmt.Sprintf("Search %q", TruncateRunes(pattern, 40))
		}
		return "Search workspace"
	case "bash":
		if cmd := ExtractString(args, "command", "cmd"); cmd != "" {
			return fmt.Sprintf("Run: %s", TruncateRunes(cmd, 40))
		}
		return "Run shell command"
	case "web_fetch":
		if url := ExtractString(args, "url"); url != "" {
			return fmt.Sprintf("Fetch %s", TruncateRunes(url, 45))
		}
		return "Fetch URL"
	case "web_search":
		if query := ExtractString(args, "query", "pattern"); query != "" {
			return fmt.Sprintf("Search web: %s", TruncateRunes(query, 35))
		}
		return "Search web"
	case "git_status":
		if path := ExtractString(args, "path"); path != "" && path != "." {
			return fmt.Sprintf("Git status (%s)", path)
		}
		return "Check git status"
	case "get_todo":
		return "Check task list"
	case "update_todo":
		if ops, ok := args["operations"].([]any); ok && len(ops) > 0 {
			return fmt.Sprintf("Update tasks (%d changes)", len(ops))
		}
		return "Update tasks"
	case "activate_skill":
		if name := ExtractString(args, "name", "skill"); name != "" {
			return fmt.Sprintf("Activate skill %s", name)
		}
		return "Activate skill"
	case "delegate_task":
		task := ExtractString(args, "task")
		profile := ExtractString(args, "profile")
		if profile != "" && task != "" {
			return fmt.Sprintf("Delegate [%s]: %s", profile, TruncateRunes(task, 30))
		}
		if task != "" {
			return fmt.Sprintf("Delegate: %s", TruncateRunes(task, 30))
		}
		return "Delegate subtask"
	case "wait_agent":
		if id := ExtractString(args, "agent_id", "id"); id != "" {
			return fmt.Sprintf("Wait for agent %s", TruncateRunes(id, 20))
		}
		return "Wait for agent"
	case "get_agent":
		if id := ExtractString(args, "agent_id", "id"); id != "" {
			return fmt.Sprintf("Get agent status %s", TruncateRunes(id, 20))
		}
		return "Get agent status"
	case "list_agents":
		return "List subagents"
	case "cancel_agent":
		if id := ExtractString(args, "agent_id", "id"); id != "" {
			return fmt.Sprintf("Cancel agent %s", TruncateRunes(id, 20))
		}
		return "Cancel agent"
	case "checkpoint_restore":
		if id := ExtractString(args, "checkpoint_id", "id"); id != "" {
			return fmt.Sprintf("Restore checkpoint %s", id)
		}
		return "Restore checkpoint"
	default:
		return c.Name
	}
}

// Target inspects the tool call and returns a human-facing target
// string (e.g. URL, filepath, pattern, command, subagent ID).
func (c Call) Target() string {
	args := c.ArgumentsMap()
	kind := c.Kind()

	switch kind {
	case KindWebFetch:
		if urlStr := ExtractString(args, "url"); urlStr != "" {
			return urlStr
		}
	case KindWebSearch:
		if query := ExtractString(args, "query", "pattern"); query != "" {
			return fmt.Sprintf("%q", query)
		}
	case KindRead:
		if c.Name == "list_dir" {
			if path := ExtractString(args, "path", "dir_path", "directory", "dir"); path != "" {
				return path
			}
			return "."
		}
		if c.Name == "git_status" {
			if path := ExtractString(args, "path"); path != "" {
				return path
			}
			return ""
		}
		if path := ExtractString(args, "path", "file_path", "file"); path != "" {
			return path
		}
	case KindGrep:
		pattern := ExtractString(args, "pattern", "query")
		path := ExtractString(args, "path", "dir_path", "directory")
		if pattern != "" && path != "" && path != "." {
			return fmt.Sprintf("%q in %s", pattern, path)
		}
		if pattern != "" {
			return fmt.Sprintf("%q", pattern)
		}
	case KindBash:
		if cmd := ExtractString(args, "command", "cmd"); cmd != "" {
			return cmd
		}
	case KindEdit:
		if path := ExtractString(args, "file_path", "path", "file", "filename", "target"); path != "" {
			return path
		}
		if patch := ExtractString(args, "patch"); patch != "" {
			paths := ParsePatchPaths(patch)
			if len(paths) == 1 {
				return paths[0]
			} else if len(paths) > 1 {
				return fmt.Sprintf("%s (+%d files)", paths[0], len(paths)-1)
			}
		}
	case KindTask:
		if operations, ok := args["operations"].([]any); ok {
			return fmt.Sprintf("%d task operations", len(operations))
		}
		return "task plan"
	case KindAgent:
		if c.Name == "delegate_task" {
			task := ExtractString(args, "task")
			profile := ExtractString(args, "profile")
			if profile != "" && task != "" {
				return fmt.Sprintf("[%s] %s", profile, TruncateRunes(task, 40))
			}
			if task != "" {
				return TruncateRunes(task, 40)
			}
		}
		if id := ExtractString(args, "agent_id", "id"); id != "" {
			return id
		}
		return "subagents"
	}

	if c.Name == "activate_skill" {
		if skillName := ExtractString(args, "name", "skill"); skillName != "" {
			return fmt.Sprintf("%q", skillName)
		}
	}
	if c.Name == "delegate_task" {
		task := ExtractString(args, "task")
		profile := ExtractString(args, "profile")
		if profile != "" && task != "" {
			return fmt.Sprintf("[%s] %s", profile, TruncateRunes(task, 40))
		}
		if task != "" {
			return TruncateRunes(task, 40)
		}
	}
	if c.Name == "checkpoint_restore" {
		if id := ExtractString(args, "checkpoint_id", "id"); id != "" {
			return id
		}
	}

	// Heuristic fallback for arbitrary MCP and custom tools:
	return ExtractString(args, "url", "file_path", "path", "file", "query", "pattern", "command", "target", "task", "name")
}

// DisplayName returns a clean, human-readable action label for a tool name.
func DisplayName(name string) string {
	switch name {
	case "read_file":
		return "Read"
	case "list_dir":
		return "List"
	case "write_file":
		return "Write"
	case "search_replace":
		return "Edit"
	case "apply_patch":
		return "Patch"
	case "grep":
		return "Search"
	case "bash":
		return "Run"
	case "web_fetch":
		return "Fetch"
	case "web_search":
		return "Search web"
	case "git_status":
		return "Git status"
	case "get_todo":
		return "Tasks"
	case "update_todo":
		return "Update tasks"
	case "activate_skill":
		return "Skill"
	case "delegate_task":
		return "Delegate"
	case "wait_agent":
		return "Wait agent"
	case "get_agent":
		return "Agent status"
	case "list_agents":
		return "Subagents"
	case "cancel_agent":
		return "Cancel agent"
	case "checkpoint_restore":
		return "Restore"
	default:
		clean := strings.TrimPrefix(name, "mcp.")
		if idx := strings.LastIndex(clean, "."); idx != -1 {
			clean = clean[idx+1:]
		}
		return clean
	}
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
	args := c.ArgumentsMap()

	switch c.Name {
	case "read_file", "write_file", "search_replace":
		if path := ExtractString(args, "file_path", "path", "file", "filename", "target", "destination", "move_path"); path != "" {
			return []string{path}
		}
	case "apply_patch":
		if path := ExtractString(args, "path", "file_path", "file", "filename", "target"); path != "" {
			return []string{path}
		}
		for _, key := range []string{"patch", "diff", "input"} {
			if patch := ExtractString(args, key); patch != "" {
				if paths := ParsePatchPaths(patch); len(paths) > 0 {
					return paths
				}
			}
		}
	default:
		for _, key := range []string{"patch", "diff", "input"} {
			if patch := ExtractString(args, key); patch != "" {
				if paths := ParsePatchPaths(patch); len(paths) > 0 {
					return paths
				}
			}
		}
		if path := ExtractString(args, "file_path", "path", "file", "filename", "target", "destination", "move_path"); path != "" {
			if KindForName(c.Name) == KindEdit || KindForName(c.Name) == KindRead {
				return []string{path}
			}
		}
	}
	return nil
}

// KindForName returns the canonical tool.Kind for a given tool name.
func KindForName(name string) Kind {
	switch name {
	case "read_file", "list_dir", "git_status":
		return KindRead
	case "write_file", "search_replace", "apply_patch", "checkpoint_restore":
		return KindEdit
	case "grep":
		return KindGrep
	case "web_search":
		return KindWebSearch
	case "bash":
		return KindBash
	case "web_fetch":
		return KindWebFetch
	case "get_todo", "update_todo":
		return KindTask
	case "delegate_task", "wait_agent", "get_agent", "list_agents", "cancel_agent":
		return KindAgent
	default:
		return ""
	}
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
	runes := []rune(s)
	if len(runes) <= maxRunes {
		return s
	}
	if maxRunes <= 1 {
		return string(runes[:maxRunes])
	}
	return string(runes[:maxRunes-1]) + "…"
}

// ParsePatchPaths extracts affected file paths from Codex-style patches and unified diffs.
func ParsePatchPaths(patch string) []string {
	if strings.TrimSpace(patch) == "" {
		return nil
	}
	var paths []string
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
