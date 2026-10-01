//go:build desktop || desktop_gio

package shell

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	conversationcomponent "github.com/phongsathornpt/protonman/internal/adapter/in/desktop/gioui/component/conversation"
	"github.com/phongsathornpt/protonman/internal/adapter/in/desktop/gioui/controller"
)

func TestParseMentionContext(t *testing.T) {
	for _, tc := range []struct {
		name      string
		input     string
		wantOK    bool
		wantQuery string
		wantStart int
		wantEnd   int
	}{
		{name: "empty", input: "", wantOK: false},
		{name: "plain text", input: "hello world", wantOK: false},
		{name: "bare at start", input: "@", wantOK: true, wantQuery: "", wantStart: 0, wantEnd: 1},
		{name: "at with query at start", input: "@str", wantOK: true, wantQuery: "str", wantStart: 0, wantEnd: 4},
		{name: "bare at with leading space", input: "hello @", wantOK: true, wantQuery: "", wantStart: 6, wantEnd: 7},
		{name: "at with query after space", input: "hello @main.go", wantOK: true, wantQuery: "main.go", wantStart: 6, wantEnd: 14},
		{name: "email address rejected", input: "user@example.com", wantOK: false},
		{name: "trailing space closes mention", input: "hello @str ", wantOK: false},
		{name: "punctuation in query rejected", input: "hello @str;foo", wantOK: false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, ok := conversationcomponent.ParseMentionContext(tc.input)
			if ok != tc.wantOK {
				t.Fatalf("conversationcomponent.ParseMentionContext(%q) ok = %v, want %v", tc.input, ok, tc.wantOK)
			}
			if !tc.wantOK {
				return
			}
			if ctx.Query != tc.wantQuery {
				t.Errorf("Query = %q, want %q", ctx.Query, tc.wantQuery)
			}
			if ctx.StartOffset != tc.wantStart {
				t.Errorf("StartOffset = %d, want %d", ctx.StartOffset, tc.wantStart)
			}
			if ctx.EndOffset != tc.wantEnd {
				t.Errorf("EndOffset = %d, want %d", ctx.EndOffset, tc.wantEnd)
			}
		})
	}
}

func TestMatchMentionItems(t *testing.T) {
	agents := conversationcomponent.DefaultMentionAgents()
	workspaceFiles := []MentionItem{
		{Kind: MentionItemKindFile, Name: "main.go", Description: "1 KB", PrefixTag: "[file]"},
		{Kind: MentionItemKindFile, Name: "internal/adapter/app.go", Description: "2 KB", PrefixTag: "[file]"},
		{Kind: MentionItemKindDir, Name: "cmd", Description: "Directory", PrefixTag: "[dir]"},
	}

	// Empty query matches all agents and files
	all := conversationcomponent.MatchMentionItems(conversationcomponent.MentionContext{Query: ""}, agents, workspaceFiles)
	if len(all) != len(agents)+len(workspaceFiles) {
		t.Fatalf("conversationcomponent.MatchMentionItems empty query = %d items, want %d", len(all), len(agents)+len(workspaceFiles))
	}

	// Query "str" matches agent "strength" first
	strMatches := conversationcomponent.MatchMentionItems(conversationcomponent.MentionContext{Query: "str"}, agents, workspaceFiles)
	if len(strMatches) == 0 || strMatches[0].Name != "strength" {
		t.Fatalf("conversationcomponent.MatchMentionItems 'str' first = %#v, want strength", strMatches)
	}

	// Query "main" matches main.go
	mainMatches := conversationcomponent.MatchMentionItems(conversationcomponent.MentionContext{Query: "main"}, agents, workspaceFiles)
	foundMain := false
	for _, m := range mainMatches {
		if m.Name == "main.go" {
			foundMain = true
			break
		}
	}
	if !foundMain {
		t.Fatalf("conversationcomponent.MatchMentionItems 'main' did not find main.go: %#v", mainMatches)
	}
}

func TestExpandMentions(t *testing.T) {
	tmpDir := t.TempDir()

	// Create test file
	filePath := filepath.Join(tmpDir, "example.txt")
	if err := os.WriteFile(filePath, []byte("line1\nline2\nline3\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Create sub directory with file
	subDir := filepath.Join(tmpDir, "pkg")
	if err := os.Mkdir(subDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(subDir, "mod.go"), []byte("package pkg\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Create dummy image file
	imgPath := filepath.Join(tmpDir, "test.png")
	if err := os.WriteFile(imgPath, []byte("fake-png-bytes"), 0o644); err != nil {
		t.Fatal(err)
	}

	t.Run("agent mention", func(t *testing.T) {
		expanded, err := controller.ExpandMentions("please ask @strength to fix", tmpDir)
		if err != nil {
			t.Fatalf("controller.ExpandMentions error: %v", err)
		}
		if len(expanded.MentionedAgents) != 1 || expanded.MentionedAgents[0] != "strength" {
			t.Errorf("MentionedAgents = %#v, want [strength]", expanded.MentionedAgents)
		}
		if !strings.HasPrefix(expanded.TurnPrompt, "[Requested Subagent: strength]") {
			t.Errorf("TurnPrompt missing agent directive: %q", expanded.TurnPrompt)
		}
	})

	t.Run("file mention", func(t *testing.T) {
		expanded, err := controller.ExpandMentions("inspect @example.txt please", tmpDir)
		if err != nil {
			t.Fatalf("controller.ExpandMentions error: %v", err)
		}
		if len(expanded.MentionedFiles) != 1 || expanded.MentionedFiles[0] != "example.txt" {
			t.Errorf("MentionedFiles = %#v, want [example.txt]", expanded.MentionedFiles)
		}
		if !strings.Contains(expanded.DisplayText, "@example.txt (3 lines)") {
			t.Errorf("DisplayText = %q, want '@example.txt (3 lines)'", expanded.DisplayText)
		}
		if !strings.Contains(expanded.TurnPrompt, "<file path=\"example.txt\">") {
			t.Errorf("TurnPrompt missing file block: %q", expanded.TurnPrompt)
		}
	})

	t.Run("directory mention", func(t *testing.T) {
		expanded, err := controller.ExpandMentions("explore @pkg", tmpDir)
		if err != nil {
			t.Fatalf("controller.ExpandMentions error: %v", err)
		}
		if !strings.Contains(expanded.DisplayText, "@pkg (1 items)") {
			t.Errorf("DisplayText = %q, want '@pkg (1 items)'", expanded.DisplayText)
		}
		if !strings.Contains(expanded.TurnPrompt, "<directory path=\"pkg\">") {
			t.Errorf("TurnPrompt missing directory block: %q", expanded.TurnPrompt)
		}
	})

	t.Run("image mention", func(t *testing.T) {
		expanded, err := controller.ExpandMentions("review @test.png", tmpDir)
		if err != nil {
			t.Fatalf("controller.ExpandMentions error: %v", err)
		}
		if len(expanded.ImagePaths) != 1 || expanded.ImagePaths[0] != imgPath {
			t.Errorf("ImagePaths = %#v, want [%s]", expanded.ImagePaths, imgPath)
		}
	})

	t.Run("nonexistent file returns error", func(t *testing.T) {
		_, err := controller.ExpandMentions("check @nonexistent.go", tmpDir)
		if err == nil {
			t.Fatal("expected error for nonexistent file, got nil")
		}
		if !strings.Contains(err.Error(), "file not found") {
			t.Errorf("unexpected error message: %v", err)
		}
	})
}
