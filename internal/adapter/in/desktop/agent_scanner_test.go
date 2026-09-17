//go:build desktop

package desktop

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestScanACPAgentsDiscoversProtonmanAndCustom(t *testing.T) {
	ctx := context.Background()
	lookPath := func(name string) (string, error) {
		switch name {
		case "protonman":
			return "/usr/local/bin/protonman", nil
		case "goose":
			return "/usr/local/bin/goose", nil
		default:
			return "", errors.New("not found")
		}
	}
	getenv := func(key string) string {
		switch key {
		case "ANTIGRAVITY_ACP_COMMAND":
			return "/opt/agy/agy_acp_server.par"
		case "ANTIGRAVITY_ACP_ARGS_JSON":
			return `["--uid=desktop"]`
		default:
			return ""
		}
	}

	discovered := scanACPAgentsWith(ctx, lookPath, getenv)
	if len(discovered) < 2 {
		t.Fatalf("expected at least 2 discovered agents, got %d: %#v", len(discovered), discovered)
	}

	foundProtonman := false
	foundAntigravity := false
	for _, a := range discovered {
		if a.ID == "protonman" {
			foundProtonman = true
			if len(a.Args) != 1 || a.Args[0] != "--acp" {
				t.Fatalf("unexpected protonman args: %#v", a.Args)
			}
		}
		if a.ID == "antigravity" {
			foundAntigravity = true
			if len(a.Args) != 1 || a.Args[0] != "--uid=desktop" {
				t.Fatalf("unexpected antigravity args: %#v", a.Args)
			}
		}
	}

	if !foundProtonman {
		t.Fatal("protonman not discovered")
	}
	if !foundAntigravity {
		t.Fatal("antigravity not discovered")
	}
}

func TestDiscoveredAgentOptionLabel(t *testing.T) {
	agent := DiscoveredAgent{
		ID:          "goose",
		DisplayName: "Block Goose",
		Command:     "/usr/local/bin/goose",
		Args:        []string{"acp"},
	}
	label := agent.OptionLabel()
	if label != "Block Goose (goose acp)" {
		t.Fatalf("unexpected option label %q", label)
	}
}

func TestSanitizeAgentID(t *testing.T) {
	if got := sanitizeAgentID("agy_acp_server.par"); got != "agy_acp_server" {
		t.Fatalf("unexpected id: %q", got)
	}
	if got := sanitizeAgentID("my-tool-acp.exe"); got != "my-tool-acp" {
		t.Fatalf("unexpected id: %q", got)
	}
}

func TestIsLikelyACPBinaryName(t *testing.T) {
	positives := []string{
		"acp",
		"acp.exe",
		"acp-server",
		"acp_server",
		"agy_acp_server.par",
		"goose-acp",
		"tool_acp",
		"custom-acp-agent",
	}
	for _, name := range positives {
		if !isLikelyACPBinaryName(name) {
			t.Errorf("expected %q to be recognized as ACP binary", name)
		}
	}

	negatives := []string{
		"acpi",
		"acpid",
		"pacparser",
		"macports",
		"bash",
		"node",
		"python",
	}
	for _, name := range negatives {
		if isLikelyACPBinaryName(name) {
			t.Errorf("expected %q to NOT be recognized as ACP binary", name)
		}
	}
}

func TestIsProtonmanExecutable(t *testing.T) {
	if !isProtonmanExecutable("/usr/local/bin/protonman") {
		t.Fatal("protonman should be recognized")
	}
	if !isProtonmanExecutable("protonman.exe") {
		t.Fatal("protonman.exe should be recognized")
	}
	if isProtonmanExecutable("protonman-desktop") {
		t.Fatal("protonman-desktop should NOT be recognized as the CLI")
	}
}

func TestFormatDisplayName(t *testing.T) {
	if got := formatDisplayName("agy_acp_server.par"); got != "Agy Acp Server" {
		t.Fatalf("unexpected display name: %q", got)
	}
	if got := formatDisplayName("claude-code"); got != "Claude Code" {
		t.Fatalf("unexpected display name: %q", got)
	}
}

func TestScanMachineACPAgentsLive(t *testing.T) {
	discovered := ScanMachineACPAgents()
	// At least protonman or another CLI should be found on this dev machine
	t.Logf("Live scan discovered %d ACP agents:", len(discovered))
	for _, d := range discovered {
		t.Logf("- %s: %s %s", d.DisplayName, d.Command, strings.Join(d.Args, " "))
	}
}
