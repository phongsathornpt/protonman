package acpclient

import (
	"encoding/json"
	"testing"
)

func TestInitializeResultDecodesAgentCapabilities(t *testing.T) {
	payload := []byte(`{
		"protocolVersion":1,
		"agentInfo":{"name":"protonman","version":"0.0.8"},
		"agentCapabilities":{
			"loadSession":true,
			"promptCapabilities":{"image":true,"audio":false,"embeddedContext":true},
			"sessionCapabilities":{"resume":{},"close":{},"additionalDirectories":{}},
			"mcpCapabilities":{"http":true,"sse":true}
		},
		"authMethods":[]
	}`)

	var got InitializeResult
	if err := json.Unmarshal(payload, &got); err != nil {
		t.Fatalf("decode initialize result: %v", err)
	}
	if got.ProtocolVersion != 1 || got.AgentInfo.Name != "protonman" {
		t.Fatalf("initialize result = %#v", got)
	}
	if !got.AgentCapabilities.LoadSession || !got.AgentCapabilities.PromptCapabilities.Image {
		t.Fatalf("agent capabilities = %#v", got.AgentCapabilities)
	}
	if got.AgentCapabilities.SessionCapabilities.Resume == nil || got.AgentCapabilities.SessionCapabilities.Close == nil {
		t.Fatalf("session capabilities = %#v", got.AgentCapabilities.SessionCapabilities)
	}
	if !got.AgentCapabilities.MCPCapabilities.HTTP || !got.AgentCapabilities.MCPCapabilities.SSE {
		t.Fatalf("mcp capabilities = %#v", got.AgentCapabilities.MCPCapabilities)
	}
}

func TestSessionConfigOptionDecodesSelectControl(t *testing.T) {
	payload := []byte(`{
		"id":"reasoning",
		"name":"Reasoning",
		"category":"thought_level",
		"type":"select",
		"currentValue":"low",
		"options":[{"value":"low","name":"Low"},{"value":"high","name":"High"}]
	}`)

	var got SessionConfigOption
	if err := json.Unmarshal(payload, &got); err != nil {
		t.Fatalf("decode session config option: %v", err)
	}
	if got.ID != "reasoning" || got.CurrentValue != "low" || len(got.Options) != 2 {
		t.Fatalf("config option = %#v", got)
	}
}
