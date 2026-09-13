package memory

import (
	"context"
	"errors"
	"io"
	"testing"
	"time"

	"github.com/phongsathornpt/protonman/internal/base/runtimepolicy"
	"github.com/phongsathornpt/protonman/internal/core/session"
	sdk "github.com/phongsathornpt/protonman/proton-sdk"
)

type failFirstExtractionModel struct {
	calls int
}

func (*failFirstExtractionModel) Provider() string { return "test" }
func (*failFirstExtractionModel) ModelID() string  { return "memory-test" }
func (*failFirstExtractionModel) Capabilities() sdk.ModelCapabilities {
	return sdk.ModelCapabilities{Streaming: true}
}
func (m *failFirstExtractionModel) Stream(context.Context, sdk.Request) (sdk.Stream, error) {
	m.calls++
	if m.calls == 1 {
		return nil, errors.New("temporary provider failure")
	}
	return &resilienceExtractionStream{events: []sdk.Event{
		{Kind: sdk.EventTextDelta, Text: `{"memories":[{"scope":"workspace","kind":"procedure","key":"verification command","value":"Run go test ./...","confidence":0.95,"message_ids":["m2"]}]}`},
		{Kind: sdk.EventFinish, FinishReason: sdk.FinishStop},
	}}, nil
}

type resilienceExtractionStream struct {
	events []sdk.Event
	index  int
}

func (s *resilienceExtractionStream) Next(context.Context) (sdk.Event, error) {
	if s.index >= len(s.events) {
		return sdk.Event{}, io.EOF
	}
	event := s.events[s.index]
	s.index++
	return event, nil
}
func (*resilienceExtractionStream) Close() error { return nil }

func TestExtractorContinuesAfterSessionModelFailure(t *testing.T) {
	now := time.Date(2026, 9, 13, 4, 0, 0, 0, time.UTC)
	failedState := session.State{
		SessionID: "failed", Revision: 1, WorkspaceKey: "ws", UpdatedAt: now.Add(-2 * time.Hour),
		Messages: []session.Message{{ID: "m1", Role: sdk.RoleUser, Content: "temporary session"}},
	}
	goodState := session.State{
		SessionID: "good", Revision: 2, WorkspaceKey: "ws", UpdatedAt: now.Add(-3 * time.Hour),
		Messages: []session.Message{{ID: "m2", Role: sdk.RoleUser, Content: "Run go test ./... before finishing."}},
	}
	sessions := &extractionSessionRepo{
		states: map[string]session.State{"failed": failedState, "good": goodState},
		summaries: []session.Summary{
			{ID: "failed", WorkspaceKey: "ws", UpdatedAt: failedState.UpdatedAt},
			{ID: "good", WorkspaceKey: "ws", UpdatedAt: goodState.UpdatedAt},
		},
	}
	memories := &fakeRepository{}
	model := &failFirstExtractionModel{}
	extractor := NewExtractor(sessions, memories, model, "current", "ws", runtimepolicy.DurableMemory())
	extractor.now = func() time.Time { return now }
	if err := extractor.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	if model.calls != 2 {
		t.Fatalf("model calls = %d, want failed session followed by good session", model.calls)
	}
	if _, ok := memories.processed["failed"]; ok {
		t.Fatalf("failed session must remain retryable: %+v", memories.processed)
	}
	if revision := memories.processed["good"]; revision != 2 {
		t.Fatalf("good session revision = %d, want 2", revision)
	}
	if len(memories.workspace) != 1 || memories.workspace[0].Key != "verification command" {
		t.Fatalf("workspace memory = %+v", memories.workspace)
	}
}
