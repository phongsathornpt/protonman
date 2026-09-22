package execview

import "testing"

func TestEveryCommandFamilyHasOneSummarizer(t *testing.T) {
	for _, row := range execCommandProfiles {
		if _, ok := familySummarizers[row.Family]; !ok {
			t.Fatalf("family %q has command rows but no summarizer", row.Family)
		}
	}
	if _, ok := familySummarizers[FamilyGeneric]; ok {
		t.Fatal("FamilyGeneric must not own a summarizer")
	}
	if len(familySummarizers) == 0 {
		t.Fatal("familySummarizers must not be empty")
	}
}
