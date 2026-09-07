package e2e_test

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestE2EReasoningGeminiProfileReachesWire(t *testing.T) {
	ws, home := newTestWorkspace(t), newTestHome(t)
	server := newMockLLMServer(t)
	writeReasoningConfig(t, home, server.URL(), "openai", "gemini-3.8-flash", "dex", "")
	server.AddToolCallResponse("reason-read", "read_file", `{"path":"hello.txt"}`)
	server.AddTextResponse("done")

	res := runProton(t, runOptions{args: []string{"-y", "-p", "Inspect hello.txt and answer done"}, dir: ws, env: []string{"PROTON_HOME=" + home}})
	if res.exitCode != 0 {
		t.Fatalf("Gemini reasoning run failed: %s %s", res.stdout, res.stderr)
	}
	requests := server.Requests()
	if len(requests) < 1 || requests[0]["reasoning_effort"] != "high" {
		t.Fatalf("Gemini request reasoning = %#v", requests)
	}
	if !requestMessagesContain(requests[0], "# Grounding Contract") ||
		!requestMessagesContain(requests[0], "Use tool names exactly as provided") ||
		!requestMessagesContain(requests[0], "primary coding agent") {
		t.Fatalf("Gemini prompt missing stable grounding/model guidance: %#v", requests[0]["messages"])
	}
	for _, leaked := range []string{"reasoning_effective=", "reasoning_source=", "model_profile=", "model_profile_match=", "provider="} {
		if requestMessagesContain(requests[0], leaked) {
			t.Fatalf("Gemini prompt leaked runtime metadata %q: %#v", leaked, requests[0]["messages"])
		}
	}
	initialTools := requestToolNames(requests[0])
	if !containsString(initialTools, "read_file") {
		t.Fatalf("initial grounding tools missing read_file: %#v", initialTools)
	}
	for _, forbidden := range []string{"get_todo", "update_todo", "delegate_task", "bash", "write_file", "apply_patch"} {
		if containsString(initialTools, forbidden) {
			t.Fatalf("initial grounding tools unexpectedly include %s: %#v", forbidden, initialTools)
		}
	}
	if len(requests) < 2 || !containsString(requestToolNames(requests[1]), "get_todo") || !containsString(requestToolNames(requests[1]), "delegate_task") {
		t.Fatalf("post-grounding request did not restore full tool set: %#v", requests)
	}
	updateSchema := requestToolParameters(requests[1], "update_todo")
	if len(updateSchema) == 0 {
		t.Fatalf("Gemini request missing update_todo schema: %#v", requests[1]["tools"])
	}
	for _, forbidden := range []string{"oneOf", "const", "additionalProperties"} {
		if schemaContainsKey(updateSchema, forbidden) {
			t.Fatalf("Gemini update_todo schema contains unsupported %s: %#v", forbidden, updateSchema)
		}
	}
	opSchema := updateSchema["properties"].(map[string]any)["operations"].(map[string]any)["items"].(map[string]any)["properties"].(map[string]any)["op"].(map[string]any)
	if !containsAnyString(opSchema["enum"], "add") || !containsAnyString(opSchema["enum"], "set_status") ||
		!containsAnyString(opSchema["enum"], "set_text") || !containsAnyString(opSchema["enum"], "remove") {
		t.Fatalf("Gemini update_todo op enum = %#v", opSchema["enum"])
	}
	assertNoSamplingControls(t, requests)
}

func TestE2EGeminiOpenAIGetTodoEmptySnapshot(t *testing.T) {
	ws, home := newTestWorkspace(t), newTestHome(t)
	server := newMockLLMServer(t)
	writeReasoningConfig(t, home, server.URL(), "openai", "gemini-3.8-flash", "dex", "")
	server.AddToolCallResponse("todo-ground", "read_file", `{"path":"hello.txt"}`)
	server.AddToolCallResponse("todo-empty", "get_todo", `{}`)
	server.AddTextResponse("done")

	res := runProton(t, runOptions{
		args: []string{"-y", "-p", "Inspect hello.txt, read the current task plan, then answer done"},
		dir:  ws,
		env:  []string{"PROTON_HOME=" + home},
	})
	if res.exitCode != 0 {
		t.Fatalf("Gemini get_todo empty snapshot run failed: %s %s", res.stdout, res.stderr)
	}
	requests := server.Requests()
	if len(requests) != 3 {
		t.Fatalf("request count = %d, want 3: %#v", len(requests), requests)
	}
	if !requestMessagesContain(requests[2], `"items":[]`) {
		t.Fatalf("follow-up request missing empty todo array structured output: %#v", requests[2]["messages"])
	}
}

func TestE2EReasoningGeminiRejectsUnsupportedExplicitLevelBeforeHTTP(t *testing.T) {
	ws, home := newTestWorkspace(t), newTestHome(t)
	server := newMockLLMServer(t)
	writeReasoningConfig(t, home, server.URL(), "openai", "gemini-3.8-flash", "", "xhigh")

	res := runProton(t, runOptions{args: []string{"-y", "-p", "Say done"}, dir: ws, env: []string{"PROTON_HOME=" + home}})
	if res.exitCode == 0 {
		t.Fatalf("unsupported Gemini xhigh unexpectedly succeeded: %s", res.stdout)
	}
	if got := len(server.Requests()); got != 0 {
		t.Fatalf("provider received %d requests before local rejection", got)
	}
	if combined := res.stdout + res.stderr; !strings.Contains(combined, "does not support reasoning effort") {
		t.Fatalf("missing local reasoning error: %s", combined)
	}
}

func TestE2EReasoningOpenAIResponsesEncoding(t *testing.T) {
	ws, home := newTestWorkspace(t), newTestHome(t)
	server := newMockLLMServer(t)
	writeReasoningConfig(t, home, server.URL(), "openai", "gpt-5.6-responses", "", "high")
	server.AddResponsesTextResponse("done")

	res := runProton(t, runOptions{args: []string{"-y", "-p", "Say done"}, dir: ws, env: []string{"PROTON_HOME=" + home}})
	if res.exitCode != 0 {
		t.Fatalf("Responses reasoning run failed: %s %s", res.stdout, res.stderr)
	}
	requests := server.Requests()
	if len(requests) != 1 {
		t.Fatalf("Responses request count = %d", len(requests))
	}
	reasoning, _ := requests[0]["reasoning"].(map[string]any)
	if reasoning["effort"] != "high" {
		t.Fatalf("Responses reasoning = %#v", reasoning)
	}
	assertNoSamplingControls(t, requests)
}

func TestE2EReasoningAnthropicAdaptiveEncoding(t *testing.T) {
	ws, home := newTestWorkspace(t), newTestHome(t)
	server := newMockLLMServer(t)
	writeReasoningConfig(t, home, server.URL(), "anthropic", "claude-sonnet-4-6", "", "high")
	server.AddAnthropicTextResponse("done")

	res := runProton(t, runOptions{args: []string{"-y", "-p", "Say done"}, dir: ws, env: []string{"PROTON_HOME=" + home}})
	if res.exitCode != 0 {
		t.Fatalf("Anthropic reasoning run failed: %s %s", res.stdout, res.stderr)
	}
	requests := server.Requests()
	if len(requests) != 1 {
		t.Fatalf("Anthropic request count = %d", len(requests))
	}
	thinking, _ := requests[0]["thinking"].(map[string]any)
	outputConfig, _ := requests[0]["output_config"].(map[string]any)
	if thinking["type"] != "adaptive" || outputConfig["effort"] != "high" {
		t.Fatalf("Anthropic reasoning payload = thinking %#v output_config %#v", thinking, outputConfig)
	}
	assertNoSamplingControls(t, requests)
}

func writeReasoningConfig(t *testing.T, home, baseURL, providerType, modelID, profile, effort string) {
	t.Helper()
	protonDir := filepath.Join(home, ".proton")
	if err := os.MkdirAll(protonDir, 0o755); err != nil {
		t.Fatal(err)
	}
	agentLines := ""
	if profile != "" || effort != "" {
		agentLines = "\n[agent]\n"
		if profile != "" {
			agentLines += fmt.Sprintf("profile = %q\n", profile)
		}
		if effort != "" {
			agentLines += fmt.Sprintf("reasoning_effort = %q\n", effort)
		}
	}
	config := fmt.Sprintf(`[model]
default = %q
provider = "protonman"

[providers.protonman]
type = %q
api_key = "mock-api-key"
base_url = %q
%s`, modelID, providerType, baseURL, agentLines)
	if err := os.WriteFile(filepath.Join(protonDir, "config.toml"), []byte(config), 0o600); err != nil {
		t.Fatal(err)
	}
}

func assertNoSamplingControls(t *testing.T, requests []map[string]any) {
	t.Helper()
	for index, request := range requests {
		for _, key := range []string{"temperature", "top_p", "top_k"} {
			if _, exists := request[key]; exists {
				t.Fatalf("request %d unexpectedly contains %s: %#v", index, key, request)
			}
		}
	}
}

func requestToolNames(request map[string]any) []string {
	raw, ok := request["tools"].([]any)
	if !ok {
		return nil
	}
	names := make([]string, 0, len(raw))
	for _, item := range raw {
		toolObject, ok := item.(map[string]any)
		if !ok {
			continue
		}
		if function, ok := toolObject["function"].(map[string]any); ok {
			if name, ok := function["name"].(string); ok {
				names = append(names, name)
			}
			continue
		}
		if name, ok := toolObject["name"].(string); ok {
			names = append(names, name)
		}
	}
	return names
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func requestToolParameters(request map[string]any, name string) map[string]any {
	raw, _ := request["tools"].([]any)
	for _, item := range raw {
		toolObject, _ := item.(map[string]any)
		if function, ok := toolObject["function"].(map[string]any); ok {
			if function["name"] == name {
				params, _ := function["parameters"].(map[string]any)
				return params
			}
			continue
		}
		if toolObject["name"] == name {
			params, _ := toolObject["parameters"].(map[string]any)
			return params
		}
	}
	return nil
}

func schemaContainsKey(value any, want string) bool {
	switch typed := value.(type) {
	case map[string]any:
		for key, child := range typed {
			if key == want || schemaContainsKey(child, want) {
				return true
			}
		}
	case []any:
		for _, child := range typed {
			if schemaContainsKey(child, want) {
				return true
			}
		}
	}
	return false
}

func containsAnyString(raw any, want string) bool {
	values, _ := raw.([]any)
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
