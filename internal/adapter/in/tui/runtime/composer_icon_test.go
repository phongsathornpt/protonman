package runtime

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	tuistyle "github.com/phongsathornpt/protonman/internal/adapter/in/tui/view/style"
	"github.com/phongsathornpt/protonman/internal/core/permission"
)

func TestBottomPaneUsesProvidedComposerIcon(t *testing.T) {
	pane := newBottomPane(true, false)
	pane.setIcons(tuistyle.NerdIcons)
	if got := pane.prompt().Prompt; got != tuistyle.NerdIcons.Composer {
		t.Fatalf("prompt = %q, want Nerd composer %q", got, tuistyle.NerdIcons.Composer)
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
	pane.setIcons(tuistyle.NerdIcons)
	pane.setBashMode(true)
	if got := pane.prompt().Prompt; got != "! " {
		t.Fatalf("bash prompt = %q, want existing command affordance", got)
	}
	pane.setBashMode(false)
	if got := pane.prompt().Prompt; got != tuistyle.NerdIcons.Composer {
		t.Fatalf("restored prompt = %q, want Nerd composer %q", got, tuistyle.NerdIcons.Composer)
	}
}

func TestNormalizePastedPath(t *testing.T) {
	tempDir := t.TempDir()
	filePath := filepath.Join(tempDir, "sample image.png")
	if err := os.WriteFile(filePath, []byte("fake"), 0o644); err != nil {
		t.Fatal(err)
	}

	// 1. Quoted path inside workspace becomes relative
	quoted := fmt.Sprintf("'%s'", filePath)
	if got := normalizePastedPath(quoted, tempDir); got != "sample image.png" {
		t.Fatalf("got %q, want 'sample image.png'", got)
	}

	// 2. Double quoted
	dquoted := fmt.Sprintf("\"%s\"", filePath)
	if got := normalizePastedPath(dquoted, tempDir); got != "sample image.png" {
		t.Fatalf("got %q, want 'sample image.png'", got)
	}

	// 3. file:// URI
	fileURI := fmt.Sprintf("file://%s", filePath)
	if got := normalizePastedPath(fileURI, tempDir); got != "sample image.png" {
		t.Fatalf("got %q, want 'sample image.png'", got)
	}

	// 4. Escaped spaces
	escaped := strings.ReplaceAll(filePath, " ", `\ `)
	if got := normalizePastedPath(escaped, tempDir); got != "sample image.png" {
		t.Fatalf("got %q, want 'sample image.png'", got)
	}

	// 5. Normal text untouched
	text := "hello world from prompt"
	if got := normalizePastedPath(text, tempDir); got != text {
		t.Fatalf("got %q, want %q", got, text)
	}

	// 6. Code untouched
	code := "func main() {\n  fmt.Println(\"ok\")\n}"
	if got := normalizePastedPath(code, tempDir); got != code {
		t.Fatalf("got %q, want %q", got, code)
	}

	// 7. Unquoted path inside workspace becomes relative
	if got := normalizePastedPath(filePath, tempDir); got != "sample image.png" {
		t.Fatalf("got %q, want 'sample image.png'", got)
	}

	// 8. Trailing newline trimmed
	withNewline := filePath + "\n"
	if got := normalizePastedPath(withNewline, tempDir); got != "sample image.png" {
		t.Fatalf("got %q, want 'sample image.png'", got)
	}
	withCRLF := filePath + "\r\n"
	if got := normalizePastedPath(withCRLF, tempDir); got != "sample image.png" {
		t.Fatalf("got %q, want 'sample image.png'", got)
	}

	// 9. External file outside workspace is cleaned and unquoted
	otherDir := t.TempDir()
	outsideFile := filepath.Join(otherDir, "external image.png")
	if err := os.WriteFile(outsideFile, []byte("fake"), 0o644); err != nil {
		t.Fatal(err)
	}
	cleanedOutside := filepath.Clean(outsideFile)
	if got := normalizePastedPath(outsideFile, tempDir); got != cleanedOutside {
		t.Fatalf("got %q, want %q", got, cleanedOutside)
	}
	quotedOutside := fmt.Sprintf("'%s'", outsideFile)
	if got := normalizePastedPath(quotedOutside, tempDir); got != cleanedOutside {
		t.Fatalf("got %q, want %q", got, cleanedOutside)
	}
}

func TestSubmitDroppedImagePathDoesNotTriggerUnknownCommand(t *testing.T) {
	wsDir := t.TempDir()
	insideFile := filepath.Join(wsDir, "screenshot.png")
	if err := os.WriteFile(insideFile, []byte("fake"), 0o644); err != nil {
		t.Fatal(err)
	}

	otherDir := t.TempDir()
	outsideFile := filepath.Join(otherDir, "desktop.png")
	if err := os.WriteFile(outsideFile, []byte("fake"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Test case 1: Dropped absolute path inside workspace (unquoted)
	m := newTestBubbleModel(t, permission.ModeAlwaysApprove, emptyTodoItems())
	m.workDir = wsDir
	m.panes.bottom.prompt().SetValue(insideFile)
	m.submit()
	if strings.Contains(plainTranscript(m), "unknown command") {
		t.Fatalf("inside image path triggered unknown command: %s", plainTranscript(m))
	}

	// Test case 2: Dropped absolute path outside workspace (unquoted)
	m2 := newTestBubbleModel(t, permission.ModeAlwaysApprove, emptyTodoItems())
	m2.workDir = wsDir
	m2.panes.bottom.prompt().SetValue(outsideFile)
	m2.submit()
	if strings.Contains(plainTranscript(m2), "unknown command") {
		t.Fatalf("outside image path triggered unknown command: %s", plainTranscript(m2))
	}

	// Test case 3: Dropped quoted path outside workspace
	m3 := newTestBubbleModel(t, permission.ModeAlwaysApprove, emptyTodoItems())
	m3.workDir = wsDir
	m3.panes.bottom.prompt().SetValue("'" + outsideFile + "'")
	m3.submit()
	if strings.Contains(plainTranscript(m3), "unknown command") {
		t.Fatalf("quoted outside image path triggered unknown command: %s", plainTranscript(m3))
	}
}
