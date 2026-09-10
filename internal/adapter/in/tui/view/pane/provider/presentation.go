package provider

import (
	"fmt"
	"strings"

	panecommon "github.com/phongsathornpt/protonman/internal/adapter/in/tui/view/pane/common"
	tuistyle "github.com/phongsathornpt/protonman/internal/adapter/in/tui/view/style"
)

type ProviderEditorState uint8

const (
	ProviderEditorInput ProviderEditorState = iota
	ProviderEditorFetching
	ProviderEditorConfirmOverwrite
	ProviderEditorSaving
	ProviderEditorSaveError
	ProviderEditorError
)

type ProviderEditorSnapshot struct {
	Width          int
	Height         int
	State          ProviderEditorState
	Name           string
	Endpoint       string
	Spinner        string
	UserConfigPath string
	ErrorMessage   string
	IsEditing      bool
	ActivateOnSave bool
	ProviderType   string
	ProtocolLabel  string
	RequiresAPIKey bool
	NameInput      string
	EndpointInput  string
	APIKeyInput    string
	FieldErrors    [3]string
}

func ProviderEditorRows(snapshot ProviderEditorSnapshot) ([]string, panecommon.Tone) {
	switch snapshot.State {
	case ProviderEditorFetching:
		return []string{
			tuistyle.BrandStyle.Render("Connecting · " + snapshot.Name),
			fmt.Sprintf("%s %s", snapshot.Spinner, snapshot.Endpoint),
		}, panecommon.ToneAssistant
	case ProviderEditorSaving:
		description := "  Applying the selected model as active"
		if snapshot.IsEditing && !snapshot.ActivateOnSave {
			description = "  Keeping the current active provider and model"
		}
		return []string{
			tuistyle.BrandStyle.Render("Saving · " + snapshot.Name),
			tuistyle.MutedStyle.Render(strings.TrimSpace(description)),
		}, panecommon.ToneAssistant
	case ProviderEditorSaveError:
		return []string{
			tuistyle.ErrorStyle.Render("Save failed"),
			snapshot.ErrorMessage,
		}, panecommon.ToneError
	case ProviderEditorConfirmOverwrite:
		return []string{
			tuistyle.WarningStyle.Render("Provider exists · " + strings.TrimSpace(snapshot.Name)),
			tuistyle.MutedStyle.Render("Continuing replaces endpoint and API key."),
		}, panecommon.ToneWarning
	case ProviderEditorError:
		return []string{
			tuistyle.ErrorStyle.Render("Connection failed"),
			snapshot.ErrorMessage,
		}, panecommon.ToneError
	default:
		return providerInputRows(snapshot), panecommon.ToneAssistant
	}
}

func providerInputRows(snapshot ProviderEditorSnapshot) []string {
	rows := []string{tuistyle.BrandStyle.Render(providerInputTitle(snapshot, true))}
	return append(rows, providerInputFields(snapshot, true)...)
}

func providerInputTitle(snapshot ProviderEditorSnapshot, compact bool) string {
	if snapshot.IsEditing {
		if compact {
			return fmt.Sprintf("Edit provider · %s", snapshot.Name)
		}
		return fmt.Sprintf("✓ Edit Provider: %s [Step 1/2: Connection]", snapshot.Name)
	}
	if compact {
		return "Add provider"
	}
	return "+ Add Model Provider [Step 1/2: Connection]"
}

func providerInputFields(snapshot ProviderEditorSnapshot, compact bool) []string {
	if compact {
		return []string{
			providerInlineField("N:", snapshot.NameInput, snapshot.FieldErrors[0]),
			providerInlineField("URL:", snapshot.EndpointInput, snapshot.FieldErrors[1]),
			providerInlineField("K:", snapshot.APIKeyInput, snapshot.FieldErrors[2]),
		}
	}
	keyLabel := "API Key:"
	if !snapshot.RequiresAPIKey {
		keyLabel = "API Key (optional):"
	}
	return []string{
		tuistyle.MutedStyle.Render("Protocol: ") + snapshot.ProtocolLabel, "",
		providerFieldLabel("Provider Name:", snapshot.FieldErrors[0]), snapshot.NameInput, "",
		providerFieldLabel("Endpoint (Base URL):", snapshot.FieldErrors[1]), snapshot.EndpointInput, "",
		providerFieldLabel(keyLabel, snapshot.FieldErrors[2]), snapshot.APIKeyInput,
	}
}

func providerInlineField(label, input, fieldError string) string {
	row := label + " " + input
	if fieldError != "" {
		row += " " + tuistyle.ErrorStyle.Render("("+fieldError+")")
	}
	return row
}

func providerFieldLabel(label, fieldError string) string {
	if fieldError == "" {
		return tuistyle.MutedStyle.Render(label)
	}
	return tuistyle.MutedStyle.Render(label+" ") + tuistyle.ErrorStyle.Render("("+fieldError+")")
}
