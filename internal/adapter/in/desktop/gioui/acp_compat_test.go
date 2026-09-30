//go:build desktop || desktop_gio

package gioui

import (
	"context"
	"encoding/json"
	"reflect"
	"testing"

	"github.com/phongsathornpt/protonman/internal/adapter/out/acpclient"
	"github.com/phongsathornpt/protonman/internal/app"
)

type scriptedACPCall struct {
	method string
	params any
}

type scriptedACPClient struct {
	calls           []scriptedACPCall
	newSessionCalls int
}

func (c *scriptedACPClient) Call(_ context.Context, method string, params any, result any) error {
	c.calls = append(c.calls, scriptedACPCall{method: method, params: params})
	switch method {
	case "session/new":
		c.newSessionCalls++
		if c.newSessionCalls == 1 {
			return &acpclient.RPCError{Code: acpErrorAuthRequired, Message: "Authentication required"}
		}
		return decodeACPResult(map[string]any{"sessionId": "session-1"}, result)
	case "authenticate", "session/set_mode", "session/set_config_option":
		return nil
	default:
		return nil
	}
}

func decodeACPResult(value any, result any) error {
	if result == nil {
		return nil
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		return err
	}
	return json.Unmarshal(encoded, result)
}

func TestCreateACPSessionAuthenticatesAndRetries(t *testing.T) {
	client := &scriptedACPClient{}
	profile := app.ACPAgentProfile{ID: "cline", DisplayName: "Cline", Command: "cline"}
	methods := []acpAuthMethod{
		{ID: "cline", Name: "Sign in with Cline"},
		{ID: "cline-pass", Name: "Sign in with ClinePass"},
	}
	var gotSession struct {
		SessionID string `json:"sessionId"`
	}
	var status string
	err := createACPSession(
		context.Background(),
		client,
		profile,
		methods,
		map[string]any{"cwd": "/workspace"},
		&gotSession,
		func(value string) { status = value },
	)
	if err != nil {
		t.Fatalf("create ACP session: %v", err)
	}
	if gotSession.SessionID != "session-1" {
		t.Fatalf("session ID = %q, want session-1", gotSession.SessionID)
	}
	if status != "Sign in with Cline" {
		t.Fatalf("authentication status = %q", status)
	}
	wantMethods := []string{"session/new", "authenticate", "session/new"}
	var gotMethods []string
	for _, call := range client.calls {
		gotMethods = append(gotMethods, call.method)
	}
	if !reflect.DeepEqual(gotMethods, wantMethods) {
		t.Fatalf("call order = %#v, want %#v", gotMethods, wantMethods)
	}
	authParams, ok := client.calls[1].params.(map[string]any)
	if !ok || authParams["methodId"] != "cline" {
		t.Fatalf("authenticate params = %#v, want Cline default auth method", client.calls[1].params)
	}
}

func TestPreferredACPAuthMethodDoesNotAuthenticateTerminalMethods(t *testing.T) {
	profile := app.ACPAgentProfile{ID: "cline", DisplayName: "Cline", Command: "cline"}
	methods := []acpAuthMethod{{ID: "cline", Name: "Sign in with Cline", Type: "terminal"}}
	if method, ok := preferredACPAuthMethod(profile, methods); ok {
		t.Fatalf("selected terminal auth method %#v; Desktop cannot perform terminal auth", method)
	}
}

func TestInitializeACPResponseFeatures(t *testing.T) {
	var supported initializeACPResponse
	if err := json.Unmarshal([]byte(`{
		"protocolVersion": 1,
		"agentCapabilities": {"sessionCapabilities": {"additionalDirectories": {}, "list": {}, "resume": {}}},
		"authMethods": [{"id":"cline","name":"Sign in with Cline"}]
	}`), &supported); err != nil {
		t.Fatalf("decode initialize result: %v", err)
	}
	features := supported.features()
	if !features.SupportsAdditionalDirectories || !features.SupportsSessionList || !features.SupportsSessionResume ||
		len(features.AuthMethods) != 1 || features.AuthMethods[0].ID != "cline" {
		t.Fatalf("features = %#v, want auth method and additional-directory support", features)
	}

	var unsupported initializeACPResponse
	if err := json.Unmarshal([]byte(`{"protocolVersion":1,"agentCapabilities":{"sessionCapabilities":{}},"authMethods":[]}`), &unsupported); err != nil {
		t.Fatalf("decode unsupported initialize result: %v", err)
	}
	unsupportedFeatures := unsupported.features()
	if unsupportedFeatures.SupportsAdditionalDirectories || unsupportedFeatures.SupportsSessionList || unsupportedFeatures.SupportsSessionResume {
		t.Fatalf("missing session capabilities were enabled: %#v", unsupportedFeatures)
	}

	var malformed initializeACPResponse
	if err := json.Unmarshal([]byte(`{"protocolVersion":1,"agentCapabilities":{"sessionCapabilities":{"additionalDirectories":true}}}`), &malformed); err != nil {
		t.Fatalf("decode malformed capability: %v", err)
	}
	if malformed.features().SupportsAdditionalDirectories {
		t.Fatal("non-object additionalDirectories capability was enabled")
	}
}

func TestACPConfigOptionsDecodeBooleanCurrentValue(t *testing.T) {
	var result struct {
		ConfigOptions []acpConfigOption `json:"configOptions"`
	}
	if err := json.Unmarshal([]byte(`{
		"configOptions": [
			{"type":"select","id":"model","currentValue":"model-a","options":[{"value":"model-a","name":"Model A"}]},
			{"type":"boolean","id":"auto_approve","currentValue":true}
		]
	}`), &result); err != nil {
		t.Fatalf("decode Cline config options: %v", err)
	}
	models, currentModel := extractModelsFromConfigOptions(result.ConfigOptions)
	if !reflect.DeepEqual(models, []string{"model-a"}) || currentModel != "model-a" {
		t.Fatalf("model options = %#v, current %q", models, currentModel)
	}
	if !clineAutoApproveValue(result.ConfigOptions, false) {
		t.Fatal("auto_approve true was not decoded")
	}
}

func TestSetClinePermissionModeUsesModeAndAutoApproveOptions(t *testing.T) {
	tests := []struct {
		mode          string
		wantMethods   []string
		wantModeID    string
		wantAutoValue bool
	}{
		{mode: "ask", wantMethods: []string{"session/set_config_option", "session/set_mode"}, wantModeID: "act"},
		{mode: "plan", wantMethods: []string{"session/set_config_option", "session/set_mode"}, wantModeID: "plan"},
		{mode: "always-approve", wantMethods: []string{"session/set_mode", "session/set_config_option"}, wantModeID: "act", wantAutoValue: true},
	}
	for _, test := range tests {
		t.Run(test.mode, func(t *testing.T) {
			client := &scriptedACPClient{}
			if err := setClinePermissionMode(context.Background(), client, "session-1", test.mode); err != nil {
				t.Fatalf("set Cline permission mode: %v", err)
			}
			var gotMethods []string
			for _, call := range client.calls {
				gotMethods = append(gotMethods, call.method)
			}
			if !reflect.DeepEqual(gotMethods, test.wantMethods) {
				t.Fatalf("call order = %#v, want %#v", gotMethods, test.wantMethods)
			}
			modeCall := client.calls[0]
			if test.mode != "always-approve" {
				modeCall = client.calls[1]
			}
			modeParams := modeCall.params.(map[string]any)
			if modeParams["modeId"] != test.wantModeID {
				t.Fatalf("mode ID = %#v, want %q", modeParams["modeId"], test.wantModeID)
			}
			autoCall := client.calls[0]
			if test.mode == "always-approve" {
				autoCall = client.calls[1]
			}
			autoParams := autoCall.params.(map[string]any)
			if autoParams["configId"] != "auto_approve" || autoParams["value"] != test.wantAutoValue {
				t.Fatalf("auto-approve params = %#v", autoCall.params)
			}
		})
	}
}

func TestAdditionalDirectoriesRequireAdvertisedAgentCapability(t *testing.T) {
	controller := &controller{
		agentFeatures: map[string]acpAgentFeatures{
			"cline":  {SupportsAdditionalDirectories: false},
			"proton": {SupportsAdditionalDirectories: true},
		},
	}
	directories := []string{"/workspace/extra"}
	if got := controller.additionalDirectoriesForAgent("cline", directories); got != nil {
		t.Fatalf("unsupported agent directories = %#v, want nil", got)
	}
	if got := controller.additionalDirectoriesForAgent("proton", directories); !reflect.DeepEqual(got, directories) {
		t.Fatalf("supported agent directories = %#v, want %#v", got, directories)
	}
}

func TestClinePermissionModeProjectsACPState(t *testing.T) {
	if got := clinePermissionMode("plan", true); got != "plan" {
		t.Fatalf("Plan mode with auto-approve = %q, want plan", got)
	}
	if got := clinePermissionMode("act", false); got != "ask" {
		t.Fatalf("Act mode with auto-approve off = %q, want ask", got)
	}
	if got := clinePermissionMode("act", true); got != "always-approve" {
		t.Fatalf("Act mode with auto-approve on = %q, want always-approve", got)
	}
}
