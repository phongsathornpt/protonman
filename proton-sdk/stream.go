package protonsdk

import (
	"context"

	"github.com/phongsathornpt/protonman/proton-sdk/domain"
	"github.com/phongsathornpt/protonman/proton-sdk/usecase"
)

const (
	FinishStop      = domain.FinishStop
	FinishLength    = domain.FinishLength
	FinishToolCalls = domain.FinishToolCalls
	FinishError     = domain.FinishError
	FinishOther     = domain.FinishOther
)

const (
	EventTextStart     = domain.EventTextStart
	EventTextDelta     = domain.EventTextDelta
	EventTextEnd       = domain.EventTextEnd
	EventToolCallStart = domain.EventToolCallStart
	EventToolCallDelta = domain.EventToolCallDelta
	EventToolCallEnd   = domain.EventToolCallEnd
	EventToolCall      = domain.EventToolCall
	EventUsage         = domain.EventUsage
	EventRaw           = domain.EventRaw
	EventFinish        = domain.EventFinish
)

func NewTextStartEvent() Event                    { return domain.NewTextStartEvent() }
func NewTextDeltaEvent(text string) Event         { return domain.NewTextDeltaEvent(text) }
func NewTextEndEvent() Event                      { return domain.NewTextEndEvent() }
func NewToolCallStartEvent(id, name string) Event { return domain.NewToolCallStartEvent(id, name) }
func NewToolCallDeltaEvent(id, delta string) Event {
	return domain.NewToolCallDeltaEvent(id, delta)
}
func NewToolCallEndEvent(id string) Event  { return domain.NewToolCallEndEvent(id) }
func NewToolCallEvent(call ToolCall) Event { return domain.NewToolCallEvent(call) }
func NewUsageEvent(usage Usage) Event      { return domain.NewUsageEvent(usage) }
func NewRawEvent(data []byte) Event        { return domain.NewRawEvent(data) }
func NewFinishEvent(reason FinishReason, metadata ProviderMetadata) Event {
	return domain.NewFinishEvent(reason, metadata)
}

func Collect(ctx context.Context, stream Stream) (Response, error) {
	return usecase.Collect(ctx, stream)
}

func CollectStep(ctx context.Context, stream Stream) (StepResult, error) {
	return usecase.CollectStep(ctx, stream)
}
