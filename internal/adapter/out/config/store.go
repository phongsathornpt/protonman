package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
)

func decodeDocument(data []byte, path, label string) (fileDocument, error) {
	var doc fileDocument
	trimmed := strings.TrimSpace(string(data))
	if strings.HasSuffix(path, ".json") || strings.HasPrefix(trimmed, "{") {
		if err := json.Unmarshal(data, &doc); err != nil {
			return fileDocument{}, fmt.Errorf("decode existing %s %q: %w", label, path, err)
		}
		return doc, nil
	}
	if err := decodeTOML(data, &doc); err != nil {
		if jsonErr := json.Unmarshal(data, &doc); jsonErr == nil {
			return doc, nil
		}
		return fileDocument{}, fmt.Errorf("decode existing %s %q: %w", label, path, err)
	}
	return doc, nil
}

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
	doc, err := decodeDocument(data, path, label)
	if err != nil {
		return fileDocument{}, false, err
	}
	return doc, true, nil
}

func writeDocumentAtomic(dir, path, label string, mode os.FileMode, doc fileDocument) error {
	encoded, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return fmt.Errorf("encode %s json: %w", label, err)
	}
	encoded = append(encoded, '\n')
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
