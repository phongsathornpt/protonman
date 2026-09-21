package app

import "github.com/phongsathornpt/protonman/internal/platform/appdirs"

type Services struct {
	Models       Models
	Providers    Providers
	Projects     Projects
	UserSettings UserSettings
	ModelFactory LanguageModelFactory
}

// SaveActiveSkills persists the active skills list to project configuration if a project root exists,
// or user-global settings otherwise.
func (s Services) SaveActiveSkills(workDir string, activeSkills []string) error {
	if appdirs.HasProjectRoot("", workDir) {
		return s.Projects.SaveActiveSkills(workDir, activeSkills)
	}
	return s.UserSettings.SaveActiveSkills(activeSkills)
}
