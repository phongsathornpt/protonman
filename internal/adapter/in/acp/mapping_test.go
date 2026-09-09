package acp

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/phongsathornpt/protonman/internal/core/tool"
)

func makeCall(name string, args map[string]any) tool.Call {
	data, _ := json.Marshal(args)
	return tool.Call{
		ID:        "call-123",
		Name:      name,
		Arguments: data,
	}
}

func TestToolKindForName(t *testing.T) {
	tests := []struct {
		name string
		want ToolKind
	}{
		{tool.NameRead, ToolKindRead},
		{tool.NameLS, ToolKindRead},
		{tool.NameGit, ToolKindExecute},
		{tool.NameEdit, ToolKindEdit},
		{"grep", ToolKindSearch},
		{tool.NameWeb, ToolKindFetch},
		{tool.NameBash, ToolKindExecute},
		{tool.NameSubagent, ToolKindExecute},
		{tool.NameSkill, ToolKindRead},
		{tool.NameTodo, ToolKindEdit},
		{"unknown_tool", ToolKindOther},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ToolKindForName(tt.name)
			if got != tt.want {
				t.Errorf("ToolKindForName(%q) = %q, want %q", tt.name, got, tt.want)
			}
		})
	}
}

func TestToolKindForCallUsesCanonicalAction(t *testing.T) {
	tests := []struct {
		name string
		call tool.Call
		want ToolKind
	}{
		{"web search", makeCall(tool.NameWeb, map[string]any{"action": tool.ActionSearch}), ToolKindSearch},
		{"web fetch", makeCall(tool.NameWeb, map[string]any{"action": tool.ActionFetch}), ToolKindFetch},
		{"todo get", makeCall(tool.NameTodo, map[string]any{"action": tool.ActionGet}), ToolKindRead},
		{"todo update", makeCall(tool.NameTodo, map[string]any{"action": tool.ActionUpdate}), ToolKindEdit},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ToolKindForCall(tt.call); got != tt.want {
				t.Fatalf("ToolKindForCall() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestTitleForToolCall(t *testing.T) {
	tests := []struct {
		name string
		call tool.Call
		want string
	}{
		// read
		{
			name: "read with path",
			call: makeCall("read", map[string]any{"path": "src/main.go"}),
			want: "Read src/main.go",
		},
		{
			name: "read with file_path alias",
			call: makeCall("read", map[string]any{"file_path": "src/main.go"}),
			want: "Read src/main.go",
		},
		{
			name: "read empty",
			call: makeCall("read", map[string]any{}),
			want: "Read file",
		},

		// edit write (schema uses file_path)
		{
			name: "edit write with file_path",
			call: makeCall("edit", map[string]any{"action": "write", "file_path": "config.json"}),
			want: "Write config.json",
		},
		{
			name: "edit write with path fallback",
			call: makeCall("edit", map[string]any{"action": "write", "path": "config.json"}),
			want: "Write config.json",
		},
		{
			name: "edit write empty",
			call: makeCall("edit", map[string]any{"action": "write"}),
			want: "Write file",
		},

		// edit replace (schema uses file_path)
		{
			name: "edit replace with file_path",
			call: makeCall("edit", map[string]any{"action": "replace", "file_path": "server.go"}),
			want: "Edit server.go",
		},
		{
			name: "edit replace with path fallback",
			call: makeCall("edit", map[string]any{"action": "replace", "path": "server.go"}),
			want: "Edit server.go",
		},
		{
			name: "edit replace empty",
			call: makeCall("edit", map[string]any{"action": "replace"}),
			want: "Search and replace",
		},

		// edit patch (schema uses patch)
		{
			name: "edit patch with patch single file",
			call: makeCall("edit", map[string]any{"action": "patch",
				"patch": "*** Begin Patch\n*** Update File: internal/acp/mapping.go\n@@ -1,2 +1,2 @@\n*** End Patch",
			}),
			want: "Patch internal/acp/mapping.go",
		},
		{
			name: "edit patch with patch multi file",
			call: makeCall("edit", map[string]any{"action": "patch",
				"patch": "*** Begin Patch\n*** Update File: file1.go\n*** Update File: file2.go\n*** Update File: file3.go\n*** End Patch",
			}),
			want: "Patch file1.go (+2 files)",
		},
		{
			name: "edit patch with path argument fallback",
			call: makeCall("edit", map[string]any{"action": "patch", "path": "file1.go"}),
			want: "Patch file1.go",
		},
		{
			name: "edit patch empty",
			call: makeCall("edit", map[string]any{"action": "patch"}),
			want: "Apply patch",
		},

		// ls (schema uses path, dir_path, directory)
		{
			name: "ls with path",
			call: makeCall("ls", map[string]any{"path": "pkg/api"}),
			want: "List pkg/api",
		},
		{
			name: "ls with dir_path",
			call: makeCall("ls", map[string]any{"dir_path": "pkg/api"}),
			want: "List pkg/api",
		},
		{
			name: "ls with directory",
			call: makeCall("ls", map[string]any{"directory": "pkg/api"}),
			want: "List pkg/api",
		},
		{
			name: "ls empty",
			call: makeCall("ls", map[string]any{}),
			want: "List directory",
		},

		// grep (schema uses pattern)
		{
			name: "grep with pattern only",
			call: makeCall("grep", map[string]any{"pattern": "ToolKind"}),
			want: `Search "ToolKind"`,
		},
		{
			name: "grep with pattern and path",
			call: makeCall("grep", map[string]any{"pattern": "ToolKind", "path": "internal/acp"}),
			want: `Search "ToolKind" in internal/acp`,
		},
		{
			name: "grep with query fallback",
			call: makeCall("grep", map[string]any{"query": "TitleForToolCall"}),
			want: `Search "TitleForToolCall"`,
		},
		{
			name: "grep empty",
			call: makeCall("grep", map[string]any{}),
			want: "Search workspace",
		},

		// bash (rune-safe truncation)
		{
			name: "bash short command",
			call: makeCall("bash", map[string]any{"command": "go test ./..."}),
			want: "Run: go test ./...",
		},
		{
			name: "bash long command",
			call: makeCall("bash", map[string]any{"command": "git commit -m 'feat(acp): implement safe unicode rune truncation and parameter mapping'"}),
			want: "Run: git commit -m 'feat(acp): implement saf…",
		},
		{
			name: "bash unicode Thai runes",
			call: makeCall("bash", map[string]any{"command": "echo 'สวัสดีชาวโลกทุกคนที่กำลังทดสอบระบบนี้อยู่นะครับ'"}),
			want: "Run: echo 'สวัสดีชาวโลกทุกคนที่กำลังทดสอบระบ…",
		},
		{
			name: "bash empty",
			call: makeCall("bash", map[string]any{}),
			want: "Run shell command",
		},

		// web fetch (clamped URL)
		{
			name: "web fetch short url",
			call: makeCall("web", map[string]any{"action": "fetch", "url": "https://example.com"}),
			want: "Fetch https://example.com",
		},
		{
			name: "web fetch long url",
			call: makeCall("web", map[string]any{"action": "fetch", "url": "https://github.com/phongsathornpt/protonman/blob/main/internal/acp/mapping.go#L1-L100"}),
			want: "Fetch https://github.com/phongsathornpt/protonman/…",
		},
		{
			name: "web fetch empty",
			call: makeCall("web", map[string]any{"action": "fetch"}),
			want: "Fetch URL",
		},

		// web search
		{
			name: "web search with query",
			call: makeCall("web", map[string]any{"action": "search", "query": "agent client protocol specification and guidelines"}),
			want: "Search web: agent client protocol specificatio…",
		},
		{
			name: "web search empty",
			call: makeCall("web", map[string]any{"action": "search"}),
			want: "Search web",
		},

		// git status
		{
			name: "git status default",
			call: makeCall("git", map[string]any{"action": "status"}),
			want: "Check git status",
		},
		{
			name: "git status with path",
			call: makeCall("git", map[string]any{"action": "status", "path": "internal/acp"}),
			want: "Git status (internal/acp)",
		},

		// todo tools
		{
			name: "todo get",
			call: makeCall("todo", map[string]any{"action": "get"}),
			want: "Check task list",
		},
		{
			name: "todo update with operations",
			call: makeCall("todo", map[string]any{"action": "update", "operations": []any{map[string]any{"op": "add"}, map[string]any{"op": "remove"}}}),
			want: "Update tasks (2 changes)",
		},
		{
			name: "todo update without operations",
			call: makeCall("todo", map[string]any{"action": "update"}),
			want: "Update tasks",
		},

		// skills
		{
			name: "skill with name",
			call: makeCall("skill", map[string]any{"name": "git-commit"}),
			want: "Activate skill git-commit",
		},
		{
			name: "skill empty",
			call: makeCall("skill", map[string]any{}),
			want: "Activate skill",
		},

		// subagent spawn
		{
			name: "subagent spawn with task and profile",
			call: makeCall("subagent", map[string]any{"action": "spawn", "profile": "researcher", "task": "Investigate unit tests"}),
			want: "Delegate [researcher]: Investigate unit tests",
		},
		{
			name: "subagent spawn with long unicode task",
			call: makeCall("subagent", map[string]any{"action": "spawn", "task": "ตรวจสอบระบบและปรับปรุงการทำงานของโปรโตคอลให้สมบูรณ์"}),
			want: "Delegate: ตรวจสอบระบบและปรับปรุงการทำงา…",
		},
		{
			name: "subagent spawn empty",
			call: makeCall("subagent", map[string]any{"action": "spawn"}),
			want: "Delegate subtask",
		},

		// subagents
		{
			name: "subagent wait",
			call: makeCall("subagent", map[string]any{"action": "wait", "timeout_seconds": 30}),
			want: "Wait for agent activity",
		},
		{
			name: "subagent get with id",
			call: makeCall("subagent", map[string]any{"action": "get", "agent_id": "agent-42"}),
			want: "Get agent status agent-42",
		},
		{
			name: "subagent list",
			call: makeCall("subagent", map[string]any{"action": "list"}),
			want: "List subagents",
		},
		{
			name: "subagent cancel with id",
			call: makeCall("subagent", map[string]any{"action": "cancel", "agent_id": "agent-42"}),
			want: "Cancel agent agent-42",
		},

		// edit restore
		{
			name: "edit restore with id",
			call: makeCall("edit", map[string]any{"action": "restore", "checkpoint_id": "chk-99"}),
			want: "Restore checkpoint chk-99",
		},
		{
			name: "edit restore empty",
			call: makeCall("edit", map[string]any{"action": "restore"}),
			want: "Restore checkpoint",
		},

		// unknown
		{
			name: "custom tool name",
			call: makeCall("custom_lint_tool", map[string]any{}),
			want: "custom_lint_tool",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := TitleForToolCall(tt.call)
			if got != tt.want {
				t.Errorf("TitleForToolCall() = %q, want %q", got, tt.want)
			}
			if !utf8.ValidString(got) {
				t.Errorf("TitleForToolCall() returned invalid UTF-8: %q", got)
			}
		})
	}
}

func TestLocationsForToolCall(t *testing.T) {
	tests := []struct {
		name string
		call tool.Call
		want []ToolCallLocation
	}{
		{
			name: "read with path",
			call: makeCall("read", map[string]any{"path": "main.go"}),
			want: []ToolCallLocation{{Path: "main.go"}},
		},
		{
			name: "read with file_path",
			call: makeCall("read", map[string]any{"file_path": "main.go"}),
			want: []ToolCallLocation{{Path: "main.go"}},
		},
		{
			name: "edit write with file_path (schema key)",
			call: makeCall("edit", map[string]any{"action": "write", "file_path": "lib.go"}),
			want: []ToolCallLocation{{Path: "lib.go"}},
		},
		{
			name: "edit replace with file_path (schema key)",
			call: makeCall("edit", map[string]any{"action": "replace", "file_path": "cmd/app.go"}),
			want: []ToolCallLocation{{Path: "cmd/app.go"}},
		},
		{
			name: "edit patch with patch single file",
			call: makeCall("edit", map[string]any{"action": "patch",
				"patch": "*** Begin Patch\n*** Update File: internal/acp/mapping.go\n@@ -1 +1 @@\n*** End Patch",
			}),
			want: []ToolCallLocation{{Path: "internal/acp/mapping.go"}},
		},
		{
			name: "edit patch with patch multi file",
			call: makeCall("edit", map[string]any{"action": "patch",
				"patch": strings.Join([]string{
					"*** Begin Patch",
					"*** Add File: pkg/a.go",
					"+package pkg",
					"*** Update File: pkg/b.go",
					"*** Move to: pkg/b_renamed.go",
					"*** Delete File: pkg/c.go",
					"*** End Patch",
				}, "\n"),
			}),
			want: []ToolCallLocation{
				{Path: "pkg/a.go"},
				{Path: "pkg/b.go"},
				{Path: "pkg/b_renamed.go"},
				{Path: "pkg/c.go"},
			},
		},
		{
			name: "edit patch with unified diff format",
			call: makeCall("edit", map[string]any{"action": "patch",
				"patch": strings.Join([]string{
					"--- a/old.go",
					"+++ b/new.go",
					"@@ -1 +1 @@",
				}, "\n"),
			}),
			want: []ToolCallLocation{
				{Path: "old.go"},
				{Path: "new.go"},
			},
		},
		{
			name: "edit patch fallback path",
			call: makeCall("edit", map[string]any{"action": "patch", "path": "fallback.go"}),
			want: []ToolCallLocation{{Path: "fallback.go"}},
		},
		{
			name: "non-file tools return nil",
			call: makeCall("bash", map[string]any{"command": "echo hi"}),
			want: nil,
		},
		{
			name: "grep returns nil",
			call: makeCall("grep", map[string]any{"pattern": "foo"}),
			want: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := LocationsForToolCall(tt.call)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("LocationsForToolCall() = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestTruncateRunes(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		maxRunes int
		want     string
	}{
		{
			name:     "within limit",
			input:    "hello world",
			maxRunes: 20,
			want:     "hello world",
		},
		{
			name:     "exact limit",
			input:    "12345",
			maxRunes: 5,
			want:     "12345",
		},
		{
			name:     "exceeds limit ASCII",
			input:    "1234567890",
			maxRunes: 6,
			want:     "12345…",
		},
		{
			name:     "unicode Thai string within limit",
			input:    "สวัสดี",
			maxRunes: 10,
			want:     "สวัสดี",
		},
		{
			name:     "unicode Thai string exceeding limit",
			input:    "สวัสดีชาวโลก", // 12 runes
			maxRunes: 7,
			want:     "สวัสดี…",
		},
		{
			name:     "limit of 1",
			input:    "abc",
			maxRunes: 1,
			want:     "a",
		},
		{
			name:     "empty string",
			input:    "",
			maxRunes: 10,
			want:     "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tool.TruncateRunes(tt.input, tt.maxRunes)
			if got != tt.want {
				t.Errorf("truncateRunes(%q, %d) = %q, want %q", tt.input, tt.maxRunes, got, tt.want)
			}
			if !utf8.ValidString(got) {
				t.Errorf("truncateRunes() produced invalid UTF-8: %q", got)
			}
			runes := []rune(got)
			if len(runes) > tt.maxRunes {
				t.Errorf("truncateRunes() length in runes = %d, exceeds max %d", len(runes), tt.maxRunes)
			}
		})
	}
}
