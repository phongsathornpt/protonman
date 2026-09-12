package app

import (
	"fmt"

	"github.com/phongsathornpt/protonman/internal/core/permission"
	sdk "github.com/phongsathornpt/protonman/proton-sdk"
)

// UserSettingsRepository persists portable user-level preferences.
type UserSettingsRepository interface {
	SaveSubagentsEnabled(bool) error
	SaveReasoningEffort(sdk.ReasoningEffort) error
	SaveMaxToolCalls(int) error
	SavePermissionRule(permission.Rule) error
	SaveActiveSkills([]string) error
}

// UserSettings owns mutations to portable user-level Protonman preferences.
type UserSettings struct{ repository UserSettingsRepository }

func NewUserSettings(repository UserSettingsRepository) UserSettings {
	return UserSettings{repository: repository}
}

func (u UserSettings) SaveSubagentsEnabled(enabled bool) error {
	if u.repository == nil {
		return fmt.Errorf("user settings repository is unavailable")
	}
	return u.repository.SaveSubagentsEnabled(enabled)
}

func (u UserSettings) SaveReasoningEffort(effort sdk.ReasoningEffort) error {
	if u.repository == nil {
		return fmt.Errorf("user settings repository is unavailable")
	}
	return u.repository.SaveReasoningEffort(effort)
}

func (u UserSettings) SaveMaxToolCalls(maxToolCalls int) error {
	if u.repository == nil {
		return fmt.Errorf("user settings repository is unavailable")
	}
	return u.repository.SaveMaxToolCalls(maxToolCalls)
}

func (u UserSettings) SavePermissionRule(rule permission.Rule) error {
	if u.repository == nil {
		return fmt.Errorf("user settings repository is unavailable")
	}
	return u.repository.SavePermissionRule(rule)
}

func (u UserSettings) SaveActiveSkills(activeSkills []string) error {
	if u.repository == nil {
		return fmt.Errorf("user settings repository is unavailable")
	}
	return u.repository.SaveActiveSkills(activeSkills)
}
