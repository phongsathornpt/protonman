package turn

import (
	"context"
	"io"
	"testing"

	"github.com/projectTHORN/proton/internal/model"
	"github.com/projectTHORN/proton/internal/permission"
	"github.com/projectTHORN/proton/internal/toolcall"
	sdk "github.com/projectTHORN/proton/proton-sdk"
)

type sdkTestModel struct {
	requests []sdk.Request
}

func (*sdkTestModel) Provider() string { return "test" }
func (*sdkTestModel) ModelID() string  { return "test-model" }
func (m *sdkTestModel) Stream(_ context.Context, request sdk.Request) (sdk.Stream, error) {
	m.requests = append(m.requests, request)
	return &sdkTestStream{events: []sdk.Event{
		{Kind: sdk.EventTextDelta, Text: "sdk-native"},
		{Kind: sdk.EventFinish, FinishReason: sdk.FinishStop},
	}}, nil
}

type sdkTestStream struct {
	events []sdk.Event
	index  int
}

func (s *sdkTestStream) Next(context.Context) (sdk.Event, error) {
	if s.index >= len(s.events) {
		return sdk.Event{}, io.EOF
	}
	event := s.events[s.index]
	s.index++
	return event, nil
}
func (*sdkTestStream) Close() error { return nil }

func TestLanguageModelLoopConsumesProtonSDKDirectly(t *testing.T) {
	policy, err := permission.NewPolicy(permission.Config{})
	if err != nil {
		t.Fatal(err)
	}
	service, err := toolcall.NewService(emptyRegistry{}, policy)
	if err != nil {
		t.Fatal(err)
	}
	languageModel := &sdkTestModel{}
	loop, err := NewLoop(languageModel, service)
	if err != nil {
		t.Fatal(err)
	}
	result, err := loop.Run(context.Background(), []model.Message{{Role: model.RoleUser, Content: "hello"}}, nil)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if result.Message.Content != "sdk-native" {
		t.Fatalf("content = %q", result.Message.Content)
	}
	if len(languageModel.requests) != 1 || len(languageModel.requests[0].Messages) != 1 {
		t.Fatalf("requests = %#v", languageModel.requests)
	}
}
