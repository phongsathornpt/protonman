package appdirs

import (
	"path/filepath"
	"testing"

	"github.com/phongsathornpt/proton/internal/base/envconfig"
)

func TestResolveExplicitHome(t *testing.T) {
	home := t.TempDir()
	t.Setenv(envconfig.Home, filepath.Join(t.TempDir(), "ignored"))

	dirs, err := Resolve(home)
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if dirs.Home != home {
		t.Fatalf("Home = %q, want %q", dirs.Home, home)
	}
	if dirs.Config != filepath.Join(home, RootDirName, ConfigFileName) {
		t.Fatalf("Config = %q", dirs.Config)
	}
	if dirs.Sessions != filepath.Join(home, RootDirName, SessionsDir) {
		t.Fatalf("Sessions = %q", dirs.Sessions)
	}
}

func TestResolveUsesProtonHome(t *testing.T) {
	home := t.TempDir()
	t.Setenv(envconfig.Home, home)

	dirs, err := Resolve("")
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if dirs.Home != home || dirs.Skills != filepath.Join(home, RootDirName, SkillsDir) {
		t.Fatalf("unexpected dirs: %+v", dirs)
	}
}

func TestProjectPaths(t *testing.T) {
	work := filepath.Join(t.TempDir(), "workspace")
	if got, want := ProjectConfig(work), filepath.Join(work, RootDirName, ConfigFileName); got != want {
		t.Fatalf("ProjectConfig() = %q, want %q", got, want)
	}
	if got, want := ProjectSkills(work), filepath.Join(work, RootDirName, SkillsDir); got != want {
		t.Fatalf("ProjectSkills() = %q, want %q", got, want)
	}
}
