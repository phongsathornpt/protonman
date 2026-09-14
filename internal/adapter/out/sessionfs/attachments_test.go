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

func TestFileStorePersistsImagePartsAsSidecars(t *testing.T) {
	root := filepath.Join(t.TempDir(), "sessions")
	store, err := NewFileStore(root)
	if err != nil {
		t.Fatal(err)
	}
	data := base64.StdEncoding.EncodeToString([]byte("session-image-payload"))
	messages := session.FromModelMessages([]model.Message{{
		Role:    model.RoleUser,
		Content: "inspect image",
		Parts: []model.ContentPart{
			{Type: model.ContentPartImage, MIMEType: "image/png", Data: data},
			{Type: model.ContentPartText, Text: "inspect image"},
		},
	}})
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
