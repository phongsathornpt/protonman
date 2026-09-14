//go:build desktop

package desktop

import (
	"strings"
	"testing"

	desktopstate "github.com/phongsathornpt/protonman/internal/feature/desktop"
)

func TestRenderConversationExcludesInspectorState(t *testing.T) {
	session := desktopstate.SessionState{
		Context: desktopstate.SessionContextState{
			Goal: "goal belongs in inspector",
			Todo: desktopstate.TodoState{Items: []desktopstate.TodoItemState{{
				ID: "todo-1", Text: "todo belongs in inspector", Status: "in_progress",
			}}},
			Memory: desktopstate.MemoryState{Workspace: []desktopstate.MemoryEntryState{{
				ID: "memory-1", Kind: "repo_fact", Key: "workspace_fact", Value: "memory belongs in inspector",
			}}},
		},
		Timeline: []desktopstate.TimelineItem{{
			Kind: desktopstate.TimelineTool, Title: "Read theme.go", Status: "completed",
		}},
	}

	got := renderConversation("Assistant response", session)

	for _, inspectorText := range []string{
		"goal belongs in inspector",
		"todo belongs in inspector",
		"memory belongs in inspector",
		"workspace_fact",
	} {
		if strings.Contains(got, inspectorText) {
			t.Fatalf("conversation included inspector state %q: %s", inspectorText, got)
		}
	}
	if !strings.Contains(got, "Assistant response") {
		t.Fatalf("conversation lost transcript: %s", got)
	}
	if !strings.Contains(got, "Read theme.go") {
		t.Fatalf("conversation lost visible activity: %s", got)
	}
}
