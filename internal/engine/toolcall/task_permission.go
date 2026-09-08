package toolcall

import (
	"encoding/json"

	"github.com/phongsathornpt/protonman/internal/core/permission"
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
	case "todo":
		switch input.Action {
		case "get":
			return true
		case "update":
			return tododomain.ClassifyPatch(input.Operations) == tododomain.PatchImpactStatusOnly
		}
	}
	return false
}
