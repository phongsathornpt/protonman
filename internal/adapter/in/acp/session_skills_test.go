package acp

import (
	"context"
	"testing"

	"github.com/phongsathornpt/protonman/internal/core/permission"
	"github.com/phongsathornpt/protonman/internal/engine/toolcall"
	"github.com/phongsathornpt/protonman/internal/feature/skill"
)

func TestSessionSkillsAndToggle(t *testing.T) {
	ctx := context.Background()

	skillReg := skill.NewRegistry(
		skill.Skill{
			Name:        "test-skill",
			Description: "a skill for testing",
			Scope:       skill.ScopeProject,
			Resources:   []string{"ref.txt"},
		},
		skill.Skill{
			Name:        "another-skill",
			Description: "another skill",
			Scope:       skill.ScopeUser,
		},
	)

	var savedWorkDir string
	var savedSkills []string
	saver := func(workDir string, activeSkills []string) error {
		savedWorkDir = workDir
		savedSkills = activeSkills
		return nil
	}

	registry := newTestRegistrar()
	policy, err := permission.NewPolicy(permission.Config{Default: permission.ActionAllow})
	if err != nil {
		t.Fatalf("NewPolicy: %v", err)
	}
	service, err := toolcall.NewService(registry, policy)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}

	server, err := New(
		service,
		registry,
		nil,
		WithSkills(skillReg, saver),
	)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	sessionID := "session-skill-test"
	sess, err := server.newSession(ctx, sessionID, "/tmp/testdir", nil)
	if err != nil {
		t.Fatalf("newSession() error = %v", err)
	}
	server.mu.Lock()
	server.sessions[sessionID] = sess
	server.mu.Unlock()

	// 1. Get initial skills
	res, err := server.sessionSkills(ctx, sessionID)
	if err != nil {
		t.Fatalf("sessionSkills() error = %v", err)
	}
	if len(res.Skills) != 2 {
		t.Fatalf("expected 2 skills, got %d", len(res.Skills))
	}
	for _, s := range res.Skills {
		if s.Active {
			t.Fatalf("skill %s should initially be inactive", s.Name)
		}
	}

	// 2. Toggle test-skill on
	toggleRes, err := server.sessionSkillToggle(ctx, ProtonmanSessionSkillToggleParams{
		SessionID: sessionID,
		Name:      "test-skill",
	})
	if err != nil {
		t.Fatalf("sessionSkillToggle() error = %v", err)
	}
	if !toggleRes.Active {
		t.Fatalf("expected test-skill to be active after toggle")
	}
	if savedWorkDir != "/tmp/testdir" {
		t.Fatalf("savedWorkDir = %q, want /tmp/testdir", savedWorkDir)
	}
	if len(savedSkills) != 1 || savedSkills[0] != "test-skill" {
		t.Fatalf("savedSkills = %v, want [test-skill]", savedSkills)
	}

	// 3. Inspect again to verify active status
	res, err = server.sessionSkills(ctx, sessionID)
	if err != nil {
		t.Fatalf("sessionSkills() error = %v", err)
	}
	var foundActive bool
	for _, s := range res.Skills {
		if s.Name == "test-skill" && s.Active {
			foundActive = true
		}
	}
	if !foundActive {
		t.Fatalf("test-skill not reported as active in sessionSkills")
	}

	// 4. Toggle test-skill off
	toggleRes, err = server.sessionSkillToggle(ctx, ProtonmanSessionSkillToggleParams{
		SessionID: sessionID,
		Name:      "test-skill",
	})
	if err != nil {
		t.Fatalf("sessionSkillToggle() off error = %v", err)
	}
	if toggleRes.Active {
		t.Fatalf("expected test-skill to be inactive after second toggle")
	}
	if len(savedSkills) != 0 {
		t.Fatalf("savedSkills = %v, want empty", savedSkills)
	}
}
