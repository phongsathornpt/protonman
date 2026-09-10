package builtin

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/phongsathornpt/protonman/internal/core/tool"
	"github.com/phongsathornpt/protonman/internal/core/workspace"
	"github.com/phongsathornpt/protonman/internal/platform/sandbox"
)

const (
	maxGitOutputBytes = 1 * 1024 * 1024
	maxGitStderrBytes = 64 * 1024
	maxGitLogEntries  = 200
)

var errGitOutputLimit = errors.New("git output exceeded configured limit")

type gitHandler struct {
	workspace *workspace.Workspace
	launcher  sandbox.Launcher
}

type gitInput struct {
	Action string `json:"action"`
	Path   string `json:"path"`
	Ref    string `json:"ref"`
	Limit  int    `json:"limit"`
}

// NewGit returns the bounded read-only Git inspection capability.
func NewGit(workspaceRoot *workspace.Workspace, launchers ...sandbox.Launcher) tool.Handler {
	var launcher sandbox.Launcher
	if len(launchers) > 0 {
		launcher = launchers[0]
	}
	return gitHandler{workspace: workspaceRoot, launcher: launcher}
}

func (gitHandler) Definition() tool.Definition {
	return tool.Definition{
		Name:                tool.NameGit,
		Description:         "Read-only Git inspection. Use action=status, diff, log, or show to inspect repository state and history.",
		Kind:                tool.KindGit,
		Mutability:          tool.MutabilityReadOnly,
		Safety:              tool.SafetyContract{MutationDomain: tool.MutationDomainNone, MutationSafety: tool.MutationSafetyNone, CheckpointPolicy: tool.CheckpointPolicyNone, Boundary: tool.BoundaryPolicyWorkspaceRead},
		Evidence:            tool.EvidenceWorkspace,
		PermissionDetailKey: "path",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"action": map[string]any{"type": "string", "enum": []string{"status", "diff", "log", "show"}, "description": "Read-only Git operation to perform"},
				"path": map[string]any{
					"type":        "string",
					"description": "Optional workspace-relative path filter",
				},
				"ref": map[string]any{
					"type":        "string",
					"description": "Optional revision/range for diff or log; required revision for show",
				},
				"limit": map[string]any{
					"type":        "integer",
					"minimum":     1,
					"maximum":     maxGitLogEntries,
					"description": "Maximum log entries for action=log; defaults to 50",
				},
			},
			"required":             []string{"action"},
			"additionalProperties": false,
		},
	}
}

func (h gitHandler) Execute(ctx context.Context, call tool.Call) (tool.Result, error) {
	if h.workspace == nil {
		return tool.Result{}, fmt.Errorf("git workspace is required")
	}
	if h.launcher == nil {
		return tool.Result{}, fmt.Errorf("git sandbox launcher is required: configure an explicit sandbox profile (use --sandbox off to opt out)")
	}
	var input gitInput
	if err := json.Unmarshal(call.Arguments, &input); err != nil {
		return tool.Result{}, tool.WrapToolError(tool.ErrorCodeInvalidArguments, "decode git arguments", err)
	}
	input.Action = strings.ToLower(strings.TrimSpace(input.Action))
	if input.Action == "" {
		return tool.Result{}, tool.NewToolError(tool.ErrorCodeInvalidArguments, "git action is required")
	}
	if input.Limit < 0 || input.Limit > maxGitLogEntries {
		return tool.Result{}, tool.NewToolError(tool.ErrorCodeInvalidArguments, "git limit must be between 1 and 200")
	}

	relativePath, err := h.resolvePath(ctx, input.Path)
	if err != nil {
		return tool.Result{}, err
	}
	arguments, err := gitArguments(input, relativePath)
	if err != nil {
		return tool.Result{}, err
	}
	if err := ctx.Err(); err != nil {
		return tool.Result{}, fmt.Errorf("before git %s: %w", input.Action, err)
	}

	execCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	shellCmd := "git " + quoteGitArgs(arguments)
	command, err := h.launcher.Command(execCtx, h.workspace.Root(), shellCmd)
	if err != nil {
		return tool.Result{}, err
	}
	stdout := &boundedBuffer{limit: maxGitOutputBytes, onLimit: cancel}
	stderr := &boundedBuffer{limit: maxGitStderrBytes}
	command.Stdout = stdout
	command.Stderr = stderr
	err = command.Run()
	if stdout.IsTruncated() {
		return tool.Result{}, tool.WrapToolError(tool.ErrorCodeOutputTooLarge, errGitOutputLimit.Error(), errGitOutputLimit)
	}
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return tool.Result{}, fmt.Errorf("git %s canceled: %w", input.Action, ctxErr)
		}
		var exitError *exec.ExitError
		if errors.As(err, &exitError) {
			message := strings.TrimSpace(stderr.String())
			if stderr.IsTruncated() {
				message += " [stderr truncated]"
			}
			if message != "" {
				return tool.Result{}, fmt.Errorf("git %s exited with code %d: %s", input.Action, exitError.ExitCode(), message)
			}
			return tool.Result{}, fmt.Errorf("git %s exited with code %d", input.Action, exitError.ExitCode())
		}
		return tool.Result{}, fmt.Errorf("run git %s: %w", input.Action, err)
	}
	return tool.Result{CallID: call.ID, ToolName: call.Name, Output: stdout.String()}, nil
}

func (h gitHandler) resolvePath(ctx context.Context, path string) (string, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return "", nil
	}
	resolvedPath, err := h.workspace.Resolve(ctx, path)
	if err != nil {
		return "", err
	}
	relativePath, err := filepath.Rel(h.workspace.Root(), resolvedPath)
	if err != nil {
		return "", fmt.Errorf("relative git path: %w", err)
	}
	return filepath.ToSlash(relativePath), nil
}

func gitArguments(input gitInput, relativePath string) ([]string, error) {
	base := []string{"-c", "core.hooksPath=/dev/null", "--no-optional-locks"}
	ref := strings.TrimSpace(input.Ref)
	switch input.Action {
	case "status":
		args := append(base, "status", "--short", "--branch", "--untracked-files=normal")
		if relativePath != "" {
			args = append(args, "--", relativePath)
		}
		return args, nil
	case "diff":
		args := append(base, "diff", "--no-ext-diff", "--no-color")
		if ref != "" {
			args = append(args, ref)
		}
		if relativePath != "" {
			args = append(args, "--", relativePath)
		}
		return args, nil
	case "log":
		limit := input.Limit
		if limit == 0 {
			limit = 50
		}
		args := append(base, "log", "--no-color", "--oneline", "--decorate=no", fmt.Sprintf("-n%d", limit))
		if ref != "" {
			args = append(args, ref)
		}
		if relativePath != "" {
			args = append(args, "--", relativePath)
		}
		return args, nil
	case "show":
		if ref == "" {
			return nil, tool.NewToolError(tool.ErrorCodeInvalidArguments, "git ref is required for action=show")
		}
		args := append(base, "show", "--no-ext-diff", "--no-color", "--decorate=no", ref)
		if relativePath != "" {
			args = append(args, "--", relativePath)
		}
		return args, nil
	default:
		return nil, tool.NewToolError(tool.ErrorCodeInvalidArguments, "git action must be status, diff, log, or show")
	}
}

func quoteGitArgs(args []string) string {
	quoted := make([]string, len(args))
	for i, arg := range args {
		quoted[i] = "'" + strings.ReplaceAll(arg, "'", "'\\''") + "'"
	}
	return strings.Join(quoted, " ")
}
