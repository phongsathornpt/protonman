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

	desktopstate "github.com/phongsathornpt/protonman/internal/feature/desktop"
)

func TestParseAssistantThinking(t *testing.T) {
	tests := []struct {
		name         string
		input        string
		wantThinking bool
		wantDone     bool
		wantThink    string
		wantResp     string
	}{
		{
			name:         "plain text without thinking",
			input:        "Hello, how can I help you?",
			wantThinking: false,
			wantDone:     false,
			wantThink:    "",
			wantResp:     "Hello, how can I help you?",
		},
		{
			name:         "complete thinking block",
			input:        "<think>\nNeed to check the repo structure.\n</think>\nHere is the answer.",
			wantThinking: true,
			wantDone:     true,
			wantThink:    "Need to check the repo structure.",
			wantResp:     "Here is the answer.",
		},
		{
			name:         "streaming unclosed thinking",
			input:        "<think>\nStill reasoning about this difficult problem...",
			wantThinking: true,
			wantDone:     false,
			wantThink:    "Still reasoning about this difficult problem...",
			wantResp:     "",
		},
		{
			name:         "thinking with empty response",
			input:        "<think>Done reasoning</think>",
			wantThinking: true,
			wantDone:     true,
			wantThink:    "Done reasoning",
			wantResp:     "",
		},
		{
			name:         "text before and after thinking",
			input:        "Prefix<think>Middle thought</think>Suffix",
			wantThinking: true,
			wantDone:     true,
			wantThink:    "Middle thought",
			wantResp:     "Prefix\n\nSuffix",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := parseAssistantThinking(tc.input)
			if got.hasThinking != tc.wantThinking {
				t.Errorf("hasThinking = %v, want %v", got.hasThinking, tc.wantThinking)
			}
			if got.thinkingDone != tc.wantDone {
				t.Errorf("thinkingDone = %v, want %v", got.thinkingDone, tc.wantDone)
			}
			if got.thinkingText != tc.wantThink {
				t.Errorf("thinkingText = %q, want %q", got.thinkingText, tc.wantThink)
			}
			if got.responseText != tc.wantResp {
				t.Errorf("responseText = %q, want %q", got.responseText, tc.wantResp)
			}
		})
	}
}

func TestShellLaysOutAIFeaturesAndThinking(t *testing.T) {
	view := newShell(newTheme("dark"))
	snapshot := controllerSnapshot{
		State: desktopstate.State{
			ActiveSessionID: "ai-session",
			ActiveProjectID: "proj-1",
			Projects: []desktopstate.ProjectState{
				{ID: "proj-1", Name: "Protonman Project", DefaultAgentID: controllerAgentID},
			},
			Sessions: []desktopstate.SessionState{
				{
					ID:        "ai-session",
					ProjectID: "proj-1",
					Title:     "Material 3 AI Chat",
					Status:    desktopstate.TaskRunning,
					Timeline: []desktopstate.TimelineItem{
						{
							ID:   "u1",
							Kind: desktopstate.TimelineUser,
							Text: "Redesign the desktop app with Material 3 and AI features.",
						},
						{
							ID:   "a1",
							Kind: desktopstate.TimelineAssistant,
							Text: "<think>\nAnalyzing Material 3 requirements:\n1. Tonal palette\n2. AI Chat UI\n3. Thinking blocks\n4. Subagents\n5. Goals and TODOs\n</think>\nHere is the updated design.",
						},
						{
							ID:     "t1",
							Kind:   desktopstate.TimelineTool,
							Title:  "bash: rtk make test",
							Status: "completed",
							Text:   "160 passed in 3 packages",
						},
					},
					Subagents: []desktopstate.SubagentState{
						{
							ID:      "sub-str",
							Profile: "strength",
							Task:    "Implement Material 3 tokens and components",
							Status:  "completed",
							Summary: "Added tonal palettes and rounded cards",
						},
						{
							ID:      "sub-agi",
							Profile: "agility",
							Task:    "Trace thinking blocks in assistant stream",
							Status:  "running",
						},
						{
							ID:      "sub-int",
							Profile: "intelligence",
							Task:    "Verify architectural invariants and contrast",
							Status:  "farming",
							Summary: "Contrast ratio > 5.5:1 across all roles",
						},
					},
					Context: desktopstate.SessionContextState{
						Goal: "Ship Material 3 AI Chat App Redesign",
						Todo: desktopstate.TodoState{
							Revision: 2,
							Items: []desktopstate.TodoItemState{
								{ID: "todo-1", Text: "Define Material 3 tokens", Status: "completed"},
								{ID: "todo-2", Text: "Implement collapsible thinking", Status: "in_progress"},
								{ID: "todo-3", Text: "Verify subagent attribute cards", Status: "pending"},
							},
						},
						Memory: desktopstate.MemoryState{
							WorkspaceKey: "workspace-1",
							Workspace: []desktopstate.MemoryEntryState{
								{ID: "m1", Kind: "repo_fact", Key: "GUI Framework", Value: "Gio (gioui.org)", Confidence: 0.95, UsageCount: 4},
							},
							Global: []desktopstate.MemoryEntryState{
								{ID: "m2", Kind: "preference", Key: "Design System", Value: "Material 3", Confidence: 1.0, UsageCount: 2},
							},
						},
					},
					Runtime: desktopstate.RuntimeSettingsState{
						Provider:       "protonman",
						Model:          "qwen3.8-27b",
						Reasoning:      "high",
						LowConcurrency: "on",
					},
				},
			},
		},
		Connection: connectionConnected,
		Status:     "Connected · running",
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
