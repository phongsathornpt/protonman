package e2e_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/projectTHORN/proton/internal/config"
)

func TestE2EHeadlessAskModeFailsClosedWithoutPrompt(t *testing.T) {
	ws := newTestWorkspace(t)
	home := newTestHome(t)

	// Run without -y (default mode is ask)
	res := runProton(t, runOptions{
		args: []string{"-p", `/call bash {"command":"echo hello"}`},
		dir:  ws,
		env:  []string{"PROTON_HOME=" + home},
	})
	if res.exitCode == 0 {
		t.Fatalf("expected ask mode to fail closed in headless run, got exit 0: %s", res.stdout)
	}
	combined := res.stdout + res.stderr
	if !strings.Contains(combined, "permission denied") && !strings.Contains(combined, "no permission prompt is configured") {
		t.Fatalf("expected permission denied error, got: %s", combined)
	}
}

func TestE2EGlobalConfigDenyRuleOverridesAlwaysApprove(t *testing.T) {
	ws := newTestWorkspace(t)
	home := newTestHome(t)

	// Write global config.toml with explicit deny rule for bash rm *
	configPath := filepath.Join(home, ".proton", "config.toml")
	configContent := `
[permission]
default = "ask"

[[permission.rules]]
action = "deny"
tool = "bash"
pattern = "rm *"
`
	if err := os.WriteFile(configPath, []byte(configContent), 0o644); err != nil {
		t.Fatalf("write global config: %v", err)
	}

	// Even with -y (always-approve), explicit deny rule must deny
	res := runProton(t, runOptions{
		args: []string{"-y", "-p", `/call bash {"command":"rm foo"}`},
		dir:  ws,
		env:  []string{"PROTON_HOME=" + home},
	})
	if res.exitCode == 0 {
		t.Fatalf("expected explicit deny rule to block call even with -y, got 0: %s", res.stdout)
	}
	combined := res.stdout + res.stderr
	if !strings.Contains(combined, "permission denied") && !strings.Contains(combined, "denied") {
		t.Fatalf("expected denial message, got: %s", combined)
	}
}

func TestE2EProjectTrustGating(t *testing.T) {
	ws := newTestWorkspace(t)
	home := newTestHome(t)

	// Create project-local .proton/config.toml denying bash
	projectProton := filepath.Join(ws, ".proton")
	if err := os.MkdirAll(projectProton, 0o755); err != nil {
		t.Fatalf("mkdir project .proton: %v", err)
	}
	projectConfig := `
[[permission.rules]]
action = "deny"
tool = "bash"
pattern = "*"
`
	if err := os.WriteFile(filepath.Join(projectProton, "config.toml"), []byte(projectConfig), 0o644); err != nil {
		t.Fatalf("write project config: %v", err)
	}

	// Case 1: Untrusted (PROTON_TRUST_PROJECT unset)
	// Project config is ignored with a warning; bash succeeds with -y
	untrustedRes := runProton(t, runOptions{
		args: []string{"-y", "-p", `/call bash {"command":"echo untrusted-ok"}`},
		dir:  ws,
		env:  []string{"PROTON_HOME=" + home},
	})
	if untrustedRes.exitCode != 0 {
		t.Fatalf("expected untrusted project to ignore local config, got code %d: %s\n%s",
			untrustedRes.exitCode, untrustedRes.stdout, untrustedRes.stderr)
	}
	if !strings.Contains(untrustedRes.stdout, "untrusted-ok") {
		t.Fatalf("output missing untrusted-ok: %s", untrustedRes.stdout)
	}
	if !strings.Contains(untrustedRes.stderr, "warning") && !strings.Contains(untrustedRes.stderr, "untrusted") {
		t.Fatalf("expected warning on stderr about untrusted project config, got: %s", untrustedRes.stderr)
	}

	// Case 2: Trusted (PROTON_TRUST_PROJECT=1)
	// Project config is loaded, bash is denied
	trustedRes := runProton(t, runOptions{
		args: []string{"-y", "-p", `/call bash {"command":"echo should-deny"}`},
		dir:  ws,
		env: []string{
			"PROTON_HOME=" + home,
			"PROTON_TRUST_PROJECT=1",
		},
	})
	if trustedRes.exitCode == 0 {
		t.Fatalf("expected trusted project deny rule to take effect, got 0: %s", trustedRes.stdout)
	}
	combined := trustedRes.stdout + trustedRes.stderr
	if !strings.Contains(combined, "permission denied") && !strings.Contains(combined, "denied") {
		t.Fatalf("expected denial message, got: %s", combined)
	}
}

func TestE2EProviderConfigSaveAndReload(t *testing.T) {
	ws := newTestWorkspace(t)
	home := newTestHome(t)

	// Step 1: Save protonman provider configuration
	prov := config.ProviderConfig{
		Name:    "protonman",
		Type:    "openai",
		BaseURL: "https://protonman.dev/api/v1",
		APIKey:  "plk_live_e2e_secret_key_12345",
	}
	err := config.SaveUserProviderConfig(home, prov, "deepseek-v4-flash-vision-exp")
	if err != nil {
		t.Fatalf("SaveUserProviderConfig error = %v", err)
	}

	// Step 2: Verify file existence and permissions on disk
	configFile := filepath.Join(home, ".proton", "config.toml")
	info, err := os.Stat(configFile)
	if err != nil {
		t.Fatalf("stat config file error = %v", err)
	}
	// Check file permissions (should be 0600 for secret safety)
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Fatalf("config file permissions = %o, want 0600", perm)
	}

	content, err := os.ReadFile(configFile)
	if err != nil {
		t.Fatalf("read config file: %v", err)
	}
	fileStr := string(content)
	if !strings.Contains(fileStr, "https://protonman.dev/api/v1") {
		t.Fatalf("config missing endpoint, got:\n%s", fileStr)
	}
	if !strings.Contains(fileStr, "plk_live_e2e_secret_key_12345") {
		t.Fatalf("config missing api key, got:\n%s", fileStr)
	}
	if !strings.Contains(fileStr, "deepseek-v4-flash-vision-exp") {
		t.Fatalf("config missing model, got:\n%s", fileStr)
	}

	// Step 3: Save a second provider and ensure existing providers are preserved
	prov2 := config.ProviderConfig{
		Name:    "custom-gateway",
		Type:    "openai",
		BaseURL: "https://custom.ai/v1",
		APIKey:  "custom_key_67890",
	}
	if err := config.SaveUserProviderConfig(home, prov2, "custom-model"); err != nil {
		t.Fatalf("SaveUserProviderConfig second provider error = %v", err)
	}

	content2, err := os.ReadFile(configFile)
	if err != nil {
		t.Fatalf("read updated config file: %v", err)
	}
	fileStr2 := string(content2)
	// Both providers and the new default model must be present
	if !strings.Contains(fileStr2, "protonman") || !strings.Contains(fileStr2, "custom-gateway") {
		t.Fatalf("both providers should be preserved, got:\n%s", fileStr2)
	}
	if !strings.Contains(fileStr2, "custom-model") {
		t.Fatalf("default model should be updated, got:\n%s", fileStr2)
	}

	// Step 4: Run compiled proton CLI binary with PROTON_HOME and verify clean startup
	res := runProton(t, runOptions{
		args: []string{"-y", "-p", `/call bash {"command":"echo config-verified"}`},
		dir:  ws,
		env:  []string{"PROTON_HOME=" + home},
	})
	if res.exitCode != 0 || !strings.Contains(res.stdout, "config-verified") {
		t.Fatalf("proton binary failed to boot with saved config: code %d, out: %s, err: %s", res.exitCode, res.stdout, res.stderr)
	}
}
