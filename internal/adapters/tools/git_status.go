package tools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/projectTHORN/proton/internal/workspace"
	"github.com/projectTHORN/proton/internal/tool"
)

const maxGitStatusBytes = 1 * 1024 * 1024

var errGitStatusOutputLimit = errors.New("git status output exceeded configured limit")

type gitStatusHandler struct {
	workspace *workspace.Workspace
}

type gitStatusInput struct {
	Path string `json:"path"`
}

// NewGitStatus returns the bounded read-only git status adapter.
func NewGitStatus(workspaceRoot *workspace.Workspace) tool.Handler {
	return gitStatusHandler{workspace: workspaceRoot}
}

func (gitStatusHandler) Definition() tool.Definition {
	return tool.Definition{
		Name:                "git_status",
		Description:         "Show compact git branch and working-tree status.",
		Kind:                tool.KindRead,
		PermissionDetailKey: "path",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"path": map[string]any{
					"type":        "string",
					"description": "Optional workspace-relative path; status is rooted at the workspace",
				},
			},
		},
	}
}

func (h gitStatusHandler) Execute(ctx context.Context, call tool.Call) (tool.Result, error) {
	if h.workspace == nil {
		return tool.Result{}, fmt.Errorf("git_status workspace is required")
	}
	var input gitStatusInput
	if err := json.Unmarshal(call.Arguments, &input); err != nil {
		return tool.Result{}, fmt.Errorf("decode git_status arguments: %w", err)
	}
	statusPath := strings.TrimSpace(input.Path)
	relativePath := ""
	if statusPath != "" {
		resolvedPath, err := h.workspace.Resolve(ctx, statusPath)
		if err != nil {
			return tool.Result{}, err
		}
		relativePath, err = filepath.Rel(h.workspace.Root(), resolvedPath)
		if err != nil {
			return tool.Result{}, fmt.Errorf("relative git status path: %w", err)
		}
		relativePath = filepath.ToSlash(relativePath)
	}
	if err := ctx.Err(); err != nil {
		return tool.Result{}, fmt.Errorf("before git status: %w", err)
	}

	arguments := []string{
		"status",
		"--short",
		"--branch",
		"--untracked-files=normal",
	}
	if relativePath != "" {
		arguments = append(arguments, "--", relativePath)
	}
	command := exec.CommandContext(ctx, "git", arguments...)
	command.Dir = h.workspace.Root()
	output, err := command.Output()
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return tool.Result{}, fmt.Errorf("git status canceled: %w", ctxErr)
		}
		var exitError *exec.ExitError
		if errors.As(err, &exitError) {
			return tool.Result{}, fmt.Errorf("git status exited with code %d", exitError.ExitCode())
		}
		return tool.Result{}, fmt.Errorf("run git status: %w", err)
	}
	if len(output) > maxGitStatusBytes {
		return tool.Result{}, tool.WrapToolError(
			tool.ErrorCodeExecution,
			errGitStatusOutputLimit.Error(),
			errGitStatusOutputLimit,
		)
	}

	return tool.Result{
		CallID:   call.ID,
		ToolName: call.Name,
		Output:   string(output),
	}, nil
}
