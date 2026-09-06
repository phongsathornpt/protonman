package tui

import (
	"encoding/json"
	"path/filepath"
	"strings"

	tododomain "github.com/projectTHORN/proton/internal/todo"
	"github.com/projectTHORN/proton/internal/tool"
)

// TodoItem is kept as a compatibility alias while TODO ownership lives in the
// domain package rather than the terminal adapter.
type TodoItem = tododomain.Item

// ParseTODO preserves the existing TUI-facing parser entry point while using
// the provider-neutral TODO domain parser.
func ParseTODO(markdown string) []TodoItem {
	return tododomain.ParseMarkdown(markdown)
}

func (m *bubbleModel) syncTodoSnapshot() bool {
	if m == nil || m.todoStore == nil {
		return false
	}
	snapshot := m.todoStore.Snapshot()
	if snapshot.Revision == m.todoRevision {
		return false
	}
	m.todo = tododomain.CloneItems(snapshot.Items)
	m.todoRevision = snapshot.Revision
	return true
}

func (m *bubbleModel) reloadTodoAfterExternalTool(call tool.Call, callErr error) {
	if m == nil || callErr != nil || m.todoStore == nil || !todoCallMayAffectFile(call, m.workDir) {
		return
	}
	reloader, ok := m.todoStore.(tododomain.ReloadableRepository)
	if !ok {
		return
	}
	if _, err := reloader.Reload(m.ctx); err != nil {
		m.appendError("reload TODO.md: " + err.Error())
		return
	}
	m.syncTodoSnapshot()
}

func todoCallMayAffectFile(call tool.Call, workDir string) bool {
	switch call.Name {
	case "write_file", "search_replace":
		var input struct {
			FilePath string `json:"file_path"`
		}
		if json.Unmarshal(call.Arguments, &input) != nil {
			return false
		}
		path := filepath.Clean(strings.TrimSpace(input.FilePath))
		if filepath.IsAbs(path) {
			return filepath.Clean(path) == filepath.Join(filepath.Clean(workDir), "TODO.md")
		}
		return path == "TODO.md"
	case "apply_patch":
		return strings.Contains(string(call.Arguments), "TODO.md")
	case "bash":
		var input struct {
			Command string `json:"command"`
		}
		if json.Unmarshal(call.Arguments, &input) != nil {
			return false
		}
		return strings.Contains(input.Command, "TODO.md")
	case "checkpoint_restore":
		return true
	default:
		return false
	}
}
