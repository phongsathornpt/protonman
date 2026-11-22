package acp

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/phongsathornpt/protonman/internal/app"
	"github.com/phongsathornpt/protonman/internal/core/permission"
	"github.com/phongsathornpt/protonman/internal/engine/toolcall"
)

func newTestServerWithConfigControls(t *testing.T) *Server {
	t.Helper()
	loop := newACPReasoningLoop(t, "gpt-4o")
	server := newTestServerWithRunner(t, permission.ModeAsk, loop)
	defaults := SessionRuntimeSettings{
		Provider:       "protonman",
		Model:          "gpt-4o",
		Reasoning:      "auto",
		LowConcurrency: "auto",
		PermissionMode: "ask",
	}
	WithSessionRuntimeControls(defaults, func(_ context.Context, _ string, _ string, _ *toolcall.Service, _ app.Agents, _ SessionRuntimeSettings) (app.Conversation, error) {
		return loop, nil
	})(server)
	WithSessionConfigModelOptions(func(_ context.Context, _ SessionRuntimeSettings) ([]SessionConfigSelectOption, error) {
		return []SessionConfigSelectOption{
			{Value: "gpt-4o", Name: "GPT-4o"},
			{Value: "claude-3-5-sonnet", Name: "Claude 3.5 Sonnet"},
		}, nil
	})(server)
	return server
}

func TestSessionConfigOptionsIncludePermissionMode(t *testing.T) {
	server := newTestServerWithConfigControls(t)
	res, _, err := server.dispatch(context.Background(), RPCRequest{
		Method: "session/new",
		Params: mustJSON(t, map[string]any{"cwd": t.TempDir()}),
	}, &bytes.Buffer{})
	if err != nil {
		t.Fatal(err)
	}
	newRes := res.(SessionNewResult)
	assertACPSchema(t, "NewSessionResponse", newRes)

	options := newRes.ConfigOptions
	permOpt, ok := findSessionConfigOption(options, configIDPermissionMode)
	if !ok {
		t.Fatalf("config option %q missing from session/new response", configIDPermissionMode)
	}
	if permOpt.Category != "_protonman_permission" {
		t.Errorf("category = %q, want _protonman_permission", permOpt.Category)
	}
	if permOpt.CurrentValue != "ask" {
		t.Errorf("currentValue = %q, want ask", permOpt.CurrentValue)
	}
	if len(permOpt.Options) < 2 {
		t.Fatalf("expected at least 2 options for permissionMode, got %d", len(permOpt.Options))
	}
}

func TestSessionSetConfigOptionPermissionMode(t *testing.T) {
	server := newTestServerWithConfigControls(t)
	res, _, err := server.dispatch(context.Background(), RPCRequest{
		Method: "session/new",
		Params: mustJSON(t, map[string]any{"cwd": t.TempDir()}),
	}, &bytes.Buffer{})
	if err != nil {
		t.Fatal(err)
	}
	sessionID := res.(SessionNewResult).SessionID

	// Change permissionMode to always-approve
	routeRes, notify, err := server.route(context.Background(), RPCRequest{
		Method: "session/set_config_option",
		Params: mustJSON(t, SetSessionConfigOptionParams{
			SessionID: sessionID,
			ConfigID:  "permissionMode",
			Value:     json.RawMessage(`"always-approve"`),
		}),
	}, &bytes.Buffer{})
	if err != nil {
		t.Fatalf("set_config_option permissionMode: %v", err)
	}
	assertACPSchema(t, "SetSessionConfigOptionResponse", routeRes)

	if notify == nil {
		t.Fatal("expected config_option_update notification, got nil")
	}
	if notify.Method != "session/update" {
		t.Errorf("notification method = %q, want session/update", notify.Method)
	}
	assertACPSchema(t, "SessionNotification", notify.Params)

	setRes := routeRes.(SetSessionConfigOptionResult)
	permOpt, ok := findSessionConfigOption(setRes.ConfigOptions, "permissionMode")
	if !ok {
		t.Fatal("permissionMode option missing in response")
	}
	if permOpt.CurrentValue != "always-approve" {
		t.Errorf("currentValue = %q, want always-approve", permOpt.CurrentValue)
	}

	// Verify session permission mode was actually updated
	sess, ok := server.lookupSession(sessionID)
	if !ok {
		t.Fatal("session not found")
	}
	if sess.service.Mode() != permission.ModeAlwaysApprove {
		t.Errorf("sess.service.Mode() = %v, want ModeAlwaysApprove", sess.service.Mode())
	}
}

func TestSessionSetConfigOptionModelAndReasoning(t *testing.T) {
	server := newTestServerWithConfigControls(t)
	res, _, err := server.dispatch(context.Background(), RPCRequest{
		Method: "session/new",
		Params: mustJSON(t, map[string]any{"cwd": t.TempDir()}),
	}, &bytes.Buffer{})
	if err != nil {
		t.Fatal(err)
	}
	sessionID := res.(SessionNewResult).SessionID

	// Change model
	routeRes, notify, err := server.route(context.Background(), RPCRequest{
		Method: "session/set_config_option",
		Params: mustJSON(t, SetSessionConfigOptionParams{
			SessionID: sessionID,
			ConfigID:  "model",
			Value:     json.RawMessage(`"claude-3-5-sonnet"`),
		}),
	}, &bytes.Buffer{})
	if err != nil {
		t.Fatalf("set_config_option model: %v", err)
	}
	assertACPSchema(t, "SetSessionConfigOptionResponse", routeRes)
	if notify == nil {
		t.Fatal("expected notification on model change")
	}
	assertACPSchema(t, "SessionNotification", notify.Params)

	modelOpt, ok := findSessionConfigOption(routeRes.(SetSessionConfigOptionResult).ConfigOptions, "model")
	if !ok || modelOpt.CurrentValue != "claude-3-5-sonnet" {
		t.Errorf("model CurrentValue = %q, want claude-3-5-sonnet", modelOpt.CurrentValue)
	}

	// Change reasoning
	routeRes, notify, err = server.route(context.Background(), RPCRequest{
		Method: "session/set_config_option",
		Params: mustJSON(t, SetSessionConfigOptionParams{
			SessionID: sessionID,
			ConfigID:  "reasoning",
			Value:     json.RawMessage(`"high"`),
		}),
	}, &bytes.Buffer{})
	if err != nil {
		t.Fatalf("set_config_option reasoning: %v", err)
	}
	assertACPSchema(t, "SetSessionConfigOptionResponse", routeRes)
	if notify == nil {
		t.Fatal("expected notification on reasoning change")
	}
	assertACPSchema(t, "SessionNotification", notify.Params)

	reasoningOpt, ok := findSessionConfigOption(routeRes.(SetSessionConfigOptionResult).ConfigOptions, "reasoning")
	if !ok || reasoningOpt.CurrentValue != "high" {
		t.Errorf("reasoning CurrentValue = %q, want high", reasoningOpt.CurrentValue)
	}
}

func TestSessionSetConfigOptionValidation(t *testing.T) {
	server := newTestServerWithConfigControls(t)
	res, _, err := server.dispatch(context.Background(), RPCRequest{
		Method: "session/new",
		Params: mustJSON(t, map[string]any{"cwd": t.TempDir()}),
	}, &bytes.Buffer{})
	if err != nil {
		t.Fatal(err)
	}
	sessionID := res.(SessionNewResult).SessionID

	// Unknown option
	_, _, err = server.route(context.Background(), RPCRequest{
		Method: "session/set_config_option",
		Params: mustJSON(t, SetSessionConfigOptionParams{
			SessionID: sessionID,
			ConfigID:  "nonExistent",
			Value:     json.RawMessage(`"val"`),
		}),
	}, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "unknown session config option") {
		t.Fatalf("expected unknown option error, got: %v", err)
	}

	// Invalid model
	_, _, err = server.route(context.Background(), RPCRequest{
		Method: "session/set_config_option",
		Params: mustJSON(t, SetSessionConfigOptionParams{
			SessionID: sessionID,
			ConfigID:  "model",
			Value:     json.RawMessage(`"unadvertised-model"`),
		}),
	}, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "not an advertised") {
		t.Fatalf("expected unadvertised model error, got: %v", err)
	}
}
