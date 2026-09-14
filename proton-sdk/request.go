package protonsdk

import (
	"encoding/json"

	"github.com/phongsathornpt/protonman/proton-sdk/domain"
	"github.com/phongsathornpt/protonman/proton-sdk/usecase"
)

const (
	ToolChoiceAuto     = domain.ToolChoiceAuto
	ToolChoiceRequired = domain.ToolChoiceRequired
)

var (
	ErrInvalidToolInput  = domain.ErrInvalidToolInput
	ErrInvalidToolOutput = domain.ErrInvalidToolOutput
)

// CompileToolInputValidator compiles a tool input schema once.
func CompileToolInputValidator(tool Tool) (*ToolSchemaValidator, error) {
	return usecase.CompileToolInputValidator(tool)
}

// CompileToolOutputValidator compiles a tool output schema once.
func CompileToolOutputValidator(tool Tool) (*ToolSchemaValidator, error) {
	return usecase.CompileToolOutputValidator(tool)
}

// ValidateToolInput validates tool arguments against an optional input schema.
func ValidateToolInput(tool Tool, input json.RawMessage) error {
	return usecase.ValidateToolInput(tool, input)
}

// ValidateToolOutput validates structured JSON against an optional output schema.
func ValidateToolOutput(tool Tool, output json.RawMessage) error {
	return usecase.ValidateToolOutput(tool, output)
}


