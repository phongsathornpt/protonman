package turn

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/projectTHORN/proton/internal/model"
	"github.com/projectTHORN/proton/internal/permission"
	"github.com/projectTHORN/proton/internal/tool"
	"github.com/projectTHORN/proton/internal/toolcall"
	sdk "github.com/projectTHORN/proton/proton-sdk"
)

type sdkTestModel struct {
	requests      []sdk.Request
	capabilities  sdk.ModelCapabilities
	contextWindow int
	tokenLimits   sdk.TokenLimits
}

func (*sdkTestModel) Provider() string     { return "test" }
func (*sdkTestModel) ModelID() string      { return "test-model" }
func (m *sdkTestModel) ContextWindow() int { return m.contextWindow }
func (m *sdkTestModel) TokenLimits() sdk.TokenLimits {
	limits := m.tokenLimits
	if limits.ContextWindow == 0 {
		limits.ContextWindow = m.contextWindow
	}
	return limits
}
func (m *sdkTestModel) Capabilities() sdk.ModelCapabilities {
	if m.capabilities == (sdk.ModelCapabilities{}) {
		return sdk.ModelCapabilities{Streaming: true, Tools: true}
	}
	return m.capabilities
}
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

func TestNewLoopRejectsNonStreamingModel(t *testing.T) {
	policy, err := permission.NewPolicy(permission.Config{})
	if err != nil {
		t.Fatal(err)
	}
	service, err := toolcall.NewService(emptyRegistry{}, policy)
	if err != nil {
		t.Fatal(err)
	}
	_, err = NewLoop(&sdkTestModel{capabilities: sdk.ModelCapabilities{Tools: true}}, service)
	if !errors.Is(err, ErrUnsupportedModelCapability) {
		t.Fatalf("NewLoop() error = %v, want ErrUnsupportedModelCapability", err)
	}
}

func TestLoopRejectsVisionInputWhenUnsupported(t *testing.T) {
	policy, err := permission.NewPolicy(permission.Config{})
	if err != nil {
		t.Fatal(err)
	}
	service, err := toolcall.NewService(emptyRegistry{}, policy)
	if err != nil {
		t.Fatal(err)
	}
	languageModel := &sdkTestModel{capabilities: sdk.ModelCapabilities{Streaming: true}}
	loop, err := NewLoop(languageModel, service)
	if err != nil {
		t.Fatal(err)
	}
	_, err = loop.Run(context.Background(), []model.Message{{Role: model.RoleUser, Parts: []model.ContentPart{{Type: model.ContentPartImage, MIMEType: "image/png", Data: "abc"}}}}, nil)
	if !errors.Is(err, ErrUnsupportedModelCapability) {
		t.Fatalf("Run() error = %v, want ErrUnsupportedModelCapability", err)
	}
	if len(languageModel.requests) != 0 {
		t.Fatalf("model received %d requests, want 0", len(languageModel.requests))
	}
}

func TestLoopOmitsToolsWhenModelDoesNotSupportThem(t *testing.T) {
	handler := &recordingHandler{definition: readFileDefinition()}
	policy, err := permission.NewPolicy(permission.Config{})
	if err != nil {
		t.Fatal(err)
	}
	service, err := toolcall.NewService(&recordingRegistry{handler: handler}, policy)
	if err != nil {
		t.Fatal(err)
	}
	languageModel := &sdkTestModel{capabilities: sdk.ModelCapabilities{Streaming: true}}
	loop, err := NewLoop(languageModel, service)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := loop.Run(context.Background(), []model.Message{{Role: model.RoleUser, Content: "answer without tools"}}, nil); err != nil {
		t.Fatal(err)
	}
	if len(languageModel.requests) != 1 || len(languageModel.requests[0].Tools) != 0 {
		t.Fatalf("published tools = %#v, want none", languageModel.requests)
	}
}

func TestLoopMarksMCPToolsDynamic(t *testing.T) {
	handler := &recordingHandler{definition: tool.Definition{Name: "mcp_lookup", Description: "lookup", Kind: tool.KindMCP, InputSchema: map[string]any{"type": "object"}}}
	policy, err := permission.NewPolicy(permission.Config{})
	if err != nil {
		t.Fatal(err)
	}
	service, err := toolcall.NewService(&recordingRegistry{handler: handler}, policy)
	if err != nil {
		t.Fatal(err)
	}
	languageModel := &sdkTestModel{}
	loop, err := NewLoop(languageModel, service)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := loop.Run(context.Background(), []model.Message{{Role: model.RoleUser, Content: "lookup"}}, nil); err != nil {
		t.Fatal(err)
	}
	if len(languageModel.requests) != 1 || len(languageModel.requests[0].Tools) != 1 || !languageModel.requests[0].Tools[0].Dynamic {
		t.Fatalf("tools = %#v, want one dynamic MCP tool", languageModel.requests)
	}
}

func TestLoopRejectsImageForProviderModelCapabilityOverride(t *testing.T) {
	policy, err := permission.NewPolicy(permission.Config{})
	if err != nil {
		t.Fatal(err)
	}
	service, err := toolcall.NewService(emptyRegistry{}, policy)
	if err != nil {
		t.Fatal(err)
	}
	languageModel := model.NewProviderLanguageModel(
		model.DefaultOpenAIName,
		string(model.ProviderProtocolOpenAI),
		"http://127.0.0.1:1/v1",
		"key",
		"text-only",
		model.WithVisionSupport(false),
	)
	loop, err := NewLoop(languageModel, service)
	if err != nil {
		t.Fatal(err)
	}
	_, err = loop.Run(context.Background(), []model.Message{{Role: model.RoleUser, Parts: []model.ContentPart{{Type: model.ContentPartImage, MIMEType: "image/png", Data: "abc"}}}}, nil)
	if !errors.Is(err, ErrUnsupportedModelCapability) {
		t.Fatalf("Run() error = %v, want ErrUnsupportedModelCapability", err)
	}
}

func TestLoopRejectsOversizedContextBeforeProviderDispatch(t *testing.T) {
	policy, err := permission.NewPolicy(permission.Config{})
	if err != nil {
		t.Fatal(err)
	}
	service, err := toolcall.NewService(emptyRegistry{}, policy)
	if err != nil {
		t.Fatal(err)
	}
	languageModel := &sdkTestModel{contextWindow: 512}
	loop, err := NewLoop(languageModel, service)
	if err != nil {
		t.Fatal(err)
	}
	_, err = loop.Run(context.Background(), []model.Message{{Role: model.RoleUser, Content: strings.Repeat("x", 3000)}}, nil)
	if !errors.Is(err, ErrContextBudgetExceeded) {
		t.Fatalf("Run() error = %v, want ErrContextBudgetExceeded", err)
	}
	if len(languageModel.requests) != 0 {
		t.Fatalf("model received %d requests, want 0", len(languageModel.requests))
	}
}

func TestLoopRejectsInputBeyondPublishedMaxInputTokens(t *testing.T) {
	policy, err := permission.NewPolicy(permission.Config{})
	if err != nil {
		t.Fatal(err)
	}
	service, err := toolcall.NewService(emptyRegistry{}, policy)
	if err != nil {
		t.Fatal(err)
	}
	languageModel := &sdkTestModel{tokenLimits: sdk.TokenLimits{MaxInputTokens: 512}}
	loop, err := NewLoop(languageModel, service)
	if err != nil {
		t.Fatal(err)
	}
	_, err = loop.Run(context.Background(), []model.Message{{Role: model.RoleUser, Content: strings.Repeat("x", 3000)}}, nil)
	if !errors.Is(err, ErrContextBudgetExceeded) {
		t.Fatalf("Run() error = %v", err)
	}
	if len(languageModel.requests) != 0 {
		t.Fatalf("model received %d requests, want 0", len(languageModel.requests))
	}
}

func TestLoopDoesNotTreatMaxInputTokensAsTotalContext(t *testing.T) {
	policy, err := permission.NewPolicy(permission.Config{})
	if err != nil {
		t.Fatal(err)
	}
	service, err := toolcall.NewService(emptyRegistry{}, policy)
	if err != nil {
		t.Fatal(err)
	}
	languageModel := &sdkTestModel{tokenLimits: sdk.TokenLimits{MaxInputTokens: 2048}}
	loop, err := NewLoop(languageModel, service)
	if err != nil {
		t.Fatal(err)
	}
	_, err = loop.Run(context.Background(), []model.Message{{Role: model.RoleUser, Content: strings.Repeat("x", 3000)}}, nil)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if len(languageModel.requests) != 1 {
		t.Fatalf("model received %d requests, want 1", len(languageModel.requests))
	}
}

func TestLoopRejectsRequestedOutputBeyondPublishedLimit(t *testing.T) {
	request := sdk.Request{Messages: []sdk.Message{{Role: sdk.RoleUser, Content: "hello"}}, Options: sdk.ModelOptions{MaxOutputTokens: 1024}}
	languageModel := &sdkTestModel{tokenLimits: sdk.TokenLimits{MaxOutputTokens: 512}}
	if err := validateContextBudget(languageModel, request); !errors.Is(err, ErrContextBudgetExceeded) {
		t.Fatalf("validateContextBudget() error = %v", err)
	}
}
