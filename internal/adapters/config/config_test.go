package config

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/projectTHORN/proton/internal/domain/permission"
)

func TestLoadLayeredConfigRequiresProjectTrust(t *testing.T) {
	homeDir := t.TempDir()
	workDir := t.TempDir()
	writeConfig(t, filepath.Join(homeDir, ".proton", "config.toml"), `[permission]
default = "ask"

[[permission.rules]]
action = "allow"
tool = "read"
pattern = "*.md"

[ui]
permission_mode = "auto"
`)
	writeConfig(t, filepath.Join(workDir, ".proton", "config.toml"), `[permission]

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
	writeConfig(t, filepath.Join(homeDir, ".proton", "config.toml"), `[permission]

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

func writeConfig(t *testing.T, path string, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
}
