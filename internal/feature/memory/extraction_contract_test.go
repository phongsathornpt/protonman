package memory

import "testing"

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
