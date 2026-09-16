package memory

import (
	"strings"
	"testing"

	"github.com/phongsathornpt/protonman/internal/core/session"
	"github.com/phongsathornpt/protonman/proton-sdk/domain"
)

func TestParseExtractionOutputAcceptsIntroAndFencedJSON(t *testing.T) {
	raw := "Here are the extracted memories:\n```json\n{\"memories\":[{\"scope\":\"workspace\",\"kind\":\"procedure\",\"key\":\"verification command\",\"value\":\"Run go test ./...\",\"confidence\":0.9,\"message_ids\":[\"m1\"]}]}\n```"
	items, err := parseExtractionOutput(raw)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].Key != "verification command" {
		t.Fatalf("items = %+v", items)
	}
}

func TestParseExtractionOutputRejectsTextWithoutJSONObject(t *testing.T) {
	if _, err := parseExtractionOutput("No durable memory found."); err == nil {
		t.Fatal("expected invalid extraction output to fail")
	}
}

func TestBuildExtractionTranscriptRedactsStructuredAndCommonSecrets(t *testing.T) {
	state := session.State{Revision: 1, Messages: []session.Message{{
		ID:      "m1",
		Role:    domain.RoleUser,
		Content: `{"api_key":"secret-value","password": "hunter2", "token":"safe"} AKIA1234567890ABCDEF xoxb-12345678901234567890`,
	}}}
	got := buildExtractionTranscript("session-1", state, 4096)
	if strings.Contains(got, "secret-value") || strings.Contains(got, "hunter2") || strings.Contains(got, "AKIA1234567890ABCDEF") || strings.Contains(got, "xoxb-12345678901234567890") {
		t.Fatalf("transcript leaked secret: %q", got)
	}
	if strings.Count(got, redactedSecret) < 4 {
		t.Fatalf("transcript redactions = %d, want at least 4: %q", strings.Count(got, redactedSecret), got)
	}
}
