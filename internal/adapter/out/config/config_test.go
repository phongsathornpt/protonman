package config

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/phongsathornpt/protonman/internal/core/permission"
	"github.com/phongsathornpt/protonman/internal/platform/sandbox"
	sdk "github.com/phongsathornpt/protonman/proton-sdk"
)

func TestLoadLayeredConfigRequiresProjectTrust(t *testing.T) {
	homeDir := t.TempDir()
	workDir := t.TempDir()
	writeConfig(t, filepath.Join(homeDir, ".protonman", "config.toml"), `[permission]
default = "ask"

[[permission.rules]]
action = "allow"
tool = "read"
pattern = "*.md"

[ui]
permission_mode = "auto"
`)
	writeConfig(t, filepath.Join(workDir, ".protonman", "config.toml"), `[permission]

[[permission.rules]]
tool = "bash"
pattern = "rm *"
`)

	untrusted, err := Load(context.Background(), Options{
		HomeDir: homeDir,
		WorkDir: workDir,
	})
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if len(untrusted.Permission.Rules) != 1 {
		t.Fatalf("untrusted rules = %d, want 1", len(untrusted.Permission.Rules))
	}
	if untrusted.Mode != permission.ModeAuto {
		t.Fatalf("untrusted mode = %s, want auto", untrusted.Mode)
	}
	if len(untrusted.Warnings) != 1 {
		t.Fatalf("untrusted warnings = %d, want 1", len(untrusted.Warnings))
	}

	trusted, err := Load(context.Background(), Options{
		HomeDir:        homeDir,
		WorkDir:        workDir,
		ProjectTrusted: true,
	})
	if err != nil {
		t.Fatalf("trusted Load() error = %v", err)
	}
	if len(trusted.Permission.Rules) != 2 {
		t.Fatalf("trusted rules = %d, want 2", len(trusted.Permission.Rules))
	}
	if trusted.Permission.Rules[1].Action != permission.ActionDeny {
		t.Fatalf("omitted project action = %s, want deny", trusted.Permission.Rules[1].Action)
	}
}

func TestLoadRejectsUnknownRuleFields(t *testing.T) {
	homeDir := t.TempDir()
	writeConfig(t, filepath.Join(homeDir, ".protonman", "config.toml"), `[permission]

[[permission.rules]]
action = "maybe"
tool = "bash"
`)

	_, err := Load(context.Background(), Options{
		HomeDir: homeDir,
		WorkDir: t.TempDir(),
	})
	if err == nil {
		t.Fatal("Load() error = nil, want invalid action error")
	}
}

func TestLoadProtectedPathsWithProjectTrust(t *testing.T) {
	homeDir := t.TempDir()
	workDir := t.TempDir()
	writeConfig(t, filepath.Join(homeDir, ".protonman", "config.toml"), `[workspace]
protected_paths = [".env", "secrets"]
`)
	writeConfig(t, filepath.Join(workDir, ".protonman", "config.toml"), `[workspace]
protected_paths = ["**/*.pem"]
`)

	untrusted, err := Load(context.Background(), Options{HomeDir: homeDir, WorkDir: workDir})
	if err != nil {
		t.Fatalf("Load() untrusted error = %v", err)
	}
	if got, want := len(untrusted.ProtectedPaths), 2; got != want {
		t.Fatalf("untrusted protected paths = %d, want %d", got, want)
	}

	trusted, err := Load(context.Background(), Options{
		HomeDir:        homeDir,
		WorkDir:        workDir,
		ProjectTrusted: true,
	})
	if err != nil {
		t.Fatalf("Load() trusted error = %v", err)
	}
	if got, want := len(trusted.ProtectedPaths), 3; got != want {
		t.Fatalf("trusted protected paths = %d, want %d", got, want)
	}
}

func TestLoadSandboxProfile(t *testing.T) {
	homeDir := t.TempDir()
	writeConfig(t, filepath.Join(homeDir, ".protonman", "config.toml"), `[sandbox]
profile = "strict"
`)
	snapshot, err := Load(context.Background(), Options{
		HomeDir: homeDir,
		WorkDir: t.TempDir(),
	})
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if snapshot.Sandbox != sandbox.NameStrict {
		t.Fatalf("sandbox = %s, want strict", snapshot.Sandbox)
	}
}

func writeConfig(t *testing.T, path string, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
}

func TestSaveAndLoadUserProviderConfig(t *testing.T) {
	homeDir := t.TempDir()
	workDir := t.TempDir()

	provider := ProviderConfig{
		Name:    "protonman",
		Type:    "openai",
		BaseURL: "https://protonman.dev/api/v1",
		APIKey:  "plk_test_12345",
	}

	err := SaveUserProviderConfig(homeDir, provider, "deepseek-v4-flash-vision-exp")
	if err != nil {
		t.Fatalf("SaveUserProviderConfig() error = %v", err)
	}

	snapshot, err := Load(context.Background(), Options{
		HomeDir: homeDir,
		WorkDir: workDir,
	})
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	savedProv, ok := snapshot.Providers["protonman"]
	if !ok {
		t.Fatalf("expected provider 'protonman' in snapshot, got: %v", snapshot.Providers)
	}
	if savedProv.BaseURL != "https://protonman.dev/api/v1" || savedProv.APIKey != "plk_test_12345" {
		t.Fatalf("unexpected provider config: %+v", savedProv)
	}
	if snapshot.Model.Default != "deepseek-v4-flash-vision-exp" || snapshot.Model.Provider != "protonman" {
		t.Fatalf("unexpected model config: %+v", snapshot.Model)
	}
}

func TestSaveUserDefaultModel(t *testing.T) {
	homeDir := t.TempDir()
	workDir := t.TempDir()

	// 1. Initial provider save
	provider := ProviderConfig{
		Name:    "protonman",
		Type:    "openai",
		BaseURL: "https://protonman.dev/api/v1",
		APIKey:  "plk_test_12345",
	}
	if err := SaveUserProviderConfig(homeDir, provider, "deepseek-v4-flash-vision-exp"); err != nil {
		t.Fatalf("SaveUserProviderConfig() error = %v", err)
	}

	// 2. Switch default model without altering provider credentials
	if err := SaveUserDefaultModel(homeDir, "protonman", "MiniMax-M3"); err != nil {
		t.Fatalf("SaveUserDefaultModel() error = %v", err)
	}

	snapshot, err := Load(context.Background(), Options{
		HomeDir: homeDir,
		WorkDir: workDir,
	})
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	savedProv, ok := snapshot.Providers["protonman"]
	if !ok || savedProv.APIKey != "plk_test_12345" {
		t.Fatalf("provider credentials should be preserved: %+v", savedProv)
	}
	if snapshot.Model.Default != "MiniMax-M3" || snapshot.Model.Provider != "protonman" {
		t.Fatalf("model was not updated, got: %+v", snapshot.Model)
	}
}

func TestSaveUserProviderConfigWithOptionsPreservesActiveProvider(t *testing.T) {
	homeDir := t.TempDir()
	workDir := t.TempDir()

	active := ProviderConfig{
		Name:    "protonman",
		Type:    "openai",
		BaseURL: "https://protonman.dev/api/v1",
		APIKey:  "active-key",
	}
	if err := SaveUserProviderConfig(homeDir, active, "active-model"); err != nil {
		t.Fatalf("save active provider: %v", err)
	}

	secondary := ProviderConfig{
		Name:    "custom-gateway",
		Type:    "openai",
		BaseURL: "https://custom.example.com/v1",
		APIKey:  "secondary-key",
	}
	if err := SaveUserProviderConfigWithOptions(homeDir, secondary, ProviderSaveOptions{
		DefaultModel: "secondary-model",
	}); err != nil {
		t.Fatalf("save inactive provider: %v", err)
	}

	snapshot, err := Load(context.Background(), Options{HomeDir: homeDir, WorkDir: workDir})
	if err != nil {
		t.Fatalf("load after inactive provider save: %v", err)
	}
	if snapshot.Model.Provider != "protonman" || snapshot.Model.Default != "active-model" {
		t.Fatalf("inactive provider save changed active defaults: %+v", snapshot.Model)
	}

	renamed := secondary
	renamed.Name = "custom-renamed"
	if err := SaveUserProviderConfigWithOptions(homeDir, renamed, ProviderSaveOptions{
		PreviousName: secondary.Name,
	}); err != nil {
		t.Fatalf("rename inactive provider: %v", err)
	}

	snapshot, err = Load(context.Background(), Options{HomeDir: homeDir, WorkDir: workDir})
	if err != nil {
		t.Fatalf("load after provider rename: %v", err)
	}
	if _, exists := snapshot.Providers["custom-gateway"]; exists {
		t.Fatal("expected old provider key to be removed after rename")
	}
	if got := snapshot.Providers["custom-renamed"]; got.BaseURL != renamed.BaseURL {
		t.Fatalf("expected renamed provider to be saved, got %+v", got)
	}
	if snapshot.Model.Provider != "protonman" {
		t.Fatalf("provider rename changed active provider: %q", snapshot.Model.Provider)
	}
}

func TestDeleteUserProviderConfig(t *testing.T) {
	homeDir := t.TempDir()
	workDir := t.TempDir()

	p1 := ProviderConfig{
		Name:    "protonman",
		Type:    "openai",
		BaseURL: "https://protonman.dev/api/v1",
		APIKey:  "plk_test_12345",
	}
	p2 := ProviderConfig{
		Name:    "opencode",
		Type:    "openai",
		BaseURL: "https://opencode.ai/zen/v1",
	}
	if err := SaveUserProviderConfig(homeDir, p1, "deepseek"); err != nil {
		t.Fatalf("SaveUserProviderConfig p1: %v", err)
	}
	if err := SaveUserProviderConfig(homeDir, p2, "nemotron"); err != nil {
		t.Fatalf("SaveUserProviderConfig p2: %v", err)
	}
	if err := SaveUserDefaultProvider(homeDir, "protonman"); err != nil {
		t.Fatalf("SaveUserDefaultProvider: %v", err)
	}

	// Delete protonman
	if err := DeleteUserProviderConfig(homeDir, "protonman"); err != nil {
		t.Fatalf("DeleteUserProviderConfig error: %v", err)
	}

	snapshot, err := Load(context.Background(), Options{
		HomeDir: homeDir,
		WorkDir: workDir,
	})
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}

	if _, exists := snapshot.Providers["protonman"]; exists {
		t.Fatal("expected protonman to be deleted")
	}
	if _, exists := snapshot.Providers["opencode"]; !exists {
		t.Fatal("expected opencode to be preserved")
	}
	if snapshot.Model.Provider != "opencode" {
		t.Fatalf("expected default provider to fall back to opencode, got: %q", snapshot.Model.Provider)
	}

	// Verify permissions
	configFile := filepath.Join(homeDir, ".protonman", "config.toml")
	info, err := os.Stat(configFile)
	if err != nil {
		t.Fatalf("stat config file: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Fatalf("permissions = %o, want 0600", perm)
	}
}

func TestSaveUserConfigRejectsCorruptExistingFile(t *testing.T) {
	homeDir := t.TempDir()
	configPath := filepath.Join(homeDir, ".protonman", "config.toml")
	corruptContent := "this is [not valid toml ::::"
	writeConfig(t, configPath, corruptContent)

	// Attempt to save default model should fail and not overwrite corrupt file
	err := SaveUserDefaultModel(homeDir, "anthropic", "claude-3-5-sonnet")
	if err == nil {
		t.Fatal("expected error saving to corrupt config file, got nil")
	}

	// Verify content was not modified
	data, readErr := os.ReadFile(configPath)
	if readErr != nil {
		t.Fatalf("failed to read config file: %v", readErr)
	}
	if string(data) != corruptContent {
		t.Fatalf("corrupt config was overwritten; got %q, want %q", string(data), corruptContent)
	}
}

func TestLoadAgentProfileConfig(t *testing.T) {
	homeDir := t.TempDir()
	configPath := filepath.Join(homeDir, ".protonman", "config.toml")
	writeConfig(t, configPath, `[agent]
profile = "dex"
`)

	snapshot, err := Load(context.Background(), Options{HomeDir: homeDir, WorkDir: t.TempDir()})
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if snapshot.Agent.Profile != "dex" {
		t.Errorf("Agent.Profile = %q, want 'dex'", snapshot.Agent.Profile)
	}
}

func TestAgentLimitsRejectNegativeValues(t *testing.T) {
	for _, test := range []struct {
		name  string
		field string
	}{
		{name: "tool calls", field: "max_tool_calls"},
	} {
		t.Run(test.name, func(t *testing.T) {
			homeDir := t.TempDir()
			writeConfig(t, filepath.Join(homeDir, ".protonman", "config.toml"), "[agent]\n"+test.field+" = -1\n")

			_, err := Load(context.Background(), Options{HomeDir: homeDir, WorkDir: t.TempDir()})
			if err == nil {
				t.Fatalf("Load() error = nil, want negative %s rejection", test.field)
			}
		})
	}

	if err := SaveUserMaxToolCalls(t.TempDir(), -1); err == nil {
		t.Fatal("SaveUserMaxToolCalls(-1) error = nil, want rejection")
	}
}

func TestSubagentsEnabledDefaultsAndLayering(t *testing.T) {
	homeDir := t.TempDir()
	workDir := t.TempDir()

	snapshot, err := Load(context.Background(), Options{HomeDir: homeDir, WorkDir: workDir})
	if err != nil {
		t.Fatal(err)
	}
	if !snapshot.Agent.SubagentsEnabled {
		t.Fatal("subagents should be enabled by default")
	}
	if got := snapshot.Provenance[FieldAgentSubagentsEnabled]; got != SourceDefault {
		t.Fatalf("default provenance = %q, want %q", got, SourceDefault)
	}

	if err := SaveUserSubagentsEnabled(homeDir, false); err != nil {
		t.Fatal(err)
	}
	snapshot, err = Load(context.Background(), Options{HomeDir: homeDir, WorkDir: workDir})
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.Agent.SubagentsEnabled {
		t.Fatal("user config should disable subagents")
	}
	if got := snapshot.Provenance[FieldAgentSubagentsEnabled]; got != SourceUser {
		t.Fatalf("user provenance = %q, want %q", got, SourceUser)
	}

	if err := SaveProjectSubagentsEnabled(workDir, true); err != nil {
		t.Fatal(err)
	}
	snapshot, err = Load(context.Background(), Options{HomeDir: homeDir, WorkDir: workDir, ProjectTrusted: true})
	if err != nil {
		t.Fatal(err)
	}
	if !snapshot.Agent.SubagentsEnabled {
		t.Fatal("trusted project config should re-enable subagents")
	}
	if got := snapshot.Provenance[FieldAgentSubagentsEnabled]; got != SourceProject {
		t.Fatalf("project provenance = %q, want %q", got, SourceProject)
	}
}

func TestSaveSubagentsEnabledPersistsBoolean(t *testing.T) {
	homeDir := t.TempDir()
	if err := SaveUserSubagentsEnabled(homeDir, false); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(homeDir, ".protonman", "config.toml"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "subagents_enabled = false") {
		t.Fatalf("saved config missing boolean subagent setting:\n%s", data)
	}
}

func TestAgentSubagentTimeoutConfig(t *testing.T) {
	homeDir := t.TempDir()
	workDir := t.TempDir()
	writeConfig(t, filepath.Join(homeDir, ".protonman", "config.toml"), `[agent]
subagent_max_runtime = "45s"
subagent_wait_timeout = "9s"
subagent_queue_timeout = "7s"
max_live_subagents = 8
max_retained_subagents = 24
completed_result_ttl = "2m"
`)
	snapshot, err := Load(context.Background(), Options{HomeDir: homeDir, WorkDir: workDir})
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if snapshot.Agent.SubagentMaxRuntime != 45*time.Second {
		t.Fatalf("max runtime = %v", snapshot.Agent.SubagentMaxRuntime)
	}
	if snapshot.Agent.SubagentWaitTimeout != 9*time.Second {
		t.Fatalf("wait timeout = %v", snapshot.Agent.SubagentWaitTimeout)
	}
	if snapshot.Agent.SubagentQueueTimeout != 7*time.Second {
		t.Fatalf("queue timeout = %v", snapshot.Agent.SubagentQueueTimeout)
	}
	if snapshot.Agent.MaxLiveSubagents != 8 {
		t.Fatalf("max live = %d", snapshot.Agent.MaxLiveSubagents)
	}
	if snapshot.Agent.MaxRetainedSubagents != 24 {
		t.Fatalf("max retained = %d", snapshot.Agent.MaxRetainedSubagents)
	}
	if snapshot.Agent.CompletedResultTTL != 2*time.Minute {
		t.Fatalf("result ttl = %v", snapshot.Agent.CompletedResultTTL)
	}
}

func TestAgentLegacySubagentTimeoutMigratesToMaxRuntime(t *testing.T) {
	homeDir := t.TempDir()
	writeConfig(t, filepath.Join(homeDir, ".protonman", "config.toml"), "[agent]\nsubagent_timeout = \"45s\"\n")
	snapshot, err := Load(context.Background(), Options{HomeDir: homeDir, WorkDir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.Agent.SubagentMaxRuntime != 45*time.Second {
		t.Fatalf("max runtime = %v", snapshot.Agent.SubagentMaxRuntime)
	}
	found := false
	for _, warning := range snapshot.Warnings {
		if strings.Contains(warning, "subagent_timeout is deprecated") {
			found = true
		}
	}
	if !found {
		t.Fatal("missing legacy timeout deprecation warning")
	}
}

func TestAgentSubagentTimeoutConfigRejectsInvalidValues(t *testing.T) {
	for _, body := range []string{
		"[agent]\nsubagent_max_runtime = \"nope\"\n",
		"[agent]\nsubagent_wait_timeout = \"0s\"\n",
		"[agent]\nsubagent_queue_timeout = \"-1s\"\n",
		"[agent]\ncompleted_result_ttl = \"0s\"\n",
		"[agent]\nmax_live_subagents = 0\n",
		"[agent]\nmax_retained_subagents = 0\n",
	} {
		homeDir := t.TempDir()
		workDir := t.TempDir()
		writeConfig(t, filepath.Join(homeDir, ".protonman", "config.toml"), body)
		if _, err := Load(context.Background(), Options{HomeDir: homeDir, WorkDir: workDir}); err == nil {
			t.Fatalf("Load(%q) error = nil, want invalid duration", body)
		}
	}
}

func TestRuntimeDefaultTurnTimeoutIsDisabled(t *testing.T) {
	homeDir := t.TempDir()
	workDir := t.TempDir()
	snapshot, err := Load(context.Background(), Options{HomeDir: homeDir, WorkDir: workDir})
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if snapshot.Runtime.TurnTimeout != 0 {
		t.Fatalf("turn timeout = %v, want disabled", snapshot.Runtime.TurnTimeout)
	}
}

func TestRuntimeConfigOverridesDefaults(t *testing.T) {
	homeDir := t.TempDir()
	workDir := t.TempDir()
	writeConfig(t, filepath.Join(homeDir, ".protonman", "config.toml"), `[runtime]
turn_timeout = "3m"
round_timeout = "45s"
tool_permission_timeout = "30s"
tool_execution_timeout = "90s"
model_request_timeout = "4m"
model_discovery_timeout = "8s"
webFetchTimeout = "12s"
model_catalog_ttl = "75s"
`)
	snapshot, err := Load(context.Background(), Options{HomeDir: homeDir, WorkDir: workDir})
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	checks := map[string]struct{ got, want time.Duration }{
		"turn":            {snapshot.Runtime.TurnTimeout, 3 * time.Minute},
		"round":           {snapshot.Runtime.RoundTimeout, 45 * time.Second},
		"permission":      {snapshot.Runtime.ToolPermissionTimeout, 30 * time.Second},
		"execution":       {snapshot.Runtime.ToolExecutionTimeout, 90 * time.Second},
		"model request":   {snapshot.Runtime.ModelRequestTimeout, 4 * time.Minute},
		"model discovery": {snapshot.Runtime.ModelDiscoveryTimeout, 8 * time.Second},
		"web fetch":       {snapshot.Runtime.WebFetchTimeout, 12 * time.Second},
		"catalog ttl":     {snapshot.Runtime.ModelCatalogTTL, 75 * time.Second},
	}
	for name, check := range checks {
		if check.got != check.want {
			t.Errorf("%s = %v, want %v", name, check.got, check.want)
		}
	}
}

func TestAgentReasoningEffortConfig(t *testing.T) {
	homeDir := t.TempDir()
	workDir := t.TempDir()
	writeConfig(t, filepath.Join(homeDir, ".protonman", "config.toml"), `[agent]
reasoning_effort = "high"
`)
	snapshot, err := Load(context.Background(), Options{HomeDir: homeDir, WorkDir: workDir})
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.Agent.ReasoningEffort != sdk.ReasoningHigh {
		t.Fatalf("reasoning_effort = %q, want high", snapshot.Agent.ReasoningEffort)
	}
	if err := SaveUserReasoningEffort(homeDir, sdk.ReasoningDefault); err != nil {
		t.Fatal(err)
	}
	snapshot, err = Load(context.Background(), Options{HomeDir: homeDir, WorkDir: workDir})
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.Agent.ReasoningEffort != sdk.ReasoningDefault {
		t.Fatalf("saved auto reasoning_effort = %q", snapshot.Agent.ReasoningEffort)
	}
}

func TestAgentReasoningEffortConfigRejectsUnknownLevel(t *testing.T) {
	homeDir := t.TempDir()
	workDir := t.TempDir()
	writeConfig(t, filepath.Join(homeDir, ".protonman", "config.toml"), `[agent]
reasoning_effort = "turbo"
`)
	if _, err := Load(context.Background(), Options{HomeDir: homeDir, WorkDir: workDir}); err == nil {
		t.Fatal("Load() error = nil")
	}
}

func TestLoadTracksSelectedFieldProvenance(t *testing.T) {
	homeDir := t.TempDir()
	workDir := t.TempDir()
	writeConfig(t, filepath.Join(homeDir, ".protonman", "config.toml"), `[model]
default = "user-model"
provider = "user-provider"

[agent]
profile = "pow"
max_tool_calls = 11
reasoning_effort = "low"

[ui]
permission_mode = "ask"
`)
	writeConfig(t, filepath.Join(workDir, ".protonman", "config.toml"), `[model]
default = "project-model"

[agent]
profile = "dex"
reasoning_effort = "high"
`)

	snapshot, err := Load(context.Background(), Options{HomeDir: homeDir, WorkDir: workDir, ProjectTrusted: true})
	if err != nil {
		t.Fatal(err)
	}
	for field, want := range map[string]ValueSource{
		FieldModelDefault:         SourceProject,
		FieldModelProvider:        SourceUser,
		FieldAgentProfile:         SourceProject,
		FieldAgentReasoningEffort: SourceProject,
		FieldAgentMaxToolCalls:    SourceUser,
		FieldUIPermissionMode:     SourceUser,
	} {
		if got := snapshot.Provenance[field]; got != want {
			t.Fatalf("provenance[%s] = %q, want %q", field, got, want)
		}
	}
}

func TestSaveUserPermissionRuleRoundTrip(t *testing.T) {
	homeDir := t.TempDir()
	workDir := t.TempDir()

	rule := permission.Rule{
		Action:      permission.ActionAllow,
		Tool:        permission.ToolRead,
		Pattern:     "*.go",
		PatternMode: permission.PatternModeGlob,
	}

	if err := SaveUserPermissionRule(homeDir, rule); err != nil {
		t.Fatalf("SaveUserPermissionRule error: %v", err)
	}

	// Saving identical rule is deduplicated
	if err := SaveUserPermissionRule(homeDir, rule); err != nil {
		t.Fatalf("SaveUserPermissionRule duplicate error: %v", err)
	}

	snapshot, err := Load(context.Background(), Options{HomeDir: homeDir, WorkDir: workDir})
	if err != nil {
		t.Fatalf("Load error: %v", err)
	}

	if len(snapshot.Permission.Rules) != 1 {
		t.Fatalf("expected 1 rule, got %d", len(snapshot.Permission.Rules))
	}
	r := snapshot.Permission.Rules[0]
	if r.Action != permission.ActionAllow || r.Tool != permission.ToolRead || r.Pattern != "*.go" {
		t.Fatalf("unexpected saved rule: %+v", r)
	}
}

func TestLoadPermissionRuleAllPattern(t *testing.T) {
	homeDir := t.TempDir()
	workDir := t.TempDir()

	writeConfig(t, filepath.Join(homeDir, ".protonman", "config.toml"), `[permission]
default = "ask"

[[permission.rules]]
action = "allow"
tool = "bash"
pattern = "all"
`)

	snapshot, err := Load(context.Background(), Options{HomeDir: homeDir, WorkDir: workDir})
	if err != nil {
		t.Fatalf("Load error: %v", err)
	}

	if len(snapshot.Permission.Rules) != 1 {
		t.Fatalf("expected 1 rule, got %d", len(snapshot.Permission.Rules))
	}
	r := snapshot.Permission.Rules[0]
	if r.Action != permission.ActionAllow || r.Tool != permission.ToolBash || r.Pattern != "*" {
		t.Fatalf("unexpected rule decoded: %+v", r)
	}
}
