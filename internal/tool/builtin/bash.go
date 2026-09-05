package builtin

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/projectTHORN/proton/internal/sandbox"
	"github.com/projectTHORN/proton/internal/telemetry"
	"github.com/projectTHORN/proton/internal/tool"
	"github.com/projectTHORN/proton/internal/workspace"
)

const (
	maxBashArgumentBytes = 512 * 1024
	maxBashCommandBytes  = 256 * 1024
	maxBashOutputBytes   = 2 * 1024 * 1024
)

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
	startedAt := time.Now()
	slog.DebugContext(ctx, "bash execution started",
		"call_id", call.ID,
		"tool_name", call.Name,
		"argument_bytes", len(call.Arguments),
	)
	if h.workspace == nil {
		logBashFailure(ctx, call, startedAt, "workspace", errors.New("workspace_missing"))
		return tool.Result{}, fmt.Errorf("bash workspace is required")
	}
	if h.launcher == nil {
		logBashFailure(ctx, call, startedAt, "launcher", errors.New("launcher_missing"))
		return tool.Result{}, fmt.Errorf("bash sandbox launcher is required: configure an explicit sandbox profile (use --sandbox off to opt out)")
	}
	if len(call.Arguments) > maxBashArgumentBytes {
		err := tool.NewToolError(tool.ErrorCodeInvalidArguments, "bash arguments exceed the 512 KiB limit")
		logBashFailure(ctx, call, startedAt, "arguments", err)
		return tool.Result{}, err
	}
	var input bashInput
	if err := json.Unmarshal(call.Arguments, &input); err != nil {
		logBashFailure(ctx, call, startedAt, "arguments", err)
		return tool.Result{}, fmt.Errorf("decode bash arguments: %w", err)
	}
	input.Command = strings.TrimSpace(input.Command)
	if input.Command == "" {
		logBashFailure(ctx, call, startedAt, "arguments", errors.New("command_missing"))
		return tool.Result{}, fmt.Errorf("bash command is required")
	}
	if len(input.Command) > maxBashCommandBytes {
		err := tool.NewToolError(tool.ErrorCodeInvalidArguments, "bash command exceeds the 256 KiB limit")
		logBashFailure(ctx, call, startedAt, "arguments", err)
		return tool.Result{}, err
	}
	slog.DebugContext(ctx, "bash command decoded",
		"call_id", call.ID,
		"command_bytes", len(input.Command),
		"command_fingerprint", telemetry.Fingerprint(input.Command),
	)
	if err := ctx.Err(); err != nil {
		logBashFailure(ctx, call, startedAt, "before_run", err)
		return tool.Result{}, fmt.Errorf("before bash command: %w", err)
	}

	command, err := h.command(ctx, input.Command)
	if err != nil {
		logBashFailure(ctx, call, startedAt, "launcher", err)
		return tool.Result{}, err
	}
	slog.DebugContext(ctx, "bash process starting",
		"call_id", call.ID,
		"executable", filepath.Base(command.Path),
		"argument_count", len(command.Args),
		"workspace_fingerprint", telemetry.Fingerprint(h.workspace.Root()),
	)

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
	attrs := []any{
		"call_id", call.ID,
		"duration_ms", time.Since(startedAt).Milliseconds(),
		"success", err == nil,
		"output_bytes", len(outputStr),
		"truncated", truncated,
		"context_error", ctx.Err() != nil,
	}
	if err != nil {
		attrs = append(attrs, "error_type", fmt.Sprintf("%T", err))
	}
	if ctxErr := ctx.Err(); ctxErr != nil {
		attrs = append(attrs,
			"context_error_type", fmt.Sprintf("%T", ctxErr),
			"deadline_exceeded", errors.Is(ctxErr, context.DeadlineExceeded),
			"canceled", errors.Is(ctxErr, context.Canceled),
		)
	}
	var exitError *exec.ExitError
	if errors.As(err, &exitError) {
		attrs = append(attrs, "exit_code", exitError.ExitCode())
	}
	slog.DebugContext(ctx, "bash process finished", attrs...)
	if err == nil {
		code := 0
		result.ExitCode = &code
		return result, nil
	}

	if errors.As(err, &exitError) {
		code := exitError.ExitCode()
		result.ExitCode = &code
	}
	if ctxErr := ctx.Err(); ctxErr != nil {
		if errors.Is(ctxErr, context.DeadlineExceeded) {
			return result, fmt.Errorf("bash command deadline exceeded: %w", ctxErr)
		}
		return result, fmt.Errorf("bash command canceled: %w", ctxErr)
	}
	return result, fmt.Errorf("bash command failed: %w", err)
}

func logBashFailure(ctx context.Context, call tool.Call, startedAt time.Time, phase string, err error) {
	slog.DebugContext(ctx, "bash execution stopped",
		"call_id", call.ID,
		"tool_name", call.Name,
		"phase", phase,
		"duration_ms", time.Since(startedAt).Milliseconds(),
		"error_type", fmt.Sprintf("%T", err),
		"context_error", ctx.Err() != nil,
	)
}

func (h bashHandler) command(ctx context.Context, command string) (*exec.Cmd, error) {
	if h.launcher == nil {
		return nil, fmt.Errorf("bash sandbox launcher is required: configure an explicit sandbox profile (use --sandbox off to opt out)")
	}
	return h.launcher.Command(ctx, h.workspace.Root(), command)
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
