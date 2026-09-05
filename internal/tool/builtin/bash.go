package builtin

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"runtime"
	"strings"
	"sync"

	"github.com/projectTHORN/proton/internal/sandbox"
	"github.com/projectTHORN/proton/internal/tool"
	"github.com/projectTHORN/proton/internal/workspace"
)

const maxBashOutputBytes = 2 * 1024 * 1024

type bashHandler struct {
	workspace *workspace.Workspace
	launcher  sandbox.Launcher
}

type bashInput struct {
	Command string `json:"command"`
}

// NewBash returns the permission-gated shell command adapter.
func NewBash(workspaceRoot *workspace.Workspace, launchers ...sandbox.Launcher) tool.Handler {
	var launcher sandbox.Launcher
	if len(launchers) > 0 {
		launcher = launchers[0]
	}
	return bashHandler{workspace: workspaceRoot, launcher: launcher}
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

	command, err := h.command(ctx, input.Command)
	if err != nil {
		return tool.Result{}, err
	}

	var buf boundedBuffer
	buf.limit = maxBashOutputBytes
	command.Stdout = &buf
	command.Stderr = &buf

	err = command.Run()
	outputStr := buf.String()
	truncated := buf.IsTruncated()
	if truncated {
		outputStr += "\n[output truncated at 2 MiB]"
	}

	result := tool.Result{
		CallID:    call.ID,
		ToolName:  call.Name,
		Output:    outputStr,
		Truncated: truncated,
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

func (h bashHandler) command(ctx context.Context, command string) (*exec.Cmd, error) {
	if h.launcher != nil {
		return h.launcher.Command(ctx, h.workspace.Root(), command)
	}
	return shellCommand(ctx, h.workspace.Root(), command), nil
}

func shellCommand(ctx context.Context, dir string, command string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, "sh", "-c", command)
	if runtime.GOOS == "windows" {
		cmd = exec.CommandContext(ctx, "cmd.exe", "/C", command)
	}
	cmd.Dir = dir
	return cmd
}

type boundedBuffer struct {
	mu        sync.Mutex
	buf       bytes.Buffer
	limit     int
	truncated bool
}

func (b *boundedBuffer) Write(p []byte) (n int, err error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.buf.Len() >= b.limit {
		b.truncated = true
		return len(p), nil
	}
	remaining := b.limit - b.buf.Len()
	if len(p) > remaining {
		b.buf.Write(p[:remaining])
		b.truncated = true
		return len(p), nil
	}
	return b.buf.Write(p)
}

func (b *boundedBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

func (b *boundedBuffer) IsTruncated() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.truncated
}
