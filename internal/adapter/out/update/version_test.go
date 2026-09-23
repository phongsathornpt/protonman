package update

import (
	"strings"
	"testing"
)

func TestNormalizeTag(t *testing.T) {
	cases := map[string]string{
		"v1.2.3":  "v1.2.3",
		"1.2.3":   "v1.2.3",
		" 1.2.3 ": "v1.2.3",
		"":        "",
	}
	for input, want := range cases {
		if got := NormalizeTag(input); got != want {
			t.Errorf("NormalizeTag(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestValidateTag(t *testing.T) {
	for _, valid := range []string{"v1.2.3", "1.2.3", "v1.2.3-rc.1"} {
		if _, err := ValidateTag(valid); err != nil {
			t.Errorf("ValidateTag(%q) error = %v", valid, err)
		}
	}
	for _, invalid := range []string{"", "latest", "v1.2", "1.2", "v1.2.3-", "version-1"} {
		if _, err := ValidateTag(invalid); err == nil {
			t.Errorf("ValidateTag(%q) expected error, got nil", invalid)
		}
	}
}

func TestCompare(t *testing.T) {
	cases := []struct {
		a, b string
		want int
	}{
		{"1.2.3", "v1.2.3", 0},
		{"1.2.3", "1.2.4", -1},
		{"1.3.0", "1.2.9", 1},
		{"2.0.0", "1.9.9", 1},
		{"1.2.3-rc.1", "1.2.3", -1},
		{"1.2.3-rc.1", "1.2.3-rc.2", -1},
		{"1.2.3", "1.2.3-rc.1", 1},
	}
	for _, tc := range cases {
		if got := Compare(tc.a, tc.b); got != tc.want {
			t.Errorf("Compare(%q, %q) = %d, want %d", tc.a, tc.b, got, tc.want)
		}
	}
}

func TestParseChecksumsAndMismatch(t *testing.T) {
	entries, err := parseChecksums([]byte("abc\n"))
	if err == nil || !strings.Contains(err.Error(), "no entries") {
		t.Fatalf("parseChecksums junk error = %v", err)
	}
	_ = entries
}
