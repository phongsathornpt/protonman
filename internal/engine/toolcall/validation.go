package toolcall

import (
	"bytes"
	"encoding/json"

	"github.com/phongsathornpt/protonman/internal/core/tool"
	"github.com/phongsathornpt/protonman/proton-sdk/domain"
	"github.com/phongsathornpt/protonman/proton-sdk/usecase"
)

type compiledToolValidators struct {
	input  *usecase.ToolSchemaValidator
	output *usecase.ToolSchemaValidator
}

type compiledValidatorRegistry interface {
	CompiledValidators(name string) (input, output *usecase.ToolSchemaValidator, ok bool)
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

func initialValidatorsForRegistry(registry tool.Registry, definition tool.Definition) (compiledToolValidators, error) {
	if snapshots, ok := registry.(tool.SnapshotRegistry); ok {
		snapshot, found := snapshots.LookupSnapshot(definition.Name)
		if found {
			if snapshot.ValidatorsCompiled {
				return compiledToolValidators{input: snapshot.InputValidator, output: snapshot.OutputValidator}, nil
			}
			return compileDefinitionValidators(snapshot.Definition)
		}
	}
	return validatorsForRegistry(registry, definition)
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
	sdkTool := domain.Tool{Name: definition.Name, Description: definition.Description, InputSchema: definition.InputSchema, OutputSchema: definition.OutputSchema}
	input, err := usecase.CompileToolInputValidator(sdkTool)
	if err != nil {
		return compiledToolValidators{}, err
	}
	output, err := usecase.CompileToolOutputValidator(sdkTool)
	if err != nil {
		return compiledToolValidators{}, err
	}
	return compiledToolValidators{input: input, output: output}, nil
}

func (s *Service) validatorsFor(definition tool.Definition) (compiledToolValidators, error) {
	_, dynamic := s.registry.(tool.DynamicRegistrar)
	if !dynamic {
		s.mu.RLock()
		validators, ok := s.validators[definition.Name]
		s.mu.RUnlock()
		if ok {
			return validators, nil
		}
	}
	var compiled compiledToolValidators
	var err error
	if dynamic {
		compiled, err = compileDefinitionValidators(definition)
	} else {
		compiled, err = validatorsForRegistry(s.registry, definition)
	}
	if err != nil {
		return compiledToolValidators{}, err
	}
	if dynamic {
		return compiled, nil
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
