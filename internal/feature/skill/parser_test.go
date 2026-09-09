package skill

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseSkillFileValid(t *testing.T) {
	tempDir := t.TempDir()
	skillDir := filepath.Join(tempDir, "pdf-processing")
	if err := os.MkdirAll(filepath.Join(skillDir, "scripts"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(skillDir, "references"), 0o755); err != nil {
		t.Fatal(err)
	}

	skillContent := `---
name: pdf-processing
description: Extract text and tables from PDF files. Use when handling PDFs.
license: Apache-2.0
compatibility: Requires python3
metadata:
  author: thorn
  version: "1.0"
allowed-tools: bash read
---
# PDF Processing Guide

Step-by-step instructions for extracting text.
`
	skillFile := filepath.Join(skillDir, "SKILL.md")
	if err := os.WriteFile(skillFile, []byte(skillContent), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "scripts", "extract.py"), []byte("#!/usr/bin/env python3"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "references", "guide.md"), []byte("# Guide"), 0o644); err != nil {
		t.Fatal(err)
	}

	parsed, err := ParseSkillFile(skillFile, ScopeUser)
	if err != nil {
		t.Fatalf("ParseSkillFile failed: %v", err)
	}

	if parsed.Name != "pdf-processing" {
		t.Errorf("got name %q, want %q", parsed.Name, "pdf-processing")
	}
	if parsed.Description != "Extract text and tables from PDF files. Use when handling PDFs." {
		t.Errorf("got description %q", parsed.Description)
	}
	if parsed.License != "Apache-2.0" {
		t.Errorf("got license %q", parsed.License)
	}
	if parsed.Compatibility != "Requires python3" {
		t.Errorf("got compatibility %q", parsed.Compatibility)
	}
	if parsed.Metadata["author"] != "thorn" {
		t.Errorf("got metadata author %q", parsed.Metadata["author"])
	}
	if len(parsed.AllowedTools) != 2 || parsed.AllowedTools[0] != "bash" || parsed.AllowedTools[1] != "read" {
		t.Errorf("got allowed tools %v", parsed.AllowedTools)
	}
	if parsed.Instructions != "# PDF Processing Guide\n\nStep-by-step instructions for extracting text." {
		t.Errorf("unexpected instructions: %q", parsed.Instructions)
	}

	// Check resources
	expectedResources := map[string]bool{
		"scripts/extract.py":  false,
		"references/guide.md": false,
	}
	for _, res := range parsed.Resources {
		if _, ok := expectedResources[res]; ok {
			expectedResources[res] = true
		}
	}
	for res, found := range expectedResources {
		if !found {
			t.Errorf("expected resource %q was not discovered", res)
		}
	}
}

func TestParseSkillFileLenientDescription(t *testing.T) {
	tempDir := t.TempDir()
	skillDir := filepath.Join(tempDir, "data-cleaner")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatal(err)
	}

	// Unquoted colon in description
	skillContent := `---
name: data-cleaner
description: Clean datasets when: the user asks for CSV normalization.
---
Instructions here.
`
	skillFile := filepath.Join(skillDir, "SKILL.md")
	if err := os.WriteFile(skillFile, []byte(skillContent), 0o644); err != nil {
		t.Fatal(err)
	}

	parsed, err := ParseSkillFile(skillFile, ScopeProject)
	if err != nil {
		t.Fatalf("expected lenient parsing to succeed, got error: %v", err)
	}
	if parsed.Name != "data-cleaner" {
		t.Errorf("got name %q, want data-cleaner", parsed.Name)
	}
}

func TestParseSkillFileErrors(t *testing.T) {
	tempDir := t.TempDir()

	t.Run("missing frontmatter", func(t *testing.T) {
		skillFile := filepath.Join(tempDir, "SKILL1.md")
		_ = os.WriteFile(skillFile, []byte("Just markdown without frontmatter"), 0o644)
		_, err := ParseSkillFile(skillFile, ScopeUser)
		if err == nil {
			t.Fatal("expected error for missing frontmatter")
		}
	})

	t.Run("invalid name in frontmatter", func(t *testing.T) {
		skillFile := filepath.Join(tempDir, "SKILL2.md")
		_ = os.WriteFile(skillFile, []byte("---\nname: Invalid_Name!\ndescription: A valid description.\n---\nBody"), 0o644)
		_, err := ParseSkillFile(skillFile, ScopeUser)
		if err == nil {
			t.Fatal("expected error for invalid name")
		}
	})

	t.Run("missing description", func(t *testing.T) {
		skillFile := filepath.Join(tempDir, "SKILL3.md")
		_ = os.WriteFile(skillFile, []byte("---\nname: valid-name\n---\nBody"), 0o644)
		_, err := ParseSkillFile(skillFile, ScopeUser)
		if err == nil {
			t.Fatal("expected error for missing description")
		}
	})
}

func TestParseSkillFileNestedMetadata(t *testing.T) {
	tempDir := t.TempDir()
	skillDir := filepath.Join(tempDir, "golang-code-style")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatal(err)
	}

	skillContent := `---
name: golang-code-style
description: "Golang code style conventions — line length, flow clarity, etc."
user-invocable: true
license: MIT
compatibility: Designed for Claude Code or similar AI coding agents.
metadata:
  author: samber
  version: "1.2.0"
  openclaw:
    emoji: "🎨"
    homepage: https://github.com/samber/cc-skills-golang
    requires:
      bins:
        - go
    install: []
allowed-tools: Read Edit Write Glob Grep
---
# Go Code Style
Style rules that require human judgment.
`
	skillFile := filepath.Join(skillDir, "SKILL.md")
	if err := os.WriteFile(skillFile, []byte(skillContent), 0o644); err != nil {
		t.Fatal(err)
	}

	parsed, err := ParseSkillFile(skillFile, ScopeUser)
	if err != nil {
		t.Fatalf("ParseSkillFile() unexpected error: %v", err)
	}
	if parsed.Name != "golang-code-style" {
		t.Errorf("got name %q, want golang-code-style", parsed.Name)
	}
	if parsed.Metadata["author"] != "samber" {
		t.Errorf("got metadata author %q", parsed.Metadata["author"])
	}
	openclaw, ok := parsed.Metadata["openclaw"].(map[string]any)
	if !ok {
		t.Fatalf("expected openclaw to be map[string]any, got %T", parsed.Metadata["openclaw"])
	}
	if openclaw["emoji"] != "🎨" {
		t.Errorf("got emoji %v, want 🎨", openclaw["emoji"])
	}
}
