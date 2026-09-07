package protonsdk

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

var (
	// ErrInvalidToolInput indicates that tool arguments do not satisfy the declared input schema.
	ErrInvalidToolInput = errors.New("invalid tool input")
	// ErrInvalidToolOutput indicates that structured tool output does not satisfy the declared output schema.
	ErrInvalidToolOutput = errors.New("invalid tool output")
)

// ToolSchemaValidator is an immutable compiled JSON-schema validator for one
// tool input or output contract. Compiling once avoids reparsing schemas on
// every model tool call.
type ToolSchemaValidator struct {
	name           string
	direction      string
	schema         *jsonschema.Schema
	sentinel       error
	requirePayload bool
}

// CompileToolInputValidator compiles a tool input schema once.
func CompileToolInputValidator(tool Tool) (*ToolSchemaValidator, error) {
	return compileToolSchemaValidator(tool.Name, "input", tool.InputSchema, ErrInvalidToolInput, false)
}

// CompileToolOutputValidator compiles a tool output schema once.
func CompileToolOutputValidator(tool Tool) (*ToolSchemaValidator, error) {
	return compileToolSchemaValidator(tool.Name, "output", tool.OutputSchema, ErrInvalidToolOutput, len(tool.OutputSchema) > 0)
}

// Validate checks one JSON payload against the precompiled schema.
func (v *ToolSchemaValidator) Validate(payload json.RawMessage) error {
	if v == nil || v.schema == nil {
		return nil
	}
	if v.requirePayload && len(payload) == 0 {
		return fmt.Errorf("%w: structured output is required for tool %q", v.sentinel, v.name)
	}
	instance, err := jsonschema.UnmarshalJSON(bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("%w: %s for %q is not valid JSON", v.sentinel, v.direction, v.name)
	}
	if err := v.schema.Validate(instance); err != nil {
		return fmt.Errorf("%w: %s for %q does not match schema: %v", v.sentinel, v.direction, v.name, err)
	}
	return nil
}

// ValidateToolInput validates tool arguments against an optional input schema.
func ValidateToolInput(tool Tool, input json.RawMessage) error {
	validator, err := CompileToolInputValidator(tool)
	if err != nil {
		return err
	}
	return validator.Validate(input)
}

// ValidateToolOutput validates structured JSON against an optional output schema.
// External schema loading is disabled so untrusted schemas cannot cause network
// or filesystem fetches during validation.
func ValidateToolOutput(tool Tool, output json.RawMessage) error {
	validator, err := CompileToolOutputValidator(tool)
	if err != nil {
		return err
	}
	return validator.Validate(output)
}

func compileToolSchemaValidator(name, direction string, schemaMap map[string]any, sentinel error, requirePayload bool) (*ToolSchemaValidator, error) {
	validator := &ToolSchemaValidator{name: name, direction: direction, sentinel: sentinel, requirePayload: requirePayload}
	if len(schemaMap) == 0 {
		return validator, nil
	}
	normalizedSchema, err := normalizeToolSchema(schemaMap)
	if err != nil {
		return nil, fmt.Errorf("%w: compile %s schema for %q: %v", sentinel, direction, name, err)
	}
	compiler := jsonschema.NewCompiler()
	compiler.UseLoader(jsonschema.SchemeURLLoader{})
	schemaURL := "urn:proton:tool-" + direction + "-schema"
	if err := compiler.AddResource(schemaURL, normalizedSchema); err != nil {
		return nil, fmt.Errorf("%w: compile %s schema for %q: %v", sentinel, direction, name, err)
	}
	schema, err := compiler.Compile(schemaURL)
	if err != nil {
		return nil, fmt.Errorf("%w: compile %s schema for %q: %v", sentinel, direction, name, err)
	}
	validator.schema = schema
	return validator, nil
}

func normalizeToolSchema(schemaMap map[string]any) (any, error) {
	encoded, err := json.Marshal(schemaMap)
	if err != nil {
		return nil, err
	}
	return jsonschema.UnmarshalJSON(bytes.NewReader(encoded))
}
