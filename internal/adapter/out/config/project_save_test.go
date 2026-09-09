package config

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/phongsathornpt/protonman/internal/app/appdirs"
	"github.com/phongsathornpt/protonman/internal/core/permission"
	sdk "github.com/phongsathornpt/protonman/proton-sdk"
)

func TestSaveProjectSettingsRoundTrip(t *testing.T) {
	workDir := t.TempDir()
	if err := SaveProjectAgentProfile(workDir, "dex"); err != nil {
		t.Fatal(err)
	}
	if err := SaveProjectReasoningEffort(workDir, sdk.ReasoningHigh); err != nil {
		t.Fatal(err)
	}
	if err := SaveProjectMaxToolCalls(workDir, 44); err != nil {
		t.Fatal(err)
	}
	if err := SaveProjectPermissionMode(workDir, permission.ModeAlwaysApprove); err != nil {
		t.Fatal(err)
	}
	snapshot, err := Load(context.Background(), Options{HomeDir: t.TempDir(), WorkDir: workDir, ProjectTrusted: true})
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.Agent.Profile != "dex" || snapshot.Agent.ReasoningEffort != sdk.ReasoningHigh || snapshot.Agent.MaxToolCalls != 44 || snapshot.Mode != permission.ModeAlwaysApprove {
		t.Fatalf("project settings did not round trip: %#v", snapshot)
	}
	for _, field := range []string{FieldAgentProfile, FieldAgentReasoningEffort, FieldAgentMaxToolCalls, FieldUIPermissionMode} {
		if snapshot.Provenance[field] != SourceProject {
			t.Fatalf("%s source = %q", field, snapshot.Provenance[field])
		}
	}
}

func TestSaveProjectPermissionRuleRoundTrip(t *testing.T) {
	workDir := t.TempDir()
	homeDir := t.TempDir()

	rule := permission.Rule{
		Action:      permission.ActionAllow,
		Tool:        permission.ToolBash,
		Pattern:     "git status",
		PatternMode: permission.PatternModeGlob,
	}

	if err := SaveProjectPermissionRule(workDir, rule); err != nil {
		t.Fatalf("SaveProjectPermissionRule error: %v", err)
	}

	// Saving identical rule is deduplicated
	if err := SaveProjectPermissionRule(workDir, rule); err != nil {
		t.Fatalf("SaveProjectPermissionRule duplicate error: %v", err)
	}

	// Saving domain rule
	domainRule := permission.Rule{
		Action:      permission.ActionAllow,
		Tool:        permission.ToolWeb,
		Pattern:     "api.github.com",
		PatternMode: permission.PatternModeDomain,
	}
	if err := SaveProjectPermissionRule(workDir, domainRule); err != nil {
		t.Fatalf("SaveProjectPermissionRule domain error: %v", err)
	}

	snapshot, err := Load(context.Background(), Options{
		HomeDir:        homeDir,
		WorkDir:        workDir,
		ProjectTrusted: true,
	})
	if err != nil {
		t.Fatalf("Load error: %v", err)
	}

	if len(snapshot.Permission.Rules) != 2 {
		t.Fatalf("expected 2 rules, got %d: %#v", len(snapshot.Permission.Rules), snapshot.Permission.Rules)
	}

	r0 := snapshot.Permission.Rules[0]
	if r0.Action != permission.ActionAllow || r0.Tool != permission.ToolBash || r0.Pattern != "git status" || r0.PatternMode != permission.PatternModeGlob {
		t.Fatalf("rule 0 mismatch: %+v", r0)
	}

	r1 := snapshot.Permission.Rules[1]
	if r1.Action != permission.ActionAllow || r1.Tool != permission.ToolWeb || r1.Pattern != "api.github.com" || r1.PatternMode != permission.PatternModeDomain {
		t.Fatalf("rule 1 mismatch: %+v", r1)
	}
}

func TestSaveProjectSettingsRejectsSymlinkRoot(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink behavior varies on Windows")
	}
	workDir := t.TempDir()
	target := t.TempDir()
	if err := os.Symlink(target, appdirs.ProjectRoot(workDir)); err != nil {
		t.Fatal(err)
	}
	if err := SaveProjectMaxToolCalls(workDir, 10); err == nil {
		t.Fatal("expected symlink project root rejection")
	}
}

func TestSaveProjectSettingsRejectsSymlinkConfig(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink behavior varies on Windows")
	}
	workDir := t.TempDir()
	if err := os.Mkdir(appdirs.ProjectRoot(workDir), 0o755); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(t.TempDir(), "outside.toml")
	if err := os.WriteFile(target, []byte("[agent]\nmax_tool_calls = 1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, appdirs.ProjectConfig(workDir)); err != nil {
		t.Fatal(err)
	}
	if err := SaveProjectMaxToolCalls(workDir, 10); err == nil {
		t.Fatal("expected symlink config rejection")
	}
}

func TestSaveProjectSettingsRejectsUserHomeAlias(t *testing.T) {
	home := t.TempDir()
	t.Setenv("PROTONMAN_HOME", home)
	if err := os.Mkdir(filepath.Join(home, appdirs.RootDirName), 0o700); err != nil {
		t.Fatal(err)
	}
	configPath := filepath.Join(home, appdirs.RootDirName, appdirs.ConfigFileName)
	if err := os.WriteFile(configPath, []byte("[agent]\nmax_tool_calls = 7\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	err := SaveProjectMaxToolCalls(home, 99)
	if !errors.Is(err, ErrProjectScopeUnavailable) {
		t.Fatalf("SaveProjectMaxToolCalls() error = %v, want project scope unavailable", err)
	}
	contents, readErr := os.ReadFile(configPath)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if string(contents) != "[agent]\nmax_tool_calls = 7\n" {
		t.Fatalf("user config was modified: %q", contents)
	}
}
