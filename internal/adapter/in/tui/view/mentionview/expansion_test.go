package mentionview

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestExtractMentionTokens(t *testing.T) {
	tests := []struct {
		input string
		want  []string
	}{
		{input: "plain text", want: nil},
		{input: "@strength", want: []string{"strength"}},
		{input: "please review @main.go.", want: []string{"main.go"}},
		{input: "check @pkg/a.go, and @pkg/b.go!", want: []string{"pkg/a.go", "pkg/b.go"}},
		{input: "email user@example.com should be ignored", want: nil},
		{input: "@agility and @intelligence together", want: []string{"agility", "intelligence"}},
	}

	for _, tt := range tests {
		tokens := ExtractMentionTokens(tt.input)
		var got []string
		for _, tok := range tokens {
			got = append(got, tok.token)
		}
		if len(got) != len(tt.want) {
			t.Fatalf("extractMentionTokens(%q) = %v, want %v", tt.input, got, tt.want)
		}
		for i := range got {
			if got[i] != tt.want[i] {
				t.Errorf("token %d = %q, want %q", i, got[i], tt.want[i])
			}
		}
	}
}

func TestExpandMentionsNonexistentFile(t *testing.T) {
	tmpDir := t.TempDir()
	_, err := ExpandMentions("look at @nonexistent.go", tmpDir)
	if err == nil {
		t.Fatalf("expected error for nonexistent file, got nil")
	}
	if !strings.Contains(err.Error(), "file not found: nonexistent.go") {
		t.Errorf("error = %q, want 'file not found'", err.Error())
	}
}

func TestExpandMentionsTextFile(t *testing.T) {
	tmpDir := t.TempDir()
	sampleFile := filepath.Join(tmpDir, "hello.txt")
	content := "line 1\nline 2\nline 3"
	if err := os.WriteFile(sampleFile, []byte(content), 0o600); err != nil {
		t.Fatalf("write sample file: %v", err)
	}

	res, err := ExpandMentions("review @hello.txt please", tmpDir)
	if err != nil {
		t.Fatalf("ExpandMentions: %v", err)
	}

	if !strings.Contains(res.DisplayText, "@hello.txt (3 lines)") {
		t.Errorf("DisplayText = %q, want '@hello.txt (3 lines)'", res.DisplayText)
	}

	if !strings.Contains(res.TurnPrompt, "<file path=\"hello.txt\">\nline 1\nline 2\nline 3\n</file>") {
		t.Errorf("TurnPrompt missing expected file block:\n%s", res.TurnPrompt)
	}
}

func TestExpandMentionsTruncatedLargeFile(t *testing.T) {
	tmpDir := t.TempDir()
	largeFile := filepath.Join(tmpDir, "large.txt")
	var sb strings.Builder
	for i := 1; i <= 600; i++ {
		sb.WriteString(fmt.Sprintf("line %d\n", i))
	}
	if err := os.WriteFile(largeFile, []byte(sb.String()), 0o600); err != nil {
		t.Fatalf("write large file: %v", err)
	}

	res, err := ExpandMentions("inspect @large.txt", tmpDir)
	if err != nil {
		t.Fatalf("ExpandMentions: %v", err)
	}

	if !strings.Contains(res.TurnPrompt, "[... truncated at 500 lines; 600 total lines") {
		t.Errorf("expected truncation notice in TurnPrompt, got:\n%s", res.TurnPrompt)
	}
}

func TestExpandMentionsDirectory(t *testing.T) {
	tmpDir := t.TempDir()
	subDir := filepath.Join(tmpDir, "pkg")
	if err := os.Mkdir(subDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	_ = os.WriteFile(filepath.Join(subDir, "a.go"), []byte("package pkg"), 0o600)
	_ = os.WriteFile(filepath.Join(subDir, "b.go"), []byte("package pkg"), 0o600)

	res, err := ExpandMentions("explore @pkg/", tmpDir)
	if err != nil {
		t.Fatalf("ExpandMentions: %v", err)
	}

	if !strings.Contains(res.DisplayText, "@pkg (2 items)") && !strings.Contains(res.DisplayText, "@pkg/ (2 items)") {
		t.Errorf("DisplayText = %q, want items count", res.DisplayText)
	}

	if !strings.Contains(res.TurnPrompt, "<directory path=\"pkg\">") && !strings.Contains(res.TurnPrompt, "<directory path=\"pkg/\">") {
		t.Errorf("TurnPrompt missing directory block:\n%s", res.TurnPrompt)
	}
}

func TestExpandMentionsAgentDirective(t *testing.T) {
	tmpDir := t.TempDir()

	// Single agent
	res1, err := ExpandMentions("@strength refactor the code", tmpDir)
	if err != nil {
		t.Fatalf("single agent: %v", err)
	}
	if !strings.HasPrefix(res1.TurnPrompt, "[Requested Subagent: strength]") {
		t.Errorf("TurnPrompt = %q, want prefix [Requested Subagent: strength]", res1.TurnPrompt)
	}

	// Multiple agents
	res2, err := ExpandMentions("@agility then @intelligence analyze this", tmpDir)
	if err != nil {
		t.Fatalf("multiple agents: %v", err)
	}
	if !strings.HasPrefix(res2.TurnPrompt, "[Requested Subagents: agility, intelligence]") {
		t.Errorf("TurnPrompt = %q, want prefix [Requested Subagents: agility, intelligence]", res2.TurnPrompt)
	}
}

func TestExpandMentionsImageFile(t *testing.T) {
	tmpDir := t.TempDir()
	imgFile := filepath.Join(tmpDir, "diagram.png")
	if err := os.WriteFile(imgFile, []byte("fake-png-bytes"), 0o600); err != nil {
		t.Fatalf("write fake image: %v", err)
	}

	res, err := ExpandMentions("see @diagram.png", tmpDir)
	if err != nil {
		t.Fatalf("ExpandMentions: %v", err)
	}

	if len(res.ImagePaths) != 1 || res.ImagePaths[0] != imgFile {
		t.Errorf("ImagePaths = %v, want [%s]", res.ImagePaths, imgFile)
	}
}
