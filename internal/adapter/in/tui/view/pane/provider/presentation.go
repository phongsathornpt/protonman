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
			tuistyle.BrandStyle.Render("Connecting to " + snapshot.Name),
			"",
			fmt.Sprintf("  %s Querying %s/models…", snapshot.Spinner, snapshot.Endpoint),
			tuistyle.MutedStyle.Render("  Checking endpoint & discovering model catalog"),
			"",
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
			tuistyle.BrandStyle.Render("Saving Provider…"),
			"",
			fmt.Sprintf("  Writing %s to %s", snapshot.Name, snapshot.UserConfigPath),
			tuistyle.MutedStyle.Render(description),
		}, panecommon.ToneAssistant
	case ProviderEditorSaveError:
		return []string{
			tuistyle.ErrorStyle.Render("✕ Provider Save Failed"), "",
			"  " + snapshot.ErrorMessage, "",
			tuistyle.MutedStyle.Render("enter retry save · esc back to models · ctrl+c cancel"),
		}, panecommon.ToneError
	case ProviderEditorConfirmOverwrite:
		return []string{
			tuistyle.WarningStyle.Render("Provider Already Exists"), "",
			fmt.Sprintf("  %q is already configured.", strings.TrimSpace(snapshot.Name)),
			tuistyle.MutedStyle.Render("  Continuing will replace its endpoint and API key."), "",
			tuistyle.MutedStyle.Render("enter overwrite · esc back · ctrl+c cancel"),
		}, panecommon.ToneWarning
	case ProviderEditorError:
		return []string{
			tuistyle.ErrorStyle.Render("✕ Connection Failed"), "",
			"  " + snapshot.ErrorMessage, "",
			tuistyle.MutedStyle.Render("enter / esc return to credentials"),
		}, panecommon.ToneError
	default:
		return providerInputRows(snapshot), panecommon.ToneAssistant
	}
}

func providerModelRows(snapshot ProviderEditorSnapshot) []string {
	titlePrefix := "✓ Select Active Model"
	if snapshot.IsEditing && !snapshot.ActivateOnSave {
		titlePrefix = "✓ Select Model · active provider unchanged"
	}
	title := fmt.Sprintf("%s (%d discovered) [Step 2/2]", titlePrefix, len(snapshot.Models))
	if snapshot.HasFreeModels {
		if snapshot.FilterFreeOnly {
			title = fmt.Sprintf("%s (%d free models · [f] show all %d) [Step 2/2]", titlePrefix, len(snapshot.Models), snapshot.TotalModels)
		} else {
			title = fmt.Sprintf("%s (%d discovered · [f] show free only) [Step 2/2]", titlePrefix, snapshot.TotalModels)
		}
	}
	if len(snapshot.Models) == 0 {
		return []string{
			tuistyle.BrandStyle.Render(title), "",
			tuistyle.MutedStyle.Render("No matching models found."), "",
			tuistyle.MutedStyle.Render("f toggle filter · esc back"),
		}
	}
	selected, offset, end := panecommon.NormalizedWindow(snapshot.SelectedIndex, snapshot.ScrollOffset, len(snapshot.Models), 8)
	rows := []string{tuistyle.BrandStyle.Render(title), ""}
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
	return append(rows, "", tuistyle.MutedStyle.Render(footer))
}

func providerInputRows(snapshot ProviderEditorSnapshot) []string {
	tiny := snapshot.Height < 14
	compact := snapshot.Height <= 20
	rows := []string{tuistyle.BrandStyle.Render(providerInputTitle(snapshot, compact))}
	if !compact {
		rows = append(rows,
			"",
			tuistyle.MutedStyle.Render("Presets: alt+1 Protonman · alt+2 OpenCode · alt+3 Ollama · alt+4 OpenAI · alt+5 Anthropic"),
			"",
		)
	}
	rows = append(rows, providerInputFields(snapshot, compact)...)
	if tiny {
		return append(rows, tuistyle.MutedStyle.Render("enter · esc"))
	}
	rows = append(rows, "")
	if compact {
		footer := fmt.Sprintf("%s · ctrl+r · tab fields · enter connect · esc", strings.ToLower(strings.TrimSpace(snapshot.ProviderType)))
		if snapshot.IsEditing && !snapshot.ActivateOnSave {
			footer = "enter save · active stays · esc cancel"
		}
		return append(rows, tuistyle.MutedStyle.Render(footer))
	}
	footer := "tab/shift+tab cycle · ctrl+r protocol · enter connect & fetch · esc cancel"
	if snapshot.IsEditing && !snapshot.ActivateOnSave {
		footer = "tab/shift+tab cycle · enter save · active provider stays · esc cancel"
	}
	return append(rows, tuistyle.MutedStyle.Render(footer))
}

func providerInputTitle(snapshot ProviderEditorSnapshot, compact bool) string {
	if snapshot.IsEditing {
		if compact {
			return fmt.Sprintf("✓ Edit %s", snapshot.Name)
		}
		return fmt.Sprintf("✓ Edit Provider: %s [Step 1/2: Connection]", snapshot.Name)
	}
	if compact {
		return "+ Add Provider"
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
