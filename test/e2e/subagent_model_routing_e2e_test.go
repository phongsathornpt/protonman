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
	child.AddTextResponse(`Child inspected hello.txt.
<proton-subagent-result>{"conclusion":"Child inspected hello.txt.","findings":[{"claim":"hello.txt was inspected","confidence":"high","evidence":[{"tool":"read","target":"hello.txt"},{"tool":"read","target":"missing.txt"}]}],"blockers":[]}</proton-subagent-result>`)

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
		if !requestMessagesContain(request, `<proton-system-prompt version="10">`) {
			t.Fatalf("child request %d missing Prompt ABI v10: %#v", i, request["messages"])
		}
	}

	primaryRequests := primary.Requests()
	if len(primaryRequests) < 2 || len(primaryRequests) > 3 {
		t.Fatalf("primary requests = %d, want 2-3 event-driven rounds without lifecycle polling", len(primaryRequests))
	}
	finalRequest := primaryRequests[len(primaryRequests)-1]
	runtimeContext := ""
	for _, message := range requestMessages(t, finalRequest) {
		content, _ := message["content"].(string)
		if strings.Contains(content, `proton-runtime-context kind="subagent-results"`) {
			runtimeContext = content
			break
		}
	}
	if runtimeContext == "" {
		t.Fatalf("final primary request missing automatically delivered child result: %#v", finalRequest["messages"])
	}
	for _, want := range []string{`"conclusion":"Child inspected hello.txt."`, `"claim":"hello.txt was inspected"`, `"confidence":"high"`, `"tool":"read"`, `"target":"hello.txt"`} {
		if !strings.Contains(runtimeContext, want) {
			t.Fatalf("runtime context missing %q: %s", want, runtimeContext)
		}
	}
	for _, forbidden := range []string{`<proton-subagent-result>`, `"summary"`, `missing.txt`} {
		if strings.Contains(runtimeContext, forbidden) {
			t.Fatalf("runtime context leaked %q: %s", forbidden, runtimeContext)
		}
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
