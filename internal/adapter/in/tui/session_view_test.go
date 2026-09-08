package tui

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/phongsathornpt/protonman/internal/adapter/out/model"
	"github.com/phongsathornpt/protonman/internal/adapter/out/sessionfs"
	"github.com/phongsathornpt/protonman/internal/app"
	"github.com/phongsathornpt/protonman/internal/core/permission"
	"github.com/phongsathornpt/protonman/internal/core/session"
)

func TestSessionCommandsExposeIdentityAndWorkspaceSessions(t *testing.T) {
	m := newTestBubbleModel(t, permission.ModeAsk, nil)
	m.sessionID = "current-session"
	m.workspaceKey = "workspace-key"
	m.messages = []model.Message{{Role: model.RoleUser, Content: "hello"}}
	m.executeCommand("/session")
	content := m.historyState.RenderContent()
	for _, want := range []string{"current-session", "workspace-key", "messages: 1"} {
		if !strings.Contains(content, want) {
			t.Fatalf("/session missing %q: %s", want, content)
		}
	}

	store, err := sessionfs.NewFileStore(filepath.Join(t.TempDir(), "sessions"))
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Save(context.Background(), "current-session", session.State{
		PermissionMode: permission.ModeAsk.String(), WorkspaceKey: "workspace-key", AgentProfile: "dex",
		Messages: []session.Message{{Role: model.RoleUser, Content: "resume this work"}},
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.Save(context.Background(), "other-session", session.State{
		PermissionMode: permission.ModeAsk.String(), WorkspaceKey: "other",
		Messages: []session.Message{{Role: model.RoleUser, Content: "do not show"}},
	}); err != nil {
		t.Fatal(err)
	}
	m.sessions = app.NewSessions(store)
	m.executeCommand("/sessions")
	content = m.historyState.RenderContent()
	for _, want := range []string{"Recent sessions:", "current-session", "resume this work", "protonman session resume"} {
		if !strings.Contains(content, want) {
			t.Fatalf("/sessions missing %q: %s", want, content)
		}
	}
	if strings.Contains(content, "other-session") {
		t.Fatalf("cross-workspace session leaked: %s", content)
	}
}
