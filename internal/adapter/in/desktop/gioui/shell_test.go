//go:build desktop || desktop_gio

package gioui

import (
	"image"
	"testing"
	"time"

	"gioui.org/io/input"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"

	"github.com/phongsathornpt/protonman/internal/app"
	desktopstate "github.com/phongsathornpt/protonman/internal/feature/desktop"
)

func TestShellLaysOutResponsiveStates(t *testing.T) {
	view := newShell(newTheme("light"))
	if view.sidebarList.Axis != layout.Vertical || view.conversationList.Axis != layout.Vertical {
		t.Fatalf("lists must scroll vertically: sidebar=%v conversation=%v", view.sidebarList.Axis, view.conversationList.Axis)
	}
	if !view.conversationList.ScrollToEnd {
		t.Fatal("conversation list must follow new timeline items")
	}
	snapshots := []controllerSnapshot{
		{State: desktopstate.State{}, Connection: connectionConnecting, Status: "Starting Protonman…"},
		{
			State: desktopstate.State{
				ActiveSessionID: "session",
				ActiveProjectID: "workspace:/workspace/alpha",
				Projects: []desktopstate.ProjectState{{
					ID:   "workspace:/workspace/alpha",
					Name: "alpha",
				}},
				Sessions: []desktopstate.SessionState{{
					ID:            "session",
					ProjectID:     "workspace:/workspace/alpha",
					Title:         "Long session title that must remain bounded",
					Workspace:     "/workspace/alpha",
					WorkspaceKey:  "workspace:/workspace/alpha",
					WorkspaceName: "alpha",
					AgentID:       controllerAgentID,
					Status:        desktopstate.TaskIdle,
				}},
			},
			Connection: connectionConnected,
			Status:     "Connected · ACP v1",
		},
		{
			State: desktopstate.State{
				ActiveSessionID: "session",
				ActiveProjectID: "workspace:/workspace/alpha",
				Projects: []desktopstate.ProjectState{{
					ID:   "workspace:/workspace/alpha",
					Name: "alpha",
				}},
				Sessions: []desktopstate.SessionState{{
					ID:            "session",
					ProjectID:     "workspace:/workspace/alpha",
					Title:         "Conversation",
					Workspace:     "/workspace/alpha",
					WorkspaceKey:  "workspace:/workspace/alpha",
					WorkspaceName: "alpha",
					AgentID:       controllerAgentID,
					Status:        desktopstate.TaskWaitingPermission,
					Timeline: []desktopstate.TimelineItem{
						{ID: "user-1", Kind: desktopstate.TimelineUser, Text: "Review the change"},
						{ID: "assistant-1", Kind: desktopstate.TimelineAssistant, Text: "**Done**"},
						{ID: "tool-1", Kind: desktopstate.TimelineTool, Title: "Read file", Status: "completed", Text: "ok"},
					},
					Subagents: []desktopstate.SubagentState{{ID: "agent-1", Profile: "strength", Task: "Inspect tests", Status: "running"}},
				}},
				PermissionInbox: []desktopstate.PermissionRequest{{
					RequestID: "permission-1",
					SessionID: "session",
					Title:     "Run tests",
					Detail:    "Tool request:\nrun tests",
					Options: []desktopstate.PermissionOption{
						{ID: "allow", Name: "Allow once"},
						{ID: "reject", Name: "Reject"},
					},
				}},
			},
			Connection: connectionConnected,
			Status:     "Permission required",
		},
	}
	view.composer.SetText("follow-up")
	sizes := []image.Point{{X: 760, Y: 600}, {X: 1180, Y: 760}}
	for _, snapshot := range snapshots {
		for _, size := range sizes {
			var operations op.Ops
			var router input.Router
			gtx := layout.Context{
				Ops:         &operations,
				Constraints: layout.Exact(size),
				Metric:      unit.Metric{},
				Now:         time.Unix(1, 0),
				Source:      router.Source(),
			}
			dims := view.layout(gtx, snapshot)
			router.Frame(gtx.Ops)
			if dims.Size != size {
				t.Fatalf("layout size = %v, want %v", dims.Size, size)
			}
		}
	}
}

func TestShellLaysOutInspectorContentAtBothBreakpoints(t *testing.T) {
	view := newShell(newTheme("dark"))
	snapshot := controllerSnapshot{
		State: desktopstate.State{
			ActiveSessionID: "session",
			Sessions: []desktopstate.SessionState{{
				ID:     "session",
				Title:  "Inspector",
				Status: desktopstate.TaskIdle,
				Context: desktopstate.SessionContextState{
					Goal: "Ship the inspector slice",
					Todo: desktopstate.TodoState{Revision: 7, Items: []desktopstate.TodoItemState{{ID: "todo", Text: "Run focused tests", Status: "in_progress"}}},
					Memory: desktopstate.MemoryState{
						WorkspaceKey: "workspace-1",
						Workspace:    []desktopstate.MemoryEntryState{{ID: "m1", Kind: "repo_fact", Key: "test command", Value: "go test ./...", Confidence: .9, UsageCount: 2}},
						Global:       []desktopstate.MemoryEntryState{{ID: "m2", Kind: "preference", Key: "style", Value: "concise", Confidence: 1}},
					},
				},
				Runtime: desktopstate.RuntimeSettingsState{Provider: "openai", Model: "gpt", Reasoning: "high", LowConcurrency: "on"},
			}},
		},
		Connection: connectionConnected,
		Status:     "Connected",
	}
	view.inspectorOverride = true
	view.inspectorVisible = true
	for _, size := range []image.Point{{X: 760, Y: 600}, {X: 1180, Y: 760}} {
		var operations op.Ops
		var router input.Router
		gtx := layout.Context{
			Ops:         &operations,
			Constraints: layout.Exact(size),
			Metric:      unit.Metric{},
			Now:         time.Unix(1, 0),
			Source:      router.Source(),
		}
		dims := view.layout(gtx, snapshot)
		router.Frame(gtx.Ops)
		if dims.Size != size {
			t.Fatalf("layout size = %v, want %v", dims.Size, size)
		}
	}
}

func TestSyncAgentProfileEditorsPreservesUserDraft(t *testing.T) {
	view := newShell(newTheme("light"))
	profile := app.ACPAgentProfile{ID: "reviewer", DisplayName: "Reviewer", Command: "reviewer-acp", Args: []string{"--stdio"}, Env: []string{"TOKEN"}}
	snapshot := controllerSnapshot{AgentProfiles: []app.ACPAgentProfile{profile}}
	view.agentEditorVisible = true
	view.agentEditorOriginalID = profile.ID
	view.syncAgentProfileEditors(snapshot)
	if view.agentCommandEditor.Text() != profile.Command || view.agentEnvEditor.Text() != `["TOKEN"]` {
		t.Fatalf("editor was not populated: command=%q env=%q", view.agentCommandEditor.Text(), view.agentEnvEditor.Text())
	}
	view.agentCommandEditor.SetText("draft-command")
	view.syncAgentProfileEditors(snapshot)
	if view.agentCommandEditor.Text() != "draft-command" {
		t.Fatalf("user draft was overwritten: %q", view.agentCommandEditor.Text())
	}
}

func TestShellLaysOutExternalAgentAndProfileEditor(t *testing.T) {
	view := newShell(newTheme("dark"))
	view.agentSelectorVisible = true
	view.inspectorOverride = true
	view.inspectorVisible = true
	view.agentEditorVisible = true
	view.agentEditorOriginalID = "reviewer"
	profiles := []app.ACPAgentProfile{
		{ID: controllerAgentID, DisplayName: "Protonman", Command: "protonman", Args: []string{"--acp"}},
		{ID: "reviewer", DisplayName: "Reviewer", Command: "reviewer-acp", Args: []string{"--stdio"}, Env: []string{"TOKEN"}},
	}
	snapshot := controllerSnapshot{
		State: desktopstate.State{
			ActiveSessionID: "review-session",
			ActiveProjectID: "project",
			Projects:        []desktopstate.ProjectState{{ID: "project", Name: "project", DefaultAgentID: "reviewer", AgentIDs: []string{"reviewer"}}},
			Sessions: []desktopstate.SessionState{{
				ID: "review-session", ProjectID: "project", Title: "Review", AgentID: "reviewer", Status: desktopstate.TaskIdle,
			}},
		},
		ActiveAgentID:    "reviewer",
		AgentProfiles:    profiles,
		AgentConnections: map[string]connectionPhase{controllerAgentID: connectionConnected, "reviewer": connectionConnected},
		Connection:       connectionConnected,
		Status:           "Connected",
	}
	view.syncAgentProfileEditors(snapshot)
	for _, size := range []image.Point{{X: 760, Y: 600}, {X: 1180, Y: 760}} {
		var operations op.Ops
		var router input.Router
		gtx := layout.Context{
			Ops:         &operations,
			Constraints: layout.Exact(size),
			Metric:      unit.Metric{},
			Now:         time.Unix(1, 0),
			Source:      router.Source(),
		}
		dims := view.layout(gtx, snapshot)
		router.Frame(gtx.Ops)
		if dims.Size != size {
			t.Fatalf("layout size = %v, want %v", dims.Size, size)
		}
	}
}

func TestShellLaysOutMCPIntegrationFormAtBothBreakpoints(t *testing.T) {
	view := newShell(newTheme("light"))
	view.inspectorOverride = true
	view.inspectorVisible = true
	view.mcpFormVisible = true
	view.mcpSelectedName = "docs"
	snapshot := controllerSnapshot{
		State: desktopstate.State{
			ActiveSessionID: "session",
			Sessions: []desktopstate.SessionState{{
				ID:     "session",
				Title:  "MCP",
				Status: desktopstate.TaskIdle,
			}},
			Integrations: []desktopstate.MCPIntegrationState{{
				Name: "docs", Command: "mcp-docs", Args: []string{"--stdio"}, Env: []string{"TOKEN"},
			}},
		},
		Connection: connectionConnected,
		Status:     "Connected",
	}
	for _, size := range []image.Point{{X: 760, Y: 600}, {X: 1180, Y: 760}} {
		var operations op.Ops
		var router input.Router
		gtx := layout.Context{
			Ops:         &operations,
			Constraints: layout.Exact(size),
			Metric:      unit.Metric{},
			Now:         time.Unix(1, 0),
			Source:      router.Source(),
		}
		dims := view.layout(gtx, snapshot)
		router.Frame(gtx.Ops)
		if dims.Size != size {
			t.Fatalf("layout size = %v, want %v", dims.Size, size)
		}
	}
}
