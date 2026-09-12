package skilltool

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/phongsathornpt/protonman/internal/adapter/out/tool/builtin"
	"github.com/phongsathornpt/protonman/internal/adapter/out/tool/builtin/readfile"
	"github.com/phongsathornpt/protonman/internal/core/tool"
	"github.com/phongsathornpt/protonman/internal/core/workspace"
	"github.com/phongsathornpt/protonman/internal/feature/skill"
)

func TestActivateSkillExecute(t *testing.T) {
	ctx := context.Background()

	s := skill.Skill{
		Name:         "pdf-processing",
		Description:  "Extract text from PDFs",
		Location:     "/home/user/.agents/skills/pdf-processing/SKILL.md",
		BaseDir:      "/home/user/.agents/skills/pdf-processing",
		Scope:        skill.ScopeUser,
		Instructions: "# PDF Processing\n\nRun scripts/extract.py to extract text.",
		Resources:    []string{"scripts/extract.py", "references/guide.md"},
	}
	skillReg := skill.NewRegistry(s)
	handler := NewActivateSkill(skillReg)

	t.Run("successful activation", func(t *testing.T) {
		args, _ := json.Marshal(map[string]any{"name": "pdf-processing"})
		call, err := tool.NewCall("call-1", "skill", args)
		if err != nil {
			t.Fatal(err)
		}

		result, err := handler.Execute(ctx, call)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result.Failure != nil {
			t.Fatalf("expected success, got failure: %+v", result.Failure)
		}

		if !strings.Contains(result.Output, `Activated skill "pdf-processing"`) {
			t.Errorf("missing activation acknowledgement: %s", result.Output)
		}
		if !strings.Contains(result.Output, "full instructions are loaded into the next model context") {
			t.Errorf("missing next-context guidance: %s", result.Output)
		}
		if strings.Contains(result.Output, s.Instructions) || strings.Contains(result.Output, "<skill_content") {
			t.Errorf("activation output duplicated full instructions: %s", result.Output)
		}
		if !strings.Contains(result.Output, "Skill directory: /home/user/.agents/skills/pdf-processing") {
			t.Errorf("missing skill directory in output: %s", result.Output)
		}
		if !strings.Contains(result.Output, "- scripts/extract.py") || !strings.Contains(result.Output, "- references/guide.md") {
			t.Errorf("missing bundled resources in output: %s", result.Output)
		}

		if !skillReg.IsActivated("pdf-processing") {
			t.Errorf("expected skill to be marked activated in registry")
		}
	})

	t.Run("skill not found", func(t *testing.T) {
		args, _ := json.Marshal(map[string]any{"name": "non-existent"})
		call, _ := tool.NewCall("call-2", "skill", args)

		_, err := handler.Execute(ctx, call)
		if err == nil {
			t.Fatal("expected error for nonexistent skill")
		}
		if !strings.Contains(err.Error(), "skill \"non-existent\" not found") {
			t.Errorf("unexpected error message: %v", err)
		}
	})

	t.Run("invalid arguments", func(t *testing.T) {
		args := []byte(`{"invalid": true}`)
		call, _ := tool.NewCall("call-3", "skill", args)

		_, err := handler.Execute(ctx, call)
		if err == nil {
			t.Fatal("expected error for missing name argument")
		}
	})
}

func TestActivateSkillAuthorizesReadRootsForFileTools(t *testing.T) {
	ctx := context.Background()

	wsRoot := t.TempDir()
	skillDir := t.TempDir()

	ws, err := workspace.New(wsRoot, nil)
	if err != nil {
		t.Fatal(err)
	}

	skillFilePath := filepath.Join(skillDir, "reference.txt")
	if err := os.WriteFile(skillFilePath, []byte("skill reference text"), 0o644); err != nil {
		t.Fatal(err)
	}

	s := skill.Skill{
		Name:         "doc-helper",
		Description:  "Helper for docs",
		Location:     filepath.Join(skillDir, "SKILL.md"),
		BaseDir:      skillDir,
		Scope:        skill.ScopeUser,
		Instructions: "Read reference.txt for guidelines.",
		Resources:    []string{"reference.txt"},
	}
	skillReg := skill.NewRegistry(s)
	activateHandler := NewActivateSkill(skillReg, ws)
	readHandler := readfile.New(ws)
	writeHandler := builtin.NewWriteFile(ws, skillCheckpointStore{})

	readArgs, _ := json.Marshal(map[string]any{"path": skillFilePath})
	readCall, _ := tool.NewCall("read-before", "read", readArgs)
	_, err = readHandler.Execute(ctx, readCall)
	if err == nil {
		t.Fatal("expected read before activation to fail")
	}

	activateArgs, _ := json.Marshal(map[string]any{"name": "doc-helper"})
	activateCall, _ := tool.NewCall("act-1", "skill", activateArgs)
	actRes, err := activateHandler.Execute(ctx, activateCall)
	if err != nil || actRes.Failure != nil {
		t.Fatalf("skill failed: %v, failure: %+v", err, actRes.Failure)
	}

	readRes, err := readHandler.Execute(ctx, readCall)
	if err != nil || readRes.Failure != nil {
		t.Fatalf("read after activation failed: %v, failure: %+v", err, readRes.Failure)
	}
	if !strings.Contains(readRes.Output, "skill reference text") {
		t.Errorf("read output = %q, want 'skill reference text'", readRes.Output)
	}

	relReadArgs, _ := json.Marshal(map[string]any{"path": "reference.txt"})
	relReadCall, _ := tool.NewCall("read-rel", "read", relReadArgs)
	relReadRes, err := readHandler.Execute(ctx, relReadCall)
	if err != nil || relReadRes.Failure != nil {
		t.Fatalf("read with relative path failed: %v, failure: %+v", err, relReadRes.Failure)
	}
	if !strings.Contains(relReadRes.Output, "skill reference text") {
		t.Errorf("relative read output = %q, want 'skill reference text'", relReadRes.Output)
	}

	writeArgs, _ := json.Marshal(map[string]any{
		"file_path": skillFilePath,
		"content":   "malicious overwrite",
	})
	writeCall, _ := tool.NewCall("write-skill", "edit", writeArgs)
	_, err = writeHandler.Execute(ctx, writeCall)
	if err == nil {
		t.Fatal("expected edit to skill directory to fail, but it succeeded")
	}
	if !errors.Is(err, workspace.ErrOutsideWorkspace) {
		t.Errorf("expected ErrOutsideWorkspace, got: %v", err)
	}
}

func TestActivateSkillFailsIfSkillDirNotFound(t *testing.T) {
	ctx := context.Background()
	wsRoot := t.TempDir()
	ws, err := workspace.New(wsRoot, nil)
	if err != nil {
		t.Fatal(err)
	}

	nonExistentDir := filepath.Join(t.TempDir(), "nonexistent-skill-dir")
	s := skill.Skill{
		Name:         "ghost-skill",
		Description:  "Skill with nonexistent directory",
		Location:     filepath.Join(nonExistentDir, "SKILL.md"),
		BaseDir:      nonExistentDir,
		Scope:        skill.ScopeUser,
		Instructions: "Should not activate.",
	}
	skillReg := skill.NewRegistry(s)
	activateHandler := NewActivateSkill(skillReg, ws)

	activateArgs, _ := json.Marshal(map[string]any{"name": "ghost-skill"})
	activateCall, _ := tool.NewCall("act-ghost", "skill", activateArgs)
	_, err = activateHandler.Execute(ctx, activateCall)
	if err == nil {
		t.Fatal("expected skill to fail for nonexistent BaseDir, got nil")
	}

	if skillReg.IsActivated("ghost-skill") {
		t.Error("expected skill not to be marked activated after failed AddReadRoot")
	}
}

func TestActivateSkillCanBindIsolatedChildRegistry(t *testing.T) {
	s := skill.Skill{
		Name: "go-review", Description: "Review Go code", Scope: skill.ScopeUser,
		Instructions: "Run focused Go checks.",
	}
	parent := skill.NewRegistry(s)
	child := parent.Fork()
	handler := NewActivateSkill(parent).(activateSkillHandler).BindSkillRegistry(child)
	args, err := json.Marshal(map[string]any{"name": "go-review"})
	if err != nil {
		t.Fatal(err)
	}
	call, err := tool.NewCall("skill-child", "skill", args)
	if err != nil {
		t.Fatal(err)
	}
	result, err := handler.Execute(context.Background(), call)
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if !child.IsActivated("go-review") {
		t.Fatal("child skill was not activated")
	}
	if parent.IsActivated("go-review") {
		t.Fatal("child skill activation leaked to parent")
	}
	if strings.Contains(result.Output, s.Instructions) {
		t.Fatalf("child activation duplicated full instructions in tool output: %q", result.Output)
	}
	if !strings.Contains(result.Output, "full instructions are loaded into the next model context") {
		t.Fatalf("compact child activation acknowledgement missing: %q", result.Output)
	}
}

type skillCheckpointStore struct{}

func (skillCheckpointStore) Capture(context.Context, []string) (string, error) {
	return "skill-test", nil
}
func (skillCheckpointStore) Restore(context.Context, string) error { return nil }
