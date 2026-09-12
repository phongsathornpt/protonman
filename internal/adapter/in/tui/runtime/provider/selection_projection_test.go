package provider

import "testing"

func TestSelectionProjection(t *testing.T) {
	entry := SelectionEntry{
		Kind: SelectionConfigured, Name: "alpha", DisplayName: "Alpha",
		BaseURL: "https://alpha.example", IsConfigured: true, IsActive: true, IsFree: true,
	}
	if got := FilterValue(entry); got != "alpha Alpha https://alpha.example " {
		t.Fatalf("filter value = %q", got)
	}
	if got := Title(entry); got != "✓ Alpha · active · free" {
		t.Fatalf("title = %q", got)
	}
	if got := Description(entry); got != "https://alpha.example" {
		t.Fatalf("description = %q", got)
	}
	if got := Marker(entry); got != "(current)" {
		t.Fatalf("marker = %q", got)
	}
}
