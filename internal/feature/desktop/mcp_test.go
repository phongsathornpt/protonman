package desktop

import "testing"

func TestReduceReplacesMCPIntegrationsWithoutAliasing(t *testing.T) {
	source := []MCPIntegrationState{{Name: "files", Command: "mcp-files", Args: []string{"--stdio"}, Env: []string{"TOKEN=x"}}}
	state := Reduce(State{}, Event{Kind: EventIntegrationsReplaced, Integrations: source})
	source[0].Args[0] = "changed"
	source[0].Env[0] = "changed"
	if state.Integrations[0].Args[0] != "--stdio" || state.Integrations[0].Env[0] != "TOKEN=x" {
		t.Fatalf("reducer aliased integration input: %#v", state.Integrations[0])
	}
	next := Reduce(state, Event{})
	next.Integrations[0].Args[0] = "next"
	if state.Integrations[0].Args[0] != "--stdio" {
		t.Fatalf("reducer state clone aliased integration args: %#v", state.Integrations[0])
	}
}
