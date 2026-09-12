package openai

import (
	"testing"

	sdk "github.com/phongsathornpt/protonman/proton-sdk"
)

func TestProcessChatPreservesCachedInputTokens(t *testing.T) {
	s := &stream{}
	payload := `{"choices":[],"usage":{"prompt_tokens":100,"completion_tokens":10,"total_tokens":110,"prompt_tokens_details":{"cached_tokens":80}}}`
	if err := s.processChat(payload); err != nil {
		t.Fatalf("processChat() error = %v", err)
	}
	if len(s.queue) != 1 || s.queue[0].Kind != sdk.EventUsage {
		t.Fatalf("queue = %#v, want one usage event", s.queue)
	}
	usage := s.queue[0].Usage
	if usage.InputTokens != 100 || usage.OutputTokens != 10 || usage.TotalTokens != 110 || usage.CachedInputTokens != 80 {
		t.Fatalf("usage = %#v", usage)
	}
}

func TestProcessResponsesPreservesCachedInputTokens(t *testing.T) {
	s := &stream{}
	payload := `{"type":"response.completed","response":{"usage":{"input_tokens":120,"output_tokens":20,"total_tokens":140,"input_tokens_details":{"cached_tokens":96,"cache_write_tokens":24}}}}`
	if err := s.processResponses(payload); err != nil {
		t.Fatalf("processResponses() error = %v", err)
	}
	if len(s.queue) < 1 || s.queue[0].Kind != sdk.EventUsage {
		t.Fatalf("queue = %#v, want usage event first", s.queue)
	}
	usage := s.queue[0].Usage
	if usage.InputTokens != 120 || usage.OutputTokens != 20 || usage.TotalTokens != 140 || usage.CachedInputTokens != 96 {
		t.Fatalf("usage = %#v", usage)
	}
}

func TestCachedInputTokensDefaultsToZero(t *testing.T) {
	if got := cachedInputTokens(nil); got != 0 {
		t.Fatalf("cachedInputTokens(nil) = %d, want 0", got)
	}
}
