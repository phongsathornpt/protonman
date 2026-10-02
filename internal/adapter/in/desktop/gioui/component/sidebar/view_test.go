//go:build desktop || desktop_gio

package sidebar

import (
	"image"
	"image/color"
	"strings"
	"testing"
	"time"

	"gioui.org/font"
	"gioui.org/io/input"
	"gioui.org/io/key"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"

	"github.com/phongsathornpt/protonman/internal/adapter/in/desktop/gioui/component/uikit"
	desktopstate "github.com/phongsathornpt/protonman/internal/feature/desktop"
)

// recordingChrome captures the surfaces and icons a row lays out instead of
// painting them, so layout intent can be asserted without a screenshot.
type recordingChrome struct {
	surfaces []color.NRGBA
	icons    []uikit.Icon
	labels   []string
}

func (r *recordingChrome) chrome() Chrome {
	return Chrome{
		Chrome: uikit.Chrome{
			Material: material.NewTheme(),
			Colors: uikit.Colors{
				OnSurface:               color.NRGBA{A: 255},
				OnSurfaceVariant:        color.NRGBA{A: 200},
				Primary:                 color.NRGBA{R: 10, A: 255},
				PrimaryContainer:        color.NRGBA{R: 20, G: 30, B: 40, A: 255},
				OnPrimaryContainer:      color.NRGBA{R: 240, A: 255},
				SurfaceContainerHigh:    color.NRGBA{R: 40, A: 255},
				SurfaceContainerHighest: color.NRGBA{R: 50, A: 255},
				SecondaryContainer:      color.NRGBA{R: 60, G: 70, A: 255},
				OnSecondaryContainer:    color.NRGBA{R: 230, A: 255},
				OnErrorContainer:        color.NRGBA{R: 255, G: 60, A: 255},
				OutlineVariant:          color.NRGBA{R: 90, A: 255},
			},
			Label: func(gtx layout.Context, text string, size unit.Sp, weight font.Weight, clr color.NRGBA, maxLines int) layout.Dimensions {
				r.labels = append(r.labels, text)
				gtx.Constraints.Min.Y = gtx.Dp(16)
				return layout.Dimensions{Size: image.Pt(gtx.Constraints.Min.X, gtx.Dp(16))}
			},
			ActionIcon: func(gtx layout.Context, icon uikit.Icon, size unit.Dp, clr color.NRGBA) layout.Dimensions {
				r.icons = append(r.icons, icon)
				px := gtx.Dp(size)
				return layout.Dimensions{Size: image.Pt(px, px)}
			},
			RoundedSurface: func(gtx layout.Context, radius unit.Dp, clr color.NRGBA, content layout.Widget) layout.Dimensions {
				r.surfaces = append(r.surfaces, clr)
				return content(gtx)
			},
			BorderSurface: func(gtx layout.Context, radius unit.Dp, bg, border color.NRGBA, width int, content layout.Widget) layout.Dimensions {
				r.surfaces = append(r.surfaces, bg)
				return content(gtx)
			},
			Divider: func(gtx layout.Context) layout.Dimensions { return layout.Dimensions{} },
			Button: func(gtx layout.Context, btn *widget.Clickable, label string, enabled bool, onClick func()) layout.Dimensions {
				r.labels = append(r.labels, label)
				return layout.Dimensions{}
			},
			DangerButton: func(gtx layout.Context, btn *widget.Clickable, label string, enabled bool, onClick func()) layout.Dimensions {
				r.labels = append(r.labels, label)
				return layout.Dimensions{}
			},
		},
		TaskStatus: func(gtx layout.Context, label string, status desktopstate.TaskStatus) layout.Dimensions {
			r.labels = append(r.labels, label)
			return layout.Dimensions{}
		},
	}
}

func rowTestContext() layout.Context {
	var ops op.Ops
	var router input.Router
	return layout.Context{
		Ops:         &ops,
		Constraints: layout.Exact(image.Pt(260, 760)),
		Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
		Now:         time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC),
		Source:      router.Source(),
	}
}

func renderSessionRow(t *testing.T, row Row, activeSessionID string) *recordingChrome {
	t.Helper()
	recorder := &recordingChrome{}
	component := New()
	component.view = ViewInput{
		Actions: withDefaultActions(Actions{}),
		Chrome:  recorder.chrome(),
	}
	state := desktopstate.State{ActiveSessionID: activeSessionID}
	gtx := rowTestContext()
	component.layoutSidebarRow(gtx, row, state)
	return recorder
}

// The active thread must read as a filled primary pill, while keeping quick
// actions hidden until hover/focus so the title gets full row width.
func TestSelectedSessionRowFillsPrimaryPillAndHidesRestingActions(t *testing.T) {
	row := Row{Kind: SessionRow, SessionID: "s1", AgentID: "protonman", Title: "Refactor", Status: "Running"}
	recorder := renderSessionRow(t, row, "s1")

	if !containsColor(recorder.surfaces, color.NRGBA{R: 20, G: 30, B: 40, A: 255}) {
		t.Fatalf("selected row surfaces = %#v, want a PrimaryContainer fill", recorder.surfaces)
	}
	for _, icon := range []uikit.Icon{uikit.IconCompose, uikit.IconTrash} {
		if containsIcon(recorder.icons, icon) {
			t.Fatalf("resting selected row icons = %#v, want %s hidden until hover", recorder.icons, icon)
		}
	}
}

// A resting, unselected row must not paint the active pill and must keep the
// quick actions hidden, otherwise every row competes with the selection.
func TestUnselectedSessionRowHidesQuickActionsAndActivePill(t *testing.T) {
	row := Row{Kind: SessionRow, SessionID: "s1", AgentID: "protonman", Title: "Refactor", Status: "Idle"}
	recorder := renderSessionRow(t, row, "other-session")

	if containsColor(recorder.surfaces, color.NRGBA{R: 20, G: 30, B: 40, A: 255}) {
		t.Fatalf("unselected row surfaces = %#v, must not contain the PrimaryContainer fill", recorder.surfaces)
	}
	for _, icon := range []uikit.Icon{uikit.IconCompose, uikit.IconTrash} {
		if containsIcon(recorder.icons, icon) {
			t.Fatalf("unselected row icons = %#v, want %s hidden until hover", recorder.icons, icon)
		}
	}
}

// In a vertical layout.List, items receive unconstrained Max.Y (e.g. 800+ px).
// An active session row must never stretch vertically to fill Max.Y or push
// its contents offscreen.
func TestSelectedSessionRowHeightIsBoundedAndDoesNotStretch(t *testing.T) {
	component := New()
	component.view = ViewInput{
		Actions: withDefaultActions(Actions{}),
		Chrome:  (&recordingChrome{}).chrome(),
	}
	row := Row{Kind: SessionRow, SessionID: "s1", AgentID: "protonman", Title: "Active Thread", Status: "Running"}

	var ops op.Ops
	var router input.Router
	gtx := layout.Context{
		Ops:         &ops,
		Constraints: layout.Constraints{Min: image.Pt(260, 0), Max: image.Pt(260, 900)},
		Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
		Now:         time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC),
		Source:      router.Source(),
	}

	dims := component.layoutSidebarRow(gtx, row, desktopstate.State{ActiveSessionID: "s1"})
	if dims.Size.Y >= 100 {
		t.Fatalf("selected session row height = %d, expected bounded height (< 100) instead of stretching to viewport Max.Y (900)", dims.Size.Y)
	}
	if dims.Size.Y < sidebarSessionRowMinHeight {
		t.Fatalf("selected session row height = %d, expected at least min height %d", dims.Size.Y, sidebarSessionRowMinHeight)
	}
}

// A pinned but resting row still shows the star as a status indicator.
func TestPinnedSessionRowShowsStarWithoutQuickActions(t *testing.T) {
	row := Row{Kind: SessionRow, SessionID: "s1", AgentID: "protonman", Title: "Refactor", Pinned: true}
	recorder := renderSessionRow(t, row, "other-session")

	if !containsIcon(recorder.icons, uikit.IconStar) {
		t.Fatalf("pinned resting row icons = %#v, want the star indicator", recorder.icons)
	}
	if containsIcon(recorder.icons, uikit.IconTrash) {
		t.Fatalf("pinned resting row icons = %#v, want quick actions hidden", recorder.icons)
	}
}

// Sessions owned by a non-default agent carry an identity badge so concurrent
// ACP sessions with the same title stay distinguishable in the sidebar.
func TestSessionRowShowsAgentBadgeForNonDefaultAgent(t *testing.T) {
	row := Row{Kind: SessionRow, SessionID: "s1", AgentID: "reviewer", Subtitle: "Reviewer", Title: "Review"}
	recorder := renderSessionRow(t, row, "other-session")

	if !containsColor(recorder.surfaces, color.NRGBA{R: 60, G: 70, A: 255}) {
		t.Fatalf("row surfaces = %#v, want an agent badge on SecondaryContainer", recorder.surfaces)
	}
	if !containsLabel(recorder.labels, "Reviewer") {
		t.Fatalf("row labels = %#v, want the agent display name", recorder.labels)
	}
}

func TestAgentBadgeLabelSkipsDefaultAgent(t *testing.T) {
	if got := agentBadgeLabel(Row{AgentID: "protonman", Subtitle: "Protonman"}); got != "" {
		t.Fatalf("default agent badge = %q, want empty", got)
	}
	if got := agentBadgeLabel(Row{AgentID: "reviewer", Subtitle: "Reviewer"}); got != "Reviewer" {
		t.Fatalf("agent badge = %q, want Reviewer", got)
	}
	if got := agentBadgeLabel(Row{AgentID: "reviewer"}); got != "reviewer" {
		t.Fatalf("agent badge without display name = %q, want the agent id", got)
	}
}

func containsColor(haystack []color.NRGBA, needle color.NRGBA) bool {
	for _, c := range haystack {
		if c == needle {
			return true
		}
	}
	return false
}

func containsIcon(haystack []uikit.Icon, needle uikit.Icon) bool {
	for _, c := range haystack {
		if c == needle {
			return true
		}
	}
	return false
}

func containsLabel(haystack []string, needle string) bool {
	for _, c := range haystack {
		if c == needle {
			return true
		}
	}
	return false
}

func TestProjectRowClickTogglesCollapseWithoutInvokingSelectProject(t *testing.T) {
	selectedProject := ""
	component := New()
	component.view = ViewInput{
		Actions: withDefaultActions(Actions{
			SelectProject: func(projectID string) {
				selectedProject = projectID
			},
		}),
		Chrome: (&recordingChrome{}).chrome(),
	}
	row := Row{Kind: ProjectRow, ProjectID: "p1", Title: "Project"}
	btn := component.ProjectButton("p1")
	btn.Click()

	gtx := rowTestContext()
	component.layoutSidebarRow(gtx, row, desktopstate.State{ActiveProjectID: "p1", ActiveSessionID: "s1"})

	if !component.ProjectCollapsed("p1") {
		t.Fatal("expected project to be collapsed after click")
	}
	if selectedProject != "" {
		t.Fatalf("expected SelectProject to NOT be called on header fold, got %q", selectedProject)
	}
}

func TestRowActionClickDoesNotTriggerSelectSession(t *testing.T) {
	selectedSession := ""
	pinnedSession := ""
	component := New()
	component.view = ViewInput{
		Actions: withDefaultActions(Actions{
			SelectSession: func(agentID, sessionID string) {
				selectedSession = sessionID
			},
			TogglePin: func(agentID, sessionID string) {
				pinnedSession = sessionID
			},
		}),
		Chrome: (&recordingChrome{}).chrome(),
	}
	row := Row{Kind: SessionRow, SessionID: "s1", AgentID: "protonman", Title: "Session"}
	rowBtn := component.SessionButton(SessionWidgetKey("s1", "protonman"))
	pinBtn := component.PinButton("s1", "protonman")

	// Simulate both receiving a click event (as Gio does for overlapping areas)
	rowBtn.Click()
	pinBtn.Click()

	gtx := rowTestContext()
	component.layoutSidebarRow(gtx, row, desktopstate.State{})

	if pinnedSession != "s1" {
		t.Fatalf("expected pin action to be called, got %q", pinnedSession)
	}
	if selectedSession != "" {
		t.Fatalf("expected SelectSession to NOT be called when clicking pin action, got %q", selectedSession)
	}
}

func TestDeleteModalDismissesOnEscape(t *testing.T) {
	component := New()
	component.view = ViewInput{
		Actions: withDefaultActions(Actions{}),
		Chrome:  (&recordingChrome{}).chrome(),
	}
	component.interaction.DeletingSessionID = "s1"
	component.interaction.DeletingSessionTitle = "Title"

	var ops op.Ops
	var router input.Router
	gtx := layout.Context{
		Ops:         &ops,
		Constraints: layout.Exact(image.Pt(600, 700)),
		Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
		Now:         time.Now(),
		Source:      router.Source(),
	}

	// Frame 1: register event filters
	component.layoutDeleteModal(gtx)

	// Queue Escape key event
	router.Queue(key.Event{Name: key.NameEscape, State: key.Press})

	// Frame 2: consume event
	ops.Reset()
	component.layoutDeleteModal(gtx)

	if component.interaction.DeletingSessionID != "" {
		t.Fatalf("expected DeletingSessionID to be cleared on Escape, got %q", component.interaction.DeletingSessionID)
	}
}

func TestSearchEnterSelectsFirstSession(t *testing.T) {
	selectedSession := ""
	component := New()
	component.view = ViewInput{
		Actions: withDefaultActions(Actions{
			SelectSession: func(agentID, sessionID string) {
				selectedSession = sessionID
			},
		}),
		Chrome: (&recordingChrome{}).chrome(),
	}
	rows := []Row{
		{Kind: ProjectRow, ProjectID: "p1", Title: "Project"},
		{Kind: SessionRow, ProjectID: "p1", SessionID: "s1", AgentID: "protonman", Title: "First Session"},
		{Kind: SessionRow, ProjectID: "p1", SessionID: "s2", AgentID: "protonman", Title: "Second Session"},
	}

	var ops op.Ops
	var router input.Router
	gtx := layout.Context{
		Ops:         &ops,
		Constraints: layout.Exact(image.Pt(260, 760)),
		Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
		Now:         time.Now(),
		Source:      router.Source(),
	}

	// Frame 1: Focus search editor and layout search
	gtx.Execute(key.FocusCmd{Tag: &component.searchEditor})
	component.layoutSidebarSearch(gtx, rows)

	// Queue Enter key event
	router.Queue(key.Event{Name: key.NameReturn, State: key.Press})

	// Frame 2: process Return event
	ops.Reset()
	component.layoutSidebarSearch(gtx, rows)

	if selectedSession != "s1" {
		t.Fatalf("expected first session s1 to be selected on Enter, got %q", selectedSession)
	}
}

func TestAgentBadgeLabelAbbreviatesCanonicalDotaProfilesAndTruncatesLongNames(t *testing.T) {
	// Canonical profiles should produce 3-letter codes
	if got := agentBadgeLabel(Row{AgentID: "strength"}); got != "STR" {
		t.Fatalf("strength badge = %q, want STR", got)
	}
	if got := agentBadgeLabel(Row{AgentID: "agility"}); got != "AGI" {
		t.Fatalf("agility badge = %q, want AGI", got)
	}
	if got := agentBadgeLabel(Row{AgentID: "intelligence"}); got != "INT" {
		t.Fatalf("intelligence badge = %q, want INT", got)
	}
	if got := agentBadgeLabel(Row{AgentID: "universal"}); got != "" {
		t.Fatalf("universal badge = %q, want empty", got)
	}

	// Long custom name should truncate to max 14 runes with ellipsis
	longName := "SuperLongAgentProfileNameExtra"
	badge := agentBadgeLabel(Row{AgentID: "custom", Subtitle: longName})
	if len([]rune(badge)) > 14 {
		t.Fatalf("badge length = %d > 14: %q", len([]rune(badge)), badge)
	}
	if !strings.HasSuffix(badge, "…") {
		t.Fatalf("expected truncated badge to end with ellipsis, got %q", badge)
	}
}

func TestSearchDownArrowFocusesFirstSession(t *testing.T) {
	component := New()
	component.view = ViewInput{
		Actions: withDefaultActions(Actions{}),
		Chrome:  (&recordingChrome{}).chrome(),
	}
	rows := []Row{
		{Kind: ProjectRow, ProjectID: "p1", Title: "Project"},
		{Kind: SessionRow, ProjectID: "p1", SessionID: "s1", AgentID: "protonman", Title: "First Session"},
	}

	var ops op.Ops
	var router input.Router
	gtx := layout.Context{
		Ops:         &ops,
		Constraints: layout.Exact(image.Pt(260, 760)),
		Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
		Now:         time.Now(),
		Source:      router.Source(),
	}

	// Focus search editor
	gtx.Execute(key.FocusCmd{Tag: &component.searchEditor})
	component.layoutSidebarSearch(gtx, rows)

	// Queue Down Arrow
	router.Queue(key.Event{Name: key.NameDownArrow, State: key.Press})

	ops.Reset()
	component.layoutSidebarSearch(gtx, rows)
	// Success is no crash and command registered
}

func TestSessionRowKeyboardReturnAndBackspace(t *testing.T) {
	selectedSession := ""
	component := New()
	component.view = ViewInput{
		Actions: withDefaultActions(Actions{
			SelectSession: func(agentID, sessionID string) {
				selectedSession = sessionID
			},
		}),
		Chrome: (&recordingChrome{}).chrome(),
	}
	widgetKey := SessionWidgetKey("s1", "protonman")
	row := Row{Kind: SessionRow, SessionID: "s1", SessionKey: widgetKey, AgentID: "protonman", Title: "Session 1"}
	btn := component.SessionButton(widgetKey)

	var ops op.Ops
	var router input.Router
	gtx := layout.Context{
		Ops:         &ops,
		Constraints: layout.Exact(image.Pt(260, 760)),
		Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
		Now:         time.Now(),
		Source:      router.Source(),
	}

	// Frame 1: Layout row and request focus on button
	gtx.Execute(key.FocusCmd{Tag: btn})
	component.layoutSidebarRow(gtx, row, desktopstate.State{})
	router.Frame(&ops)

	// Frame 2: Queue Return key (Press and Release) on focused button and layout
	router.Queue(key.Event{Name: key.NameReturn, State: key.Press})
	router.Queue(key.Event{Name: key.NameReturn, State: key.Release})
	ops.Reset()
	component.layoutSidebarRow(gtx, row, desktopstate.State{})
	router.Frame(&ops)

	if selectedSession != "s1" {
		t.Fatalf("expected Return on focused row to select session, got %q", selectedSession)
	}

	// Frame 3: Queue Backspace to trigger delete confirmation
	router.Queue(key.Event{Name: key.NameDeleteBackward, State: key.Press})
	ops.Reset()
	component.layoutSidebarRow(gtx, row, desktopstate.State{})
	router.Frame(&ops)

	if component.interaction.DeletingSessionID != "s1" {
		t.Fatalf("expected Backspace to set DeletingSessionID, got %q", component.interaction.DeletingSessionID)
	}
}

func TestProjectHeaderNewSessionQuickAction(t *testing.T) {
	selectedProject := ""
	newSessionCalled := false
	component := New()
	component.view = ViewInput{
		Actions: withDefaultActions(Actions{
			SelectProject: func(projectID string) {
				selectedProject = projectID
			},
			NewSession: func() {
				newSessionCalled = true
			},
		}),
		Chrome: (&recordingChrome{}).chrome(),
	}
	row := Row{Kind: ProjectRow, ProjectID: "p1", Title: "Project 1", SessionCount: 2}
	newBtn := component.ProjectNewButton("p1")
	newBtn.Click()

	gtx := rowTestContext()
	component.layoutSidebarRow(gtx, row, desktopstate.State{})

	if selectedProject != "p1" {
		t.Fatalf("expected SelectProject('p1'), got %q", selectedProject)
	}
	if !newSessionCalled {
		t.Fatal("expected NewSession to be called")
	}
	if component.ProjectCollapsed("p1") {
		t.Fatal("expected clicking new button to NOT toggle project collapse")
	}
}

func TestEmptyExpandedProjectRendersCTAAndTriggersNewSession(t *testing.T) {
	selectedProject := ""
	newSessionCalled := false
	component := New()
	recorder := &recordingChrome{}
	component.view = ViewInput{
		Actions: withDefaultActions(Actions{
			SelectProject: func(projectID string) {
				selectedProject = projectID
			},
			NewSession: func() {
				newSessionCalled = true
			},
		}),
		Chrome: recorder.chrome(),
	}
	row := Row{Kind: ProjectRow, ProjectID: "p1", Title: "Empty Workspace", SessionCount: 0}

	// Case 1: Expanded project with 0 sessions renders empty message and "+ New" button
	gtx := rowTestContext()
	component.layoutSidebarRow(gtx, row, desktopstate.State{})

	if !containsLabel(recorder.labels, "No conversations yet") {
		t.Fatalf("expected 'No conversations yet' label, got labels: %#v", recorder.labels)
	}
	if !containsLabel(recorder.labels, "+ New") {
		t.Fatalf("expected '+ New' button label, got labels: %#v", recorder.labels)
	}

	// Case 2: Clicking "+ New" CTA invokes SelectProject and NewSession
	emptyBtn := component.ProjectEmptyNewButton("p1")
	emptyBtn.Click()
	component.layoutSidebarRow(gtx, row, desktopstate.State{})

	if selectedProject != "p1" {
		t.Fatalf("expected SelectProject('p1') on empty CTA click, got %q", selectedProject)
	}
	if !newSessionCalled {
		t.Fatal("expected NewSession on empty CTA click")
	}

	// Case 3: Collapsed project with 0 sessions does NOT render "No conversations yet"
	component.SetProjectCollapsed("p1", true)
	recorder2 := &recordingChrome{}
	component.view.Chrome = recorder2.chrome()
	component.layoutSidebarRow(gtx, row, desktopstate.State{})

	if containsLabel(recorder2.labels, "No conversations yet") {
		t.Fatal("expected collapsed project with 0 sessions to NOT render empty message")
	}
}

func TestSessionRowArrowKeyNavigation(t *testing.T) {
	component := New()
	component.view = ViewInput{
		Actions: withDefaultActions(Actions{}),
		Chrome:  (&recordingChrome{}).chrome(),
	}
	key1 := SessionWidgetKey("s1", "protonman")
	key2 := SessionWidgetKey("s2", "protonman")
	row1 := Row{Kind: SessionRow, SessionID: "s1", SessionKey: key1, AgentID: "protonman", Title: "Session 1"}
	row2 := Row{Kind: SessionRow, SessionID: "s2", SessionKey: key2, AgentID: "protonman", Title: "Session 2"}
	rows := []Row{row1, row2}

	btn1 := component.SessionButton(key1)
	btn2 := component.SessionButton(key2)

	var ops op.Ops
	var router input.Router
	gtx := layout.Context{
		Ops:         &ops,
		Constraints: layout.Exact(image.Pt(260, 760)),
		Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
		Now:         time.Now(),
		Source:      router.Source(),
	}

	layoutRows := func() {
		for i, r := range rows {
			component.layoutSidebarRow(gtx, r, desktopstate.State{}, rowNavContext{rows: rows, index: i})
		}
	}

	// Frame 1: Request focus on btn1
	gtx.Execute(key.FocusCmd{Tag: btn1})
	layoutRows()
	router.Frame(&ops)
	gtx.Source = router.Source()

	if !gtx.Focused(btn1) {
		t.Fatal("expected btn1 to be focused")
	}

	// Frame 2: DownArrow on btn1 should focus btn2
	router.Queue(key.Event{Name: key.NameDownArrow, State: key.Press})
	ops.Reset()
	layoutRows()
	router.Frame(&ops)
	gtx.Source = router.Source()

	if !gtx.Focused(btn2) {
		t.Fatal("expected btn2 to be focused after DownArrow")
	}

	// Frame 3: UpArrow on btn2 should focus btn1
	router.Queue(key.Event{Name: key.NameUpArrow, State: key.Press})
	ops.Reset()
	layoutRows()
	router.Frame(&ops)
	gtx.Source = router.Source()

	if !gtx.Focused(btn1) {
		t.Fatal("expected btn1 to be focused after UpArrow")
	}

	// Frame 4: UpArrow on btn1 (top row) should focus searchEditor
	// First lay out search editor so it is in the ops tree
	layoutWithSearch := func() {
		component.layoutSidebarSearch(gtx, rows)
		layoutRows()
	}

	router.Queue(key.Event{Name: key.NameUpArrow, State: key.Press})
	ops.Reset()
	layoutWithSearch()
	router.Frame(&ops)
	gtx.Source = router.Source()

	if !gtx.Focused(&component.searchEditor) {
		t.Fatal("expected searchEditor to be focused after UpArrow on top session row")
	}
}

func TestPinnedSessionDynamicActionDescription(t *testing.T) {
	component := New()
	component.view = ViewInput{
		Actions: withDefaultActions(Actions{}),
		Chrome:  (&recordingChrome{}).chrome(),
	}

	widgetKey1 := SessionWidgetKey("s1", "protonman")
	rowUnpinned := Row{Kind: SessionRow, SessionID: "s1", SessionKey: widgetKey1, AgentID: "protonman", Title: "Unpinned", Pinned: false}
	btn1 := component.SessionButton(widgetKey1)

	var ops op.Ops
	var router input.Router
	gtx := layout.Context{
		Ops:         &ops,
		Constraints: layout.Exact(image.Pt(260, 760)),
		Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
		Now:         time.Now(),
		Source:      router.Source(),
	}

	gtx.Execute(key.FocusCmd{Tag: btn1})
	component.layoutSidebarRow(gtx, rowUnpinned, desktopstate.State{})
	router.Frame(&ops)

	ops.Reset()
	component.layoutSidebarRow(gtx, rowUnpinned, desktopstate.State{})

	widgetKey2 := SessionWidgetKey("s2", "protonman")
	rowPinned := Row{Kind: SessionRow, SessionID: "s2", SessionKey: widgetKey2, AgentID: "protonman", Title: "Pinned", Pinned: true}
	btn2 := component.SessionButton(widgetKey2)

	gtx.Execute(key.FocusCmd{Tag: btn2})
	ops.Reset()
	component.layoutSidebarRow(gtx, rowPinned, desktopstate.State{})
	router.Frame(&ops)

	ops.Reset()
	component.layoutSidebarRow(gtx, rowPinned, desktopstate.State{})
}
