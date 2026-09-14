package protonsdk

import (
	"github.com/phongsathornpt/protonman/proton-sdk/domain"
	"github.com/phongsathornpt/protonman/proton-sdk/usecase"
)

const (
	RoleSystem    = domain.RoleSystem
	RoleUser      = domain.RoleUser
	RoleAssistant = domain.RoleAssistant
	RoleTool      = domain.RoleTool
)

const (
	ContentPartText  = domain.ContentPartText
	ContentPartImage = domain.ContentPartImage
)

const (
	ReasoningDefault = domain.ReasoningDefault
	ReasoningNone    = domain.ReasoningNone
	ReasoningMinimal = domain.ReasoningMinimal
	ReasoningLow     = domain.ReasoningLow
	ReasoningMedium  = domain.ReasoningMedium
	ReasoningHigh    = domain.ReasoningHigh
	ReasoningXHigh   = domain.ReasoningXHigh
	ReasoningMax     = domain.ReasoningMax
)

func ParseReasoningEffort(value string) (ReasoningEffort, error) {
	return domain.ParseReasoningEffort(value)
}

func NewMessageID() string {
	return domain.NewMessageID()
}

func ValidMessageID(id string) bool {
	return domain.ValidMessageID(id)
}

func EnsureMessageIDs(messages []Message) []Message {
	return domain.EnsureMessageIDs(messages)
}

func CloneMessages(messages []Message) []Message {
	return domain.CloneMessages(messages)
}

func AppendAssistantResponse(messages []Message, response Response) []Message {
	return usecase.AppendAssistantResponse(messages, response)
}

func AppendAssistantStep(messages []Message, result StepResult) []Message {
	return usecase.AppendAssistantStep(messages, result)
}

func AppendToolResults(messages []Message, results []ToolResult) ([]Message, error) {
	return usecase.AppendToolResults(messages, results)
}
