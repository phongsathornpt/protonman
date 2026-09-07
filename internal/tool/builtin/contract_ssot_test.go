package builtin

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/projectTHORN/proton/internal/tool"
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
			name:            "read_file",
			args:            map[string]any{"path": "pkg/api.go"},
			wantKind:        tool.KindRead,
			wantDisplayName: "Read",
			targetSub:       "pkg/api.go",
			titlePrefix:     "Read pkg/api.go",
		},
		{
			name:            "write_file",
			args:            map[string]any{"file_path": "pkg/out.go", "content": "package pkg"},
			wantKind:        tool.KindEdit,
			wantDisplayName: "Write",
			targetSub:       "pkg/out.go",
			titlePrefix:     "Write pkg/out.go",
		},
		{
			name:            "search_replace",
			args:            map[string]any{"file_path": "pkg/out.go", "old_string": "a", "new_string": "b"},
			wantKind:        tool.KindEdit,
			wantDisplayName: "Edit",
			targetSub:       "pkg/out.go",
			titlePrefix:     "Edit pkg/out.go",
		},
		{
			name: "apply_patch",
			args: map[string]any{
				"patch": "*** Begin Patch\n*** Update File: config.json\n@@ -1 +1 @@\n*** End Patch",
			},
			wantKind:        tool.KindEdit,
			wantDisplayName: "Patch",
			targetSub:       "config.json",
			titlePrefix:     "Patch config.json",
		},
		{
			name:            "list_dir",
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
			name:            "web_fetch",
			args:            map[string]any{"url": "https://example.com/api"},
			wantKind:        tool.KindWebFetch,
			wantDisplayName: "Fetch",
			targetSub:       "https://example.com/api",
			titlePrefix:     "Fetch https://example.com/api",
		},
		{
			name:            "web_search",
			args:            map[string]any{"query": "golang testing"},
			wantKind:        tool.KindWebSearch,
			wantDisplayName: "Search web",
			targetSub:       `"golang testing"`,
			titlePrefix:     "Search web: golang testing",
		},
		{
			name:            "git_status",
			args:            map[string]any{"path": "."},
			wantKind:        tool.KindRead,
			wantDisplayName: "Git status",
			targetSub:       "",
			titlePrefix:     "Check git status",
		},
		{
			name:            "get_todo",
			args:            map[string]any{},
			wantKind:        tool.KindTask,
			wantDisplayName: "Tasks",
			targetSub:       "task plan",
			titlePrefix:     "Check task list",
		},
		{
			name:            "update_todo",
			args:            map[string]any{"operations": []any{"op1"}},
			wantKind:        tool.KindTask,
			wantDisplayName: "Update tasks",
			targetSub:       "1 task operations",
			titlePrefix:     "Update tasks (1 changes)",
		},
		{
			name:            "activate_skill",
			args:            map[string]any{"name": "git-commit"},
			wantKind:        "",
			wantDisplayName: "Skill",
			targetSub:       `"git-commit"`,
			titlePrefix:     "Activate skill git-commit",
		},
		{
			name:            "delegate_task",
			args:            map[string]any{"profile": "researcher", "task": "analyze security"},
			wantKind:        tool.KindAgent,
			wantDisplayName: "Delegate",
			targetSub:       "[researcher] analyze security",
			titlePrefix:     "Delegate [researcher]: analyze security",
		},
		{
			name:            "wait_agent",
			args:            map[string]any{"agent_id": "agent-99"},
			wantKind:        tool.KindAgent,
			wantDisplayName: "Wait agent",
			targetSub:       "agent-99",
			titlePrefix:     "Wait for agent agent-99",
		},
		{
			name:            "get_agent",
			args:            map[string]any{"agent_id": "agent-99"},
			wantKind:        tool.KindAgent,
			wantDisplayName: "Agent status",
			targetSub:       "agent-99",
			titlePrefix:     "Get agent status agent-99",
		},
		{
			name:            "list_agents",
			args:            map[string]any{},
			wantKind:        tool.KindAgent,
			wantDisplayName: "Subagents",
			targetSub:       "subagents",
			titlePrefix:     "List subagents",
		},
		{
			name:            "cancel_agent",
			args:            map[string]any{"agent_id": "agent-99"},
			wantKind:        tool.KindAgent,
			wantDisplayName: "Cancel agent",
			targetSub:       "agent-99",
			titlePrefix:     "Cancel agent agent-99",
		},
		{
			name:            "checkpoint_restore",
			args:            map[string]any{"checkpoint_id": "chk-42"},
			wantKind:        tool.KindEdit,
			wantDisplayName: "Restore",
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
