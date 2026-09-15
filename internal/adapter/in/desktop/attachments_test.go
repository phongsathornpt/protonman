//go:build desktop

package desktop

import (
	"encoding/base64"
	"testing"
)

func TestPromptContentBlocksEncodeImageAndText(t *testing.T) {
	a := &application{}
	blocks := a.promptContentBlocks("inspect this", []composerAttachment{
		{Name: "shot.png", MIMEType: "image/png", Data: []byte{0x89, 0x50, 0x4e, 0x47}, Image: true},
	})
	if len(blocks) != 2 {
		t.Fatalf("blocks = %d, want 2", len(blocks))
	}
	if blocks[0].Type != "text" || blocks[0].Text != "inspect this" {
		t.Fatalf("text block = %#v", blocks[0])
	}
	if blocks[1].Type != "image" || blocks[1].MIMEType != "image/png" {
		t.Fatalf("image block = %#v", blocks[1])
	}
	want := base64.StdEncoding.EncodeToString([]byte{0x89, 0x50, 0x4e, 0x47})
	if blocks[1].Data != want {
		t.Fatalf("image data = %q, want %q", blocks[1].Data, want)
	}
}

func TestPromptContentBlocksEmbedTextResource(t *testing.T) {
	a := &application{}
	blocks := a.promptContentBlocks("", []composerAttachment{
		{Name: "notes.txt", URI: "file:///tmp/notes.txt", MIMEType: "text/plain", Data: []byte("hello")},
	})
	if len(blocks) != 1 || blocks[0].Type != "resource" || blocks[0].Resource == nil {
		t.Fatalf("resource block = %#v", blocks)
	}
	if blocks[0].Resource.URI != "file:///tmp/notes.txt" || blocks[0].Resource.Text != "hello" {
		t.Fatalf("embedded resource = %#v", blocks[0].Resource)
	}
}
