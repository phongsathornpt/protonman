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
}
