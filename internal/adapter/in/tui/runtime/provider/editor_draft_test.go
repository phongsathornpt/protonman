package provider

import (
	"testing"

	"github.com/phongsathornpt/protonman/internal/adapter/out/model"
)

func TestNewEditorDraftFromPreset(t *testing.T) {
	draft := NewEditorDraft(model.DefaultAnthropicName)
	if draft.PresetID != model.DefaultAnthropicName || draft.Endpoint == "" {
		t.Fatalf("anthropic draft = %#v", draft)
	}
	if draft.FocusField != FieldAPIKey {
		t.Fatalf("focus field = %v, want API key", draft.FocusField)
	}
}

func TestNewEditorDraftFreeAlias(t *testing.T) {
	draft := NewEditorDraft("free")
	if draft.PresetID != model.DefaultOpenCodeName || !draft.FilterFreeOnly {
		t.Fatalf("free draft = %#v", draft)
	}
}
