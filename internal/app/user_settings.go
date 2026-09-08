package app

import (
	"github.com/phongsathornpt/protonman/internal/adapter/out/config"
	sdk "github.com/phongsathornpt/protonman/proton-sdk"
)

// UserSettings owns mutations to portable user-level Protonman preferences.
type UserSettings struct{}

func (UserSettings) SaveSubagentsEnabled(enabled bool) error {
	homeDir, err := userHomeDir()
	if err != nil {
		return err
	}
	return config.SaveUserSubagentsEnabled(homeDir, enabled)
}

func (UserSettings) SaveReasoningEffort(effort sdk.ReasoningEffort) error {
	homeDir, err := userHomeDir()
	if err != nil {
		return err
	}
	return config.SaveUserReasoningEffort(homeDir, effort)
}

func (UserSettings) SaveMaxToolCalls(maxToolCalls int) error {
	homeDir, err := userHomeDir()
	if err != nil {
		return err
	}
	return config.SaveUserMaxToolCalls(homeDir, maxToolCalls)
}
