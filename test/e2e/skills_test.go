package e2e_test

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestE2ESkillDiscoveryAndActivation(t *testing.T) {
	ws := newTestWorkspace(t)
	home := newTestHome(t)

	// Create user-level skill
	skillDir := filepath.Join(home, ".protonman", "skills", "sample-skill")
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
		env:  []string{"PROTONMAN_HOME=" + home},
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
	skillDir := filepath.Join(ws, ".protonman", "skills", "project-skill")
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
			env:  []string{"PROTONMAN_HOME=" + home},
		})

		// Should emit warning on stderr
		if !strings.Contains(result.stderr, "skipping project skills") || !strings.Contains(result.stderr, "PROTONMAN_TRUST_PROJECT=1") {
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
			env:  []string{"PROTONMAN_HOME=" + home, "PROTONMAN_TRUST_PROJECT=1"},
		})

		if result.exitCode != 0 {
			t.Fatalf("exit code = %d, want 0; stderr = %s", result.exitCode, result.stderr)
		}
		if !strings.Contains(result.stdout, `<skill_content name="project-skill">`) {
			t.Fatalf("expected skill_content in stdout, got: %s", result.stdout)
		}
	})
}

func TestE2EMultiSkillDiscoveryAndActivation(t *testing.T) {
	ws := newTestWorkspace(t)
	home := newTestHome(t)

	// Create 3 user skills in PROTONMAN_HOME
	for _, name := range []string{"skill-alpha", "skill-beta", "skill-gamma"} {
		skillDir := filepath.Join(home, ".protonman", "skills", name)
		if err := os.MkdirAll(skillDir, 0o755); err != nil {
			t.Fatal(err)
		}
		content := fmt.Sprintf("---\nname: %s\ndescription: Skill description for %s\n---\n# Instructions for %s\nRun %s properly.\n", name, name, name, name)
		if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	// 1. Activate skill-alpha
	result := runProton(t, runOptions{
		args: []string{"-y", "-p", `/call activate_skill {"name":"skill-alpha"}`},
		dir:  ws,
		env:  []string{"PROTONMAN_HOME=" + home},
	})
	if result.exitCode != 0 {
		t.Fatalf("exit code = %d, want 0; stderr = %s", result.exitCode, result.stderr)
	}
	if !strings.Contains(result.stdout, `<skill_content name="skill-alpha">`) {
		t.Fatalf("stdout missing skill-alpha content: %s", result.stdout)
	}

	// 2. Activate skill-gamma
	result = runProton(t, runOptions{
		args: []string{"-y", "-p", `/call activate_skill {"name":"skill-gamma"}`},
		dir:  ws,
		env:  []string{"PROTONMAN_HOME=" + home},
	})
	if result.exitCode != 0 {
		t.Fatalf("exit code = %d, want 0; stderr = %s", result.exitCode, result.stderr)
	}
	if !strings.Contains(result.stdout, `<skill_content name="skill-gamma">`) {
		t.Fatalf("stdout missing skill-gamma content: %s", result.stdout)
	}

	// 3. Attempt unknown skill: should report available skills
	result = runProton(t, runOptions{
		args: []string{"-y", "-p", `/call activate_skill {"name":"skill-delta"}`},
		dir:  ws,
		env:  []string{"PROTONMAN_HOME=" + home},
	})
	if !strings.Contains(result.stdout, "skill-alpha") || !strings.Contains(result.stdout, "skill-beta") || !strings.Contains(result.stdout, "skill-gamma") {
		t.Fatalf("expected available skills list in error response, got stdout: %s, stderr: %s", result.stdout, result.stderr)
	}
}

func TestE2ESkillSessionPersistenceAndHeadlessParity(t *testing.T) {
	ws := newTestWorkspace(t)
	home := newTestHome(t)

	// Create user skill in PROTONMAN_HOME
	skillDir := filepath.Join(home, ".protonman", "skills", "code-reviewer")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatal(err)
	}
	content := `---
name: code-reviewer
description: Automated code review guide
---
# Code Reviewer Instructions
Review code thoroughly.
`
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	// 1. Check initial headless /skills list (unchecked)
	res := runProton(t, runOptions{
		args: []string{"-y", "-p", "/skills"},
		dir:  ws,
		env:  []string{"PROTONMAN_HOME=" + home},
	})
	if res.exitCode != 0 {
		t.Fatalf("exit code = %d, want 0; stderr = %s", res.exitCode, res.stderr)
	}
	if !strings.Contains(res.stdout, "Agent Skills (0/1 active):") || !strings.Contains(res.stdout, "[ ] code-reviewer") {
		t.Fatalf("stdout missing unchecked skill list: %s", res.stdout)
	}

	// 2. Activate skill using mixed case (case-insensitivity test)
	res = runProton(t, runOptions{
		args: []string{"-y", "-p", "/skill Code-Reviewer"},
		dir:  ws,
		env:  []string{"PROTONMAN_HOME=" + home},
	})
	if res.exitCode != 0 {
		t.Fatalf("exit code = %d, want 0; stderr = %s", res.exitCode, res.stderr)
	}
	if !strings.Contains(res.stdout, "[x] Activated skill code-reviewer [user]: Automated code review guide") {
		t.Fatalf("stdout missing activation confirmation: %s", res.stdout)
	}
	// Verify raw instructions are not flooded into stdout
	if strings.Contains(res.stdout, "Review code thoroughly.") {
		t.Fatalf("expected stdout not to flood raw instructions, got: %s", res.stdout)
	}

	// 3. New process run in same workspace: verify skill activation persisted across CLI runs
	res = runProton(t, runOptions{
		args: []string{"-y", "--resume", "-p", "/skills active"},
		dir:  ws,
		env:  []string{"PROTONMAN_HOME=" + home},
	})
	if res.exitCode != 0 {
		t.Fatalf("exit code = %d, want 0; stderr = %s", res.exitCode, res.stderr)
	}
	if !strings.Contains(res.stdout, "Active Agent Skills (1):") || !strings.Contains(res.stdout, "[x] code-reviewer") {
		t.Fatalf("expected active skill to persist across CLI sessions, got: %s", res.stdout)
	}

	// 4. Toggle skill to inactive
	res = runProton(t, runOptions{
		args: []string{"-y", "--resume", "-p", "/skill toggle code-reviewer"},
		dir:  ws,
		env:  []string{"PROTONMAN_HOME=" + home},
	})
	if res.exitCode != 0 {
		t.Fatalf("exit code = %d, want 0; stderr = %s", res.exitCode, res.stderr)
	}
	if !strings.Contains(res.stdout, "[ ] Skill \"code-reviewer\" deactivated.") {
		t.Fatalf("expected deactivated message: %s", res.stdout)
	}

	// 5. Verify deactivation persisted across CLI runs
	res = runProton(t, runOptions{
		args: []string{"-y", "--resume", "-p", "/skills active"},
		dir:  ws,
		env:  []string{"PROTONMAN_HOME=" + home},
	})
	if res.exitCode != 0 {
		t.Fatalf("exit code = %d, want 0; stderr = %s", res.exitCode, res.stderr)
	}
	if !strings.Contains(res.stdout, "No active agent skills in this session.") {
		t.Fatalf("expected no active skills after deactivation persistence, got: %s", res.stdout)
	}
}

func TestE2EUnifiedSkillSlashCommand(t *testing.T) {
	ws := newTestWorkspace(t)
	home := newTestHome(t)

	// Create a user skill in PROTONMAN_HOME
	skillDir := filepath.Join(home, ".protonman", "skills", "linter")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatal(err)
	}
	content := `---
name: linter
description: Linting tools
---
# Linter Instructions
Lint cleanly.
`
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	// 1. /skill without arguments lists skills (unified alias)
	res := runProton(t, runOptions{
		args: []string{"-y", "-p", "/skill"},
		dir:  ws,
		env:  []string{"PROTONMAN_HOME=" + home},
	})
	if res.exitCode != 0 {
		t.Fatalf("exit code = %d, want 0; stderr = %s", res.exitCode, res.stderr)
	}
	if !strings.Contains(res.stdout, "Agent Skills (0/1 active):") || !strings.Contains(res.stdout, "[ ] linter") {
		t.Fatalf("expected /skill without args to list skills, got: %s", res.stdout)
	}

	// 2. /skills <name> activates the skill
	res = runProton(t, runOptions{
		args: []string{"-y", "-p", "/skills linter"},
		dir:  ws,
		env:  []string{"PROTONMAN_HOME=" + home},
	})
	if res.exitCode != 0 {
		t.Fatalf("exit code = %d, want 0; stderr = %s", res.exitCode, res.stderr)
	}
	if !strings.Contains(res.stdout, "[x] Activated skill linter") {
		t.Fatalf("expected /skills linter to activate skill, got: %s", res.stdout)
	}

	// 3. /skills toggle <name> in resumed session deactivates the skill
	res = runProton(t, runOptions{
		args: []string{"-y", "--resume", "-p", "/skills toggle linter"},
		dir:  ws,
		env:  []string{"PROTONMAN_HOME=" + home},
	})
	if res.exitCode != 0 {
		t.Fatalf("exit code = %d, want 0; stderr = %s", res.exitCode, res.stderr)
	}
	if !strings.Contains(res.stdout, `[ ] Skill "linter" deactivated.`) {
		t.Fatalf("expected /skills toggle to deactivate, got: %s", res.stdout)
	}
}
