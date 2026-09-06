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

func (m *bubbleModel) reloadTodoAfterExternalTool(call tool.Call, result tool.Result, callErr error) {
	if m == nil || callErr != nil || m.todoStore == nil || !todoCallMayAffectFile(call, result, m.workDir) {
		return
	}
	reloader, ok := m.todoStore.(tododomain.ReloadableRepository)
	if !ok {
		return
	}
	if _, err := reloader.Reload(m.ctx); err != nil {
		m.appendError("reload " + tododomain.DefaultFilename + ": " + err.Error())
		return
	}
	m.syncTodoSnapshot()
}

func todoCallMayAffectFile(call tool.Call, result tool.Result, workDir string) bool {
	for _, affected := range result.AffectedPaths {
		path := filepath.Clean(strings.TrimSpace(affected))
		if filepath.IsAbs(path) {
			if path == filepath.Join(filepath.Clean(workDir), tododomain.DefaultFilename) {
				return true
			}
		} else if path == tododomain.DefaultFilename {
			return true
		}
	}
	switch call.Name {
	case "write_file", "search_replace", "apply_patch":
		// Built-in file mutators publish AffectedPaths; no path means no successful mutation.
		return false
	case "bash":
		if len(result.AffectedPaths) > 0 {
			return false
		}
		var input struct {
			Command string `json:"command"`
		}
		if json.Unmarshal(call.Arguments, &input) != nil {
			return false
		}
		analysis := tool.AnalyzeCommand(input.Command)
		if analysis.Effect == tool.CommandEffectReadOnly {
			return false
		}
		return strings.Contains(input.Command, tododomain.DefaultFilename)
	case "checkpoint_restore":
		return true
	default:
		return false
	}
}
