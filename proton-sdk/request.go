package protonsdk

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

type ToolChoice string

const (
	ToolChoiceAuto     ToolChoice = ""
	ToolChoiceRequired ToolChoice = "required"
)

type ModelOptions struct {
	MaxOutputTokens  int
	ToolChoice       ToolChoice
	ReasoningEffort  ReasoningEffort
	ProviderOptions  ProviderOptions
	IncludeRawChunks bool
}

// RequestMetadata carries provider-neutral, request-scoped transport metadata.
// Providers may map these values to protocol headers, but callers own the identity.
type RequestMetadata struct {
	SessionID string
}

type Request struct {
	Messages []Message
	Tools    []Tool
	Options  ModelOptions
	Metadata RequestMetadata
}

func (r Request) Validate() error {
	if r.Options.MaxOutputTokens < 0 {
		return fmt.Errorf("%w: max output tokens cannot be negative", ErrInvalidRequest)
	}
	if r.Options.ToolChoice != ToolChoiceAuto && r.Options.ToolChoice != ToolChoiceRequired {
		return fmt.Errorf("%w: unsupported tool choice %q", ErrInvalidRequest, r.Options.ToolChoice)
	}
	if !r.Options.ReasoningEffort.Valid() {
		return fmt.Errorf("%w: unsupported reasoning effort %q", ErrInvalidRequest, r.Options.ReasoningEffort)
	}
	if r.Options.ToolChoice == ToolChoiceRequired && len(r.Tools) == 0 {
		return fmt.Errorf("%w: required tool choice needs at least one tool", ErrInvalidRequest)
	}
	if len(r.Messages) == 0 {
		return fmt.Errorf("%w: at least one message is required", ErrInvalidRequest)
	}
	for _, message := range r.Messages {
		if err := message.Validate(); err != nil {
			return err
		}
	}
	for _, tool := range r.Tools {
		if err := tool.Validate(); err != nil {
			return err
		}
	}
	return nil
}

type ToolCall struct {
	ID        string
	Name      string
	Arguments json.RawMessage
}

// Clone returns a deep copy of the tool call, copying raw JSON arguments.
func (c ToolCall) Clone() ToolCall {
	c.Arguments = append(json.RawMessage(nil), c.Arguments...)
	return c
}

func (c ToolCall) Validate() error {
	return validateToolCall(c, ErrInvalidEvent)
}

func validateToolCall(c ToolCall, sentinel error) error {
	if strings.TrimSpace(c.ID) == "" {
		return fmt.Errorf("%w: tool call id is required", sentinel)
	}
	if strings.TrimSpace(c.Name) == "" {
		return fmt.Errorf("%w: tool name is required", sentinel)
	}
	if len(c.Arguments) > 0 && !json.Valid(c.Arguments) {
		return fmt.Errorf("%w: tool arguments must be valid JSON", sentinel)
	}
	return nil
}

type Tool struct {
	Name            string
	Description     string
	InputSchema     map[string]any
	OutputSchema    map[string]any
	ProviderOptions ProviderOptions
	Dynamic         bool
}

func (t Tool) Validate() error {
	if strings.TrimSpace(t.Name) == "" {
		return fmt.Errorf("%w: tool name is required", ErrInvalidRequest)
	}
	if strings.TrimSpace(t.Description) == "" {
		return fmt.Errorf("%w: description is required for %q", ErrInvalidRequest, t.Name)
	}
	return nil
}

// ToolResult is the provider-neutral result returned to a model after an agent
// executes a tool call. Execution policy remains owned by the agent runtime.
type ToolResult struct {
	ToolCallID string
	ToolName   string
	Content    string
	Parts      []ContentPart
	IsError    bool
}

func (r ToolResult) Validate() error {
	if strings.TrimSpace(r.ToolCallID) == "" {
		return fmt.Errorf("%w: tool result call id is required", ErrInvalidRequest)
	}
	if strings.TrimSpace(r.ToolName) == "" {
		return fmt.Errorf("%w: tool result name is required", ErrInvalidRequest)
	}
	return nil
}

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
