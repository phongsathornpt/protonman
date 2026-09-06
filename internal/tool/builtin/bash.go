package builtin

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/projectTHORN/proton/internal/checkpoint"
	"github.com/projectTHORN/proton/internal/sandbox"
	"github.com/projectTHORN/proton/internal/telemetry"
	"github.com/projectTHORN/proton/internal/tool"
	"github.com/projectTHORN/proton/internal/workspace"
)

const (
	maxBashArgumentBytes  = 512 * 1024
	maxBashCommandBytes   = 256 * 1024
	maxBashOutputBytes    = 2 * 1024 * 1024
	maxBashStreamBytes    = maxBashOutputBytes / 2
	maxBashTimeoutSeconds = 120
)

type bashHandler struct {
	workspace   *workspace.Workspace
	launcher    sandbox.Launcher
	checkpoints checkpoint.Store
}

type bashInput struct {
	Command        string `json:"command"`
	Cwd            string `json:"cwd,omitempty"`
	TimeoutSeconds int64  `json:"timeout_seconds,omitempty"`
}

// NewBash returns the permission-gated shell command adapter.
func NewBash(workspaceRoot *workspace.Workspace, launchers ...sandbox.Launcher) tool.Handler {
	var launcher sandbox.Launcher
	if len(launchers) > 0 {
		launcher = launchers[0]
	}
	return bashHandler{workspace: workspaceRoot, launcher: launcher}
}

// NewBashWithCheckpoint returns a shell adapter that checkpoints proven file mutations before execution.
func NewBashWithCheckpoint(workspaceRoot *workspace.Workspace, launcher sandbox.Launcher, store checkpoint.Store) tool.Handler {
	return bashHandler{workspace: workspaceRoot, launcher: launcher, checkpoints: store}
}

func (bashHandler) Definition() tool.Definition {
	return tool.Definition{
		Name:                "bash",
		Description:         "Run a shell command in the current workspace.",
		Kind:                tool.KindBash,
		Mutability:          tool.MutabilityMutating,
		PermissionDetailKey: "command",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"command": map[string]any{
					"type":        "string",
					"description": "Shell command to execute",
				},
				"cwd": map[string]any{
					"type":        "string",
					"description": "Optional workspace-relative working directory",
				},
				"timeout_seconds": map[string]any{
					"type": "integer", "minimum": 1, "maximum": maxBashTimeoutSeconds,
					"description": "Optional shorter execution timeout; cannot extend the caller deadline",
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
		err := tool.NewToolError(tool.ErrorCodeSandboxUnavailable, "bash sandbox launcher is required: configure an explicit sandbox profile (use --sandbox off to opt out)")
		logBashFailure(ctx, call, startedAt, "launcher", err)
		return tool.Result{}, err
	}
	if len(call.Arguments) > maxBashArgumentBytes {
		err := tool.NewToolError(tool.ErrorCodeInvalidArguments, "bash arguments exceed the 512 KiB limit")
		logBashFailure(ctx, call, startedAt, "arguments", err)
		return tool.Result{}, err
	}
	var input bashInput
	if err := json.Unmarshal(call.Arguments, &input); err != nil {
		logBashFailure(ctx, call, startedAt, "arguments", err)
		return tool.Result{}, tool.WrapToolError(tool.ErrorCodeInvalidArguments, "decode bash arguments", err)
	}
	input.Command = strings.TrimSpace(input.Command)
	if input.Command == "" {
		err := tool.NewToolError(tool.ErrorCodeInvalidArguments, "bash command is required")
		logBashFailure(ctx, call, startedAt, "arguments", err)
		return tool.Result{}, err
	}
	if len(input.Command) > maxBashCommandBytes {
		err := tool.NewToolError(tool.ErrorCodeInvalidArguments, "bash command exceeds the 256 KiB limit")
		logBashFailure(ctx, call, startedAt, "arguments", err)
		return tool.Result{}, err
	}
	if input.TimeoutSeconds < 0 || input.TimeoutSeconds > maxBashTimeoutSeconds {
		err := tool.NewToolError(tool.ErrorCodeInvalidArguments, "bash timeout_seconds must be between 1 and 120 when provided")
		logBashFailure(ctx, call, startedAt, "arguments", err)
		return tool.Result{}, err
	}
	cwd, cwdRel, err := h.resolveCwd(ctx, input.Cwd)
	if err != nil {
		logBashFailure(ctx, call, startedAt, "cwd", err)
		return tool.Result{}, err
	}
	if input.TimeoutSeconds > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, time.Duration(input.TimeoutSeconds)*time.Second)
		defer cancel()
	}
	analysis := tool.AnalyzeCommand(input.Command)
	affectedPaths := bashAffectedPaths(cwdRel, analysis.AffectedPaths)
	resolvedMutationPaths := make([]string, 0, len(affectedPaths))
	checkpointID := ""
	if analysis.Effect == tool.CommandEffectMutating {
		for _, path := range affectedPaths {
			resolved, resolveErr := h.workspace.Resolve(ctx, path)
			if resolveErr != nil {
				logBashFailure(ctx, call, startedAt, "mutation_guard", resolveErr)
				return tool.Result{}, resolveErr
			}
			resolvedMutationPaths = append(resolvedMutationPaths, resolved)
		}
		if guardErr := h.workspace.GuardWholeFileMutation(ctx, resolvedMutationPaths...); guardErr != nil {
			logBashFailure(ctx, call, startedAt, "mutation_guard", guardErr)
			return tool.Result{}, guardErr
		}
		if len(resolvedMutationPaths) > 0 && h.checkpoints != nil {
			checkpointID, err = h.checkpoints.Capture(ctx, resolvedMutationPaths)
			if err != nil {
				checkpointErr := fmt.Errorf("checkpoint bash mutation: %w", err)
				logBashFailure(ctx, call, startedAt, "checkpoint", checkpointErr)
				return tool.Result{}, checkpointErr
			}
		}
	}
	slog.DebugContext(ctx, "bash command decoded",
		"call_id", call.ID,
		"command_bytes", len(input.Command),
		"command_fingerprint", telemetry.Fingerprint(input.Command),
		"effect", analysis.Effect,
		"effect_confidence", analysis.Confidence,
		"affected_path_count", len(affectedPaths),
		"cwd_fingerprint", telemetry.Fingerprint(cwdRel),
	)
	if err := ctx.Err(); err != nil {
		logBashFailure(ctx, call, startedAt, "before_run", err)
		if errors.Is(err, context.DeadlineExceeded) {
			return tool.Result{}, tool.WrapToolError(tool.ErrorCodeDeadlineExceeded, "before bash command", err)
		}
		return tool.Result{}, tool.WrapToolError(tool.ErrorCodeCanceled, "before bash command", err)
	}

	command, err := h.command(ctx, cwd, input.Command)
	if err != nil {
		logBashFailure(ctx, call, startedAt, "launcher", err)
		if errors.Is(err, sandbox.ErrUnavailable) {
			return tool.Result{}, tool.WrapToolError(tool.ErrorCodeSandboxUnavailable, "bash sandbox unavailable", err)
		}
		return tool.Result{}, tool.WrapToolError(tool.ErrorCodeExecution, "prepare bash command", err)
	}
	slog.DebugContext(ctx, "bash process starting",
		"call_id", call.ID,
		"executable", filepath.Base(command.Path),
		"argument_count", len(command.Args),
		"workspace_fingerprint", telemetry.Fingerprint(h.workspace.Root()),
	)

	stdoutBuf := boundedBuffer{limit: maxBashStreamBytes}
	stderrBuf := boundedBuffer{limit: maxBashStreamBytes}
	command.Stdout = &stdoutBuf
	command.Stderr = &stderrBuf

	err = command.Run()
	stdoutStr := stdoutBuf.String()
	stderrStr := stderrBuf.String()
	stdoutTruncated := stdoutBuf.IsTruncated()
	stderrTruncated := stderrBuf.IsTruncated()
	outputStr := combineBashOutput(stdoutStr, stderrStr)
	truncated := stdoutTruncated || stderrTruncated

	result := tool.Result{
		CallID:          call.ID,
		ToolName:        call.Name,
		Output:          outputStr,
		CheckpointID:    checkpointID,
		Stdout:          stdoutStr,
		Stderr:          stderrStr,
		StdoutBytes:     stdoutBuf.BytesSeen(),
		StderrBytes:     stderrBuf.BytesSeen(),
		StdoutTruncated: stdoutTruncated,
		StderrTruncated: stderrTruncated,
		Truncated:       truncated,
		AffectedPaths:   append([]string(nil), affectedPaths...),
	}
	attrs := []any{
		"call_id", call.ID,
		"duration_ms", time.Since(startedAt).Milliseconds(),
		"success", err == nil,
		"output_bytes", len(outputStr),
		"stdout_bytes", result.StdoutBytes,
		"stderr_bytes", result.StderrBytes,
		"stdout_truncated", stdoutTruncated,
		"stderr_truncated", stderrTruncated,
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
		h.workspace.MarkMutationOwned(ctx, resolvedMutationPaths...)
		return result, nil
	}

	if errors.As(err, &exitError) {
		code := exitError.ExitCode()
		result.ExitCode = &code
	}
	if ctxErr := ctx.Err(); ctxErr != nil {
		if errors.Is(ctxErr, context.DeadlineExceeded) {
			return result, tool.WrapToolError(tool.ErrorCodeDeadlineExceeded, "bash command deadline exceeded", ctxErr)
		}
		return result, tool.WrapToolError(tool.ErrorCodeCanceled, "bash command canceled", ctxErr)
	}
	return result, tool.WrapToolError(tool.ErrorCodeExecution, "bash command failed", err)
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

func (h bashHandler) command(ctx context.Context, cwd string, command string) (*exec.Cmd, error) {
	if h.launcher == nil {
		return nil, fmt.Errorf("bash sandbox launcher is required: configure an explicit sandbox profile (use --sandbox off to opt out)")
	}
	if directoryLauncher, ok := h.launcher.(sandbox.DirectoryLauncher); ok {
		return directoryLauncher.CommandInDir(ctx, h.workspace.Root(), cwd, command)
	}
	if filepath.Clean(cwd) != filepath.Clean(h.workspace.Root()) {
		return nil, fmt.Errorf("bash launcher does not support a custom working directory")
	}
	return h.launcher.Command(ctx, h.workspace.Root(), command)
}

func (h bashHandler) resolveCwd(ctx context.Context, input string) (string, string, error) {
	input = strings.TrimSpace(input)
	if input == "" || input == "." {
		return h.workspace.Root(), ".", nil
	}
	resolved, err := h.workspace.Resolve(ctx, input)
	if err != nil {
		return "", "", err
	}
	info, err := os.Stat(resolved)
	if err != nil {
		return "", "", tool.WrapToolError(tool.ErrorCodeNotFound, "bash cwd does not exist", err)
	}
	if !info.IsDir() {
		return "", "", tool.NewToolError(tool.ErrorCodeInvalidArguments, "bash cwd must be a directory")
	}
	rel, err := filepath.Rel(h.workspace.Root(), resolved)
	if err != nil {
		return "", "", fmt.Errorf("resolve bash cwd: %w", err)
	}
	return resolved, filepath.Clean(rel), nil
}

func bashAffectedPaths(cwd string, paths []string) []string {
	if len(paths) == 0 {
		return nil
	}
	out := make([]string, 0, len(paths))
	for _, path := range paths {
		if cwd != "" && cwd != "." {
			path = filepath.Join(cwd, path)
		}
		path = filepath.Clean(path)
		seen := false
		for _, existing := range out {
			if existing == path {
				seen = true
				break
			}
		}
		if !seen {
			out = append(out, path)
		}
	}
	return out
}

type boundedBuffer struct {
	mu        sync.Mutex
	buf       bytes.Buffer
	limit     int
	total     int64
	truncated bool
}

func (b *boundedBuffer) Write(p []byte) (n int, err error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.total += int64(len(p))
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

func (b *boundedBuffer) BytesSeen() int64 {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.total
}

func combineBashOutput(stdout, stderr string) string {
	if stdout == "" {
		return stderr
	}
	if stderr == "" {
		return stdout
	}
	if strings.HasSuffix(stdout, "\n") {
		return stdout + stderr
	}
	return stdout + "\n" + stderr
}
