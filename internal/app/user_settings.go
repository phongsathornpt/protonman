package app

import "github.com/phongsathornpt/proton/internal/adapter/out/config"

// UserSettings owns mutations to portable user-level Proton preferences.
type UserSettings struct{}

func (UserSettings) SaveSubagentsEnabled(enabled bool) error {
	homeDir, err := userHomeDir()
	if err != nil {
		return err
	}
	return config.SaveUserSubagentsEnabled(homeDir, enabled)
}
