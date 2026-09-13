package memory

import (
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/phongsathornpt/protonman/internal/core/session"
)

const maxTranscriptMessageBytes = 12 * 1024

func buildExtractionTranscript(sessionID string, state session.State, maxBytes int) string {
	if maxBytes <= 0 || len(state.Messages) == 0 {
		return ""
	}
	chunks := make([]string, 0, len(state.Messages))
	for _, message := range state.Messages {
		content := strings.TrimSpace(redactSecrets(message.Content))
		if content == "" {
			continue
		}
		content = truncateUTF8Text(content, maxTranscriptMessageBytes)
		id := strings.TrimSpace(message.ID)
		if id == "" {
			id = "unknown"
		}
		chunks = append(chunks, fmt.Sprintf("[%s] %s: %s", id, message.Role, content))
	}
	if len(chunks) == 0 {
		return ""
	}
	header := fmt.Sprintf("Historical session %s (revision %d). Extract durable memory only from the evidence below.\n", strings.TrimSpace(sessionID), state.Revision)
	if len(header) >= maxBytes {
		return truncateUTF8Text(header, maxBytes)
	}
	remaining := maxBytes - len(header)
	selected := make([]string, 0, len(chunks))
	// Preserve the initial intent and then spend the remaining budget on the most
	// recent evidence, where corrections and verified outcomes usually live.
	first := chunks[0]
	if len(first)+1 <= remaining {
		selected = append(selected, first)
		remaining -= len(first) + 1
	}
	recent := make([]string, 0, len(chunks)-1)
	for index := len(chunks) - 1; index >= 1; index-- {
		chunk := chunks[index]
		if len(chunk)+1 > remaining {
			continue
		}
		recent = append(recent, chunk)
		remaining -= len(chunk) + 1
	}
	for left, right := 0, len(recent)-1; left < right; left, right = left+1, right-1 {
		recent[left], recent[right] = recent[right], recent[left]
	}
	selected = append(selected, recent...)
	if len(selected) == 0 {
		return ""
	}
	return header + strings.Join(selected, "\n")
}

func truncateUTF8Text(value string, limit int) string {
	if limit <= 0 {
		return ""
	}
	if len(value) <= limit {
		return value
	}
	value = value[:limit]
	for len(value) > 0 && !utf8.ValidString(value) {
		value = value[:len(value)-1]
	}
	return value
}
