package skill

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func createSkill(t *testing.T, dir string, name string, desc string) {
	t.Helper()
	skillDir := filepath.Join(dir, name)
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatal(err)
	}
	content := "---\nname: " + name + "\ndescription: " + desc + "\n---\n# " + name + "\nInstructions"
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestDiscover_UserAndProjectSkills(t *testing.T) {
	homeDir := t.TempDir()
	workDir := t.TempDir()

	// Create user-level skill in ~/.protonman/skills/user-skill
	createSkill(t, filepath.Join(homeDir, ".protonman", "skills"), "user-skill", "User-level skill")
	// Create user-level skill in ~/.agents/skills/shared-skill
	createSkill(t, filepath.Join(homeDir, ".agents", "skills"), "shared-skill", "Cross-client shared skill")

	// Create project-level skill in workDir/.protonman/skills/project-skill
	createSkill(t, filepath.Join(workDir, ".protonman", "skills"), "project-skill", "Project-level skill")
	// Create project-level skill overriding shared-skill
	createSkill(t, filepath.Join(workDir, ".protonman", "skills"), "shared-skill", "Project overridden shared skill")

	t.Run("untrusted project skips project skills", func(t *testing.T) {
		res, err := Discover(context.Background(), Options{
			HomeDir:        homeDir,
			WorkDir:        workDir,
			ProjectTrusted: false,
		})
		if err != nil {
			t.Fatalf("Discover failed: %v", err)
		}

		// Only user skills should be discovered
		if len(res.Skills) != 2 {
			t.Fatalf("expected 2 skills, got %d", len(res.Skills))
		}
		for _, s := range res.Skills {
			if s.Scope != ScopeUser {
				t.Errorf("expected ScopeUser, got %s for skill %s", s.Scope, s.Name)
			}
			if s.Name == "project-skill" {
				t.Errorf("untrusted project skill should not be loaded")
			}
		}

		// Check that a warning was emitted about skipping project skills
		foundWarning := false
		for _, w := range res.Warnings {
			if strings.Contains(w, "skipping project skills") && strings.Contains(w, "PROTONMAN_TRUST_PROJECT=1") {
				foundWarning = true
				break
			}
		}
		if !foundWarning {
			t.Errorf("expected warning about skipping untrusted project skills, got: %v", res.Warnings)
		}
	})

	t.Run("trusted project loads project skills and overrides user skills", func(t *testing.T) {
		res, err := Discover(context.Background(), Options{
			HomeDir:        homeDir,
			WorkDir:        workDir,
			ProjectTrusted: true,
		})
		if err != nil {
			t.Fatalf("Discover failed: %v", err)
		}

		if len(res.Skills) != 3 {
			t.Fatalf("expected 3 skills, got %d", len(res.Skills))
		}

		skillMap := make(map[string]Skill)
		for _, s := range res.Skills {
			skillMap[s.Name] = s
		}

		if s, ok := skillMap["user-skill"]; !ok || s.Scope != ScopeUser {
			t.Errorf("expected user-skill to have ScopeUser")
		}
		if s, ok := skillMap["project-skill"]; !ok || s.Scope != ScopeProject {
			t.Errorf("expected project-skill to have ScopeProject")
		}
		if s, ok := skillMap["shared-skill"]; !ok {
			t.Errorf("expected shared-skill to be present")
		} else {
			if s.Scope != ScopeProject {
				t.Errorf("expected project scope to override user scope, got %s", s.Scope)
			}
			if s.Description != "Project overridden shared skill" {
				t.Errorf("expected project description to override user, got %q", s.Description)
			}
		}
	})
}

func TestDiscoverHomeWorkspaceKeepsAliasedSkillsInUserScope(t *testing.T) {
	home := t.TempDir()
	createSkill(t, filepath.Join(home, ".protonman", "skills"), "protonman-user", "User Protonman skill")
	createSkill(t, filepath.Join(home, ".agents", "skills"), "agents-user", "User shared skill")

	for _, trusted := range []bool{false, true} {
		res, err := Discover(context.Background(), Options{
			HomeDir:        home,
			WorkDir:        home,
			ProjectTrusted: trusted,
		})
		if err != nil {
			t.Fatal(err)
		}
		if len(res.Skills) != 2 {
			t.Fatalf("trusted=%v skills=%d, want 2", trusted, len(res.Skills))
		}
		if len(res.Warnings) != 0 {
			t.Fatalf("trusted=%v warnings=%v, want none", trusted, res.Warnings)
		}
		for _, discovered := range res.Skills {
			if discovered.Scope != ScopeUser {
				t.Fatalf("trusted=%v skill %q scope=%s, want user", trusted, discovered.Name, discovered.Scope)
			}
		}
	}
}
