package provider

import (
	"testing"

	panecommon "github.com/phongsathornpt/protonman/internal/adapter/in/tui/view/pane/common"
	tuistyle "github.com/phongsathornpt/protonman/internal/adapter/in/tui/view/style"
)

func TestProviderEditorRowsUseSemanticStateStyles(t *testing.T) {
	tests := []struct {
		name      string
		snapshot  ProviderEditorSnapshot
		wantTone  panecommon.Tone
		wantFirst string
	}{
		{
			name:      "fetching",
			snapshot:  ProviderEditorSnapshot{State: ProviderEditorFetching, Name: "OpenAI"},
			wantTone:  panecommon.ToneAssistant,
			wantFirst: tuistyle.ActivityStyle.Render("Connecting · OpenAI"),
		},
		{
			name:      "saving",
			snapshot:  ProviderEditorSnapshot{State: ProviderEditorSaving, Name: "OpenAI"},
			wantTone:  panecommon.ToneAssistant,
			wantFirst: tuistyle.ActivityStyle.Render("Saving · OpenAI"),
		},
		{
			name:      "input",
			snapshot:  ProviderEditorSnapshot{State: ProviderEditorInput},
			wantTone:  panecommon.ToneAssistant,
			wantFirst: tuistyle.PaneTitleStyle.Render("Add provider"),
		},
		{
			name:      "overwrite warning",
			snapshot:  ProviderEditorSnapshot{State: ProviderEditorConfirmOverwrite, Name: "OpenAI"},
			wantTone:  panecommon.ToneWarning,
			wantFirst: tuistyle.WarningStyle.Render("Provider exists · OpenAI"),
		},
		{
			name:      "connection error",
			snapshot:  ProviderEditorSnapshot{State: ProviderEditorError, ErrorMessage: "boom"},
			wantTone:  panecommon.ToneError,
			wantFirst: tuistyle.ErrorStyle.Render("Connection failed"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rows, tone := ProviderEditorRows(tt.snapshot)
			if tone != tt.wantTone {
				t.Fatalf("tone = %v, want %v", tone, tt.wantTone)
			}
			if len(rows) == 0 || rows[0] != tt.wantFirst {
				t.Fatalf("first row = %q, want %q", rows, tt.wantFirst)
			}
		})
	}
}
