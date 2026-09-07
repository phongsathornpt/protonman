package builtin

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/projectTHORN/proton/internal/platform/checkpoint"
	"github.com/projectTHORN/proton/internal/core/tool"
	"github.com/projectTHORN/proton/internal/core/workspace"
)

type applyPatchHandler struct {
	workspace   *workspace.Workspace
	checkpoints checkpoint.Store
}

type applyPatchInput struct {
	Patch string `json:"patch"`
}

type patchOperation struct {
	kind     patchOperationKind
	path     string
	movePath string
	content  string
	chunks   []patchChunk
}

type patchOperationKind uint8

const (
	patchUnknown patchOperationKind = iota
	patchAdd
	patchDelete
	patchUpdate
)

func (k patchOperationKind) String() string {
	switch k {
	case patchAdd:
		return "add"
	case patchDelete:
		return "delete"
	case patchUpdate:
		return "update"
	default:
		return "unknown"
	}
}

type patchChunk struct {
	context    string
	hasContext bool
	oldLines   []string
	newLines   []string
	endOfFile  bool
}

type plannedPatchChange struct {
	kind        patchOperationKind
	path        string
	destination string
	content     string
}

// NewApplyPatch returns the Codex-format multi-file patch adapter.
func NewApplyPatch(workspaceRoot *workspace.Workspace, stores ...checkpoint.Store) tool.Handler {
	return applyPatchHandler{
		workspace:   workspaceRoot,
		checkpoints: selectCheckpointStore(stores),
	}
}

func (applyPatchHandler) Definition() tool.Definition {
	return tool.Definition{
		Name:                "apply_patch",
		Description:         "Apply a bounded multi-file patch to the workspace.",
		Kind:                tool.KindForName("apply_patch"),
		Mutability:          tool.MutabilityMutating,
		Safety:              tool.SafetyContract{MutationDomain: tool.MutationDomainWorkspace, MutationSafety: tool.MutationSafetyDynamic, CheckpointPolicy: tool.CheckpointPolicyRequired, Boundary: tool.BoundaryPolicyWorkspaceWrite},
		PermissionDetailKey: "patch",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"patch": map[string]any{
					"type":        "string",
					"description": "Patch enclosed by *** Begin Patch and *** End Patch",
				},
			},
			"required":             []string{"patch"},
			"additionalProperties": false,
		},
	}
}

func (h applyPatchHandler) PermissionDetail(arguments json.RawMessage) string {
	var input applyPatchInput
	if err := json.Unmarshal(arguments, &input); err != nil {
		return ""
	}
	operations, err := parsePatch(input.Patch)
	if err != nil || len(operations) == 0 {
		return ""
	}
	counts := summarizePatchOperations(operations)
	details := []string{counts.String()}
	seen := make(map[string]struct{}, len(operations)*2)
	for _, op := range operations {
		for _, path := range []string{op.path, op.movePath} {
			if path == "" {
				continue
			}
			if _, ok := seen[path]; ok {
				continue
			}
			seen[path] = struct{}{}
			details = append(details, managedFileDetail(path, nil))
		}
	}
	return strings.Join(details, " · ")
}

type patchOperationSummary struct {
	adds, deletes, modifies, moves int
}

func summarizePatchOperations(operations []patchOperation) patchOperationSummary {
	var summary patchOperationSummary
	for _, op := range operations {
		switch {
		case op.kind == patchAdd:
			summary.adds++
		case op.kind == patchDelete:
			summary.deletes++
		case op.kind == patchUpdate && op.movePath != "":
			summary.moves++
		case op.kind == patchUpdate:
			summary.modifies++
		}
	}
	return summary
}

func (s patchOperationSummary) String() string {
	parts := make([]string, 0, 4)
	if s.adds > 0 {
		parts = append(parts, fmt.Sprintf("add %d", s.adds))
	}
	if s.modifies > 0 {
		parts = append(parts, fmt.Sprintf("modify %d", s.modifies))
	}
	if s.moves > 0 {
		parts = append(parts, fmt.Sprintf("move %d", s.moves))
	}
	if s.deletes > 0 {
		parts = append(parts, fmt.Sprintf("delete %d", s.deletes))
	}
	return strings.Join(parts, " · ")
}

func (h applyPatchHandler) Execute(ctx context.Context, call tool.Call) (tool.Result, error) {
	if h.workspace == nil {
		return tool.Result{}, fmt.Errorf("apply_patch workspace is required")
	}
	var input applyPatchInput
	if err := json.Unmarshal(call.Arguments, &input); err != nil {
		return tool.Result{}, fmt.Errorf("decode apply_patch arguments: %w", err)
	}
	if strings.TrimSpace(input.Patch) == "" {
		return tool.Result{}, fmt.Errorf("apply_patch patch is required")
	}
	operations, err := parsePatch(input.Patch)
	if err != nil {
		return tool.Result{}, fmt.Errorf("parse apply_patch: %w", err)
	}
	changes, err := h.planPatch(ctx, operations)
	if err != nil {
		return tool.Result{}, fmt.Errorf("plan apply_patch: %w", err)
	}
	riskyPaths := make([]string, 0, len(changes)*2)
	for _, change := range changes {
		if change.kind == patchDelete || change.destination != "" {
			riskyPaths = append(riskyPaths, change.path)
		}
		if change.destination != "" {
			riskyPaths = append(riskyPaths, change.destination)
		}
	}
	checkpointPaths := make([]string, 0, len(changes)*2)
	for _, change := range changes {
		checkpointPaths = append(checkpointPaths, change.path)
		if change.destination != "" {
			checkpointPaths = append(checkpointPaths, change.destination)
		}
	}
	checkpointID, err := prepareWorkspaceMutation(ctx, h.workspace, h.checkpoints, h.Definition().Safety, riskyPaths, checkpointPaths)
	if err != nil {
		return tool.Result{}, err
	}
	for _, change := range changes {
		if err := ctx.Err(); err != nil {
			return patchFailureResult(call, checkpointID, fmt.Errorf("before applying patch: %w", err))
		}
		switch change.kind {
		case patchAdd:
			if err := atomicWrite(ctx, h.workspace, change.path, []byte(change.content)); err != nil {
				return patchFailureResult(call, checkpointID, fmt.Errorf("write patched file %q: %w", change.path, err))
			}
		case patchUpdate:
			if change.destination == "" {
				if err := atomicWrite(ctx, h.workspace, change.path, []byte(change.content)); err != nil {
					return patchFailureResult(call, checkpointID, fmt.Errorf("write patched file %q: %w", change.path, err))
				}
			}
		case patchDelete:
			if err := removeWorkspaceFile(ctx, h.workspace, change.path); err != nil {
				return patchFailureResult(call, checkpointID, fmt.Errorf("delete patched file %q: %w", change.path, err))
			}
		default:
			return patchFailureResult(
				call,
				checkpointID,
				fmt.Errorf("apply_patch has unknown change kind %d", change.kind),
			)
		}
		if change.destination != "" {
			if err := atomicWrite(ctx, h.workspace, change.destination, []byte(change.content)); err != nil {
				return patchFailureResult(call, checkpointID, fmt.Errorf("write moved file %q: %w", change.destination, err))
			}
			if err := removeWorkspaceFile(ctx, h.workspace, change.path); err != nil {
				return patchFailureResult(call, checkpointID, fmt.Errorf("remove moved source %q: %w", change.path, err))
			}
		}
	}

	affectedPaths := make([]string, 0, len(operations)*2)
	var output strings.Builder
	output.WriteString("Success. Updated the following files:\n")
	for _, operation := range operations {
		switch operation.kind {
		case patchAdd:
			output.WriteString("A ")
		case patchDelete:
			output.WriteString("D ")
		default:
			output.WriteString("M ")
		}
		output.WriteString(operation.path)
		output.WriteByte('\n')
		affectedPaths = append(affectedPaths, operation.path)
		if operation.movePath != "" {
			affectedPaths = append(affectedPaths, operation.movePath)
		}
	}
	h.workspace.MarkMutationOwned(ctx, checkpointPaths...)
	return tool.Result{
		CallID:           call.ID,
		ToolName:         call.Name,
		Output:           output.String(),
		CheckpointID:     checkpointID,
		MutationCoverage: tool.MutationCoverageFull,
		AffectedPaths:    affectedPaths,
	}, nil
}

func patchFailureResult(call tool.Call, checkpointID string, err error) (tool.Result, error) {
	return tool.Result{
		CallID:       call.ID,
		ToolName:     call.Name,
		CheckpointID: checkpointID,
	}, err
}

func (h applyPatchHandler) planPatch(ctx context.Context, operations []patchOperation) ([]plannedPatchChange, error) {
	changes := make([]plannedPatchChange, 0, len(operations))
	for _, operation := range operations {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		path, err := h.workspace.Resolve(ctx, operation.path)
		if err != nil {
			return nil, fmt.Errorf("resolve %q: %w", operation.path, err)
		}
		switch operation.kind {
		case patchAdd:
			if err := requireMissing(path); err != nil {
				return nil, fmt.Errorf("add %q: %w", operation.path, err)
			}
			changes = append(changes, plannedPatchChange{
				kind:    patchAdd,
				path:    path,
				content: operation.content,
			})
		case patchDelete:
			contents, exists, err := readEditFile(ctx, h.workspace, path)
			if err != nil {
				return nil, fmt.Errorf("delete %q: %w", operation.path, err)
			}
			if !exists {
				return nil, fmt.Errorf("delete %q: file does not exist", operation.path)
			}
			if contents == nil {
				return nil, fmt.Errorf("delete %q: target is not a regular file", operation.path)
			}
			changes = append(changes, plannedPatchChange{kind: patchDelete, path: path})
		case patchUpdate:
			contents, exists, err := readEditFile(ctx, h.workspace, path)
			if err != nil {
				return nil, fmt.Errorf("update %q: %w", operation.path, err)
			}
			if !exists {
				return nil, fmt.Errorf("update %q: file does not exist", operation.path)
			}
			updated, err := derivePatchedContent(string(contents), operation.path, operation.chunks)
			if err != nil {
				return nil, err
			}
			if operation.movePath == "" {
				changes = append(changes, plannedPatchChange{
					kind:    patchUpdate,
					path:    path,
					content: updated,
				})
				continue
			}
			destination, err := h.workspace.Resolve(ctx, operation.movePath)
			if err != nil {
				return nil, fmt.Errorf("resolve move destination %q: %w", operation.movePath, err)
			}
			if err := requireMissing(destination); err != nil {
				return nil, fmt.Errorf("move %q: %w", operation.movePath, err)
			}
			changes = append(changes, plannedPatchChange{
				kind:        patchUpdate,
				path:        path,
				destination: destination,
				content:     updated,
			})
		default:
			return nil, fmt.Errorf("unknown patch operation kind %s", operation.kind)
		}
	}
	return changes, nil
}

func requireMissing(path string) error {
	_, err := os.Lstat(path)
	if err == nil {
		return errors.New("target already exists")
	}
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return fmt.Errorf("inspect target: %w", err)
}

func parsePatch(input string) ([]patchOperation, error) {
	input = strings.ReplaceAll(input, "\r\n", "\n")
	input = strings.Trim(input, "\n")
	lines := strings.Split(input, "\n")
	if len(lines) < 2 || strings.TrimSpace(lines[0]) != "*** Begin Patch" {
		return nil, errors.New("first line must be *** Begin Patch")
	}
	if strings.TrimSpace(lines[len(lines)-1]) != "*** End Patch" {
		return nil, errors.New("last line must be *** End Patch")
	}

	operations := make([]patchOperation, 0)
	for index := 1; index < len(lines)-1; {
		line := lines[index]
		switch {
		case strings.HasPrefix(line, "*** Add File: "):
			path := strings.TrimSpace(strings.TrimPrefix(line, "*** Add File: "))
			if path == "" {
				return nil, fmt.Errorf("line %d: add path is required", index+1)
			}
			index++
			contentLines := make([]string, 0)
			for index < len(lines)-1 && strings.HasPrefix(lines[index], "+") {
				contentLines = append(contentLines, lines[index][1:])
				index++
			}
			if len(contentLines) == 0 {
				return nil, fmt.Errorf("line %d: add file must contain + lines", index+1)
			}
			operations = append(operations, patchOperation{
				kind:    patchAdd,
				path:    path,
				content: strings.Join(contentLines, "\n") + "\n",
			})
		case strings.HasPrefix(line, "*** Delete File: "):
			path := strings.TrimSpace(strings.TrimPrefix(line, "*** Delete File: "))
			if path == "" {
				return nil, fmt.Errorf("line %d: delete path is required", index+1)
			}
			operations = append(operations, patchOperation{kind: patchDelete, path: path})
			index++
		case strings.HasPrefix(line, "*** Update File: "):
			operation, nextIndex, err := parseUpdateOperation(lines, index)
			if err != nil {
				return nil, err
			}
			operations = append(operations, operation)
			index = nextIndex
		default:
			return nil, fmt.Errorf("line %d: invalid patch operation %q", index+1, line)
		}
	}
	if len(operations) == 0 {
		return nil, errors.New("patch must contain at least one file operation")
	}
	return operations, nil
}

func parseUpdateOperation(lines []string, start int) (patchOperation, int, error) {
	path := strings.TrimSpace(strings.TrimPrefix(lines[start], "*** Update File: "))
	if path == "" {
		return patchOperation{}, 0, fmt.Errorf("line %d: update path is required", start+1)
	}
	index := start + 1
	movePath := ""
	if index < len(lines)-1 && strings.HasPrefix(lines[index], "*** Move to: ") {
		movePath = strings.TrimSpace(strings.TrimPrefix(lines[index], "*** Move to: "))
		if movePath == "" {
			return patchOperation{}, 0, fmt.Errorf("line %d: move destination is required", index+1)
		}
		index++
	}

	chunks := make([]patchChunk, 0)
	for index < len(lines)-1 && !isPatchOperationHeader(lines[index]) {
		if strings.TrimSpace(lines[index]) == "" {
			index++
			continue
		}
		if !strings.HasPrefix(lines[index], "@@") {
			return patchOperation{}, 0, fmt.Errorf("line %d: update hunk must start with @@", index+1)
		}
		chunk := patchChunk{}
		chunk.context = strings.TrimPrefix(lines[index], "@@")
		chunk.context = strings.TrimPrefix(chunk.context, " ")
		chunk.hasContext = chunk.context != ""
		index++
		lineCount := 0
		for index < len(lines)-1 && !strings.HasPrefix(lines[index], "@@") && !isPatchOperationHeader(lines[index]) {
			line := lines[index]
			if line == "*** End of File" {
				chunk.endOfFile = true
				index++
				break
			}
			if line == "" {
				chunk.oldLines = append(chunk.oldLines, "")
				chunk.newLines = append(chunk.newLines, "")
				lineCount++
				index++
				continue
			}
			switch line[0] {
			case ' ':
				value := line[1:]
				chunk.oldLines = append(chunk.oldLines, value)
				chunk.newLines = append(chunk.newLines, value)
			case '-':
				chunk.oldLines = append(chunk.oldLines, line[1:])
			case '+':
				chunk.newLines = append(chunk.newLines, line[1:])
			default:
				return patchOperation{}, 0, fmt.Errorf("line %d: invalid update line prefix", index+1)
			}
			lineCount++
			index++
		}
		if lineCount == 0 {
			return patchOperation{}, 0, fmt.Errorf("line %d: update hunk is empty", index+1)
		}
		chunks = append(chunks, chunk)
	}
	if len(chunks) == 0 {
		return patchOperation{}, 0, fmt.Errorf("line %d: update file must contain a hunk", start+1)
	}
	return patchOperation{
		kind:     patchUpdate,
		path:     path,
		movePath: movePath,
		chunks:   chunks,
	}, index, nil
}

func isPatchOperationHeader(line string) bool {
	return strings.HasPrefix(line, "*** Add File: ") ||
		strings.HasPrefix(line, "*** Delete File: ") ||
		strings.HasPrefix(line, "*** Update File: ")
}

func derivePatchedContent(original string, path string, chunks []patchChunk) (string, error) {
	original = strings.ReplaceAll(original, "\r\n", "\n")
	lines := strings.Split(original, "\n")
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	cursor := 0
	for _, chunk := range chunks {
		if chunk.hasContext {
			found := findLine(lines, chunk.context, cursor)
			if found < 0 {
				return "", fmt.Errorf("failed to find context %q in %s", chunk.context, path)
			}
			cursor = found + 1
		}
		if len(chunk.oldLines) == 0 {
			lines = insertLines(lines, len(lines), chunk.newLines)
			cursor = len(lines)
			continue
		}
		start := findSequence(lines, chunk.oldLines, cursor, chunk.endOfFile)
		if start < 0 {
			return "", fmt.Errorf("failed to find expected lines in %s:\n%s", path, strings.Join(chunk.oldLines, "\n"))
		}
		lines = replaceLines(lines, start, len(chunk.oldLines), chunk.newLines)
		cursor = start + len(chunk.newLines)
	}
	return strings.Join(lines, "\n") + "\n", nil
}

func findLine(lines []string, target string, start int) int {
	for index := start; index < len(lines); index++ {
		if lines[index] == target {
			return index
		}
	}
	return -1
}

func findSequence(lines []string, pattern []string, start int, endOfFile bool) int {
	if len(pattern) == 0 {
		return len(lines)
	}
	for index := start; index+len(pattern) <= len(lines); index++ {
		if endOfFile && index+len(pattern) != len(lines) {
			continue
		}
		matched := true
		for offset, value := range pattern {
			if lines[index+offset] != value {
				matched = false
				break
			}
		}
		if matched {
			return index
		}
	}
	return -1
}

func insertLines(lines []string, index int, values []string) []string {
	result := make([]string, 0, len(lines)+len(values))
	result = append(result, lines[:index]...)
	result = append(result, values...)
	result = append(result, lines[index:]...)
	return result
}

func replaceLines(lines []string, start int, count int, values []string) []string {
	result := make([]string, 0, len(lines)-count+len(values))
	result = append(result, lines[:start]...)
	result = append(result, values...)
	result = append(result, lines[start+count:]...)
	return result
}
