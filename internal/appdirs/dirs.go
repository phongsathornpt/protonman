// Package appdirs centralizes Proton filesystem layout and home resolution.
package appdirs

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const (
	EnvHome        = "PROTON_HOME"
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
		home = strings.TrimSpace(os.Getenv(EnvHome))
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

// ProjectRoot returns the project-local Proton directory.
func ProjectRoot(workDir string) string { return filepath.Join(workDir, RootDirName) }

// ProjectConfig returns the project-local config path.
func ProjectConfig(workDir string) string { return filepath.Join(ProjectRoot(workDir), ConfigFileName) }

// ProjectSkills returns the project-local skills directory.
func ProjectSkills(workDir string) string { return filepath.Join(ProjectRoot(workDir), SkillsDir) }
