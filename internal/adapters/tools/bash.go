package tools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"runtime"
	"strings"

	"github.com/projectTHORN/proton/internal/adapters/workspace"
	"github.com/projectTHORN/proton/internal/domain/tool"
)

type bashHandler struct {
	workspace *workspace.Workspace
}

type bashInput struct {
	Command string `json:"command"`
}

// NewBash returns the permission-gated shell command adapter.
func NewBash(workspaceRoot *workspace.Workspace) tool.Handler {
	return bashHandler{workspace: workspaceRoot}
}

func (bashHandler) Definition() tool.Definition {
	return tool.Definition{
		Name:                "bash",
		Description:         "Run a shell command in the current workspace.",
		Kind:                tool.KindBash,
		PermissionDetailKey: "command",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"command": map[string]any{
					"type":        "string",
					"description": "Shell command to execute",
				},
			},
			"required": []string{"command"},
		},
	}
}

func (h bashHandler) Execute(ctx context.Context, call tool.Call) (tool.Result, error) {
	if h.workspace == nil {
		return tool.Result{}, fmt.Errorf("bash workspace is required")
	}
	var input bashInput
	if err := json.Unmarshal(call.Arguments, &input); err != nil {
		return tool.Result{}, fmt.Errorf("decode bash arguments: %w", err)
	}
	input.Command = strings.TrimSpace(input.Command)
	if input.Command == "" {
		return tool.Result{}, fmt.Errorf("bash command is required")
	}
	if err := ctx.Err(); err != nil {
		return tool.Result{}, fmt.Errorf("before bash command: %w", err)
	}

	command := shellCommand(ctx, input.Command)
	command.Dir = h.workspace.Root()
	output, err := command.CombinedOutput()
	result := tool.Result{
		CallID:   call.ID,
		ToolName: call.Name,
		Output:   string(output),
	}
	if err == nil {
		code := 0
		result.ExitCode = &code
		return result, nil
	}

	var exitError *exec.ExitError
	if errors.As(err, &exitError) {
		code := exitError.ExitCode()
		result.ExitCode = &code
	}
	if ctxErr := ctx.Err(); ctxErr != nil {
		return result, fmt.Errorf("bash command canceled: %w", ctxErr)
	}
	return result, fmt.Errorf("bash command failed: %w", err)
}

func shellCommand(ctx context.Context, command string) *exec.Cmd {
	if runtime.GOOS == "windows" {
		return exec.CommandContext(ctx, "cmd.exe", "/C", command)
	}
	return exec.CommandContext(ctx, "sh", "-c", command)
}
