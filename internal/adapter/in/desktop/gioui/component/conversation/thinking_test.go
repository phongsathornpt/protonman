//go:build desktop || desktop_gio

package conversation_test

import (
	"testing"

	"github.com/phongsathornpt/protonman/internal/adapter/in/desktop/gioui/component/conversation"
)

func TestParseAssistantThinking(t *testing.T) {
	tests := []struct {
		name         string
		input        string
		wantThinking bool
		wantDone     bool
		wantThink    string
		wantResp     string
	}{
		{
			name:         "plain text without thinking",
			input:        "Hello, how can I help you?",
			wantThinking: false,
			wantDone:     false,
			wantThink:    "",
			wantResp:     "Hello, how can I help you?",
		},
		{
			name:         "complete thinking block",
			input:        "<think>\nNeed to check the repo structure.\n</think>\nHere is the answer.",
			wantThinking: true,
			wantDone:     true,
			wantThink:    "Need to check the repo structure.",
			wantResp:     "Here is the answer.",
		},
		{
			name:         "streaming unclosed thinking",
			input:        "<think>\nStill reasoning about this difficult problem...",
			wantThinking: true,
			wantDone:     false,
			wantThink:    "Still reasoning about this difficult problem...",
			wantResp:     "",
		},
		{
			name:         "thinking with empty response",
			input:        "<think>Done reasoning</think>",
			wantThinking: true,
			wantDone:     true,
			wantThink:    "Done reasoning",
			wantResp:     "",
		},
		{
			name:         "text before and after thinking",
			input:        "Prefix<think>Middle thought</think>Suffix",
			wantThinking: true,
			wantDone:     true,
			wantThink:    "Middle thought",
			wantResp:     "Prefix\n\nSuffix",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := conversation.ParseAssistantThinking(tc.input)
			if got.HasThinking != tc.wantThinking {
				t.Errorf("HasThinking = %v, want %v", got.HasThinking, tc.wantThinking)
			}
			if got.ThinkingDone != tc.wantDone {
				t.Errorf("ThinkingDone = %v, want %v", got.ThinkingDone, tc.wantDone)
			}
			if got.ThinkingText != tc.wantThink {
				t.Errorf("ThinkingText = %q, want %q", got.ThinkingText, tc.wantThink)
			}
			if got.ResponseText != tc.wantResp {
				t.Errorf("ResponseText = %q, want %q", got.ResponseText, tc.wantResp)
			}
		})
	}
}
