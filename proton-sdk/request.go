package protonsdk

import "fmt"

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
