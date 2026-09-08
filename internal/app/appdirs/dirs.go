// Package appdirs centralizes Proton filesystem layout and home resolution.
package appdirs

import (
	"fmt"
	"github.com/phongsathornpt/protonman/internal/base/envconfig"
	"os"
	"path/filepath"
	"strings"
)

const (
	RootDirName    = ".proton"
	ConfigFileName = "config.toml"
	SessionsDir    = "sessions"
	CheckpointsDir = "checkpoints"
	SkillsDir      = "skills"
	LogsDir        = "logs"
)

// Dirs is the resolved Proton filesystem layout for one user home.
type Dirs struct {
	Home        string
	Root        string
	Config      string
	Sessions    string
	Checkpoints string
	Skills      string
	Logs        string
}

// Resolve returns Proton directories using explicitHome, PROTON_HOME, or os.UserHomeDir.
func Resolve(explicitHome string) (Dirs, error) {
	home := strings.TrimSpace(explicitHome)
	if home == "" {
		home = envconfig.Value(envconfig.Home)
	}
	if home == "" {
		resolved, err := os.UserHomeDir()
		if err != nil {
			return Dirs{}, fmt.Errorf("resolve home directory: %w", err)
		}
		home = resolved
	}
	root := filepath.Join(home, RootDirName)
	return Dirs{
		Home:        home,
		Root:        root,
		Config:      filepath.Join(root, ConfigFileName),
		Sessions:    filepath.Join(root, SessionsDir),
		Checkpoints: filepath.Join(root, CheckpointsDir),
		Skills:      filepath.Join(root, SkillsDir),
		Logs:        filepath.Join(root, LogsDir),
	}, nil
}

// UserConfigDisplay returns the config path shown to users, honoring PROTON_HOME.
func UserConfigDisplay() string {
	if envconfig.Value(envconfig.Home) == "" {
		return "~/.proton/config.toml"
	}
	dirs, err := Resolve("")
	if err != nil {
		return "~/.proton/config.toml"
	}
	return dirs.Config
}

// UserSkillsDisplay returns the user skill directory shown to users, honoring PROTON_HOME.
func UserSkillsDisplay() string {
	if envconfig.Value(envconfig.Home) == "" {
		return "~/.proton/skills/"
	}
	dirs, err := Resolve("")
	if err != nil {
		return "~/.proton/skills/"
	}
	return dirs.Skills + string(filepath.Separator)
}

// UserMCPLogsDisplay returns the MCP log directory shown to users.
func UserMCPLogsDisplay() string {
	if envconfig.Value(envconfig.Home) == "" {
		return "~/.proton/logs/mcp/"
	}
	dirs, err := Resolve("")
	if err != nil {
		return "~/.proton/logs/mcp/"
	}
	return filepath.Join(dirs.Logs, "mcp") + string(filepath.Separator)
}

// ProjectRoot returns the project-local Proton directory.
func ProjectRoot(workDir string) string { return filepath.Join(workDir, RootDirName) }

// ProjectConfig returns the project-local config path.
func ProjectConfig(workDir string) string { return filepath.Join(ProjectRoot(workDir), ConfigFileName) }

// ProjectSkills returns the project-local skills directory.
func ProjectSkills(workDir string) string { return filepath.Join(ProjectRoot(workDir), SkillsDir) }
