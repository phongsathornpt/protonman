package agentprofile

import (
	"testing"

	"github.com/phongsathornpt/protonman/internal/core/tool"
)

func TestParseProfileCanonicalVocabulary(t *testing.T) {
	tests := []struct {
		raw       string
		want      Profile
		mutating  bool
		label     string
		delegated bool
	}{
		{" universal ", ProfileUniversal, true, "UNI", false},
		{"STRENGTH", ProfileStrength, true, "STR", true},
		{"Agility", ProfileAgility, false, "AGI", true},
		{"intelligence", ProfileIntelligence, true, "INT", true},
	}
	for _, tt := range tests {
		t.Run(tt.raw, func(t *testing.T) {
			got, err := ParseProfile(tt.raw)
			if err != nil {
				t.Fatalf("ParseProfile(%q) error = %v", tt.raw, err)
			}
			if got != tt.want {
				t.Fatalf("ParseProfile(%q) = %q, want %q", tt.raw, got, tt.want)
			}
			if got.IsMutating() != tt.mutating {
				t.Errorf("%q IsMutating() = %v, want %v", got, got.IsMutating(), tt.mutating)
			}
			if got.ShortLabel() != tt.label {
				t.Errorf("%q ShortLabel() = %q, want %q", got, got.ShortLabel(), tt.label)
			}
			if got.IsSubagent() != tt.delegated {
				t.Errorf("%q IsSubagent() = %v, want %v", got, got.IsSubagent(), tt.delegated)
			}
		})
	}
}

func TestParseProfileRejectsUnknownAndUniversalDelegation(t *testing.T) {
	for _, raw := range []string{"pow", "worker", "int", "", "unknown"} {
		if _, err := ParseProfile(raw); err == nil {
			t.Errorf("ParseProfile(%q) error = nil, want error", raw)
		}
	}
	if _, err := ParseSubagentProfile("universal"); err == nil {
		t.Error("ParseSubagentProfile(\"universal\") error = nil, want error")
	}
}

func TestSubagentProfilesAndToolPolicy(t *testing.T) {
	want := []Profile{ProfileStrength, ProfileAgility, ProfileIntelligence}
	got := SubagentProfiles()
	if len(got) != len(want) {
		t.Fatalf("SubagentProfiles() = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("SubagentProfiles()[%d] = %q, want %q", i, got[i], want[i])
		}
	}

	for _, profile := range want {
		spec, ok := SpecForProfile(profile)
		if !ok {
			t.Fatalf("SpecForProfile(%q) not found", profile)
		}
		if !spec.Allows(tool.KindRead) || !spec.Allows(tool.KindGrep) {
			t.Errorf("%q should allow read and grep", profile)
		}
		if profile == ProfileAgility {
			if spec.Allows(tool.KindEdit) || spec.Allows(tool.KindBash) {
				t.Errorf("agility must not allow edit or bash")
			}
		} else if !spec.Allows(tool.KindEdit) || !spec.Allows(tool.KindBash) {
			t.Errorf("%q should allow edit and bash", profile)
		}
	}
}
