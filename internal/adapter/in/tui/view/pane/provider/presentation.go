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
	ProviderEditorSelectModel
	ProviderEditorConfirmOverwrite
	ProviderEditorSaving
	ProviderEditorSaveError
	ProviderEditorError
)

type ProviderEditorModel struct {
	Label     string
	Free      bool
	Limits    string
	Features  string
	Reasoning string
}

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
	Models         []ProviderEditorModel
	SelectedIndex  int
	ScrollOffset   int
	FilterFreeOnly bool
	HasFreeModels  bool
	TotalModels    int
}

func ProviderEditorRows(snapshot ProviderEditorSnapshot) ([]string, panecommon.Tone) {
	switch snapshot.State {
	case ProviderEditorFetching:
		return []string{
			tuistyle.BrandStyle.Render("Connecting · " + snapshot.Name),
			fmt.Sprintf("%s %s", snapshot.Spinner, snapshot.Endpoint),
			tuistyle.MutedStyle.Render("esc cancel"),
		}, panecommon.ToneAssistant
	case ProviderEditorSelectModel:
		return providerModelRows(snapshot), panecommon.ToneUser
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
			tuistyle.MutedStyle.Render("enter retry · esc back · ctrl+c cancel"),
		}, panecommon.ToneError
	case ProviderEditorConfirmOverwrite:
		return []string{
			tuistyle.WarningStyle.Render("Provider exists · " + strings.TrimSpace(snapshot.Name)),
			tuistyle.MutedStyle.Render("Continuing replaces endpoint and API key."),
			tuistyle.MutedStyle.Render("enter overwrite · esc back · ctrl+c cancel"),
		}, panecommon.ToneWarning
	case ProviderEditorError:
		return []string{
			tuistyle.ErrorStyle.Render("Connection failed"),
			snapshot.ErrorMessage,
			tuistyle.MutedStyle.Render("enter or esc back"),
		}, panecommon.ToneError
	default:
		return providerInputRows(snapshot), panecommon.ToneAssistant
	}
}

func providerModelRows(snapshot ProviderEditorSnapshot) []string {
	titlePrefix := "Models"
	if snapshot.IsEditing && !snapshot.ActivateOnSave {
		titlePrefix = "Models · active unchanged"
	}
	title := fmt.Sprintf("%s · %d", titlePrefix, len(snapshot.Models))
	if snapshot.HasFreeModels {
		if snapshot.FilterFreeOnly {
			title = fmt.Sprintf("%s · %d free · f all %d", titlePrefix, len(snapshot.Models), snapshot.TotalModels)
		} else {
			title = fmt.Sprintf("%s · %d · f free", titlePrefix, snapshot.TotalModels)
		}
	}
	if len(snapshot.Models) == 0 {
		return []string{
			tuistyle.BrandStyle.Render(title),
			tuistyle.MutedStyle.Render("No matching models."),
			tuistyle.MutedStyle.Render("f toggle filter · esc back"),
		}
	}
	selected, offset, end := panecommon.NormalizedWindow(snapshot.SelectedIndex, snapshot.ScrollOffset, len(snapshot.Models), 8)
	rows := []string{tuistyle.BrandStyle.Render(title)}
	if offset > 0 {
		rows = append(rows, tuistyle.MutedStyle.Render(fmt.Sprintf("  ▲ %d more above", offset)))
	}
	for i, md := range snapshot.Models[offset:end] {
		idx := offset + i
		prefix := "    "
		if idx == selected {
			prefix = tuistyle.BrandStyle.Render("  ❯ ")
		}
		line := fmt.Sprintf("%d. %s", idx+1, md.Label)
		if md.Free {
			line += " " + tuistyle.SuccessStyle.Render("[FREE]")
		}
		if md.Limits != "" {
			line += " [" + md.Limits + "]"
		}
		if md.Features != "" {
			line += " (" + md.Features + ")"
		}
		if md.Reasoning != "" {
			line += " [" + md.Reasoning + "]"
		}
		if idx == selected {
			rows = append(rows, prefix+tuistyle.BrandStyle.Render(line))
		} else {
			rows = append(rows, prefix+tuistyle.MutedStyle.Render(line))
		}
	}
	if end < len(snapshot.Models) {
		rows = append(rows, tuistyle.MutedStyle.Render(fmt.Sprintf("  ▼ %d more below", len(snapshot.Models)-end)))
	}
	footer := "↑/↓ or j/k move · 1-9 select · enter confirm & save · esc back"
	if snapshot.IsEditing && !snapshot.ActivateOnSave {
		footer = "↑/↓ move · 1-9 select · enter save details · esc back"
	}
	if snapshot.HasFreeModels {
		footer = "↑/↓ move · 1-9 select · f toggle free only · enter confirm · esc back"
		if snapshot.IsEditing && !snapshot.ActivateOnSave {
			footer = "↑/↓ move · 1-9 select · f free only · enter save · esc back"
		}
	}
	return append(rows, tuistyle.MutedStyle.Render(footer))
}

func providerInputRows(snapshot ProviderEditorSnapshot) []string {
	rows := []string{tuistyle.BrandStyle.Render(providerInputTitle(snapshot, true))}
	rows = append(rows, providerInputFields(snapshot, true)...)
	footer := "tab fields · ctrl+r protocol · enter connect · esc"
	if snapshot.IsEditing && !snapshot.ActivateOnSave {
		footer = "tab fields · enter save · active stays · esc"
	}
	return append(rows, tuistyle.MutedStyle.Render(footer))
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
