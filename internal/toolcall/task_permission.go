package toolcall

import (
	"encoding/json"

	"github.com/projectTHORN/proton/internal/permission"
	tododomain "github.com/projectTHORN/proton/internal/todo"
)

func taskMetadataAutoAllowed(request permission.Request) bool {
	if request.ToolKind != permission.ToolTask {
		return false
	}
	switch request.ToolName {
	case "get_todo":
		return true
	case "update_todo":
		var input struct {
			Operations []tododomain.Operation `json:"operations"`
		}
		if err := json.Unmarshal(request.Arguments, &input); err != nil {
			return false
		}
		return tododomain.ClassifyPatch(input.Operations) == tododomain.PatchImpactStatusOnly
	default:
		return false
	}
}
