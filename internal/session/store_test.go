package session

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/projectTHORN/proton/internal/model"
	"github.com/projectTHORN/proton/internal/permission"
)

func TestFileStoreRoundTrip(t *testing.T) {
	store, err := NewFileStore(filepath.Join(t.TempDir(), "sessions"))
	if err != nil {
		t.Fatalf("NewFileStore() error = %v", err)
	}

	want := State{
		PermissionMode: permission.ModeAlwaysApprove.String(),
	}
	if err := store.Save(context.Background(), "session-1", want); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	got, found, err := store.Load(context.Background(), "session-1")
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if !found {
		t.Fatal("Load() found = false, want true")
	}
	if got.Version != currentStateVersion {
		t.Fatalf("state version = %d, want %d", got.Version, currentStateVersion)
	}
	if got.PermissionMode != want.PermissionMode {
		t.Fatalf("permission mode = %q, want %q", got.PermissionMode, want.PermissionMode)
	}
	if got.UpdatedAt.IsZero() {
		t.Fatal("updated_at is zero, want persisted timestamp")
	}
}

func TestFileStorePersistsActiveSkills(t *testing.T) {
	store, err := NewFileStore(filepath.Join(t.TempDir(), "sessions"))
	if err != nil {
		t.Fatalf("NewFileStore() error = %v", err)
	}
	want := State{
		PermissionMode: permission.ModeAsk.String(),
		ActiveSkills:   []string{"skill-a", "skill-b"},
	}
	if err := store.Save(context.Background(), "skills-session", want); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	got, found, err := store.Load(context.Background(), "skills-session")
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if !found {
		t.Fatal("Load() found = false")
	}
	if len(got.ActiveSkills) != 2 || got.ActiveSkills[0] != "skill-a" || got.ActiveSkills[1] != "skill-b" {
		t.Fatalf("ActiveSkills = %v, want %v", got.ActiveSkills, want.ActiveSkills)
	}
}

func TestFileStorePersistsMessagesWithoutArguments(t *testing.T) {
	store, err := NewFileStore(filepath.Join(t.TempDir(), "sessions"))
	if err != nil {
		t.Fatalf("NewFileStore() error = %v", err)
	}
	want := State{
		PermissionMode: permission.ModeAsk.String(),
		Messages: []Message{
			{Role: model.RoleUser, Content: "list tools"},
			{Role: model.RoleAssistant, Content: "use /tools", ToolCalls: []ToolCall{{ID: "c1", Name: "read_file"}}},
			{Role: model.RoleTool, Content: "ok", ToolName: "read_file", ToolCallID: "c1"},
		},
	}
	if err := store.Save(context.Background(), "chat-1", want); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	got, found, err := store.Load(context.Background(), "chat-1")
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if !found {
		t.Fatal("Load() found = false")
	}
	if len(got.Messages) != 3 {
		t.Fatalf("messages = %d, want 3", len(got.Messages))
	}
	if got.Messages[2].ToolName != "read_file" || got.Messages[2].Content != "ok" {
		t.Fatalf("tool message = %+v", got.Messages[2])
	}
	if len(got.Messages[1].ToolCalls) != 1 || got.Messages[1].ToolCalls[0].Name != "read_file" {
		t.Fatalf("assistant tool calls = %+v", got.Messages[1].ToolCalls)
	}
}

func TestToolCallArgumentsAreNotPersisted(t *testing.T) {
	store, err := NewFileStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewFileStore() error = %v", err)
	}
	if err := store.Save(context.Background(), "redacted", State{
		PermissionMode: permission.ModeAsk.String(),
		Messages: []Message{{
			Role:      model.RoleAssistant,
			ToolCalls: []ToolCall{{ID: "c1", Name: "bash"}},
		}},
	}); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	loaded, found, err := store.Load(context.Background(), "redacted")
	if err != nil || !found {
		t.Fatalf("Load() = found %v, err %v", found, err)
	}
	modelMessages := ToModelMessages(loaded.Messages)
	if got := string(modelMessages[0].ToolCalls[0].Arguments); got != "{}" {
		t.Fatalf("restored tool arguments = %q, want redacted empty object", got)
	}
}

func TestFileStoreRejectsUnknownMessageRole(t *testing.T) {
	store, err := NewFileStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewFileStore() error = %v", err)
	}
	err = store.Save(context.Background(), "bad", State{
		PermissionMode: permission.ModeAsk.String(),
		Messages:       []Message{{Role: model.Role("root"), Content: "nope"}},
	})
	if err == nil {
		t.Fatal("Save() error = nil, want invalid role")
	}
}

func TestFileStoreMissingStateIsNotAnError(t *testing.T) {
	store, err := NewFileStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewFileStore() error = %v", err)
	}
	_, found, err := store.Load(context.Background(), "missing")
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if found {
		t.Fatal("Load() found = true, want false")
	}
}

func TestFileStoreRejectsPathTraversal(t *testing.T) {
	store, err := NewFileStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewFileStore() error = %v", err)
	}
	_, _, err = store.Load(context.Background(), "../escape")
	if !errors.Is(err, ErrInvalidSessionID) {
		t.Fatalf("Load() error = %v, want invalid session id", err)
	}
}

func TestFileStoreUsesPrivateStateFile(t *testing.T) {
	root := t.TempDir()
	store, err := NewFileStore(root)
	if err != nil {
		t.Fatalf("NewFileStore() error = %v", err)
	}
	if err := store.Save(context.Background(), "private", State{
		PermissionMode: permission.ModeAsk.String(),
	}); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	info, err := os.Stat(filepath.Join(root, "private.json"))
	if err != nil {
		t.Fatalf("Stat() error = %v", err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Fatalf("state file mode = %#o, want %#o", got, 0o600)
	}
}
