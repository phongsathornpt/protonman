package todotool

import (
	"encoding/json"
	"testing"
)

func TestNormalizeArgumentsCanonicalizesQuotedRevision(t *testing.T) {
	normalized := (taskArgumentNormalizer{}).NormalizeArguments(
		json.RawMessage(`{"action":"update","expectedRevision":"7","operations":[{"op":"remove","id":"a"}]}`),
	)
	var payload map[string]any
	if err := json.Unmarshal(normalized, &payload); err != nil {
		t.Fatalf("normalized payload is not JSON: %v (%s)", err, normalized)
	}
	if got, ok := payload["expectedRevision"].(float64); !ok || got != 7 {
		t.Fatalf("expectedRevision = %#v, want numeric 7", payload["expectedRevision"])
	}
}

func TestNormalizeArgumentsLeavesCanonicalPayloadAlone(t *testing.T) {
	original := `{"action":"update","expectedRevision":3,"operations":[{"op":"remove","id":"a"}]}`
	if normalized := (taskArgumentNormalizer{}).NormalizeArguments(json.RawMessage(original)); normalized != nil {
		t.Fatalf("normalizer rewrote canonical payload: %s", normalized)
	}
}

func TestNormalizeArgumentsCanonicalizesUppercaseEnums(t *testing.T) {
	normalized := (taskArgumentNormalizer{}).NormalizeArguments(
		json.RawMessage(`{"action":"update","expectedRevision":0,"operations":[{"op":"ADD","id":"a","text":"t","status":"PENDING"}]}`),
	)
	var payload struct {
		Operations []struct {
			Op     string `json:"op"`
			Status string `json:"status"`
		} `json:"operations"`
	}
	if err := json.Unmarshal(normalized, &payload); err != nil {
		t.Fatalf("normalized payload is not JSON: %v (%s)", err, normalized)
	}
	if len(payload.Operations) != 1 || payload.Operations[0].Op != "add" || payload.Operations[0].Status != "pending" {
		t.Fatalf("operations = %#v, want canonical add/pending", payload.Operations)
	}
}

func TestNormalizeArgumentsPreservesGenuineTypeErrors(t *testing.T) {
	cases := map[string]string{
		"non-numeric revision": `{"action":"update","expectedRevision":"abc","operations":[{"op":"remove","id":"a"}]}`,
		"negative revision":    `{"action":"update","expectedRevision":"-1","operations":[{"op":"remove","id":"a"}]}`,
		"unknown op":           `{"action":"update","expectedRevision":0,"operations":[{"op":"nope","id":"a"}]}`,
		"unknown status":       `{"action":"update","expectedRevision":0,"operations":[{"op":"add","id":"a","text":"t","status":"nope"}]}`,
	}
	for name, args := range cases {
		if normalized := (taskArgumentNormalizer{}).NormalizeArguments(json.RawMessage(args)); normalized != nil {
			t.Fatalf("%s was silently repaired: %s", name, normalized)
		}
	}
}

func TestNormalizeArgumentsRejectsMalformedJSON(t *testing.T) {
	for _, args := range []string{``, `null`, `[]`, `"str"`, `{`} {
		if normalized := (taskArgumentNormalizer{}).NormalizeArguments(json.RawMessage(args)); normalized != nil {
			t.Fatalf("malformed %q was normalized to %s", args, normalized)
		}
	}
}

func TestCanonicalRevisionRejectsNonIntegers(t *testing.T) {
	for _, raw := range []string{`"1.5"`, `"1e3"`, `" 7 "`, `"-2"`, `"0x10"`} {
		got, ok := canonicalRevision(json.RawMessage(raw))
		if raw == `" 7 "` {
			if !ok || string(got) != "7" {
				t.Fatalf("padded revision %s = %s (ok=%v), want trimmed 7", raw, got, ok)
			}
			continue
		}
		if ok {
			t.Fatalf("canonicalRevision(%s) = %s, want rejection", raw, got)
		}
	}
}
