package mentionview

import (
	"path/filepath"
	"sort"
	"strings"
	"unicode"

	"github.com/phongsathornpt/protonman/internal/core/agentprofile"
)

// ItemKind represents the type of item in a mention list.
type ItemKind uint8

const (
	ItemKindAgent ItemKind = iota
	ItemKindFile
	ItemKindDir
)

// Item represents a single selectable mention suggestion.
type Item struct {
	Kind        ItemKind
	Name        string
	Description string
	PrefixTag   string
	LineCount   int
	SizeBytes   int64
}

// Title returns the display title for the suggestion item.
func (i Item) Title() string {
	prefix := "@"
	if i.PrefixTag != "" {
		prefix = i.PrefixTag + " @"
	}
	if i.Kind == ItemKindDir && !strings.HasSuffix(i.Name, "/") {
		return prefix + i.Name + "/"
	}
	return prefix + i.Name
}

// DisplayDescription returns the human-readable description or metadata.
func (i Item) DisplayDescription() string {
	return i.Description
}

// FilterValue returns the search string used for filtering.
func (i Item) FilterValue() string {
	return i.Name + " " + i.Description
}

// InsertionText returns the text to insert into the composer when selected.
func (i Item) InsertionText() string {
	if i.Kind == ItemKindDir {
		if strings.HasSuffix(i.Name, "/") {
			return "@" + i.Name
		}
		return "@" + i.Name + "/"
	}
	return "@" + i.Name + " "
}

// Context contains details about the active mention being typed.
type Context struct {
	Query       string
	Lead        string
	StartOffset int
	EndOffset   int
}

// ParseContext examines the text up to the cursor to determine if an active `@` mention is being typed.
func ParseContext(textBeforeCursor string) (Context, bool) {
	runes := []rune(textBeforeCursor)
	if len(runes) == 0 {
		return Context{}, false
	}

	// Search backwards for '@'
	atIndex := -1
	for idx := len(runes) - 1; idx >= 0; idx-- {
		r := runes[idx]
		if unicode.IsSpace(r) {
			// Found whitespace before finding '@', meaning cursor is past any mention token
			return Context{}, false
		}
		if r == '@' {
			atIndex = idx
			break
		}
	}

	if atIndex < 0 {
		return Context{}, false
	}

	// '@' must be at the very start of the text or preceded by whitespace
	if atIndex > 0 {
		prev := runes[atIndex-1]
		if !unicode.IsSpace(prev) {
			// Preceded by a non-whitespace character (e.g. user@example.com)
			return Context{}, false
		}
	}

	query := string(runes[atIndex+1:])
	// Mention queries cannot contain spaces, quotes, or control characters
	if strings.ContainsAny(query, " \t\r\n\"'`;()[]{}") {
		return Context{}, false
	}

	return Context{
		Query:       query,
		Lead:        "@",
		StartOffset: atIndex,
		EndOffset:   len(runes),
	}, true
}

// DefaultAgents returns the canonical agent profile suggestions.
func DefaultAgents() []Item {
	specs := []struct {
		profile     agentprofile.Profile
		description string
	}{
		{agentprofile.ProfileStrength, "substantial implementation, fixes, and refactors"},
		{agentprofile.ProfileAgility, "fast read-only exploration and tracing"},
		{agentprofile.ProfileIntelligence, "deep reasoning, architecture, high-risk engineering"},
		{agentprofile.ProfileUniversal, "adaptive primary software engineering orchestrator"},
	}

	items := make([]Item, 0, len(specs))
	for _, spec := range specs {
		items = append(items, Item{
			Kind:        ItemKindAgent,
			Name:        string(spec.profile),
			Description: spec.description,
			PrefixTag:   "[agent]",
		})
	}
	return items
}

// FuzzyContains returns true if query characters appear in target in sequence.
func FuzzyContains(target, query string) bool {
	target = strings.ToLower(target)
	query = strings.ToLower(query)
	if strings.Contains(target, query) {
		return true
	}
	targetRunes := []rune(target)
	ti := 0
	for _, q := range query {
		found := false
		for ti < len(targetRunes) {
			if targetRunes[ti] == q {
				ti++
				found = true
				break
			}
			ti++
		}
		if !found {
			return false
		}
	}
	return true
}

type scoredItem struct {
	item  Item
	score int
}

func scoreMatch(name, query string) int {
	nameLower := strings.ToLower(name)
	queryLower := strings.ToLower(query)
	if queryLower == "" {
		return 100
	}
	if nameLower == queryLower {
		return 1000
	}
	base := filepath.Base(nameLower)
	if base == queryLower {
		return 900
	}
	if strings.HasPrefix(base, queryLower) {
		return 800
	}
	if strings.HasPrefix(nameLower, queryLower) {
		return 700
	}
	if strings.Contains(base, queryLower) {
		return 500
	}
	if strings.Contains(nameLower, queryLower) {
		return 400
	}
	if FuzzyContains(base, queryLower) {
		return 200
	}
	if FuzzyContains(nameLower, queryLower) {
		return 100
	}
	return 0
}

const maxMentionResults = 50

// Matches filters and ranks agent and workspace items according to the mention context.
func Matches(context Context, agents []Item, workspaceItems []Item) []Item {
	query := context.Query

	var agentMatches []Item
	for _, agent := range agents {
		if query == "" || FuzzyContains(agent.Name, query) {
			agentMatches = append(agentMatches, agent)
		}
	}

	var scoredFiles []scoredItem
	for _, item := range workspaceItems {
		score := scoreMatch(item.Name, query)
		if score == 0 && query != "" && !FuzzyContains(item.Description, query) {
			continue
		}
		if score == 0 {
			score = 50
		}
		scoredFiles = append(scoredFiles, scoredItem{item: item, score: score})
	}

	sort.SliceStable(scoredFiles, func(i, j int) bool {
		if scoredFiles[i].score != scoredFiles[j].score {
			return scoredFiles[i].score > scoredFiles[j].score
		}
		return scoredFiles[i].item.Name < scoredFiles[j].item.Name
	})

	results := make([]Item, 0, len(agentMatches)+len(scoredFiles))
	results = append(results, agentMatches...)

	remainingCap := maxMentionResults - len(results)
	if remainingCap < 0 {
		remainingCap = 0
	}
	limit := min(remainingCap, len(scoredFiles))
	for i := 0; i < limit; i++ {
		results = append(results, scoredFiles[i].item)
	}

	return results
}
