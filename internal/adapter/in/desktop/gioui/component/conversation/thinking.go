//go:build desktop || desktop_gio

package conversation

import (
	"strings"
)

type ParsedAssistantMessage struct {
	HasThinking  bool
	ThinkingDone bool
	ThinkingText string
	ResponseText string
}

func ParseAssistantThinking(text string) ParsedAssistantMessage {
	const openTag = "<think>"
	const closeTag = "</think>"

	openIdx := strings.Index(text, openTag)
	if openIdx == -1 {
		return ParsedAssistantMessage{
			HasThinking:  false,
			ResponseText: text,
		}
	}

	beforeThink := text[:openIdx]
	afterOpen := text[openIdx+len(openTag):]

	closeIdx := strings.Index(afterOpen, closeTag)
	if closeIdx == -1 {
		return ParsedAssistantMessage{
			HasThinking:  true,
			ThinkingDone: false,
			ThinkingText: strings.TrimSpace(afterOpen),
			ResponseText: strings.TrimSpace(beforeThink),
		}
	}

	thinking := afterOpen[:closeIdx]
	afterClose := afterOpen[closeIdx+len(closeTag):]

	resp := strings.TrimSpace(beforeThink)
	trimmedAfter := strings.TrimSpace(afterClose)
	if resp != "" && trimmedAfter != "" {
		resp += "\n\n" + trimmedAfter
	} else if resp == "" {
		resp = trimmedAfter
	}

	return ParsedAssistantMessage{
		HasThinking:  true,
		ThinkingDone: true,
		ThinkingText: strings.TrimSpace(thinking),
		ResponseText: resp,
	}
}
