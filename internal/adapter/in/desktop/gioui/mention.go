//go:build desktop || desktop_gio

package gioui

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/phongsathornpt/protonman/internal/base/glob"
)

// MentionItemKind represents the category of an item in the mention suggestion list.
type MentionItemKind uint8

const (
	MentionItemKindAgent MentionItemKind = iota
	MentionItemKindFile
	MentionItemKindDir
)

// MentionItem represents a single selectable mention suggestion.
type MentionItem struct {
	Kind        MentionItemKind
	Name        string
	Description string
	PrefixTag   string
	LineCount   int
	SizeBytes   int64
}

// Title returns the display title for the suggestion item.
func (i MentionItem) Title() string {
	prefix := "@"
	if i.PrefixTag != "" {
		prefix = i.PrefixTag + " @"
	}
	if i.Kind == MentionItemKindDir && !strings.HasSuffix(i.Name, "/") {
		return prefix + i.Name + "/"
	}
	return prefix + i.Name
}

// InsertionText returns the text to insert into the composer when selected.
func (i MentionItem) InsertionText() string {
	if i.Kind == MentionItemKindDir {
		if strings.HasSuffix(i.Name, "/") {
			return "@" + i.Name
		}
		return "@" + i.Name + "/"
	}
	return "@" + i.Name + " "
}

// MentionContext contains details about the active mention being typed.
type MentionContext struct {
	Query       string
	Lead        string
	StartOffset int
	EndOffset   int
}

// parseMentionContext examines the text up to the caret to determine if an active `@` mention is being typed.
func parseMentionContext(textBeforeCursor string) (MentionContext, bool) {
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
func defaultMentionAgents() []MentionItem {
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
func matchMentionItems(context MentionContext, agents []MentionItem, workspaceItems []MentionItem) []MentionItem {
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

const (
	maxMentionInlinedLines = 500
	maxMentionInlinedBytes = 50 * 1024 // 50 KB
	maxDirectoryListItems  = 100
	maxWorkspaceScanItems  = 3000
	maxWorkspaceScanDepth  = 8
	workspaceCacheTTL      = 15 * time.Second
)

type workspaceMentionCache struct {
	mu          sync.Mutex
	workDir     string
	items       []MentionItem
	lastIndexed time.Time
}

func (c *workspaceMentionCache) get(workDir string) []MentionItem {
	if strings.TrimSpace(workDir) == "" {
		return nil
	}
	c.mu.Lock()
	if c.workDir == workDir && time.Since(c.lastIndexed) < workspaceCacheTTL && len(c.items) > 0 {
		items := c.items
		c.mu.Unlock()
		return items
	}
	c.mu.Unlock()

	items := scanWorkspaceFiles(workDir)
	c.mu.Lock()
	c.workDir = workDir
	c.items = items
	c.lastIndexed = time.Now()
	c.mu.Unlock()
	return items
}

func scanWorkspaceFiles(workDir string) []MentionItem {
	if strings.TrimSpace(workDir) == "" {
		return nil
	}

	ignorePatterns := loadGitIgnore(workDir)
	var items []MentionItem

	var walk func(dir string, depth int)
	walk = func(dir string, depth int) {
		if depth > maxWorkspaceScanDepth || len(items) >= maxWorkspaceScanItems {
			return
		}

		entries, err := os.ReadDir(dir)
		if err != nil {
			return
		}

		for _, entry := range entries {
			name := entry.Name()
			if isIgnoredDefault(name) {
				continue
			}

			fullPath := filepath.Join(dir, name)
			relPath, err := filepath.Rel(workDir, fullPath)
			if err != nil {
				continue
			}

			if matchesAnyPattern(relPath, name, ignorePatterns) {
				continue
			}

			if entry.IsDir() {
				items = append(items, MentionItem{
					Kind:        MentionItemKindDir,
					Name:        relPath,
					Description: "Directory",
					PrefixTag:   "[dir]",
				})
				walk(fullPath, depth+1)
			} else {
				info, err := entry.Info()
				sizeStr := ""
				var sz int64
				if err == nil {
					sz = info.Size()
					if sz >= 1024*1024 {
						sizeStr = fmt.Sprintf("%.1f MB", float64(sz)/(1024*1024))
					} else if sz >= 1024 {
						sizeStr = fmt.Sprintf("%d KB", sz/1024)
					} else {
						sizeStr = fmt.Sprintf("%d B", sz)
					}
				}
				items = append(items, MentionItem{
					Kind:        MentionItemKindFile,
					Name:        relPath,
					Description: sizeStr,
					PrefixTag:   "[file]",
					SizeBytes:   sz,
				})
			}

			if len(items) >= maxWorkspaceScanItems {
				return
			}
		}
	}

	walk(workDir, 0)
	return items
}

func isIgnoredDefault(name string) bool {
	switch name {
	case ".git", "node_modules", "target", "bin", "dist", ".idea", ".vscode", ".bak":
		return true
	}
	return strings.HasPrefix(name, ".") && len(name) > 1
}

func loadGitIgnore(workDir string) []string {
	path := filepath.Join(workDir, ".gitignore")
	file, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer func() { _ = file.Close() }()

	var patterns []string
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		patterns = append(patterns, line)
	}
	return patterns
}

func matchesAnyPattern(relPath, baseName string, patterns []string) bool {
	for _, p := range patterns {
		p = strings.TrimPrefix(p, "/")
		p = strings.TrimSuffix(p, "/")
		if glob.Match(p, relPath) || glob.Match(p, baseName) {
			return true
		}
	}
	return false
}

// ExpandedPrompt holds the processed prompt for both presentation and execution.
type ExpandedPrompt struct {
	DisplayText     string
	TurnPrompt      string
	ImagePaths      []string
	MentionedAgents []string
	MentionedFiles  []string
}

type mentionToken struct {
	raw         string
	token       string
	startOffset int
	endOffset   int
}

func extractMentionTokens(text string) []mentionToken {
	runes := []rune(text)
	var tokens []mentionToken

	idx := 0
	for idx < len(runes) {
		if runes[idx] != '@' {
			idx++
			continue
		}

		if idx > 0 && !unicode.IsSpace(runes[idx-1]) {
			idx++
			continue
		}

		atIdx := idx
		idx++

		tokenStart := idx
		for idx < len(runes) && !unicode.IsSpace(runes[idx]) && !strings.ContainsRune("\"'`;()[]{}", runes[idx]) {
			idx++
		}

		if idx == tokenStart {
			continue
		}

		rawToken := string(runes[tokenStart:idx])
		trimmedToken := strings.TrimRight(rawToken, ".,!?:;")
		if trimmedToken == "" {
			continue
		}

		trimmedEnd := tokenStart + len([]rune(trimmedToken))
		tokens = append(tokens, mentionToken{
			raw:         string(runes[atIdx:trimmedEnd]),
			token:       trimmedToken,
			startOffset: atIdx,
			endOffset:   trimmedEnd,
		})
	}

	return tokens
}

func isAgentMention(token string) (string, bool) {
	canonical := strings.ToLower(strings.TrimPrefix(token, "agent:"))
	switch canonical {
	case "universal", "strength", "agility", "intelligence":
		return canonical, true
	default:
		return "", false
	}
}

// isImageFile returns true if the path has a supported image extension.
func isImageFile(path string) bool {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".png", ".jpg", ".jpeg", ".gif", ".webp":
		return true
	default:
		return false
	}
}

// detectImageMIME returns the MIME type corresponding to the image path's extension.
func detectImageMIME(path string) string {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".gif":
		return "image/gif"
	case ".webp":
		return "image/webp"
	default:
		return "image/png"
	}
}

// expandMentions processes all @file and @agent mentions in the prompt text.
// Returns an error if a mentioned file does not exist.
func expandMentions(text string, workDir string) (ExpandedPrompt, error) {
	tokens := extractMentionTokens(text)
	if len(tokens) == 0 {
		return ExpandedPrompt{
			DisplayText: text,
			TurnPrompt:  text,
		}, nil
	}

	var mentionedAgents []string
	var mentionedFiles []string
	var imagePaths []string
	var fileContextBlocks []string

	displayText := text
	type replacement struct {
		start int
		end   int
		text  string
	}
	var displayReplacements []replacement

	for _, m := range tokens {
		if agentName, ok := isAgentMention(m.token); ok {
			mentionedAgents = append(mentionedAgents, agentName)
			continue
		}

		relPath := filepath.Clean(m.token)
		fullPath := relPath
		if !filepath.IsAbs(fullPath) && workDir != "" {
			fullPath = filepath.Join(workDir, fullPath)
		}

		info, err := os.Stat(fullPath)
		if err != nil {
			if os.IsNotExist(err) {
				return ExpandedPrompt{}, fmt.Errorf("file not found: %s", m.token)
			}
			return ExpandedPrompt{}, fmt.Errorf("cannot access %s: %w", m.token, err)
		}

		mentionedFiles = append(mentionedFiles, relPath)

		if info.IsDir() {
			dirBlock, count, err := readDirectoryListing(fullPath, relPath)
			if err != nil {
				return ExpandedPrompt{}, fmt.Errorf("read directory %s: %w", m.token, err)
			}
			fileContextBlocks = append(fileContextBlocks, dirBlock)
			displayReplacements = append(displayReplacements, replacement{
				start: m.startOffset,
				end:   m.endOffset,
				text:  fmt.Sprintf("@%s (%d items)", relPath, count),
			})
			continue
		}

		if isImageFile(fullPath) {
			imagePaths = append(imagePaths, fullPath)
			continue
		}

		fileBlock, lineCount, err := readFileContentBounded(fullPath, relPath)
		if err != nil {
			return ExpandedPrompt{}, fmt.Errorf("read file %s: %w", m.token, err)
		}

		fileContextBlocks = append(fileContextBlocks, fileBlock)
		displayReplacements = append(displayReplacements, replacement{
			start: m.startOffset,
			end:   m.endOffset,
			text:  fmt.Sprintf("@%s (%d lines)", relPath, lineCount),
		})
	}

	runes := []rune(displayText)
	for i := len(displayReplacements) - 1; i >= 0; i-- {
		r := displayReplacements[i]
		if r.start >= 0 && r.end <= len(runes) && r.start <= r.end {
			before := runes[:r.start]
			after := runes[r.end:]
			runes = append(before, append([]rune(r.text), after...)...)
		}
	}
	displayText = string(runes)

	var turnPromptParts []string
	if len(mentionedAgents) > 0 {
		if len(mentionedAgents) == 1 {
			turnPromptParts = append(turnPromptParts, fmt.Sprintf("[Requested Subagent: %s]", mentionedAgents[0]))
		} else {
			turnPromptParts = append(turnPromptParts, fmt.Sprintf("[Requested Subagents: %s]", strings.Join(mentionedAgents, ", ")))
		}
	}

	turnPromptParts = append(turnPromptParts, text)

	if len(fileContextBlocks) > 0 {
		turnPromptParts = append(turnPromptParts, strings.Join(fileContextBlocks, "\n\n"))
	}

	return ExpandedPrompt{
		DisplayText:     displayText,
		TurnPrompt:      strings.Join(turnPromptParts, "\n\n"),
		ImagePaths:      imagePaths,
		MentionedAgents: mentionedAgents,
		MentionedFiles:  mentionedFiles,
	}, nil
}

func readFileContentBounded(absPath, relPath string) (string, int, error) {
	file, err := os.Open(absPath)
	if err != nil {
		return "", 0, err
	}
	defer func() { _ = file.Close() }()

	var lines []string
	scanner := bufio.NewScanner(file)
	buf := make([]byte, 64*1024)
	scanner.Buffer(buf, 256*1024)

	totalLines := 0
	totalBytes := 0
	truncated := false

	for scanner.Scan() {
		totalLines++
		line := scanner.Text()
		lineBytes := len(line) + 1

		if !truncated {
			if len(lines) >= maxMentionInlinedLines || totalBytes+lineBytes > maxMentionInlinedBytes {
				truncated = true
			} else {
				lines = append(lines, line)
				totalBytes += lineBytes
			}
		}
	}

	if err := scanner.Err(); err != nil && err != io.EOF {
		return "", 0, err
	}

	var contentBuilder strings.Builder
	contentBuilder.WriteString(fmt.Sprintf("<file path=\"%s\">\n", relPath))
	contentBuilder.WriteString(strings.Join(lines, "\n"))
	if truncated {
		contentBuilder.WriteString(fmt.Sprintf("\n[... truncated at %d lines; %d total lines. Use the 'read' tool to inspect the rest if needed ...]", len(lines), totalLines))
	}
	contentBuilder.WriteString("\n</file>")

	return contentBuilder.String(), totalLines, nil
}

func readDirectoryListing(absPath, relPath string) (string, int, error) {
	entries, err := os.ReadDir(absPath)
	if err != nil {
		return "", 0, err
	}

	var names []string
	for _, entry := range entries {
		name := entry.Name()
		if strings.HasPrefix(name, ".") {
			continue
		}
		if entry.IsDir() {
			name += "/"
		}
		names = append(names, name)
		if len(names) >= maxDirectoryListItems {
			break
		}
	}

	var builder strings.Builder
	builder.WriteString(fmt.Sprintf("<directory path=\"%s\">\n", relPath))
	builder.WriteString(strings.Join(names, "\n"))
	if len(entries) > maxDirectoryListItems {
		builder.WriteString(fmt.Sprintf("\n[... truncated at %d items; %d total items ...]", maxDirectoryListItems, len(entries)))
	}
	builder.WriteString("\n</directory>")

	return builder.String(), len(names), nil
}
