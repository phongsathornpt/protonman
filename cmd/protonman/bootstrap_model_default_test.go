package main

import (
	"context"
	"os"
	"testing"

	"github.com/phongsathornpt/protonman/internal/adapter/out/config"
	"github.com/phongsathornpt/protonman/internal/adapter/out/model"
)

func TestBuildRuntimeUsesOpenCodeDefaultModelOnFirstRun(t *testing.T) {
	home := t.TempDir()
	work := t.TempDir()
	oldHome := os.Getenv("PROTONMAN_HOME")
	oldTelemetry := os.Getenv("PROTONMAN_TELEMETRY")
	oldWD, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		_ = os.Setenv("PROTONMAN_HOME", oldHome)
		_ = os.Setenv("PROTONMAN_TELEMETRY", oldTelemetry)
		_ = os.Chdir(oldWD)
	}()
	_ = os.Setenv("PROTONMAN_HOME", home)
	_ = os.Setenv("PROTONMAN_TELEMETRY", "off")
	if err := os.Chdir(work); err != nil {
		t.Fatal(err)
	}

	runtime, err := buildRuntime(context.Background(), cliOptions{})
	if err != nil {
		t.Fatalf("buildRuntime() error = %v", err)
	}
	defer runtime.Close()
	if runtime.config.Model.Provider != model.DefaultOpenCodeName {
		t.Fatalf("provider = %q, want %q", runtime.config.Model.Provider, model.DefaultOpenCodeName)
	}
	if runtime.config.Model.Default != model.DefaultOpenCodeModel {
		t.Fatalf("model = %q, want %q", runtime.config.Model.Default, model.DefaultOpenCodeModel)
	}
	if runtime.runner == nil {
		t.Fatal("default OpenCode model did not create a conversation runner")
	}
	if runtime.config.Provenance[config.FieldModelProvider] != config.SourceDefault || runtime.config.Provenance[config.FieldModelDefault] != config.SourceDefault {
		t.Fatalf("default provenance = provider:%q model:%q", runtime.config.Provenance[config.FieldModelProvider], runtime.config.Provenance[config.FieldModelDefault])
	}
}

func TestBuildRuntimeRepairsOpenCodeSelectionWithoutModel(t *testing.T) {
	home := t.TempDir()
	work := t.TempDir()
	oldHome := os.Getenv("PROTONMAN_HOME")
	oldTelemetry := os.Getenv("PROTONMAN_TELEMETRY")
	oldWD, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		_ = os.Setenv("PROTONMAN_HOME", oldHome)
		_ = os.Setenv("PROTONMAN_TELEMETRY", oldTelemetry)
		_ = os.Chdir(oldWD)
	}()
	_ = os.Setenv("PROTONMAN_HOME", home)
	_ = os.Setenv("PROTONMAN_TELEMETRY", "off")
	if err := os.Chdir(work); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(home+"/.protonman", 0o700); err != nil {
		t.Fatal(err)
	}
	configText := `[providers.opencode]
name = "opencode"
type = "openai"
base_url = "https://opencode.ai/zen/v1"

[model]
provider = "opencode"
`
	if err := os.WriteFile(home+"/.protonman/config.toml", []byte(configText), 0o600); err != nil {
		t.Fatal(err)
	}

	runtime, err := buildRuntime(context.Background(), cliOptions{})
	if err != nil {
		t.Fatalf("buildRuntime() error = %v", err)
	}
	defer runtime.Close()
	if runtime.config.Model.Default != model.DefaultOpenCodeModel || runtime.runner == nil {
		t.Fatalf("repaired model=%q runnerNil=%v", runtime.config.Model.Default, runtime.runner == nil)
	}
	if runtime.config.Provenance[config.FieldModelProvider] != config.SourceUser {
		t.Fatalf("provider provenance = %q, want user", runtime.config.Provenance[config.FieldModelProvider])
	}
	if runtime.config.Provenance[config.FieldModelDefault] != config.SourceDefault {
		t.Fatalf("model provenance = %q, want default", runtime.config.Provenance[config.FieldModelDefault])
	}
}

func TestBuildRuntimeFallsBackToOpenCodeWhenSavedProviderIsMissing(t *testing.T) {
	home := t.TempDir()
	work := t.TempDir()
	oldHome := os.Getenv("PROTONMAN_HOME")
	oldTelemetry := os.Getenv("PROTONMAN_TELEMETRY")
	oldWD, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		_ = os.Setenv("PROTONMAN_HOME", oldHome)
		_ = os.Setenv("PROTONMAN_TELEMETRY", oldTelemetry)
		_ = os.Chdir(oldWD)
	}()
	_ = os.Setenv("PROTONMAN_HOME", home)
	_ = os.Setenv("PROTONMAN_TELEMETRY", "off")
	if err := os.Chdir(work); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(home+"/.protonman", 0o700); err != nil {
		t.Fatal(err)
	}
	configText := `[providers.9router]
name = "9router"
type = "openai"
base_url = "https://router.example/v1"
api_key = "test-key"

[providers.opencode]
name = "opencode"
type = "openai"
base_url = "https://opencode.ai/zen/v1"

[providers.protonman]
name = "protonman"
type = "openai"
base_url = "https://protonman.dev/api/v1"
api_key = "test-key"

[model]
default = "manager"
provider = "beta"
`
	if err := os.WriteFile(home+"/.protonman/config.toml", []byte(configText), 0o600); err != nil {
		t.Fatal(err)
	}
	runtime, err := buildRuntime(context.Background(), cliOptions{})
	if err != nil {
		t.Fatalf("buildRuntime() error = %v", err)
	}
	defer runtime.Close()
	if runtime.config.Model.Provider != model.DefaultOpenCodeName {
		t.Fatalf("provider = %q, want %q", runtime.config.Model.Provider, model.DefaultOpenCodeName)
	}
	if runtime.config.Model.Default != model.DefaultOpenCodeModel {
		t.Fatalf("model = %q, want %q", runtime.config.Model.Default, model.DefaultOpenCodeModel)
	}
	if runtime.runner == nil {
		t.Fatal("missing saved provider fallback did not create OpenCode runner")
	}
	if runtime.config.Provenance[config.FieldModelProvider] != config.SourceDefault || runtime.config.Provenance[config.FieldModelDefault] != config.SourceDefault {
		t.Fatalf("fallback provenance = provider:%q model:%q", runtime.config.Provenance[config.FieldModelProvider], runtime.config.Provenance[config.FieldModelDefault])
	}
}

func TestBuildRuntimePreservesConfiguredModelFromProtonmanHome(t *testing.T) {
	home := t.TempDir()
	work := t.TempDir()
	oldHome := os.Getenv("PROTONMAN_HOME")
	oldTelemetry := os.Getenv("PROTONMAN_TELEMETRY")
	oldWD, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		_ = os.Setenv("PROTONMAN_HOME", oldHome)
		_ = os.Setenv("PROTONMAN_TELEMETRY", oldTelemetry)
		_ = os.Chdir(oldWD)
	}()
	_ = os.Setenv("PROTONMAN_HOME", home)
	_ = os.Setenv("PROTONMAN_TELEMETRY", "off")
	if err := os.Chdir(work); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(home+"/.protonman", 0o700); err != nil {
		t.Fatal(err)
	}
	configText := `[providers.custom]
name = "custom"
type = "openai"
base_url = "https://custom.example/v1"
api_key = "test-key"

[model]
default = "custom-model"
provider = "custom"
`
	if err := os.WriteFile(home+"/.protonman/config.toml", []byte(configText), 0o600); err != nil {
		t.Fatal(err)
	}
	runtime, err := buildRuntime(context.Background(), cliOptions{})
	if err != nil {
		t.Fatalf("buildRuntime() error = %v", err)
	}
	defer runtime.Close()
	if runtime.config.Model.Provider != "custom" {
		t.Fatalf("provider = %q, want custom", runtime.config.Model.Provider)
	}
	if runtime.config.Model.Default != "custom-model" {
		t.Fatalf("model = %q, want custom-model", runtime.config.Model.Default)
	}
	if runtime.runner == nil {
		t.Fatal("configured .protonman model did not create a conversation runner")
	}
	if runtime.config.Provenance[config.FieldModelProvider] != config.SourceUser || runtime.config.Provenance[config.FieldModelDefault] != config.SourceUser {
		t.Fatalf("configured provenance = provider:%q model:%q", runtime.config.Provenance[config.FieldModelProvider], runtime.config.Provenance[config.FieldModelDefault])
	}
}
