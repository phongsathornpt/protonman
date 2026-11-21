//go:build desktop || desktop_gio

package controller

import (
	"context"
	"encoding/json"
	"reflect"
	"testing"

	"github.com/phongsathornpt/protonman/internal/adapter/out/acpclient"
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
	profile := DefaultACPAgentProfile()
	methods := []acpAuthMethod{
		{ID: "proton-auth", Name: "Sign in with Proton"},
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
	if status != "Sign in with Proton" {
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
	if !ok || authParams["methodId"] != "proton-auth" {
		t.Fatalf("authenticate params = %#v, want Proton auth method", client.calls[1].params)
	}
}

func TestPromptACPPassesThroughDirectly(t *testing.T) {
	client := &scriptedACPClient{}
	profile := DefaultACPAgentProfile()
	methods := []acpAuthMethod{}
	var result struct {
		SessionID string `json:"sessionId"`
	}
	err := promptACPWithReauthentication(
		context.Background(),
		client,
		profile,
		methods,
		map[string]any{"sessionId": "session-1", "prompt": []any{}},
		&result,
		nil,
	)
	if err != nil {
		t.Fatalf("prompt passthrough: %v", err)
	}
}

func TestPromptACPReauthenticatesAndRetries(t *testing.T) {
	client := &reauthScriptClient{}
	profile := DefaultACPAgentProfile()
	methods := []acpAuthMethod{{ID: "proton-auth", Name: "Sign in with Proton"}}
	var result struct {
		StopReason string `json:"stopReason"`
	}
	err := promptACPWithReauthentication(
		context.Background(),
		client,
		profile,
		methods,
		map[string]any{"sessionId": "session-1", "prompt": []any{}},
		&result,
		nil,
	)
	if err != nil {
		t.Fatalf("prompt with re-auth: %v", err)
	}
	got := []string{}
	for _, call := range client.calls {
		got = append(got, call.method)
	}
	want := []string{"session/prompt", "authenticate", "session/prompt"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("call order = %#v, want %#v", got, want)
	}
	if auth, ok := client.calls[1].params.(map[string]any); !ok || auth["methodId"] != "proton-auth" {
		t.Fatalf("authenticate params = %#v, want proton method", client.calls[1].params)
	}
}

type reauthScriptClient struct {
	calls   []scriptedACPCall
	prompts int
}

func (c *reauthScriptClient) Call(_ context.Context, method string, params any, result any) error {
	c.calls = append(c.calls, scriptedACPCall{method: method, params: params})
	if method == "session/prompt" {
		c.prompts++
		if c.prompts == 1 {
			return &acpclient.RPCError{Code: -32603, Message: "Internal error", Data: json.RawMessage(`{"details":"token expired"}`)}
		}
		return nil
	}
	return nil
}

func TestIsACPReauthenticationRequired(t *testing.T) {
	for _, test := range []struct {
		name string
		err  error
		want bool
	}{
		{
			name: "Cline internal error with re-authentication detail",
			err:  &acpclient.RPCError{Code: -32603, Message: "Internal error", Data: json.RawMessage(`{"details":"cline requires re-authentication."}`)},
			want: true,
		},
		{
			name: "authentication required code",
			err:  &acpclient.RPCError{Code: acpErrorAuthRequired, Message: "Authentication required"},
			want: true,
		},
		{
			name: "unrelated server error code",
			err:  &acpclient.RPCError{Code: acpErrorAuthRequired, Message: "Session is busy"},
			want: false,
		},
		{
			name: "generic internal error",
			err:  &acpclient.RPCError{Code: -32603, Message: "Internal error", Data: json.RawMessage(`{}`)},
			want: false,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := isACPReauthenticationRequired(test.err); got != test.want {
				t.Fatalf("isACPReauthenticationRequired() = %t, want %t", got, test.want)
			}
		})
	}
}

func TestPreferredACPAuthMethodDoesNotAuthenticateTerminalMethods(t *testing.T) {
	profile := DefaultACPAgentProfile()
	methods := []acpAuthMethod{{ID: "proton-auth", Name: "Sign in with Proton", Type: "terminal"}}
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
