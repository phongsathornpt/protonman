//go:build desktop || desktop_gio

package shell

import (
	"image"
	"image/png"
	"os"
	"testing"
	"time"

	"gioui.org/gpu/headless"
	"gioui.org/io/input"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/paint"
	"gioui.org/unit"

	"github.com/phongsathornpt/protonman/internal/adapter/in/desktop/gioui/controller"
	"github.com/phongsathornpt/protonman/internal/app"
	desktopstate "github.com/phongsathornpt/protonman/internal/feature/desktop"
)

func captureDesktopWidget(t *testing.T, path string, size image.Point, view *Shell, render layout.Widget) {
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
		paint.Fill(&ops, view.theme.Colors.Surface)
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

func desktopCaptureSnapshot() controller.Snapshot {
	return controller.Snapshot{
		Connection: controller.ConnectionConnected, Status: "Connected", HistoryState: controller.HistoryStateLoaded, Revision: 1, ActiveAgentID: controller.ProtonmanAgentID,
		AgentConnections: map[string]controller.ConnectionPhase{controller.ProtonmanAgentID: controller.ConnectionConnected, "reviewer": controller.ConnectionConnected},
		AgentProfiles:    []app.ACPAgentProfile{{ID: controller.ProtonmanAgentID, DisplayName: "Protonman", Command: "protonman", Args: []string{"--acp"}}, {ID: "reviewer", DisplayName: "Code reviewer", Command: "review-agent"}},
		State: desktopstate.State{
			ActiveProjectID: "project", ActiveSessionID: "session",
			Projects:     []desktopstate.ProjectState{{ID: "project", Name: "Protonman", DefaultAgentID: controller.ProtonmanAgentID}},
			Integrations: []desktopstate.MCPIntegrationState{{Name: "docs", Command: "docs-server"}},
			Sessions: []desktopstate.SessionState{{ID: "session", ProjectID: "project", AgentID: controller.ProtonmanAgentID, Title: "Refine the desktop experience", Workspace: "/workspaces/protonman", Status: desktopstate.TaskCompleted,
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
