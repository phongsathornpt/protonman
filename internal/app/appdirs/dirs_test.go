package appdirs

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/phongsathornpt/protonman/internal/base/envconfig"
)

func TestResolveExplicitHomeUsesProtonmanRoot(t *testing.T) {
	home := t.TempDir()
	t.Setenv(envconfig.Home, filepath.Join(t.TempDir(), "ignored"))
	t.Setenv(envconfig.LegacyHome, filepath.Join(t.TempDir(), "legacy-ignored"))

	dirs, err := Resolve(home)
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if dirs.Home != home {
		t.Fatalf("Home = %q, want %q", dirs.Home, home)
	}
	if dirs.Root != filepath.Join(home, RootDirName) {
		t.Fatalf("Root = %q, want canonical Protonman root", dirs.Root)
	}
	if dirs.Config != filepath.Join(home, RootDirName, ConfigFileName) {
		t.Fatalf("Config = %q", dirs.Config)
	}
}

func TestResolveIgnoresLegacyProtonDirectory(t *testing.T) {
	home := t.TempDir()
	legacy := filepath.Join(home, ".proton")
	if err := os.Mkdir(legacy, 0o700); err != nil {
		t.Fatal(err)
	}

	dirs, err := Resolve(home)
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if got, want := dirs.Root, filepath.Join(home, RootDirName); got != want {
		t.Fatalf("Root = %q, want %q", got, want)
	}
}

func TestResolveEnvironmentHomeUsesProtonmanRoot(t *testing.T) {
	home := t.TempDir()
	t.Setenv(envconfig.Home, home)
	t.Setenv(envconfig.LegacyHome, filepath.Join(t.TempDir(), "legacy"))

	dirs, err := Resolve("")
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if dirs.Home != home || dirs.Root != filepath.Join(home, RootDirName) {
		t.Fatalf("dirs = %+v", dirs)
	}
}

func TestProjectPathsUseOnlyProtonmanNamespace(t *testing.T) {
	work := filepath.Join(t.TempDir(), "workspace")
	if err := os.Mkdir(work, 0o755); err != nil {
		t.Fatal(err)
	}
	root := filepath.Join(work, RootDirName)

	if got := ProjectRoot(work); got != root {
		t.Fatalf("ProjectRoot() = %q, want %q", got, root)
	}
	if got := ResolvedProjectRoot(work); got != root {
		t.Fatalf("ResolvedProjectRoot() = %q, want %q", got, root)
	}
	if got := ResolvedProjectConfig(work); got != filepath.Join(root, ConfigFileName) {
		t.Fatalf("ResolvedProjectConfig() = %q", got)
	}
	if got := ResolvedProjectSkills(work); got != filepath.Join(root, SkillsDir) {
		t.Fatalf("ResolvedProjectSkills() = %q", got)
	}
}
