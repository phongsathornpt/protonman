//go:build desktop || desktop_gio

// Package sidebar owns the desktop sidebar's row projection, Gio state, and
// rendering. The shell supplies immutable frame snapshots, actions, and shared
// visual primitives at the layout boundary.
package sidebar

import (
	"fmt"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"

	"gioui.org/layout"
	"gioui.org/widget"

	"github.com/phongsathornpt/protonman/internal/adapter/in/desktop/gioui/component/uikit"
	"github.com/phongsathornpt/protonman/internal/app"
	desktopstate "github.com/phongsathornpt/protonman/internal/feature/desktop"
)

type RowKind uint8

const (
	ProjectRow RowKind = iota
	SessionRow
	PinnedHeaderRow
)

// pinnedSessionKeyPrefix namespaces the widget/row key of the copy shown in the
// pinned section so it cannot collide with the project-grouped copy.
const pinnedSessionKeyPrefix = "pinned:"

// Row is the render-ready sidebar representation of a project, session, or
// pinned-session heading.
type Row struct {
	Kind           RowKind
	ProjectID      string
	SessionID      string
	SessionKey     string
	AgentID        string
	Title          string
	Subtitle       string
	Status         string
	SessionCount   int
	LastActivityAt time.Time
	Pinned         bool
}

type projectCacheEntry struct {
	id   string
	name string
}

type sessionCacheEntry struct {
	id             string
	key            string
	projectID      string
	agentID        string
	title          string
	subtitle       string
	status         string
	lastActivityAt time.Time
}

type RowsCache struct {
	valid      bool
	revision   uint64
	rows       []Row
	projects   []projectCacheEntry
	sessions   []sessionCacheEntry
	filterMode string
	pinned     []string
}

type displayCache struct {
	valid            bool
	rows             []Row
	sourceRevision   uint64
	collapseRevision uint64
	query            string
	filterMode       string
	pinnedCollapsed  bool
}

// InteractionState stores transient navigation, rename, and delete targets
// owned by the sidebar. Application actions remain callback-driven.
type InteractionState struct {
	EditingSessionID     string
	EditingAgentID       string
	DeletingSessionID    string
	DeletingAgentID      string
	DeletingSessionTitle string
}

// Model is the immutable input used to build the row projection.
type Model struct {
	State         desktopstate.State
	AgentProfiles []app.ACPAgentProfile
	Pinned        []string
	CustomTitles  map[string]string
	FilterMode    string
	Revision      uint64

	// sessionSortAt maps session keys to their sort timestamps. This keeps
	// the active session in place when its activity updates, prevents
	// session positions from swapping when clicked, and allows non-active
	// sessions to re-sort when they receive background activity.
	sessionSortAt map[string]time.Time
}

// Snapshot is the immutable desktop state needed to render the sidebar.
type Snapshot struct {
	State             desktopstate.State
	AgentProfiles     []app.ACPAgentProfile
	PinnedSessions    []string
	CustomTitles      map[string]string
	FilterMode        string
	Revision          uint64
	Connection        string
	Status            string
	CreatingSession   bool
	SettingsModalOpen bool
}

// Actions are shell-owned application operations. The component decides when
// an interaction occurs; the shell supplies the operation that handles it.
type Actions struct {
	SelectSession func(agentID, sessionID string)
	SelectProject func(projectID string)
	NewSession    func()
	DeleteSession func(agentID, sessionID string)
	RenameSession func(agentID, sessionID, title string)
	TogglePin     func(agentID, sessionID string)
	SetFilterMode func(mode string)
	OpenSettings  func()
	OpenCommunity func()
}

func withDefaultActions(actions Actions) Actions {
	if actions.SelectSession == nil {
		actions.SelectSession = func(string, string) {}
	}
	if actions.SelectProject == nil {
		actions.SelectProject = func(string) {}
	}
	if actions.NewSession == nil {
		actions.NewSession = func() {}
	}
	if actions.DeleteSession == nil {
		actions.DeleteSession = func(string, string) {}
	}
	if actions.RenameSession == nil {
		actions.RenameSession = func(string, string, string) {}
	}
	if actions.TogglePin == nil {
		actions.TogglePin = func(string, string) {}
	}
	if actions.SetFilterMode == nil {
		actions.SetFilterMode = func(string) {}
	}
	if actions.OpenSettings == nil {
		actions.OpenSettings = func() {}
	}
	if actions.OpenCommunity == nil {
		actions.OpenCommunity = func() {}
	}
	return actions
}

// Chrome supplies the shared uikit visual surface plus the sidebar's one
// domain-specific primitive. TaskStatus stays here because it renders task
// lifecycle state, which is sidebar projection, not a generic theme primitive.
type Chrome struct {
	uikit.Chrome
	TaskStatus func(layout.Context, string, desktopstate.TaskStatus) layout.Dimensions
}

// ViewInput binds a frame snapshot and shell-provided actions/chrome for one
// layout pass.
type ViewInput struct {
	Snapshot Snapshot
	Actions  Actions
	Chrome   Chrome
}

// Component owns sidebar projection caches, Gio widgets, interaction state,
// and rendering-local caches.
type Component struct {
	rowsCache              RowsCache
	displayCache           displayCache
	projectCollapsed       map[string]bool
	pinnedCollapsed        bool
	collapseRevision       uint64
	list                   layout.List
	searchEditor           widget.Editor
	sessionButtons         map[string]*widget.Clickable
	projectButtons         map[string]*widget.Clickable
	projectNewButtons      map[string]*widget.Clickable
	projectEmptyNewButtons map[string]*widget.Clickable
	sessionButtonLive      map[string]struct{}
	projectButtonLive      map[string]struct{}
	pinButtons             map[string]*widget.Clickable
	renameButtons          map[string]*widget.Clickable
	deleteButtons          map[string]*widget.Clickable
	buttonRevision         uint64
	buttonRevisionSet      bool
	pinnedButton           widget.Clickable
	toggleButton           widget.Clickable
	visible                bool
	interaction            InteractionState
	searchClearButton      widget.Clickable
	clearFilterButton      widget.Clickable
	newSessionButton       widget.Clickable
	inspectorButton        widget.Clickable
	pinnedFilterButton     widget.Clickable
	runningFilterButton    widget.Clickable
	communityButton        widget.Clickable
	renameEditor           widget.Editor
	renameConfirm          widget.Clickable
	renameCancel           widget.Clickable
	deleteScrim            widget.Clickable
	deleteCancel           widget.Clickable
	deleteConfirm          widget.Clickable
	view                   ViewInput
	sessionSortAt          map[string]time.Time
	lastKnownActivity      map[string]time.Time
	lastEnsuredSessionID   string
}

func New() *Component {
	return &Component{
		projectCollapsed:       make(map[string]bool),
		list:                   layout.List{Axis: layout.Vertical},
		searchEditor:           widget.Editor{SingleLine: true, MaxLen: 128},
		sessionButtons:         make(map[string]*widget.Clickable),
		projectButtons:         make(map[string]*widget.Clickable),
		projectNewButtons:      make(map[string]*widget.Clickable),
		projectEmptyNewButtons: make(map[string]*widget.Clickable),
		sessionButtonLive:      make(map[string]struct{}),
		projectButtonLive:      make(map[string]struct{}),
		pinButtons:             make(map[string]*widget.Clickable),
		renameButtons:          make(map[string]*widget.Clickable),
		deleteButtons:          make(map[string]*widget.Clickable),
		sessionSortAt:          make(map[string]time.Time),
		lastKnownActivity:      make(map[string]time.Time),
		visible:                true,
		renameEditor:           widget.Editor{SingleLine: true, MaxLen: 256},
	}
}

func (c *Component) SearchQuery() string { return c.searchEditor.Text() }

func (c *Component) ScrollPosition() layout.Position { return c.list.Position }

func (c *Component) ScrollTo(index int) {
	c.list.Position = layout.Position{First: max(0, index)}
}

func (c *Component) PinnedButton() *widget.Clickable { return &c.pinnedButton }

func (c *Component) ToggleButton() *widget.Clickable { return &c.toggleButton }

func (c *Component) Visible() bool { return c.visible }

func (c *Component) SetVisible(visible bool) { c.visible = visible }

func (c *Component) ToggleVisible() { c.visible = !c.visible }

func (c *Component) SearchClearButton() *widget.Clickable { return &c.searchClearButton }

func (c *Component) ClearFilterButton() *widget.Clickable { return &c.clearFilterButton }

func (c *Component) NewSessionButton() *widget.Clickable { return &c.newSessionButton }

func (c *Component) InspectorButton() *widget.Clickable { return &c.inspectorButton }

func (c *Component) PinnedFilterButton() *widget.Clickable { return &c.pinnedFilterButton }

func (c *Component) RunningFilterButton() *widget.Clickable { return &c.runningFilterButton }

func (c *Component) CommunityButton() *widget.Clickable { return &c.communityButton }

func (c *Component) RenameEditor() *widget.Editor { return &c.renameEditor }

func (c *Component) RenameConfirmButton() *widget.Clickable { return &c.renameConfirm }

func (c *Component) RenameCancelButton() *widget.Clickable { return &c.renameCancel }

func (c *Component) DeleteScrim() *widget.Clickable { return &c.deleteScrim }

func (c *Component) DeleteCancelButton() *widget.Clickable { return &c.deleteCancel }

func (c *Component) DeleteConfirmButton() *widget.Clickable { return &c.deleteConfirm }

func (c *Component) HasDeleteTarget() bool { return c.interaction.DeletingSessionID != "" }

func (c *Component) ProjectButton(projectID string) *widget.Clickable {
	if c.projectButtons[projectID] == nil {
		c.projectButtons[projectID] = new(widget.Clickable)
	}
	return c.projectButtons[projectID]
}

func (c *Component) ProjectNewButton(projectID string) *widget.Clickable {
	if c.projectNewButtons[projectID] == nil {
		c.projectNewButtons[projectID] = new(widget.Clickable)
	}
	return c.projectNewButtons[projectID]
}

func (c *Component) ProjectEmptyNewButton(projectID string) *widget.Clickable {
	if c.projectEmptyNewButtons[projectID] == nil {
		c.projectEmptyNewButtons[projectID] = new(widget.Clickable)
	}
	return c.projectEmptyNewButtons[projectID]
}

func (c *Component) SessionButton(sessionKey string) *widget.Clickable {
	if c.sessionButtons[sessionKey] == nil {
		c.sessionButtons[sessionKey] = new(widget.Clickable)
	}
	return c.sessionButtons[sessionKey]
}

func (c *Component) PinButton(sessionID string, agentID ...string) *widget.Clickable {
	return c.actionButton(c.pinButtons, sessionID, agentID...)
}

func (c *Component) RenameButton(sessionID string, agentID ...string) *widget.Clickable {
	return c.actionButton(c.renameButtons, sessionID, agentID...)
}

func (c *Component) DeleteButton(sessionID string, agentID ...string) *widget.Clickable {
	return c.actionButton(c.deleteButtons, sessionID, agentID...)
}

func (c *Component) actionButton(buttons map[string]*widget.Clickable, sessionID string, agentIDs ...string) *widget.Clickable {
	key := sessionID
	if len(agentIDs) > 0 {
		key = SessionWidgetKey(sessionID, agentIDs[0])
	}
	if buttons[key] == nil {
		buttons[key] = new(widget.Clickable)
	}
	return buttons[key]
}

func (c *Component) HasPinButton(key string) bool { return c.pinButtons[key] != nil }

func (c *Component) HasSessionButton(key string) bool { return c.sessionButtons[key] != nil }

func (c *Component) HasProjectButton(projectID string) bool {
	return c.projectButtons[projectID] != nil
}

func (c *Component) HasProjectNewButton(projectID string) bool {
	return c.projectNewButtons[projectID] != nil
}

func (c *Component) SyncSessionButtons(state desktopstate.State, revision uint64) {
	if revision != 0 && c.buttonRevisionSet && c.buttonRevision == revision {
		return
	}
	clear(c.projectButtonLive)
	for _, project := range state.Projects {
		c.projectButtonLive[project.ID] = struct{}{}
		c.ProjectButton(project.ID)
		c.ProjectNewButton(project.ID)
		c.ProjectEmptyNewButton(project.ID)
	}
	for projectID := range c.projectButtons {
		if _, ok := c.projectButtonLive[projectID]; !ok {
			delete(c.projectButtons, projectID)
		}
	}
	for projectID := range c.projectNewButtons {
		if _, ok := c.projectButtonLive[projectID]; !ok {
			delete(c.projectNewButtons, projectID)
		}
	}
	for projectID := range c.projectEmptyNewButtons {
		if _, ok := c.projectButtonLive[projectID]; !ok {
			delete(c.projectEmptyNewButtons, projectID)
		}
	}
	clear(c.sessionButtonLive)
	for _, session := range state.Sessions {
		key := SessionWidgetKey(session.ID, session.AgentID)
		c.sessionButtonLive[key] = struct{}{}
		c.SessionButton(key)
		c.PinButton(key)
		c.RenameButton(key)
		c.DeleteButton(key)

		pinnedKey := pinnedSessionKeyPrefix + key
		c.sessionButtonLive[pinnedKey] = struct{}{}
		c.SessionButton(pinnedKey)
		c.PinButton(pinnedKey)
		c.RenameButton(pinnedKey)
		c.DeleteButton(pinnedKey)
	}
	pruneButtons := func(buttons map[string]*widget.Clickable) {
		for key := range buttons {
			if _, ok := c.sessionButtonLive[key]; !ok {
				delete(buttons, key)
			}
		}
	}
	pruneButtons(c.sessionButtons)
	pruneButtons(c.pinButtons)
	pruneButtons(c.renameButtons)
	pruneButtons(c.deleteButtons)
	if revision != 0 {
		c.buttonRevision = revision
		c.buttonRevisionSet = true
	}
}

func SessionWidgetKey(sessionID string, agentID ...string) string {
	owner := ""
	if len(agentID) > 0 {
		owner = agentID[0]
	}
	return sessionRefStorageKey(desktopstate.SessionRef{AgentID: owner, SessionID: sessionID})
}

// EnsureVisible adjusts the scroll position so that the specified session is within view.
func (c *Component) EnsureVisible(sessionID string, rows []Row) {
	if sessionID == "" {
		return
	}
	targetIdx := -1
	for i, r := range rows {
		if r.Kind == SessionRow && r.SessionID == sessionID {
			targetIdx = i
			break
		}
	}
	if targetIdx < 0 {
		return
	}
	if targetIdx < c.list.Position.First {
		c.list.Position.First = targetIdx
		c.list.Position.Offset = 0
		return
	}
	const estimatedVisible = 8
	if targetIdx >= c.list.Position.First+estimatedVisible {
		c.list.Position.First = max(0, targetIdx-estimatedVisible+1)
		c.list.Position.Offset = 0
	}
}

// Rows returns the cached sidebar row projection for model.
func (c *Component) Rows(model Model) []Row {
	if c.sessionSortAt == nil {
		c.sessionSortAt = make(map[string]time.Time)
		c.lastKnownActivity = make(map[string]time.Time)
	}
	liveKeys := make(map[string]struct{}, len(model.State.Sessions))
	for _, s := range model.State.Sessions {
		key := SessionWidgetKey(s.ID, s.AgentID)
		liveKeys[key] = struct{}{}
		prevActivity, known := c.lastKnownActivity[key]
		c.lastKnownActivity[key] = s.LastActivityAt

		if !known || c.sessionSortAt[key].IsZero() {
			c.sessionSortAt[key] = s.LastActivityAt
		} else if s.ID == model.State.ActiveSessionID && (model.State.ActiveAgentID == "" || s.AgentID == model.State.ActiveAgentID) {
			// Active session keeps its established sort timestamp so it does not
			// jump or shuffle under the cursor while prompting or typing.
		} else if !s.LastActivityAt.Equal(prevActivity) {
			// Non-active session received new activity: update sort timestamp so
			// it re-sorts by recency.
			c.sessionSortAt[key] = s.LastActivityAt
		}
	}
	for key := range c.sessionSortAt {
		if _, ok := liveKeys[key]; !ok {
			delete(c.sessionSortAt, key)
			delete(c.lastKnownActivity, key)
		}
	}
	if model.sessionSortAt == nil {
		model.sessionSortAt = c.sessionSortAt
	}

	if model.Revision != 0 && c.rowsCache.valid && c.rowsCache.revision == model.Revision {
		return c.rowsCache.rows
	}
	if c.rowsCache.valid && c.rowsCache.matches(model) {
		c.rowsCache.revision = model.Revision
		return c.rowsCache.rows
	}
	c.rowsCache = BuildRows(model)
	c.rowsCache.revision = model.Revision
	return c.rowsCache.rows
}

// DisplayRows applies search, status filters, and expansion state. The
// returned slice is reused while all presentation inputs remain unchanged.
func (c *Component) DisplayRows(rows []Row, query, filterMode string, sourceRevision uint64) []Row {
	query = strings.ToLower(strings.TrimSpace(query))
	filterMode = strings.ToLower(strings.TrimSpace(filterMode))
	cache := &c.displayCache
	if cache.valid && cache.sourceRevision == sourceRevision && cache.collapseRevision == c.collapseRevision &&
		cache.query == query && cache.filterMode == filterMode && cache.pinnedCollapsed == c.pinnedCollapsed {
		return cache.rows
	}

	display := make([]Row, 0, len(rows))
	currentProjectCollapsed := false
	inPinnedSection := false
	for _, row := range rows {
		switch row.Kind {
		case PinnedHeaderRow:
			inPinnedSection = true
			if filterMode != "running" {
				display = append(display, row)
			}
		case ProjectRow:
			inPinnedSection = false
			if filterMode == "pinned" {
				continue
			}
			currentProjectCollapsed = c.projectCollapsed[row.ProjectID]
			display = append(display, row)
		case SessionRow:
			if filterMode == "pinned" && !inPinnedSection {
				continue
			}
			if query == "" && (inPinnedSection && c.pinnedCollapsed || !inPinnedSection && currentProjectCollapsed) {
				continue
			}
			if matchesFilter(row, filterMode) && matchesQuery(row, query) {
				display = append(display, row)
			}
		}
	}

	if query != "" || filterMode == "running" || filterMode == "pinned" {
		cleaned := make([]Row, 0, len(display))
		for i, row := range display {
			if row.Kind != ProjectRow && row.Kind != PinnedHeaderRow {
				cleaned = append(cleaned, row)
				continue
			}
			hasSessions := false
			for j := i + 1; j < len(display); j++ {
				if display[j].Kind == ProjectRow || display[j].Kind == PinnedHeaderRow {
					break
				}
				if display[j].Kind == SessionRow {
					hasSessions = true
					break
				}
			}
			if hasSessions {
				cleaned = append(cleaned, row)
			}
		}
		display = cleaned
	}

	*cache = displayCache{
		valid: true, rows: display, sourceRevision: sourceRevision,
		collapseRevision: c.collapseRevision, query: query,
		filterMode: filterMode, pinnedCollapsed: c.pinnedCollapsed,
	}
	return display
}

func (c *Component) ProjectCollapsed(projectID string) bool {
	return c.projectCollapsed[projectID]
}

func (c *Component) SetProjectCollapsed(projectID string, collapsed bool) {
	if c.projectCollapsed == nil {
		c.projectCollapsed = make(map[string]bool)
	}
	if c.projectCollapsed[projectID] != collapsed {
		c.projectCollapsed[projectID] = collapsed
		c.collapseRevision++
	}
}

func (c *Component) ToggleProject(projectID string) {
	if c.projectCollapsed == nil {
		c.projectCollapsed = make(map[string]bool)
	}
	c.projectCollapsed[projectID] = !c.projectCollapsed[projectID]
	c.collapseRevision++
}

func (c *Component) PinnedCollapsed() bool { return c.pinnedCollapsed }

func (c *Component) SetPinnedCollapsed(collapsed bool) {
	if c.pinnedCollapsed != collapsed {
		c.pinnedCollapsed = collapsed
		c.collapseRevision++
	}
}

func (c *Component) TogglePinned() {
	c.pinnedCollapsed = !c.pinnedCollapsed
	c.collapseRevision++
}

// BuildRows derives the sidebar row projection for model. It is exposed for
// callers and tests that manage their own RowsCache lifecycle; callers holding
// a Component should normally use Rows instead.
func BuildRows(model Model) RowsCache {
	return buildRows(model)
}

func (cache RowsCache) Rows() []Row { return cache.rows }

func (cache RowsCache) Matches(model Model) bool { return cache.matches(model) }

func buildRows(model Model) RowsCache {
	state, profiles := model.State, model.AgentProfiles
	rows := make([]Row, 0, len(state.Sessions)+len(state.Projects)+1)
	projects := make([]projectCacheEntry, 0, len(state.Projects))
	sessions := make([]sessionCacheEntry, 0, len(state.Sessions))
	sessionsByProject := make(map[string][]Row, len(state.Sessions))
	pinnedMap := make(map[string]bool, len(model.Pinned))
	for _, id := range model.Pinned {
		pinnedMap[id] = true
	}
	pinnedRows := make([]Row, 0, len(model.Pinned))
	displayNames := make(map[string]string)
	subtitleFor := func(agentID string) string {
		if name, ok := displayNames[agentID]; ok {
			return name
		}
		name := app.AgentDisplayName(profiles, agentID)
		displayNames[agentID] = name
		return name
	}

	for _, session := range state.Sessions {
		storageKey := sessionRefStorageKey(session.Ref())
		title := strings.TrimSpace(session.Title)
		if custom := customTitle(model.CustomTitles, storageKey, session.ID); custom != "" {
			title = custom
		}
		if title == "" {
			title = "Untitled conversation"
		}
		pinned := pinnedMap[storageKey] || pinnedMap[session.ID]
		subtitle := subtitleFor(session.AgentID)
		sessions = append(sessions, sessionCacheEntry{
			id: session.ID, key: storageKey, projectID: session.ProjectID,
			agentID: session.AgentID, title: title, subtitle: subtitle,
			status: string(session.Status), lastActivityAt: session.LastActivityAt,
		})
		row := Row{
			Kind: SessionRow, ProjectID: session.ProjectID, SessionID: session.ID,
			SessionKey: storageKey, AgentID: session.AgentID, Title: title,
			Subtitle: subtitle, Status: string(session.Status),
			LastActivityAt: session.LastActivityAt, Pinned: pinned,
		}
		sessionsByProject[session.ProjectID] = append(sessionsByProject[session.ProjectID], row)
		if pinned {
			pinnedRow := row
			pinnedRow.SessionKey = pinnedSessionKeyPrefix + storageKey
			pinnedRows = append(pinnedRows, pinnedRow)
		}
	}

	sortKey := func(row Row) time.Time {
		if model.sessionSortAt != nil {
			key := SessionWidgetKey(row.SessionID, row.AgentID)
			if t, ok := model.sessionSortAt[key]; ok {
				return t
			}
			if t, ok := model.sessionSortAt[row.SessionID]; ok {
				return t
			}
		}
		return row.LastActivityAt
	}
	sortSessions := func(sessions []Row) {
		sort.SliceStable(sessions, func(i, j int) bool {
			left, right := sortKey(sessions[i]), sortKey(sessions[j])
			if left.IsZero() && !right.IsZero() {
				return false
			}
			if !left.IsZero() && right.IsZero() {
				return true
			}
			if !left.Equal(right) {
				return left.After(right)
			}
			if sessions[i].SessionID != sessions[j].SessionID {
				return sessions[i].SessionID < sessions[j].SessionID
			}
			return sessions[i].AgentID < sessions[j].AgentID
		})
	}

	if len(pinnedRows) > 0 {
		sortSessions(pinnedRows)
		rows = append(rows, Row{Kind: PinnedHeaderRow, Title: "Pinned", SessionCount: len(pinnedRows)})
		rows = append(rows, pinnedRows...)
	}

	for _, project := range state.Projects {
		projects = append(projects, projectCacheEntry{id: project.ID, name: project.Name})
		projectSessions := sessionsByProject[project.ID]
		sortSessions(projectSessions)
		rows = append(rows, Row{Kind: ProjectRow, ProjectID: project.ID, Title: project.Name, SessionCount: len(projectSessions)})
		rows = append(rows, projectSessions...)
	}
	return RowsCache{
		valid:      true,
		rows:       rows,
		projects:   projects,
		sessions:   sessions,
		pinned:     slices.Clone(model.Pinned),
		filterMode: model.FilterMode,
	}
}

func (cache RowsCache) matches(model Model) bool {
	if cache.filterMode != model.FilterMode || !slices.Equal(cache.pinned, model.Pinned) ||
		len(cache.projects) != len(model.State.Projects) || len(cache.sessions) != len(model.State.Sessions) {
		return false
	}
	for index, project := range model.State.Projects {
		cached := cache.projects[index]
		if cached.id != project.ID || cached.name != project.Name {
			return false
		}
	}
	displayNames := make(map[string]string)
	for index, session := range model.State.Sessions {
		cached := cache.sessions[index]
		storageKey := cached.key
		expectedTitle := strings.TrimSpace(session.Title)
		if custom := customTitle(model.CustomTitles, storageKey, session.ID); custom != "" {
			expectedTitle = custom
		}
		if expectedTitle == "" {
			expectedTitle = "Untitled conversation"
		}
		subtitle, ok := displayNames[session.AgentID]
		if !ok {
			subtitle = app.AgentDisplayName(model.AgentProfiles, session.AgentID)
			displayNames[session.AgentID] = subtitle
		}
		if cached.id != session.ID || cached.projectID != session.ProjectID || cached.agentID != session.AgentID ||
			cached.title != expectedTitle || cached.subtitle != subtitle || cached.status != string(session.Status) ||
			!cached.lastActivityAt.Equal(session.LastActivityAt) {
			return false
		}
	}
	return true
}

func matchesFilter(row Row, filterMode string) bool {
	switch filterMode {
	case "running":
		status := strings.ToLower(row.Status)
		return strings.Contains(status, "running") || strings.Contains(status, "permission") || strings.Contains(status, "approval")
	case "pinned":
		return row.Pinned
	default:
		return true
	}
}

func matchesQuery(row Row, query string) bool {
	if query == "" {
		return true
	}
	return strings.Contains(strings.ToLower(row.Title), query) ||
		strings.Contains(strings.ToLower(row.Subtitle), query) ||
		strings.Contains(strings.ToLower(row.SessionID), query) ||
		strings.Contains(strings.ToLower(row.ProjectID), query)
}

// customTitle resolves a user-assigned session title, preferring the
// agent-scoped storage key and falling back to the bare session ID.
func customTitle(titles map[string]string, storageKey, sessionID string) string {
	custom, ok := titles[storageKey]
	if !ok && storageKey != sessionID {
		custom = titles[sessionID]
	}
	return strings.TrimSpace(custom)
}

func sessionRefStorageKey(ref desktopstate.SessionRef) string {
	if ref.AgentID == "" {
		return ref.SessionID
	}
	key := make([]byte, 0, len(ref.AgentID)+len(ref.SessionID)+42)
	key = strconv.AppendInt(key, int64(len(ref.AgentID)), 10)
	key = append(key, ':')
	key = append(key, ref.AgentID...)
	key = strconv.AppendInt(key, int64(len(ref.SessionID)), 10)
	key = append(key, ':')
	key = append(key, ref.SessionID...)
	return string(key)
}

// StatusLabel maps internal lifecycle values to the sidebar's compact text.
func StatusLabel(status string) string {
	status = strings.ReplaceAll(strings.ToLower(strings.TrimSpace(status)), "_", " ")
	switch status {
	case "", "idle":
		return ""
	case "waiting permission":
		return "Approval"
	case "waiting user":
		return "Your input"
	default:
		return strings.ToUpper(status[:1]) + status[1:]
	}
}

// SessionSubtitle returns the relative activity label for a row if recorded.
func SessionSubtitle(row Row, now time.Time) string {
	if !row.LastActivityAt.IsZero() {
		return ActivityLabel(row.LastActivityAt, now)
	}
	return ""
}

func ActivityLabel(activity, now time.Time) string {
	ago := now.Sub(activity)
	if ago < time.Minute {
		return "Just now"
	}
	if ago < time.Hour {
		return fmt.Sprintf("%dm ago", max(1, int(ago.Minutes())))
	}
	if ago < 24*time.Hour {
		return fmt.Sprintf("%dh ago", max(1, int(ago.Hours())))
	}
	if ago < 7*24*time.Hour {
		return fmt.Sprintf("%dd ago", max(1, int(ago.Hours()/24)))
	}
	return activity.Local().Format("Jan 2")
}
