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

func TestResolveProjectScopeDisablesHomeAlias(t *testing.T) {
	home := t.TempDir()
	scope, err := ResolveProjectScope(home, home)
	if err != nil {
		t.Fatal(err)
	}
	if scope.Available {
		t.Fatalf("project scope = %+v, want unavailable", scope)
	}
	if got, want := scope.Root, filepath.Join(home, RootDirName); got != want {
		t.Fatalf("Root = %q, want %q", got, want)
	}
}

func TestResolveProjectScopeDetectsSymlinkAlias(t *testing.T) {
	home := t.TempDir()
	parent := t.TempDir()
	alias := filepath.Join(parent, "home-link")
	if err := os.Symlink(home, alias); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	scope, err := ResolveProjectScope(home, alias)
	if err != nil {
		t.Fatal(err)
	}
	if scope.Available {
		t.Fatalf("project scope = %+v, want unavailable", scope)
	}
}

func TestResolveProjectScopeKeepsDistinctWorkspaceAvailable(t *testing.T) {
	home := t.TempDir()
	work := t.TempDir()
	scope, err := ResolveProjectScope(home, work)
	if err != nil {
		t.Fatal(err)
	}
	if !scope.Available {
		t.Fatalf("project scope = %+v, want available", scope)
	}
	if got, want := scope.Config, filepath.Join(work, RootDirName, ConfigFileName); got != want {
		t.Fatalf("Config = %q, want %q", got, want)
	}
}

func TestResolveRuntimeLayoutDisablesProjectScopeAtHome(t *testing.T) {
	home := t.TempDir()
	layout, err := ResolveRuntimeLayout(home, home)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := layout.User.Root, filepath.Join(home, RootDirName); got != want {
		t.Fatalf("User.Root = %q, want %q", got, want)
	}
	if got, want := layout.Workspace, home; got != want {
		t.Fatalf("Workspace = %q, want %q", got, want)
	}
	if layout.Project.Available {
		t.Fatalf("Project = %+v, want unavailable", layout.Project)
	}
}
