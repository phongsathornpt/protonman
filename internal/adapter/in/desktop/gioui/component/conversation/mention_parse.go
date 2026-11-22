//go:build desktop || desktop_gio

package conversation

import (
	"path/filepath"
	"sort"
	"strings"
	"unicode"
)

// Mention suggestion parsing and ranking. The composer owns the presentation;
// this is the pure text analysis behind the suggestion list, kept here so it can
// be tested and reasoned about without a shell.

// MentionContext contains details about the active mention being typed.
type MentionContext struct {
	Query       string
	Lead        string
	StartOffset int
	EndOffset   int
}

// parseMentionContext examines the text up to the caret to determine if an active `@` mention is being typed.
func ParseMentionContext(textBeforeCursor string) (MentionContext, bool) {
	runes := []rune(textBeforeCursor)
	if len(runes) == 0 {
		return MentionContext{}, false
	}

	atIndex := -1
	for idx := len(runes) - 1; idx >= 0; idx-- {
		r := runes[idx]
		if unicode.IsSpace(r) {
			return MentionContext{}, false
		}
		if r == '@' {
			atIndex = idx
			break
		}
	}

	if atIndex < 0 {
		return MentionContext{}, false
	}

	if atIndex > 0 {
		prev := runes[atIndex-1]
		if !unicode.IsSpace(prev) {
			return MentionContext{}, false
		}
	}

	query := string(runes[atIndex+1:])
	if strings.ContainsAny(query, " \t\r\n\"'`;()[]{}") {
		return MentionContext{}, false
	}

	return MentionContext{
		Query:       query,
		Lead:        "@",
		StartOffset: atIndex,
		EndOffset:   len(runes),
	}, true
}

// defaultMentionAgents returns the canonical agent profile suggestions.
func DefaultMentionAgents() []MentionItem {
	specs := []struct {
		name        string
		description string
	}{
		{"strength", "substantial implementation, fixes, and refactors"},
		{"agility", "fast read-only exploration and tracing"},
		{"intelligence", "deep reasoning, architecture, high-risk engineering"},
		{"universal", "adaptive primary software engineering orchestrator"},
	}

	items := make([]MentionItem, 0, len(specs))
	for _, spec := range specs {
		items = append(items, MentionItem{
			Kind:        MentionItemKindAgent,
			Name:        spec.name,
			Description: spec.description,
			PrefixTag:   "[agent]",
		})
	}
	return items
}

// fuzzyContains returns true if query characters appear in target in sequence.
func fuzzyContains(target, query string) bool {
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

type scoredMentionItem struct {
	item  MentionItem
	score int
}

func scoreMentionMatch(name, query string) int {
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
	if fuzzyContains(base, queryLower) {
		return 200
	}
	if fuzzyContains(nameLower, queryLower) {
		return 100
	}
	return 0
}

const maxMentionResults = 30

// matchMentionItems filters and ranks agent and workspace items according to the mention context.
func MatchMentionItems(context MentionContext, agents []MentionItem, workspaceItems []MentionItem) []MentionItem {
	query := context.Query

	var agentMatches []MentionItem
	for _, agent := range agents {
		if query == "" || fuzzyContains(agent.Name, query) {
			agentMatches = append(agentMatches, agent)
		}
	}

	var scoredFiles []scoredMentionItem
	for _, item := range workspaceItems {
		score := scoreMentionMatch(item.Name, query)
		if score == 0 && query != "" && !fuzzyContains(item.Description, query) {
			continue
		}
		if score == 0 {
			score = 50
		}
		scoredFiles = append(scoredFiles, scoredMentionItem{item: item, score: score})
	}

	sort.SliceStable(scoredFiles, func(i, j int) bool {
		if scoredFiles[i].score != scoredFiles[j].score {
			return scoredFiles[i].score > scoredFiles[j].score
		}
		return scoredFiles[i].item.Name < scoredFiles[j].item.Name
	})

	results := make([]MentionItem, 0, len(agentMatches)+len(scoredFiles))
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
