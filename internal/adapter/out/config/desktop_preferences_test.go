package config_test

import (
	"context"
	"testing"

	"github.com/phongsathornpt/protonman/internal/adapter/out/config"
	"github.com/phongsathornpt/protonman/internal/app"
)

func TestUserDesktopPreferencesStore(t *testing.T) {
	tempHome := t.TempDir()
	store := config.NewUserDesktopPreferencesStore(tempHome)
	ctx := context.Background()

	// Initial load from empty config
	state, err := store.Load(ctx)
	if err != nil {
		t.Fatalf("Load on empty config: %v", err)
	}
	if len(state.PinnedSessions) != 0 {
		t.Fatalf("expected 0 pinned sessions, got %d", len(state.PinnedSessions))
	}

	// Save preferences
	toSave := app.DesktopPreferencesState{
		PinnedSessions: []string{"sess-abc", "sess-xyz"},
		CustomTitles:   map[string]string{"sess-abc": "Feature A"},
		FilterMode:     "pinned",
	}
	if err := store.Save(ctx, toSave); err != nil {
		t.Fatalf("Save: %v", err)
	}

	// Reload and verify
	loaded, err := store.Load(ctx)
	if err != nil {
		t.Fatalf("Load after save: %v", err)
	}
	if len(loaded.PinnedSessions) != 2 || loaded.PinnedSessions[0] != "sess-abc" {
		t.Fatalf("unexpected pinned sessions: %+v", loaded.PinnedSessions)
	}
	if loaded.CustomTitles["sess-abc"] != "Feature A" {
		t.Fatalf("unexpected custom title: %+v", loaded.CustomTitles)
	}
	if loaded.FilterMode != "pinned" {
		t.Fatalf("unexpected filterMode: %s", loaded.FilterMode)
	}
}
