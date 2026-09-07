package prompt

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

const MaxProjectInstructionsBytes = 32 * 1024

// LoadProjectInstructions returns the workspace-level agent instructions.
// AGENTS.override.md takes precedence over AGENTS.md when both exist.
func LoadProjectInstructions(workspace string) (string, error) {
	root := strings.TrimSpace(workspace)
	if root == "" {
		return "", nil
	}
	for _, name := range []string{"AGENTS.override.md", "AGENTS.md"} {
		path := filepath.Join(root, name)
		content, found, err := readBoundedInstructionFile(path)
		if err != nil {
			return "", err
		}
		if found {
			return "Source: " + name + "\n" + content, nil
		}
	}
	return "", nil
}

func readBoundedInstructionFile(path string) (string, bool, error) {
	file, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return "", false, nil
	}
	if err != nil {
		return "", false, fmt.Errorf("open project instructions: %w", err)
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, MaxProjectInstructionsBytes+1))
	if err != nil {
		return "", false, fmt.Errorf("read project instructions: %w", err)
	}
	truncated := len(data) > MaxProjectInstructionsBytes
	if truncated {
		data = data[:MaxProjectInstructionsBytes]
	}
	text := strings.TrimSpace(strings.ToValidUTF8(string(data), "�"))
	if truncated {
		text += "\n\n[project instructions truncated by Proton]"
	}
	return text, true, nil
}
