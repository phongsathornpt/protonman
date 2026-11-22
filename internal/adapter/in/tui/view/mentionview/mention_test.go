package mentionview

import (
	"testing"
)

func TestParseContext(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		wantOK    bool
		wantQuery string
		wantStart int
		wantEnd   int
	}{
		{name: "empty", input: "", wantOK: false},
		{name: "bare at", input: "@", wantOK: true, wantQuery: "", wantStart: 0, wantEnd: 1},
		{name: "prefix query", input: "@str", wantOK: true, wantQuery: "str", wantStart: 0, wantEnd: 4},
		{name: "mid sentence mention", input: "please check @main.go", wantOK: true, wantQuery: "main.go", wantStart: 13, wantEnd: 21},
		{name: "email address ignored", input: "contact user@example.com", wantOK: false},
		{name: "mention followed by space", input: "check @main.go now", wantOK: false},
		{name: "newline before mention", input: "line 1\n@agility", wantOK: true, wantQuery: "agility", wantStart: 7, wantEnd: 15},
		{name: "directory path query", input: "look at @internal/adapter/", wantOK: true, wantQuery: "internal/adapter/", wantStart: 8, wantEnd: 26},
		{name: "tab before mention", input: "code:\t@strength", wantOK: true, wantQuery: "strength", wantStart: 6, wantEnd: 15},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, ok := ParseContext(tt.input)
			if ok != tt.wantOK {
				t.Fatalf("ParseContext(%q) ok = %v, want %v", tt.input, ok, tt.wantOK)
			}
			if !ok {
				return
			}
			if ctx.Query != tt.wantQuery {
				t.Errorf("Query = %q, want %q", ctx.Query, tt.wantQuery)
			}
			if ctx.StartOffset != tt.wantStart {
				t.Errorf("StartOffset = %d, want %d", ctx.StartOffset, tt.wantStart)
			}
			if ctx.EndOffset != tt.wantEnd {
				t.Errorf("EndOffset = %d, want %d", ctx.EndOffset, tt.wantEnd)
			}
		})
	}
}

func TestMatchesRanking(t *testing.T) {
	agents := DefaultAgents()
	files := []Item{
		{Kind: ItemKindFile, Name: "cmd/protonman/main.go", Description: "Go source"},
		{Kind: ItemKindFile, Name: "internal/adapter/in/tui/composer.go", Description: "Go source"},
		{Kind: ItemKindDir, Name: "internal/adapter", Description: "Directory"},
		{Kind: ItemKindFile, Name: "README.md", Description: "Markdown"},
	}

	// Empty query: agents pinned at top, then files
	ctxEmpty, _ := ParseContext("@")
	resultsEmpty := Matches(ctxEmpty, agents, files)
	if len(resultsEmpty) != len(agents)+len(files) {
		t.Fatalf("Matches(@) returned %d items, want %d", len(resultsEmpty), len(agents)+len(files))
	}
	if resultsEmpty[0].Kind != ItemKindAgent {
		t.Errorf("expected first item to be agent, got %v", resultsEmpty[0].Kind)
	}

	// Query "str": agent "strength" must be first
	ctxStr, _ := ParseContext("@str")
	resultsStr := Matches(ctxStr, agents, files)
	if len(resultsStr) == 0 || resultsStr[0].Name != "strength" {
		t.Fatalf("Matches(@str) top item = %v, want strength", resultsStr)
	}

	// Query "main": cmd/protonman/main.go should match
	ctxMain, _ := ParseContext("@main")
	resultsMain := Matches(ctxMain, agents, files)
	if len(resultsMain) == 0 {
		t.Fatalf("Matches(@main) returned 0 items")
	}
	if resultsMain[0].Name != "cmd/protonman/main.go" {
		t.Errorf("Matches(@main) top item = %q, want cmd/protonman/main.go", resultsMain[0].Name)
	}

	// Insertion text checks
	fileItem := Item{Kind: ItemKindFile, Name: "main.go"}
	if got := fileItem.InsertionText(); got != "@main.go " {
		t.Errorf("file insertion text = %q, want @main.go ", got)
	}
	dirItem := Item{Kind: ItemKindDir, Name: "internal/adapter"}
	if got := dirItem.InsertionText(); got != "@internal/adapter/" {
		t.Errorf("dir insertion text = %q, want @internal/adapter/", got)
	}
}
