// Package appdirs centralizes Protonman filesystem layout and home resolution.
package appdirs

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/phongsathornpt/protonman/internal/base/envconfig"
)

const (
	RootDirName       = ".protonman"
	LegacyRootDirName = ".proton"
	ConfigFileName    = "config.toml"
	SessionsDir       = "sessions"
	CheckpointsDir    = "checkpoints"
	SkillsDir         = "skills"
	LogsDir           = "logs"
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

// Resolve returns Protonman directories using explicitHome, PROTONMAN_HOME,
// legacy PROTON_HOME, or os.UserHomeDir. Existing .protonman data wins over
// .proton; when only legacy data exists it remains in place.
func Resolve(explicitHome string) (Dirs, error) {
	home := strings.TrimSpace(explicitHome)
	preferLegacy := false
	if home == "" {
		if configured := envconfig.DirectValue(envconfig.Home); configured != "" {
			home = configured
		} else if legacy := envconfig.DirectValue(envconfig.LegacyHome); legacy != "" {
			home = legacy
			preferLegacy = true
		}
	} else if envconfig.DirectValue(envconfig.Home) == "" {
		legacy := envconfig.DirectValue(envconfig.LegacyHome)
		preferLegacy = legacy != "" && samePath(home, legacy)
	}
	if home == "" {
		resolved, err := os.UserHomeDir()
		if err != nil {
			return Dirs{}, fmt.Errorf("resolve home directory: %w", err)
		}
		home = resolved
	}
	root := resolvedUserRoot(home, preferLegacy)
	return dirsForRoot(home, root), nil
}

func resolvedUserRoot(home string, preferLegacy bool) string {
	canonical := filepath.Join(home, RootDirName)
	legacy := filepath.Join(home, LegacyRootDirName)
	if pathEntryExists(canonical) {
		return canonical
	}
	if pathEntryExists(legacy) {
		return legacy
	}
	if preferLegacy {
		return legacy
	}
	return canonical
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

func samePath(left, right string) bool {
	leftAbs, leftErr := filepath.Abs(left)
	rightAbs, rightErr := filepath.Abs(right)
	if leftErr == nil && rightErr == nil {
		return filepath.Clean(leftAbs) == filepath.Clean(rightAbs)
	}
	return filepath.Clean(left) == filepath.Clean(right)
}

func pathEntryExists(path string) bool {
	_, err := os.Lstat(path)
	return err == nil || !os.IsNotExist(err)
}

func displayPath(path string) string {
	if envconfig.DirectValue(envconfig.Home) != "" || envconfig.DirectValue(envconfig.LegacyHome) != "" {
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

// ProjectRoot returns the canonical project-local Protonman directory for new data.
func ProjectRoot(workDir string) string { return filepath.Join(workDir, RootDirName) }

// LegacyProjectRoot returns the legacy project-local Proton directory.
func LegacyProjectRoot(workDir string) string { return filepath.Join(workDir, LegacyRootDirName) }

// ResolvedProjectRoot returns .protonman when present, otherwise legacy .proton
// when present, and defaults to .protonman for new project data.
func ResolvedProjectRoot(workDir string) string {
	canonical := ProjectRoot(workDir)
	if pathEntryExists(canonical) {
		return canonical
	}
	legacy := LegacyProjectRoot(workDir)
	if pathEntryExists(legacy) {
		return legacy
	}
	return canonical
}

// ProjectConfig returns the canonical project-local config path for new data.
func ProjectConfig(workDir string) string { return filepath.Join(ProjectRoot(workDir), ConfigFileName) }

// ResolvedProjectConfig returns the effective project-local config path.
func ResolvedProjectConfig(workDir string) string {
	return filepath.Join(ResolvedProjectRoot(workDir), ConfigFileName)
}

// ProjectSkills returns the canonical project-local skills directory for new data.
func ProjectSkills(workDir string) string { return filepath.Join(ProjectRoot(workDir), SkillsDir) }

// ResolvedProjectSkills returns the effective project-local skills directory.
func ResolvedProjectSkills(workDir string) string {
	return filepath.Join(ResolvedProjectRoot(workDir), SkillsDir)
}
