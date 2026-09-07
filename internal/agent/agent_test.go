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
		{"explorer", ProfileINT, false, false},
		{"reviewer", ProfileINT, false, false},
		{"worker", ProfilePOW, false, true},
		{"pow", ProfilePOW, false, true},
		{"dex", ProfileDEX, false, true},
		{"int", ProfileINT, false, false},
		{"  POW  ", ProfilePOW, false, true},
		{"  Dex  ", ProfileDEX, false, true},
		{"  INT  ", ProfileINT, false, false},
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
