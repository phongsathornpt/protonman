//go:build desktop || desktop_gio

package gioui

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
	"gioui.org/unit"

	desktopstate "github.com/phongsathornpt/protonman/internal/feature/desktop"
)

func TestRenderDesktopScreenshots(t *testing.T) {
	for _, mode := range []string{"light", "dark"} {
		view := newShell(newTheme(mode))
		snapshot := controllerSnapshot{
			State: desktopstate.State{
				ActiveSessionID: "ai-chat-1",
				ActiveProjectID: "proj-1",
				Projects: []desktopstate.ProjectState{
					{ID: "proj-1", Name: "Protonman", DefaultAgentID: controllerAgentID},
				},
				Sessions: []desktopstate.SessionState{
					{
						ID:        "ai-chat-1",
						ProjectID: "proj-1",
						Title:     "Material 3 AI Chat",
						Status:    desktopstate.TaskRunning,
						Timeline: []desktopstate.TimelineItem{
							{
								ID:   "u1",
								Kind: desktopstate.TimelineUser,
								Text: "Please redesign the desktop application with Material 3 styling and integrate the core AI features: active goal, task plan, durable memory, thinking blocks, and subagents.",
							},
							{
								ID:   "a1",
								Kind: desktopstate.TimelineAssistant,
								Text: "<think>\n1. Material 3 Design System:\n   - Clean neutral surfaces (surfaceContainerLowest to surfaceContainerHighest)\n   - One cobalt accent (#315BE8 in light, #9DB7FF in dark)\n   - Dota-style attribute tokens for STR/AGI/INT subagents\n2. AI Chat Features:\n   - Thinking block with 1-click collapse\n   - Tool cards with semantic badges and icons\n   - Interactive task progress indicator\n   - Durable memory categorized by workspace facts vs global preferences\n3. Verification:\n   - Contrast ratios > 5:1 for accessibility\n</think>\nI've redesigned the application following the Material 3 design system. Here are the components and live runtime status.",
							},
							{
								ID:     "t1",
								Kind:   desktopstate.TimelineTool,
								Title:  "edit: write internal/adapter/in/desktop/gioui/theme.go",
								Status: "completed",
								Text:   "@@ -10,6 +10,18 @@\n+ surfaceContainerLowest: hex(0xFFFFFF),\n+ tertiary: hex(0x6B21A8),\n+ tertiaryContainer: hex(0xF3E8FF),",
							},
							{
								ID:     "t2",
								Kind:   desktopstate.TimelineTool,
								Title:  "bash: rtk make test-desktop",
								Status: "completed",
								Text:   "PASS: 143 tests passed in 0.84s",
							},
						},
						Subagents: []desktopstate.SubagentState{
							{
								ID:      "sub-str",
								Profile: "strength",
								Task:    "Implement Material 3 surface roles and component styling",
								Status:  "completed",
								Summary: "Configured 13 M3 tonal roles and rounded corners",
							},
							{
								ID:      "sub-agi",
								Profile: "agility",
								Task:    "Audit thinking stream parser and tool card rendering",
								Status:  "running",
							},
							{
								ID:      "sub-int",
								Profile: "intelligence",
								Task:    "Verify contrast compliance and architectural boundaries",
								Status:  "farming",
								Summary: "Contrast ratio > 5.3:1 across light and dark palettes",
							},
						},
						Context: desktopstate.SessionContextState{
							Goal: "Ship Material 3 AI Chat App Redesign",
							Todo: desktopstate.TodoState{
								Revision: 3,
								Items: []desktopstate.TodoItemState{
									{ID: "todo-1", Text: "Design system tokens and contrast tests", Status: "completed"},
									{ID: "todo-2", Text: "Collapsible thinking blocks and tool cards", Status: "completed"},
									{ID: "todo-3", Text: "Task planning with linear progress bar", Status: "in_progress"},
									{ID: "todo-4", Text: "Visual audit and screenshot verification", Status: "pending"},
								},
							},
							Memory: desktopstate.MemoryState{
								WorkspaceKey: "protonman",
								Workspace: []desktopstate.MemoryEntryState{
									{ID: "m1", Kind: "repo_fact", Key: "UI Framework", Value: "Gio (gioui.org)", Confidence: 0.98, UsageCount: 12},
									{ID: "m2", Kind: "repo_fact", Key: "Architecture", Value: "Clean Architecture (hexagonal ports)", Confidence: 0.95, UsageCount: 8},
								},
								Global: []desktopstate.MemoryEntryState{
									{ID: "m3", Kind: "preference", Key: "Design Language", Value: "Material 3 (M3)", Confidence: 1.0, UsageCount: 5},
									{ID: "m4", Kind: "preference", Key: "AI Agent Attributes", Value: "Dota-style STR/AGI/INT roles", Confidence: 0.9, UsageCount: 3},
								},
							},
						},
						Runtime: desktopstate.RuntimeSettingsState{
							Provider:       "protonman",
							Model:          "qwen3.8-27b",
							Reasoning:      "high",
							LowConcurrency: "auto",
						},
					},
				},
			},
			Connection: connectionConnected,
			Status:     "Connected · Ready",
			Revision:   1,
		}

		view.inspectorOverride = true
		view.inspectorVisible = true

		width, height := 1180, 760
		win, err := headless.NewWindow(width, height)
		if err != nil {
			t.Fatalf("headless.NewWindow failed: %v", err)
		}
		defer win.Release()

		var operations op.Ops
		var router input.Router
		gtx := layout.Context{
			Ops:         &operations,
			Constraints: layout.Exact(image.Pt(width, height)),
			Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
			Now:         time.Unix(1, 0),
			Source:      router.Source(),
		}

		view.layout(gtx, snapshot)
		if err := win.Frame(gtx.Ops); err != nil {
			t.Fatalf("win.Frame failed: %v", err)
		}

		img := image.NewRGBA(image.Rect(0, 0, width, height))
		if err := win.Screenshot(img); err != nil {
			t.Fatalf("win.Screenshot failed: %v", err)
		}

		outPath := "/tmp/desktop_audit_" + mode + ".png"
		f, err := os.Create(outPath)
		if err != nil {
			t.Fatalf("failed to create %s: %v", outPath, err)
		}
		if err := png.Encode(f, img); err != nil {
			f.Close()
			t.Fatalf("failed to encode png: %v", err)
		}
		f.Close()
		t.Logf("Wrote screenshot to %s", outPath)
	}
}
