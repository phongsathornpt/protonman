package main

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/phongsathornpt/proton/internal/adapter/out/sessionfs"
	"github.com/phongsathornpt/proton/internal/core/permission"
	"github.com/phongsathornpt/proton/internal/core/session"
)

func TestResolveSessionExplicitIdentitySemantics(t *testing.T) {
	store, err := sessionfs.NewFileStore(filepath.Join(t.TempDir(), "sessions"))
	if err != nil {
		t.Fatal(err)
	}
	work := filepath.Join(t.TempDir(), "project")
	ctx := context.Background()
	id, state, found, err := resolveSession(ctx, store, work, cliOptions{sessionID: "named-session"})
	if err != nil || found || id != "named-session" || state.WorkspaceKey != workspaceKey(work) {
		t.Fatalf("new explicit = id %q found %v state %+v err %v", id, found, state, err)
	}
	if err := store.Save(ctx, id, session.State{PermissionMode: permission.ModeAsk.String(), WorkspaceKey: state.WorkspaceKey}); err != nil {
		t.Fatal(err)
	}
	if _, _, _, err := resolveSession(ctx, store, work, cliOptions{sessionID: id}); err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("existing explicit error = %v", err)
	}
	gotID, _, gotFound, err := resolveSession(ctx, store, work, cliOptions{resume: true, sessionID: id})
	if err != nil || !gotFound || gotID != id {
		t.Fatalf("resume = %q %v %v", gotID, gotFound, err)
	}
}

func TestResolveSessionRejectsCrossWorkspaceResume(t *testing.T) {
	store, err := sessionfs.NewFileStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if err := store.Save(ctx, "cross", session.State{PermissionMode: permission.ModeAsk.String(), WorkspaceKey: workspaceKey("/workspace/a")}); err != nil {
		t.Fatal(err)
	}
	_, _, _, err = resolveSession(ctx, store, "/workspace/b", cliOptions{resume: true, sessionID: "cross"})
	if err == nil || !strings.Contains(err.Error(), "another workspace") {
		t.Fatalf("cross-workspace error = %v", err)
	}
}

func TestGenerateSessionIDIsCollisionResistant(t *testing.T) {
	a := generateSessionID("/workspace/a")
	b := generateSessionID("/workspace/a")
	if a == b {
		t.Fatalf("generated duplicate session id %q", a)
	}
	if !strings.HasPrefix(a, "workspace-"+workspaceKey("/workspace/a")+"-") {
		t.Fatalf("id = %q", a)
	}
}
