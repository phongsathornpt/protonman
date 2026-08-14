// Package config loads layered Proton configuration files.
package config

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/pelletier/go-toml/v2"

	"github.com/projectTHORN/proton/internal/domain/permission"
)

const (
	userConfigRelativePath    = ".proton/config.toml"
	projectConfigRelativePath = ".proton/config.toml"
)

// Options controls which configuration layers are considered.
type Options struct {
	// HomeDir is the user's home directory. An empty value resolves through
	// os.UserHomeDir.
	HomeDir string
	// WorkDir is the workspace directory. An empty value resolves through
	// os.Getwd.
	WorkDir string
	// ProjectTrusted allows project-local config to contribute rules. An
	// untrusted project file is detected and reported but never loaded.
	ProjectTrusted bool
}

// Snapshot is the effective configuration after layered loading.
type Snapshot struct {
	// Permission is the static permission policy configuration.
	Permission permission.Config
	// Mode is the configured initial permission mode.
	Mode permission.Mode
	// Sources lists files that were loaded successfully.
	Sources []string
	// Warnings reports safe skips, such as an untrusted project config.
	Warnings []string
}

type fileDocument struct {
	Permission filePermission `toml:"permission"`
	UI         fileUI         `toml:"ui"`
}

type filePermission struct {
	Default string     `toml:"default"`
	Rules   []fileRule `toml:"rules"`
}

type fileRule struct {
	Action      string `toml:"action"`
	Tool        string `toml:"tool"`
	Pattern     string `toml:"pattern"`
	PatternMode string `toml:"pattern_mode"`
}

type fileUI struct {
	PermissionMode string `toml:"permission_mode"`
}

// Load reads user config and, when trusted, project config.
func Load(ctx context.Context, options Options) (Snapshot, error) {
	homeDir := options.HomeDir
	if homeDir == "" {
		resolvedHome, err := os.UserHomeDir()
		if err != nil {
			return Snapshot{}, fmt.Errorf("resolve home directory: %w", err)
		}
		homeDir = resolvedHome
	}
	workDir := options.WorkDir
	if workDir == "" {
		resolvedWorkDir, err := os.Getwd()
		if err != nil {
			return Snapshot{}, fmt.Errorf("resolve work directory: %w", err)
		}
		workDir = resolvedWorkDir
	}

	if err := validateDirectory(homeDir, "home"); err != nil {
		return Snapshot{}, err
	}
	if err := validateDirectory(workDir, "work"); err != nil {
		return Snapshot{}, err
	}

	snapshot := Snapshot{
		Permission: permission.Config{
			Rules:   make([]permission.Rule, 0),
			Default: permission.ActionAsk,
		},
		Mode:     permission.ModeAsk,
		Sources:  make([]string, 0, 2),
		Warnings: make([]string, 0),
	}

	userPath := filepath.Join(homeDir, userConfigRelativePath)
	if err := loadFile(ctx, userPath, &snapshot, false); err != nil {
		return Snapshot{}, err
	}

	projectPath := filepath.Join(workDir, projectConfigRelativePath)
	if !options.ProjectTrusted {
		exists, err := fileExists(projectPath)
		if err != nil {
			return Snapshot{}, fmt.Errorf("inspect project config: %w", err)
		}
		if exists {
			snapshot.Warnings = append(snapshot.Warnings,
				"ignored untrusted project config: "+projectPath,
			)
		}
		return snapshot, nil
	}
	if err := loadFile(ctx, projectPath, &snapshot, true); err != nil {
		return Snapshot{}, err
	}
	return snapshot, nil
}

func loadFile(ctx context.Context, path string, snapshot *Snapshot, project bool) (loadErr error) {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("before loading %s: %w", path, err)
	}
	file, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("open config %s: %w", path, err)
	}
	defer func() {
		if closeErr := file.Close(); closeErr != nil && loadErr == nil {
			loadErr = fmt.Errorf("close config %s: %w", path, closeErr)
		}
	}()

	var document fileDocument
	if err := toml.NewDecoder(file).Decode(&document); err != nil {
		return fmt.Errorf("decode config %s: %w", path, err)
	}
	if err := mergeDocument(document, snapshot); err != nil {
		return fmt.Errorf("apply config %s: %w", path, err)
	}
	snapshot.Sources = append(snapshot.Sources, path)
	if project && document.UI.PermissionMode != "" {
		snapshot.Warnings = append(snapshot.Warnings,
			"project permission mode is active because project config is trusted: "+path,
		)
	}
	return nil
}

func mergeDocument(document fileDocument, snapshot *Snapshot) error {
	if document.Permission.Default != "" {
		defaultAction, err := permission.ParseAction(document.Permission.Default)
		if err != nil {
			return fmt.Errorf("permission.default: %w", err)
		}
		snapshot.Permission.Default = defaultAction
	}
	for index, rawRule := range document.Permission.Rules {
		rule, err := decodeRule(rawRule)
		if err != nil {
			return fmt.Errorf("permission.rules[%d]: %w", index, err)
		}
		snapshot.Permission.Rules = append(snapshot.Permission.Rules, rule)
	}
	if document.UI.PermissionMode != "" {
		mode, err := permission.ParseMode(document.UI.PermissionMode)
		if err != nil {
			return fmt.Errorf("ui.permission_mode: %w", err)
		}
		snapshot.Mode = mode
	}
	return nil
}

func decodeRule(raw fileRule) (permission.Rule, error) {
	// Grok defaults omitted rule actions to deny. Keeping that default avoids
	// turning a partially written rule into an accidental allow.
	action := permission.ActionDeny
	if strings.TrimSpace(raw.Action) != "" {
		parsedAction, err := permission.ParseAction(raw.Action)
		if err != nil {
			return permission.Rule{}, fmt.Errorf("action: %w", err)
		}
		action = parsedAction
	}
	toolKind, err := permission.ParseToolKind(raw.Tool)
	if err != nil {
		return permission.Rule{}, fmt.Errorf("tool: %w", err)
	}
	patternMode, err := permission.ParsePatternMode(raw.PatternMode)
	if err != nil {
		return permission.Rule{}, fmt.Errorf("pattern_mode: %w", err)
	}
	return permission.Rule{
		Action:      action,
		Tool:        toolKind,
		Pattern:     raw.Pattern,
		PatternMode: patternMode,
	}, nil
}

func fileExists(path string) (bool, error) {
	_, err := os.Stat(path)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

func validateDirectory(path string, label string) error {
	if strings.TrimSpace(path) == "" {
		return fmt.Errorf("%s directory is required", label)
	}
	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("stat %s directory %q: %w", label, path, err)
	}
	if !info.IsDir() {
		return fmt.Errorf("%s path %q is not a directory", label, path)
	}
	return nil
}
