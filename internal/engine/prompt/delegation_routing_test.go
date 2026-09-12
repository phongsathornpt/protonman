package prompt

import (
	"strings"
	"testing"
)

func TestDelegationProtocolDefinesExplicitRouting(t *testing.T) {
	section := delegationSection(Spec{})
	for _, want := range []string{
		"target is already known",
		"simple and directed",
		"small localized edit",
		"Prefer AGILITY",
		"several distinct searches",
		"multiple repository areas",
		"Prefer STRENGTH",
		"bounded cleanly",
		"Prefer INTELLIGENCE",
		"one active owner",
		"do not independently repeat the same investigation",
		"continue only parent work that is independent",
	} {
		if !strings.Contains(section, want) {
			t.Fatalf("delegation protocol missing routing contract %q:\n%s", want, section)
		}
	}
}

func TestDelegationProtocolKeepsRuntimeOwnedLifecycle(t *testing.T) {
	section := delegationSection(Spec{})
	for _, want := range []string{
		"subagent action=spawn",
		"optional=true",
		"depends_on",
		"delivered automatically by the runtime",
		"Do not poll child state",
		"runtime owns lifecycle observation",
		"subagent action=cancel",
		"subagent action=resume",
		"primary agent owns integration and final verification",
	} {
		if !strings.Contains(section, want) {
			t.Fatalf("delegation protocol lost runtime lifecycle contract %q:\n%s", want, section)
		}
	}
}

func TestDelegationProtocolRemainsCompact(t *testing.T) {
	const maxBytes = 6000
	if got := len(delegationSection(Spec{})); got > maxBytes {
		t.Fatalf("delegation protocol = %d bytes, want <= %d", got, maxBytes)
	}
}

func TestTaskCoordinationDoesNotCreateTodoOnlyForDelegation(t *testing.T) {
	section := taskSection(Spec{Capabilities: ToolCapabilities{Agents: true}})
	if !strings.Contains(section, "Do not create a TODO solely because work is delegated") {
		t.Fatalf("task coordination missing delegation/TODO boundary:\n%s", section)
	}
}
