package config

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/phongsathornpt/protonman/internal/app"
	"github.com/phongsathornpt/protonman/internal/platform/appdirs"
)

// UserDesktopPreferencesStore persists desktop UI preferences (pinned sessions,
// custom session titles, filter mode) in user-global Protonman configuration
// without exposing config persistence directly to an inbound adapter.
type UserDesktopPreferencesStore struct{ homeDir string }

const desktopPreferencesKey = "desktop.preferences.v1"

func NewUserDesktopPreferencesStore(homeDir string) *UserDesktopPreferencesStore {
	return &UserDesktopPreferencesStore{homeDir: homeDir}
}

func (s *UserDesktopPreferencesStore) Load(ctx context.Context) (app.DesktopPreferencesState, error) {
	if err := ctx.Err(); err != nil {
		return app.DesktopPreferencesState{}, err
	}
	dirs, err := appdirs.Resolve(s.homeDir)
	if err != nil {
		return app.DesktopPreferencesState{}, err
	}
	doc, exists, err := readDocument(dirs.Config, "config file", false)
	if err != nil {
		return app.DesktopPreferencesState{}, err
	}
	if !exists || doc.Preferences == nil {
		return app.DesktopPreferencesState{}, nil
	}
	raw, ok := doc.Preferences[desktopPreferencesKey]
	if !ok || len(raw) == 0 {
		return app.DesktopPreferencesState{}, nil
	}
	var state app.DesktopPreferencesState
	if err := json.Unmarshal(raw, &state); err != nil {
		return app.DesktopPreferencesState{}, fmt.Errorf("decode desktop preferences: %w", err)
	}
	return state, nil
}

func (s *UserDesktopPreferencesStore) Save(ctx context.Context, state app.DesktopPreferencesState) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	dirs, err := appdirs.Resolve(s.homeDir)
	if err != nil {
		return err
	}
	encoded, err := json.Marshal(state)
	if err != nil {
		return fmt.Errorf("encode desktop preferences: %w", err)
	}
	return modifyUserConfigFile(dirs.Home, false, func(doc *fileDocument) {
		if doc.Preferences == nil {
			doc.Preferences = make(map[string]json.RawMessage)
		}
		doc.Preferences[desktopPreferencesKey] = encoded
	})
}
