package turn

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/phongsathornpt/protonman/internal/adapter/out/model"
	"github.com/phongsathornpt/protonman/internal/core/modelprofile"
	"github.com/phongsathornpt/protonman/internal/core/permission"
	"github.com/phongsathornpt/protonman/internal/core/tool"
	"github.com/phongsathornpt/protonman/internal/engine/toolcall"
	domain "github.com/phongsathornpt/protonman/pkg/proton-sdk/domain"
	port "github.com/phongsathornpt/protonman/pkg/proton-sdk/port"
)

type sdkTestModel struct {
	requests      []domain.Request
	capabilities  domain.ModelCapabilities
	contextWindow int
	tokenLimits   domain.TokenLimits
}

func (*sdkTestModel) Provider() string     { return "test" }
func (*sdkTestModel) ModelID() string      { return "test-model" }
func (m *sdkTestModel) ContextWindow() int { return m.contextWindow }
func (m *sdkTestModel) TokenLimits() domain.TokenLimits {
	limits := m.tokenLimits
	if limits.ContextWindow == 0 {
		limits.ContextWindow = m.contextWindow
	}
	return limits
}
func (m *sdkTestModel) Capabilities() domain.ModelCapabilities {
	if m.capabilities == (domain.ModelCapabilities{}) {
		return domain.ModelCapabilities{Streaming: true, Tools: true}
	}
	return m.capabilities
}
func (m *sdkTestModel) Stream(_ context.Context, request domain.Request) (port.Stream, error) {
	m.requests = append(m.requests, request)
	return &sdkTestStream{events: []domain.Event{
		{Kind: domain.EventTextDelta, Text: "sdk-native"},
		{Kind: domain.EventFinish, FinishReason: domain.FinishStop},
	}}, nil
}

type sdkTestStream struct {
	events []domain.Event
	index  int
}

func (s *sdkTestStream) Next(context.Context) (domain.Event, error) {
	if s.index >= len(s.events) {
		return domain.Event{}, io.EOF
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
	_, err = NewLoop(&sdkTestModel{capabilities: domain.ModelCapabilities{Tools: true}}, service)
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
	languageModel := &sdkTestModel{capabilities: domain.ModelCapabilities{Streaming: true}}
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
	languageModel := &sdkTestModel{capabilities: domain.ModelCapabilities{Streaming: true}}
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
	languageModel := &sdkTestModel{tokenLimits: domain.TokenLimits{MaxInputTokens: 512}}
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
	languageModel := &sdkTestModel{tokenLimits: domain.TokenLimits{MaxInputTokens: 2048}}
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
	request := domain.Request{Messages: []domain.Message{{Role: domain.RoleUser, Content: "hello"}}, Options: domain.ModelOptions{MaxOutputTokens: 1024}}
	languageModel := &sdkTestModel{tokenLimits: domain.TokenLimits{MaxOutputTokens: 512}}
	if err := validateContextBudgetWithVisionPolicy(languageModel, request, modelprofile.DefaultVisionPolicy()); !errors.Is(err, ErrContextBudgetExceeded) {
		t.Fatalf("validateContextBudget() error = %v", err)
	}
}
