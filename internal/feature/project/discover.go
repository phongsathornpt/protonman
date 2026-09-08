package project

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/phongsathornpt/protonman/internal/app/appdirs"
)

// Options controls project-local Proton discovery.
type Options struct {
	WorkDir       string
	Trusted       bool
	ConfigSources []string
}

// State describes project-local Proton resources without loading configuration.
type State struct {
	WorkDir      string
	ProtonDir    string
	Exists       bool
	Trusted      bool
	ConfigPath   string
	ConfigExists bool
	ConfigLoaded bool
	SkillsPath   string
	SkillsExists bool
	SkillCount   int
}

// Discover inspects the project-local .proton directory.
func Discover(ctx context.Context, opts Options) (State, error) {
	if err := ctx.Err(); err != nil {
		return State{}, err
	}
	workDir := strings.TrimSpace(opts.WorkDir)
	if workDir == "" {
		resolved, err := os.Getwd()
		if err != nil {
			return State{}, fmt.Errorf("resolve project work directory: %w", err)
		}
		workDir = resolved
	}
	absWorkDir, err := filepath.Abs(workDir)
	if err != nil {
		return State{}, fmt.Errorf("resolve absolute project path: %w", err)
	}
	state := State{
		WorkDir:    absWorkDir,
		ProtonDir:  appdirs.ProjectRoot(absWorkDir),
		Trusted:    opts.Trusted,
		ConfigPath: appdirs.ProjectConfig(absWorkDir),
		SkillsPath: appdirs.ProjectSkills(absWorkDir),
	}
	if state.Exists, err = isDir(state.ProtonDir); err != nil {
		return State{}, fmt.Errorf("inspect project proton directory: %w", err)
	}
	if state.ConfigExists, err = isFile(state.ConfigPath); err != nil {
		return State{}, fmt.Errorf("inspect project config: %w", err)
	}
	state.ConfigLoaded = sourceContains(opts.ConfigSources, state.ConfigPath)
	if state.SkillsExists, err = isDir(state.SkillsPath); err != nil {
		return State{}, fmt.Errorf("inspect project skills: %w", err)
	}
	if state.SkillsExists {
		if state.SkillCount, err = countSkillDirs(ctx, state.SkillsPath); err != nil {
			return State{}, err
		}
	}
	return state, nil
}

func sourceContains(sources []string, target string) bool {
	target = filepath.Clean(target)
	return slices.ContainsFunc(sources, func(source string) bool {
		return filepath.Clean(source) == target
	})
}

func isDir(path string) (bool, error) {
	info, err := os.Stat(path)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return info.IsDir(), nil
}

func isFile(path string) (bool, error) {
	info, err := os.Stat(path)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return !info.IsDir(), nil
}

func countSkillDirs(ctx context.Context, root string) (int, error) {
	entries, err := os.ReadDir(root)
	if err != nil {
		return 0, fmt.Errorf("read project skills: %w", err)
	}
	count := 0
	for _, entry := range entries {
		if err := ctx.Err(); err != nil {
			return 0, err
		}
		if !entry.IsDir() || strings.HasPrefix(entry.Name(), ".") || entry.Name() == "node_modules" {
			continue
		}
		ok, statErr := isFile(filepath.Join(root, entry.Name(), "SKILL.md"))
		if statErr != nil {
			return 0, fmt.Errorf("inspect project skill %q: %w", entry.Name(), statErr)
		}
		if ok {
			count++
		}
	}
	return count, nil
}
