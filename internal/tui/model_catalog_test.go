package tui

import (
	"testing"

	"github.com/projectTHORN/proton/internal/model"
)

func TestModelCatalogStateScopesByProvider(t *testing.T) {
	var state modelCatalogState
	state.set("Provider-A", []model.RemoteModel{{ID: "a-1"}})
	state.set("provider-b", []model.RemoteModel{{ID: "b-1"}})

	if got := state.models("provider-a"); len(got) != 1 || got[0].ID != "a-1" {
		t.Fatalf("provider-a catalog = %#v", got)
	}
	if got := state.models("PROVIDER-B"); len(got) != 1 || got[0].ID != "b-1" {
		t.Fatalf("provider-b catalog = %#v", got)
	}
}

func TestModelCatalogStateReturnsCopies(t *testing.T) {
	var state modelCatalogState
	state.set("provider", []model.RemoteModel{{ID: "original"}})
	got := state.models("provider")
	got[0].ID = "mutated"

	if stored := state.models("provider"); stored[0].ID != "original" {
		t.Fatalf("catalog mutation leaked into state: %#v", stored)
	}
}
