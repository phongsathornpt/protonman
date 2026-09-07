package skilltool

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/projectTHORN/proton/internal/skill"
	"github.com/projectTHORN/proton/internal/tool"
	"github.com/projectTHORN/proton/internal/adapter/tool/builtin"
	"github.com/projectTHORN/proton/internal/workspace"
)

func TestActivateSkill_Execute(t *testing.T) {
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
		call, err := tool.NewCall("call-1", "activate_skill", args)
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

		// Check output contents
		if !strings.Contains(result.Output, `<skill_content name="pdf-processing">`) {
			t.Errorf("missing skill_content tag: %s", result.Output)
		}
		if !strings.Contains(result.Output, "Run scripts/extract.py to extract text.") {
			t.Errorf("missing instructions in output: %s", result.Output)
		}
		if !strings.Contains(result.Output, "Skill directory: /home/user/.agents/skills/pdf-processing") {
			t.Errorf("missing skill directory in output: %s", result.Output)
		}
		if !strings.Contains(result.Output, "<file>scripts/extract.py</file>") {
			t.Errorf("missing bundled resource in output: %s", result.Output)
		}
		if !strings.Contains(result.Output, "</skill_content>") {
			t.Errorf("missing closing skill_content tag: %s", result.Output)
		}

		// Check that it was marked activated
		if !skillReg.IsActivated("pdf-processing") {
			t.Errorf("expected skill to be marked activated in registry")
		}
	})

	t.Run("skill not found", func(t *testing.T) {
		args, _ := json.Marshal(map[string]any{"name": "non-existent"})
		call, _ := tool.NewCall("call-2", "activate_skill", args)

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
		call, _ := tool.NewCall("call-3", "activate_skill", args)

		_, err := handler.Execute(ctx, call)
		if err == nil {
			t.Fatal("expected error for missing name argument")
		}
	})
}

func TestActivateSkill_AuthorizesReadRootsForFileTools(t *testing.T) {
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
	readHandler := builtin.NewReadFile(ws)
	writeHandler := builtin.NewWriteFile(ws, skillCheckpointStore{})

	// 1. Before activation, reading reference.txt fails with outside workspace
	readArgs, _ := json.Marshal(map[string]any{"path": skillFilePath})
	readCall, _ := tool.NewCall("read-before", "read_file", readArgs)
	_, err = readHandler.Execute(ctx, readCall)
	if err == nil {
		t.Fatal("expected read_file before activation to fail")
	}

	// 2. Activate the skill
	activateArgs, _ := json.Marshal(map[string]any{"name": "doc-helper"})
	activateCall, _ := tool.NewCall("act-1", "activate_skill", activateArgs)
	actRes, err := activateHandler.Execute(ctx, activateCall)
	if err != nil || actRes.Failure != nil {
		t.Fatalf("activate_skill failed: %v, failure: %+v", err, actRes.Failure)
	}

	// 3. After activation, read_file succeeds with absolute path
	readRes, err := readHandler.Execute(ctx, readCall)
	if err != nil || readRes.Failure != nil {
		t.Fatalf("read_file after activation failed: %v, failure: %+v", err, readRes.Failure)
	}
	if !strings.Contains(readRes.Output, "skill reference text") {
		t.Errorf("read_file output = %q, want 'skill reference text'", readRes.Output)
	}

	// 4. After activation, read_file succeeds with relative path fallback
	relReadArgs, _ := json.Marshal(map[string]any{"path": "reference.txt"})
	relReadCall, _ := tool.NewCall("read-rel", "read_file", relReadArgs)
	relReadRes, err := readHandler.Execute(ctx, relReadCall)
	if err != nil || relReadRes.Failure != nil {
		t.Fatalf("read_file with relative path failed: %v, failure: %+v", err, relReadRes.Failure)
	}
	if !strings.Contains(relReadRes.Output, "skill reference text") {
		t.Errorf("relative read_file output = %q, want 'skill reference text'", relReadRes.Output)
	}

	// 5. write_file must STILL fail with outside workspace (read-only confinement!)
	writeArgs, _ := json.Marshal(map[string]any{
		"file_path": skillFilePath,
		"content":   "malicious overwrite",
	})
	writeCall, _ := tool.NewCall("write-skill", "write_file", writeArgs)
	_, err = writeHandler.Execute(ctx, writeCall)
	if err == nil {
		t.Fatal("expected write_file to skill directory to fail, but it succeeded")
	}
	if !errors.Is(err, workspace.ErrOutsideWorkspace) {
		t.Errorf("expected ErrOutsideWorkspace, got: %v", err)
	}
}

func TestActivateSkill_FailsIfSkillDirNotFound(t *testing.T) {
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
	activateCall, _ := tool.NewCall("act-ghost", "activate_skill", activateArgs)
	_, err = activateHandler.Execute(ctx, activateCall)
	if err == nil {
		t.Fatal("expected activate_skill to fail for nonexistent BaseDir, got nil")
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
	call, err := tool.NewCall("skill-child", "activate_skill", args)
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
