package composer

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"charm.land/bubbles/v2/textarea"

	tuiconv "github.com/phongsathornpt/protonman/internal/adapter/in/tui/runtime/conversation"
)

func newTestPrompt() textarea.Model {
	prompt := textarea.New()
	prompt.CharLimit = 0
	return prompt
}

func TestNormalizePastedPath(t *testing.T) {
	tempDir := t.TempDir()
	filePath := filepath.Join(tempDir, "sample image.png")
	if err := os.WriteFile(filePath, []byte("fake"), 0o644); err != nil {
		t.Fatal(err)
	}

	for _, input := range []string{
		fmt.Sprintf("'%s'", filePath),
		fmt.Sprintf("\"%s\"", filePath),
		fmt.Sprintf("file://%s", filePath),
		strings.ReplaceAll(filePath, " ", `\ `),
		filePath,
	} {
		if got := NormalizePastedPath(input, tempDir); got != "sample image.png" {
			t.Fatalf("NormalizePastedPath(%q) = %q", input, got)
		}
	}

	text := "hello world from prompt"
	if got := NormalizePastedPath(text, tempDir); got != text {
		t.Fatalf("plain text changed: %q", got)
	}
}

func TestLocalImagePathFromPasteRecognizesSupportedImagesOnly(t *testing.T) {
	tempDir := t.TempDir()
	imagePath := filepath.Join(tempDir, "screen shot.png")
	if err := os.WriteFile(imagePath, []byte("placeholder"), 0o644); err != nil {
		t.Fatal(err)
	}
	path, ok := LocalImagePathFromPaste(fmt.Sprintf("'%s'", imagePath), tempDir)
	if !ok || path != imagePath {
		t.Fatalf("image paste = (%q, %v), want %q", path, ok, imagePath)
	}

	svgPath := filepath.Join(tempDir, "vector.svg")
	if err := os.WriteFile(svgPath, []byte("<svg/>"), 0o644); err != nil {
		t.Fatal(err)
	}
	if path, ok := LocalImagePathFromPaste(svgPath, tempDir); ok {
		t.Fatalf("unsupported svg accepted as image: %q", path)
	}
}

func TestLocalImagePathFromPasteRejectsMultilineAndMissingFiles(t *testing.T) {
	tempDir := t.TempDir()
	if _, ok := LocalImagePathFromPaste("a.png\nb.png", tempDir); ok {
		t.Fatal("multi-line paste accepted as image path")
	}
	if _, ok := LocalImagePathFromPaste(filepath.Join(tempDir, "absent.png"), tempDir); ok {
		t.Fatal("missing file accepted as image path")
	}
}

// Attachment placeholders are user-visible text in the prompt, so renumbering
// after the user deletes one must keep labels contiguous and visible.
func TestSyncWithTextRenumbersSurvivors(t *testing.T) {
	prompt := newTestPrompt()
	var state State
	state.Attach(&prompt, "/tmp/one.png")
	state.Attach(&prompt, "/tmp/two.png")
	state.Attach(&prompt, "/tmp/three.png")

	prompt.SetValue("[Image #1] [Image #3]")
	got := state.Snapshot(&prompt)
	if len(got) != 2 {
		t.Fatalf("attachments = %d, want 2", len(got))
	}
	if got[0].Placeholder != "[Image #1]" || got[1].Placeholder != "[Image #2]" {
		t.Fatalf("placeholders not renumbered: %+v", got)
	}
	if value := prompt.Value(); value != "[Image #1] [Image #2]" {
		t.Fatalf("prompt = %q, want renumbered placeholders", value)
	}
}

func TestSyncWithTextDeletesOrphanedTemporaryFiles(t *testing.T) {
	tempDir := t.TempDir()
	kept := filepath.Join(tempDir, "kept.png")
	dropped := filepath.Join(tempDir, "dropped.png")
	for _, path := range []string{kept, dropped} {
		if err := os.WriteFile(path, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	prompt := newTestPrompt()
	var state State
	state.AttachTemporary(&prompt, kept)
	state.AttachTemporary(&prompt, dropped)

	prompt.SetValue("[Image #1]")
	got := state.Snapshot(&prompt)
	if len(got) != 1 || got[0].Path != kept {
		t.Fatalf("attachments = %+v, want only %q", got, kept)
	}
	if _, err := os.Stat(dropped); !os.IsNotExist(err) {
		t.Fatalf("orphaned temporary survived pruning: %v", err)
	}
	if _, err := os.Stat(kept); err != nil {
		t.Fatalf("retained temporary was deleted: %v", err)
	}
}

func TestRemoveLastDeletesOwnedTemporaryFile(t *testing.T) {
	tempDir := t.TempDir()
	path := filepath.Join(tempDir, "shot.png")
	if err := os.WriteFile(path, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	prompt := newTestPrompt()
	var state State
	state.AttachTemporary(&prompt, path)
	if !state.RemoveLast(&prompt) {
		t.Fatal("RemoveLast reported no removal")
	}
	if state.Len() != 0 {
		t.Fatalf("Len = %d, want 0", state.Len())
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("owned temporary survived removal: %v", err)
	}
}

func TestReleaseKeepsFilesButClearDeletesThem(t *testing.T) {
	tempDir := t.TempDir()
	releasePath := filepath.Join(tempDir, "release.png")
	clearPath := filepath.Join(tempDir, "clear.png")
	for _, path := range []string{releasePath, clearPath} {
		if err := os.WriteFile(path, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	prompt := newTestPrompt()
	var released State
	released.AttachTemporary(&prompt, releasePath)
	released.Release()
	if released.Len() != 0 {
		t.Fatalf("Release left %d attachments", released.Len())
	}
	if _, err := os.Stat(releasePath); err != nil {
		t.Fatalf("Release deleted an owned file: %v", err)
	}

	var cleared State
	cleared.AttachTemporary(&prompt, clearPath)
	cleared.Clear()
	if _, err := os.Stat(clearPath); !os.IsNotExist(err) {
		t.Fatalf("Clear kept an owned temporary: %v", err)
	}
}

// StripPlaceholders removes the tokens without rewriting surrounding spacing;
// only the overall result is trimmed. This keeps submitted prose byte-identical
// to what the user typed around their attachments.
func TestStripPlaceholdersRemovesEveryAttachmentToken(t *testing.T) {
	attachments := []tuiconv.Attachment{
		{Placeholder: "[Image #1]", Path: "/tmp/one.png"},
		{Placeholder: "[Image #2]", Path: "/tmp/two.png"},
	}
	if got := StripPlaceholders("look at [Image #1] and [Image #2] closely", attachments); got != "look at  and  closely" {
		t.Fatalf("StripPlaceholders = %q", got)
	}
	if got := StripPlaceholders("[Image #1]", attachments); got != "" {
		t.Fatalf("image-only text = %q, want empty", got)
	}
}
