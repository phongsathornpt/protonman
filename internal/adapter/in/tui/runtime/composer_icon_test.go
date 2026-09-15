package runtime

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	tuistyle "github.com/phongsathornpt/protonman/internal/adapter/in/tui/view/style"
)

func TestBottomPaneUsesProvidedComposerIcon(t *testing.T) {
	pane := newBottomPane(true, false)
	pane.setIcons(tuistyle.UnicodeIcons)
	if got := pane.prompt().Prompt; got != tuistyle.UnicodeIcons.Composer {
		t.Fatalf("prompt = %q, want Unicode composer %q", got, tuistyle.UnicodeIcons.Composer)
	}
}

func TestUnicodeProfilePreservesHistoricalComposerPrompt(t *testing.T) {
	pane := newBottomPane(true, false)
	pane.setIcons(tuistyle.UnicodeIcons)
	if got := pane.prompt().Prompt; got != "> " {
		t.Fatalf("unicode composer = %q, want historical prompt", got)
	}
}

func TestBashModePreservesCommandPromptAcrossIconProfiles(t *testing.T) {
	pane := newBottomPane(true, false)
	pane.setIcons(tuistyle.UnicodeIcons)
	pane.setBashMode(true)
	if got := pane.prompt().Prompt; got != "! " {
		t.Fatalf("bash prompt = %q, want existing command affordance", got)
	}
	pane.setBashMode(false)
	if got := pane.prompt().Prompt; got != tuistyle.UnicodeIcons.Composer {
		t.Fatalf("restored prompt = %q, want Unicode composer %q", got, tuistyle.UnicodeIcons.Composer)
	}
}

func TestAttachImageUsesStructuredPlaceholderWithoutChangingPromptIcon(t *testing.T) {
	pane := newBottomPane(true, false)
	pane.setIcons(tuistyle.ASCIIIcons)
	pane.attachImage("/tmp/one.png")
	pane.attachImage("/tmp/two.webp")

	if got := pane.prompt().Value(); got != "[Image #1] [Image #2]" {
		t.Fatalf("composer value = %q", got)
	}
	if got := pane.prompt().Prompt; got != tuistyle.ASCIIIcons.Composer {
		t.Fatalf("attachment changed prompt icon = %q", got)
	}
	attachments := pane.composer.attachments.snapshot(pane.prompt())
	if len(attachments) != 2 || attachments[0].Path != "/tmp/one.png" || attachments[1].Path != "/tmp/two.webp" {
		t.Fatalf("attachments = %+v", attachments)
	}
	if got := stripAttachmentPlaceholders(pane.prompt().Value(), attachments); got != "" {
		t.Fatalf("model text = %q, want empty image-only text", got)
	}
}

func TestDeletedImagePlaceholderPrunesAttachment(t *testing.T) {
	pane := newBottomPane(true, false)
	pane.attachImage("/tmp/one.png")
	pane.attachImage("/tmp/two.png")
	pane.prompt().SetValue("[Image #2] explain this")

	attachments := pane.composer.attachments.snapshot(pane.prompt())
	if len(attachments) != 1 || attachments[0].Path != "/tmp/two.png" {
		t.Fatalf("attachments after prune = %+v", attachments)
	}
	if got := pane.prompt().Value(); !strings.Contains(got, "[Image #1]") || strings.Contains(got, "[Image #2]") {
		t.Fatalf("placeholder was not relabeled: %q", got)
	}
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
		if got := normalizePastedPath(input, tempDir); got != "sample image.png" {
			t.Fatalf("normalizePastedPath(%q) = %q", input, got)
		}
	}

	text := "hello world from prompt"
	if got := normalizePastedPath(text, tempDir); got != text {
		t.Fatalf("plain text changed: %q", got)
	}
}

func TestLocalImagePathFromPasteRecognizesSupportedImagesOnly(t *testing.T) {
	tempDir := t.TempDir()
	imagePath := filepath.Join(tempDir, "screen shot.png")
	if err := os.WriteFile(imagePath, []byte("placeholder"), 0o644); err != nil {
		t.Fatal(err)
	}
	path, ok := localImagePathFromPaste(fmt.Sprintf("'%s'", imagePath), tempDir)
	if !ok || path != imagePath {
		t.Fatalf("image paste = (%q, %v), want %q", path, ok, imagePath)
	}

	svgPath := filepath.Join(tempDir, "vector.svg")
	if err := os.WriteFile(svgPath, []byte("<svg/>"), 0o644); err != nil {
		t.Fatal(err)
	}
	if path, ok := localImagePathFromPaste(svgPath, tempDir); ok {
		t.Fatalf("unsupported svg accepted as image: %q", path)
	}
}
