package sessionfs

import (
	"context"
	"encoding/base64"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/phongsathornpt/protonman/internal/adapter/out/model"
	"github.com/phongsathornpt/protonman/internal/core/permission"
	"github.com/phongsathornpt/protonman/internal/core/session"
)

func imageSessionMessages(raw []byte, text string) []session.Message {
	data := base64.StdEncoding.EncodeToString(raw)
	return session.FromModelMessages([]model.Message{{
		Role:    model.RoleUser,
		Content: text,
		Parts: []model.ContentPart{
			{Type: model.ContentPartImage, MIMEType: "image/png", Data: data},
			{Type: model.ContentPartText, Text: text},
		},
	}})
}

func TestFileStorePersistsImagePartsAsSidecars(t *testing.T) {
	root := filepath.Join(t.TempDir(), "sessions")
	store, err := NewFileStore(root)
	if err != nil {
		t.Fatal(err)
	}
	rawImage := []byte("session-image-payload")
	data := base64.StdEncoding.EncodeToString(rawImage)
	messages := imageSessionMessages(rawImage, "inspect image")
	if err := store.Save(context.Background(), "image-session", State{
		PermissionMode: permission.ModeAsk.String(),
		Messages:       messages,
	}); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	resources, err := session.ResolveResources(root, "image-session")
	if err != nil {
		t.Fatal(err)
	}
	rawState, err := os.ReadFile(resources.State)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(rawState), data) || strings.Contains(string(rawState), "session-image-payload") {
		t.Fatalf("state.json contains inline image payload: %s", rawState)
	}
	if !strings.Contains(string(rawState), `"blob":`) {
		t.Fatalf("state.json has no image blob reference: %s", rawState)
	}
	entries, err := os.ReadDir(resources.Attachments)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("attachment sidecars = %d, want 1", len(entries))
	}

	loaded, found, err := store.Load(context.Background(), "image-session")
	if err != nil || !found {
		t.Fatalf("Load() = found %v, err %v", found, err)
	}
	restored := session.ToModelMessages(loaded.Messages)
	if len(restored) != 1 || len(restored[0].Parts) != 2 {
		t.Fatalf("restored messages = %+v", restored)
	}
	if restored[0].Parts[0].Type != model.ContentPartImage || restored[0].Parts[0].Data != data {
		t.Fatalf("restored image part = %+v", restored[0].Parts[0])
	}
}

func TestFileStorePrunesUnreferencedImageSidecarsAfterCommit(t *testing.T) {
	root := filepath.Join(t.TempDir(), "sessions")
	store, err := NewFileStore(root)
	if err != nil {
		t.Fatal(err)
	}
	const sessionID = "image-gc"
	first := []byte("first-image")
	second := []byte("second-image")
	if err := store.Save(context.Background(), sessionID, State{
		PermissionMode: permission.ModeAsk.String(),
		Messages:       imageSessionMessages(first, "first"),
	}); err != nil {
		t.Fatal(err)
	}
	resources, err := session.ResolveResources(root, sessionID)
	if err != nil {
		t.Fatal(err)
	}
	firstPath := filepath.Join(resources.Attachments, attachmentBlobName(first))
	if _, err := os.Stat(firstPath); err != nil {
		t.Fatalf("first sidecar missing before replacement: %v", err)
	}
	foreignPath := filepath.Join(resources.Attachments, "keep.txt")
	if err := os.WriteFile(foreignPath, []byte("foreign"), 0o600); err != nil {
		t.Fatal(err)
	}

	loaded, found, err := store.Load(context.Background(), sessionID)
	if err != nil || !found {
		t.Fatalf("Load() = found %v, err %v", found, err)
	}
	if err := store.Save(context.Background(), sessionID, State{
		Revision:       loaded.Revision,
		PermissionMode: permission.ModeAsk.String(),
		Messages:       imageSessionMessages(second, "second"),
	}); err != nil {
		t.Fatal(err)
	}

	if _, err := os.Stat(firstPath); !os.IsNotExist(err) {
		t.Fatalf("stale sidecar still exists, err=%v", err)
	}
	secondPath := filepath.Join(resources.Attachments, attachmentBlobName(second))
	if _, err := os.Stat(secondPath); err != nil {
		t.Fatalf("replacement sidecar missing: %v", err)
	}
	if _, err := os.Stat(foreignPath); err != nil {
		t.Fatalf("foreign file should not be owned by attachment GC: %v", err)
	}
}

func TestFileStoreDeduplicatesRepeatedImageSidecars(t *testing.T) {
	root := filepath.Join(t.TempDir(), "sessions")
	store, err := NewFileStore(root)
	if err != nil {
		t.Fatal(err)
	}
	raw := []byte("same-image")
	message := imageSessionMessages(raw, "same")[0]
	if err := store.Save(context.Background(), "image-dedupe", State{
		PermissionMode: permission.ModeAsk.String(),
		Messages:       []session.Message{message, message},
	}); err != nil {
		t.Fatal(err)
	}
	resources, err := session.ResolveResources(root, "image-dedupe")
	if err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(resources.Attachments)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Name() != attachmentBlobName(raw) {
		t.Fatalf("attachment sidecars = %+v, want one deduplicated blob", entries)
	}
}
