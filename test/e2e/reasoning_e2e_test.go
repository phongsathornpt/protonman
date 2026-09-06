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
	server.AddToolCallResponse("reason-read", "read_file", `{"file_path":"hello.txt"}`)
	server.AddTextResponse("done")

	res := runProton(t, runOptions{args: []string{"-y", "-p", "Inspect hello.txt and answer done"}, dir: ws, env: []string{"PROTON_HOME=" + home}})
	if res.exitCode != 0 {
		t.Fatalf("Gemini reasoning run failed: %s %s", res.stdout, res.stderr)
	}
	requests := server.Requests()
	if len(requests) < 1 || requests[0]["reasoning_effort"] != "high" {
		t.Fatalf("Gemini request reasoning = %#v", requests)
	}
	if !requestMessagesContain(requests[0], "reasoning_effective=high") || !requestMessagesContain(requests[0], "reasoning_source=agent_profile") {
		t.Fatalf("Gemini prompt missing reasoning provenance: %#v", requests[0]["messages"])
	}
	assertNoSamplingControls(t, requests)
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
