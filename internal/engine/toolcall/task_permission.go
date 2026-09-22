package toolcall

import (
	"encoding/json"

	"github.com/phongsathornpt/protonman/internal/core/permission"
	"github.com/phongsathornpt/protonman/internal/core/tool"
	tododomain "github.com/phongsathornpt/protonman/internal/feature/todo"
)

func taskMetadataAutoAllowed(request permission.Request) bool {
	if request.ToolKind != permission.ToolTask {
		return false
	}
	var input struct {
		Action     string                 `json:"action"`
		Operations []tododomain.Operation `json:"operations"`
	}
	if err := json.Unmarshal(request.Arguments, &input); err != nil {
		return false
	}
	switch request.ToolName {
	case tool.NameTodo:
		switch input.Action {
		case tool.ActionGet:
			return true
		case tool.ActionUpdate:
			return tododomain.ClassifyPatch(input.Operations) == tododomain.PatchImpactStatusOnly
		}
	}
	return false
}
