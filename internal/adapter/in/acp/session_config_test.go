package acp

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"

	"github.com/phongsathornpt/protonman/internal/app"
	"github.com/phongsathornpt/protonman/internal/core/permission"
	"github.com/phongsathornpt/protonman/internal/engine/toolcall"
)

func TestSessionConfigOptionsIncludesDiscoveredModels(t *testing.T) {
	server := newTestServer(t, permission.ModeAsk)
	WithSessionRuntimeControls(SessionRuntimeSettings{
		Provider:       "openai",
		Model:          "current-model",
		Reasoning:      "auto",
		LowConcurrency: "auto",
	}, nil)(server)
	WithSessionConfigModelOptions(func(context.Context, SessionRuntimeSettings) ([]SessionConfigSelectOption, error) {
		return []SessionConfigSelectOption{
			{Value: "fast-model", Name: "Fast model", Description: "Low latency"},
			{Value: "deep-model", Name: "Deep model", Description: "More reasoning"},
		}, nil
	})(server)
	WithSessionConfigProviderOptions(func(context.Context, SessionRuntimeSettings) ([]SessionConfigSelectOption, error) {
		return []SessionConfigSelectOption{
			{Value: "openai", Name: "OpenAI"},
			{Value: "anthropic", Name: "Anthropic"},
		}, nil
	})(server)

	sess := &Session{}
	bindSessionRuntime(server, sess)
	options := server.sessionConfigOptions(context.Background(), sess)
	model, ok := findSessionConfigOption(options, configIDModel)
	if !ok {
		t.Fatal("model config option was not advertised")
	}
	if model.CurrentValue != "current-model" {
		t.Fatalf("current model = %q, want current-model", model.CurrentValue)
	}
	if len(model.Options) != 3 {
		t.Fatalf("model options = %d, want discovered models plus current value", len(model.Options))
	}
	if model.Options[0].Value != "current-model" {
		t.Fatalf("first model option = %q, want current-model", model.Options[0].Value)
	}
	provider, ok := findSessionConfigOption(options, configIDProvider)
	if !ok {
		t.Fatal("provider config option was not advertised")
	}
	if provider.CurrentValue != "openai" || len(provider.Options) != 2 {
		t.Fatalf("provider option = %#v, want openai with two values", provider)
	}
}

func newConfigTestServer(t *testing.T) (*Server, string) {
	t.Helper()
	runner := &streamingACPRunner{}
	server := newTestServerWithRunner(t, permission.ModeAlwaysApprove, runner)
	WithSessionRuntimeControls(SessionRuntimeSettings{
		Provider:       "openai",
		Model:          "gpt-4o",
		Reasoning:      "auto",
		LowConcurrency: "auto",
	}, func(ctx context.Context, sessionID, cwd string, service *toolcall.Service, agents app.Agents, settings SessionRuntimeSettings) (app.Conversation, error) {
		return runner, nil
	})(server)
	WithSessionConfigModelOptions(func(ctx context.Context, settings SessionRuntimeSettings) ([]SessionConfigSelectOption, error) {
		if settings.Provider == "anthropic" {
			return []SessionConfigSelectOption{
				{Value: "claude-3-5-sonnet", Name: "Claude 3.5 Sonnet"},
				{Value: "claude-3-opus", Name: "Claude 3 Opus"},
			}, nil
		}
		return []SessionConfigSelectOption{
			{Value: "gpt-4o", Name: "GPT-4o"},
			{Value: "gpt-4o-mini", Name: "GPT-4o Mini"},
		}, nil
	})(server)
	WithSessionConfigProviderOptions(func(context.Context, SessionRuntimeSettings) ([]SessionConfigSelectOption, error) {
		return []SessionConfigSelectOption{
			{Value: "openai", Name: "OpenAI"},
			{Value: "anthropic", Name: "Anthropic"},
		}, nil
	})(server)

	created, _, err := server.dispatch(context.Background(), RPCRequest{Method: "session/new"}, &bytes.Buffer{})
	if err != nil {
		t.Fatalf("session/new: %v", err)
	}
	sessionID := created.(SessionNewResult).SessionID
	return server, sessionID
}

func TestDispatchSessionSetConfigOption_Provider(t *testing.T) {
	server, sessionID := newConfigTestServer(t)

	raw, _, err := server.dispatch(context.Background(), RPCRequest{
		Method: "session/set_config_option",
		Params: json.RawMessage(`{"sessionId":"` + sessionID + `","configId":"provider","type":"select","value":"anthropic"}`),
	}, &bytes.Buffer{})
	if err != nil {
		t.Fatalf("session/set_config_option error: %v", err)
	}

	result, ok := raw.(SetSessionConfigOptionResult)
	if !ok {
		t.Fatalf("result type = %T, want SetSessionConfigOptionResult", raw)
	}

	providerOpt, ok := findSessionConfigOption(result.ConfigOptions, configIDProvider)
	if !ok {
		t.Fatal("provider option not found in result")
	}
	if providerOpt.CurrentValue != "anthropic" {
		t.Fatalf("provider currentValue = %q, want anthropic", providerOpt.CurrentValue)
	}

	modelOpt, ok := findSessionConfigOption(result.ConfigOptions, configIDModel)
	if !ok {
		t.Fatal("model option not found in result")
	}
	if modelOpt.CurrentValue != "claude-3-5-sonnet" {
		t.Fatalf("model currentValue = %q, want claude-3-5-sonnet", modelOpt.CurrentValue)
	}

	sess, _ := server.lookupSession(sessionID)
	settings := sessionRuntimeFor(sess)
	if settings.Provider != "anthropic" || settings.Model != "claude-3-5-sonnet" {
		t.Fatalf("runtime settings = %#v, want anthropic and claude-3-5-sonnet", settings)
	}
}

func TestDispatchSessionSetConfigOption_Model(t *testing.T) {
	server, sessionID := newConfigTestServer(t)

	raw, _, err := server.dispatch(context.Background(), RPCRequest{
		Method: "session/set_config_option",
		Params: json.RawMessage(`{"sessionId":"` + sessionID + `","configId":"model","value":"gpt-4o-mini"}`),
	}, &bytes.Buffer{})
	if err != nil {
		t.Fatalf("session/set_config_option error: %v", err)
	}

	result, ok := raw.(SetSessionConfigOptionResult)
	if !ok {
		t.Fatalf("result type = %T, want SetSessionConfigOptionResult", raw)
	}

	modelOpt, ok := findSessionConfigOption(result.ConfigOptions, configIDModel)
	if !ok {
		t.Fatal("model option not found in result")
	}
	if modelOpt.CurrentValue != "gpt-4o-mini" {
		t.Fatalf("model currentValue = %q, want gpt-4o-mini", modelOpt.CurrentValue)
	}

	sess, _ := server.lookupSession(sessionID)
	settings := sessionRuntimeFor(sess)
	if settings.Model != "gpt-4o-mini" {
		t.Fatalf("runtime model = %q, want gpt-4o-mini", settings.Model)
	}
}

func TestDispatchSessionSetConfigOption_Reasoning(t *testing.T) {
	server, sessionID := newConfigTestServer(t)

	raw, _, err := server.dispatch(context.Background(), RPCRequest{
		Method: "session/set_config_option",
		Params: json.RawMessage(`{"sessionId":"` + sessionID + `","configId":"reasoning","value":"high"}`),
	}, &bytes.Buffer{})
	if err != nil {
		t.Fatalf("session/set_config_option error: %v", err)
	}

	result, ok := raw.(SetSessionConfigOptionResult)
	if !ok {
		t.Fatalf("result type = %T, want SetSessionConfigOptionResult", raw)
	}

	reasoningOpt, ok := findSessionConfigOption(result.ConfigOptions, configIDReasoning)
	if !ok {
		t.Fatal("reasoning option not found in result")
	}
	if reasoningOpt.CurrentValue != "high" {
		t.Fatalf("reasoning currentValue = %q, want high", reasoningOpt.CurrentValue)
	}

	sess, _ := server.lookupSession(sessionID)
	settings := sessionRuntimeFor(sess)
	if settings.Reasoning != "high" {
		t.Fatalf("runtime reasoning = %q, want high", settings.Reasoning)
	}
}

func TestDispatchSessionSetConfigOption_LowConcurrency(t *testing.T) {
	server, sessionID := newConfigTestServer(t)

	raw, _, err := server.dispatch(context.Background(), RPCRequest{
		Method: "session/set_config_option",
		Params: json.RawMessage(`{"sessionId":"` + sessionID + `","configId":"lowConcurrency","value":"on"}`),
	}, &bytes.Buffer{})
	if err != nil {
		t.Fatalf("session/set_config_option error: %v", err)
	}

	result, ok := raw.(SetSessionConfigOptionResult)
	if !ok {
		t.Fatalf("result type = %T, want SetSessionConfigOptionResult", raw)
	}

	lowOpt, ok := findSessionConfigOption(result.ConfigOptions, configIDLowConcurrency)
	if !ok {
		t.Fatal("lowConcurrency option not found in result")
	}
	if lowOpt.CurrentValue != "on" {
		t.Fatalf("lowConcurrency currentValue = %q, want on", lowOpt.CurrentValue)
	}

	sess, _ := server.lookupSession(sessionID)
	settings := sessionRuntimeFor(sess)
	if settings.LowConcurrency != "on" {
		t.Fatalf("runtime lowConcurrency = %q, want on", settings.LowConcurrency)
	}
}

func TestDispatchSessionSetConfigOption_ObjectValueAndAliases(t *testing.T) {
	server, sessionID := newConfigTestServer(t)

	// Object value with "value" key and "thought_level" alias
	_, _, err := server.dispatch(context.Background(), RPCRequest{
		Method: "session/set_config_option",
		Params: json.RawMessage(`{"sessionId":"` + sessionID + `","configId":"thought_level","value":{"value":"medium"}}`),
	}, &bytes.Buffer{})
	if err != nil {
		t.Fatalf("thought_level error: %v", err)
	}
	sess, _ := server.lookupSession(sessionID)
	if settings := sessionRuntimeFor(sess); settings.Reasoning != "medium" {
		t.Fatalf("reasoning = %q, want medium", settings.Reasoning)
	}

	// Object value with "id" key and "low_concurrency" alias
	_, _, err = server.dispatch(context.Background(), RPCRequest{
		Method: "session/set_config_option",
		Params: json.RawMessage(`{"sessionId":"` + sessionID + `","configId":"low_concurrency","value":{"id":"off"}}`),
	}, &bytes.Buffer{})
	if err != nil {
		t.Fatalf("low_concurrency error: %v", err)
	}
	if settings := sessionRuntimeFor(sess); settings.LowConcurrency != "off" {
		t.Fatalf("lowConcurrency = %q, want off", settings.LowConcurrency)
	}
}

func TestDispatchSessionSetConfigOption_ValidationErrors(t *testing.T) {
	server, sessionID := newConfigTestServer(t)

	// Unknown session
	_, _, err := server.dispatch(context.Background(), RPCRequest{
		Method: "session/set_config_option",
		Params: json.RawMessage(`{"sessionId":"unknown-sess","configId":"reasoning","value":"low"}`),
	}, &bytes.Buffer{})
	if err == nil {
		t.Fatal("expected error for unknown session")
	}

	// Unknown config ID
	_, _, err = server.dispatch(context.Background(), RPCRequest{
		Method: "session/set_config_option",
		Params: json.RawMessage(`{"sessionId":"` + sessionID + `","configId":"nonexistent","value":"val"}`),
	}, &bytes.Buffer{})
	if err == nil {
		t.Fatal("expected error for unknown configId")
	}

	// Unadvertised provider
	_, _, err = server.dispatch(context.Background(), RPCRequest{
		Method: "session/set_config_option",
		Params: json.RawMessage(`{"sessionId":"` + sessionID + `","configId":"provider","value":"unadvertised-provider"}`),
	}, &bytes.Buffer{})
	if err == nil {
		t.Fatal("expected error for unadvertised provider")
	}

	// Invalid reasoning value
	_, _, err = server.dispatch(context.Background(), RPCRequest{
		Method: "session/set_config_option",
		Params: json.RawMessage(`{"sessionId":"` + sessionID + `","configId":"reasoning","value":"ultra-mega-high"}`),
	}, &bytes.Buffer{})
	if err == nil {
		t.Fatal("expected error for invalid reasoning")
	}
}

func TestHandleRequestSessionSetConfigOptionEndToEnd(t *testing.T) {
	server, sessionID := newConfigTestServer(t)

	var output bytes.Buffer
	req := RPCRequest{
		JSONRPC: "2.0",
		ID:      json.RawMessage("42"),
		Method:  "session/set_config_option",
		Params:  json.RawMessage(`{"sessionId":"` + sessionID + `","configId":"provider","type":"select","value":"anthropic"}`),
	}
	if err := server.handleRequest(context.Background(), req, &output); err != nil {
		t.Fatalf("handleRequest error = %v", err)
	}

	var resp RPCResponse
	if err := json.Unmarshal(output.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal response error = %v, output = %s", err, output.String())
	}
	if resp.Error != nil {
		t.Fatalf("unexpected RPC error: code=%d message=%q", resp.Error.Code, resp.Error.Message)
	}
	if string(resp.ID) != "42" {
		t.Fatalf("response ID = %s, want 42", string(resp.ID))
	}
}
