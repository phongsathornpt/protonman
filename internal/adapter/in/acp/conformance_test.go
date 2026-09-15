package acp

import (
	"encoding/json"
	"testing"
)

func TestStableV1CoverageHasUniqueFeatures(t *testing.T) {
	seen := make(map[string]struct{}, len(StableV1Coverage))
	for _, feature := range StableV1Coverage {
		if feature.Name == "" {
			t.Fatal("ACP conformance feature name must not be empty")
		}
		if _, ok := seen[feature.Name]; ok {
			t.Fatalf("duplicate ACP conformance feature %q", feature.Name)
		}
		seen[feature.Name] = struct{}{}
		if feature.Advertised && !feature.Supported {
			t.Fatalf("ACP feature %q is advertised without implementation", feature.Name)
		}
	}
}

func TestMetaCarrierRoundTripPreservesRawJSON(t *testing.T) {
	type message struct {
		MetaCarrier
		SessionID string `json:"sessionId"`
	}

	input := []byte(`{"sessionId":"s1","_meta":{"protonman":{"workspace":"demo"},"future":{"enabled":true}}}`)
	var got message
	if err := json.Unmarshal(input, &got); err != nil {
		t.Fatalf("unmarshal ACP metadata: %v", err)
	}
	if got.SessionID != "s1" {
		t.Fatalf("session id = %q, want s1", got.SessionID)
	}
	if len(got.Meta) != 2 {
		t.Fatalf("metadata entries = %d, want 2", len(got.Meta))
	}

	encoded, err := json.Marshal(got)
	if err != nil {
		t.Fatalf("marshal ACP metadata: %v", err)
	}
	var roundTrip map[string]json.RawMessage
	if err := json.Unmarshal(encoded, &roundTrip); err != nil {
		t.Fatalf("decode round-trip JSON: %v", err)
	}
	if _, ok := roundTrip["_meta"]; !ok {
		t.Fatalf("round-trip JSON lost _meta: %s", encoded)
	}
}
