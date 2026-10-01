//go:build desktop || desktop_gio

package shell

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
	"unicode"

	conversationcomponent "github.com/phongsathornpt/protonman/internal/adapter/in/desktop/gioui/component/conversation"
	"github.com/phongsathornpt/protonman/internal/base/glob"
)

// MentionItem and MentionItemKind live in component/conversation alongside the
// suggestion list they render. The workspace index below produces them, so the
// aliases keep a single definition of the suggestion shape.
type MentionItem = conversationcomponent.MentionItem

type MentionItemKind = conversationcomponent.MentionItemKind

const (
	MentionItemKindAgent = conversationcomponent.MentionItemKindAgent
	MentionItemKindFile  = conversationcomponent.MentionItemKindFile
	MentionItemKindDir   = conversationcomponent.MentionItemKindDir
)

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
	refreshing  bool
}

func (c *workspaceMentionCache) get(workDir string) []MentionItem {
	if strings.TrimSpace(workDir) == "" {
		return nil
	}
	c.mu.Lock()
	if c.workDir == workDir && len(c.items) > 0 {
		items := c.items
		stale := time.Since(c.lastIndexed) >= workspaceCacheTTL
		shouldRefresh := stale && !c.refreshing
		if shouldRefresh {
			c.refreshing = true
		}
		c.mu.Unlock()
		if shouldRefresh {
			go c.refresh(workDir)
		}
		return items
	}
	if c.workDir == workDir && c.refreshing {
		c.mu.Unlock()
		return nil
	}
	c.refreshing = true
	c.mu.Unlock()
	go c.refresh(workDir)
	return nil
}

func (c *workspaceMentionCache) refresh(workDir string) {
	items := scanWorkspaceFiles(workDir)
	c.mu.Lock()
	c.workDir = workDir
	c.items = items
	c.lastIndexed = time.Now()
	c.refreshing = false
	c.mu.Unlock()
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
