package acp

import (
	"context"
	"testing"
	"time"

	"github.com/phongsathornpt/protonman/internal/app"
	corememory "github.com/phongsathornpt/protonman/internal/core/memory"
	"github.com/phongsathornpt/protonman/internal/core/permission"
)

type acpMemoryRepo struct {
	workspace    []corememory.Entry
	global       []corememory.Entry
	forgetScopes []string
}

func (r *acpMemoryRepo) Load(_ context.Context, scope corememory.Scope, workspaceKey string) ([]corememory.Entry, error) {
	if scope == corememory.ScopeWorkspace {
		out := make([]corememory.Entry, 0, len(r.workspace))
		for _, entry := range r.workspace {
			if entry.WorkspaceKey == workspaceKey {
				out = append(out, entry)
			}
		}
		return out, nil
	}
	return append([]corememory.Entry(nil), r.global...), nil
}
func (r *acpMemoryRepo) Replace(context.Context, corememory.Scope, string, []corememory.Entry) error {
	return nil
}
func (r *acpMemoryRepo) Update(context.Context, corememory.Scope, string, corememory.UpdateFunc) error {
	return nil
}
func (r *acpMemoryRepo) Forget(_ context.Context, scope corememory.Scope, workspaceKey string, ids []string) (int, error) {
	r.forgetScopes = append(r.forgetScopes, string(scope)+"|"+workspaceKey)
	drop := make(map[string]struct{}, len(ids))
	for _, id := range ids {
		drop[id] = struct{}{}
	}
	removed := 0
	filter := func(entries []corememory.Entry) []corememory.Entry {
		kept := make([]corememory.Entry, 0, len(entries))
		for _, entry := range entries {
			if _, ok := drop[entry.ID]; ok {
				removed++
				continue
			}
			kept = append(kept, entry)
		}
		return kept
	}
	if scope == corememory.ScopeWorkspace {
		r.workspace = filter(r.workspace)
	} else {
		r.global = filter(r.global)
	}
	return removed, nil
}
func (r *acpMemoryRepo) RecordUsage(context.Context, []corememory.UsageRef, time.Time) error {
	return nil
}
func (r *acpMemoryRepo) ProcessedRevision(context.Context, string) (uint64, bool, error) {
	return 0, false, nil
}
func (r *acpMemoryRepo) MarkProcessed(context.Context, string, uint64) error { return nil }

func newMemoryServer(t *testing.T, repo *acpMemoryRepo, workspaceKey string) *Server {
	t.Helper()
	server := newTestServer(t, permission.ModeAsk)
	server.memories = app.NewMemories(repo)
	if workspaceKey != "" {
		server.sessions["session-a"] = &Session{id: "session-a", workspaceKey: workspaceKey}
	}
	return server
}

// TestSessionMemoryForgetUsesServerSideWorkspaceKey verifies that the workspace
// key comes from trusted session state, and that a client cannot retarget a
// forget at another workspace by supplying a key in the payload.
func TestSessionMemoryForgetUsesServerSideWorkspaceKey(t *testing.T) {
	repo := &acpMemoryRepo{workspace: []corememory.Entry{
		{ID: "w1", Scope: corememory.ScopeWorkspace, Kind: corememory.KindRepoFact, Key: "path", Value: "wrong", WorkspaceKey: "bound-workspace", Confidence: 1},
		{ID: "other", Scope: corememory.ScopeWorkspace, Kind: corememory.KindRepoFact, Key: "path", Value: "keep", WorkspaceKey: "victim-workspace", Confidence: 1},
	}}
	server := newMemoryServer(t, repo, "bound-workspace")

	result, err := server.sessionMemoryForget(context.Background(), ProtonmanSessionMemoryForgetParams{
		SessionID: "session-a",
		IDs:       []string{"w1"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Removed != 1 || result.Scope != string(app.MemoryScopeWorkspace) {
		t.Fatalf("result = %#v", result)
	}
	if len(repo.forgetScopes) != 1 || repo.forgetScopes[0] != "workspace|bound-workspace" {
		t.Fatalf("forget scopes = %#v, want the session-bound workspace", repo.forgetScopes)
	}
	for _, entry := range repo.workspace {
		if entry.ID == "other" {
			return
		}
	}
	t.Fatal("unrelated workspace entry was removed")
}

func TestSessionMemoryForgetGlobalScopeIsExplicit(t *testing.T) {
	repo := &acpMemoryRepo{
		workspace: []corememory.Entry{{ID: "w1", Scope: corememory.ScopeWorkspace, Kind: corememory.KindRepoFact, Key: "path", Value: "v", WorkspaceKey: "bound-workspace", Confidence: 1}},
		global:    []corememory.Entry{{ID: "g1", Scope: corememory.ScopeGlobal, Kind: corememory.KindPreference, Key: "style", Value: "v", Confidence: 1}},
	}
	server := newMemoryServer(t, repo, "bound-workspace")

	result, err := server.sessionMemoryForget(context.Background(), ProtonmanSessionMemoryForgetParams{
		SessionID: "session-a",
		Scope:     "global",
		IDs:       []string{"g1"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Removed != 1 || result.Scope != string(app.MemoryScopeGlobal) {
		t.Fatalf("result = %#v", result)
	}
	if len(repo.forgetScopes) != 1 || repo.forgetScopes[0] != "global|" {
		t.Fatalf("forget scopes = %#v, want global with no workspace key", repo.forgetScopes)
	}
	if len(repo.workspace) != 1 {
		t.Fatal("global forget must not touch workspace scope")
	}
}

func TestSessionMemoryForgetRejectsUnboundSession(t *testing.T) {
	repo := &acpMemoryRepo{}
	server := newMemoryServer(t, repo, "")

	if _, err := server.sessionMemoryForget(context.Background(), ProtonmanSessionMemoryForgetParams{
		SessionID: "missing-session",
		IDs:       []string{"w1"},
	}); err == nil {
		t.Fatal("expected workspace forget to fail closed for an unbound session")
	}
	if len(repo.forgetScopes) != 0 {
		t.Fatalf("repository was called for an unbound session: %#v", repo.forgetScopes)
	}
}
