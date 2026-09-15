//go:build desktop

package desktop

import (
	"encoding/json"
	"testing"
)

func TestDecodeSessionUpdate(t *testing.T) {
	params := json.RawMessage(`{"sessionId":"s1","update":{"sessionUpdate":"config_option_update","configOptions":[]}}`)
	sessionID, kind, raw, ok := decodeSessionUpdate(params)
	if !ok {
		t.Fatal("expected update to decode")
	}
	if sessionID != "s1" || kind != "config_option_update" {
		t.Fatalf("decoded session=%q kind=%q", sessionID, kind)
	}
	var update configOptionUpdate
	if err := json.Unmarshal(raw, &update); err != nil {
		t.Fatalf("decode config update: %v", err)
	}
}

func TestDecodeSessionUpdateRejectsMissingDiscriminator(t *testing.T) {
	params := json.RawMessage(`{"sessionId":"s1","update":{"title":"Missing kind"}}`)
	if _, _, _, ok := decodeSessionUpdate(params); ok {
		t.Fatal("expected update without sessionUpdate discriminator to be ignored")
	}
}

func TestSessionInfoUpdatePreservesExplicitClear(t *testing.T) {
	var update sessionInfoUpdate
	if err := json.Unmarshal([]byte(`{"sessionUpdate":"session_info_update","title":""}`), &update); err != nil {
		t.Fatal(err)
	}
	if update.Title == nil || *update.Title != "" {
		t.Fatalf("title clear was not preserved: %#v", update.Title)
	}
}

func TestUsageUpdateDecodesCost(t *testing.T) {
	var update usageUpdate
	if err := json.Unmarshal([]byte(`{"sessionUpdate":"usage_update","used":53000,"size":200000,"cost":{"amount":0.045,"currency":"USD"}}`), &update); err != nil {
		t.Fatal(err)
	}
	if update.Used != 53000 || update.Size != 200000 || update.Cost == nil || update.Cost.Currency != "USD" {
		t.Fatalf("unexpected usage update: %#v", update)
	}
}
