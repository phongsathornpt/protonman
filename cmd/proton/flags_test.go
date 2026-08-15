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
	for _, expected := range []string{"-p", "--headless", "--output", "--acp", "--sandbox"} {
		if !strings.Contains(text, expected) {
			t.Fatalf("usage missing %q: %s", expected, text)
		}
	}
}
