package builtin

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/projectTHORN/proton/internal/sandbox"
	"github.com/projectTHORN/proton/internal/tool"
	"github.com/projectTHORN/proton/internal/workspace"
)

const maxGitStatusBytes = 1 * 1024 * 1024

var errGitStatusOutputLimit = errors.New("git status output exceeded configured limit")

type gitStatusHandler struct {
	workspace *workspace.Workspace
	launcher  sandbox.Launcher
}

type gitStatusInput struct {
	Path string `json:"path"`
}

// NewGitStatus returns the bounded read-only git status adapter.
func NewGitStatus(workspaceRoot *workspace.Workspace, launchers ...sandbox.Launcher) tool.Handler {
	var launcher sandbox.Launcher
	if len(launchers) > 0 {
		launcher = launchers[0]
	}
	return gitStatusHandler{workspace: workspaceRoot, launcher: launcher}
}

func (gitStatusHandler) Definition() tool.Definition {
	return tool.Definition{
		Name:                "git_status",
		Description:         "Show compact git branch and working-tree status.",
		Kind:                tool.KindRead,
		Mutability:          tool.MutabilityReadOnly,
		Evidence:            tool.EvidenceWorkspace,
		PermissionDetailKey: "path",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"path": map[string]any{
					"type":        "string",
					"description": "Optional workspace-relative path; status is rooted at the workspace",
				},
			},
			"additionalProperties": false,
		},
	}
}

func (h gitStatusHandler) Execute(ctx context.Context, call tool.Call) (tool.Result, error) {
	if h.workspace == nil {
		return tool.Result{}, fmt.Errorf("git_status workspace is required")
	}
	if h.launcher == nil {
		return tool.Result{}, fmt.Errorf("git_status sandbox launcher is required: configure an explicit sandbox profile (use --sandbox off to opt out)")
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
		"-c", "core.hooksPath=/dev/null",
		"--no-optional-locks",
		"status",
		"--short",
		"--branch",
		"--untracked-files=normal",
	}
	if relativePath != "" {
		arguments = append(arguments, "--", relativePath)
	}

	var command *exec.Cmd
	shellCmd := "git " + quoteGitArgs(arguments)
	var err error
	command, err = h.launcher.Command(ctx, h.workspace.Root(), shellCmd)
	if err != nil {
		return tool.Result{}, err
	}
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

func quoteGitArgs(args []string) string {
	quoted := make([]string, len(args))
	for i, arg := range args {
		quoted[i] = "'" + strings.ReplaceAll(arg, "'", "'\\''") + "'"
	}
	return strings.Join(quoted, " ")
}
