package tool

import "testing"

func TestVerificationCommandConservativeRecognition(t *testing.T) {
	tests := []struct {
		command string
		label   string
		ok      bool
	}{
		{"go test ./...", "go test", true},
		{"cargo clippy --all-targets", "cargo clippy", true},
		{"pytest -q", "pytest", true},
		{"pnpm run typecheck", "pnpm typecheck", true},
		{"git diff --check", "git diff --check", true},
		{"go test ./... && git diff --check", "go test + git diff --check", true},
		{"echo go test ./...", "", false},
		{"go test ./... > result.txt", "", false},
		{"make test", "", false},
	}
	for _, test := range tests {
		label, ok := VerificationCommand(test.command)
		if ok != test.ok || label != test.label {
			t.Fatalf("VerificationCommand(%q) = (%q,%v), want (%q,%v)", test.command, label, ok, test.label, test.ok)
		}
	}
}
