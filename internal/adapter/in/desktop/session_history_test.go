//go:build desktop

package desktop

import "testing"

func TestFormatUserTranscriptQuotesMultilinePrompt(t *testing.T) {
	got := formatUserTranscript("  hello\nworld  ")
	want := "\n\n> hello\n> world\n\n"
	if got != want {
		t.Fatalf("formatUserTranscript() = %q, want %q", got, want)
	}
}

func TestFormatUserTranscriptIgnoresWhitespace(t *testing.T) {
	if got := formatUserTranscript("  \n\t "); got != "" {
		t.Fatalf("formatUserTranscript() = %q, want empty string", got)
	}
}

func TestSessionHistoryLoadingState(t *testing.T) {
	a := &application{}
	tracker := sessionHistoryTrackerFor(a)
	tracker.mu.Lock()
	tracker.states["session-1"] = sessionHistoryLoading
	tracker.mu.Unlock()

	if !sessionHistoryIsLoading(a, "session-1") {
		t.Fatal("expected loading session to be reported as loading")
	}
	if sessionHistoryIsLoading(a, "session-2") {
		t.Fatal("untracked session must not be reported as loading")
	}

	sessionHistoryTrackers.Delete(a)
}
