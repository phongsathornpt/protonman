package runtime

import (
	"testing"

	tuistyle "github.com/phongsathornpt/protonman/internal/adapter/in/tui/view/style"
)

func TestBottomPaneUsesProvidedPromptIcon(t *testing.T) {
	pane := newBottomPane(true, false)
	pane.setIcons(tuistyle.NerdIcons)
	if got := pane.prompt().Prompt; got != tuistyle.NerdIcons.Prompt {
		t.Fatalf("prompt = %q, want Nerd prompt %q", got, tuistyle.NerdIcons.Prompt)
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
	if got := pane.prompt().Prompt; got != tuistyle.NerdIcons.Prompt {
		t.Fatalf("restored prompt = %q, want Nerd prompt %q", got, tuistyle.NerdIcons.Prompt)
	}
}
