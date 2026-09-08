package config

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/phongsathornpt/protonman/internal/app/appdirs"
)

func TestLoadHomeWorkspaceDoesNotAliasUserConfigAsProjectConfig(t *testing.T) {
	home := t.TempDir()
	configPath := filepath.Join(home, appdirs.RootDirName, appdirs.ConfigFileName)
	writeConfig(t, configPath, `[permission]
default = "ask"

[[permission.rules]]
tool = "edit"
action = "ask"

[workspace]
protected_paths = [".env"]

[agent]
max_tool_calls = 17
`)

	for _, trusted := range []bool{false, true} {
		t.Run(map[bool]string{false: "untrusted", true: "trusted"}[trusted], func(t *testing.T) {
			snapshot, err := Load(context.Background(), Options{
				HomeDir:        home,
				WorkDir:        home,
				ProjectTrusted: trusted,
			})
			if err != nil {
				t.Fatalf("Load() error = %v", err)
			}
			if got, want := snapshot.Sources, []string{configPath}; len(got) != len(want) || got[0] != want[0] {
				t.Fatalf("Sources = %v, want %v", got, want)
			}
			if len(snapshot.Warnings) != 0 {
				t.Fatalf("Warnings = %v, want none", snapshot.Warnings)
			}
			if got := snapshot.Provenance[FieldAgentMaxToolCalls]; got != SourceUser {
				t.Fatalf("max_tool_calls provenance = %q, want %q", got, SourceUser)
			}
			if got := len(snapshot.Permission.Rules); got != 1 {
				t.Fatalf("permission rule count = %d, want 1", got)
			}
			if got := len(snapshot.ProtectedPaths); got != 1 {
				t.Fatalf("protected path count = %d, want 1", got)
			}
			if snapshot.Agent.MaxToolCalls != 17 {
				t.Fatalf("max_tool_calls = %d, want 17", snapshot.Agent.MaxToolCalls)
			}
		})
	}
}
