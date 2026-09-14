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
		Resume:                &struct{}{},
		Delete:                &struct{}{},
		Close:                 &struct{}{},
		AdditionalDirectories: &struct{}{},
	})
	if err != nil {
		t.Fatalf("marshal session capabilities: %v", err)
	}
	got := string(payload)
	for _, capability := range []string{"resume", "delete", "close", "additionalDirectories"} {
		if !strings.Contains(got, `"`+capability+`"`) {
			t.Fatalf("supported capability %q missing from %s", capability, got)
		}
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
