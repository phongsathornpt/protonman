package app_test

import (
	"context"
	"fmt"
	"sync"
	"testing"

	"github.com/phongsathornpt/protonman/internal/app"
)

type memoryPreferencesRepo struct {
	mu    sync.Mutex
	state app.DesktopPreferencesState
}

func (m *memoryPreferencesRepo) Load(ctx context.Context) (app.DesktopPreferencesState, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.state, nil
}

func (m *memoryPreferencesRepo) Save(ctx context.Context, s app.DesktopPreferencesState) error {
	m.mu.Lock()
	defer m.mu.Unlock()
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

func TestDesktopPreferencesSetTheme(t *testing.T) {
	ctx := context.Background()
	repo := &memoryPreferencesRepo{}
	prefs := app.NewDesktopPreferences(repo)
	_, _ = prefs.Load(ctx)

	if err := prefs.SetTheme(ctx, "slate-dark"); err != nil {
		t.Fatalf("SetTheme: %v", err)
	}

	snap := prefs.Snapshot()
	if snap.Theme != "slate-dark" {
		t.Fatalf("expected theme=slate-dark, got %s", snap.Theme)
	}

	if repo.state.Theme != "slate-dark" {
		t.Fatalf("expected repo theme=slate-dark, got %s", repo.state.Theme)
	}
}

func TestDesktopPreferencesConcurrentWrites(t *testing.T) {
	ctx := context.Background()
	repo := &memoryPreferencesRepo{}
	prefs := app.NewDesktopPreferences(repo)
	_, _ = prefs.Load(ctx)

	const goroutines = 20
	var wg sync.WaitGroup
	wg.Add(goroutines)

	for i := range goroutines {
		go func(idx int) {
			defer wg.Done()
			sessID := fmt.Sprintf("session-%d", idx)
			_, _ = prefs.TogglePin(ctx, sessID)
			_ = prefs.SetCustomTitle(ctx, sessID, fmt.Sprintf("Title %d", idx))
			_ = prefs.SetTheme(ctx, fmt.Sprintf("theme-%d", idx))
		}(i)
	}
	wg.Wait()

	snap := prefs.Snapshot()
	repo.mu.Lock()
	repoSnap := repo.state
	repo.mu.Unlock()

	if len(snap.PinnedSessions) != goroutines {
		t.Fatalf("expected %d pinned sessions, got %d", goroutines, len(snap.PinnedSessions))
	}
	if len(repoSnap.PinnedSessions) != goroutines {
		t.Fatalf("expected repo to have %d pinned sessions, got %d", goroutines, len(repoSnap.PinnedSessions))
	}
	if len(snap.CustomTitles) != goroutines {
		t.Fatalf("expected %d custom titles, got %d", goroutines, len(snap.CustomTitles))
	}
	if len(repoSnap.CustomTitles) != goroutines {
		t.Fatalf("expected repo to have %d custom titles, got %d", goroutines, len(repoSnap.CustomTitles))
	}
}
