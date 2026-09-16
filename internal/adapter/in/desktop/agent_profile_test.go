//go:build desktop

package desktop

import (
	"encoding/json"
	"testing"

	"github.com/phongsathornpt/protonman/internal/adapter/out/acpclient"
)

type testPreferences struct{ values map[string]string }

func (p *testPreferences) String(key string) string { return p.values[key] }
func (p *testPreferences) SetString(key, value string) {
	p.values[key] = value
}

func TestResolveAgentProfileDefaultsToProtonman(t *testing.T) {
	t.Setenv("PROTONMAN_AGENT", "")
	profile, err := resolveAgentProfile()
	if err != nil {
		t.Fatal(err)
	}
	if profile.ID != "protonman" || profile.DisplayName != "Protonman" {
		t.Fatalf("profile = %#v", profile)
	}
	if len(profile.Command.Args) != 1 || profile.Command.Args[0] != "--acp" {
		t.Fatalf("args = %#v", profile.Command.Args)
	}
}

func TestResolveAgentProfileAntigravity(t *testing.T) {
	t.Setenv("PROTONMAN_AGENT", "antigravity")
	t.Setenv("ANTIGRAVITY_ACP_COMMAND", "/opt/agy/agy_acp_server.par")
	t.Setenv("ANTIGRAVITY_ACP_ARGS_JSON", `["--uid=desktop"]`)
	profile, err := resolveAgentProfile()
	if err != nil {
		t.Fatal(err)
	}
	if profile.ID != "antigravity" || profile.DisplayName != "Google Antigravity" {
		t.Fatalf("profile = %#v", profile)
	}
	if profile.Command.Path != "/opt/agy/agy_acp_server.par" || len(profile.Command.Args) != 1 || profile.Command.Args[0] != "--uid=desktop" {
		t.Fatalf("command = %#v", profile.Command)
	}
}

func TestResolveAgentProfileRejectsInvalidAntigravityArguments(t *testing.T) {
	t.Setenv("PROTONMAN_AGENT", "antigravity")
	t.Setenv("ANTIGRAVITY_ACP_ARGS_JSON", `"--uid=desktop"`)
	if _, err := resolveAgentProfile(); err == nil {
		t.Fatal("resolveAgentProfile() error = nil")
	}
}

func TestResolveAgentProfilesSupportsMultipleACPAgents(t *testing.T) {
	t.Setenv("PROTONMAN_ACP_AGENTS_JSON", `[{"id":"protonman","displayName":"Protonman","command":"protonman","args":["--acp"]},{"id":"antigravity","displayName":"Google Antigravity","command":"agy_acp_server.par"}]`)
	profiles, err := resolveAgentProfiles()
	if err != nil {
		t.Fatal(err)
	}
	if len(profiles) != 2 || profiles[0].ID != "protonman" || profiles[1].ID != "antigravity" {
		t.Fatalf("profiles = %#v", profiles)
	}
}

func TestIsACPMethodNotFound(t *testing.T) {
	if !isACPMethodNotFound(&acpclient.RPCError{Code: -32601}) {
		t.Fatal("method-not-found error was not recognized")
	}
	if isACPMethodNotFound(&acpclient.RPCError{Code: -32000}) {
		t.Fatal("server error was recognized as method-not-found")
	}
}

func TestACPAgentProfilesPersistAndLoadFromPreferences(t *testing.T) {
	t.Setenv("PROTONMAN_ACP_AGENTS_JSON", "")
	prefs := &testPreferences{values: make(map[string]string)}
	profiles := map[string]agentProfile{
		"antigravity": {ID: "antigravity", DisplayName: "Google Antigravity", Command: acpclient.CommandSpec{Path: "/opt/agy/agy_acp_server.par"}},
	}
	if err := persistACPAgentProfiles(prefs, profiles); err != nil {
		t.Fatal(err)
	}
	var stored []configuredAgent
	if err := json.Unmarshal([]byte(prefs.String(agentProfilesPreferencesKey)), &stored); err != nil {
		t.Fatal(err)
	}
	loaded, err := resolveAgentProfilesForPreferences(prefs)
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded) != 1 || loaded[0].ID != stored[0].ID || loaded[0].Command.Path != profiles["antigravity"].Command.Path {
		t.Fatalf("loaded = %#v, stored = %#v", loaded, stored)
	}
}
