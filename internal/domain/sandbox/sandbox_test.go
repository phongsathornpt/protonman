package sandbox

import "testing"

func TestParseNameAliases(t *testing.T) {
	cases := map[string]Name{
		"":          NameOff,
		"off":       NameOff,
		"workspace": NameWorkspace,
		"read-only": NameReadOnly,
		"readonly":  NameReadOnly,
		"strict":    NameStrict,
	}
	for input, want := range cases {
		got, err := ParseName(input)
		if err != nil {
			t.Fatalf("ParseName(%q) error = %v", input, err)
		}
		if got != want {
			t.Fatalf("ParseName(%q) = %s, want %s", input, got, want)
		}
	}
	if _, err := ParseName("custom"); err == nil {
		t.Fatal("ParseName(custom) error = nil, want invalid")
	}
}

func TestParseOriginRejectsIPAndPath(t *testing.T) {
	if _, err := ParseOrigin("https://example.com"); err != nil {
		t.Fatalf("valid origin error = %v", err)
	}
	for _, raw := range []string{
		"https://127.0.0.1",
		"https://example.com/secret",
		"ftp://example.com",
		"",
	} {
		if _, err := ParseOrigin(raw); err == nil {
			t.Fatalf("ParseOrigin(%q) error = nil", raw)
		}
	}
}

func TestNetworkPolicyAllowURL(t *testing.T) {
	blocked := NetworkPolicy{Mode: NetworkBlocked}
	if err := blocked.AllowURL("https://example.com"); err == nil {
		t.Fatal("blocked policy allowed a URL")
	}
	open := NetworkPolicy{Mode: NetworkUnrestricted}
	if err := open.AllowURL("https://example.com/x"); err != nil {
		t.Fatalf("unrestricted error = %v", err)
	}
	origin, err := ParseOrigin("https://example.com")
	if err != nil {
		t.Fatalf("ParseOrigin() error = %v", err)
	}
	allow := NetworkPolicy{Mode: NetworkAllowlist, Allowed: []Origin{origin}}
	if err := allow.AllowURL("https://example.com/docs"); err != nil {
		t.Fatalf("allowlist exact origin error = %v", err)
	}
	if err := allow.AllowURL("https://evil.example"); err == nil {
		t.Fatal("allowlist accepted a different host")
	}
}

func TestNewProfileStrictBlocksNetwork(t *testing.T) {
	profile, err := NewProfile(NameStrict, "/tmp/ws")
	if err != nil {
		t.Fatalf("NewProfile() error = %v", err)
	}
	if !profile.RestrictNetwork || profile.Network.Mode != NetworkBlocked {
		t.Fatalf("strict profile = %+v", profile)
	}
	if !profile.Confines() {
		t.Fatal("strict profile does not confine")
	}
}

func FuzzParseName(f *testing.F) {
	for _, seed := range []string{"", "off", "workspace", "strict", "read-only", "nope"} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, value string) {
		name, err := ParseName(value)
		if err != nil {
			if name != NameUnknown {
				t.Fatalf("invalid parse returned %s", name)
			}
			return
		}
		parsed, err := ParseName(name.String())
		if err != nil || parsed != name {
			t.Fatalf("round-trip %q -> %s", value, name)
		}
	})
}

func FuzzParseOrigin(f *testing.F) {
	for _, seed := range []string{
		"https://example.com",
		"http://example.com:8080",
		"https://127.0.0.1",
		"not a url",
	} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, value string) {
		origin, err := ParseOrigin(value)
		if err != nil {
			return
		}
		if origin.Scheme != "http" && origin.Scheme != "https" {
			t.Fatalf("accepted scheme %q", origin.Scheme)
		}
		if origin.Host == "" || origin.Port == "" {
			t.Fatalf("incomplete origin %+v", origin)
		}
	})
}
