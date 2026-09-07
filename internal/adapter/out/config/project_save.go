package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/pelletier/go-toml/v2"

	"github.com/projectTHORN/proton/internal/app/appdirs"
	"github.com/projectTHORN/proton/internal/core/permission"
	sdk "github.com/projectTHORN/proton/proton-sdk"
)

// SaveProjectAgentProfile updates the project-local agent profile.
func SaveProjectAgentProfile(workDir, profile string) error {
	profile = strings.TrimSpace(profile)
	if profile == "" {
		return fmt.Errorf("agent profile must not be empty")
	}
	return modifyProjectConfigFile(workDir, func(doc *fileDocument) {
		doc.Agent.Profile = &profile
	})
}

// SaveProjectSubagentsEnabled updates the project-local subagent capability switch.
func SaveProjectSubagentsEnabled(workDir string, enabled bool) error {
	return modifyProjectConfigFile(workDir, func(doc *fileDocument) {
		doc.Agent.SubagentsEnabled = &enabled
	})
}

// SaveProjectReasoningEffort updates the project-local reasoning preference.
func SaveProjectReasoningEffort(workDir string, effort sdk.ReasoningEffort) error {
	if !effort.Valid() {
		return fmt.Errorf("invalid reasoning effort %q", effort)
	}
	value := string(effort)
	if effort == sdk.ReasoningDefault {
		value = "auto"
	}
	return modifyProjectConfigFile(workDir, func(doc *fileDocument) {
		doc.Agent.ReasoningEffort = &value
	})
}

// SaveProjectMaxToolCalls updates the project-local cumulative tool-call limit.
func SaveProjectMaxToolCalls(workDir string, maxToolCalls int) error {
	if maxToolCalls < 0 {
		return fmt.Errorf("max tool calls cannot be negative")
	}
	return modifyProjectConfigFile(workDir, func(doc *fileDocument) {
		doc.Agent.MaxToolCalls = &maxToolCalls
	})
}

// SaveProjectPermissionMode updates the project-local initial permission mode.
func SaveProjectPermissionMode(workDir string, mode permission.Mode) error {
	if _, err := permission.ParseMode(mode.String()); err != nil {
		return err
	}
	value := mode.String()
	return modifyProjectConfigFile(workDir, func(doc *fileDocument) {
		doc.UI.PermissionMode = value
	})
}

func modifyProjectConfigFile(workDir string, mutate func(*fileDocument)) error {
	absWorkDir, err := filepath.Abs(strings.TrimSpace(workDir))
	if err != nil {
		return fmt.Errorf("resolve project work directory: %w", err)
	}
	root := appdirs.ProjectRoot(absWorkDir)
	if info, statErr := os.Lstat(root); statErr == nil {
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("refusing project config write through symlink: %s", root)
		}
		if !info.IsDir() {
			return fmt.Errorf("project proton path is not a directory: %s", root)
		}
	} else if errors.Is(statErr, os.ErrNotExist) {
		if err := os.Mkdir(root, 0o755); err != nil && !errors.Is(err, os.ErrExist) {
			return fmt.Errorf("create project proton directory: %w", err)
		}
	} else {
		return fmt.Errorf("inspect project proton directory: %w", statErr)
	}

	path := appdirs.ProjectConfig(absWorkDir)
	var doc fileDocument
	if info, statErr := os.Lstat(path); statErr == nil {
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("refusing project config write through symlink: %s", path)
		}
		data, readErr := os.ReadFile(path)
		if readErr != nil {
			return fmt.Errorf("read project config %q: %w", path, readErr)
		}
		if err := toml.Unmarshal(data, &doc); err != nil {
			return fmt.Errorf("decode existing project config %q: %w", path, err)
		}
	} else if !errors.Is(statErr, os.ErrNotExist) {
		return fmt.Errorf("inspect project config %q: %w", path, statErr)
	}

	mutate(&doc)
	encoded, err := toml.Marshal(doc)
	if err != nil {
		return fmt.Errorf("encode project config toml: %w", err)
	}
	temp, err := os.CreateTemp(root, ".config-*.tmp")
	if err != nil {
		return fmt.Errorf("create temporary project config: %w", err)
	}
	tempPath := temp.Name()
	defer func() { _ = os.Remove(tempPath) }()
	if _, err := temp.Write(encoded); err != nil {
		_ = temp.Close()
		return fmt.Errorf("write temporary project config: %w", err)
	}
	if err := temp.Chmod(0o644); err != nil {
		_ = temp.Close()
		return fmt.Errorf("set project config permissions: %w", err)
	}
	if err := temp.Close(); err != nil {
		return fmt.Errorf("close temporary project config: %w", err)
	}
	if err := os.Rename(tempPath, path); err != nil {
		return fmt.Errorf("persist project config: %w", err)
	}
	return nil
}
