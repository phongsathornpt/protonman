package runtime

import (
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
