package project

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/phongsathornpt/protonman/internal/app/appdirs"
)

func TestDiscoverProjectProtonState(t *testing.T) {
	workDir := t.TempDir()
	protonDir := appdirs.ProjectRoot(workDir)
	if err := os.MkdirAll(filepath.Join(protonDir, "skills", "review"), 0o755); err != nil {
		t.Fatal(err)
	}
	configPath := appdirs.ProjectConfig(workDir)
	if err := os.WriteFile(configPath, []byte("[agent]\nprofile = \"dex\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(protonDir, "skills", "review", "SKILL.md"), []byte("# Review"), 0o644); err != nil {
		t.Fatal(err)
	}

	state, err := Discover(context.Background(), Options{WorkDir: workDir, Trusted: true, ConfigSources: []string{configPath}})
	if err != nil {
		t.Fatal(err)
	}
	if !state.Exists || !state.ConfigExists || !state.ConfigLoaded || !state.SkillsExists || state.SkillCount != 1 || !state.Trusted {
		t.Fatalf("unexpected state: %#v", state)
	}
}

func TestDiscoverUntrustedConfigIsDetectedButNotLoaded(t *testing.T) {
	workDir := t.TempDir()
	if err := os.MkdirAll(appdirs.ProjectRoot(workDir), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(appdirs.ProjectConfig(workDir), []byte("[agent]\nprofile = \"dex\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	state, err := Discover(context.Background(), Options{WorkDir: workDir})
	if err != nil {
		t.Fatal(err)
	}
	if !state.ConfigExists || state.ConfigLoaded || state.Trusted {
		t.Fatalf("unexpected state: %#v", state)
	}
}

func TestDiscoverFallsBackToLegacyProjectDirectory(t *testing.T) {
	workDir := t.TempDir()
	legacyRoot := appdirs.LegacyProjectRoot(workDir)
	if err := os.MkdirAll(filepath.Join(legacyRoot, "skills", "legacy"), 0o755); err != nil {
		t.Fatal(err)
	}
	legacyConfig := filepath.Join(legacyRoot, appdirs.ConfigFileName)
	if err := os.WriteFile(legacyConfig, []byte("[agent]\nprofile = \"dex\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(legacyRoot, "skills", "legacy", "SKILL.md"), []byte("# Legacy"), 0o644); err != nil {
		t.Fatal(err)
	}

	state, err := Discover(context.Background(), Options{WorkDir: workDir, Trusted: true, ConfigSources: []string{legacyConfig}})
	if err != nil {
		t.Fatal(err)
	}
	if state.ProtonDir != legacyRoot || !state.ConfigLoaded || state.SkillCount != 1 {
		t.Fatalf("legacy project state = %#v", state)
	}
}
