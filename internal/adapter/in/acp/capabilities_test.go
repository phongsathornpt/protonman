package acp

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/phongsathornpt/protonman/internal/core/permission"
	"github.com/phongsathornpt/protonman/internal/core/tool"
)

func TestSessionCapabilitiesIncludeSupportedAdditionalDirectories(t *testing.T) {
	payload, err := json.Marshal(SessionCapabilities{
		List:                  &struct{}{},
		Resume:                &struct{}{},
		Delete:                &struct{}{},
		Close:                 &struct{}{},
		AdditionalDirectories: &struct{}{},
	})
	if err != nil {
		t.Fatalf("marshal session capabilities: %v", err)
	}
	got := string(payload)
	for _, capability := range []string{"list", "resume", "delete", "close", "additionalDirectories"} {
		if !strings.Contains(got, `"`+capability+`"`) {
			t.Fatalf("supported capability %q missing from %s", capability, got)
		}
	}
}

func TestACPInitializeAdvertisesImplementedSessionList(t *testing.T) {
	server := newTestServer(t, permission.ModeAsk)
	result, _, err := server.dispatch(context.Background(), RPCRequest{Method: "initialize"}, &bytes.Buffer{})
	if err != nil {
		t.Fatal(err)
	}
	if result.(InitializeResult).AgentCapabilities.SessionCapabilities.List == nil {
		t.Fatal("session/list is implemented but not advertised")
	}
	payload, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("marshal initialize response: %v", err)
	}
	var wire struct {
		AgentCapabilities struct {
			SessionCapabilities map[string]json.RawMessage `json:"sessionCapabilities"`
		} `json:"agentCapabilities"`
	}
	if err := json.Unmarshal(payload, &wire); err != nil {
		t.Fatalf("decode initialize response: %v", err)
	}
	if _, ok := wire.AgentCapabilities.SessionCapabilities["list"]; !ok {
		t.Fatalf("initialize wire response omitted sessionCapabilities.list: %s", payload)
	}
}

func TestACPInitializeOnlyAdvertisesAdditionalDirectoriesWhenRuntimeCanEnforceThem(t *testing.T) {
	server := newTestServer(t, permission.ModeAsk)
	result, _, err := server.dispatch(context.Background(), RPCRequest{Method: "initialize"}, &bytes.Buffer{})
	if err != nil {
		t.Fatal(err)
	}
	if result.(InitializeResult).AgentCapabilities.SessionCapabilities.AdditionalDirectories != nil {
		t.Fatal("additionalDirectories advertised without a session registry factory")
	}

	WithSessionRegistryFactory(func(_ string, _ string, _ []string) (tool.Registry, error) {
		return server.registry, nil
	})(server)
	result, _, err = server.dispatch(context.Background(), RPCRequest{Method: "initialize"}, &bytes.Buffer{})
	if err != nil {
		t.Fatal(err)
	}
	if result.(InitializeResult).AgentCapabilities.SessionCapabilities.AdditionalDirectories == nil {
		t.Fatal("additionalDirectories not advertised after runtime enforcement was configured")
	}
}
