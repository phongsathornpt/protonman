package builtin

import (
	"context"
	"fmt"

	"github.com/projectTHORN/proton/internal/platform/checkpoint"
	"github.com/projectTHORN/proton/internal/core/tool"
	"github.com/projectTHORN/proton/internal/core/workspace"
)

func prepareWorkspaceMutation(
	ctx context.Context,
	workspaceRoot *workspace.Workspace,
	store checkpoint.Store,
	contract tool.SafetyContract,
	guardPaths []string,
	checkpointPaths []string,
) (string, error) {
	if workspaceRoot == nil {
		return "", fmt.Errorf("workspace mutation guard requires a workspace")
	}
	if contract.MutationDomain != tool.MutationDomainWorkspace {
		return "", fmt.Errorf("workspace mutation guard received domain %q", contract.MutationDomain)
	}
	if contract.MutationSafety == tool.MutationSafetyWholeFile || len(guardPaths) > 0 {
		if err := workspaceRoot.GuardWholeFileMutation(ctx, guardPaths...); err != nil {
			return "", err
		}
	}
	switch contract.CheckpointPolicy {
	case tool.CheckpointPolicyNone:
		return "", nil
	case tool.CheckpointPolicyRequired:
		if len(checkpointPaths) == 0 {
			return "", tool.NewToolError(tool.ErrorCodeExecution, "required workspace checkpoint has no mutation paths")
		}
	case tool.CheckpointPolicyWhenKnown:
		if len(checkpointPaths) == 0 || store == nil {
			return "", nil
		}
	default:
		return "", tool.NewToolError(tool.ErrorCodeExecution, "workspace mutation checkpoint policy is not configured")
	}
	if store == nil {
		return "", tool.NewToolError(tool.ErrorCodeExecution, "checkpoint store is not configured")
	}
	checkpointID, err := store.Capture(ctx, checkpointPaths)
	if err != nil {
		return "", fmt.Errorf("checkpoint workspace mutation: %w", err)
	}
	return checkpointID, nil
}
