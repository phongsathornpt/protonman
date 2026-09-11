package prompt

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/phongsathornpt/protonman/internal/core/workspace"
)

const MaxProjectInstructionsBytes = 32 * 1024

// LoadProjectInstructions returns the workspace-level agent instructions.
// AGENTS.override.md takes precedence over AGENTS.md when both exist.
//
// When policy is non-nil, candidate files are opened through the workspace
// safety boundary (confined root, protected paths, symlink checks) instead of
// the process filesystem directly. A nil policy preserves the legacy direct
// open for contexts without an assembled workspace.
func LoadProjectInstructions(root string) (string, error) {
	return LoadProjectInstructionsWithPolicy(context.Background(), nil, root)
}

// LoadProjectInstructionsWithPolicy is LoadProjectInstructions with an explicit
// workspace policy and cancellation context.
func LoadProjectInstructionsWithPolicy(ctx context.Context, policy *workspace.Workspace, root string) (string, error) {
	trimmed := strings.TrimSpace(root)
	if trimmed == "" {
		return "", nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	for _, name := range []string{"AGENTS.override.md", "AGENTS.md"} {
		path := filepath.Join(trimmed, name)
		content, found, err := readBoundedInstructionFile(ctx, policy, path)
		if err != nil {
			return "", err
		}
		if found {
			return "Source: " + name + "\n" + content, nil
		}
	}
	return "", nil
}

func readBoundedInstructionFile(ctx context.Context, policy *workspace.Workspace, path string) (string, bool, error) {
	file, err := openInstructionFile(ctx, policy, path)
	if errors.Is(err, os.ErrNotExist) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
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
		text += "\n\n[project instructions truncated by Protonman]"
	}
	return text, true, nil
}

// openInstructionFile resolves one candidate through the workspace boundary
// when a policy is available, rejecting symlink escapes, protected paths, and
// paths outside the authorized root before opening.
func openInstructionFile(ctx context.Context, policy *workspace.Workspace, path string) (*os.File, error) {
	if policy == nil {
		file, err := os.Open(path)
		if err != nil {
			return nil, err
		}
		return file, nil
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, fmt.Errorf("resolve project instructions path: %w", err)
	}
	file, err := policy.OpenReadFile(ctx, abs)
	if err != nil {
		return nil, fmt.Errorf("open project instructions: %w", err)
	}
	return file, nil
}
