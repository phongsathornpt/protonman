package e2e_test

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestE2ESubagentUsesConfiguredProjectModelRoute(t *testing.T) {
	ws := newTestWorkspace(t)
	home := newTestHome(t)
	primary := newMockLLMServer(t)
	child := newMockLLMServer(t)

	primary.AddToolCallResponse("delegate-1", "subagent", `{"action":"spawn","task":"Inspect hello.txt","profile":"agility","timeout_seconds":30}`)
	primary.AddTextResponse("Delegation complete from automatic child result.")
	primary.AddTextResponse("Delegation complete from automatic child result.")

	child.AddToolCallResponse("child-read-1", "read", `{"path":"hello.txt"}`)
	child.AddTextResponse("Child inspected hello.txt.")

	userConfig := fmt.Sprintf(`[model]
default = "universal-model"
provider = "primary"

[providers.primary]
type = "openai"
base_url = %q
api_key = "primary-key"

[providers.fast]
type = "openai"
base_url = %q
api_key = "fast-key"
`, primary.URL(), child.URL())
	if err := os.WriteFile(filepath.Join(home, ".protonman", "config.toml"), []byte(userConfig), 0o644); err != nil {
		t.Fatalf("write user config: %v", err)
	}

	projectDir := filepath.Join(ws, ".protonman")
	if err := os.MkdirAll(projectDir, 0o755); err != nil {
		t.Fatalf("create project config dir: %v", err)
	}
	projectConfig := `[agent.subagents.agility]
provider = "fast"
model = "agility-model"
reasoning_effort = "low"
`
	if err := os.WriteFile(filepath.Join(projectDir, "config.toml"), []byte(projectConfig), 0o644); err != nil {
		t.Fatalf("write project config: %v", err)
	}

	res := runProton(t, runOptions{
		args: []string{"-y", "-p", "Delegate inspection of hello.txt and report the result"},
		dir:  ws,
		env:  []string{"PROTONMAN_HOME=" + home, "PROTONMAN_TRUST_PROJECT=1"},
	})
	if res.exitCode != 0 {
		t.Fatalf("proton failed: stdout=%s stderr=%s", res.stdout, res.stderr)
	}

	childRequests := child.Requests()
	if len(childRequests) < 2 {
		t.Fatalf("child requests = %d, want at least 2", len(childRequests))
	}
	for i, request := range childRequests {
		if got, _ := request["model"].(string); got != "agility-model" {
			t.Fatalf("child request %d model = %q, want agility-model", i, got)
		}
		if got, _ := request["reasoning_effort"].(string); got != "low" {
			t.Fatalf("child request %d reasoning_effort = %q, want low", i, got)
		}
	}

	primaryRequests := primary.Requests()
	if len(primaryRequests) < 2 || len(primaryRequests) > 3 {
		t.Fatalf("primary requests = %d, want 2-3 event-driven rounds without lifecycle polling", len(primaryRequests))
	}
	finalRequest := primaryRequests[len(primaryRequests)-1]
	if !requestMessagesContain(finalRequest, "Child inspected hello.txt.") || !requestMessagesContain(finalRequest, "proton-runtime-context") {
		t.Fatalf("final primary request missing automatically delivered child result: %#v", finalRequest["messages"])
	}
	for i, request := range primaryRequests {
		if got, _ := request["model"].(string); got != "universal-model" {
			t.Fatalf("primary request %d model = %q, want universal-model", i, got)
		}
		if got, exists := request["reasoning_effort"]; exists {
			t.Fatalf("primary request %d unexpectedly inherited child reasoning: %#v", i, got)
		}
	}
	for i, request := range primaryRequests {
		payload, err := json.Marshal(request["messages"])
		if err != nil {
			t.Fatalf("marshal primary request %d messages: %v", i, err)
		}
		for _, action := range []string{`\"action\":\"wait\"`, `\"action\":\"get\"`, `\"action\":\"list\"`} {
			if strings.Contains(string(payload), action) {
				t.Fatalf("primary request %d contains lifecycle polling action %s: %s", i, action, payload)
			}
		}
	}
}
