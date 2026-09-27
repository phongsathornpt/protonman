package app_test

import (
	"context"
	"testing"

	"github.com/phongsathornpt/protonman/internal/app"
)

type memoryPreferencesRepo struct {
	state app.DesktopPreferencesState
}

func (m *memoryPreferencesRepo) Load(ctx context.Context) (app.DesktopPreferencesState, error) {
	return m.state, nil
}

func (m *memoryPreferencesRepo) Save(ctx context.Context, s app.DesktopPreferencesState) error {
	m.state = s
	return nil
}

func TestDesktopPreferencesTogglePin(t *testing.T) {
	ctx := context.Background()
	repo := &memoryPreferencesRepo{}
	prefs := app.NewDesktopPreferences(repo)

	// Initial load
	_, err := prefs.Load(ctx)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	pinned, err := prefs.TogglePin(ctx, "session-1")
	if err != nil || !pinned {
		t.Fatalf("expected pinned=true, got pinned=%v, err=%v", pinned, err)
	}

	snap := prefs.Snapshot()
	if len(snap.PinnedSessions) != 1 || snap.PinnedSessions[0] != "session-1" {
		t.Fatalf("unexpected snapshot: %+v", snap)
	}

	// Toggle off
	pinned, err = prefs.TogglePin(ctx, "session-1")
	if err != nil || pinned {
		t.Fatalf("expected pinned=false, got pinned=%v, err=%v", pinned, err)
	}
	snap = prefs.Snapshot()
	if len(snap.PinnedSessions) != 0 {
		t.Fatalf("expected 0 pinned sessions, got %+v", snap)
	}
}

func TestDesktopPreferencesCustomTitleAndFilter(t *testing.T) {
	ctx := context.Background()
	repo := &memoryPreferencesRepo{}
	prefs := app.NewDesktopPreferences(repo)
	_, _ = prefs.Load(ctx)

	if err := prefs.SetCustomTitle(ctx, "sess-1", "Custom Title"); err != nil {
		t.Fatalf("SetCustomTitle: %v", err)
	}
	if err := prefs.SetFilterMode(ctx, "running"); err != nil {
		t.Fatalf("SetFilterMode: %v", err)
	}

	snap := prefs.Snapshot()
	if snap.CustomTitles["sess-1"] != "Custom Title" {
		t.Fatalf("expected custom title, got %v", snap.CustomTitles["sess-1"])
	}
	if snap.FilterMode != "running" {
		t.Fatalf("expected filterMode=running, got %s", snap.FilterMode)
	}

	// Remove session
	if err := prefs.RemoveSession(ctx, "sess-1"); err != nil {
		t.Fatalf("RemoveSession: %v", err)
	}
	snap = prefs.Snapshot()
	if _, ok := snap.CustomTitles["sess-1"]; ok {
		t.Fatal("expected custom title to be removed")
	}
}
