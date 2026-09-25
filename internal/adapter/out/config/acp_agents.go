package config

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/phongsathornpt/protonman/internal/app"
	"github.com/phongsathornpt/protonman/internal/platform/appdirs"
)

// UserACPAgentsStore persists desktop-owned ACP profiles in user-global
// Protonman configuration without exposing config persistence to an inbound
// adapter.
type UserACPAgentsStore struct{ homeDir string }

const acpAgentsPreferencesKey = "acp.agents.v1"

type fileACPAgentProfile struct {
	ID          string   `json:"id"`
	DisplayName string   `json:"displayName,omitempty"`
	Command     string   `json:"command"`
	Args        []string `json:"args,omitempty"`
	Env         []string `json:"env,omitempty"`
}

func NewUserACPAgentsStore(homeDir string) *UserACPAgentsStore {
	return &UserACPAgentsStore{homeDir: homeDir}
}

func (s *UserACPAgentsStore) Load(ctx context.Context) ([]app.ACPAgentProfile, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	dirs, err := appdirs.Resolve(s.homeDir)
	if err != nil {
		return nil, err
	}
	doc, exists, err := readDocument(dirs.Config, "config file", false)
	if err != nil {
		return nil, err
	}
	if !exists || doc.Preferences == nil {
		return nil, nil
	}
	raw, ok := doc.Preferences[acpAgentsPreferencesKey]
	if !ok {
		return nil, nil
	}
	stored, err := decodeACPAgentProfiles(raw)
	if err != nil {
		return nil, fmt.Errorf("decode ACP agents preference: %w", err)
	}
	profiles := make([]app.ACPAgentProfile, 0, len(stored))
	for _, profile := range stored {
		profiles = append(profiles, app.ACPAgentProfile{
			ID:          profile.ID,
			DisplayName: profile.DisplayName,
			Command:     profile.Command,
			Args:        append([]string(nil), profile.Args...),
			Env:         append([]string(nil), profile.Env...),
		})
	}
	return profiles, nil
}

func (s *UserACPAgentsStore) Save(ctx context.Context, profiles []app.ACPAgentProfile) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	dirs, err := appdirs.Resolve(s.homeDir)
	if err != nil {
		return err
	}
	stored := make([]fileACPAgentProfile, 0, len(profiles))
	for _, profile := range profiles {
		for _, value := range profile.Env {
			if strings.ContainsRune(value, '=') {
				return errors.New("ACP agent environment values are not persisted")
			}
		}
		stored = append(stored, fileACPAgentProfile{
			ID:          profile.ID,
			DisplayName: profile.DisplayName,
			Command:     profile.Command,
			Args:        append([]string(nil), profile.Args...),
			Env:         append([]string(nil), profile.Env...),
		})
	}
	encoded, err := json.Marshal(stored)
	if err != nil {
		return fmt.Errorf("encode ACP agents preference: %w", err)
	}
	return modifyUserConfigFile(dirs.Home, false, func(doc *fileDocument) {
		if doc.Preferences == nil {
			doc.Preferences = make(map[string]json.RawMessage)
		}
		doc.Preferences[acpAgentsPreferencesKey] = encoded
	})
}

func decodeACPAgentProfiles(raw json.RawMessage) ([]fileACPAgentProfile, error) {
	var stored []fileACPAgentProfile
	if err := json.Unmarshal(raw, &stored); err == nil {
		return stored, nil
	}
	var encoded string
	if err := json.Unmarshal(raw, &encoded); err != nil {
		return nil, err
	}
	if err := json.Unmarshal([]byte(encoded), &stored); err != nil {
		return nil, err
	}
	return stored, nil
}
