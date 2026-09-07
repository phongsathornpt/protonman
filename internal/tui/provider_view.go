package tui

import (
	"context"
	"fmt"
	"github.com/projectTHORN/proton/internal/runtimepolicy"
	"net/url"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/projectTHORN/proton/internal/appdirs"
	"github.com/projectTHORN/proton/internal/config"
	"github.com/projectTHORN/proton/internal/model"
)

const providerViewID = "add_provider"

type providerPaneState int

const (
	providerStateInput providerPaneState = iota
	providerStateFetching
	providerStateSelectModel
	providerStateConfirmOverwrite
	providerStateSaving
	providerStateSaveError
	providerStateError
)

type providerField int

const (
	providerFieldName providerField = iota
	providerFieldEndpoint
	providerFieldAPIKey
	providerFieldCount
)

type modelsFetchedMsg struct {
	providerName string
	baseURL      string
	apiKey       string
	models       []model.RemoteModel
	requestID    uint64
	err          error
}

type providerSavedMsg struct {
	providerName string
	providerType string
	previousName string
	baseURL      string
	apiKey       string
	modelID      string
	activated    bool
	err          error
}

const maxProviderSelectRows = 8

type providerPaneView struct {
	state          providerPaneState
	providerType   string
	focusIndex     int
	isEditing      bool
	originalName   string
	presetID       string
	requiresAPIKey bool
	nameInput      textinput.Model
	endpointInput  textinput.Model
	apiKeyInput    textinput.Model
	models         []model.RemoteModel
	selectedIndex  int
	scrollOffset   int
	filterFreeOnly bool
	errorMessage   string
	fieldErrors    [providerFieldCount]string
	fetchRequestID uint64
	fetchCancel    context.CancelFunc
	selectedModel  string
	activateOnSave bool
}

func newProviderPaneView() *providerPaneView {
	return newProviderPaneViewWithPreset("")
}

func newProviderPaneViewWithConfig(cfg config.ProviderConfig) *providerPaneView {
	pv := newProviderPaneViewWithPreset(cfg.Name)
	pv.isEditing = true
	pv.originalName = strings.TrimSpace(cfg.Name)
	if strings.TrimSpace(cfg.Type) != "" {
		pv.providerType = strings.ToLower(strings.TrimSpace(cfg.Type))
	}
	if cfg.Name != "" {
		pv.nameInput.SetValue(cfg.Name)
	}
	if cfg.BaseURL != "" {
		pv.endpointInput.SetValue(cfg.BaseURL)
	}
	if cfg.APIKey != "" {
		pv.apiKeyInput.SetValue(cfg.APIKey)
	}
	return pv
}

func newProviderPaneViewWithPreset(preset string) *providerPaneView {
	preset = strings.ToLower(strings.TrimSpace(preset))
	name := ""
	endpoint := ""
	keyPlaceholder := "API key (optional)…"
	presetID := ""
	requiresAPIKey := false
	filterFree := false
	providerType := string(model.ProviderProtocolOpenAI)

	if p := model.LookupPreset(preset); p != nil {
		presetID = p.ID
		name = p.ID
		endpoint = p.BaseURL
		requiresAPIKey = p.RequiresKey
		providerType = string(p.Protocol)
		keyPlaceholder = providerKeyPlaceholder(*p)
		if p.ID == model.DefaultOpenCodeName {
			filterFree = true
		}
	} else if preset == "opencode-free" || preset == "free" {
		presetID = model.DefaultOpenCodeName
		name = model.DefaultOpenCodeName
		endpoint = model.DefaultOpenCodeEndpoint
		keyPlaceholder = "API key (optional)…"
		filterFree = true
	} else if preset != "" {
		name = preset
	}

	nameIn := textinput.New()
	nameIn.Prompt = glyphPrompt
	nameIn.Placeholder = "provider name…"
	if name != "" {
		nameIn.SetValue(name)
	}
	nameIn.CharLimit = 64

	endpointIn := textinput.New()
	endpointIn.Prompt = glyphPrompt
	endpointIn.Placeholder = "https://api.example.com/v1"
	if endpoint != "" {
		endpointIn.SetValue(endpoint)
	}
	endpointIn.CharLimit = 256

	keyIn := textinput.New()
	keyIn.Prompt = glyphPrompt
	keyIn.Placeholder = keyPlaceholder
	keyIn.EchoMode = textinput.EchoPassword
	keyIn.EchoCharacter = '•'
	keyIn.CharLimit = 256

	focusIndex := providerFieldName
	if name != "" && endpoint != "" {
		focusIndex = providerFieldAPIKey
	}

	pv := &providerPaneView{
		state:          providerStateInput,
		focusIndex:     int(focusIndex),
		presetID:       presetID,
		providerType:   providerType,
		requiresAPIKey: requiresAPIKey,
		nameInput:      nameIn,
		endpointInput:  endpointIn,
		apiKeyInput:    keyIn,
		filterFreeOnly: filterFree,
		activateOnSave: true,
	}
	pv.syncInputFocus()
	return pv
}

func (*providerPaneView) ID() string             { return providerViewID }
func (*providerPaneView) ReplacesComposer() bool { return true }

func providerKeyPlaceholder(p model.SupportedProviderPreset) string {
	if strings.TrimSpace(p.KeyPlaceholder) != "" {
		return p.KeyPlaceholder
	}
	if !p.RequiresKey {
		return "API key (optional)…"
	}
	return "API key…"
}

func (v *providerPaneView) isOpenCode() bool {
	return model.IsProvider(model.DefaultOpenCodeName, v.nameInput.Value(), v.endpointInput.Value())
}

func (v *providerPaneView) applyPreset(preset string) {
	preset = strings.ToLower(strings.TrimSpace(preset))
	if preset == "1" {
		preset = model.DefaultProtonmanName
	} else if preset == "2" {
		preset = model.DefaultOpenCodeName
	} else if preset == "3" {
		preset = model.DefaultOllamaName
	} else if preset == "4" {
		preset = model.DefaultOpenAIName
	} else if preset == "5" {
		preset = model.DefaultAnthropicName
	}

	if p := model.LookupPreset(preset); p != nil {
		previousName := strings.TrimSpace(v.nameInput.Value())
		v.nameInput.SetValue(p.ID)
		v.endpointInput.SetValue(p.BaseURL)
		if !strings.EqualFold(previousName, p.ID) {
			v.apiKeyInput.SetValue("")
		}
		v.apiKeyInput.Placeholder = providerKeyPlaceholder(*p)
		v.presetID = p.ID
		v.requiresAPIKey = p.RequiresKey
		v.providerType = string(p.Protocol)
		v.filterFreeOnly = (p.ID == model.DefaultOpenCodeName)
		v.clearValidation()
		v.focusIndex = int(providerFieldAPIKey)
		v.syncInputFocus()
	}
}

func (v *providerPaneView) protocolLabel() string {
	label := strings.ToLower(strings.TrimSpace(v.providerType))
	if label == "" {
		label = string(model.ProviderProtocolOpenAI)
	}
	if v.presetID == "" {
		return label + " · ctrl+r to switch"
	}
	return label
}

func (v *providerPaneView) toggleProtocol() {
	if v.presetID != "" {
		return
	}
	switch model.ProviderProtocol(strings.ToLower(strings.TrimSpace(v.providerType))) {
	case model.ProviderProtocolAnthropic:
		v.providerType = string(model.ProviderProtocolOpenAI)
	default:
		v.providerType = string(model.ProviderProtocolAnthropic)
	}
}

func (v *providerPaneView) clearValidation() {
	v.fieldErrors = [providerFieldCount]string{}
	v.errorMessage = ""
}

func (v *providerPaneView) clearFieldError(field providerField) {
	if field >= 0 && field < providerFieldCount {
		v.fieldErrors[field] = ""
	}
}

func (v *providerPaneView) validateDraft() bool {
	v.clearValidation()
	firstInvalid := providerFieldCount

	name := strings.TrimSpace(v.nameInput.Value())
	if name == "" {
		v.fieldErrors[providerFieldName] = "required"
		firstInvalid = providerFieldName
	}

	endpoint := strings.TrimSpace(v.endpointInput.Value())
	if endpoint == "" {
		v.fieldErrors[providerFieldEndpoint] = "required"
		if firstInvalid == providerFieldCount {
			firstInvalid = providerFieldEndpoint
		}
	} else if !isValidProviderEndpoint(endpoint) {
		v.fieldErrors[providerFieldEndpoint] = "use an HTTP(S) URL"
		if firstInvalid == providerFieldCount {
			firstInvalid = providerFieldEndpoint
		}
	}

	key := strings.TrimSpace(v.apiKeyInput.Value())
	if v.requiresAPIKey && key == "" {
		v.fieldErrors[providerFieldAPIKey] = "required for this provider"
		if firstInvalid == providerFieldCount {
			firstInvalid = providerFieldAPIKey
		}
	}

	if firstInvalid != providerFieldCount {
		v.focusIndex = int(firstInvalid)
		v.syncInputFocus()
		return false
	}

	v.nameInput.SetValue(name)
	v.endpointInput.SetValue(strings.TrimRight(endpoint, "/"))
	v.apiKeyInput.SetValue(key)
	return true
}

func isValidProviderEndpoint(raw string) bool {
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Hostname() == "" {
		return false
	}

	scheme := strings.ToLower(parsed.Scheme)
	return scheme == "http" || scheme == "https"
}

func (v *providerPaneView) hasNameConflict(m *bubbleModel) bool {
	if m == nil {
		return false
	}

	name := strings.TrimSpace(v.nameInput.Value())
	original := strings.TrimSpace(v.originalName)
	for providerKey, cfg := range m.providers {
		existingName := strings.TrimSpace(cfg.Name)
		if existingName == "" {
			existingName = providerKey
		}
		if strings.EqualFold(existingName, name) && !strings.EqualFold(existingName, original) {
			return true
		}
	}
	return false
}

func (v *providerPaneView) setFetchedModels(models []model.RemoteModel) {
	if v.isOpenCode() {
		freeList := make([]model.RemoteModel, 0)
		paidList := make([]model.RemoteModel, 0)
		for _, m := range models {
			if model.IsFreeModel(m.ID) {
				freeList = append(freeList, m)
			} else {
				paidList = append(paidList, m)
			}
		}
		v.models = append(freeList, paidList...)
		v.filterFreeOnly = len(freeList) > 0
	} else {
		v.models = models
		v.filterFreeOnly = false
	}
	v.selectedIndex = 0
	v.scrollOffset = 0
}

func (v *providerPaneView) currentModels() []model.RemoteModel {
	if !v.filterFreeOnly {
		return v.models
	}
	var filtered []model.RemoteModel
	for _, m := range v.models {
		if model.IsFreeModel(m.ID) {
			filtered = append(filtered, m)
		}
	}
	if len(filtered) == 0 {
		return v.models
	}
	return filtered
}

func (v *providerPaneView) Render(m *bubbleModel) string {
	v.resizeInputs(m.width)

	switch v.state {
	case providerStateFetching:
		rows := []string{
			brandStyle.Render("Connecting to " + v.nameInput.Value()),
			"",
			fmt.Sprintf("  %s Querying %s/models…", m.spinner.View(), v.endpointInput.Value()),
			mutedStyle.Render("  Checking endpoint & discovering model catalog"),
			"",
			mutedStyle.Render("esc cancel"),
		}
		return renderProviderModal(m, accentAssistant, rows)

	case providerStateSelectModel:
		models := v.currentModels()
		hasFreeModels := false
		for _, md := range v.models {
			if model.IsFreeModel(md.ID) {
				hasFreeModels = true
				break
			}
		}

		titlePrefix := "✓ Select Active Model"
		if v.isEditing && !v.activateOnSave {
			titlePrefix = "✓ Select Model · active provider unchanged"
		}
		title := fmt.Sprintf("%s (%d discovered) [Step 2/2]", titlePrefix, len(models))
		if hasFreeModels {
			if v.filterFreeOnly {
				title = fmt.Sprintf("%s (%d free models · [f] show all %d) [Step 2/2]", titlePrefix, len(models), len(v.models))
			} else {
				title = fmt.Sprintf("%s (%d discovered · [f] show free only) [Step 2/2]", titlePrefix, len(v.models))
			}
		}

		if len(models) == 0 {
			rows := []string{
				brandStyle.Render(title),
				"",
				mutedStyle.Render("No matching models found."),
				"",
				mutedStyle.Render("f toggle filter · esc back"),
			}
			return renderProviderModal(m, accentUser, rows)
		}

		selectedIndex, scrollOffset, visibleEnd := normalizedPickerWindow(v.selectedIndex, v.scrollOffset, len(models), maxProviderSelectRows)
		visible := models[scrollOffset:visibleEnd]

		rows := []string{
			brandStyle.Render(title),
			"",
		}

		if scrollOffset > 0 {
			rows = append(rows, mutedStyle.Render(fmt.Sprintf("  ▲ %d more above", scrollOffset)))
		}

		for i, md := range visible {
			idx := scrollOffset + i
			resolved := model.ResolveRemoteMetadata(strings.TrimSpace(v.nameInput.Value()), md)
			prefix := "    "
			if idx == selectedIndex {
				prefix = brandStyle.Render("  ❯ ")
			}
			line := fmt.Sprintf("%d. %s", idx+1, providerModelLabel(md))
			if model.IsFreeModel(md.ID) {
				line += " " + successStyle.Render("[FREE]")
			}
			if limits := formatModelTokenLimits(resolved.Profile.ContextWindow, resolved.Profile.MaxInputTokens, resolved.Profile.MaxOutputTokens); limits != "" {
				line += " [" + limits + "]"
			}
			if len(resolved.Features) > 0 {
				line += fmt.Sprintf(" (%s)", strings.Join(resolved.Features, ", "))
			}
			if reasoning := remoteModelReasoningSummary(strings.TrimSpace(v.nameInput.Value()), md, false); reasoning != "" {
				line += " [" + reasoning + "]"
			}
			if idx == v.selectedIndex {
				rows = append(rows, prefix+brandStyle.Render(line))
			} else {
				rows = append(rows, prefix+mutedStyle.Render(line))
			}
		}

		if visibleEnd < len(models) {
			rows = append(rows, mutedStyle.Render(fmt.Sprintf("  ▼ %d more below", len(models)-visibleEnd)))
		}

		footer := "↑/↓ or j/k move · 1-9 select · enter confirm & save · esc back"
		if v.isEditing && !v.activateOnSave {
			footer = "↑/↓ move · 1-9 select · enter save details · esc back"
		}
		if hasFreeModels {
			footer = "↑/↓ move · 1-9 select · f toggle free only · enter confirm · esc back"
			if v.isEditing && !v.activateOnSave {
				footer = "↑/↓ move · 1-9 select · f free only · enter save · esc back"
			}
		}
		rows = append(rows, "", mutedStyle.Render(footer))
		return renderProviderModal(m, accentUser, rows)

	case providerStateSaving:
		savingDescription := "  Applying the selected model as active"
		if v.isEditing && !v.activateOnSave {
			savingDescription = "  Keeping the current active provider and model"
		}
		rows := []string{
			brandStyle.Render("Saving Provider…"),
			"",
			fmt.Sprintf("  Writing %s to %s", v.nameInput.Value(), appdirs.UserConfigDisplay()),
			mutedStyle.Render(savingDescription),
		}
		return renderProviderModal(m, accentAssistant, rows)

	case providerStateSaveError:
		rows := []string{
			errorStyle.Render("✕ Provider Save Failed"),
			"",
			"  " + v.errorMessage,
			"",
			mutedStyle.Render("enter retry save · esc back to models · ctrl+c cancel"),
		}
		return renderProviderModal(m, accentError, rows)

	case providerStateConfirmOverwrite:
		rows := []string{
			warningStyle.Render("Provider Already Exists"),
			"",
			fmt.Sprintf("  %q is already configured.", strings.TrimSpace(v.nameInput.Value())),
			mutedStyle.Render("  Continuing will replace its endpoint and API key."),
			"",
			mutedStyle.Render("enter overwrite · esc back · ctrl+c cancel"),
		}
		return renderProviderModal(m, warningColor, rows)

	case providerStateError:
		rows := []string{
			errorStyle.Render("✕ Connection Failed"),
			"",
			"  " + v.errorMessage,
			"",
			mutedStyle.Render("enter / esc return to credentials"),
		}
		return renderProviderModal(m, accentError, rows)

	case providerStateInput:
		fallthrough
	default:
		return renderProviderInput(m)
	}
}

func (v *providerPaneView) resizeInputs(width int) {
	inputWidth := maxInt(8, width-18)
	v.nameInput.Width = inputWidth
	v.endpointInput.Width = inputWidth
	v.apiKeyInput.Width = inputWidth
}

func (v *providerPaneView) inputTitle(compact bool) string {
	if v.isEditing {
		if compact {
			return fmt.Sprintf("✓ Edit %s", v.nameInput.Value())
		}
		return fmt.Sprintf("✓ Edit Provider: %s [Step 1/2: Connection]", v.nameInput.Value())
	}
	if compact {
		return "+ Add Provider"
	}
	return "+ Add Model Provider [Step 1/2: Connection]"
}

func (v *providerPaneView) inputFieldRows(compact bool) []string {
	keyLabel := "API Key:"
	if !v.requiresAPIKey {
		keyLabel = "API Key (optional):"
	}

	if compact {
		return []string{
			renderProviderInlineField("N:", v.nameInput.View(), v.fieldErrors[providerFieldName]),
			renderProviderInlineField("URL:", v.endpointInput.View(), v.fieldErrors[providerFieldEndpoint]),
			renderProviderInlineField("K:", v.apiKeyInput.View(), v.fieldErrors[providerFieldAPIKey]),
		}
	}

	return []string{
		mutedStyle.Render("Protocol: ") + v.protocolLabel(),
		"",
		renderProviderFieldLabel("Provider Name:", v.fieldErrors[providerFieldName]),
		v.nameInput.View(),
		"",
		renderProviderFieldLabel("Endpoint (Base URL):", v.fieldErrors[providerFieldEndpoint]),
		v.endpointInput.View(),
		"",
		renderProviderFieldLabel(keyLabel, v.fieldErrors[providerFieldAPIKey]),
		v.apiKeyInput.View(),
	}
}

func renderProviderInput(m *bubbleModel) string {
	if m == nil || m.bottom == nil {
		return ""
	}
	view, ok := m.bottom.find(providerViewID).(*providerPaneView)
	if !ok || view == nil {
		return ""
	}
	compact := m.height <= 20
	rows := []string{brandStyle.Render(view.inputTitle(compact))}
	if !compact {
		rows = append(rows,
			"",
			mutedStyle.Render("Presets: alt+1 Protonman · alt+2 OpenCode · alt+3 Ollama · alt+4 OpenAI · alt+5 Anthropic"),
			"",
		)
	}
	rows = append(rows, view.inputFieldRows(compact)...)
	rows = append(rows, "")
	if compact {
		footer := fmt.Sprintf("%s · ctrl+r · tab fields · enter connect · esc", strings.ToLower(strings.TrimSpace(view.providerType)))
		if view.isEditing && !view.activateOnSave {
			footer = "enter save · active stays · esc cancel"
		}
		rows = append(rows, mutedStyle.Render(footer))
	} else {
		footer := "tab/shift+tab cycle · ctrl+r protocol · enter connect & fetch · esc cancel"
		if view.isEditing && !view.activateOnSave {
			footer = "tab/shift+tab cycle · enter save · active provider stays · esc cancel"
		}
		rows = append(rows, mutedStyle.Render(footer))
	}
	return renderProviderModal(m, accentAssistant, rows)
}

func renderProviderInlineField(label, input, fieldError string) string {
	row := label + " " + input
	if fieldError != "" {
		row += " " + errorStyle.Render("("+fieldError+")")
	}
	return row
}

func providerModelLabel(md model.RemoteModel) string {
	id := strings.TrimSpace(md.ID)
	name := strings.TrimSpace(md.Name)
	if name == "" || strings.EqualFold(name, id) {
		return id
	}
	if id == "" {
		return name
	}
	return fmt.Sprintf("%s (%s)", name, id)
}

func renderProviderModal(m *bubbleModel, border lipgloss.TerminalColor, rows []string) string {
	contentWidth := providerModalContentWidth(m)
	wrappedRows := make([]string, 0, len(rows))
	for _, row := range rows {
		if row == "" || lipgloss.Width(row) <= contentWidth {
			wrappedRows = append(wrappedRows, row)
			continue
		}
		wrappedRows = append(wrappedRows, strings.Split(wrapWords(row, contentWidth), "\n")...)
	}
	return renderModalRows(m, border, wrappedRows)
}

func providerModalContentWidth(m *bubbleModel) int {
	return maxInt(1, maxInt(1, m.width-4)-6)
}

func renderProviderFieldLabel(label, fieldError string) string {
	if fieldError == "" {
		return mutedStyle.Render(label)
	}
	return mutedStyle.Render(label+" ") + errorStyle.Render("("+fieldError+")")
}

func (v *providerPaneView) HandleKey(m *bubbleModel, message tea.KeyMsg) (bool, tea.Cmd) {
	defer v.normalizeModelSelection()
	switch v.state {
	case providerStateFetching:
		return v.handleFetchingKey(m, message)
	case providerStateSelectModel:
		return v.handleModelSelectKey(m, message)
	case providerStateSaving:
		return true, nil
	case providerStateSaveError:
		return v.handleSaveErrorKey(m, message)
	case providerStateConfirmOverwrite:
		return v.handleOverwriteKey(m, message)
	case providerStateError:
		return v.handleProviderErrorKey(m, message)
	default:
		return v.handleInputKey(m, message)
	}
}

func (v *providerPaneView) syncInputFocus() {
	v.nameInput.Blur()
	v.endpointInput.Blur()
	v.apiKeyInput.Blur()
	switch v.focusIndex {
	case 0:
		v.nameInput.Focus()
	case 1:
		v.endpointInput.Focus()
	case 2:
		v.apiKeyInput.Focus()
	}
}

type providerFetchRequest struct {
	ctx              context.Context
	requestID        uint64
	providerName     string
	providerType     string
	baseURL          string
	apiKey           string
	discoveryTimeout time.Duration
}

func (v *providerPaneView) beginFetch(parent context.Context, timeouts ...time.Duration) tea.Cmd {
	discoveryTimeout := runtimepolicy.ModelDiscoveryTimeout
	if len(timeouts) > 0 && timeouts[0] > 0 {
		discoveryTimeout = timeouts[0]
	}
	if v.fetchCancel != nil {
		v.fetchCancel()
	}

	if parent == nil {
		parent = context.Background()
	}
	ctx, cancel := context.WithCancel(parent)
	v.fetchCancel = cancel
	v.fetchRequestID++
	v.state = providerStateFetching

	return fetchProviderModelsCmd(providerFetchRequest{
		ctx:              ctx,
		requestID:        v.fetchRequestID,
		providerName:     strings.TrimSpace(v.nameInput.Value()),
		providerType:     v.providerType,
		baseURL:          strings.TrimSpace(v.endpointInput.Value()),
		apiKey:           strings.TrimSpace(v.apiKeyInput.Value()),
		discoveryTimeout: discoveryTimeout,
	})
}

func (v *providerPaneView) cancelFetch() {
	if v.fetchCancel == nil {
		return
	}
	v.fetchCancel()
	v.fetchCancel = nil
}

func fetchProviderModelsCmd(request providerFetchRequest) tea.Cmd {
	return func() tea.Msg {
		parent := request.ctx
		if parent == nil {
			parent = context.Background()
		}
		timeout := request.discoveryTimeout
		if timeout <= 0 {
			timeout = runtimepolicy.ModelDiscoveryTimeout
		}
		ctx, cancel := context.WithTimeout(parent, timeout)
		defer cancel()

		protocol := model.ProviderProtocol(strings.ToLower(strings.TrimSpace(request.providerType)))
		if protocol == "" {
			if preset := model.MatchProviderPreset(request.providerName, request.baseURL); preset != nil {
				protocol = preset.Protocol
			}
		}
		models, err := model.FetchProviderModelsForProtocol(ctx, protocol, request.baseURL, request.apiKey, model.WithDiscoveryTimeout(timeout))
		return modelsFetchedMsg{
			providerName: request.providerName,
			baseURL:      request.baseURL,
			apiKey:       request.apiKey,
			models:       models,
			requestID:    request.requestID,
			err:          err,
		}
	}
}

type providerSaveRequest struct {
	providerName string
	providerType string
	previousName string
	baseURL      string
	apiKey       string
	defaultModel string
	activate     bool
}

func saveProviderCmd(request providerSaveRequest) tea.Cmd {
	return func() tea.Msg {
		dirs, resolveErr := appdirs.Resolve("")
		if resolveErr != nil {
			return providerSavedMsg{providerName: request.providerName, providerType: request.providerType, previousName: request.previousName, baseURL: request.baseURL, apiKey: request.apiKey, modelID: request.defaultModel, activated: request.activate, err: resolveErr}
		}
		homeDir := dirs.Home

		prov := config.ProviderConfig{
			Name:    request.providerName,
			Type:    request.providerType,
			BaseURL: request.baseURL,
			APIKey:  request.apiKey,
		}

		err := config.SaveUserProviderConfigWithOptions(homeDir, prov, config.ProviderSaveOptions{
			DefaultModel: request.defaultModel,
			PreviousName: request.previousName,
			Activate:     request.activate,
		})
		return providerSavedMsg{
			providerName: request.providerName,
			providerType: request.providerType,
			previousName: request.previousName,
			baseURL:      request.baseURL,
			apiKey:       request.apiKey,
			modelID:      request.defaultModel,
			activated:    request.activate,
			err:          err,
		}
	}
}

func formatContextTokens(tokens int) string {
	if tokens >= 1000000 {
		return fmt.Sprintf("%.1fM", float64(tokens)/1000000.0)
	}
	if tokens >= 1000 {
		return fmt.Sprintf("%dK", tokens/1000)
	}
	return fmt.Sprintf("%d", tokens)
}
