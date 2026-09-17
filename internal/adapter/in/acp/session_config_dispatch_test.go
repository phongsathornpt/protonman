package acp

import (
	"context"
	"io"
	"strings"
	"testing"

	"github.com/phongsathornpt/protonman/internal/core/permission"
)

func TestACPSessionLifecycleReturnsConfigOptions(t *testing.T) {
	server := newTestServer(t, permission.ModeAsk)
	WithSessionRuntimeControls(SessionRuntimeSettings{
		Provider:       "openai",
		Model:          "test-model",
		Reasoning:      "auto",
		LowConcurrency: "auto",
	}, nil)(server)

	ctx := context.Background()
	newResultRaw, _, err := server.dispatch(ctx, RPCRequest{
		Method: "session/new",
		Params: mustJSON(t, SessionNewParams{Cwd: "/tmp/config-options"}),
	}, io.Discard)
	if err != nil {
		t.Fatalf("session/new error = %v", err)
	}
	newResult, ok := newResultRaw.(SessionNewResult)
	if !ok {
		t.Fatalf("session/new result type = %T, want SessionNewResult", newResultRaw)
	}
	assertSessionConfigOptions(t, newResult.ConfigOptions)

	loadResultRaw, _, err := server.dispatch(ctx, RPCRequest{
		Method: "session/load",
		Params: mustJSON(t, SessionLoadParams{SessionID: newResult.SessionID, Cwd: "/tmp/config-options"}),
	}, io.Discard)
	if err != nil {
		t.Fatalf("session/load error = %v", err)
	}
	loadResult, ok := loadResultRaw.(SessionLoadResult)
	if !ok {
		t.Fatalf("session/load result type = %T, want SessionLoadResult", loadResultRaw)
	}
	assertSessionConfigOptions(t, loadResult.ConfigOptions)

	resumeResultRaw, _, err := server.dispatch(ctx, RPCRequest{
		Method: "session/resume",
		Params: mustJSON(t, SessionResumeParams{SessionID: newResult.SessionID, Cwd: "/tmp/config-options"}),
	}, io.Discard)
	if err != nil {
		t.Fatalf("session/resume error = %v", err)
	}
	resumeResult, ok := resumeResultRaw.(SessionResumeResult)
	if !ok {
		t.Fatalf("session/resume result type = %T, want SessionResumeResult", resumeResultRaw)
	}
	assertSessionConfigOptions(t, resumeResult.ConfigOptions)
}

func TestACPSessionSetConfigOptionIsDispatched(t *testing.T) {
	server := newTestServer(t, permission.ModeAsk)
	WithSessionRuntimeControls(SessionRuntimeSettings{
		Provider:       "openai",
		Model:          "test-model",
		Reasoning:      "auto",
		LowConcurrency: "auto",
	}, nil)(server)

	ctx := context.Background()
	newResultRaw, _, err := server.dispatch(ctx, RPCRequest{Method: "session/new"}, io.Discard)
	if err != nil {
		t.Fatalf("session/new error = %v", err)
	}
	newResult := newResultRaw.(SessionNewResult)

	_, _, err = server.dispatch(ctx, RPCRequest{
		Method: methodSessionSetConfigOption,
		Params: mustJSON(t, SetSessionConfigOptionParams{
			SessionID: newResult.SessionID,
			ConfigID:  "unknown-config",
			Value:     mustJSON(t, "value"),
		}),
	}, io.Discard)
	if err == nil || !strings.Contains(err.Error(), "unknown session config option") {
		t.Fatalf("session/set_config_option error = %v, want config validation error", err)
	}
}

func assertSessionConfigOptions(t *testing.T, options []SessionConfigOption) {
	t.Helper()
	for _, id := range []string{configIDModel, configIDReasoning, configIDLowConcurrency} {
		if _, ok := findSessionConfigOption(options, id); !ok {
			t.Fatalf("config options missing %q: %#v", id, options)
		}
	}
}
