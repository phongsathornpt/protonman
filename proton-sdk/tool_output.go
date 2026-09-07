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

// ValidateToolInput validates tool arguments against an optional input schema.
func ValidateToolInput(tool Tool, input json.RawMessage) error {
	return validateToolJSON(tool.Name, "input", tool.InputSchema, input, ErrInvalidToolInput)
}

// ValidateToolOutput validates structured JSON against an optional output schema.
// External schema loading is disabled so untrusted schemas cannot cause network
// or filesystem fetches during validation.
func ValidateToolOutput(tool Tool, output json.RawMessage) error {
	if len(tool.OutputSchema) > 0 && len(output) == 0 {
		return fmt.Errorf("%w: structured output is required for tool %q", ErrInvalidToolOutput, tool.Name)
	}
	return validateToolJSON(tool.Name, "output", tool.OutputSchema, output, ErrInvalidToolOutput)
}

func validateToolJSON(name, direction string, schemaMap map[string]any, payload json.RawMessage, sentinel error) error {
	if len(schemaMap) == 0 {
		return nil
	}
	normalizedSchema, err := normalizeToolSchema(schemaMap)
	if err != nil {
		return fmt.Errorf("%w: compile %s schema for %q", sentinel, direction, name)
	}
	compiler := jsonschema.NewCompiler()
	compiler.UseLoader(jsonschema.SchemeURLLoader{})
	schemaURL := "urn:proton:tool-" + direction + "-schema"
	if err := compiler.AddResource(schemaURL, normalizedSchema); err != nil {
		return fmt.Errorf("%w: compile %s schema for %q", sentinel, direction, name)
	}
	schema, err := compiler.Compile(schemaURL)
	if err != nil {
		return fmt.Errorf("%w: compile %s schema for %q", sentinel, direction, name)
	}
	instance, err := jsonschema.UnmarshalJSON(bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("%w: %s for %q is not valid JSON", sentinel, direction, name)
	}
	if err := schema.Validate(instance); err != nil {
		return fmt.Errorf("%w: %s for %q does not match schema: %v", sentinel, direction, name, err)
	}
	return nil
}

func normalizeToolSchema(schemaMap map[string]any) (any, error) {
	encoded, err := json.Marshal(schemaMap)
	if err != nil {
		return nil, err
	}
	return jsonschema.UnmarshalJSON(bytes.NewReader(encoded))
}
