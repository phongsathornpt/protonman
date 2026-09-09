package runtime

import (
	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/list"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"context"
	"fmt"
	"github.com/phongsathornpt/protonman/internal/adapter/in/tui/view/pane"
	"github.com/phongsathornpt/protonman/internal/adapter/out/config"
	"github.com/phongsathornpt/protonman/internal/adapter/out/model"
	"github.com/phongsathornpt/protonman/internal/app"
	"github.com/phongsathornpt/protonman/internal/base/runtimepolicy"
	"github.com/phongsathornpt/protonman/internal/core/modelprofile"
	"github.com/phongsathornpt/protonman/internal/core/tool"
	sdk "github.com/phongsathornpt/protonman/proton-sdk"
	"sort"
	"strings"
	"time"
)

type providerModelCatalog struct {
	models    []model.RemoteModel
	fetchedAt time.Time
}

type modelCatalogState struct {
	entries map[string]providerModelCatalog
}

func normalizeProviderKey(name string) string {
	return strings.ToLower(strings.TrimSpace(name))
}

func (s *modelCatalogState) set(provider string, models []model.RemoteModel) {
	s.setAt(provider, models, time.Now())
}

func (s *modelCatalogState) setAt(provider string, models []model.RemoteModel, fetchedAt time.Time) {
	key := normalizeProviderKey(provider)
	if key == "" {
		return
	}
	if s.entries == nil {
		s.entries = make(map[string]providerModelCatalog)
	}
	s.entries[key] = providerModelCatalog{models: append([]model.RemoteModel(nil), models...), fetchedAt: fetchedAt}
}

func (s *modelCatalogState) delete(provider string) {
	if s == nil || s.entries == nil {
		return
	}
	delete(s.entries, normalizeProviderKey(provider))
}

func (s *modelCatalogState) models(provider string) []model.RemoteModel {
	if s == nil || s.entries == nil {
		return nil
	}
	entry, ok := s.entries[normalizeProviderKey(provider)]
	if !ok {
		return nil
	}
	return append([]model.RemoteModel(nil), entry.models...)
}

func (s *modelCatalogState) freshModels(provider string, now time.Time, ttl time.Duration) ([]model.RemoteModel, bool) {
	if s == nil || s.entries == nil {
		return nil, false
	}
	entry, ok := s.entries[normalizeProviderKey(provider)]
	if !ok || entry.fetchedAt.IsZero() || ttl <= 0 || now.Sub(entry.fetchedAt) >= ttl {
		return nil, false
	}
	return append([]model.RemoteModel(nil), entry.models...), true
}

func (m *bubbleModel) modelIDKnown(provider, modelID string) bool {
	if m == nil {
		return false
	}
	models := m.modelCatalogs.models(provider)
	for _, candidate := range models {
		if strings.EqualFold(strings.TrimSpace(candidate.ID), strings.TrimSpace(modelID)) {
			return true
		}
	}
	return false
}

func (m *bubbleModel) activeRemoteModel() (model.RemoteModel, bool) {
	if m == nil {
		return model.RemoteModel{}, false
	}
	for _, candidate := range m.modelCatalogs.models(m.activeProvider) {
		if strings.EqualFold(strings.TrimSpace(candidate.ID), strings.TrimSpace(m.activeModel)) {
			return candidate, true
		}
	}
	return model.RemoteModel{}, false
}

func (m *bubbleModel) syncPromptHeight() {
	if m.bottom == nil {
		return
	}
	prompt := m.bottom.prompt()
	if prompt == nil {
		return
	}
	lines := strings.Count(prompt.Value(), "\n") + 1
	if lines < 1 {
		lines = 1
	}
	if lines > 4 {
		lines = 4
	}
	if prompt.Height() != lines {
		prompt.SetHeight(lines)
	}
}

func (m *bubbleModel) resize(width int, height int) {
	if width <= 0 {
		width = defaultBubbleWidth
	}
	if height <= 0 {
		height = defaultBubbleHeight
	}
	m.width = width
	m.height = height
	m.help.SetWidth(maxInt(1, width-2))
	prompt := m.bottom.prompt()
	prompt.SetWidth(maxInt(1, width-4))
	m.syncPromptHeight()
	m.transcriptViewport.SetWidth(maxInt(1, width-10))
	m.transcriptViewport.SetHeight(maxInt(1, height-10))
	if m.historyState != nil {
		m.historyState.SetWidth(width)
	}
	m.relayout()
	m.refreshTranscriptViewport(false)
}

func (m *bubbleModel) relayoutIfSlashChanged(bool) {
	m.relayout()
}

type frameChrome struct {
	generation uint64
	status     string
	top        string
	composer   string
	footer     string
	height     int
}

func (m *bubbleModel) buildFrameChrome() frameChrome {
	frame := frameChrome{}
	frame.status = m.statusView()
	frame.top = m.bottom.renderTop(m)
	if m.bottom.composerVisible() {
		// The composer is small and stateful (cursor, focus, placeholder, bash mode).
		// Render it from the textarea model every frame instead of reusing terminal
		// output from a previous frame. Caching this string can leave stale prompt
		// rows behind when the transcript scrolls while the textarea changes.
		frame.composer = m.promptView()
	}
	frame.footer = m.footerView()
	for _, part := range []string{frame.status, frame.top, frame.composer} {
		if part != "" {
			frame.height += lipgloss.Height(part)
		}
	}
	// The footer is always joined into the live view; even an empty footer
	// occupies one physical row in lipgloss.JoinVertical.
	frame.height += lipgloss.Height(frame.footer)

	return frame
}

type viewportScrollSnapshot struct {
	follow      bool
	yOffset     int
	anchor      ScrollAnchor
	anchorValid bool
}

func (m *bubbleModel) relayout() {
	scroll := m.captureViewportScroll()
	m.syncPromptHeight()
	m.applyFrameLayout(scroll, m.buildFrameChrome())
}

func (m *bubbleModel) applyFrameLayout(scroll viewportScrollSnapshot, frame frameChrome) {
	m.layoutGeneration++
	frame.generation = m.layoutGeneration
	m.frameChrome = frame
	viewportHeight := m.height - frame.height
	if viewportHeight < 1 {
		viewportHeight = 1
	}
	if m.viewport.Width() != m.width || m.viewport.Height() != viewportHeight {
		m.viewport.SetWidth(m.width)
		m.viewport.SetHeight(viewportHeight)
		m.markViewportViewDirty()
	}
	m.refreshViewportWithScroll(scroll)
}

func (m *bubbleModel) frameChromeForView() frameChrome {
	m.syncPromptHeight()
	frame := m.buildFrameChrome()
	if m.frameChrome.generation == 0 || frame.height != m.frameChrome.height {
		m.applyFrameLayout(m.captureViewportScroll(), frame)
		return m.frameChrome
	}
	frame.generation = m.frameChrome.generation
	m.frameChrome = frame
	return frame
}

func (m *bubbleModel) chromeHeight() int {
	return m.buildFrameChrome().height
}

func (m *bubbleModel) refreshViewport() {
	m.refreshViewportWithScroll(m.captureViewportScroll())
}

func (m *bubbleModel) refreshViewportWithScroll(scroll viewportScrollSnapshot) {
	if m.historyState != nil && !scroll.follow && !m.viewportTailOnly {
		committedRevision, activeRevision := m.historyState.Revisions()
		if committedRevision == m.viewportCommittedRevision {
			// The user is reading older content and only the mutable tail changed.
			// Keep the viewport buffer stable until they scroll again instead of
			// rebuilding the entire transcript for invisible streaming deltas.
			m.viewportStaleTail = activeRevision != m.viewportActiveRevision
			m.followTail = false
			if m.showTranscript {
				m.refreshTranscriptViewport(false)
			}
			return
		}
	}

	content := ""
	tailOnly := false
	if scroll.follow && m.busy && m.historyState.Active() != nil {
		content, tailOnly = m.historyState.RenderTailContent(maxInt(1, m.viewport.Height()))
	}
	if !tailOnly {
		content = m.fullViewportContent()
	}
	m.setViewportContent(content, !tailOnly)
	m.viewportTailOnly = tailOnly
	m.restoreViewportScroll(scroll)
	if m.showTranscript {
		m.refreshTranscriptViewport(false)
	}
}

func (m *bubbleModel) setViewportContent(content string, fullHistory bool) {
	m.viewport.SetContent(content)
	m.markViewportViewDirty()
	m.viewportStaleTail = false
	m.viewportLineAnchors = nil
	if m.historyState != nil {
		m.viewportCommittedRevision, m.viewportActiveRevision = m.historyState.Revisions()
	}
	if !fullHistory || m.historyState == nil {
		return
	}
	historyAnchors := m.historyState.ScrollAnchors()
	if len(historyAnchors) == 0 {
		return
	}
	prefix := m.historyViewportPrefixLines()
	m.viewportLineAnchors = make([]ScrollAnchor, prefix+len(historyAnchors))
	copy(m.viewportLineAnchors[prefix:], historyAnchors)
}

func (m *bubbleModel) captureViewportScroll() viewportScrollSnapshot {
	scroll := viewportScrollSnapshot{follow: m.followTail, yOffset: m.viewport.YOffset()}
	if scroll.follow || m.viewportTailOnly || m.historyState == nil {
		return scroll
	}
	if m.viewport.YOffset() >= 0 && m.viewport.YOffset() < len(m.viewportLineAnchors) {
		scroll.anchor = m.viewportLineAnchors[m.viewport.YOffset()]
		_, scroll.anchorValid = m.historyState.ResolveScrollAnchor(scroll.anchor)
		if scroll.anchorValid {
			return scroll
		}
	}
	historyLine := m.viewport.YOffset() - m.historyViewportPrefixLines()
	if historyLine < 0 {
		return scroll
	}
	scroll.anchor = m.historyState.CaptureScrollAnchor(historyLine)
	_, scroll.anchorValid = m.historyState.ResolveScrollAnchor(scroll.anchor)
	return scroll
}

func (m *bubbleModel) restoreViewportScroll(scroll viewportScrollSnapshot) {
	if scroll.follow {
		before := m.viewport.YOffset()
		m.viewport.GotoBottom()
		if m.viewport.YOffset() != before {
			m.markViewportViewDirty()
		}
		m.followTail = true
		return
	}
	m.followTail = false
	yOffset := scroll.yOffset
	if scroll.anchorValid && m.historyState != nil {
		if historyLine, ok := m.historyState.ResolveScrollAnchor(scroll.anchor); ok {
			yOffset = m.historyViewportPrefixLines() + historyLine
		}
	}
	if m.viewport.YOffset() != yOffset {
		m.viewport.SetYOffset(yOffset)
		m.markViewportViewDirty()
	}
}

func (m *bubbleModel) historyViewportPrefixLines() int {
	if !m.showWelcome || m.historyState == nil || m.historyState.RenderContent() == "" {
		return 0
	}
	return lipgloss.Height(m.welcomeCard())
}

func (m *bubbleModel) fullViewportContent() string {
	content := m.historyState.RenderContent()
	if !m.showWelcome {
		return content
	}
	welcome := m.welcomeCard()
	if content == "" {
		return welcome
	}
	return welcome + "\n" + content
}

func (m *bubbleModel) hydrateViewportForScroll() {
	if !m.viewportTailOnly && !m.viewportStaleTail {
		return
	}
	scroll := m.captureViewportScroll()
	m.setViewportContent(m.fullViewportContent(), true)
	m.viewportTailOnly = false
	m.restoreViewportScroll(scroll)
}

func (m *bubbleModel) View() tea.View {
	if m.width == 0 || m.height == 0 {
		return tea.NewView("Starting Protonman…")
	}
	base := m.liveView()
	if m.showTranscript {
		base = overlayCenter(base, m.transcriptOverlayView(), m.width, m.height)
	}
	view := tea.NewView(base)
	view.AltScreen = true
	view.MouseMode = tea.MouseModeCellMotion
	return view
}

func (m *bubbleModel) markViewportViewDirty() {
	if m != nil {
		m.viewportViewDirty = true
	}
}

func (m *bubbleModel) renderedViewport() string {
	if m == nil {
		return ""
	}
	if !m.viewportViewDirty && m.viewportViewCache != "" {
		return m.viewportViewCache
	}
	m.viewportViewCache = m.viewport.View()
	m.viewportViewDirty = false
	return m.viewportViewCache
}

func (m *bubbleModel) liveView() string {
	frame := m.frameChromeForView()
	parts := []string{m.renderedViewport()}
	for _, part := range []string{frame.status, frame.top, frame.composer} {
		if part != "" {
			parts = append(parts, part)
		}
	}
	parts = append(parts, frame.footer)
	return lipgloss.JoinVertical(lipgloss.Left, parts...)
}

func (m *bubbleModel) footerView() string {
	if top := m.bottom.top(); top != nil {
		if top.ReplacesComposer() {
			return ""
		}
		return m.shortcutHint()
	}
	return m.infoView()
}

func (m *bubbleModel) syncLegacyToComponents() {
	if m.bottom == nil {
		return
	}
	if m.prompt == nil {
		m.prompt = m.bottom.prompt()
	}
	if m.modal != nil && !m.hasPermissionView() {
		m.openPermission(*m.modal)
		if view := m.permissionView(); view != nil {
			view.parked = m.modalParked
			view.index = m.permIndex
		}
	}
}

func (m *bubbleModel) syncComponentsToLegacy() {
	if m.bottom == nil {
		return
	}
	m.prompt = m.bottom.prompt()
	if view := m.slashState(); view != nil {
		m.slashIndex = view.index
	} else {
		m.slashIndex = 0
	}
	if view := m.permissionView(); view != nil {
		pending := view.pending
		m.modal = &pending
		m.modalParked = view.parked
		m.permIndex = view.index
	} else {
		m.modal = nil
		m.modalParked = false
		m.permIndex = 0
	}
}

type toolResultMsg struct {
	call   tool.Call
	result tool.Result
	err    error
}

type turnDeltaMsg struct{ event app.Event }

type turnEventsClosedMsg struct{}

type turnDoneMsg struct {
	result app.Result
	err    error
}

const modelSelectViewID = "model_select"

const maxModelSelectRows = 6

type modelSelectedMsg struct {
	providerName string
	modelID      string
	unverified   bool
	err          error
}

type modelSelectPaneView struct {
	picker         list.Model
	pickerReady    bool
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
	seen := make(map[string]bool)
	if m != nil && len(m.providers) > 0 {
		for name := range m.providers {
			providers = append(providers, name)
			seen[strings.ToLower(name)] = true
		}
		sort.Strings(providers)
	}
	if m != nil && m.activeProvider != "" {
		if !seen[strings.ToLower(m.activeProvider)] {
			providers = append([]string{m.activeProvider}, providers...)
			seen[strings.ToLower(m.activeProvider)] = true
		}
	} else if len(providers) == 0 {
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
	var modelsList []model.RemoteModel
	hasFreshCatalog := false
	if m != nil && providerIdx < len(providers) {
		modelsList, hasFreshCatalog = m.modelCatalogs.freshModels(providers[providerIdx], time.Now(), m.runtimeConfig.ModelCatalogTTL)
	}
	if !hasFreshCatalog {
		modelsList = nil
	}
	delegate := list.NewDefaultDelegate()
	delegate.SetSpacing(0)
	view := &modelSelectPaneView{providerNames: providers, providerIndex: providerIdx}
	view.initPicker(delegate)
	activeModel := ""
	if m != nil {
		activeModel = m.activeModel
	}
	view.setModels(modelsList, activeModel)
	return view
}

type modelListItem struct {
	model        model.RemoteModel
	providerName string
	current      bool
}

func (i modelListItem) FilterValue() string {
	return strings.Join([]string{i.model.ID, i.model.Name, i.model.Provider, strings.Join(i.model.Features, " ")}, " ")
}

func (i modelListItem) Title() string {
	label := strings.TrimSpace(i.model.Name)
	if label == "" {
		label = i.model.ID
	}
	if model.IsFreeModel(i.model.ID) {
		label += " · FREE"
	}
	if i.current {
		label = "✓ " + label
	}
	return label
}

func (i modelListItem) Description() string {
	parts := make([]string, 0, 4)
	if name := strings.TrimSpace(i.model.Name); name != "" && !strings.EqualFold(name, i.model.ID) {
		parts = append(parts, i.model.ID)
	}
	resolved := model.ResolveRemoteMetadata(i.providerName, i.model)
	if limits := formatModelTokenLimits(resolved.Profile.ContextWindow, resolved.Profile.MaxInputTokens, resolved.Profile.MaxOutputTokens); limits != "" {
		parts = append(parts, limits)
	}
	if len(resolved.Features) > 0 {
		parts = append(parts, strings.Join(resolved.Features, " · "))
	}
	if reasoning := remoteModelReasoningSummary(i.providerName, i.model, true); reasoning != "" {
		parts = append(parts, reasoning)
	}
	return strings.Join(parts, " · ")
}

func (v *modelSelectPaneView) initPicker(delegates ...list.DefaultDelegate) {
	if v == nil || v.pickerReady {
		return
	}
	delegate := list.NewDefaultDelegate()
	if len(delegates) > 0 {
		delegate = delegates[0]
	}
	delegate.SetSpacing(0)
	v.picker = list.New(nil, delegate, defaultBubbleWidth-8, defaultBubbleHeight-8)
	v.picker.DisableQuitKeybindings()
	v.picker.SetStatusBarItemName("model", "models")
	v.picker.FilterInput.Prompt = "Search: "
	v.pickerReady = true
}

func (*modelSelectPaneView) ID() string {
	return modelSelectViewID
}

func (*modelSelectPaneView) ReplacesComposer() bool {
	return true
}

func (v *modelSelectPaneView) setModels(models []model.RemoteModel, activeModel string) {
	if v == nil {
		return
	}
	v.initPicker()
	v.allModels = append([]model.RemoteModel(nil), models...)
	items := make([]list.Item, 0, len(v.allModels))
	for _, md := range v.allModels {
		items = append(items, modelListItem{model: md, providerName: v.activeProviderName(), current: strings.EqualFold(md.ID, activeModel)})
	}
	_ = v.picker.SetItems(items)
	v.applyFilter(activeModel)
}

func (v *modelSelectPaneView) applyFilter(activeModel string) {
	if v == nil {
		return
	}
	if strings.TrimSpace(v.filter) == "" {
		v.picker.ResetFilter()
	} else {
		v.picker.SetFilterText(v.filter)
		if v.filtering {
			v.picker.SetFilterState(list.Filtering)
		}
	}
	v.resetSelection(activeModel)
}

func (v *modelSelectPaneView) resetSelection(activeModel string) {
	if v == nil {
		return
	}
	if !v.pickerReady {
		source := v.allModels
		if len(source) == 0 {
			source = v.models
		}
		v.initPicker()
		items := make([]list.Item, 0, len(source))
		for _, md := range source {
			items = append(items, modelListItem{model: md, providerName: v.activeProviderName(), current: strings.EqualFold(md.ID, activeModel)})
		}
		_ = v.picker.SetItems(items)
	}
	v.picker.GoToStart()
	for i, item := range v.picker.VisibleItems() {
		md, ok := item.(modelListItem)
		if ok && strings.EqualFold(md.model.ID, activeModel) {
			v.picker.Select(i)
			break
		}
	}
	v.syncPickerProjection()
}

func (v *modelSelectPaneView) syncPickerProjection() {
	if v == nil {
		return
	}
	visible := v.picker.VisibleItems()
	v.models = make([]model.RemoteModel, 0, len(visible))
	for _, item := range visible {
		if md, ok := item.(modelListItem); ok {
			v.models = append(v.models, md.model)
		}
	}
	v.index = v.picker.Index()
	v.offset = v.picker.Paginator.Page * v.picker.Paginator.PerPage
	v.filter = v.picker.FilterValue()
	v.filtering = v.picker.SettingFilter()
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
	return fetchProviderModelsCmd(providerFetchRequest{ctx: ctx, requestID: v.fetchRequestID, providerName: providerName, providerType: cfg.Type, baseURL: cfg.BaseURL, apiKey: cfg.APIKey, discoveryTimeout: discoveryTimeout})
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
		if model.ProviderHasUsableAuth(providerName, cfg.BaseURL, cfg.APIKey) {
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
	v.initPicker()
	if m == nil {
		return ""
	}
	providerName := v.activeProviderName()
	v.picker.Title = "Select Model · " + providerName
	if len(v.providerNames) > 1 {
		v.picker.Title += " · tab provider"
	}
	v.picker.SetSize(maxInt(12, m.width-8), maxInt(5, min(16, m.height-4)))
	mode := layoutModeForHeight(m.height)
	v.picker.SetShowStatusBar(mode == layoutNormal)
	v.picker.SetShowPagination(mode != layoutTiny)
	v.picker.SetShowHelp(mode != layoutTiny)
	delegate := list.NewDefaultDelegate()
	delegate.SetSpacing(0)
	delegate.ShowDescription = mode == layoutNormal
	v.picker.SetDelegate(delegate)
	if v.loading {
		rows := []string{brandStyle.Render("Select Model · " + providerName), "", mutedStyle.Render("Loading models..."), "", mutedStyle.Render("esc close")}
		return renderModalRows(m, accentAssistant, rows)
	}
	if v.err != nil {
		rows := []string{brandStyle.Render("Select Model · " + providerName), "", errorStyle.Render("Failed to load models"), mutedStyle.Render(truncateWithEllipsis(v.err.Error(), maxInt(8, m.width-8))), "", mutedStyle.Render("r retry · p providers · esc close")}
		return renderModalRows(m, accentAssistant, rows)
	}
	if len(v.picker.Items()) == 0 && !v.picker.SettingFilter() && !v.picker.IsFiltered() {
		rows := []string{brandStyle.Render("Select Model · " + providerName), "", mutedStyle.Render("No models available for the selected provider."), "", mutedStyle.Render("a add provider · r retry · esc close")}
		return renderModalRows(m, accentAssistant, rows)
	}
	if len(v.picker.VisibleItems()) == 0 && strings.TrimSpace(v.picker.FilterValue()) != "" {
		rows := []string{brandStyle.Render("Select Model · " + providerName), mutedStyle.Render("Search: " + v.picker.FilterValue()), "", mutedStyle.Render("No models match the current search."), "", mutedStyle.Render("esc clear filter")}
		return renderModalRows(m, accentAssistant, rows)
	}
	return renderModalRows(m, accentAssistant, strings.Split(v.picker.View(), "\n"))
}

func (v *modelSelectPaneView) HandleKey(m *bubbleModel, message tea.KeyPressMsg) (bool, tea.Cmd) {
	if key.Matches(message, m.keys.ToggleModel) {
		v.cancelFetch()
		m.bottom.remove(modelSelectViewID)
		return true, nil
	}
	v.initPicker()
	if v.picker.SettingFilter() {
		updated, cmd := v.picker.Update(message)
		v.picker = updated
		v.syncPickerProjection()
		return true, cmd
	}
	switch message.String() {
	case "/":
		v.picker.SetFilterState(list.Filtering)
		v.syncPickerProjection()
		return true, nil
	case "ctrl+u":
		v.picker.ResetFilter()
		v.syncPickerProjection()
		return true, nil
	case "esc":
		if v.picker.IsFiltered() {
			v.picker.ResetFilter()
			v.syncPickerProjection()
			return true, nil
		}
		v.cancelFetch()
		m.bottom.remove(modelSelectViewID)
		return true, nil
	case "q":
		v.cancelFetch()
		m.bottom.remove(modelSelectViewID)
		return true, nil
	case "p":
		v.cancelFetch()
		m.bottom.remove(modelSelectViewID)
		if !m.bottom.has(providerSelectViewID) {
			m.bottom.push(newProviderSelectPaneView(m))
		}
		return true, nil
	case "a":
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
	case "shift+tab":
		if len(v.providerNames) > 1 {
			v.providerIndex = (v.providerIndex - 1 + len(v.providerNames)) % len(v.providerNames)
			return true, v.loadProvider(m, false)
		}
		return true, nil
	case "up", "k", "down", "j", "pgup", "pgdown", "home", "g", "end", "G":
		updated, cmd := v.picker.Update(message)
		v.picker = updated
		v.syncPickerProjection()
		return true, cmd
	case "1", "2", "3", "4", "5", "6", "7", "8", "9":
		targetIdx := int(message.String()[0]-'1') + v.offset
		if targetIdx >= 0 && targetIdx < len(v.models) {
			selected := v.models[targetIdx]
			provName := model.DefaultProtonmanName
			if v.providerIndex >= 0 && v.providerIndex < len(v.providerNames) {
				provName = v.providerNames[v.providerIndex]
			}
			cmd := saveDefaultModelCmd(provName, selected.ID)
			m.bottom.remove(modelSelectViewID)
			return true, cmd
		}
		return true, nil
	case "enter":
		if len(v.models) == 0 {
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
		return modelSelectedMsg{providerName: providerName, modelID: modelID, unverified: unverified, err: err}
	}
}

const reasoningViewID = "reasoning"

type reasoningListItem struct {
	effort  sdk.ReasoningEffort
	current bool
}

func (i reasoningListItem) FilterValue() string { return reasoningEffortLabel(i.effort) }
func (i reasoningListItem) Title() string {
	label := reasoningEffortLabel(i.effort)
	if i.current {
		return "✓ " + label
	}
	return label
}
func (i reasoningListItem) Description() string { return reasoningEffortDescription(i.effort) }

type reasoningPaneView struct {
	index   int
	picker  list.Model
	choices []sdk.ReasoningEffort
}

func (*reasoningPaneView) ID() string             { return reasoningViewID }
func (*reasoningPaneView) ReplacesComposer() bool { return true }

func (m *bubbleModel) handleReasoningCommand(argument string) tea.Cmd {
	argument = strings.TrimSpace(argument)
	if argument == "" {
		if !m.bottom.has(reasoningViewID) {
			m.bottom.push(newReasoningPaneView(m))
		}
		m.relayout()
		return nil
	}
	effort, err := sdk.ParseReasoningEffort(argument)
	if err != nil {
		m.appendError("invalid reasoning effort: use auto, none, low, medium, high, xhigh, or max")
		m.refreshViewport()
		return nil
	}
	return m.setReasoningEffort(effort)
}

func (m *bubbleModel) setReasoningEffort(effort sdk.ReasoningEffort) tea.Cmd {
	if effort != sdk.ReasoningDefault {
		profile := m.activeResolvedModelProfile()
		if _, err := profile.ResolveExplicitReasoning(effort); err != nil {
			m.appendError(err.Error())
			m.refreshViewport()
			return nil
		}
	}
	m.reasoningEffort = effort
	m.agents.SetReasoningEffort(effort)
	m.reconfigureRunner()
	m.appendLine(successStyle.Render("Thinking level set to " + reasoningEffortLabel(effort) + " for this session."))
	m.refreshViewport()
	return nil
}

func newReasoningPaneView(m *bubbleModel) *reasoningPaneView {
	choices := reasoningChoices(m.activeResolvedModelProfile())
	if len(choices) == 0 {
		choices = []sdk.ReasoningEffort{sdk.ReasoningDefault}
	}
	items := make([]list.Item, 0, len(choices))
	selected := 0
	for i, effort := range choices {
		current := effort == m.reasoningEffort
		items = append(items, reasoningListItem{effort: effort, current: current})
		if current {
			selected = i
		}
	}
	delegate := list.NewDefaultDelegate()
	delegate.SetSpacing(0)
	picker := list.New(items, delegate, maxInt(20, m.width-8), maxInt(6, minInt(18, m.height-4)))
	picker.DisableQuitKeybindings()
	picker.SetFilteringEnabled(false)
	picker.SetShowStatusBar(false)
	picker.SetShowPagination(false)
	picker.SetStatusBarItemName("level", "levels")
	picker.Select(selected)
	return &reasoningPaneView{index: selected, picker: picker, choices: choices}
}

func reasoningChoices(profile modelprofile.Resolved) []sdk.ReasoningEffort {
	return pane.ReasoningChoices(profile)
}

func (v *reasoningPaneView) Render(m *bubbleModel) string {
	v.picker.SetSize(maxInt(20, m.width-8), maxInt(6, minInt(18, m.height-4)))
	v.picker.Title = "Thinking level"
	if modelName := strings.TrimSpace(m.activeModel); modelName != "" {
		v.picker.Title += " · " + modelName
	}
	mode := layoutModeForHeight(m.height)
	v.picker.SetShowHelp(mode != layoutTiny)
	delegate := list.NewDefaultDelegate()
	delegate.SetSpacing(0)
	delegate.ShowDescription = mode == layoutNormal
	v.picker.SetDelegate(delegate)
	return renderModalRows(m, accentAssistant, strings.Split(v.picker.View(), "\n"))
}

func (v *reasoningPaneView) HandleKey(m *bubbleModel, message tea.KeyPressMsg) (bool, tea.Cmd) {
	if len(v.choices) == 0 {
		return true, nil
	}
	if v.index != v.picker.Index() {
		v.picker.Select(maxInt(0, minInt(v.index, len(v.choices)-1)))
	}
	switch message.String() {
	case "1", "2", "3", "4", "5", "6", "7", "8", "9":
		idx := int(message.String()[0] - '1')
		if idx >= 0 && idx < len(v.choices) {
			m.bottom.remove(reasoningViewID)
			return true, m.setReasoningEffort(v.choices[idx])
		}
		return true, nil
	case "tab", "shift+tab":
		return true, nil
	case "enter":
		idx := v.picker.Index()
		if idx < 0 || idx >= len(v.choices) {
			return true, nil
		}
		m.bottom.remove(reasoningViewID)
		return true, m.setReasoningEffort(v.choices[idx])
	case "esc", "q":
		m.bottom.remove(reasoningViewID)
		return true, nil
	case "up", "k", "down", "j", "home", "g", "end", "G", "pgup", "pgdown":
		updated, cmd := v.picker.Update(message)
		v.picker = updated
		v.index = v.picker.Index()
		return true, cmd
	default:
		return false, nil
	}
}

func reasoningEffortDescription(effort sdk.ReasoningEffort) string {
	return pane.ReasoningEffortDescription(effort)
}

func (m *bubbleModel) activeResolvedModelProfile() modelprofile.Resolved {
	var remote *model.RemoteModel
	if candidate, ok := m.activeRemoteModel(); ok {
		copy := candidate
		remote = &copy
	}
	return model.ResolveModelProfile(m.activeProvider, m.activeModel, remote)
}

func reasoningEffortLabel(effort sdk.ReasoningEffort) string {
	return pane.ReasoningEffortLabel(effort)
}

func remoteModelReasoningSummary(providerName string, md model.RemoteModel, includeDefault bool) string {
	profile := model.ResolveModelProfile(providerName, md.ID, &md)
	supported, known := profile.Reasoning.Support.Bool()
	if !known || !supported {
		return ""
	}
	if len(profile.Reasoning.Levels) == 0 {
		return "reasoning"
	}
	levels := make([]string, 0, len(profile.Reasoning.Levels))
	for _, level := range profile.Reasoning.Levels {
		levels = append(levels, string(level))
	}
	summary := "reasoning " + strings.Join(levels, "/")
	if includeDefault && profile.Reasoning.Default != sdk.ReasoningDefault {
		summary += " (default " + string(profile.Reasoning.Default) + ")"
	}
	return summary
}
