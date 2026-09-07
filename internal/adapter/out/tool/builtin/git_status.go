package builtin

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/projectTHORN/proton/internal/core/tool"
	"github.com/projectTHORN/proton/internal/core/workspace"
	"github.com/projectTHORN/proton/internal/platform/sandbox"
)

const (
	maxGitStatusBytes       = 1 * 1024 * 1024
	maxGitStatusStderrBytes = 64 * 1024
)

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
		Kind:                tool.KindForName("git_status"),
		Mutability:          tool.MutabilityReadOnly,
		Safety:              tool.SafetyContract{MutationDomain: tool.MutationDomainNone, MutationSafety: tool.MutationSafetyNone, CheckpointPolicy: tool.CheckpointPolicyNone, Boundary: tool.BoundaryPolicyWorkspaceRead},
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

	execCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	shellCmd := "git " + quoteGitArgs(arguments)
	command, err := h.launcher.Command(execCtx, h.workspace.Root(), shellCmd)
	if err != nil {
		return tool.Result{}, err
	}
	stdout := &boundedBuffer{limit: maxGitStatusBytes, onLimit: cancel}
	stderr := &boundedBuffer{limit: maxGitStatusStderrBytes}
	command.Stdout = stdout
	command.Stderr = stderr
	err = command.Run()
	if stdout.IsTruncated() {
		return tool.Result{}, tool.WrapToolError(
			tool.ErrorCodeOutputTooLarge,
			errGitStatusOutputLimit.Error(),
			errGitStatusOutputLimit,
		)
	}
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return tool.Result{}, fmt.Errorf("git status canceled: %w", ctxErr)
		}
		var exitError *exec.ExitError
		if errors.As(err, &exitError) {
			message := strings.TrimSpace(stderr.String())
			if stderr.IsTruncated() {
				message += " [stderr truncated]"
			}
			if message != "" {
				return tool.Result{}, fmt.Errorf("git status exited with code %d: %s", exitError.ExitCode(), message)
			}
			return tool.Result{}, fmt.Errorf("git status exited with code %d", exitError.ExitCode())
		}
		return tool.Result{}, fmt.Errorf("run git status: %w", err)
	}

	return tool.Result{
		CallID:   call.ID,
		ToolName: call.Name,
		Output:   stdout.String(),
	}, nil
}

func quoteGitArgs(args []string) string {
	quoted := make([]string, len(args))
	for i, arg := range args {
		quoted[i] = "'" + strings.ReplaceAll(arg, "'", "'\\''") + "'"
	}
	return strings.Join(quoted, " ")
}
