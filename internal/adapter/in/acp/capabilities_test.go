package acp

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestSessionCapabilitiesSuppressUnsupportedAdditionalDirectories(t *testing.T) {
	payload, err := json.Marshal(SessionCapabilities{
		Resume:                &struct{}{},
		Delete:                &struct{}{},
		Close:                 &struct{}{},
		AdditionalDirectories: &struct{}{},
	})
	if err != nil {
		t.Fatalf("marshal session capabilities: %v", err)
	}
	got := string(payload)
	if strings.Contains(got, "additionalDirectories") {
		t.Fatalf("unsupported additionalDirectories capability advertised: %s", got)
	}
	for _, capability := range []string{"resume", "delete", "close"} {
		if !strings.Contains(got, `"`+capability+`"`) {
			t.Fatalf("supported capability %q missing from %s", capability, got)
		}
	}
}
