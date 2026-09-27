//go:build desktop || desktop_gio

package gioui

import (
	"image"
	"image/png"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"gioui.org/gpu/headless"
	"gioui.org/io/input"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/paint"
	"gioui.org/unit"

	"github.com/phongsathornpt/protonman/internal/app"
	desktopstate "github.com/phongsathornpt/protonman/internal/feature/desktop"
)

// Opt in to GPU captures; ordinary desktop tests do not need a display or GPU.
func TestDesktopComponentCaptures(t *testing.T) {
	dir := os.Getenv("PROTONMAN_DESKTOP_CAPTURE_DIR")
	if dir == "" {
		t.Skip("set PROTONMAN_DESKTOP_CAPTURE_DIR to capture the desktop component gallery")
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, mode := range []string{"light", "dark"} {
		for _, width := range []int{640, 760, 1180} {
			for _, scene := range []string{"conversation", "tools", "subagents", "permission", "empty", "disconnected", "inspector", "selector"} {
				t.Run(mode+"/"+strconv.Itoa(width)+"/"+scene, func(t *testing.T) {
					snapshot := desktopCaptureSnapshot()
					view := newShell(newTheme(mode))
					view.syncConversation(snapshot.State)
					view.inspectorOverride = true
					view.inspectorVisible = scene == "inspector"
					view.agentSelectorVisible = scene == "selector"
					view.conversationList.ScrollToEnd = false
					session := &snapshot.State.Sessions[0]
					switch scene {
					case "tools":
						session.Timeline = session.Timeline[2:]
					case "subagents":
						session.Timeline = nil
						session.Status = desktopstate.TaskRunning
						session.Subagents = []desktopstate.SubagentState{
							{ID: "s1", Profile: "strength", Task: "Refine the desktop components", Status: "completed", Summary: "Updated shared controls and spacing."},
							{ID: "s2", Profile: "agility", Task: "Trace layout and keyboard behavior", Status: "running"},
							{ID: "s3", Profile: "intelligence", Task: "Review the evidence and architecture boundaries", Status: "queued"},
						}
					case "permission":
						session.Status = desktopstate.TaskWaitingPermission
						snapshot.State.PermissionInbox = []desktopstate.PermissionRequest{{RequestID: "p1", SessionID: session.ID, Title: "Run the desktop tests", Detail: "go test -tags desktop ./internal/adapter/in/desktop/gioui", Options: []desktopstate.PermissionOption{{ID: "allow", Name: "Allow once"}, {ID: "reject", Name: "Reject"}}}}
					case "empty":
						session.Timeline = nil
					case "disconnected":
						snapshot.Connection = connectionReconnecting
						snapshot.Status = "Disconnected · reconnecting"
						session.Timeline = nil
					}
					height := 760
					switch width {
					case 640:
						height = 480
					case 760:
						height = 600
					}
					captureDesktopWidget(t, filepath.Join(dir, mode+"-"+strconv.Itoa(width)+"-"+scene+".png"), image.Pt(width, height), view, func(gtx layout.Context) layout.Dimensions { return view.layout(gtx, snapshot) })
				})
			}
		}
		for index, panel := range []string{"goal", "todo", "memory", "runtime", "mcp", "agents"} {
			t.Run(mode+"/"+panel, func(t *testing.T) {
				view := newShell(newTheme(mode))
				snapshot := desktopCaptureSnapshot()
				view.syncMCPIntegrationEditors(snapshot.State)
				view.syncAgentProfileEditors(snapshot)
				view.syncRuntimeEditors(snapshot.State)
				view.mcpFormVisible = true
				view.agentEditorVisible = true
				view.mcpNameEditor.SetText("docs")
				view.mcpCommandEditor.SetText("docs-server")
				view.mcpArgsEditor.SetText("[]")
				view.mcpEnvEditor.SetText("[]")
				view.agentIDEditor.SetText("reviewer")
				view.agentNameEditor.SetText("Code reviewer")
				view.agentCommandEditor.SetText("review-agent")
				view.agentArgsEditor.SetText("[\"--acp\"]")
				view.agentEnvEditor.SetText("[]")
				captureDesktopWidget(t, filepath.Join(dir, mode+"-panel-"+panel+".png"), image.Pt(336, 940), view, func(gtx layout.Context) layout.Dimensions {
					gtx.Constraints.Min.Y = 0
					return view.layoutInspectorPanel(gtx, snapshot.State.Sessions[0], snapshot, index)
				})
			})
		}
	}
}

func captureDesktopWidget(t *testing.T, path string, size image.Point, view *shell, render layout.Widget) {
	t.Helper()
	window, err := headless.NewWindow(size.X, size.Y)
	if err != nil {
		t.Fatal(err)
	}
	defer window.Release()
	var ops op.Ops
	var router input.Router
	for frame := range 3 {
		ops.Reset()
		gtx := layout.Context{Ops: &ops, Constraints: layout.Exact(size), Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1}, Now: time.Unix(1, int64(frame)*16_000_000), Source: router.Source()}
		paint.Fill(&ops, view.theme.surface)
		render(gtx)
		router.Frame(&ops)
		if err := window.Frame(&ops); err != nil {
			t.Fatal(err)
		}
	}
	img := image.NewRGBA(image.Rectangle{Max: size})
	if err := window.Screenshot(img); err != nil {
		t.Fatal(err)
	}
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := png.Encode(file, img); err != nil {
		file.Close()
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
}

func desktopCaptureSnapshot() controllerSnapshot {
	return controllerSnapshot{
		Connection: connectionConnected, Status: "Connected", HistoryState: historyStateLoaded, Revision: 1, ActiveAgentID: controllerAgentID,
		AgentConnections: map[string]connectionPhase{controllerAgentID: connectionConnected, "reviewer": connectionConnected},
		AgentProfiles:    []app.ACPAgentProfile{{ID: controllerAgentID, DisplayName: "Protonman", Command: "protonman", Args: []string{"--acp"}}, {ID: "reviewer", DisplayName: "Code reviewer", Command: "review-agent"}},
		State: desktopstate.State{
			ActiveProjectID: "project", ActiveSessionID: "session",
			Projects:     []desktopstate.ProjectState{{ID: "project", Name: "Protonman", DefaultAgentID: controllerAgentID}},
			Integrations: []desktopstate.MCPIntegrationState{{Name: "docs", Command: "docs-server"}},
			Sessions: []desktopstate.SessionState{{ID: "session", ProjectID: "project", AgentID: controllerAgentID, Title: "Refine the desktop experience", Workspace: "/workspaces/protonman", Status: desktopstate.TaskCompleted,
				Timeline: []desktopstate.TimelineItem{
					{ID: "u1", Kind: desktopstate.TimelineUser, Text: "Review the desktop components at wide and compact window sizes."},
					{ID: "a1", Kind: desktopstate.TimelineAssistant, Text: "<think>Check shared spacing, text contrast, and keyboard focus before reviewing each panel.</think>I found layout issues in the shared controls. The next step is to check the conversation, inspector, and settings at both sizes."},
					{ID: "t1", Kind: desktopstate.TimelineTool, Title: "edit: internal/adapter/in/desktop/gioui/conversation_view.go", Status: "completed", Text: "Updated the conversation layout.\n+ 12 additions\n- 8 deletions"},
					{ID: "t2", Kind: desktopstate.TimelineTool, Title: "bash: go test -tags desktop ./internal/adapter/in/desktop/gioui", Status: "failed", Text: "Example fixture output: a layout assertion failed."},
				},
				Runtime: desktopstate.RuntimeSettingsState{Provider: "protonman", Model: "coding-model", Reasoning: "high", LowConcurrency: "auto"},
				Context: desktopstate.SessionContextState{Goal: "Refine desktop components and verify the rendered experience", Todo: desktopstate.TodoState{Revision: 3, Items: []desktopstate.TodoItemState{{ID: "1", Text: "Inspect shared component spacing", Status: "completed"}, {ID: "2", Text: "Refine conversation and inspector", Status: "in_progress"}, {ID: "3", Text: "Capture and audit both themes", Status: "pending"}}}, Memory: desktopstate.MemoryState{Workspace: []desktopstate.MemoryEntryState{{ID: "m1", Kind: "repo_fact", Key: "Desktop architecture", Value: "Gio presentation drives the CLI runtime through ACP.", Confidence: 0.95, UsageCount: 4}}, Global: []desktopstate.MemoryEntryState{{ID: "m2", Kind: "preference", Key: "Verification", Value: "Inspect rendered evidence before claiming visual completion.", Confidence: 1, UsageCount: 2}}}},
			}},
		},
	}
}
