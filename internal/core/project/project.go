// Package project owns the pure project-discovery contracts shared by the
// application layer and the filesystem implementation in internal/feature/project.
package project

// Options controls project discovery.
type Options struct {
	HomeDir       string
	WorkDir       string
	Trusted       bool
	ConfigSources []string
}

// State describes project-local Protonman resources without loading configuration.
type State struct {
	WorkDir      string
	Available    bool
	ProtonDir    string
	Exists       bool
	Trusted      bool
	ConfigPath   string
	ConfigExists bool
	ConfigLoaded bool
	SkillsPath   string
	SkillsExists bool
	SkillCount   int
	LockPath     string
	LockExists   bool
}

// InitResult reports what project initialization created.
type InitResult struct {
	ProtonDir  string
	ConfigPath string
	Created    bool
}
