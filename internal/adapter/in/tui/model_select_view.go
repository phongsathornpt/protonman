package tui

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/phongsathornpt/protonman/internal/adapter/in/tui/pane"
	"github.com/phongsathornpt/protonman/internal/adapter/out/config"
	"github.com/phongsathornpt/protonman/internal/adapter/out/model"
	"github.com/phongsathornpt/protonman/internal/app"
	"github.com/phongsathornpt/protonman/internal/base/runtimepolicy"
)

const modelSelectViewID = "model_select"
const maxModelSelectRows = 6

type modelSelectedMsg struct {
	providerName string
	modelID      string
	unverified   bool
	err          error
}

type modelSelectPaneView struct {
	index          int
	offset         int
	models         []model.RemoteModel
	allModels      []model.RemoteModel
	filter         string
	filtering      bool
	providerNames  []string
	providerIndex  int
	fetchRequestID uint64
	fetchCancel    context.CancelFunc
	loading        bool
	err            error
}

func newModelSelectPaneView(m *bubbleModel) *modelSelectPaneView {
	providers := make([]string, 0)
	if m != nil && len(m.providers) > 0 {
		for name := range m.providers {
			providers = append(providers, name)
		}
		sort.Strings(providers)
	} else if m != nil && m.activeProvider != "" {
		providers = append(providers, m.activeProvider)
	} else {
		providers = append(providers, model.DefaultProtonmanName)
	}

	providerIdx := 0
	if m != nil && m.activeProvider != "" {
		for i, name := range providers {
			if strings.EqualFold(name, m.activeProvider) {
				providerIdx = i
				break
			}
		}
	}

	// Resolve the catalog for the selected provider only.
	var modelsList []model.RemoteModel
	hasFreshCatalog := false
	if m != nil {
		modelsList, hasFreshCatalog = m.modelCatalogs.freshModels(providers[providerIdx], time.Now(), m.runtimeConfig.ModelCatalogTTL)
	}
	if !hasFreshCatalog {
		modelsList = nil
	}

	view := &modelSelectPaneView{
		providerNames: providers,
		providerIndex: providerIdx,
	}
	activeModel := ""
	if m != nil {
		activeModel = m.activeModel
	}
	view.setModels(modelsList, activeModel)
	return view
}

func (*modelSelectPaneView) ID() string             { return modelSelectViewID }
func (*modelSelectPaneView) ReplacesComposer() bool { return true }

func (v *modelSelectPaneView) setModels(models []model.RemoteModel, activeModel string) {
	if v == nil {
		return
	}
	v.allModels = append([]model.RemoteModel(nil), models...)
	v.applyFilter(activeModel)
}

func (v *modelSelectPaneView) applyFilter(activeModel string) {
	if v == nil {
		return
	}
	query := strings.ToLower(strings.TrimSpace(v.filter))
	if query == "" {
		v.models = append([]model.RemoteModel(nil), v.allModels...)
		v.resetSelection(activeModel)
		return
	}
	filtered := make([]model.RemoteModel, 0, len(v.allModels))
	for _, md := range v.allModels {
		haystack := strings.ToLower(strings.Join([]string{
			md.ID, md.Name, md.Provider, strings.Join(md.Features, " "),
		}, " "))
		if strings.Contains(haystack, query) {
			filtered = append(filtered, md)
		}
	}
	v.models = filtered
	v.resetSelection(activeModel)
}

func (v *modelSelectPaneView) resetSelection(activeModel string) {
	if v == nil {
		return
	}
	v.index = 0
	v.offset = 0
	for i, md := range v.models {
		if strings.EqualFold(md.ID, activeModel) {
			v.index = i
			return
		}
	}
}

func (v *modelSelectPaneView) activeProviderName() string {
	if v == nil || v.providerIndex < 0 || v.providerIndex >= len(v.providerNames) {
		return model.DefaultProtonmanName
	}
	return v.providerNames[v.providerIndex]
}

func (v *modelSelectPaneView) beginFetch(parent context.Context, providerName string, cfg config.ProviderConfig, timeouts ...time.Duration) tea.Cmd {
	discoveryTimeout := runtimepolicy.ModelDiscoveryTimeout
	if len(timeouts) > 0 && timeouts[0] > 0 {
		discoveryTimeout = timeouts[0]
	}
	v.cancelFetch()
	if parent == nil {
		parent = context.Background()
	}
	ctx, cancel := context.WithCancel(parent)
	v.fetchCancel = cancel
	v.fetchRequestID++
	v.loading = true
	v.err = nil
	v.models = nil
	v.allModels = nil
	v.index = 0
	v.offset = 0
	return fetchProviderModelsCmd(providerFetchRequest{
		ctx:              ctx,
		requestID:        v.fetchRequestID,
		providerName:     providerName,
		baseURL:          cfg.BaseURL,
		apiKey:           cfg.APIKey,
		discoveryTimeout: discoveryTimeout,
	})
}

func (v *modelSelectPaneView) cancelFetch() {
	if v == nil || v.fetchCancel == nil {
		return
	}
	v.fetchCancel()
	v.fetchCancel = nil
}

func (v *modelSelectPaneView) loadProvider(m *bubbleModel, force bool) tea.Cmd {
	if v == nil || m == nil {
		return nil
	}
	providerName := v.activeProviderName()
	v.cancelFetch()
	v.loading = false
	v.err = nil
	if !force {
		if models, ok := m.modelCatalogs.freshModels(providerName, time.Now(), m.runtimeConfig.ModelCatalogTTL); ok {
			v.setModels(models, m.activeModel)
			return nil
		}
	}

	cfg, configured := m.providers[normalizeProviderKey(providerName)]
	if configured {
		isOpenCode := model.IsProvider(model.DefaultOpenCodeName, providerName, cfg.BaseURL)
		if strings.TrimSpace(cfg.APIKey) != "" || isOpenCode {
			return v.beginFetch(m.ctx, providerName, cfg, m.runtimeConfig.ModelDiscoveryTimeout)
		}
	}
	v.setModels(nil, m.activeModel)
	return nil
}

func (m *bubbleModel) openModelSelectPane() tea.Cmd {
	if m == nil || m.bottom.has(modelSelectViewID) {
		return nil
	}
	view := newModelSelectPaneView(m)
	m.bottom.push(view)
	m.relayout()
	return view.loadProvider(m, false)
}

func (v *modelSelectPaneView) Render(m *bubbleModel) string {
	items := make([]pane.ModelItem, 0, len(v.models))
	providerName := v.activeProviderName()
	for _, md := range v.models {
		label := strings.TrimSpace(md.Name)
		if label == "" {
			label = md.ID
		}
		details := make([]string, 0, 4)
		resolved := model.ResolveRemoteMetadata(providerName, md)
		if label != md.ID && strings.TrimSpace(md.ID) != "" {
			details = append(details, md.ID)
		}
		if limits := formatModelTokenLimits(resolved.Profile.ContextWindow, resolved.Profile.MaxInputTokens, resolved.Profile.MaxOutputTokens); limits != "" {
			details = append(details, limits)
		}
		if len(resolved.Features) > 0 {
			details = append(details, strings.Join(resolved.Features, " · "))
		}
		if reasoning := remoteModelReasoningSummary(providerName, md, true); reasoning != "" {
			details = append(details, reasoning)
		}
		items = append(items, pane.ModelItem{
			ID:      md.ID,
			Label:   label,
			Free:    model.IsFreeModel(md.ID),
			Current: m != nil && strings.EqualFold(md.ID, m.activeModel),
			Details: strings.Join(details, " · "),
		})
	}
	errorText := ""
	if v.err != nil {
		errorText = v.err.Error()
	}
	rows := pane.ModelRows(pane.ModelSnapshot{
		Width:         m.width,
		Height:        m.height,
		Index:         v.index,
		Offset:        v.offset,
		ProviderName:  providerName,
		ProviderCount: len(v.providerNames),
		Models:        items,
		Filter:        v.filter,
		Filtering:     v.filtering,
		Loading:       v.loading,
		ErrorText:     errorText,
	})
	return renderModalRows(m, accentAssistant, rows)
}

func (v *modelSelectPaneView) HandleKey(m *bubbleModel, message tea.KeyMsg) (bool, tea.Cmd) {
	if key.Matches(message, m.keys.ToggleModel) {
		v.cancelFetch()
		m.bottom.remove(modelSelectViewID)
		return true, nil
	}
	if v.filtering {
		switch message.Type {
		case tea.KeyEsc:
			v.filtering = false
			return true, nil
		case tea.KeyBackspace, tea.KeyCtrlH, tea.KeyDelete:
			runes := []rune(v.filter)
			if len(runes) > 0 {
				v.filter = string(runes[:len(runes)-1])
				v.applyFilter(m.activeModel)
			}
			return true, nil
		case tea.KeyEnter:
			v.filtering = false
			return true, nil
		case tea.KeyRunes:
			v.filter += string(message.Runes)
			v.applyFilter(m.activeModel)
			return true, nil
		}
	}

	switch message.String() {
	case "/":
		v.filtering = true
		return true, nil
	case "ctrl+u":
		v.filter = ""
		v.filtering = false
		v.applyFilter(m.activeModel)
		return true, nil
	case "esc", "q":
		v.cancelFetch()
		m.bottom.remove(modelSelectViewID)
		return true, nil

	case "p":
		// Switch to provider select view
		v.cancelFetch()
		m.bottom.remove(modelSelectViewID)
		if !m.bottom.has(providerSelectViewID) {
			m.bottom.push(newProviderSelectPaneView(m))
		}
		return true, nil

	case "a":
		// Switch to add provider view
		v.cancelFetch()
		m.bottom.remove(modelSelectViewID)
		if !m.bottom.has(providerViewID) {
			m.bottom.push(newProviderPaneView())
		}
		return true, nil

	case "r":
		return true, v.loadProvider(m, true)

	case "tab":
		if len(v.providerNames) > 1 {
			v.providerIndex = (v.providerIndex + 1) % len(v.providerNames)
			return true, v.loadProvider(m, false)
		}
		return true, nil

	case "up", "k":
		if v.index > 0 {
			v.index--
		}
		return true, nil

	case "down", "j":
		if v.index < len(v.models)-1 {
			v.index++
		}
		return true, nil

	case "pgup":
		v.index -= pickerVisibleRows(m.height, maxModelSelectRows)
		if v.index < 0 {
			v.index = 0
		}
		return true, nil

	case "pgdown":
		v.index += pickerVisibleRows(m.height, maxModelSelectRows)
		if v.index >= len(v.models) {
			v.index = len(v.models) - 1
		}
		return true, nil

	case "home", "g":
		v.index = 0
		return true, nil

	case "end", "G":
		if len(v.models) > 0 {
			v.index = len(v.models) - 1
		}
		return true, nil

	case "1", "2", "3", "4", "5", "6", "7", "8", "9":
		return true, nil

	case "enter":
		if len(v.models) == 0 {
			m.bottom.remove(modelSelectViewID)
			if !m.bottom.has(providerViewID) {
				m.bottom.push(newProviderPaneView())
			}
			return true, nil
		}
		if v.index >= 0 && v.index < len(v.models) {
			selected := v.models[v.index]
			provName := model.DefaultProtonmanName
			if v.providerIndex >= 0 && v.providerIndex < len(v.providerNames) {
				provName = v.providerNames[v.providerIndex]
			}
			cmd := saveDefaultModelCmd(provName, selected.ID)
			m.bottom.remove(modelSelectViewID)
			return true, cmd
		}
		return true, nil

	default:
		return false, nil
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

func formatModelTokenLimits(contextWindow, maxInput, maxOutput int) string {
	parts := make([]string, 0, 3)
	if contextWindow > 0 {
		parts = append(parts, formatContextTokens(contextWindow)+" context")
	}
	if maxInput > 0 {
		parts = append(parts, formatContextTokens(maxInput)+" input")
	}
	if maxOutput > 0 {
		parts = append(parts, formatContextTokens(maxOutput)+" output")
	}
	return strings.Join(parts, " · ")
}

func saveDefaultModelCmd(providerName, modelID string) tea.Cmd {
	return saveModelSelectionCmd(providerName, modelID, false)
}

func saveModelSelectionCmd(providerName, modelID string, unverified bool) tea.Cmd {
	return func() tea.Msg {
		err := (app.Providers{}).SelectModel(providerName, modelID)
		return modelSelectedMsg{
			providerName: providerName,
			modelID:      modelID,
			unverified:   unverified,
			err:          err,
		}
	}
}
