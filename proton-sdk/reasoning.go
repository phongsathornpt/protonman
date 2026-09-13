package protonsdk

import (
	"fmt"
	"strings"
)

// ReasoningEffort is the provider-neutral reasoning intensity requested from a model.
// The empty value preserves the model/provider default.
type ReasoningEffort string

const (
	ReasoningDefault ReasoningEffort = ""
	ReasoningNone    ReasoningEffort = "none"
	ReasoningMinimal ReasoningEffort = "minimal"
	ReasoningLow     ReasoningEffort = "low"
	ReasoningMedium  ReasoningEffort = "medium"
	ReasoningHigh    ReasoningEffort = "high"
	ReasoningXHigh   ReasoningEffort = "xhigh"
	ReasoningMax     ReasoningEffort = "max"
)

func ParseReasoningEffort(value string) (ReasoningEffort, error) {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "auto" || value == "default" {
		return ReasoningDefault, nil
	}
	effort := ReasoningEffort(value)
	if !effort.Valid() {
		return ReasoningDefault, fmt.Errorf("%w: unsupported reasoning effort %q", ErrInvalidRequest, value)
	}
	return effort, nil
}

func (e ReasoningEffort) Valid() bool {
	switch e {
	case ReasoningDefault, ReasoningNone, ReasoningMinimal, ReasoningLow, ReasoningMedium, ReasoningHigh, ReasoningXHigh, ReasoningMax:
		return true
	default:
		return false
	}
}
