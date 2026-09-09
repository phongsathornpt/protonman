package readfile

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/phongsathornpt/protonman/internal/core/tool"
	"github.com/phongsathornpt/protonman/internal/core/workspace"
)

type readFileHandler struct {
	workspace *workspace.Workspace
}

func New(workspaceRoot *workspace.Workspace) tool.Handler {
	return readFileHandler{workspace: workspaceRoot}
}

func (h readFileHandler) Execute(ctx context.Context, call tool.Call) (tool.Result, error) {
	if h.workspace == nil {
		return tool.Result{}, fmt.Errorf("read workspace is required")
	}
	var input readFileInput
	if err := json.Unmarshal(call.Arguments, &input); err != nil {
		return tool.Result{}, tool.WrapToolError(tool.ErrorCodeInvalidArguments, "decode read arguments", err)
	}
	input.Path = strings.TrimSpace(input.Path)
	if input.Path == "" {
		return tool.Result{}, tool.NewToolError(tool.ErrorCodeInvalidArguments, "read path is required")
	}
	input.View = strings.ToLower(strings.TrimSpace(input.View))
	if input.View == "" {
		input.View = "auto"
	}
	switch input.View {
	case "auto", "text", "image", "structured", "metadata":
	default:
		return tool.Result{}, tool.NewToolError(tool.ErrorCodeInvalidArguments, "read view must be auto, text, image, structured, or metadata")
	}
	if input.Offset < 0 {
		return tool.Result{}, tool.NewToolError(tool.ErrorCodeInvalidArguments, "read offset must be non-negative")
	}
	if input.StartLine < 0 || input.EndLine < 0 {
		return tool.Result{}, tool.NewToolError(tool.ErrorCodeInvalidArguments, "read start_line and end_line must be non-negative")
	}
	lineMode := input.StartLine > 0 || input.EndLine > 0 || input.LineNumbers
	if lineMode && input.View != "auto" && input.View != "text" {
		return tool.Result{}, tool.NewToolError(tool.ErrorCodeInvalidArguments, "read line selection requires text view")
	}
	if lineMode && (input.Offset != 0 || input.Continuation != "") {
		return tool.Result{}, tool.NewToolError(tool.ErrorCodeInvalidArguments, "read line selection cannot be combined with offset or continuation")
	}
	if input.View == "image" || input.View == "structured" || input.View == "metadata" {
		if input.Offset != 0 || input.Continuation != "" || input.Limit != 0 {
			return tool.Result{}, tool.NewToolError(tool.ErrorCodeInvalidArguments, "read image, structured, and metadata views do not accept text pagination arguments")
		}
	}
	if input.StartLine == 0 && input.EndLine > 0 {
		input.StartLine = 1
	}
	if input.EndLine > 0 && input.EndLine < input.StartLine {
		return tool.Result{}, tool.NewToolError(tool.ErrorCodeInvalidArguments, "read end_line must be greater than or equal to start_line")
	}
	if input.Limit < 0 || input.Limit > MaxReadFileBytes {
		return tool.Result{}, tool.NewToolError(tool.ErrorCodeInvalidArguments, "read limit must be between 1 byte and 2 MiB")
	}
	if input.Limit == 0 {
		input.Limit = DefaultReadFileBytes
	}
	path, err := h.workspace.ResolveExistingRead(ctx, input.Path)
	if err != nil {
		return tool.Result{}, err
	}

	file, err := h.workspace.OpenReadFile(ctx, path)
	if err != nil {
		switch {
		case errors.Is(err, os.ErrNotExist):
			return tool.Result{}, tool.WrapToolError(tool.ErrorCodeNotFound, fmt.Sprintf("not found: %q", input.Path), err)
		case errors.Is(err, os.ErrPermission):
			return tool.Result{}, tool.WrapToolError(tool.ErrorCodePermissionDenied, fmt.Sprintf("cannot read path: %q", input.Path), err)
		default:
			return tool.Result{}, fmt.Errorf("open %q: %w", input.Path, err)
		}
	}
	fileInfo, err := file.Stat()
	if err != nil {
		_ = file.Close()
		return tool.Result{}, fmt.Errorf("stat %q: %w", input.Path, err)
	}
	if fileInfo.IsDir() {
		_ = file.Close()
		recoveryArgs, marshalErr := json.Marshal(map[string]any{"path": input.Path})
		if marshalErr != nil {
			return tool.Result{}, fmt.Errorf("encode ls recovery for %q: %w", input.Path, marshalErr)
		}
		return tool.Result{}, tool.NewToolError(
			tool.ErrorCodeInvalidArguments,
			fmt.Sprintf("%q is a directory; use ls instead", input.Path),
		).WithRecovery(tool.Recovery{
			Action: tool.RecoveryUseDedicatedTool, Tool: "ls", Arguments: recoveryArgs,
		})
	}
	if !fileInfo.Mode().IsRegular() {
		_ = file.Close()
		return tool.Result{}, tool.NewToolError(
			tool.ErrorCodeInvalidArguments,
			fmt.Sprintf("%q is not a regular file", input.Path),
		)
	}
	if lineMode {
		return readFileLines(ctx, file, input, call)
	}

	artifactView := input.View
	if artifactView != "text" && input.Offset == 0 && input.Continuation == "" {
		artifact, detectErr := detectArtifact(file, input.Path)
		if detectErr != nil {
			_ = file.Close()
			return tool.Result{}, fmt.Errorf("detect %q: %w", input.Path, detectErr)
		}
		switch artifactView {
		case "image":
			if artifact.Kind != artifactImage {
				_ = file.Close()
				return tool.Result{}, tool.NewToolError(tool.ErrorCodeInvalidArguments, fmt.Sprintf("%q is not a supported image", input.Path))
			}
			return readImageArtifact(ctx, file, fileInfo, input, artifact, call)
		case "structured":
			if !artifact.Kind.structured() {
				_ = file.Close()
				return tool.Result{}, tool.NewToolError(tool.ErrorCodeInvalidArguments, fmt.Sprintf("%q is not supported structured data", input.Path))
			}
			return readStructuredArtifact(ctx, file, fileInfo, input, artifact, call)
		case "metadata":
			_ = file.Close()
			return artifactResult(call, artifactEnvelope{
				Kind: artifact.Kind, Path: input.Path, MIMEType: artifact.MIMEType, SizeBytes: fileInfo.Size(),
			}, fmt.Sprintf("%s %s · %d bytes", artifact.Kind, artifact.MIMEType, fileInfo.Size()))
		case "auto":
			switch {
			case artifact.Kind == artifactImage:
				return readImageArtifact(ctx, file, fileInfo, input, artifact, call)
			case artifact.Kind == artifactBinary:
				_ = file.Close()
				return artifactResult(call, artifactEnvelope{
					Kind: artifact.Kind, Path: input.Path, MIMEType: artifact.MIMEType, SizeBytes: fileInfo.Size(),
				}, fmt.Sprintf("binary %s · %d bytes", artifact.MIMEType, fileInfo.Size()))
			}
		}
	}

	return readTextBytes(ctx, file, fileInfo, input, call)
}
