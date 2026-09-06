package tui

import (
	"testing"

	"github.com/projectTHORN/proton/internal/config"
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

func TestModelPickerRejectsStaleProviderResponse(t *testing.T) {
	m := newTestSkillsModel(t, 1)
	m.providers = map[string]config.ProviderConfig{
		"alpha": {Name: "alpha", APIKey: "a"},
		"beta":  {Name: "beta", APIKey: "b"},
	}
	m.activeProvider = "alpha"
	view := newModelSelectPaneView(m)
	m.bottom.push(view)

	view.fetchRequestID = 2
	view.providerIndex = 1 // beta after sort
	updated, _ := m.Update(modelsFetchedMsg{
		providerName: "alpha",
		requestID:    1,
		models:       []model.RemoteModel{{ID: "stale-alpha"}},
	})
	m = updated.(*bubbleModel)
	view = m.bottom.find(modelSelectViewID).(*modelSelectPaneView)
	if got := m.modelCatalogs.models("alpha"); len(got) != 0 {
		t.Fatalf("stale alpha response mutated catalog: %#v", got)
	}
	if len(view.models) > 0 && view.models[0].ID == "stale-alpha" {
		t.Fatalf("stale alpha response mutated beta picker: %#v", view.models)
	}
}

func TestModelPickerAcceptsCurrentProviderResponse(t *testing.T) {
	m := newTestSkillsModel(t, 1)
	m.providers = map[string]config.ProviderConfig{
		"alpha": {Name: "alpha", APIKey: "a"},
		"beta":  {Name: "beta", APIKey: "b"},
	}
	m.activeProvider = "beta"
	view := newModelSelectPaneView(m)
	m.bottom.push(view)
	view.fetchRequestID = 3

	updated, _ := m.Update(modelsFetchedMsg{
		providerName: "beta",
		requestID:    3,
		models:       []model.RemoteModel{{ID: "beta-model"}},
	})
	m = updated.(*bubbleModel)
	view = m.bottom.find(modelSelectViewID).(*modelSelectPaneView)
	if len(view.models) != 1 || view.models[0].ID != "beta-model" {
		t.Fatalf("current response not applied: %#v", view.models)
	}
}
