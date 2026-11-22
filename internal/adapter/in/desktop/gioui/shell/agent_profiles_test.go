//go:build desktop || desktop_gio

package shell

import (
	"slices"
	"testing"

	"github.com/phongsathornpt/protonman/internal/adapter/in/desktop/gioui/controller"
)

func TestCommandSpecForACPAgentResolvesEnvironmentAtRuntime(t *testing.T) {
	t.Setenv("CUSTOM_ACP_TOKEN", "secret")
	spec := controller.CommandSpecForACPAgent(controller.ACPAgentProfile{
		ID:      "custom",
		Command: "custom-acp",
		Args:    []string{"--stdio"},
		Env:     []string{"CUSTOM_ACP_TOKEN"},
	})
	if spec.Path != "custom-acp" || !slices.Equal(spec.Args, []string{"--stdio"}) {
		t.Fatalf("command = %#v", spec)
	}
	if !slices.Equal(spec.Env, []string{"CUSTOM_ACP_TOKEN=secret"}) {
		t.Fatalf("environment = %#v", spec.Env)
	}
}

func TestCommandSpecForACPAgentRetainsOverrideEnvironmentValues(t *testing.T) {
	spec := controller.CommandSpecForACPAgent(controller.ACPAgentProfile{
		ID:      "custom",
		Command: "custom-acp",
		Env:     []string{"CUSTOM_ACP_TOKEN=override-secret"},
	})
	if !slices.Equal(spec.Env, []string{"CUSTOM_ACP_TOKEN=override-secret"}) {
		t.Fatalf("environment = %#v", spec.Env)
	}
}

func TestSessionConnectionProtonOnly(t *testing.T) {
	snapshot := controller.Snapshot{
		Connection: controller.ConnectionConnected,
	}
	if got := sessionConnection(snapshot, controller.ProtonmanAgentID); got != controller.ConnectionConnected {
		t.Fatalf("proton connection = %q", got)
	}
	if got := sessionConnection(snapshot, ""); got != controller.ConnectionConnected {
		t.Fatalf("empty agent connection = %q", got)
	}
	if !anyAgentConnected(snapshot) {
		t.Fatal("a connected Protonman agent was not detected")
	}
	disconnected := controller.Snapshot{Connection: controller.ConnectionReconnecting}
	if anyAgentConnected(disconnected) {
		t.Fatal("reconnecting should not count as connected")
	}
}

func TestActiveAgentDisplayNameIsProtonman(t *testing.T) {
	if got := activeAgentDisplayName(controller.Snapshot{}); got != "Protonman" {
		t.Fatalf("display name = %q, want Protonman", got)
	}
}
