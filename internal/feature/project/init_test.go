package project

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/phongsathornpt/protonman/internal/app/appdirs"
)

func TestInitCreatesMinimalProjectConfigWithoutOverwrite(t *testing.T) {
	workDir := t.TempDir()
	result, err := Init(context.Background(), workDir)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Created {
		t.Fatal("first init did not create config")
	}
	original, err := os.ReadFile(result.ConfigPath)
	if err != nil {
		t.Fatal(err)
	}

	second, err := Init(context.Background(), workDir)
	if err != nil {
		t.Fatal(err)
	}
	if second.Created {
		t.Fatal("second init overwrote existing config")
	}
	current, err := os.ReadFile(result.ConfigPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(current) != string(original) {
		t.Fatalf("config changed on second init: %q", current)
	}
}

func TestInitRejectsSymlinkedProjectRoot(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink creation may require elevated privileges")
	}
	workDir := t.TempDir()
	outside := t.TempDir()
	if err := os.Symlink(outside, appdirs.ProjectRoot(workDir)); err != nil {
		t.Fatal(err)
	}
	if _, err := Init(context.Background(), workDir); err == nil {
		t.Fatal("expected symlinked project root rejection")
	}
	if _, err := os.Stat(filepath.Join(outside, appdirs.ConfigFileName)); !os.IsNotExist(err) {
		t.Fatalf("init wrote through symlink, stat err=%v", err)
	}
}

func TestInitRejectsUserHomeAlias(t *testing.T) {
	home := t.TempDir()
	t.Setenv("PROTONMAN_HOME", home)
	if _, err := Init(context.Background(), home); err == nil {
		t.Fatal("Init() error = nil, want unavailable project scope")
	}
	if _, err := os.Stat(filepath.Join(home, appdirs.RootDirName)); !os.IsNotExist(err) {
		t.Fatalf("Init() created user-global root as project state: %v", err)
	}
}
