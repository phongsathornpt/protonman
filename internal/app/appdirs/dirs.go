// Package appdirs centralizes Protonman filesystem layout and home resolution.
package appdirs

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/phongsathornpt/protonman/internal/base/envconfig"
	"github.com/phongsathornpt/protonman/internal/base/pathutil"
)

const (
	RootDirName    = ".protonman"
	ConfigFileName = "config.toml"
	SessionsDir    = "sessions"
	CheckpointsDir = "checkpoints"
	SkillsDir      = "skills"
	LogsDir        = "logs"
)

// Dirs is the resolved Protonman filesystem layout for one user home.
type Dirs struct {
	Home        string
	Root        string
	Config      string
	Sessions    string
	Checkpoints string
	Skills      string
	Logs        string
}

// ProjectScope describes the project-local Protonman namespace after alias checks.
type ProjectScope struct {
	Root      string
	Config    string
	Skills    string
	Available bool
}

// RuntimeLayout is the resolved filesystem topology for one Protonman process.
type RuntimeLayout struct {
	User      Dirs
	Workspace string
	Project   ProjectScope
}

// Resolve returns Protonman directories using explicitHome, PROTONMAN_HOME, or os.UserHomeDir.
func Resolve(explicitHome string) (Dirs, error) {
	home := strings.TrimSpace(explicitHome)
	if home == "" {
		home = envconfig.DirectValue(envconfig.Home)
	}
	if home == "" {
		resolved, err := os.UserHomeDir()
		if err != nil {
			return Dirs{}, fmt.Errorf("resolve home directory: %w", err)
		}
		home = resolved
	}
	root := filepath.Join(home, RootDirName)
	return dirsForRoot(home, root), nil
}

func dirsForRoot(home, root string) Dirs {
	return Dirs{
		Home:        home,
		Root:        root,
		Config:      filepath.Join(root, ConfigFileName),
		Sessions:    filepath.Join(root, SessionsDir),
		Checkpoints: filepath.Join(root, CheckpointsDir),
		Skills:      filepath.Join(root, SkillsDir),
		Logs:        filepath.Join(root, LogsDir),
	}
}

func displayPath(path string) string {
	if envconfig.DirectValue(envconfig.Home) != "" {
		return path
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return path
	}
	cleanHome := filepath.Clean(home)
	cleanPath := filepath.Clean(path)
	if cleanPath == cleanHome {
		return "~"
	}
	prefix := cleanHome + string(filepath.Separator)
	if strings.HasPrefix(cleanPath, prefix) {
		return "~" + cleanPath[len(cleanHome):]
	}
	return path
}

// UserConfigDisplay returns the effective user config path shown to users.
func UserConfigDisplay() string {
	dirs, err := Resolve("")
	if err != nil {
		return "~/.protonman/config.toml"
	}
	return displayPath(dirs.Config)
}

// UserSkillsDisplay returns the effective user skill directory shown to users.
func UserSkillsDisplay() string {
	dirs, err := Resolve("")
	if err != nil {
		return "~/.protonman/skills/"
	}
	return displayPath(dirs.Skills) + string(filepath.Separator)
}

// UserMCPLogsDisplay returns the effective MCP log directory shown to users.
func UserMCPLogsDisplay() string {
	dirs, err := Resolve("")
	if err != nil {
		return "~/.protonman/logs/mcp/"
	}
	return displayPath(filepath.Join(dirs.Logs, "mcp")) + string(filepath.Separator)
}

// ProjectRoot returns the project-local Protonman directory.
func ProjectRoot(workDir string) string { return filepath.Join(workDir, RootDirName) }

// ProjectConfig returns the canonical project-local config path for new data.
func ProjectConfig(workDir string) string { return filepath.Join(ProjectRoot(workDir), ConfigFileName) }

// ProjectSkills returns the canonical project-local skills directory for new data.
func ProjectSkills(workDir string) string { return filepath.Join(ProjectRoot(workDir), SkillsDir) }

// ResolveRuntimeLayout resolves user-global, workspace, and project-local paths once.
func ResolveRuntimeLayout(explicitHome, workDir string) (RuntimeLayout, error) {
	userDirs, err := Resolve(explicitHome)
	if err != nil {
		return RuntimeLayout{}, err
	}
	workDir = strings.TrimSpace(workDir)
	if workDir == "" {
		workDir, err = os.Getwd()
		if err != nil {
			return RuntimeLayout{}, fmt.Errorf("resolve work directory: %w", err)
		}
	}
	absoluteWorkDir, err := filepath.Abs(workDir)
	if err != nil {
		return RuntimeLayout{}, fmt.Errorf("resolve project work directory: %w", err)
	}
	project, err := resolveProjectScope(userDirs, absoluteWorkDir)
	if err != nil {
		return RuntimeLayout{}, err
	}
	return RuntimeLayout{User: userDirs, Workspace: absoluteWorkDir, Project: project}, nil
}

// ResolveProjectScope resolves project-local paths and disables the scope when
// its root aliases the user-global Protonman root.
func ResolveProjectScope(homeDir, workDir string) (ProjectScope, error) {
	userDirs, err := Resolve(homeDir)
	if err != nil {
		return ProjectScope{}, err
	}
	absoluteWorkDir, err := filepath.Abs(strings.TrimSpace(workDir))
	if err != nil {
		return ProjectScope{}, fmt.Errorf("resolve project work directory: %w", err)
	}
	return resolveProjectScope(userDirs, absoluteWorkDir)
}

func resolveProjectScope(userDirs Dirs, absoluteWorkDir string) (ProjectScope, error) {
	root := ProjectRoot(absoluteWorkDir)
	same, err := pathutil.Same(root, userDirs.Root)
	if err != nil {
		return ProjectScope{}, fmt.Errorf("compare user and project roots: %w", err)
	}
	return ProjectScope{
		Root:      root,
		Config:    filepath.Join(root, ConfigFileName),
		Skills:    filepath.Join(root, SkillsDir),
		Available: !same,
	}, nil
}
