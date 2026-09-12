package cmdpolicy

import "testing"

func TestClassifyCommands(t *testing.T) {
	tests := []struct {
		line     string
		wantKind Kind
		wantName string
		wantArg  string
	}{
		{"/help", KindHelp, "help", ""},
		{"/model gpt-4o", KindModel, "model", "gpt-4o"},
		{"/provider add", KindProvider, "provider", "add"},
		{"/low on", KindLow, "low", "on"},
		{"/clear", KindClear, "clear", ""},
		{"/resume abc", KindResume, "resume", "abc"},
		{"/todo show", KindTodo, "todo", "show"},
		{"/goal fix bug", KindGoal, "goal", "fix"},
		{"/quit", KindQuit, "quit", ""},
		{"/unknown-cmd", KindUnknown, "unknown-cmd", ""},
	}

	for _, tc := range tests {
		cmd := Classify(tc.line)
		if cmd.Kind != tc.wantKind {
			t.Errorf("Classify(%q).Kind = %v; want %v", tc.line, cmd.Kind, tc.wantKind)
		}
		if cmd.Name != tc.wantName {
			t.Errorf("Classify(%q).Name = %q; want %q", tc.line, cmd.Name, tc.wantName)
		}
		if cmd.Argument != tc.wantArg {
			t.Errorf("Classify(%q).Argument = %q; want %q", tc.line, cmd.Argument, tc.wantArg)
		}
	}
}

func TestCommandIsKnown(t *testing.T) {
	if !Classify("/help").IsKnown() {
		t.Error("expected /help to be known")
	}
	if Classify("/not-a-command").IsKnown() {
		t.Error("expected /not-a-command to be unknown")
	}
}
