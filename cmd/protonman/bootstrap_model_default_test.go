package main

import (
	"context"
	"os"
	"testing"

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
}
