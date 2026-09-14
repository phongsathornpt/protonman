package domain_test

import (
	"encoding/json"
	"testing"

	"github.com/phongsathornpt/protonman/proton-sdk/domain"
)

func TestEventConstructorsAndValidation(t *testing.T) {
	events := []domain.Event{
		domain.NewTextStartEvent(),
		domain.NewTextDeltaEvent("text"),
		domain.NewTextEndEvent(),
		domain.NewToolCallStartEvent("c1", "read"),
		domain.NewToolCallDeltaEvent("c1", "{}"),
		domain.NewToolCallEndEvent("c1"),
		domain.NewToolCallEvent(domain.ToolCall{ID: "c1", Name: "read", Arguments: json.RawMessage(`{}`)}),
		domain.NewUsageEvent(domain.Usage{InputTokens: 10, OutputTokens: 5, TotalTokens: 15}),
		domain.NewRawEvent([]byte(`raw`)),
		domain.NewFinishEvent(domain.FinishStop, domain.ProviderMetadata{"k": json.RawMessage(`{}`)}),
	}
	for _, ev := range events {
		if err := ev.Validate(); err != nil {
			t.Fatalf("event %q validation failed: %v", ev.Kind, err)
		}
	}

	invalidKind := domain.Event{Kind: "invalid_kind"}
	if err := invalidKind.Validate(); err == nil {
		t.Fatal("invalid kind should fail")
	}

	emptyRaw := domain.Event{Kind: domain.EventRaw}
	if err := emptyRaw.Validate(); err == nil {
		t.Fatal("empty raw should fail")
	}

	badFinish := domain.Event{Kind: domain.EventFinish, FinishReason: "fake"}
	if err := badFinish.Validate(); err == nil {
		t.Fatal("bad finish reason should fail")
	}
}

func TestUsageValidation(t *testing.T) {
	valid := domain.Usage{InputTokens: 10, OutputTokens: 2, TotalTokens: 12}
	if err := valid.Validate(); err != nil {
		t.Fatalf("valid usage failed: %v", err)
	}

	negInput := valid
	negInput.InputTokens = -1
	if err := negInput.Validate(); err == nil {
		t.Fatal("negative input tokens should fail")
	}

	negCached := valid
	negCached.CachedInputTokens = -1
	if err := negCached.Validate(); err == nil {
		t.Fatal("negative cached tokens should fail")
	}
}
