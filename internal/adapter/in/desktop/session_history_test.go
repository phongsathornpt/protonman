//go:build desktop

package desktop

import (
	"errors"
	"strings"
	"testing"
)

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

func TestSessionHistoryReplayCommitsOnlyAfterSuccess(t *testing.T) {
	a := &application{transcripts: map[string]*strings.Builder{"session-1": &strings.Builder{}}}
	a.transcripts["session-1"].WriteString("existing transcript")
	tracker := sessionHistoryTrackerFor(a)
	tracker.mu.Lock()
	tracker.states["session-1"] = sessionHistoryLoading
	tracker.staging["session-1"] = &strings.Builder{}
	tracker.mu.Unlock()

	if !stageSessionHistoryChunk(a, "session-1", "replayed transcript") {
		t.Fatal("expected replay chunk to be staged")
	}
	if got := a.transcripts["session-1"].String(); got != "existing transcript" {
		t.Fatalf("live transcript changed before replay completed: %q", got)
	}

	finishSessionHistoryLoad(a, "session-1", nil)
	if got := a.transcripts["session-1"].String(); got != "replayed transcript" {
		t.Fatalf("successful replay was not committed atomically: %q", got)
	}
	if sessionHistoryIsLoading(a, "session-1") {
		t.Fatal("successful replay remained loading")
	}

	sessionHistoryTrackers.Delete(a)
}

func TestSessionHistoryReplayFailureDiscardsPartialReplay(t *testing.T) {
	a := &application{transcripts: map[string]*strings.Builder{"session-1": &strings.Builder{}}}
	a.transcripts["session-1"].WriteString("known good transcript")
	tracker := sessionHistoryTrackerFor(a)
	tracker.mu.Lock()
	tracker.states["session-1"] = sessionHistoryLoading
	tracker.staging["session-1"] = &strings.Builder{}
	tracker.mu.Unlock()

	stageSessionHistoryChunk(a, "session-1", "partial replay")
	finishSessionHistoryLoad(a, "session-1", errors.New("connection lost"))

	if got := a.transcripts["session-1"].String(); got != "known good transcript" {
		t.Fatalf("failed replay replaced known good transcript: %q", got)
	}
	tracker.mu.Lock()
	state := tracker.states["session-1"]
	_, staged := tracker.staging["session-1"]
	tracker.mu.Unlock()
	if state != sessionHistoryUnloaded {
		t.Fatalf("failed replay state = %v, want unloaded", state)
	}
	if staged {
		t.Fatal("failed replay staging buffer was retained")
	}

	sessionHistoryTrackers.Delete(a)
}
