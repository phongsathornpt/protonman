package providerutil

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestMarshalWithOptions(t *testing.T) {
	got, err := MarshalWithOptions(struct {
		Model string `json:"model"`
	}{Model: "m"}, json.RawMessage(`{"reasoning_effort":"high"}`), "model")
	if err != nil || !strings.Contains(string(got), `"reasoning_effort":"high"`) {
		t.Fatalf("got=%s err=%v", got, err)
	}
	if _, err := MarshalWithOptions(struct {
		Model string `json:"model"`
	}{Model: "m"}, json.RawMessage(`{"model":"other"}`), "model"); err == nil {
		t.Fatal("expected protected field error")
	}
}
