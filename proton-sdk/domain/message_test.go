package domain_test

import (
	"encoding/json"
	"testing"

	"github.com/phongsathornpt/protonman/proton-sdk/domain"
)

func TestParseReasoningEffort(t *testing.T) {
	tests := []struct {
		input   string
		want    domain.ReasoningEffort
		wantErr bool
	}{
		{"", domain.ReasoningDefault, false},
		{"auto", domain.ReasoningDefault, false},
		{"default", domain.ReasoningDefault, false},
		{"none", domain.ReasoningNone, false},
		{"minimal", domain.ReasoningMinimal, false},
		{"low", domain.ReasoningLow, false},
		{"medium", domain.ReasoningMedium, false},
		{"high", domain.ReasoningHigh, false},
		{"xhigh", domain.ReasoningXHigh, false},
		{"max", domain.ReasoningMax, false},
		{"invalid", "", true},
	}
	for _, tc := range tests {
		got, err := domain.ParseReasoningEffort(tc.input)
		if (err != nil) != tc.wantErr {
			t.Errorf("ParseReasoningEffort(%q) error = %v, wantErr %v", tc.input, err, tc.wantErr)
		}
		if got != tc.want {
			t.Errorf("ParseReasoningEffort(%q) = %v, want %v", tc.input, got, tc.want)
		}
	}
}

func TestMessageIDs(t *testing.T) {
	id1 := domain.NewMessageID()
	id2 := domain.NewMessageID()
	if id1 == "" || id2 == "" || id1 == id2 {
		t.Fatalf("expected unique message IDs, got %q and %q", id1, id2)
	}
	if !domain.ValidMessageID(id1) {
		t.Fatalf("id %q is not valid", id1)
	}
	if domain.ValidMessageID("invalid_id_without_prefix") {
		t.Fatal("expected invalid message id")
	}

	msgs := []domain.Message{
		{Content: "no id"},
		{ID: id1, Content: "has id"},
	}
	ensured := domain.EnsureMessageIDs(msgs)
	if ensured[0].ID == "" || ensured[1].ID != id1 {
		t.Fatalf("unexpected IDs after EnsureMessageIDs: %+v", ensured)
	}
	// Calling EnsureMessageIDs on already IDed slice
	ensured2 := domain.EnsureMessageIDs(ensured)
	if ensured2[0].ID != ensured[0].ID {
		t.Fatal("re-ensuring changed ID")
	}

	var empty []domain.Message
	if domain.EnsureMessageIDs(empty) != nil {
		t.Fatal("empty EnsureMessageIDs should return nil")
	}
	if domain.CloneMessages(empty) != nil {
		t.Fatal("empty CloneMessages should return nil")
	}
}

func TestMessageTextContent(t *testing.T) {
	m1 := domain.Message{Content: "direct content"}
	if m1.TextContent() != "direct content" {
		t.Fatalf("unexpected: %q", m1.TextContent())
	}

	m2 := domain.Message{
		Parts: []domain.ContentPart{
			{Type: domain.ContentPartText, Text: "part 1"},
			{Type: domain.ContentPartImage, Data: "binary"},
			{Type: domain.ContentPartText, Text: "part 2"},
		},
	}
	if m2.TextContent() != "part 1\npart 2" {
		t.Fatalf("unexpected parts text: %q", m2.TextContent())
	}
}

func TestMessageValidation(t *testing.T) {
	valid := domain.Message{Role: domain.RoleUser, Content: "hello"}
	if err := valid.Validate(); err != nil {
		t.Fatalf("valid message failed: %v", err)
	}

	invalidRole := domain.Message{Role: "unknown", Content: "hello"}
	if err := invalidRole.Validate(); err == nil {
		t.Fatal("expected error for unknown role")
	}

	invalidID := domain.Message{ID: "bad", Role: domain.RoleUser}
	if err := invalidID.Validate(); err == nil {
		t.Fatal("expected error for bad ID")
	}

	toolNoID := domain.Message{Role: domain.RoleTool, ToolName: "read"}
	if err := toolNoID.Validate(); err == nil {
		t.Fatal("expected error for tool role without call ID")
	}
}

func TestCloneMessagesCopiesToolCalls(t *testing.T) {
	msgs := []domain.Message{
		{
			Role: domain.RoleAssistant,
			ToolCalls: []domain.ToolCall{
				{ID: "c1", Name: "read", Arguments: json.RawMessage(`{"path":"a"}`)},
			},
		},
	}
	cloned := domain.CloneMessages(msgs)
	cloned[0].ToolCalls[0].Arguments[0] = '['
	if string(msgs[0].ToolCalls[0].Arguments) != `{"path":"a"}` {
		t.Fatal("original arguments mutated")
	}
}
