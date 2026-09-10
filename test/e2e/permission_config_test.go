package e2e_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/phongsathornpt/protonman/internal/adapter/out/config"
	"github.com/phongsathornpt/protonman/internal/adapter/out/model"
	"github.com/phongsathornpt/protonman/internal/app"
)

func TestE2EHeadlessAskModeFailsClosedWithoutPrompt(t *testing.T) {
	ws := newTestWorkspace(t)
	home := newTestHome(t)

	// Run without -y (default mode is ask)
	res := runProton(t, runOptions{
		args: []string{"-p", `/call bash {"command":"echo hello"}`},
		dir:  ws,
		env:  []string{"PROTONMAN_HOME=" + home},
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
	configPath := filepath.Join(home, ".protonman", "config.toml")
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
		env:  []string{"PROTONMAN_HOME=" + home},
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

	// Create project-local .protonman/config.toml denying bash
	projectProton := filepath.Join(ws, ".protonman")
	if err := os.MkdirAll(projectProton, 0o755); err != nil {
		t.Fatalf("mkdir project .protonman: %v", err)
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

	// Case 1: Untrusted (PROTONMAN_TRUST_PROJECT unset)
	// Project config is ignored with a warning; bash succeeds with -y
	untrustedRes := runProton(t, runOptions{
		args: []string{"-y", "-p", `/call bash {"command":"echo untrusted-ok"}`},
		dir:  ws,
		env:  []string{"PROTONMAN_HOME=" + home},
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

	// Case 2: Trusted (PROTONMAN_TRUST_PROJECT=1)
	// Project config is loaded, bash is denied
	trustedRes := runProton(t, runOptions{
		args: []string{"-y", "-p", `/call bash {"command":"echo should-deny"}`},
		dir:  ws,
		env: []string{
			"PROTONMAN_HOME=" + home,
			"PROTONMAN_TRUST_PROJECT=1",
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
	configFile := filepath.Join(home, ".protonman", "config.toml")
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

	// Step 4: Run compiled proton CLI binary with PROTONMAN_HOME and verify clean startup
	res := runProton(t, runOptions{
		args: []string{"-y", "-p", `/call bash {"command":"echo config-verified"}`},
		dir:  ws,
		env:  []string{"PROTONMAN_HOME=" + home},
	})
	if res.exitCode != 0 || !strings.Contains(res.stdout, "config-verified") {
		t.Fatalf("proton binary failed to boot with saved config: code %d, out: %s, err: %s", res.exitCode, res.stdout, res.stderr)
	}
}

func TestE2EOpenCodeFreeProviderConfig(t *testing.T) {
	ws := newTestWorkspace(t)
	home := newTestHome(t)

	// Step 1: Save OpenCode provider with zero API key
	prov := config.ProviderConfig{
		Name:    model.DefaultOpenCodeName,
		Type:    "openai",
		BaseURL: model.DefaultOpenCodeEndpoint,
		APIKey:  "",
	}
	defaultModel := "nemotron-3.5-lightning-free"
	err := config.SaveUserProviderConfig(home, prov, defaultModel)
	if err != nil {
		t.Fatalf("SaveUserProviderConfig for OpenCode error = %v", err)
	}

	// Step 2: Verify file existence and 0600 permissions
	configFile := filepath.Join(home, ".protonman", "config.toml")
	info, err := os.Stat(configFile)
	if err != nil {
		t.Fatalf("stat config file error = %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Fatalf("config file permissions = %o, want 0600", perm)
	}

	content, err := os.ReadFile(configFile)
	if err != nil {
		t.Fatalf("read config file: %v", err)
	}
	fileStr := string(content)
	if !strings.Contains(fileStr, "https://opencode.ai/zen/v1") {
		t.Fatalf("config missing OpenCode endpoint, got:\n%s", fileStr)
	}
	if !strings.Contains(fileStr, defaultModel) {
		t.Fatalf("config missing default free model, got:\n%s", fileStr)
	}

	// Step 3: Load config via config.Load and verify parsed struct
	loaded, err := config.Load(context.Background(), config.Options{
		HomeDir: home,
		WorkDir: ws,
	})
	if err != nil {
		t.Fatalf("config.Load error = %v", err)
	}
	opencodeProv, ok := loaded.Providers["opencode"]
	if !ok {
		t.Fatalf("loaded config missing 'opencode' provider: %+v", loaded.Providers)
	}
	if opencodeProv.BaseURL != model.DefaultOpenCodeEndpoint {
		t.Fatalf("loaded endpoint %q, want %q", opencodeProv.BaseURL, model.DefaultOpenCodeEndpoint)
	}
	if loaded.Model.Default != defaultModel {
		t.Fatalf("loaded default model %q, want %q", loaded.Model.Default, defaultModel)
	}
	if !model.IsFreeModel(loaded.Model.Default) {
		t.Fatalf("expected IsFreeModel(%q) to be true", loaded.Model.Default)
	}

	// Step 4: Boot proton CLI binary with OpenCode config
	res := runProton(t, runOptions{
		args: []string{"-y", "-p", `/call bash {"command":"echo opencode-e2e-ok"}`},
		dir:  ws,
		env:  []string{"PROTONMAN_HOME=" + home},
	})
	if res.exitCode != 0 || !strings.Contains(res.stdout, "opencode-e2e-ok") {
		t.Fatalf("proton binary failed with OpenCode config: code %d, out: %s, err: %s", res.exitCode, res.stdout, res.stderr)
	}
}

func TestE2EProviderSwitchAndSelect(t *testing.T) {
	ws := newTestWorkspace(t)
	home := newTestHome(t)

	// Step 1: Save two providers (protonman and opencode)
	pmProv := config.ProviderConfig{
		Name:    "protonman",
		Type:    "openai",
		BaseURL: "https://api.protonman.dev/v1",
		APIKey:  "pm_key_123",
	}
	if err := config.SaveUserProviderConfig(home, pmProv, "deepseek-v4-flash-vision-exp"); err != nil {
		t.Fatalf("SaveUserProviderConfig(protonman) error: %v", err)
	}

	ocProv := config.ProviderConfig{
		Name:    "opencode",
		Type:    "openai",
		BaseURL: "https://opencode.ai/zen/v1",
		APIKey:  "",
	}
	if err := config.SaveUserProviderConfig(home, ocProv, "nemotron-3.5-lightning-free"); err != nil {
		t.Fatalf("SaveUserProviderConfig(opencode) error: %v", err)
	}

	// Step 2: Switch active provider to opencode via SaveUserDefaultProvider
	if err := config.SaveUserDefaultProvider(home, "opencode"); err != nil {
		t.Fatalf("SaveUserDefaultProvider(opencode) error: %v", err)
	}

	// Step 3: Check permissions on ~/.protonman/config.toml (must be 0600)
	configFile := filepath.Join(home, ".protonman", "config.toml")
	info, err := os.Stat(configFile)
	if err != nil {
		t.Fatalf("stat config file error: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Fatalf("config file permissions = %o, want 0600", perm)
	}

	// Step 4: Verify snapshot reflects active provider opencode and preserves both providers
	snap, err := config.Load(context.Background(), config.Options{
		HomeDir: home,
		WorkDir: ws,
	})
	if err != nil {
		t.Fatalf("config.Load error: %v", err)
	}
	if snap.Model.Provider != "opencode" {
		t.Fatalf("active provider = %q, want 'opencode'", snap.Model.Provider)
	}
	if len(snap.Providers) != 2 {
		t.Fatalf("expected 2 providers in snapshot, got %d", len(snap.Providers))
	}

	// Step 5: Switch active provider back to protonman
	if err := config.SaveUserDefaultProvider(home, "protonman"); err != nil {
		t.Fatalf("SaveUserDefaultProvider(protonman) error: %v", err)
	}

	snap2, err := config.Load(context.Background(), config.Options{
		HomeDir: home,
		WorkDir: ws,
	})
	if err != nil {
		t.Fatalf("config.Load error: %v", err)
	}
	if snap2.Model.Provider != "protonman" {
		t.Fatalf("active provider = %q, want 'protonman'", snap2.Model.Provider)
	}

	// Step 6: Verify Protonman boots cleanly with updated config
	res := runProton(t, runOptions{
		args: []string{"-y", "-p", `/call bash {"command":"echo provider-switch-verified"}`},
		dir:  ws,
		env:  []string{"PROTONMAN_HOME=" + home},
	})
	if res.exitCode != 0 || !strings.Contains(res.stdout, "provider-switch-verified") {
		t.Fatalf("proton binary failed after provider switch: code %d, out: %s, err: %s", res.exitCode, res.stdout, res.stderr)
	}

	// Step 7: Edit provider details and verify update
	updatedPM := config.ProviderConfig{
		Name:    "protonman",
		Type:    "openai",
		BaseURL: "https://api-v2.protonman.dev/v1",
		APIKey:  "pm_key_updated_456",
	}
	if err := config.SaveUserProviderConfig(home, updatedPM, "glm-5.3-flash"); err != nil {
		t.Fatalf("edit SaveUserProviderConfig error: %v", err)
	}
	snap3, err := config.Load(context.Background(), config.Options{HomeDir: home, WorkDir: ws})
	if err != nil {
		t.Fatalf("config.Load error: %v", err)
	}
	if snap3.Providers["protonman"].BaseURL != "https://api-v2.protonman.dev/v1" {
		t.Fatalf("expected updated endpoint, got %s", snap3.Providers["protonman"].BaseURL)
	}

	// Step 8: Delete provider and verify fallback
	if err := config.DeleteUserProviderConfig(home, "protonman"); err != nil {
		t.Fatalf("DeleteUserProviderConfig error: %v", err)
	}
	snap4, err := config.Load(context.Background(), config.Options{HomeDir: home, WorkDir: ws})
	if err != nil {
		t.Fatalf("config.Load error: %v", err)
	}
	if _, exists := snap4.Providers["protonman"]; exists {
		t.Fatal("expected protonman removed")
	}
	if snap4.Model.Provider != "" || snap4.Model.Default != "" {
		t.Fatalf("expected low-level delete to clear active selection, got %+v", snap4.Model)
	}
	resolved, _ := app.ResolvePrimaryModelDefaults(snap4.Model, snap4.Providers)
	if resolved.Provider != model.DefaultOpenCodeName || resolved.Default != model.DefaultOpenCodeModel {
		t.Fatalf("application fallback = %+v, want OpenCode default", resolved)
	}

	// Verify permissions remain 0600
	infoAfter, err := os.Stat(configFile)
	if err != nil {
		t.Fatalf("stat config file error: %v", err)
	}
	if perm := infoAfter.Mode().Perm(); perm != 0o600 {
		t.Fatalf("config file permissions = %o, want 0600", perm)
	}
}
