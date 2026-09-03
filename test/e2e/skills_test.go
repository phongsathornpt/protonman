package e2e_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestE2ESkillDiscoveryAndActivation(t *testing.T) {
	ws := newTestWorkspace(t)
	home := newTestHome(t)

	// Create user-level skill
	skillDir := filepath.Join(home, ".proton", "skills", "sample-skill")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatal(err)
	}
	skillContent := `---
name: sample-skill
description: A sample skill for E2E testing
---
# Sample Skill Instructions

Execute test workflows efficiently.
`
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(skillContent), 0o644); err != nil {
		t.Fatal(err)
	}

	result := runProton(t, runOptions{
		args: []string{"-y", "-p", `/call activate_skill {"name":"sample-skill"}`},
		dir:  ws,
		env:  []string{"PROTON_HOME=" + home},
	})

	if result.exitCode != 0 {
		t.Fatalf("exit code = %d, want 0; stderr = %s", result.exitCode, result.stderr)
	}
	if !strings.Contains(result.stdout, `<skill_content name="sample-skill">`) {
		t.Fatalf("stdout missing skill_content: %s", result.stdout)
	}
	if !strings.Contains(result.stdout, "Execute test workflows efficiently.") {
		t.Fatalf("stdout missing skill instructions: %s", result.stdout)
	}
}

func TestE2ESkillProjectTrustGating(t *testing.T) {
	ws := newTestWorkspace(t)
	home := newTestHome(t)

	// Create project-level skill in workspace
	skillDir := filepath.Join(ws, ".proton", "skills", "project-skill")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatal(err)
	}
	skillContent := `---
name: project-skill
description: Project specific skill
---
# Project Skill Content
`
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(skillContent), 0o644); err != nil {
		t.Fatal(err)
	}

	t.Run("untrusted project warns and skips project skill", func(t *testing.T) {
		result := runProton(t, runOptions{
			args: []string{"-y", "-p", `/call activate_skill {"name":"project-skill"}`},
			dir:  ws,
			env:  []string{"PROTON_HOME=" + home},
		})

		// Should emit warning on stderr
		if !strings.Contains(result.stderr, "skipping project skills") || !strings.Contains(result.stderr, "PROTON_TRUST_PROJECT=1") {
			t.Fatalf("expected trust warning on stderr, got: %s", result.stderr)
		}
		// activate_skill should fail since skill is not loaded
		if !strings.Contains(result.stdout, "skill \\\"project-skill\\\" not found") && !strings.Contains(result.stderr, "not found") {
			t.Fatalf("expected skill not found, got stdout: %s, stderr: %s", result.stdout, result.stderr)
		}
	})

	t.Run("trusted project loads and executes project skill", func(t *testing.T) {
		result := runProton(t, runOptions{
			args: []string{"-y", "-p", `/call activate_skill {"name":"project-skill"}`},
			dir:  ws,
			env:  []string{"PROTON_HOME=" + home, "PROTON_TRUST_PROJECT=1"},
		})

		if result.exitCode != 0 {
			t.Fatalf("exit code = %d, want 0; stderr = %s", result.exitCode, result.stderr)
		}
		if !strings.Contains(result.stdout, `<skill_content name="project-skill">`) {
			t.Fatalf("expected skill_content in stdout, got: %s", result.stdout)
		}
	})
}
