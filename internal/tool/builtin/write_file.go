package builtin

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/projectTHORN/proton/internal/checkpoint"
	"github.com/projectTHORN/proton/internal/tool"
	"github.com/projectTHORN/proton/internal/workspace"
)

type writeFileHandler struct {
	workspace   *workspace.Workspace
	checkpoints checkpoint.Store
}

type writeFileInput struct {
	FilePath       string `json:"file_path"`
	Content        string `json:"content"`
	ExpectedSHA256 string `json:"expected_sha256,omitempty"`
}

// NewWriteFile returns the atomic whole-file write adapter.
func NewWriteFile(workspaceRoot *workspace.Workspace, stores ...checkpoint.Store) tool.Handler {
	return writeFileHandler{
		workspace:   workspaceRoot,
		checkpoints: selectCheckpointStore(stores),
	}
}

func (h writeFileHandler) PermissionDetail(arguments json.RawMessage) string {
	var input writeFileInput
	if err := json.Unmarshal(arguments, &input); err != nil {
		return ""
	}
	return managedFileDetail(input.FilePath, []byte(input.Content))
}

func (writeFileHandler) Definition() tool.Definition {
	return tool.Definition{
		Name:                "write_file",
		Description:         "Create or replace a UTF-8 text file atomically.",
		Kind:                tool.KindEdit,
		Mutability:          tool.MutabilityMutating,
		Safety:              tool.SafetyContract{MutationDomain: tool.MutationDomainWorkspace, MutationSafety: tool.MutationSafetyWholeFile, CheckpointPolicy: tool.CheckpointPolicyRequired, Boundary: tool.BoundaryPolicyWorkspaceWrite},
		PermissionDetailKey: "file_path",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"file_path": map[string]any{"type": "string"},
				"content":   map[string]any{"type": "string"},
				"expected_sha256": map[string]any{
					"type":        "string",
					"description": "SHA-256 from a complete read_file result; required when overwriting an existing file",
				},
			},
			"required":             []string{"file_path", "content"},
			"additionalProperties": false,
		},
	}
}

func (h writeFileHandler) Execute(ctx context.Context, call tool.Call) (tool.Result, error) {
	if h.workspace == nil {
		return tool.Result{}, fmt.Errorf("write_file workspace is required")
	}
	var input writeFileInput
	if err := json.Unmarshal(call.Arguments, &input); err != nil {
		return tool.Result{}, fmt.Errorf("decode write_file arguments: %w", err)
	}
	input.FilePath = strings.TrimSpace(input.FilePath)
	if input.FilePath == "" {
		return tool.Result{}, fmt.Errorf("write_file file_path is required")
	}
	resolvedPath, err := h.workspace.Resolve(ctx, input.FilePath)
	if err != nil {
		return tool.Result{}, err
	}
	if err := h.workspace.GuardWholeFileMutation(ctx, resolvedPath); err != nil {
		return tool.Result{}, err
	}
	existing, exists, err := readEditFile(ctx, h.workspace, resolvedPath)
	if err != nil {
		return tool.Result{}, err
	}
	if exists {
		expected := strings.ToLower(strings.TrimSpace(input.ExpectedSHA256))
		decoded, decodeErr := hex.DecodeString(expected)
		if expected == "" {
			return tool.Result{}, tool.NewToolError(tool.ErrorCodeInvalidArguments, "expected_sha256 is required when overwriting an existing file; call read_file first")
		}
		if decodeErr != nil || len(decoded) != sha256.Size {
			return tool.Result{}, tool.NewToolError(tool.ErrorCodeInvalidArguments, "expected_sha256 must be a 64-character SHA-256 hex digest")
		}
		current := sha256.Sum256(existing)
		if expected != fmt.Sprintf("%x", current[:]) {
			return tool.Result{}, tool.NewToolError(tool.ErrorCodeConflict, "write_file target changed since it was read; refresh the file and retry")
		}
	}
	checkpointID, err := h.checkpoints.Capture(ctx, []string{resolvedPath})
	if err != nil {
		return tool.Result{}, fmt.Errorf("checkpoint %q: %w", input.FilePath, err)
	}
	displayPath := input.FilePath
	if rel, relErr := h.workspace.RelRead(resolvedPath); relErr == nil && rel != "" {
		displayPath = rel
	}
	if err := atomicWriteResolved(ctx, h.workspace, resolvedPath, []byte(input.Content)); err != nil {
		return tool.Result{
			CallID:       call.ID,
			ToolName:     call.Name,
			CheckpointID: checkpointID,
		}, fmt.Errorf("write %q: %w", input.FilePath, err)
	}
	h.workspace.MarkMutationOwned(ctx, resolvedPath)
	newDigest := sha256.Sum256([]byte(input.Content))
	return tool.Result{
		CallID:        call.ID,
		ToolName:      call.Name,
		Output:        fmt.Sprintf("Wrote file successfully to %s.", displayPath),
		SHA256:        fmt.Sprintf("%x", newDigest[:]),
		CheckpointID:  checkpointID,
		AffectedPaths: []string{displayPath},
	}, nil
}
