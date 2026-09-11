package config

import (
	"errors"
	"fmt"
	"os"

	"github.com/pelletier/go-toml/v2"
)

func readDocument(path, label string, rejectSymlink bool) (fileDocument, bool, error) {
	if rejectSymlink {
		info, err := os.Lstat(path)
		if errors.Is(err, os.ErrNotExist) {
			return fileDocument{}, false, nil
		}
		if err != nil {
			return fileDocument{}, false, fmt.Errorf("inspect %s %q: %w", label, path, err)
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fileDocument{}, false, fmt.Errorf("refusing %s write through symlink: %s", label, path)
		}
	}
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return fileDocument{}, false, nil
	}
	if err != nil {
		return fileDocument{}, false, fmt.Errorf("read %s %q: %w", label, path, err)
	}
	var doc fileDocument
	if err := toml.Unmarshal(data, &doc); err != nil {
		return fileDocument{}, false, fmt.Errorf("decode existing %s %q: %w", label, path, err)
	}
	return doc, true, nil
}

func writeDocumentAtomic(dir, path, label string, mode os.FileMode, doc fileDocument) error {
	encoded, err := toml.Marshal(doc)
	if err != nil {
		return fmt.Errorf("encode %s toml: %w", label, err)
	}
	temp, err := os.CreateTemp(dir, ".config-*.tmp")
	if err != nil {
		return fmt.Errorf("create temporary %s: %w", label, err)
	}
	tempPath := temp.Name()
	defer func() { _ = os.Remove(tempPath) }()
	if _, err := temp.Write(encoded); err != nil {
		_ = temp.Close()
		return fmt.Errorf("write temporary %s: %w", label, err)
	}
	if err := temp.Chmod(mode); err != nil {
		_ = temp.Close()
		return fmt.Errorf("set %s permissions: %w", label, err)
	}
	if err := temp.Close(); err != nil {
		return fmt.Errorf("close temporary %s: %w", label, err)
	}
	if err := os.Rename(tempPath, path); err != nil {
		return fmt.Errorf("persist %s: %w", label, err)
	}
	return nil
}
