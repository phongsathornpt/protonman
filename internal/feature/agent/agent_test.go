package agent

import (
	"testing"
)

func TestProfile(t *testing.T) {
	tests := []struct {
		raw        string
		wantProf   Profile
		wantErr    bool
		isMutating bool
	}{
		{"universal", ProfileUniversal, false, true},
		{"strength", ProfileStrength, false, true},
		{"agility", ProfileAgility, false, false},
		{"intelligence", ProfileIntelligence, false, true},
		{"  STRENGTH  ", ProfileStrength, false, true},
		{"  Agility  ", ProfileAgility, false, false},
		{"  INTELLIGENCE  ", ProfileIntelligence, false, true},
		{"pow", "", true, false},
		{"dex", "", true, false},
		{"int", "", true, false},
		{"worker", "", true, false},
		{"explorer", "", true, false},
		{"reviewer", "", true, false},
		{"invalid", "", true, false},
		{"", "", true, false},
	}

	for _, tt := range tests {
		t.Run(tt.raw, func(t *testing.T) {
			got, err := ParseProfile(tt.raw)
			if (err != nil) != tt.wantErr {
				t.Fatalf("ParseProfile(%q) error = %v, wantErr %v", tt.raw, err, tt.wantErr)
			}
			if !tt.wantErr {
				if got != tt.wantProf {
					t.Errorf("ParseProfile(%q) = %q, want %q", tt.raw, got, tt.wantProf)
				}
				if !got.Valid() {
					t.Errorf("profile %q should be valid", got)
				}
				if got.IsMutating() != tt.isMutating {
					t.Errorf("profile %q IsMutating() = %v, want %v", got, got.IsMutating(), tt.isMutating)
				}
			}
		})
	}

	invalid := Profile("unknown")
	if invalid.Valid() {
		t.Error("unknown profile should not be valid")
	}
	if invalid.IsMutating() {
		t.Error("unknown profile should not be mutating")
	}
}

func TestUniversalCannotBeDelegated(t *testing.T) {
	if _, err := ParseSubagentProfile("universal"); err == nil {
		t.Fatal("universal should not be a delegated subagent profile")
	}
	for _, name := range []string{"strength", "agility", "intelligence"} {
		if _, err := ParseSubagentProfile(name); err != nil {
			t.Fatalf("ParseSubagentProfile(%q) error = %v", name, err)
		}
	}
}
