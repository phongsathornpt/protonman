package protonsdk

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

// ErrInvalidToolOutput indicates that structured tool output does not satisfy
// the tool's declared output schema.
var ErrInvalidToolOutput = errors.New("invalid tool output")

// ValidateToolOutput validates structured JSON against an optional tool output
// schema. External schema loading is disabled so untrusted tool schemas cannot
// cause network or filesystem fetches during validation.
func ValidateToolOutput(tool Tool, output json.RawMessage) error {
	if len(tool.OutputSchema) == 0 {
		return nil
	}
	if len(output) == 0 {
		return fmt.Errorf("%w: structured output is required for tool %q", ErrInvalidToolOutput, tool.Name)
	}
	compiler := jsonschema.NewCompiler()
	compiler.UseLoader(jsonschema.SchemeURLLoader{})
	const schemaURL = "urn:proton:tool-output-schema"
	if err := compiler.AddResource(schemaURL, tool.OutputSchema); err != nil {
		return fmt.Errorf("%w: compile output schema for %q", ErrInvalidToolOutput, tool.Name)
	}
	schema, err := compiler.Compile(schemaURL)
	if err != nil {
		return fmt.Errorf("%w: compile output schema for %q", ErrInvalidToolOutput, tool.Name)
	}
	instance, err := jsonschema.UnmarshalJSON(bytes.NewReader(output))
	if err != nil {
		return fmt.Errorf("%w: structured output for %q is not valid JSON", ErrInvalidToolOutput, tool.Name)
	}
	if err := schema.Validate(instance); err != nil {
		return fmt.Errorf("%w: structured output for %q does not match schema", ErrInvalidToolOutput, tool.Name)
	}
	return nil
}
