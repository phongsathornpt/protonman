package main

import (
	"strings"
	"testing"
)

func TestParseArgsPromptAliases(t *testing.T) {
	options, err := parseArgs([]string{"-p", "hello"})
	if err != nil {
		t.Fatalf("parseArgs() error = %v", err)
	}
	if options.prompt != "hello" {
		t.Fatalf("prompt = %q, want hello", options.prompt)
	}

	options, err = parseArgs([]string{"--prompt", "/call read_file {}"})
	if err != nil {
		t.Fatalf("parseArgs() error = %v", err)
	}
	if options.prompt != "/call read_file {}" {
		t.Fatalf("prompt = %q", options.prompt)
	}
}

func TestParseArgsPositionalPrompt(t *testing.T) {
	options, err := parseArgs([]string{"list", "the", "tools"})
	if err != nil {
		t.Fatalf("parseArgs() error = %v", err)
	}
	if options.prompt != "list the tools" {
		t.Fatalf("prompt = %q, want positional join", options.prompt)
	}
}

func TestUsageMentionsHeadless(t *testing.T) {
	text := usage()
	for _, expected := range []string{"-p", "--headless", "--output", "--acp", "--sandbox", "--resume", "--new-session", "--session"} {
		if !strings.Contains(text, expected) {
			t.Fatalf("usage missing %q: %s", expected, text)
		}
	}
}

func TestParseArgsSessionFlags(t *testing.T) {
	for _, flag := range []string{"-r", "--resume", "--continue", "-c"} {
		opts, err := parseArgs([]string{flag})
		if err != nil {
			t.Fatalf("parseArgs(%q) error = %v", flag, err)
		}
		if !opts.resume {
			t.Fatalf("parseArgs(%q) resume = false, want true", flag)
		}
	}

	for _, flag := range []string{"-n", "--new-session", "--new"} {
		opts, err := parseArgs([]string{flag})
		if err != nil {
			t.Fatalf("parseArgs(%q) error = %v", flag, err)
		}
		if !opts.newSession {
			t.Fatalf("parseArgs(%q) newSession = false, want true", flag)
		}
	}

	for _, flag := range []string{"-s", "--session"} {
		opts, err := parseArgs([]string{flag, "test-sess"})
		if err != nil {
			t.Fatalf("parseArgs(%q) error = %v", flag, err)
		}
		if opts.sessionID != "test-sess" {
			t.Fatalf("parseArgs(%q) sessionID = %q, want test-sess", flag, opts.sessionID)
		}
	}
}

func TestParseArgsSessionFlagsMutualExclusion(t *testing.T) {
	_, err := parseArgs([]string{"--resume", "--new-session"})
	if err == nil {
		t.Fatal("parseArgs(--resume, --new-session) expected error, got nil")
	}
	if !strings.Contains(err.Error(), "cannot specify both --resume and --new-session") {
		t.Fatalf("unexpected error message: %v", err)
	}
}

