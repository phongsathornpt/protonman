package builtin

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/phongsathornpt/protonman/internal/core/tool"
)

func TestBuiltinToolSSOTContract(t *testing.T) {
	sampleCalls := []struct {
		name            string
		args            map[string]any
		wantKind        tool.Kind
		wantDisplayName string
		targetSub       string
		titlePrefix     string
	}{
		{
			name:            "read",
			args:            map[string]any{"path": "pkg/api.go"},
			wantKind:        tool.KindRead,
			wantDisplayName: "Read",
			targetSub:       "pkg/api.go",
			titlePrefix:     "Read pkg/api.go",
		},
		{
			name:            "math",
			args:            map[string]any{"expression": "2+2"},
			wantKind:        tool.KindCompute,
			wantDisplayName: "Calculate",
			targetSub:       "2+2",
			titlePrefix:     "Calculate 2+2",
		},
		{
			name:            "edit",
			args:            map[string]any{"action": "write", "file_path": "pkg/out.go", "content": "package pkg"},
			wantKind:        tool.KindEdit,
			wantDisplayName: "Edit",
			targetSub:       "pkg/out.go",
			titlePrefix:     "Write pkg/out.go",
		},
		{
			name:            "edit",
			args:            map[string]any{"action": "replace", "file_path": "pkg/out.go", "old_string": "a", "new_string": "b"},
			wantKind:        tool.KindEdit,
			wantDisplayName: "Edit",
			targetSub:       "pkg/out.go",
			titlePrefix:     "Edit pkg/out.go",
		},
		{
			name: "edit",
			args: map[string]any{
				"action": "patch",
				"patch":  "*** Begin Patch\n*** Update File: config.json\n@@ -1 +1 @@\n*** End Patch",
			},
			wantKind:        tool.KindEdit,
			wantDisplayName: "Edit",
			targetSub:       "config.json",
			titlePrefix:     "Patch config.json",
		},
		{
			name:            "ls",
			args:            map[string]any{"path": "cmd"},
			wantKind:        tool.KindRead,
			wantDisplayName: "List",
			targetSub:       "cmd",
			titlePrefix:     "List cmd",
		},
		{
			name:            "grep",
			args:            map[string]any{"pattern": "func", "path": "pkg"},
			wantKind:        tool.KindGrep,
			wantDisplayName: "Search",
			targetSub:       `"func" in pkg`,
			titlePrefix:     `Search "func" in pkg`,
		},
		{
			name:            "bash",
			args:            map[string]any{"command": "go test ./..."},
			wantKind:        tool.KindBash,
			wantDisplayName: "Run",
			targetSub:       "go test ./...",
			titlePrefix:     "Run: go test ./...",
		},
		{
			name:            "web",
			args:            map[string]any{"action": "fetch", "url": "https://example.com/api"},
			wantKind:        tool.KindWeb,
			wantDisplayName: "Web",
			targetSub:       "https://example.com/api",
			titlePrefix:     "Fetch https://example.com/api",
		},
		{
			name:            "web",
			args:            map[string]any{"action": "search", "query": "golang testing"},
			wantKind:        tool.KindWeb,
			wantDisplayName: "Web",
			targetSub:       `"golang testing"`,
			titlePrefix:     "Search web: golang testing",
		},
		{
			name:            "git",
			args:            map[string]any{"action": "status", "path": "."},
			wantKind:        tool.KindGit,
			wantDisplayName: "Git",
			targetSub:       "",
			titlePrefix:     "Check git status",
		},
		{
			name:            "todo",
			args:            map[string]any{"action": "get"},
			wantKind:        tool.KindTask,
			wantDisplayName: "Tasks",
			targetSub:       "task plan",
			titlePrefix:     "Check task list",
		},
		{
			name:            "todo",
			args:            map[string]any{"action": "update", "operations": []any{"op1"}},
			wantKind:        tool.KindTask,
			wantDisplayName: "Tasks",
			targetSub:       "1 task operations",
			titlePrefix:     "Update tasks (1 changes)",
		},
		{
			name:            "skill",
			args:            map[string]any{"name": "git-commit"},
			wantKind:        tool.KindRead,
			wantDisplayName: "Skill",
			targetSub:       `"git-commit"`,
			titlePrefix:     "Activate skill git-commit",
		},
		{
			name:            "subagent",
			args:            map[string]any{"action": "spawn", "profile": "researcher", "task": "analyze security"},
			wantKind:        tool.KindAgent,
			wantDisplayName: "Subagent",
			targetSub:       "[researcher] analyze security",
			titlePrefix:     "Delegate [researcher]: analyze security",
		},
		{
			name:            "subagent",
			args:            map[string]any{"action": "wait", "timeout_seconds": 30},
			wantKind:        tool.KindAgent,
			wantDisplayName: "Subagent",
			targetSub:       "subagents",
			titlePrefix:     "Wait for agent activity",
		},
		{
			name:            "subagent",
			args:            map[string]any{"action": "get", "agent_id": "agent-99"},
			wantKind:        tool.KindAgent,
			wantDisplayName: "Subagent",
			targetSub:       "agent-99",
			titlePrefix:     "Get agent status agent-99",
		},
		{
			name:            "subagent",
			args:            map[string]any{"action": "list"},
			wantKind:        tool.KindAgent,
			wantDisplayName: "Subagent",
			targetSub:       "subagents",
			titlePrefix:     "List subagents",
		},
		{
			name:            "subagent",
			args:            map[string]any{"action": "cancel", "agent_id": "agent-99"},
			wantKind:        tool.KindAgent,
			wantDisplayName: "Subagent",
			targetSub:       "agent-99",
			titlePrefix:     "Cancel agent agent-99",
		},
		{
			name:            "edit",
			args:            map[string]any{"action": "restore", "checkpoint_id": "chk-42"},
			wantKind:        tool.KindEdit,
			wantDisplayName: "Edit",
			targetSub:       "chk-42",
			titlePrefix:     "Restore checkpoint chk-42",
		},
	}

	for _, tt := range sampleCalls {
		t.Run(tt.name, func(t *testing.T) {
			rawArgs, err := json.Marshal(tt.args)
			if err != nil {
				t.Fatalf("marshal error: %v", err)
			}
			call := tool.Call{
				ID:        "call-" + tt.name,
				Name:      tt.name,
				Arguments: rawArgs,
			}

			// 1. Check Tool Kind
			gotKind := tool.KindForName(tt.name)
			if gotKind != tt.wantKind {
				t.Errorf("KindForName(%q) = %q, want %q", tt.name, gotKind, tt.wantKind)
			}

			// 2. Check Target extraction
			gotTarget := call.Target()
			if tt.targetSub != "" && !strings.Contains(gotTarget, tt.targetSub) {
				t.Errorf("call.Target() = %q, missing substring %q", gotTarget, tt.targetSub)
			}

			// 3. Check Title generation
			gotTitle := call.Title()
			if !strings.HasPrefix(gotTitle, tt.titlePrefix) {
				t.Errorf("call.Title() = %q, want prefix %q", gotTitle, tt.titlePrefix)
			}

			// 4. Check DisplayName SSOT contract (No raw underscores allowed in human-facing names!)
			gotDisplayName := tool.DisplayName(tt.name)
			if gotDisplayName != tt.wantDisplayName {
				t.Errorf("DisplayName(%q) = %q, want %q", tt.name, gotDisplayName, tt.wantDisplayName)
			}
			if strings.Contains(gotDisplayName, "_") {
				t.Errorf("DisplayName(%q) contains raw underscore: %q", tt.name, gotDisplayName)
			}
			if call.DisplayName() != tt.wantDisplayName {
				t.Errorf("call.DisplayName() = %q, want %q", call.DisplayName(), tt.wantDisplayName)
			}
		})
	}
}
