package config

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/phongsathornpt/protonman/internal/app/appdirs"
)

func TestLoadUserConfigNamespaceFallbackAndPrecedence(t *testing.T) {
	homeDir := t.TempDir()
	workDir := t.TempDir()
	legacyPath := filepath.Join(homeDir, appdirs.LegacyRootDirName, appdirs.ConfigFileName)
	canonicalPath := filepath.Join(homeDir, appdirs.RootDirName, appdirs.ConfigFileName)
	writeConfig(t, legacyPath, "[agent]\nmax_tool_calls = 11\n")

	legacy, err := Load(context.Background(), Options{HomeDir: homeDir, WorkDir: workDir})
	if err != nil {
		t.Fatalf("Load() legacy error = %v", err)
	}
	if legacy.Agent.MaxToolCalls != 11 {
		t.Fatalf("legacy max_tool_calls = %d, want 11", legacy.Agent.MaxToolCalls)
	}
	if len(legacy.Sources) != 1 || legacy.Sources[0] != legacyPath {
		t.Fatalf("legacy sources = %v", legacy.Sources)
	}

	writeConfig(t, canonicalPath, "[agent]\nmax_tool_calls = 22\n")
	canonical, err := Load(context.Background(), Options{HomeDir: homeDir, WorkDir: workDir})
	if err != nil {
		t.Fatalf("Load() canonical error = %v", err)
	}
	if canonical.Agent.MaxToolCalls != 22 {
		t.Fatalf("canonical max_tool_calls = %d, want 22", canonical.Agent.MaxToolCalls)
	}
	if len(canonical.Sources) != 1 || canonical.Sources[0] != canonicalPath {
		t.Fatalf("canonical sources = %v", canonical.Sources)
	}
}

func TestLoadProjectConfigNamespaceFallbackAndPrecedence(t *testing.T) {
	homeDir := t.TempDir()
	workDir := t.TempDir()
	legacyPath := filepath.Join(workDir, appdirs.LegacyRootDirName, appdirs.ConfigFileName)
	canonicalPath := filepath.Join(workDir, appdirs.RootDirName, appdirs.ConfigFileName)
	writeConfig(t, legacyPath, "[agent]\nmax_tool_calls = 33\n")

	legacy, err := Load(context.Background(), Options{HomeDir: homeDir, WorkDir: workDir, ProjectTrusted: true})
	if err != nil {
		t.Fatalf("Load() legacy project error = %v", err)
	}
	if legacy.Agent.MaxToolCalls != 33 {
		t.Fatalf("legacy project max_tool_calls = %d, want 33", legacy.Agent.MaxToolCalls)
	}

	writeConfig(t, canonicalPath, "[agent]\nmax_tool_calls = 44\n")
	canonical, err := Load(context.Background(), Options{HomeDir: homeDir, WorkDir: workDir, ProjectTrusted: true})
	if err != nil {
		t.Fatalf("Load() canonical project error = %v", err)
	}
	if canonical.Agent.MaxToolCalls != 44 {
		t.Fatalf("canonical project max_tool_calls = %d, want 44", canonical.Agent.MaxToolCalls)
	}
	if len(canonical.Sources) != 1 || canonical.Sources[0] != canonicalPath {
		t.Fatalf("canonical project sources = %v", canonical.Sources)
	}
}
