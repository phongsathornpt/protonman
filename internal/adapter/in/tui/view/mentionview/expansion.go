package mentionview

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"unicode"

	"github.com/phongsathornpt/protonman/internal/core/agentprofile"
)

const (
	maxMentionInlinedLines = 500
	maxMentionInlinedBytes = 50 * 1024 // 50 KB
	maxDirectoryListItems  = 100
)

// ExpandedPrompt holds the processed prompt for both presentation and execution.
type ExpandedPrompt struct {
	DisplayText     string
	TurnPrompt      string
	ImagePaths      []string
	MentionedAgents []string
	MentionedFiles  []string
}

// mentionToken represents an extracted @token from user input.
type mentionToken struct {
	raw         string // e.g. "@main.go"
	token       string // e.g. "main.go"
	startOffset int
	endOffset   int
}

// ExtractMentionTokens scans the text for @token occurrences.
func ExtractMentionTokens(text string) []mentionToken {
	runes := []rune(text)
	var tokens []mentionToken

	idx := 0
	for idx < len(runes) {
		if runes[idx] != '@' {
			idx++
			continue
		}

		// Must be start of string or preceded by whitespace
		if idx > 0 && !unicode.IsSpace(runes[idx-1]) {
			idx++
			continue
		}

		atIdx := idx
		idx++ // skip '@'

		tokenStart := idx
		for idx < len(runes) && !unicode.IsSpace(runes[idx]) && !strings.ContainsRune("\"'`;()[]{}", runes[idx]) {
			idx++
		}

		if idx == tokenStart {
			// Bare '@' with nothing following
			continue
		}

		rawToken := string(runes[tokenStart:idx])
		// Trim common trailing punctuation from the token (e.g. commas, periods)
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

func isAgentMention(token string) (agentprofile.Profile, bool) {
	canonical := strings.ToLower(strings.TrimPrefix(token, "agent:"))
	for _, p := range agentprofile.SupportedProfiles() {
		if string(p) == canonical {
			return p, true
		}
	}
	return "", false
}

// IsImageFile returns true if the path has a supported image extension.
func IsImageFile(path string) bool {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".png", ".jpg", ".jpeg", ".gif", ".webp":
		return true
	default:
		return false
	}
}

// ExpandMentions processes all @file and @agent mentions in the prompt text.
// Returns an error if a mentioned file does not exist.
func ExpandMentions(text string, workDir string) (ExpandedPrompt, error) {
	tokens := ExtractMentionTokens(text)
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
	// We replace tokens in displayText from right to left to preserve offsets
	type replacement struct {
		start int
		end   int
		text  string
	}
	var displayReplacements []replacement

	for _, m := range tokens {
		if profile, ok := isAgentMention(m.token); ok {
			mentionedAgents = append(mentionedAgents, string(profile))
			continue
		}

		// File or directory mention
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

		if IsImageFile(fullPath) {
			imagePaths = append(imagePaths, fullPath)
			continue
		}

		// Regular text/code file
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

	// Apply display text replacements in reverse order
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

	// Build TurnPrompt
	var turnPromptParts []string

	// Agent directive at the top if any agents were mentioned
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
	// Allow scanning lines up to 256KB
	buf := make([]byte, 64*1024)
	scanner.Buffer(buf, 256*1024)

	totalLines := 0
	totalBytes := 0
	truncated := false

	for scanner.Scan() {
		totalLines++
		line := scanner.Text()
		lineBytes := len(line) + 1 // including \n

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
