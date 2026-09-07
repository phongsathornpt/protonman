package acp

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/projectTHORN/proton/internal/core/tool"
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
		{"read_file", ToolKindRead},
		{"list_dir", ToolKindRead},
		{"git_status", ToolKindRead},
		{"get_todo", ToolKindRead},
		{"write_file", ToolKindEdit},
		{"search_replace", ToolKindEdit},
		{"apply_patch", ToolKindEdit},
		{"checkpoint_restore", ToolKindEdit},
		{"update_todo", ToolKindEdit},
		{"grep", ToolKindSearch},
		{"web_search", ToolKindSearch},
		{"bash", ToolKindExecute},
		{"delegate_task", ToolKindExecute},
		{"wait_agent", ToolKindExecute},
		{"get_agent", ToolKindExecute},
		{"list_agents", ToolKindExecute},
		{"cancel_agent", ToolKindExecute},
		{"web_fetch", ToolKindFetch},
		{"activate_skill", ToolKindRead},
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

func TestTitleForToolCall(t *testing.T) {
	tests := []struct {
		name string
		call tool.Call
		want string
	}{
		// read_file
		{
			name: "read_file with path",
			call: makeCall("read_file", map[string]any{"path": "src/main.go"}),
			want: "Read src/main.go",
		},
		{
			name: "read_file with file_path alias",
			call: makeCall("read_file", map[string]any{"file_path": "src/main.go"}),
			want: "Read src/main.go",
		},
		{
			name: "read_file empty",
			call: makeCall("read_file", map[string]any{}),
			want: "Read file",
		},

		// write_file (schema uses file_path)
		{
			name: "write_file with file_path",
			call: makeCall("write_file", map[string]any{"file_path": "config.json"}),
			want: "Write config.json",
		},
		{
			name: "write_file with path fallback",
			call: makeCall("write_file", map[string]any{"path": "config.json"}),
			want: "Write config.json",
		},
		{
			name: "write_file empty",
			call: makeCall("write_file", map[string]any{}),
			want: "Write file",
		},

		// search_replace (schema uses file_path)
		{
			name: "search_replace with file_path",
			call: makeCall("search_replace", map[string]any{"file_path": "server.go"}),
			want: "Edit server.go",
		},
		{
			name: "search_replace with path fallback",
			call: makeCall("search_replace", map[string]any{"path": "server.go"}),
			want: "Edit server.go",
		},
		{
			name: "search_replace empty",
			call: makeCall("search_replace", map[string]any{}),
			want: "Search and replace",
		},

		// apply_patch (schema uses patch)
		{
			name: "apply_patch with patch single file",
			call: makeCall("apply_patch", map[string]any{
				"patch": "*** Begin Patch\n*** Update File: internal/acp/mapping.go\n@@ -1,2 +1,2 @@\n*** End Patch",
			}),
			want: "Patch internal/acp/mapping.go",
		},
		{
			name: "apply_patch with patch multi file",
			call: makeCall("apply_patch", map[string]any{
				"patch": "*** Begin Patch\n*** Update File: file1.go\n*** Update File: file2.go\n*** Update File: file3.go\n*** End Patch",
			}),
			want: "Patch file1.go (+2 files)",
		},
		{
			name: "apply_patch with path argument fallback",
			call: makeCall("apply_patch", map[string]any{"path": "file1.go"}),
			want: "Patch file1.go",
		},
		{
			name: "apply_patch empty",
			call: makeCall("apply_patch", map[string]any{}),
			want: "Apply patch",
		},

		// list_dir (schema uses path, dir_path, directory)
		{
			name: "list_dir with path",
			call: makeCall("list_dir", map[string]any{"path": "pkg/api"}),
			want: "List pkg/api",
		},
		{
			name: "list_dir with dir_path",
			call: makeCall("list_dir", map[string]any{"dir_path": "pkg/api"}),
			want: "List pkg/api",
		},
		{
			name: "list_dir with directory",
			call: makeCall("list_dir", map[string]any{"directory": "pkg/api"}),
			want: "List pkg/api",
		},
		{
			name: "list_dir empty",
			call: makeCall("list_dir", map[string]any{}),
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

		// web_fetch (clamped URL)
		{
			name: "web_fetch short url",
			call: makeCall("web_fetch", map[string]any{"url": "https://example.com"}),
			want: "Fetch https://example.com",
		},
		{
			name: "web_fetch long url",
			call: makeCall("web_fetch", map[string]any{"url": "https://github.com/projectTHORN/proton/blob/main/internal/acp/mapping.go#L1-L100"}),
			want: "Fetch https://github.com/projectTHORN/proton/blob/…",
		},
		{
			name: "web_fetch empty",
			call: makeCall("web_fetch", map[string]any{}),
			want: "Fetch URL",
		},

		// web_search
		{
			name: "web_search with query",
			call: makeCall("web_search", map[string]any{"query": "agent client protocol specification and guidelines"}),
			want: "Search web: agent client protocol specificatio…",
		},
		{
			name: "web_search empty",
			call: makeCall("web_search", map[string]any{}),
			want: "Search web",
		},

		// git_status
		{
			name: "git_status default",
			call: makeCall("git_status", map[string]any{}),
			want: "Check git status",
		},
		{
			name: "git_status with path",
			call: makeCall("git_status", map[string]any{"path": "internal/acp"}),
			want: "Git status (internal/acp)",
		},

		// todo tools
		{
			name: "get_todo",
			call: makeCall("get_todo", map[string]any{}),
			want: "Check task list",
		},
		{
			name: "update_todo with operations",
			call: makeCall("update_todo", map[string]any{"operations": []any{map[string]any{"op": "add"}, map[string]any{"op": "remove"}}}),
			want: "Update tasks (2 changes)",
		},
		{
			name: "update_todo without operations",
			call: makeCall("update_todo", map[string]any{}),
			want: "Update tasks",
		},

		// skills
		{
			name: "activate_skill with name",
			call: makeCall("activate_skill", map[string]any{"name": "git-commit"}),
			want: "Activate skill git-commit",
		},
		{
			name: "activate_skill empty",
			call: makeCall("activate_skill", map[string]any{}),
			want: "Activate skill",
		},

		// delegate_task
		{
			name: "delegate_task with task and profile",
			call: makeCall("delegate_task", map[string]any{"profile": "researcher", "task": "Investigate unit tests"}),
			want: "Delegate [researcher]: Investigate unit tests",
		},
		{
			name: "delegate_task with long unicode task",
			call: makeCall("delegate_task", map[string]any{"task": "ตรวจสอบระบบและปรับปรุงการทำงานของโปรโตคอลให้สมบูรณ์"}),
			want: "Delegate: ตรวจสอบระบบและปรับปรุงการทำงา…",
		},
		{
			name: "delegate_task empty",
			call: makeCall("delegate_task", map[string]any{}),
			want: "Delegate subtask",
		},

		// subagents
		{
			name: "wait_agent with id",
			call: makeCall("wait_agent", map[string]any{"agent_id": "agent-42"}),
			want: "Wait for agent agent-42",
		},
		{
			name: "get_agent with id",
			call: makeCall("get_agent", map[string]any{"agent_id": "agent-42"}),
			want: "Get agent status agent-42",
		},
		{
			name: "list_agents",
			call: makeCall("list_agents", map[string]any{}),
			want: "List subagents",
		},
		{
			name: "cancel_agent with id",
			call: makeCall("cancel_agent", map[string]any{"agent_id": "agent-42"}),
			want: "Cancel agent agent-42",
		},

		// checkpoint_restore
		{
			name: "checkpoint_restore with id",
			call: makeCall("checkpoint_restore", map[string]any{"checkpoint_id": "chk-99"}),
			want: "Restore checkpoint chk-99",
		},
		{
			name: "checkpoint_restore empty",
			call: makeCall("checkpoint_restore", map[string]any{}),
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
			name: "read_file with path",
			call: makeCall("read_file", map[string]any{"path": "main.go"}),
			want: []ToolCallLocation{{Path: "main.go"}},
		},
		{
			name: "read_file with file_path",
			call: makeCall("read_file", map[string]any{"file_path": "main.go"}),
			want: []ToolCallLocation{{Path: "main.go"}},
		},
		{
			name: "write_file with file_path (schema key)",
			call: makeCall("write_file", map[string]any{"file_path": "lib.go"}),
			want: []ToolCallLocation{{Path: "lib.go"}},
		},
		{
			name: "search_replace with file_path (schema key)",
			call: makeCall("search_replace", map[string]any{"file_path": "cmd/app.go"}),
			want: []ToolCallLocation{{Path: "cmd/app.go"}},
		},
		{
			name: "apply_patch with patch single file",
			call: makeCall("apply_patch", map[string]any{
				"patch": "*** Begin Patch\n*** Update File: internal/acp/mapping.go\n@@ -1 +1 @@\n*** End Patch",
			}),
			want: []ToolCallLocation{{Path: "internal/acp/mapping.go"}},
		},
		{
			name: "apply_patch with patch multi file",
			call: makeCall("apply_patch", map[string]any{
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
			name: "apply_patch with unified diff format",
			call: makeCall("apply_patch", map[string]any{
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
			name: "apply_patch fallback path",
			call: makeCall("apply_patch", map[string]any{"path": "fallback.go"}),
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
