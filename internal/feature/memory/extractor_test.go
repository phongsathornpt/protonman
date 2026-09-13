package memory

import (
	"context"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/phongsathornpt/protonman/internal/base/runtimepolicy"
	"github.com/phongsathornpt/protonman/internal/core/session"
	sdk "github.com/phongsathornpt/protonman/proton-sdk"
)

type extractionSessionRepo struct {
	states    map[string]session.State
	summaries []session.Summary
}

func (r *extractionSessionRepo) Load(_ context.Context, id string) (session.State, bool, error) {
	state, ok := r.states[id]
	return state, ok, nil
}
func (r *extractionSessionRepo) LatestSession(context.Context, string) (string, session.State, bool, error) {
	return "", session.State{}, false, nil
}
func (r *extractionSessionRepo) Save(context.Context, string, session.State) error { return nil }
func (r *extractionSessionRepo) Delete(context.Context, string) error              { return nil }
func (r *extractionSessionRepo) List(context.Context, string) ([]string, error)    { return nil, nil }
func (r *extractionSessionRepo) ListSummaries(_ context.Context, options session.ListOptions) ([]session.Summary, error) {
	out := make([]session.Summary, 0, len(r.summaries))
	for _, summary := range r.summaries {
		if options.WorkspaceKey != "" && summary.WorkspaceKey != options.WorkspaceKey {
			continue
		}
		out = append(out, summary)
		if options.Limit > 0 && len(out) >= options.Limit {
			break
		}
	}
	return out, nil
}

type extractionModel struct {
	output   string
	requests []sdk.Request
}

func (m *extractionModel) Provider() string { return "test" }
func (m *extractionModel) ModelID() string  { return "memory-test" }
func (m *extractionModel) Capabilities() sdk.ModelCapabilities {
	return sdk.ModelCapabilities{Streaming: true}
}
func (m *extractionModel) Stream(_ context.Context, request sdk.Request) (sdk.Stream, error) {
	m.requests = append(m.requests, request)
	return &extractionStream{events: []sdk.Event{
		{Kind: sdk.EventTextDelta, Text: m.output},
		{Kind: sdk.EventFinish, FinishReason: sdk.FinishStop},
	}}, nil
}

type extractionStream struct {
	events []sdk.Event
	index  int
}

func (s *extractionStream) Next(context.Context) (sdk.Event, error) {
	if s.index >= len(s.events) {
		return sdk.Event{}, io.EOF
	}
	event := s.events[s.index]
	s.index++
	return event, nil
}
func (*extractionStream) Close() error { return nil }

func TestExtractorPersistsWorkspaceMemoryAndRevision(t *testing.T) {
	now := time.Date(2026, 9, 13, 4, 0, 0, 0, time.UTC)
	state := session.State{
		SessionID: "previous", Revision: 3, WorkspaceKey: "ws", UpdatedAt: now.Add(-time.Hour),
		Messages: []session.Message{{ID: "m1", Role: sdk.RoleUser, Content: "Run go test ./... before finishing."}},
	}
	sessions := &extractionSessionRepo{
		states: map[string]session.State{"previous": state},
		summaries: []session.Summary{{ID: "previous", WorkspaceKey: "ws", UpdatedAt: state.UpdatedAt}},
	}
	memories := &fakeRepository{}
	model := &extractionModel{output: `{"memories":[{"scope":"workspace","kind":"procedure","key":"verification command","value":"Run go test ./... before finishing","keywords":["go test"],"confidence":0.95,"message_ids":["m1"]}]}`}
	extractor := NewExtractor(sessions, memories, model, "current", "ws", runtimepolicy.DurableMemory())
	extractor.now = func() time.Time { return now }
	if err := extractor.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(memories.workspace) != 1 || memories.workspace[0].Value != "Run go test ./... before finishing" {
		t.Fatalf("workspace memory = %+v", memories.workspace)
	}
	if revision, ok := memories.processed["previous"]; !ok || revision != 3 {
		t.Fatalf("processed = %+v", memories.processed)
	}
}

func TestExtractorNoOpStillMarksRevisionProcessed(t *testing.T) {
	now := time.Date(2026, 9, 13, 4, 0, 0, 0, time.UTC)
	state := session.State{SessionID: "previous", Revision: 2, WorkspaceKey: "ws", UpdatedAt: now.Add(-time.Hour), Messages: []session.Message{{ID: "m1", Role: sdk.RoleUser, Content: "hello"}}}
	sessions := &extractionSessionRepo{states: map[string]session.State{"previous": state}, summaries: []session.Summary{{ID: "previous", WorkspaceKey: "ws", UpdatedAt: state.UpdatedAt}}}
	memories := &fakeRepository{}
	model := &extractionModel{output: `{"memories":[]}`}
	extractor := NewExtractor(sessions, memories, model, "current", "ws", runtimepolicy.DurableMemory())
	extractor.now = func() time.Time { return now }
	if err := extractor.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	if memories.processed["previous"] != 2 {
		t.Fatalf("processed = %+v", memories.processed)
	}
	if len(model.requests) != 1 {
		t.Fatalf("requests = %d, want 1", len(model.requests))
	}
	if err := extractor.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(model.requests) != 1 {
		t.Fatalf("processed revision was extracted twice: requests=%d", len(model.requests))
	}
}

func TestExtractorRedactsSecretsBeforeModelAndPersistence(t *testing.T) {
	now := time.Date(2026, 9, 13, 4, 0, 0, 0, time.UTC)
	state := session.State{SessionID: "previous", Revision: 1, WorkspaceKey: "ws", UpdatedAt: now.Add(-time.Hour), Messages: []session.Message{{ID: "m1", Role: sdk.RoleUser, Content: "api_key=super-secret-token use headers"}}}
	sessions := &extractionSessionRepo{states: map[string]session.State{"previous": state}, summaries: []session.Summary{{ID: "previous", WorkspaceKey: "ws", UpdatedAt: state.UpdatedAt}}}
	memories := &fakeRepository{}
	model := &extractionModel{output: `{"memories":[{"scope":"workspace","kind":"repo_fact","key":"auth header","value":"api_key=another-secret-token","confidence":0.9,"message_ids":["m1"]}]}`}
	extractor := NewExtractor(sessions, memories, model, "current", "ws", runtimepolicy.DurableMemory())
	extractor.now = func() time.Time { return now }
	if err := extractor.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(model.requests) != 1 || strings.Contains(model.requests[0].Messages[1].Content, "super-secret-token") {
		t.Fatalf("secret reached extraction model: %+v", model.requests)
	}
	if len(memories.workspace) != 1 || strings.Contains(memories.workspace[0].Value, "another-secret-token") {
		t.Fatalf("secret reached memory store: %+v", memories.workspace)
	}
}

func TestExtractorSkipsCurrentSession(t *testing.T) {
	now := time.Date(2026, 9, 13, 4, 0, 0, 0, time.UTC)
	state := session.State{SessionID: "current", Revision: 1, WorkspaceKey: "ws", UpdatedAt: now.Add(-time.Hour), Messages: []session.Message{{ID: "m1", Role: sdk.RoleUser, Content: "remember this"}}}
	sessions := &extractionSessionRepo{states: map[string]session.State{"current": state}, summaries: []session.Summary{{ID: "current", WorkspaceKey: "ws", UpdatedAt: state.UpdatedAt}}}
	model := &extractionModel{output: `{"memories":[]}`}
	extractor := NewExtractor(sessions, &fakeRepository{}, model, "current", "ws", runtimepolicy.DurableMemory())
	extractor.now = func() time.Time { return now }
	if err := extractor.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(model.requests) != 0 {
		t.Fatalf("current session was extracted: requests=%d", len(model.requests))
	}
}
