//go:build desktop || desktop_gio

package gioui

import (
	"image"
	"strings"
	"testing"
	"time"

	"gioui.org/io/input"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"

	desktopstate "github.com/phongsathornpt/protonman/internal/feature/desktop"
)

func TestParsePermissionDetails(t *testing.T) {
	tests := []struct {
		name         string
		toolCall     map[string]any
		wantTool     string
		wantCmd      string
		wantRisk     string
		wantJSONPart string
	}{
		{
			name: "bash command with destructive indicator",
			toolCall: map[string]any{
				"name": "bash",
				"rawInput": map[string]any{
					"command": "rm -rf ./build",
				},
			},
			wantTool:     "bash",
			wantCmd:      "rm -rf ./build",
			wantRisk:     "destructive",
			wantJSONPart: "rm -rf",
		},
		{
			name: "edit file path with arguments",
			toolCall: map[string]any{
				"kind": "edit",
				"arguments": map[string]any{
					"filePath": "/workspace/main.go",
				},
			},
			wantTool:     "edit",
			wantCmd:      "/workspace/main.go",
			wantRisk:     "",
			wantJSONPart: "main.go",
		},
		{
			name: "explicit elevated risk from runtime",
			toolCall: map[string]any{
				"name": "git",
				"risk": "elevated",
				"rawInput": map[string]any{
					"command": "git push origin main",
				},
			},
			wantTool:     "git",
			wantCmd:      "git push origin main",
			wantRisk:     "elevated",
			wantJSONPart: "origin main",
		},
		{
			name:     "empty toolCall",
			toolCall: nil,
			wantTool: "",
			wantCmd:  "",
			wantRisk: "",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			toolName, cmd, risk, rawJSON := parsePermissionDetails(tc.toolCall)
			if toolName != tc.wantTool {
				t.Errorf("toolName = %q, want %q", toolName, tc.wantTool)
			}
			if cmd != tc.wantCmd {
				t.Errorf("cmd = %q, want %q", cmd, tc.wantCmd)
			}
			if risk != tc.wantRisk {
				t.Errorf("risk = %q, want %q", risk, tc.wantRisk)
			}
			if tc.wantJSONPart != "" && !strings.Contains(rawJSON, tc.wantJSONPart) {
				t.Errorf("rawJSON %q missing part %q", rawJSON, tc.wantJSONPart)
			}
		})
	}
}

func TestPermissionPanelEnrichedRendering(t *testing.T) {
	view := newShell(newTheme("dark"))
	state := desktopstate.State{
		ActiveSessionID: "s1",
		PermissionInbox: []desktopstate.PermissionRequest{
			{
				RequestID: "p1",
				SessionID: "s1",
				Title:     "Execute shell script",
				ToolName:  "bash",
				Command:   "go test -v ./...",
				Risk:      "destructive",
				RawJSON:   `{"command": "go test -v ./..."}`,
				Options: []desktopstate.PermissionOption{
					{ID: "allow_once", Name: "Allow once"},
					{ID: "allow_session", Name: "Allow for session"},
					{ID: "reject_once", Name: "Deny"},
				},
			},
		},
	}

	var operations op.Ops
	var router input.Router
	gtx := layout.Context{
		Ops:         &operations,
		Constraints: layout.Exact(image.Point{X: 800, Y: 600}),
		Metric:      unit.Metric{},
		Now:         time.Now(),
		Source:      router.Source(),
	}

	dims := view.layoutPermissionPanel(gtx, state, state.PermissionInbox[0])
	if dims.Size.X <= 0 || dims.Size.Y <= 0 {
		t.Fatalf("layoutPermissionPanel dims = %v, want positive size", dims.Size)
	}
}

func TestQuestionPanelEnrichedRendering(t *testing.T) {
	view := newShell(newTheme("dark"))
	state := desktopstate.State{
		ActiveSessionID: "s1",
		QuestionInbox: []desktopstate.QuestionRequest{
			{
				RequestID: "q1",
				SessionID: "s1",
				Questions: []desktopstate.QuestionItemState{
					{
						Question:    "Which architecture layer should this adapter target?",
						Options:     []string{"internal/adapter/in", "internal/adapter/out", "internal/core"},
						Multiple:    false,
						Recommended: "internal/adapter/in",
					},
				},
			},
		},
	}

	var operations op.Ops
	var router input.Router
	gtx := layout.Context{
		Ops:         &operations,
		Constraints: layout.Exact(image.Point{X: 800, Y: 600}),
		Metric:      unit.Metric{},
		Now:         time.Now(),
		Source:      router.Source(),
	}

	dims := view.layoutQuestionPanel(gtx, state, state.QuestionInbox[0])
	if dims.Size.X <= 0 || dims.Size.Y <= 0 {
		t.Fatalf("layoutQuestionPanel dims = %v, want positive size", dims.Size)
	}
}

func TestBackgroundAttentionBanner(t *testing.T) {
	view := newShell(newTheme("dark"))
	var selectedSession string
	view.onSelectSession = func(s string) {
		selectedSession = s
	}

	state := desktopstate.State{
		ActiveSessionID: "session-active",
		Sessions: []desktopstate.SessionState{
			{ID: "session-active", Title: "Current Work"},
			{ID: "session-bg", Title: "Background Refactor"},
		},
		PermissionInbox: []desktopstate.PermissionRequest{
			{
				RequestID: "p-bg",
				SessionID: "session-bg",
				Title:     "Run migrations",
				Command:   "npm run migrate",
			},
		},
	}

	var operations op.Ops
	var router input.Router
	gtx := layout.Context{
		Ops:         &operations,
		Constraints: layout.Exact(image.Point{X: 800, Y: 600}),
		Metric:      unit.Metric{},
		Now:         time.Now(),
		Source:      router.Source(),
	}

	dims := view.layoutBackgroundAttentionBanner(gtx, state, "session-active")
	if dims.Size.Y <= 0 {
		t.Fatalf("layoutBackgroundAttentionBanner expected positive height, got %v", dims.Size)
	}

	// When current session is the waiting one, banner should not appear
	dimsCurrent := view.layoutBackgroundAttentionBanner(gtx, state, "session-bg")
	if dimsCurrent.Size.Y != 0 {
		t.Fatalf("layoutBackgroundAttentionBanner expected zero height for active session, got %v", dimsCurrent.Size)
	}
	_ = selectedSession
}

func TestPermissionAuditTimelineItemRendering(t *testing.T) {
	view := newShell(newTheme("dark"))
	item := desktopstate.TimelineItem{
		ID:     "perm-audit-1",
		Kind:   desktopstate.TimelinePermission,
		Title:  "Allowed once · go test -v ./...",
		Text:   "Decision: allow_once",
		Status: "Allowed once",
	}

	var operations op.Ops
	var router input.Router
	gtx := layout.Context{
		Ops:         &operations,
		Constraints: layout.Exact(image.Point{X: 800, Y: 600}),
		Metric:      unit.Metric{},
		Now:         time.Now(),
		Source:      router.Source(),
	}

	dims := view.layoutPermissionAuditItem(gtx, item)
	if dims.Size.X <= 0 || dims.Size.Y <= 0 {
		t.Fatalf("layoutPermissionAuditItem dims = %v, want positive size", dims.Size)
	}
}
