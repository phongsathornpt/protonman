//go:build desktop || desktop_gio

package shell

import (
	"fmt"
	"image"
	"image/color"
	"strings"
	"time"

	"gioui.org/font"
	"gioui.org/io/key"
	"gioui.org/layout"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/widget"
	"gioui.org/x/markdown"

	conversationcomponent "github.com/phongsathornpt/protonman/internal/adapter/in/desktop/gioui/component/conversation"
	inspectorcomponent "github.com/phongsathornpt/protonman/internal/adapter/in/desktop/gioui/component/inspector"
	runtimecomponent "github.com/phongsathornpt/protonman/internal/adapter/in/desktop/gioui/component/runtime"
	settingscomponent "github.com/phongsathornpt/protonman/internal/adapter/in/desktop/gioui/component/settings"
	sidebarcomponent "github.com/phongsathornpt/protonman/internal/adapter/in/desktop/gioui/component/sidebar"
	"github.com/phongsathornpt/protonman/internal/base/modelcatalogpolicy"
	desktopstate "github.com/phongsathornpt/protonman/internal/feature/desktop"

	"github.com/phongsathornpt/protonman/internal/adapter/in/desktop/gioui/component/uikit"
	"github.com/phongsathornpt/protonman/internal/adapter/in/desktop/gioui/controller"
)

// Shell is the Gio desktop host: it owns the window layout, the widget trees it
// renders into, and the per-frame caches. It holds no application logic; every
// user action leaves through Bindings.
type Shell struct {
	theme *theme

	// bind carries the application actions the Shell invokes. It is supplied by
	// the composition root and defaults to no-ops so a partially wired Shell is
	// still safe to render.
	bind Bindings

	// onSetThemeNow re-points derived render state (the markdown renderer) at a
	// replacement palette. Set once during construction.
	onSetThemeNow func(*Theme)

	sidebarData                  *sidebarcomponent.Component
	emptyNewSessionButton        widget.Clickable
	conversationUI               *conversationcomponent.Component
	sessionScrollPositions       uikit.BoundedCache[desktopstate.SessionRef, layout.Position]
	conversationMarkdown         *markdown.Renderer
	conversationCache            map[conversationCacheKey]conversationMarkdownCache
	conversationCacheBytes       int
	conversationCacheOrder       []conversationCacheKey
	conversationCodeCache        map[conversationCacheKey]conversationCodeCache
	conversationCodeCacheBytes   int
	conversationCodeCacheOrder   []conversationCacheKey
	conversationResponseCache    map[conversationCacheKey]conversationResponseCache
	conversationResponseBytes    int
	conversationResponseOrder    []conversationCacheKey
	conversationExpanded         map[conversationCacheKey]bool
	conversationPage             map[conversationCacheKey]int
	conversationExpandButtons    map[conversationCacheKey]*conversationDisclosureButtons
	toolOutputPages              map[conversationCacheKey]*toolOutputPageButtons
	expansionOrder               uikit.BoundedCache[conversationCacheKey, struct{}]
	conversationDescriptions     uikit.BoundedCache[conversationCacheKey, descriptionCacheEntry]
	conversationThinkingExpanded map[conversationCacheKey]bool
	conversationThinkingButtons  map[conversationCacheKey]*widget.Clickable
	conversationThinkingCache    uikit.BoundedCache[conversationCacheKey, thinkingParseCacheEntry]
	toolDiffCache                uikit.BoundedCache[string, toolDiffCacheEntry]
	inspectorComponent           *inspectorcomponent.Component
	inspectorToggle              widget.Clickable
	composerError                string
	activeWorkspace              string
	mentionActive                bool
	mentionDismissed             bool
	mentionDismissedQuery        string
	mentionContext               conversationcomponent.MentionContext
	mentionStateDirty            bool
	mentionWorkspace             string
	composerHasContent           bool
	mentionItems                 []conversationcomponent.MentionItem
	mentionSelectedIndex         int
	mentionCache                 workspaceMentionCache
	codeCopyButtons              map[string]*widget.Clickable
	codeCopiedAt                 map[string]time.Time
	messageCopyButtons           map[string]*widget.Clickable
	messageCopiedAt              map[string]time.Time
	userRetryButtons             map[string]*widget.Clickable
	toolExpanded                 map[string]bool
	toolExpandButtons            map[string]*widget.Clickable
	permissionButtons            map[string]map[string]*widget.Clickable
	permissionButtonLive         map[string]struct{}
	permissionButtonRevision     uint64
	permissionButtonRevisionSet  bool
	permissionRawToggles         map[string]bool
	permissionRawClickable       map[string]*widget.Clickable
	permissionCopyButtons        map[string]*widget.Clickable
	questionStates               map[string]*questionInteractionState
	activeSessionID              string
	activeSessionAgentID         string
	syncRevision                 uint64

	settingsComponent *settingscomponent.Component
	runtimeComponent  *runtimecomponent.Component

	tailFollowBeforeOverlay bool
	recentModels            []modelPresetRecord

	// Settings Providers tab widgets
	backgroundAttentionButton widget.Clickable
}

type modelPresetRecord = runtimecomponent.ModelPresetRecord

// curatedModelPresets derives its quick-pick models from the shared catalog
// policy instead of restating model identifiers here.
var curatedModelPresets = func() []modelPresetRecord {
	presets := modelcatalogpolicy.CuratedPresets()
	records := make([]modelPresetRecord, 0, len(presets))
	for _, preset := range presets {
		records = append(records, modelPresetRecord(preset))
	}
	return records
}()

// New builds the Shell for a resolved theme and a set of application actions.
// Bindings are defaulted to no-ops, so a caller that wires only some actions
// still gets a Shell that renders and handles input safely.
func New(theme *Theme, bind Bindings) *Shell {
	shell := &Shell{
		theme:                        theme,
		bind:                         bind.withDefaults(),
		sidebarData:                  sidebarcomponent.New(),
		conversationUI:               conversationcomponent.New(maxComposerDrafts),
		sessionScrollPositions:       uikit.NewBoundedCache[desktopstate.SessionRef, layout.Position](maxComposerDrafts),
		inspectorComponent:           inspectorcomponent.New(),
		settingsComponent:            settingscomponent.New(),
		runtimeComponent:             runtimecomponent.New(),
		conversationMarkdown:         newMarkdownRenderer(theme),
		conversationCache:            make(map[conversationCacheKey]conversationMarkdownCache),
		conversationCodeCache:        make(map[conversationCacheKey]conversationCodeCache),
		conversationResponseCache:    make(map[conversationCacheKey]conversationResponseCache),
		mentionStateDirty:            true,
		conversationExpanded:         make(map[conversationCacheKey]bool),
		conversationPage:             make(map[conversationCacheKey]int),
		conversationExpandButtons:    make(map[conversationCacheKey]*conversationDisclosureButtons),
		expansionOrder:               uikit.NewBoundedCache[conversationCacheKey, struct{}](maxConversationExpansionKeys),
		conversationThinkingExpanded: make(map[conversationCacheKey]bool),
		conversationThinkingButtons:  make(map[conversationCacheKey]*widget.Clickable),
		conversationThinkingCache:    uikit.NewBoundedCache[conversationCacheKey, thinkingParseCacheEntry](maxThinkingParseCacheEntries),
		toolDiffCache:                uikit.NewBoundedCache[string, toolDiffCacheEntry](maxToolDiffCacheEntries),
		conversationDescriptions:     uikit.NewBoundedCache[conversationCacheKey, descriptionCacheEntry](maxConversationDescriptionCacheEntries),
		permissionButtons:            make(map[string]map[string]*widget.Clickable),
		permissionButtonLive:         make(map[string]struct{}),
		permissionRawToggles:         make(map[string]bool),
		permissionRawClickable:       make(map[string]*widget.Clickable),
		permissionCopyButtons:        make(map[string]*widget.Clickable),
		questionStates:               make(map[string]*questionInteractionState),
		codeCopyButtons:              make(map[string]*widget.Clickable),
		codeCopiedAt:                 make(map[string]time.Time),
	}
	shell.onSetThemeNow = func(next *Theme) {
		shell.conversationMarkdown.Config.DefaultFont = next.TextFont(font.Normal)
		shell.conversationMarkdown.Config.DefaultColor = next.Colors.OnSurface
		shell.conversationMarkdown.Config.InteractiveColor = next.Colors.Primary
	}
	return shell
}

// newMarkdownRenderer builds the transcript renderer bound to a palette.
func newMarkdownRenderer(theme *Theme) *markdown.Renderer {
	renderer := markdown.NewRenderer()
	renderer.Config.DefaultFont = theme.TextFont(font.Normal)
	renderer.Config.DefaultColor = theme.Colors.OnSurface
	renderer.Config.InteractiveColor = theme.Colors.Primary
	return renderer
}

func (s *Shell) handleGlobalShortcuts(gtx layout.Context, snapshot controller.Snapshot) {
	for {
		event, ok := gtx.Event(
			key.Filter{Name: "B", Required: key.ModShortcut},
			key.Filter{Name: "I", Required: key.ModShortcut},
			key.Filter{Name: "N", Required: key.ModShortcut},
			key.Filter{Name: "K", Required: key.ModShortcut},
			key.Filter{Name: "F", Required: key.ModShortcut},
			key.Filter{Name: ",", Required: key.ModShortcut},
			key.Filter{Name: "M", Required: key.ModAlt},
			key.Filter{Name: "R", Required: key.ModAlt},
			key.Filter{Name: "P", Required: key.ModAlt},
			key.Filter{Name: key.NameEscape},
		)
		if !ok {
			break
		}
		if ev, ok := event.(key.Event); ok && ev.State == key.Press {
			if s.settingsComponent.IsOpen() && ev.Name != "," && ev.Name != key.NameEscape {
				continue
			}

			switch ev.Name {
			case "B":
				s.sidebarData.ToggleVisible()
			case "I":
				wideInspector := gtx.Constraints.Max.X >= gtx.Dp(inspectorcomponent.WideBreakpoint)
				s.inspectorComponent.Toggle(wideInspector)
			case "N":
				if snapshot.Connection == controller.ConnectionConnected && !snapshot.CreatingSession {
					s.bind.NewSession()
				}
			case "K", "F":
				s.sidebarData.SetVisible(true)
				s.sidebarData.FocusSearch(gtx)
			case ",":
				if s.settingsComponent.IsOpen() {
					s.closeSettingsModal()
					gtx.Execute(key.FocusCmd{Tag: s.conversationUI.Editor()})
				} else {
					s.openSettingsModal()
					gtx.Execute(key.FocusCmd{Tag: nil})
				}
			case "M":
				if s.runtimeComponent.Widgets().ModelPopoverVisible {
					s.closePopovers()
					gtx.Execute(key.FocusCmd{Tag: s.conversationUI.Editor()})
				} else {
					s.openModelPopover()
				}
			case "R":
				if s.runtimeComponent.Widgets().ReasoningPopoverVisible {
					s.closePopovers()
				} else {
					s.openReasoningPopover()
					gtx.Execute(key.FocusCmd{Tag: s.conversationUI.Editor()})
				}
			case "P":
				if s.runtimeComponent.Widgets().PermissionModePopoverVisible {
					s.closePopovers()
				} else {
					s.openPermissionModePopover()
					gtx.Execute(key.FocusCmd{Tag: s.conversationUI.Editor()})
				}
			case key.NameEscape:
				if s.settingsComponent.IsOpen() {
					s.closeSettingsModal()
					gtx.Execute(key.FocusCmd{Tag: s.conversationUI.Editor()})
					break
				}
				if s.runtimeComponent.Widgets().ModelPopoverVisible || s.runtimeComponent.Widgets().ReasoningPopoverVisible || s.runtimeComponent.Widgets().PermissionModePopoverVisible {
					s.closePopovers()
					gtx.Execute(key.FocusCmd{Tag: s.conversationUI.Editor()})
				}
			}
		}
	}
}

// SetTheme swaps the resolved palette and discards render state that was
// measured against the old one, so the next frame is laid out from scratch.
func (s *Shell) SetTheme(theme *Theme) {
	if theme == nil || theme == s.theme {
		return
	}
	s.theme = theme
	s.conversationCacheBytes = 0
	s.conversationCodeCacheBytes = 0
	s.conversationResponseBytes = 0
	s.onSetThemeNow(theme)
}

// Layout renders one frame from the controller snapshot.
func (s *Shell) Layout(gtx layout.Context, snapshot controller.Snapshot) layout.Dimensions {
	s.syncRevision = snapshot.Revision
	s.syncSessionButtons(snapshot.State, snapshot.Revision)
	s.syncConversation(snapshot.State)
	s.syncRuntimeEditors(snapshot.State)
	s.syncMCPIntegrationEditors(snapshot.State)
	s.syncAgentProfileEditors(snapshot)
	s.handleGlobalShortcuts(gtx, snapshot)
	paint.Fill(gtx.Ops, s.theme.Colors.Surface)

	children := make([]layout.FlexChild, 0, 3)
	if s.sidebarData.Visible() {
		children = append(children,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return s.layoutSidebar(gtx, snapshot)
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return s.layoutVerticalDivider(gtx)
			}),
		)
	}
	children = append(children, layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return s.layoutTopBar(gtx, snapshot)
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				if !s.settingsComponent.AgentSelectorVisible() {
					return layout.Dimensions{}
				}
				return s.layoutAgentSelectorBar(gtx, snapshot)
			}),
			layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
				return s.layoutMain(gtx, snapshot)
			}),
		)
	}))

	dims := layout.Flex{Axis: layout.Horizontal}.Layout(gtx, children...)
	if s.sidebarData.HasDeleteTarget() {
		s.sidebarData.LayoutDeleteModal(gtx, s.sidebarViewInput(snapshot))
	}
	if s.settingsComponent.IsOpen() {
		s.layoutSettingsModal(gtx, snapshot)
	}
	return dims
}

func (s *Shell) layoutVerticalDivider(gtx layout.Context) layout.Dimensions {
	gtx.Constraints.Min.X = gtx.Dp(1)
	gtx.Constraints.Max.X = gtx.Dp(1)
	paint.FillShape(gtx.Ops, s.theme.Colors.OutlineVariant, clip.Rect{
		Max: image.Pt(gtx.Dp(1), gtx.Constraints.Max.Y),
	}.Op())
	return layout.Dimensions{Size: image.Pt(gtx.Dp(1), gtx.Constraints.Max.Y)}
}

func (s *Shell) layoutHorizontalDivider(gtx layout.Context) layout.Dimensions {
	gtx.Constraints.Min.Y = gtx.Dp(1)
	gtx.Constraints.Max.Y = gtx.Dp(1)
	paint.FillShape(gtx.Ops, s.theme.Colors.OutlineVariant, clip.Rect{
		Max: image.Pt(gtx.Constraints.Max.X, gtx.Dp(1)),
	}.Op())
	return layout.Dimensions{Size: image.Pt(gtx.Constraints.Max.X, gtx.Dp(1))}
}

func (s *Shell) layoutMain(gtx layout.Context, snapshot controller.Snapshot) layout.Dimensions {
	session, ok := selectedSession(snapshot.State)
	if !ok {
		return s.roundedSurface(gtx, shapeNone, s.theme.Colors.Surface, func(gtx layout.Context) layout.Dimensions {
			return s.layoutMainEmptyState(gtx, snapshot)
		})
	}
	wideInspector := gtx.Constraints.Max.X >= gtx.Dp(inspectorcomponent.WideBreakpoint)
	showInspector := s.inspectorComponent.ShouldShow(wideInspector)
	if showInspector && wideInspector {
		return layout.Flex{Axis: layout.Horizontal}.Layout(gtx,
			layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
				return s.layoutConversationPane(gtx, session, snapshot)
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return s.layoutVerticalDivider(gtx)
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return s.layoutInspector(gtx, session, snapshot, false)
			}),
		)
	} else if showInspector {
		return s.layoutInspector(gtx, session, snapshot, true)
	} else {
		return s.layoutConversationPane(gtx, session, snapshot)
	}
}

func (s *Shell) layoutConversationPane(gtx layout.Context, session desktopstate.SessionState, snapshot controller.Snapshot) layout.Dimensions {
	children := make([]layout.FlexChild, 0, 5)

	if banner := s.layoutBackgroundAttentionBanner(gtx, snapshot.State, session.AgentID, session.ID); banner.Size.Y > 0 {
		children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return banner
		}))
	}

	children = append(children, layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
		return s.layoutConversation(gtx, session, snapshot.HistoryState)
	}))

	if permission := activePermission(snapshot.State, session.ID, session.AgentID); permission != nil {
		children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			if gtx.Constraints.Max.X > 0 {
				gtx.Constraints.Min.X = gtx.Constraints.Max.X
			}
			return layout.Stack{Alignment: layout.Center}.Layout(gtx, layout.Stacked(func(gtx layout.Context) layout.Dimensions {
				gtx.Constraints.Min = image.Point{}
				gtx.Constraints.Max.X = min(gtx.Constraints.Max.X, gtx.Dp(960))
				return uikit.Inset{Top: 4, Bottom: 4, Left: 16, Right: 16}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					return s.layoutPermissionPanel(gtx, snapshot.State, *permission)
				})
			}))
		}))
	}

	if question := activeQuestion(snapshot.State, session.ID, session.AgentID); question != nil {
		children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			if gtx.Constraints.Max.X > 0 {
				gtx.Constraints.Min.X = gtx.Constraints.Max.X
			}
			return layout.Stack{Alignment: layout.Center}.Layout(gtx, layout.Stacked(func(gtx layout.Context) layout.Dimensions {
				gtx.Constraints.Min = image.Point{}
				gtx.Constraints.Max.X = min(gtx.Constraints.Max.X, gtx.Dp(960))
				return uikit.Inset{Top: 4, Bottom: 4, Left: 16, Right: 16}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					return s.layoutQuestionPanel(gtx, snapshot.State, *question)
				})
			}))
		}))
	}

	children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
		return s.layoutComposer(gtx, session, snapshot)
	}))

	return layout.Flex{Axis: layout.Vertical, Alignment: layout.Middle}.Layout(gtx, children...)
}

func (s *Shell) layoutBackgroundAttentionBanner(gtx layout.Context, state desktopstate.State, currentAgentID, currentSessionID string) layout.Dimensions {
	var targetAgentID, targetSessionID, targetTitle, targetKind string
	for _, p := range state.PermissionInbox {
		if p.SessionID != currentSessionID || p.AgentID != "" && p.AgentID != currentAgentID {
			targetAgentID = p.AgentID
			targetSessionID = p.SessionID
			targetTitle = p.Title
			targetKind = "permission"
			break
		}
	}
	if targetSessionID == "" {
		for _, q := range state.QuestionInbox {
			if q.SessionID != currentSessionID || q.AgentID != "" && q.AgentID != currentAgentID {
				targetAgentID = q.AgentID
				targetSessionID = q.SessionID
				if len(q.Questions) > 0 {
					targetTitle = q.Questions[0].Question
				} else {
					targetTitle = "question"
				}
				targetKind = "question"
				break
			}
		}
	}
	if targetSessionID == "" {
		return layout.Dimensions{}
	}
	if targetAgentID == "" {
		if session, ok := controller.SessionByID(state, targetSessionID); ok {
			targetAgentID = session.AgentID
		}
	}

	sessionName := controller.SessionTitle(state, targetSessionID)
	if s.backgroundAttentionButton.Clicked(gtx) {
		s.bind.SelectSession(targetAgentID, targetSessionID)
	}

	return layout.Stack{Alignment: layout.Center}.Layout(gtx, layout.Stacked(func(gtx layout.Context) layout.Dimensions {
		gtx.Constraints.Min.X = 0
		gtx.Constraints.Max.X = min(gtx.Constraints.Max.X, gtx.Dp(960))
		return uikit.Inset{Top: 6, Bottom: 4, Left: 16, Right: 16}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return s.roundedBorderSurface(gtx, shapeSmall, s.theme.Colors.WarningContainer, s.theme.Colors.OutlineVariant, 1, func(gtx layout.Context) layout.Dimensions {
				return uikit.Inset{Top: 6, Bottom: 6, Left: 12, Right: 12}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle, Spacing: layout.SpaceBetween}.Layout(gtx,
						layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
							msg := fmt.Sprintf("⚠️ Session %q is waiting for %s: %s", sessionName, targetKind, targetTitle)
							return s.layoutLabel(gtx, msg, textBodySmall, font.Medium, s.theme.Colors.OnWarningContainer, 1)
						}),
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							return uikit.Inset{Left: 8}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
								return s.layoutButton(gtx, &s.backgroundAttentionButton, "Jump to session", true, func() {
									s.bind.SelectSession(targetAgentID, targetSessionID)
								})
							})
						}),
					)
				})
			})
		})
	}))
}

func (s *Shell) taskStatusColors(status desktopstate.TaskStatus) (color.NRGBA, color.NRGBA) {
	switch status {
	case desktopstate.TaskRunning:
		return s.theme.Colors.PrimaryContainer, s.theme.Colors.OnPrimaryContainer
	case desktopstate.TaskWaitingPermission, desktopstate.TaskWaitingUser:
		return s.theme.Colors.WarningContainer, s.theme.Colors.OnWarningContainer
	case desktopstate.TaskCompleted:
		return s.theme.Colors.SuccessContainer, s.theme.Colors.OnSuccessContainer
	case desktopstate.TaskFailed:
		return s.theme.Colors.ErrorContainer, s.theme.Colors.OnErrorContainer
	default:
		return s.theme.Colors.SecondaryContainer, s.theme.Colors.OnSecondaryContainer
	}
}

func taskStatusFromLabel(label string) desktopstate.TaskStatus {
	return desktopstate.TaskStatus(strings.ReplaceAll(strings.ToLower(strings.TrimSpace(label)), " ", "_"))
}

func (s *Shell) layoutMainEmptyState(gtx layout.Context, snapshot controller.Snapshot) layout.Dimensions {
	title, body, ready := mainEmptyStateCopy(snapshot.State)
	return s.layoutCenteredCard(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Vertical, Alignment: layout.Middle}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return s.layoutLabel(gtx, title, textDisplaySmall, font.SemiBold, s.theme.Colors.OnSurface, 2)
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return uikit.UniformInset(10).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					return s.layoutLabel(gtx, body, textBodyLarge, font.Normal, s.theme.Colors.OnSurfaceVariant, 5)
				})
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				if len(snapshot.State.Sessions) != 0 {
					return layout.Dimensions{}
				}
				label := "New conversation"
				if snapshot.CreatingSession {
					label = "Creating…"
				}
				return uikit.UniformInset(10).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					return s.layoutPrimaryButton(gtx, &s.emptyNewSessionButton, label, ready && snapshot.Connection == controller.ConnectionConnected && !snapshot.CreatingSession, s.bind.NewSession)
				})
			}),
		)
	})
}

func mainEmptyStateCopy(state desktopstate.State) (title, body string, ready bool) {
	if len(state.Projects) == 0 {
		return "Start with a project", "Choose an existing project from the sidebar to begin. Projects are created from workspace conversations; adding a new project or folder isn’t available in this screen yet.", false
	}
	if state.ActiveProjectID == "" {
		return "Select a project", "1. Choose a project in the sidebar.\n2. Start a conversation to begin working. Your conversations stay with the agent that started them.", false
	}
	return "Your workspace is ready", "Start a conversation to begin working. Your conversations stay with the agent that started them.", true
}

func (s *Shell) layoutSessionDetails(gtx layout.Context, session desktopstate.SessionState) layout.Dimensions {
	return s.layoutCenteredCard(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return s.layoutLabel(gtx, "Session overview", textTitleMedium, font.SemiBold, s.theme.Colors.OnSurface, 1)
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return uikit.UniformInset(8).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					return s.layoutLabel(gtx, "Your conversation, permissions, and session controls appear here.", textBodyMedium, font.Normal, s.theme.Colors.OnSurfaceVariant, 4)
				})
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return s.layoutDetailRow(gtx, "Workspace", session.Workspace)
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return s.layoutDetailRow(gtx, "Workspace key", session.WorkspaceKey)
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return s.layoutDetailRow(gtx, "Agent", session.AgentID)
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return s.layoutDetailRow(gtx, "Session ID", session.ID)
			}),
		)
	})
}

func (s *Shell) layoutDetailRow(gtx layout.Context, label, value string) layout.Dimensions {
	if value == "" {
		value = "Not reported"
	}
	if gtx.Constraints.Max.X-gtx.Constraints.Min.X < gtx.Dp(480) {
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return s.layoutLabel(gtx, label, textLabelMedium, font.SemiBold, s.theme.Colors.OnSurfaceVariant, 1)
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return s.layoutLabel(gtx, value, textBodyMedium, font.Normal, s.theme.Colors.OnSurface, 2)
			}),
		)
	}
	return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Start}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			gtx.Constraints.Min.X = gtx.Dp(112)
			return s.layoutLabel(gtx, label, textLabelMedium, font.SemiBold, s.theme.Colors.OnSurfaceVariant, 2)
		}),
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			return s.layoutLabel(gtx, value, textBodyMedium, font.Normal, s.theme.Colors.OnSurface, 2)
		}),
	)
}

func selectedSession(state desktopstate.State) (desktopstate.SessionState, bool) {
	for _, session := range state.Sessions {
		if session.ID == state.ActiveSessionID {
			return session, true
		}
	}
	return desktopstate.SessionState{}, false
}
