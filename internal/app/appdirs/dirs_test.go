package appdirs

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/phongsathornpt/protonman/internal/base/envconfig"
)

func TestResolveExplicitHomeDefaultsToProtonman(t *testing.T) {
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

func TestResolveReusesLegacyDataWhenCanonicalMissing(t *testing.T) {
	home := t.TempDir()
	legacyRoot := filepath.Join(home, LegacyRootDirName)
	if err := os.Mkdir(legacyRoot, 0o700); err != nil {
		t.Fatal(err)
	}

	dirs, err := Resolve(home)
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if dirs.Root != legacyRoot {
		t.Fatalf("Root = %q, want legacy %q", dirs.Root, legacyRoot)
	}
}

func TestResolveCanonicalDataWinsOverLegacy(t *testing.T) {
	home := t.TempDir()
	canonical := filepath.Join(home, RootDirName)
	legacy := filepath.Join(home, LegacyRootDirName)
	for _, root := range []string{legacy, canonical} {
		if err := os.Mkdir(root, 0o700); err != nil {
			t.Fatal(err)
		}
	}

	dirs, err := Resolve(home)
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if dirs.Root != canonical {
		t.Fatalf("Root = %q, want canonical %q", dirs.Root, canonical)
	}
}

func TestResolveEnvironmentPrecedenceAndLegacyCreation(t *testing.T) {
	canonicalHome := t.TempDir()
	legacyHome := t.TempDir()
	t.Setenv(envconfig.Home, canonicalHome)
	t.Setenv(envconfig.LegacyHome, legacyHome)

	dirs, err := Resolve("")
	if err != nil {
		t.Fatalf("Resolve() canonical env error = %v", err)
	}
	if dirs.Home != canonicalHome || dirs.Root != filepath.Join(canonicalHome, RootDirName) {
		t.Fatalf("canonical env dirs = %+v", dirs)
	}

	t.Setenv(envconfig.Home, "")
	dirs, err = Resolve("")
	if err != nil {
		t.Fatalf("Resolve() legacy env error = %v", err)
	}
	if dirs.Home != legacyHome || dirs.Root != filepath.Join(legacyHome, LegacyRootDirName) {
		t.Fatalf("legacy env dirs = %+v", dirs)
	}
}

func TestProjectPathsPreferCanonicalThenLegacy(t *testing.T) {
	work := filepath.Join(t.TempDir(), "workspace")
	if err := os.Mkdir(work, 0o755); err != nil {
		t.Fatal(err)
	}
	canonical := filepath.Join(work, RootDirName)
	legacy := filepath.Join(work, LegacyRootDirName)

	if got := ProjectRoot(work); got != canonical {
		t.Fatalf("ProjectRoot() = %q, want %q", got, canonical)
	}
	if got := ResolvedProjectRoot(work); got != canonical {
		t.Fatalf("ResolvedProjectRoot() with no data = %q, want canonical", got)
	}
	if err := os.Mkdir(legacy, 0o755); err != nil {
		t.Fatal(err)
	}
	if got := ResolvedProjectRoot(work); got != legacy {
		t.Fatalf("ResolvedProjectRoot() legacy = %q, want %q", got, legacy)
	}
	if err := os.Mkdir(canonical, 0o755); err != nil {
		t.Fatal(err)
	}
	if got := ResolvedProjectRoot(work); got != canonical {
		t.Fatalf("ResolvedProjectRoot() canonical = %q, want %q", got, canonical)
	}
	if got := ResolvedProjectConfig(work); got != filepath.Join(canonical, ConfigFileName) {
		t.Fatalf("ResolvedProjectConfig() = %q", got)
	}
	if got := ResolvedProjectSkills(work); got != filepath.Join(canonical, SkillsDir) {
		t.Fatalf("ResolvedProjectSkills() = %q", got)
	}
}
