package turn

import (
	"context"
	"testing"

	"github.com/phongsathornpt/protonman/proton-sdk/domain"
	"github.com/phongsathornpt/protonman/proton-sdk/port"
)

type usageTestModel struct {
	contextWindow int
}

func (m usageTestModel) Provider() string { return "test" }
func (m usageTestModel) ModelID() string  { return "usage-test" }
func (m usageTestModel) Capabilities() domain.ModelCapabilities {
	return domain.ModelCapabilities{Streaming: true}
}
func (m usageTestModel) Stream(context.Context, domain.Request) (port.Stream, error) {
	return nil, nil
}
func (m usageTestModel) Metadata() domain.ModelMetadata {
	return domain.ModelMetadata{TokenLimits: domain.TokenLimits{ContextWindow: m.contextWindow}}
}

func TestLoopContextUsageTracksLatestProviderUsage(t *testing.T) {
	loop := &Loop{languageModel: usageTestModel{contextWindow: 200_000}}
	recordModelUsage(loop, domain.Usage{InputTokens: 100, OutputTokens: 20, TotalTokens: 120})

	used, size, generation, ok := loop.ContextUsage()
	if !ok {
		t.Fatal("ContextUsage() ok = false, want true")
	}
	if used != 120 || size != 200_000 || generation != 1 {
		t.Fatalf("ContextUsage() = (%d, %d, %d), want (120, 200000, 1)", used, size, generation)
	}

	recordModelUsage(loop, domain.Usage{InputTokens: 90, OutputTokens: 10})
	used, size, generation, ok = loop.ContextUsage()
	if !ok {
		t.Fatal("ContextUsage() after second event ok = false, want true")
	}
	if used != 100 || size != 200_000 || generation != 2 {
		t.Fatalf("ContextUsage() after second event = (%d, %d, %d), want (100, 200000, 2)", used, size, generation)
	}
}

func TestLoopContextUsageRequiresKnownContextWindow(t *testing.T) {
	loop := &Loop{languageModel: usageTestModel{}}
	recordModelUsage(loop, domain.Usage{TotalTokens: 42})
	if _, _, _, ok := loop.ContextUsage(); ok {
		t.Fatal("ContextUsage() ok = true with unknown context window")
	}
}
