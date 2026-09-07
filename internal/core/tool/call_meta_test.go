package tool

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
	"unicode/utf8"
)

func makeTestCall(name string, args map[string]any) Call {
	data, _ := json.Marshal(args)
	return Call{
		ID:        "test-call-1",
		Name:      name,
		Arguments: data,
	}
}

func TestCallTitle(t *testing.T) {
	tests := []struct {
		name string
		call Call
		want string
	}{
		{
			name: "read_file with path",
			call: makeTestCall("read_file", map[string]any{"path": "main.go"}),
			want: "Read main.go",
		},
		{
			name: "read_file with file_path alias",
			call: makeTestCall("read_file", map[string]any{"file_path": "main.go"}),
			want: "Read main.go",
		},
		{
			name: "read_file empty",
			call: makeTestCall("read_file", map[string]any{}),
			want: "Read file",
		},
		{
			name: "write_file with file_path",
			call: makeTestCall("write_file", map[string]any{"file_path": "server.go"}),
			want: "Write server.go",
		},
		{
			name: "write_file with path fallback",
			call: makeTestCall("write_file", map[string]any{"path": "server.go"}),
			want: "Write server.go",
		},
		{
			name: "search_replace with file_path",
			call: makeTestCall("search_replace", map[string]any{"file_path": "handler.go"}),
			want: "Edit handler.go",
		},
		{
			name: "apply_patch with patch single file",
			call: makeTestCall("apply_patch", map[string]any{
				"patch": "*** Begin Patch\n*** Update File: config.yaml\n@@ -1 +1 @@\n*** End Patch",
			}),
			want: "Patch config.yaml",
		},
		{
			name: "apply_patch with patch multi file",
			call: makeTestCall("apply_patch", map[string]any{
				"patch": "*** Begin Patch\n*** Update File: a.go\n*** Update File: b.go\n*** End Patch",
			}),
			want: "Patch a.go (+1 files)",
		},
		{
			name: "list_dir with dir_path",
			call: makeTestCall("list_dir", map[string]any{"dir_path": "cmd"}),
			want: "List cmd",
		},
		{
			name: "grep with pattern and path",
			call: makeTestCall("grep", map[string]any{"pattern": "Proton", "path": "internal"}),
			want: `Search "Proton" in internal`,
		},
		{
			name: "grep with query fallback",
			call: makeTestCall("grep", map[string]any{"query": "Proton"}),
			want: `Search "Proton"`,
		},
		{
			name: "bash command truncated",
			call: makeTestCall("bash", map[string]any{"command": "git commit -m 'feat(tool): add centralized call metadata and helpers'"}),
			want: "Run: git commit -m 'feat(tool): add centrali…",
		},
		{
			name: "bash unicode Thai runes",
			call: makeTestCall("bash", map[string]any{"command": "echo 'สวัสดีชาวโลกทุกคนที่กำลังทดสอบระบบนี้อยู่นะครับ'"}),
			want: "Run: echo 'สวัสดีชาวโลกทุกคนที่กำลังทดสอบระบ…",
		},
		{
			name: "web_fetch url",
			call: makeTestCall("web_fetch", map[string]any{"url": "https://protonman.dev"}),
			want: "Fetch https://protonman.dev",
		},
		{
			name: "web_search query",
			call: makeTestCall("web_search", map[string]any{"query": "agent client protocol specification and guidelines"}),
			want: "Search web: agent client protocol specificatio…",
		},
		{
			name: "git_status with path",
			call: makeTestCall("git_status", map[string]any{"path": "pkg"}),
			want: "Git status (pkg)",
		},
		{
			name: "git_status default",
			call: makeTestCall("git_status", map[string]any{}),
			want: "Check git status",
		},
		{
			name: "get_todo",
			call: makeTestCall("get_todo", map[string]any{}),
			want: "Check task list",
		},
		{
			name: "update_todo with ops",
			call: makeTestCall("update_todo", map[string]any{"operations": []any{"op1", "op2"}}),
			want: "Update tasks (2 changes)",
		},
		{
			name: "activate_skill with name",
			call: makeTestCall("activate_skill", map[string]any{"name": "tester"}),
			want: "Activate skill tester",
		},
		{
			name: "delegate_task with profile and task",
			call: makeTestCall("delegate_task", map[string]any{"profile": "researcher", "task": "run analysis"}),
			want: "Delegate [researcher]: run analysis",
		},
		{
			name: "wait_agent with id",
			call: makeTestCall("wait_agent", map[string]any{"agent_id": "agent-101"}),
			want: "Wait for agent agent-101",
		},
		{
			name: "get_agent with id",
			call: makeTestCall("get_agent", map[string]any{"agent_id": "agent-101"}),
			want: "Get agent status agent-101",
		},
		{
			name: "list_agents",
			call: makeTestCall("list_agents", map[string]any{}),
			want: "List subagents",
		},
		{
			name: "cancel_agent with id",
			call: makeTestCall("cancel_agent", map[string]any{"agent_id": "agent-101"}),
			want: "Cancel agent agent-101",
		},
		{
			name: "checkpoint_restore with id",
			call: makeTestCall("checkpoint_restore", map[string]any{"checkpoint_id": "cp-1"}),
			want: "Restore checkpoint cp-1",
		},
		{
			name: "custom tool",
			call: makeTestCall("custom_tool", map[string]any{}),
			want: "custom_tool",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.call.Title()
			if got != tt.want {
				t.Errorf("Title() = %q, want %q", got, tt.want)
			}
			if !utf8.ValidString(got) {
				t.Errorf("Title() invalid UTF-8: %q", got)
			}
		})
	}
}

func TestCallTarget(t *testing.T) {
	tests := []struct {
		name string
		call Call
		want string
	}{
		{
			name: "web_fetch url",
			call: makeTestCall("web_fetch", map[string]any{"url": "https://example.com"}),
			want: "https://example.com",
		},
		{
			name: "web_search query",
			call: makeTestCall("web_search", map[string]any{"query": "golang"}),
			want: `"golang"`,
		},
		{
			name: "read_file path",
			call: makeTestCall("read_file", map[string]any{"path": "main.go"}),
			want: "main.go",
		},
		{
			name: "write_file file_path (canonical)",
			call: makeTestCall("write_file", map[string]any{"file_path": "out.txt"}),
			want: "out.txt",
		},
		{
			name: "write_file path (fallback)",
			call: makeTestCall("write_file", map[string]any{"path": "out.txt"}),
			want: "out.txt",
		},
		{
			name: "apply_patch patch target",
			call: makeTestCall("apply_patch", map[string]any{
				"patch": "*** Begin Patch\n*** Update File: lib.go\n@@ -1 +1 @@\n*** End Patch",
			}),
			want: "lib.go",
		},
		{
			name: "list_dir path",
			call: makeTestCall("list_dir", map[string]any{"path": "docs"}),
			want: "docs",
		},
		{
			name: "list_dir default",
			call: makeTestCall("list_dir", map[string]any{}),
			want: ".",
		},
		{
			name: "grep pattern and path",
			call: makeTestCall("grep", map[string]any{"pattern": "test", "path": "pkg"}),
			want: `"test" in pkg`,
		},
		{
			name: "bash command",
			call: makeTestCall("bash", map[string]any{"command": "echo hi"}),
			want: "echo hi",
		},
		{
			name: "delegate_task profile and task",
			call: makeTestCall("delegate_task", map[string]any{"profile": "audit", "task": "check security"}),
			want: "[audit] check security",
		},
		{
			name: "checkpoint_restore id",
			call: makeTestCall("checkpoint_restore", map[string]any{"checkpoint_id": "chk-1"}),
			want: "chk-1",
		},
		{
			name: "mcp heuristic fallback",
			call: makeTestCall("mcp_server_lookup", map[string]any{"query": "users"}),
			want: "users",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.call.Target()
			if got != tt.want {
				t.Errorf("Target() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestCallAffectedPaths(t *testing.T) {
	tests := []struct {
		name string
		call Call
		want []string
	}{
		{
			name: "read_file with path",
			call: makeTestCall("read_file", map[string]any{"path": "a.txt"}),
			want: []string{"a.txt"},
		},
		{
			name: "write_file with file_path",
			call: makeTestCall("write_file", map[string]any{"file_path": "b.txt"}),
			want: []string{"b.txt"},
		},
		{
			name: "search_replace with file_path",
			call: makeTestCall("search_replace", map[string]any{"file_path": "c.txt"}),
			want: []string{"c.txt"},
		},
		{
			name: "apply_patch multi-file patch",
			call: makeTestCall("apply_patch", map[string]any{
				"patch": strings.Join([]string{
					"*** Begin Patch",
					"*** Add File: f1.go",
					"+line",
					"*** Update File: f2.go",
					"*** Move to: f2_new.go",
					"*** End Patch",
				}, "\n"),
			}),
			want: []string{"f1.go", "f2.go", "f2_new.go"},
		},
		{
			name: "bash returns nil",
			call: makeTestCall("bash", map[string]any{"command": "ls"}),
			want: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.call.AffectedPaths()
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("AffectedPaths() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestKindForName(t *testing.T) {
	tests := []struct {
		name string
		want Kind
	}{
		{"read_file", KindRead},
		{"list_dir", KindRead},
		{"git_status", KindRead},
		{"write_file", KindEdit},
		{"search_replace", KindEdit},
		{"apply_patch", KindEdit},
		{"checkpoint_restore", KindEdit},
		{"grep", KindGrep},
		{"web_search", KindWebSearch},
		{"bash", KindBash},
		{"web_fetch", KindWebFetch},
		{"get_todo", KindTask},
		{"update_todo", KindTask},
		{"delegate_task", KindAgent},
		{"wait_agent", KindAgent},
		{"get_agent", KindAgent},
		{"list_agents", KindAgent},
		{"cancel_agent", KindAgent},
		{"other_tool", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := KindForName(tt.name)
			if got != tt.want {
				t.Errorf("KindForName(%q) = %q, want %q", tt.name, got, tt.want)
			}
		})
	}
}

func TestTruncateRunes(t *testing.T) {
	tests := []struct {
		input    string
		maxRunes int
		want     string
	}{
		{"hello", 10, "hello"},
		{"hello world", 6, "hello…"},
		{"สวัสดีชาวโลก", 7, "สวัสดี…"},
		{"", 5, ""},
		{"test", 0, ""},
		{"test", 1, "t"},
	}

	for _, tt := range tests {
		got := TruncateRunes(tt.input, tt.maxRunes)
		if got != tt.want {
			t.Errorf("TruncateRunes(%q, %d) = %q, want %q", tt.input, tt.maxRunes, got, tt.want)
		}
		if !utf8.ValidString(got) {
			t.Errorf("TruncateRunes(%q, %d) invalid UTF-8: %q", tt.input, tt.maxRunes, got)
		}
	}
}

func TestDisplayName(t *testing.T) {
	tests := []struct {
		name string
		want string
	}{
		{"read_file", "Read"},
		{"list_dir", "List"},
		{"write_file", "Write"},
		{"search_replace", "Edit"},
		{"apply_patch", "Patch"},
		{"grep", "Search"},
		{"bash", "Run"},
		{"web_fetch", "Fetch"},
		{"web_search", "Search web"},
		{"git_status", "Git status"},
		{"get_todo", "Tasks"},
		{"update_todo", "Update tasks"},
		{"activate_skill", "Skill"},
		{"delegate_task", "Delegate"},
		{"wait_agent", "Wait agent"},
		{"get_agent", "Agent status"},
		{"list_agents", "Subagents"},
		{"cancel_agent", "Cancel agent"},
		{"checkpoint_restore", "Restore"},
		{"mcp.filesystem.read", "read"},
		{"custom_tool", "custom_tool"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := DisplayName(tt.name)
			if got != tt.want {
				t.Errorf("DisplayName(%q) = %q, want %q", tt.name, got, tt.want)
			}
			call := Call{Name: tt.name}
			if call.DisplayName() != tt.want {
				t.Errorf("Call.DisplayName() = %q, want %q", call.DisplayName(), tt.want)
			}
			def := Definition{Name: tt.name}
			if def.DisplayName() != tt.want {
				t.Errorf("Definition.DisplayName() = %q, want %q", def.DisplayName(), tt.want)
			}
		})
	}
}

