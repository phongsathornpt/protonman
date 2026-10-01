//go:build desktop || desktop_gio

package settings

import (
	"errors"
	"image"
	"testing"
	"time"

	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"
)

var errTestProviderDelete = errors.New("store rejected the delete")

func providersTestInput(onSave func(ProviderSaveParams, func(error)), onDelete func(string, func(error))) ProviderFormInput {
	return ProviderFormInput{
		ProviderModels:        map[string][]string{},
		ProviderModelsLoading: map[string]bool{},
		Chrome:                agentsTestChrome(),
		OnSave:                onSave,
		OnDelete:              onDelete,
	}
}

func providersTestContext(now int64) layout.Context {
	return layout.Context{
		Ops:         new(op.Ops),
		Constraints: layout.Exact(image.Pt(900, 500)),
		Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
		Now:         time.Unix(now, 0),
	}
}

// Save must hand the form's field values through untouched, and the
// Save & Activate variant must be the only one that requests activation. If the
// two paths collapsed, every edit would silently switch the active provider.
func TestProviderFormSaveSubmitsTypedValues(t *testing.T) {
	component := New()
	component.OpenProviderForm("acme", "Acme", "https://acme.test", "openai", "acme-large")
	var got ProviderSaveParams
	input := providersTestInput(func(p ProviderSaveParams, done func(error)) {
		got = p
		done(nil)
	}, nil)

	component.LayoutProviderForm(providersTestContext(1), input)
	// A Clickable reports Clicked on the frame after the click.
	component.provider.SaveButton.Click()
	component.LayoutProviderForm(providersTestContext(2), input)
	component.LayoutProviderForm(providersTestContext(3), input)

	if got.ProviderName != "Acme" {
		t.Fatalf("saved provider name = %q, want the form's name %q", got.ProviderName, "Acme")
	}
	if got.BaseURL != "https://acme.test" {
		t.Fatalf("saved base URL = %q, want the form's endpoint", got.BaseURL)
	}
	if got.Activate {
		t.Fatal("plain Save must not request activation")
	}
}

// Deleting a provider must close the form on success and keep it open on
// failure, so the user does not lose the form they were editing because the
// store rejected the delete.
func TestProviderDeleteClosesFormOnlyOnSuccess(t *testing.T) {
	for _, tc := range []struct {
		name      string
		deleteErr error
		wantOpen  bool
	}{
		{"success", nil, false},
		{"failure", errTestProviderDelete, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			component := New()
			component.OpenProviderForm("acme", "Acme", "https://acme.test", "openai", "")
			var asked string
			input := providersTestInput(nil, func(id string, done func(error)) {
				asked = id
				done(tc.deleteErr)
			})
			component.LayoutProviderForm(providersTestContext(1), input)
			// A Clickable reports Clicked on the frame after the click.
			component.provider.DeleteButton.Click()
			component.LayoutProviderForm(providersTestContext(2), input)
			component.LayoutProviderForm(providersTestContext(3), input)

			// The form resolves its subject from the name field, falling back to
			// the selected id when the name is blank.
			if asked != "Acme" {
				t.Fatalf("delete asked for %q, want the form's name %q", asked, "Acme")
			}
			if got := component.provider.FormVisible; got != tc.wantOpen {
				t.Fatalf("form visible = %v after a %s delete, want %v", got, tc.name, tc.wantOpen)
			}
		})
	}
}

// An in-flight update must disable save and delete, otherwise the user can fire
// a second mutation against a provider that is mid-write.
func TestProviderFormIsInertWhileUpdating(t *testing.T) {
	component := New()
	component.OpenProviderForm("acme", "Acme", "https://acme.test", "openai", "")
	saves, deletes := 0, 0
	input := providersTestInput(func(ProviderSaveParams, func(error)) { saves++ }, func(string, func(error)) { deletes++ })
	input.Updating = true

	component.LayoutProviderForm(providersTestContext(1), input)
	component.LayoutProviderForm(providersTestContext(2), input)
	component.LayoutProviderForm(providersTestContext(3), input)
	component.LayoutProviderForm(providersTestContext(4), input)

	if saves != 0 || deletes != 0 {
		t.Fatalf("inert form fired %d saves and %d deletes, want 0 and 0", saves, deletes)
	}
}

// Cancelling must discard the form without invoking any store action.
func TestProviderFormCancelDoesNotSave(t *testing.T) {
	component := New()
	component.OpenProviderForm("acme", "Acme", "https://acme.test", "openai", "")
	saves := 0
	input := providersTestInput(func(ProviderSaveParams, func(error)) { saves++ }, nil)

	component.LayoutProviderForm(providersTestContext(1), input)
	// A Clickable reports Clicked on the frame after the click.
	component.provider.CancelButton.Click()
	component.LayoutProviderForm(providersTestContext(2), input)
	component.LayoutProviderForm(providersTestContext(3), input)

	if saves != 0 {
		t.Fatalf("cancelling fired %d saves, want 0", saves)
	}
	if component.provider.FormVisible {
		t.Fatal("cancelling must close the form")
	}
}
