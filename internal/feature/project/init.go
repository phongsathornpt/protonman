package project

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/phongsathornpt/proton/internal/app/appdirs"
)

// InitResult reports whether project-local Proton configuration was created.
type InitResult struct {
	ProtonDir  string
	ConfigPath string
	Created    bool
}

// Init creates a minimal project-local Proton configuration without overwriting
// an existing config file or following an existing .proton symlink.
func Init(ctx context.Context, workDir string) (InitResult, error) {
	if err := ctx.Err(); err != nil {
		return InitResult{}, err
	}
	absWorkDir, err := filepath.Abs(workDir)
	if err != nil {
		return InitResult{}, fmt.Errorf("resolve absolute project path: %w", err)
	}
	root := appdirs.ProjectRoot(absWorkDir)
	configPath := appdirs.ProjectConfig(absWorkDir)
	result := InitResult{ProtonDir: root, ConfigPath: configPath}

	if info, statErr := os.Lstat(root); statErr == nil {
		if info.Mode()&os.ModeSymlink != 0 {
			return result, fmt.Errorf("refusing project init through symlink: %s", root)
		}
		if !info.IsDir() {
			return result, fmt.Errorf("project proton path is not a directory: %s", root)
		}
	} else if !errors.Is(statErr, os.ErrNotExist) {
		return result, fmt.Errorf("inspect project proton directory: %w", statErr)
	} else if mkdirErr := os.Mkdir(root, 0o755); mkdirErr != nil && !errors.Is(mkdirErr, os.ErrExist) {
		return result, fmt.Errorf("create project proton directory: %w", mkdirErr)
	}

	if err := ctx.Err(); err != nil {
		return result, err
	}
	file, err := os.OpenFile(configPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if errors.Is(err, os.ErrExist) {
		return result, nil
	}
	if err != nil {
		return result, fmt.Errorf("create project config: %w", err)
	}
	const initialConfig = "# Proton project configuration\n"
	if _, err := file.WriteString(initialConfig); err != nil {
		_ = file.Close()
		_ = os.Remove(configPath)
		return result, fmt.Errorf("write project config: %w", err)
	}
	if err := file.Close(); err != nil {
		_ = os.Remove(configPath)
		return result, fmt.Errorf("close project config: %w", err)
	}
	result.Created = true
	return result, nil
}
