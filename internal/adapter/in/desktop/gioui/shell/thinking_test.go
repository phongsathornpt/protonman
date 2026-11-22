//go:build desktop || desktop_gio

package shell

import (
	"testing"
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
			got := parseAssistantThinking(tc.input)
			if got.hasThinking != tc.wantThinking {
				t.Errorf("hasThinking = %v, want %v", got.hasThinking, tc.wantThinking)
			}
			if got.thinkingDone != tc.wantDone {
				t.Errorf("thinkingDone = %v, want %v", got.thinkingDone, tc.wantDone)
			}
			if got.thinkingText != tc.wantThink {
				t.Errorf("thinkingText = %q, want %q", got.thinkingText, tc.wantThink)
			}
			if got.responseText != tc.wantResp {
				t.Errorf("responseText = %q, want %q", got.responseText, tc.wantResp)
			}
		})
	}
}
