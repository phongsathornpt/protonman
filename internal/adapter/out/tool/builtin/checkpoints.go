package builtin

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/phongsathornpt/protonman/internal/core/tool"
	"github.com/phongsathornpt/protonman/internal/platform/checkpoint"
)

type noCheckpointStore struct{}

var errNoCheckpointStore = fmt.Errorf("checkpoint store is not configured")

func (noCheckpointStore) Capture(context.Context, []string) (string, error) {
	return "", errNoCheckpointStore
}

func (noCheckpointStore) Restore(context.Context, string) error {
	return tool.NewToolError(tool.ErrorCodeExecution, "checkpoint store is not configured")
}

func selectCheckpointStore(stores []checkpoint.Store) checkpoint.Store {
	if len(stores) > 0 && stores[0] != nil {
		return stores[0]
	}
	return noCheckpointStore{}
}

type restoreCheckpointHandler struct {
	checkpoints checkpoint.Store
}

type restoreCheckpointInput struct {
	CheckpointID string `json:"checkpoint_id"`
}

// NewCheckpointRestore returns the permission-gated checkpoint restore adapter.
func NewCheckpointRestore(store checkpoint.Store) tool.Handler {
	return restoreCheckpointHandler{checkpoints: selectCheckpointStore([]checkpoint.Store{store})}
}

func (restoreCheckpointHandler) Definition() tool.Definition {
	return tool.Definition{
		Name:                "edit",
		Description:         "Restore files from a previous Protonman edit checkpoint.",
		Kind:                tool.KindEdit,
		Mutability:          tool.MutabilityMutating,
		Safety:              tool.SafetyContract{MutationDomain: tool.MutationDomainWorkspace, MutationSafety: tool.MutationSafetyWholeFile, CheckpointPolicy: tool.CheckpointPolicyNone, Boundary: tool.BoundaryPolicyWorkspaceWrite},
		PermissionDetailKey: "checkpoint_id",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"checkpoint_id": map[string]any{"type": "string"},
			},
			"required":             []string{"checkpoint_id"},
			"additionalProperties": false,
		},
	}
}

func (h restoreCheckpointHandler) Execute(ctx context.Context, call tool.Call) (tool.Result, error) {
	var input restoreCheckpointInput
	if err := json.Unmarshal(call.Arguments, &input); err != nil {
		return tool.Result{}, tool.WrapToolError(tool.ErrorCodeInvalidArguments, "decode edit restore arguments", err)
	}
	input.CheckpointID = strings.TrimSpace(input.CheckpointID)
	if input.CheckpointID == "" {
		return tool.Result{}, tool.NewToolError(tool.ErrorCodeInvalidArguments, "edit restore checkpoint_id is required")
	}
	if err := h.checkpoints.Restore(ctx, input.CheckpointID); err != nil {
		return tool.Result{}, fmt.Errorf("restore checkpoint %q: %w", input.CheckpointID, err)
	}
	return tool.Result{
		CallID:   call.ID,
		ToolName: call.Name,
		Output:   fmt.Sprintf("Restored checkpoint %s.", input.CheckpointID),
	}, nil
}
