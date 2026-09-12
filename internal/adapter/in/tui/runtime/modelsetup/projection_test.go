package modelsetup

import (
	"testing"

	"github.com/phongsathornpt/protonman/internal/adapter/out/model"
)

func TestProjectMarksOnlyActiveProviderModel(t *testing.T) {
	items := Project([]model.RemoteModel{{ID: "one"}, {ID: "two"}}, "alpha", "alpha", "two")
	if len(items) != 2 || items[0].Current || !items[1].Current {
		t.Fatalf("projection current flags = %#v", items)
	}
	other := Project([]model.RemoteModel{{ID: "two"}}, "beta", "alpha", "two")
	if other[0].Current {
		t.Fatal("inactive provider model marked current")
	}
}

func TestProjectionMetadataAndDescription(t *testing.T) {
	item := Projection{Model: model.RemoteModel{ID: "big-pickle", Name: "Big Pickle", Features: []string{"tools"}}, ProviderName: "opencode", Current: true}
	if got := Metadata(item); len(got) != 2 || got[0] != "FREE" || got[1] != "(current)" {
		t.Fatalf("metadata = %#v", got)
	}
}
