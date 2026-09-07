package config

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/projectTHORN/proton/internal/app/appdirs"
	"github.com/projectTHORN/proton/internal/permission"
	sdk "github.com/projectTHORN/proton/proton-sdk"
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
