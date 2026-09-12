package toolcall

import (
	"bytes"
	"encoding/json"

	"github.com/phongsathornpt/protonman/internal/core/tool"
	sdk "github.com/phongsathornpt/protonman/proton-sdk"
)

type compiledToolValidators struct {
	input  *sdk.ToolSchemaValidator
	output *sdk.ToolSchemaValidator
}

type compiledValidatorRegistry interface {
	CompiledValidators(name string) (input, output *sdk.ToolSchemaValidator, ok bool)
}

func validatorsForRegistry(registry tool.Registry, definition tool.Definition) (compiledToolValidators, error) {
	if cached, ok := registry.(compiledValidatorRegistry); ok {
		input, output, found := cached.CompiledValidators(definition.Name)
		if found {
			return compiledToolValidators{input: input, output: output}, nil
		}
	}
	return compileDefinitionValidators(definition)
}

func structuredJSONType(raw json.RawMessage) string {
	value := bytes.TrimSpace(raw)
	if len(value) == 0 {
		return "missing"
	}
	switch value[0] {
	case '{':
		return "object"
	case '[':
		return "array"
	case '"':
		return "string"
	case 't', 'f':
		return "boolean"
	case 'n':
		return "null"
	default:
		if value[0] == '-' || (value[0] >= '0' && value[0] <= '9') {
			return "number"
		}
		return "invalid"
	}
}

func compileDefinitionValidators(definition tool.Definition) (compiledToolValidators, error) {
	sdkTool := sdk.Tool{Name: definition.Name, Description: definition.Description, InputSchema: definition.InputSchema, OutputSchema: definition.OutputSchema}
	input, err := sdk.CompileToolInputValidator(sdkTool)
	if err != nil {
		return compiledToolValidators{}, err
	}
	output, err := sdk.CompileToolOutputValidator(sdkTool)
	if err != nil {
		return compiledToolValidators{}, err
	}
	return compiledToolValidators{input: input, output: output}, nil
}

func (s *Service) validatorsFor(definition tool.Definition) (compiledToolValidators, error) {
	s.mu.RLock()
	validators, ok := s.validators[definition.Name]
	s.mu.RUnlock()
	if ok {
		return validators, nil
	}
	compiled, err := validatorsForRegistry(s.registry, definition)
	if err != nil {
		return compiledToolValidators{}, err
	}
	s.mu.Lock()
	if existing, exists := s.validators[definition.Name]; exists {
		compiled = existing
	} else {
		s.validators[definition.Name] = compiled
	}
	s.mu.Unlock()
	return compiled, nil
}
