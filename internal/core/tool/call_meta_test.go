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
			name: "read with path",
			call: makeTestCall("read", map[string]any{"path": "main.go"}),
			want: "Read main.go",
		},
		{
			name: "read with file_path alias",
			call: makeTestCall("read", map[string]any{"file_path": "main.go"}),
			want: "Read main.go",
		},
		{
			name: "read empty",
			call: makeTestCall("read", map[string]any{}),
			want: "Read file",
		},
		{
			name: "edit write with file_path",
			call: makeTestCall("edit", map[string]any{"action": "write", "file_path": "server.go"}),
			want: "Write server.go",
		},
		{
			name: "edit write with path fallback",
			call: makeTestCall("edit", map[string]any{"action": "write", "path": "server.go"}),
			want: "Write server.go",
		},
		{
			name: "edit replace with file_path",
			call: makeTestCall("edit", map[string]any{"action": "replace", "file_path": "handler.go"}),
			want: "Edit handler.go",
		},
		{
			name: "edit patch with patch single file",
			call: makeTestCall("edit", map[string]any{"action": "patch",
				"patch": "*** Begin Patch\n*** Update File: config.yaml\n@@ -1 +1 @@\n*** End Patch",
			}),
			want: "Patch config.yaml",
		},
		{
			name: "edit patch with patch multi file",
			call: makeTestCall("edit", map[string]any{"action": "patch",
				"patch": "*** Begin Patch\n*** Update File: a.go\n*** Update File: b.go\n*** End Patch",
			}),
			want: "Patch a.go (+1 files)",
		},
		{
			name: "ls with dir_path",
			call: makeTestCall("ls", map[string]any{"dir_path": "cmd"}),
			want: "List cmd",
		},
		{
			name: "grep with pattern and path",
			call: makeTestCall("grep", map[string]any{"pattern": "Protonman", "path": "internal"}),
			want: `Search "Protonman" in internal`,
		},
		{
			name: "grep with query fallback",
			call: makeTestCall("grep", map[string]any{"query": "Protonman"}),
			want: `Search "Protonman"`,
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
			name: "web fetch url",
			call: makeTestCall("web", map[string]any{"action": "fetch", "url": "https://protonman.dev"}),
			want: "Fetch https://protonman.dev",
		},
		{
			name: "web search query",
			call: makeTestCall("web", map[string]any{"action": "search", "query": "agent client protocol specification and guidelines"}),
			want: "Search web: agent client protocol specificatio…",
		},
		{
			name: "git status with path",
			call: makeTestCall("git", map[string]any{"action": "status", "path": "pkg"}),
			want: "Git status (pkg)",
		},
		{
			name: "git status default",
			call: makeTestCall("git", map[string]any{"action": "status"}),
			want: "Check git status",
		},
		{
			name: "todo get",
			call: makeTestCall("todo", map[string]any{"action": "get"}),
			want: "Check task list",
		},
		{
			name: "todo update with ops",
			call: makeTestCall("todo", map[string]any{"action": "update", "operations": []any{"op1", "op2"}}),
			want: "Update tasks (2 changes)",
		},
		{
			name: "skill with name",
			call: makeTestCall("skill", map[string]any{"name": "tester"}),
			want: "Activate skill tester",
		},
		{
			name: "subagent spawn with profile and task",
			call: makeTestCall("subagent", map[string]any{"action": "spawn", "profile": "researcher", "task": "run analysis"}),
			want: "Delegate [researcher]: run analysis",
		},
		{
			name: "subagent wait",
			call: makeTestCall("subagent", map[string]any{"action": "wait", "timeout_seconds": 30}),
			want: "Wait for agent activity",
		},
		{
			name: "subagent get with id",
			call: makeTestCall("subagent", map[string]any{"action": "get", "agent_id": "agent-101"}),
			want: "Get agent status agent-101",
		},
		{
			name: "subagent list",
			call: makeTestCall("subagent", map[string]any{"action": "list"}),
			want: "List subagents",
		},
		{
			name: "subagent cancel with id",
			call: makeTestCall("subagent", map[string]any{"action": "cancel", "agent_id": "agent-101"}),
			want: "Cancel agent agent-101",
		},
		{
			name: "edit restore with id",
			call: makeTestCall("edit", map[string]any{"action": "restore", "checkpoint_id": "cp-1"}),
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
			name: "web fetch url",
			call: makeTestCall("web", map[string]any{"action": "fetch", "url": "https://example.com"}),
			want: "https://example.com",
		},
		{
			name: "web search query",
			call: makeTestCall("web", map[string]any{"action": "search", "query": "golang"}),
			want: `"golang"`,
		},
		{
			name: "read path",
			call: makeTestCall("read", map[string]any{"path": "main.go"}),
			want: "main.go",
		},
		{
			name: "edit write file_path (canonical)",
			call: makeTestCall("edit", map[string]any{"action": "write", "file_path": "out.txt"}),
			want: "out.txt",
		},
		{
			name: "edit write path (fallback)",
			call: makeTestCall("edit", map[string]any{"action": "write", "path": "out.txt"}),
			want: "out.txt",
		},
		{
			name: "edit patch patch target",
			call: makeTestCall("edit", map[string]any{"action": "patch",
				"patch": "*** Begin Patch\n*** Update File: lib.go\n@@ -1 +1 @@\n*** End Patch",
			}),
			want: "lib.go",
		},
		{
			name: "ls path",
			call: makeTestCall("ls", map[string]any{"path": "docs"}),
			want: "docs",
		},
		{
			name: "ls default",
			call: makeTestCall("ls", map[string]any{}),
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
			name: "subagent spawn profile and task",
			call: makeTestCall("subagent", map[string]any{"action": "spawn", "profile": "audit", "task": "check security"}),
			want: "[audit] check security",
		},
		{
			name: "edit restore id",
			call: makeTestCall("edit", map[string]any{"action": "restore", "checkpoint_id": "chk-1"}),
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
			name: "read with path",
			call: makeTestCall("read", map[string]any{"path": "a.txt"}),
			want: []string{"a.txt"},
		},
		{
			name: "edit write with file_path",
			call: makeTestCall("edit", map[string]any{"action": "write", "file_path": "b.txt"}),
			want: []string{"b.txt"},
		},
		{
			name: "edit replace with file_path",
			call: makeTestCall("edit", map[string]any{"action": "replace", "file_path": "c.txt"}),
			want: []string{"c.txt"},
		},
		{
			name: "edit patch multi-file patch",
			call: makeTestCall("edit", map[string]any{"action": "patch",
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
		{"read", KindRead},
		{"ls", KindRead},
		{"git", KindGit},
		{"edit", KindEdit},
		{"edit", KindEdit},
		{"edit", KindEdit},
		{"edit", KindEdit},
		{"grep", KindGrep},
		{"web", KindWeb},
		{"bash", KindBash},
		{"web", KindWeb},
		{"todo", KindTask},
		{"todo", KindTask},
		{"subagent", KindAgent},
		{"subagent", KindAgent},
		{"subagent", KindAgent},
		{"subagent", KindAgent},
		{"subagent", KindAgent},
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
		{"read", "Read"},
		{"ls", "List"},
		{"edit", "Edit"},
		{"edit", "Edit"},
		{"edit", "Edit"},
		{"grep", "Search"},
		{"bash", "Run"},
		{"web", "Web"},
		{"web", "Web"},
		{"git", "Git"},
		{"todo", "Tasks"},
		{"todo", "Tasks"},
		{"skill", "Skill"},
		{"subagent", "Subagent"},
		{"subagent", "Subagent"},
		{"subagent", "Subagent"},
		{"subagent", "Subagent"},
		{"subagent", "Subagent"},
		{"edit", "Edit"},
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
