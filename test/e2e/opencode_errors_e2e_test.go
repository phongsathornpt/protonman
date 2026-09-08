package e2e_test

import (
	"strings"
	"testing"
)

func TestE2EOpenCodeModelNotFoundWithFuzzySuggestions(t *testing.T) {
	ws := newTestWorkspace(t)
	home := newTestHome(t)

	server := newMockLLMServer(t)
	server.SetupWorkspaceConfig(t, home)

	// Simulate OpenCode Zen 401 ModelError
	server.AddErrorResponse(401, `{"type":"error","error":{"type":"ModelError","message":"Model gemini-2.5-flash is not supported"}}`, "application/json")

	res := runProton(t, runOptions{
		args: []string{"-y", "-p", "Trigger model error"},
		dir:  ws,
		env:  []string{"PROTONMAN_HOME=" + home},
	})
	if res.exitCode == 0 {
		t.Fatalf("expected non-zero exit on model error, got 0: %s", res.stdout)
	}

	combined := res.stdout + res.stderr
	if !strings.Contains(combined, "ModelError") || !strings.Contains(combined, "not supported") {
		t.Fatalf("expected ModelError in output, got: %s", combined)
	}
}

func TestE2EOpenCodeContextOverflow(t *testing.T) {
	ws := newTestWorkspace(t)
	home := newTestHome(t)

	server := newMockLLMServer(t)
	server.SetupWorkspaceConfig(t, home)

	// Simulate context overflow error (413 or message pattern)
	server.AddErrorResponse(400, `{"error":{"message":"maximum context length is 128000 tokens, but your request resulted in 135000 tokens"}}`, "application/json")

	res := runProton(t, runOptions{
		args: []string{"-y", "-p", "Trigger overflow"},
		dir:  ws,
		env:  []string{"PROTONMAN_HOME=" + home},
	})
	if res.exitCode == 0 {
		t.Fatalf("expected non-zero exit on context overflow, got 0: %s", res.stdout)
	}

	combined := res.stdout + res.stderr
	if !strings.Contains(combined, "maximum context length") && !strings.Contains(combined, "tokens") {
		t.Fatalf("expected context length error, got: %s", combined)
	}
}

func TestE2EOpenCodeAuthAndForbiddenErrors(t *testing.T) {
	ws := newTestWorkspace(t)
	home := newTestHome(t)

	// 1. 401 Auth Error
	serverAuth := newMockLLMServer(t)
	serverAuth.SetupWorkspaceConfig(t, home)
	serverAuth.AddErrorResponse(401, `{"error":{"message":"Incorrect API key provided"}}`, "application/json")

	resAuth := runProton(t, runOptions{
		args: []string{"-y", "-p", "Trigger 401"},
		dir:  ws,
		env:  []string{"PROTONMAN_HOME=" + home},
	})
	if resAuth.exitCode == 0 {
		t.Fatalf("expected non-zero exit on 401, got 0")
	}
	combinedAuth := resAuth.stdout + resAuth.stderr
	if !strings.Contains(combinedAuth, "Incorrect API key") {
		t.Fatalf("expected authentication error, got: %s", combinedAuth)
	}

	// 2. 403 Forbidden Error
	server403 := newMockLLMServer(t)
	server403.SetupWorkspaceConfig(t, home)
	server403.AddErrorResponse(403, `{"error":{"message":"Access denied by organization policy"}}`, "application/json")

	res403 := runProton(t, runOptions{
		args: []string{"-y", "-p", "Trigger 403"},
		dir:  ws,
		env:  []string{"PROTONMAN_HOME=" + home},
	})
	if res403.exitCode == 0 {
		t.Fatalf("expected non-zero exit on 403, got 0")
	}
	combined403 := res403.stdout + res403.stderr
	if !strings.Contains(combined403, "Access denied") {
		t.Fatalf("expected forbidden error, got: %s", combined403)
	}
}

func TestE2EOpenCodeRateLimitAndOverloadErrors(t *testing.T) {
	ws := newTestWorkspace(t)
	home := newTestHome(t)

	// 1. 429 Rate Limit (persistent to exhaust retries)
	server429 := newMockLLMServer(t)
	server429.SetupWorkspaceConfig(t, home)
	server429.AddPersistentErrorResponse(429, `{"error":{"message":"Rate limit reached: 60 requests per minute"}}`, "application/json")

	res429 := runProton(t, runOptions{
		args: []string{"-y", "-p", "Trigger 429"},
		dir:  ws,
		env:  []string{"PROTONMAN_HOME=" + home},
	})
	if res429.exitCode == 0 {
		t.Fatalf("expected non-zero exit on 429, got 0")
	}
	combined429 := res429.stdout + res429.stderr
	if !strings.Contains(combined429, "Rate limit") && !strings.Contains(combined429, "429") {
		t.Fatalf("expected rate limit error, got: %s", combined429)
	}

	// 2. 503 Overloaded (persistent to exhaust retries)
	server503 := newMockLLMServer(t)
	server503.SetupWorkspaceConfig(t, home)
	server503.AddPersistentErrorResponse(503, `{"error":{"message":"The server is temporarily overloaded"}}`, "application/json")

	res503 := runProton(t, runOptions{
		args: []string{"-y", "-p", "Trigger 503"},
		dir:  ws,
		env:  []string{"PROTONMAN_HOME=" + home},
	})
	if res503.exitCode == 0 {
		t.Fatalf("expected non-zero exit on 503, got 0")
	}
	combined503 := res503.stdout + res503.stderr
	if !strings.Contains(combined503, "overloaded") && !strings.Contains(combined503, "503") {
		t.Fatalf("expected server overloaded error, got: %s", combined503)
	}
}

func TestE2EOpenCodeHTMLProxyGatewayError(t *testing.T) {
	ws := newTestWorkspace(t)
	home := newTestHome(t)

	server := newMockLLMServer(t)
	server.SetupWorkspaceConfig(t, home)

	// Cloudflare 502 Bad Gateway HTML page (persistent to exhaust retries)
	htmlBody := `<!DOCTYPE html><html><head><title>502 Bad Gateway</title></head><body><center><h1>502 Bad Gateway</h1></center><hr><center>cloudflare</center></body></html>`
	server.AddPersistentErrorResponse(502, htmlBody, "text/html")

	res := runProton(t, runOptions{
		args: []string{"-y", "-p", "Trigger 502 HTML"},
		dir:  ws,
		env:  []string{"PROTONMAN_HOME=" + home},
	})
	if res.exitCode == 0 {
		t.Fatalf("expected non-zero exit on HTML gateway error, got 0")
	}
	combined := res.stdout + res.stderr
	if !strings.Contains(combined, "Bad Gateway") && !strings.Contains(combined, "502") {
		t.Fatalf("expected Bad Gateway error, got: %s", combined)
	}
}
