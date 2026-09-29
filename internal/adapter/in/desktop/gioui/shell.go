//go:build desktop || desktop_gio

package gioui

import (
	"image"
	"image/color"
	"strings"
	"time"

	"gioui.org/font"
	"gioui.org/io/key"
	"gioui.org/io/semantic"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/x/markdown"

	desktopstate "github.com/phongsathornpt/protonman/internal/feature/desktop"
)

type shell struct {
	theme *theme

	sidebarList                  layout.List
	sessionButtons               map[string]*widget.Clickable
	projectButtons               map[string]*widget.Clickable
	sessionButtonLive            map[string]struct{}
	projectButtonLive            map[string]struct{}
	buttonRevision               uint64
	buttonRevisionSet            bool
	sidebarRowsCache             sidebarRowsCache
	sidebarVisible               bool
	sidebarToggle                widget.Clickable
	sidebarSearchEditor          widget.Editor
	sidebarSearchClearButton     widget.Clickable
	sidebarClearFilterButton     widget.Clickable
	projectCollapsed             map[string]bool
	sidebarNewSessionButton      widget.Clickable
	emptyNewSessionButton        widget.Clickable
	conversationList             layout.List
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
	conversationThinkingExpanded map[conversationCacheKey]bool
	conversationThinkingButtons  map[conversationCacheKey]*widget.Clickable
	inspectorList                layout.List
	inspectorVisible             bool
	inspectorOverride            bool
	inspectorToggle              widget.Clickable
	activeInspectorTab           int
	inspectorTabButtons          [4]widget.Clickable
	skillToggleButtons           map[string]*widget.Clickable
	skillFilterEditor            widget.Editor
	composer                     widget.Editor
	composerDrafts               map[string]string
	composerDraftOrder           []string
	composerError                string
	activeWorkspace              string
	mentionActive                bool
	mentionDismissed             bool
	mentionDismissedQuery        string
	mentionContext               MentionContext
	mentionStateDirty            bool
	mentionWorkspace             string
	composerHasContent           bool
	mentionItems                 []MentionItem
	mentionSelectedIndex         int
	mentionList                  layout.List
	mentionButtons               map[string]*widget.Clickable
	mentionCache                 workspaceMentionCache
	codeCopyButtons              map[string]*widget.Clickable
	codeCopiedAt                 map[string]time.Time
	messageCopyButtons           map[string]*widget.Clickable
	messageCopiedAt              map[string]time.Time
	userRetryButtons             map[string]*widget.Clickable
	toolExpanded                 map[string]bool
	toolExpandButtons            map[string]*widget.Clickable
	sendButton                   widget.Clickable
	stopButton                   widget.Clickable
	jumpToBottomButton           widget.Clickable
	starterPromptButtons         [4]widget.Clickable
	permissionButtons            map[string]map[string]*widget.Clickable
	permissionButtonLive         map[string]struct{}
	permissionButtonRevision     uint64
	permissionButtonRevisionSet  bool
	questionStates               map[string]*questionInteractionState
	runtimeProviderEditor        widget.Editor
	runtimeModelEditor           widget.Editor
	runtimeApplyButton           widget.Clickable
	reasoningButtons             map[string]*widget.Clickable
	lowConcurrencyButtons        map[string]*widget.Clickable
	runtimeEditorKey             string
	runtimeEditorProvider        string
	runtimeEditorModel           string
	mcpNameEditor                widget.Editor
	mcpCommandEditor             widget.Editor
	mcpArgsEditor                widget.Editor
	mcpEnvEditor                 widget.Editor
	mcpSelectedName              string
	mcpEditorKey                 string
	mcpFormVisible               bool
	mcpIntegrationButtons        map[string]*widget.Clickable
	mcpIntegrationLive           map[string]struct{}
	mcpSyncRevision              uint64
	mcpSyncRevisionSet           bool
	mcpFormToggleButton          widget.Clickable
	mcpSaveButton                widget.Clickable
	mcpRemoveButton              widget.Clickable
	mcpCloseButton               widget.Clickable
	mcpCancelButton              widget.Clickable
	mcpPresetGitHubBtn           widget.Clickable
	mcpPresetMemoryBtn           widget.Clickable
	mcpPresetFilesystemBtn       widget.Clickable
	mcpPresetFetchBtn            widget.Clickable
	mcpPresetCustomBtn           widget.Clickable
	mcpConfirmDelete             bool
	mcpConfirmDeleteBtn          widget.Clickable
	mcpCancelDeleteBtn           widget.Clickable
	mcpReconnectButton           widget.Clickable
	agentSelectorButton          widget.Clickable
	agentSelectorList            layout.List
	agentSelectorVisible         bool
	agentChoiceButtons           map[string]*widget.Clickable
	agentIDEditor                widget.Editor
	agentNameEditor              widget.Editor
	agentCommandEditor           widget.Editor
	agentArgsEditor              widget.Editor
	agentEnvEditor               widget.Editor
	agentProfileButtons          map[string]*widget.Clickable
	agentProfileLive             map[string]struct{}
	agentSyncRevision            uint64
	agentSyncRevisionSet         bool
	agentEditorVisible           bool
	agentEditorOriginalID        string
	agentEditorKey               string
	agentFormToggleButton        widget.Clickable
	agentSaveButton              widget.Clickable
	agentRemoveButton            widget.Clickable
	agentCloseButton             widget.Clickable
	agentCancelButton            widget.Clickable
	agentPresetProtonmanBtn      widget.Clickable
	agentPresetOpencodeBtn       widget.Clickable
	agentPresetClineBtn          widget.Clickable
	agentPresetAntigravityBtn    widget.Clickable
	agentPresetClaudeBtn         widget.Clickable
	agentPresetCustomBtn         widget.Clickable
	agentConfirmDelete           bool
	agentConfirmDeleteBtn        widget.Clickable
	agentCancelDeleteBtn         widget.Clickable
	activeSessionID              string
	syncRevision                 uint64

	sessionMenuButtons        map[string]*widget.Clickable
	sessionMenuLive           map[string]struct{}
	sessionPinButtons         map[string]*widget.Clickable
	sessionQuickRenameButtons map[string]*widget.Clickable
	sessionQuickDeleteButtons map[string]*widget.Clickable
	menuSessionID             string
	menuPinButton             widget.Clickable
	menuRenameButton          widget.Clickable
	menuDeleteButton          widget.Clickable

	editingSessionID    string
	sessionRenameEditor widget.Editor
	renameConfirmButton widget.Clickable
	renameCancelButton  widget.Clickable

	deletingSessionID        string
	deletingSessionTitle     string
	deleteModalScrim         widget.Clickable
	deleteModalCancelButton  widget.Clickable
	deleteModalConfirmButton widget.Clickable

	pinnedCollapsed bool
	pinnedButton    widget.Clickable

	sidebarInspectorButton    widget.Clickable
	sidebarPinnedFilterButton widget.Clickable
	sidebarCommunityButton    widget.Clickable

	settingsModalOpen     bool
	settingsModalScrim    widget.Clickable
	settingsModalCard     widget.Clickable
	settingsModalCloseBtn widget.Clickable
	settingsActiveTab     int
	settingsTabButtons    [3]widget.Clickable
	settingsThemeButtons  map[string]*widget.Clickable
	settingsModalList     layout.List
	onSetTheme            func(string)

	modelChipButton          widget.Clickable
	reasoningChipButton      widget.Clickable
	modelPopoverVisible      bool
	reasoningPopoverVisible  bool
	modelSearchFocusPending  bool
	modelPopoverCloseButton  widget.Clickable
	reasoningPopoverCloseBtn widget.Clickable
	modelSearchEditor        widget.Editor
	modelSearchClearBtn      widget.Clickable
	modelRefreshButton       widget.Clickable
	popoverProviderEditor    widget.Editor
	popoverModelEditor       widget.Editor
	popoverApplyModelButton  widget.Clickable
	modelList                layout.List
	recentModels             []modelPresetRecord
	agentModelButtons        map[string]*widget.Clickable
	modelPresetButtons       map[string]*widget.Clickable
	popoverReasoningButtons  map[string]*widget.Clickable

	onSelectSession        func(string)
	onSelectProject        func(string)
	onNewSession           func()
	onDeleteSession        func(string)
	onRenameSession        func(string, string)
	onTogglePinSession     func(string)
	onToggleSkill          func(string, string)
	onSetFilterMode        func(string)
	onSendPrompt           func(ExpandedPrompt)
	onCancelPrompt         func()
	onResolvePermission    func(string, string)
	onResolveQuestion      func(string, desktopstate.QuestionResponse)
	onSetRuntimeModel      func(string, string)
	onSetRuntimeReasoning  func(string)
	onSetRuntimeLow        func(string)
	onRefreshRuntime       func()
	onSaveMCPIntegration   func(string, string, string, string)
	onRemoveMCPIntegration func(string)
	onReconnectMCP         func()
	onSelectAgent          func(string)
	onSaveAgentProfile     func(string, string, string, string, string, string)
	onRemoveAgentProfile   func(string)
}

type modelPresetRecord struct {
	Provider string
	Model    string
	Name     string
}

var curatedModelPresets = []modelPresetRecord{
	{Provider: "protonman", Model: "claude-3-7-sonnet-20250219", Name: "Claude 3.7 Sonnet"},
	{Provider: "protonman", Model: "claude-3-5-sonnet-20241022", Name: "Claude 3.5 Sonnet"},
	{Provider: "openai", Model: "gpt-4o", Name: "GPT-4o"},
	{Provider: "openai", Model: "o3-mini", Name: "o3-mini"},
	{Provider: "deepseek", Model: "deepseek-reasoner", Name: "DeepSeek-R1"},
}

func newShell(theme *theme) *shell {
	markdownRenderer := markdown.NewRenderer()
	markdownRenderer.Config.DefaultFont = theme.textFont(font.Normal)
	markdownRenderer.Config.DefaultColor = theme.onSurface
	markdownRenderer.Config.InteractiveColor = theme.primary
	return &shell{
		theme:                        theme,
		sessionButtons:               make(map[string]*widget.Clickable),
		projectButtons:               make(map[string]*widget.Clickable),
		sessionButtonLive:            make(map[string]struct{}),
		projectButtonLive:            make(map[string]struct{}),
		sessionMenuButtons:           make(map[string]*widget.Clickable),
		sessionMenuLive:              make(map[string]struct{}),
		sessionPinButtons:            make(map[string]*widget.Clickable),
		sessionQuickRenameButtons:    make(map[string]*widget.Clickable),
		sessionQuickDeleteButtons:    make(map[string]*widget.Clickable),
		sessionRenameEditor:          widget.Editor{SingleLine: true, MaxLen: 256},
		sidebarVisible:               true,
		sidebarSearchEditor:          widget.Editor{SingleLine: true, MaxLen: 128},
		skillFilterEditor:            widget.Editor{SingleLine: true, MaxLen: 128},
		projectCollapsed:             make(map[string]bool),
		sidebarList:                  layout.List{Axis: layout.Vertical},
		conversationList:             layout.List{Axis: layout.Vertical, ScrollToEnd: true},
		inspectorList:                layout.List{Axis: layout.Vertical},
		settingsModalList:            layout.List{Axis: layout.Vertical},
		composer:                     widget.Editor{Submit: true, MaxLen: 1 << 20},
		composerDrafts:               make(map[string]string),
		modelSearchEditor:            widget.Editor{SingleLine: true, MaxLen: 256},
		popoverProviderEditor:        widget.Editor{SingleLine: true, MaxLen: 256},
		popoverModelEditor:           widget.Editor{SingleLine: true, MaxLen: 256},
		modelList:                    layout.List{Axis: layout.Vertical},
		agentModelButtons:            make(map[string]*widget.Clickable),
		modelPresetButtons:           make(map[string]*widget.Clickable),
		popoverReasoningButtons:      make(map[string]*widget.Clickable),
		runtimeProviderEditor:        widget.Editor{SingleLine: true, MaxLen: 512},
		runtimeModelEditor:           widget.Editor{SingleLine: true, MaxLen: 512},
		mcpNameEditor:                widget.Editor{SingleLine: true, MaxLen: 256},
		mcpCommandEditor:             widget.Editor{SingleLine: true, MaxLen: 1024},
		mcpArgsEditor:                widget.Editor{SingleLine: true, MaxLen: 4096},
		mcpEnvEditor:                 widget.Editor{SingleLine: true, MaxLen: 4096},
		agentSelectorList:            layout.List{Axis: layout.Horizontal},
		agentChoiceButtons:           make(map[string]*widget.Clickable),
		agentIDEditor:                widget.Editor{SingleLine: true, MaxLen: 128},
		agentNameEditor:              widget.Editor{SingleLine: true, MaxLen: 256},
		agentCommandEditor:           widget.Editor{SingleLine: true, MaxLen: 1024},
		agentArgsEditor:              widget.Editor{SingleLine: true, MaxLen: 4096},
		agentEnvEditor:               widget.Editor{SingleLine: true, MaxLen: 4096},
		agentProfileButtons:          make(map[string]*widget.Clickable),
		agentProfileLive:             make(map[string]struct{}),
		conversationMarkdown:         markdownRenderer,
		conversationCache:            make(map[conversationCacheKey]conversationMarkdownCache),
		conversationCodeCache:        make(map[conversationCacheKey]conversationCodeCache),
		conversationResponseCache:    make(map[conversationCacheKey]conversationResponseCache),
		mentionStateDirty:            true,
		conversationExpanded:         make(map[conversationCacheKey]bool),
		conversationPage:             make(map[conversationCacheKey]int),
		conversationExpandButtons:    make(map[conversationCacheKey]*conversationDisclosureButtons),
		conversationThinkingExpanded: make(map[conversationCacheKey]bool),
		conversationThinkingButtons:  make(map[conversationCacheKey]*widget.Clickable),
		permissionButtons:            make(map[string]map[string]*widget.Clickable),
		permissionButtonLive:         make(map[string]struct{}),
		questionStates:               make(map[string]*questionInteractionState),
		reasoningButtons:             make(map[string]*widget.Clickable),
		lowConcurrencyButtons:        make(map[string]*widget.Clickable),
		skillToggleButtons:           make(map[string]*widget.Clickable),
		mentionButtons:               make(map[string]*widget.Clickable),
		codeCopyButtons:              make(map[string]*widget.Clickable),
		codeCopiedAt:                 make(map[string]time.Time),
		mcpIntegrationButtons:        make(map[string]*widget.Clickable),
		mcpIntegrationLive:           make(map[string]struct{}),
		onSelectSession:              func(string) {},
		onSelectProject:              func(string) {},
		onNewSession:                 func() {},
		onDeleteSession:              func(string) {},
		onRenameSession:              func(string, string) {},
		onTogglePinSession:           func(string) {},
		onToggleSkill:                func(string, string) {},
		onSetFilterMode:              func(string) {},
		onSendPrompt:                 func(ExpandedPrompt) {},
		onCancelPrompt:               func() {},
		onResolvePermission:          func(string, string) {},
		onResolveQuestion:            func(string, desktopstate.QuestionResponse) {},
		onSetRuntimeModel:            func(string, string) {},
		onSetRuntimeReasoning:        func(string) {},
		onSetRuntimeLow:              func(string) {},
		onSaveMCPIntegration:         func(string, string, string, string) {},
		onRemoveMCPIntegration:       func(string) {},
		onReconnectMCP:               func() {},
		onSelectAgent:                func(string) {},
		onSaveAgentProfile:           func(string, string, string, string, string, string) {},
		onRemoveAgentProfile:         func(string) {},
		settingsThemeButtons:         make(map[string]*widget.Clickable),
		onSetTheme:                   func(string) {},
	}
}

func (s *shell) handleGlobalShortcuts(gtx layout.Context, snapshot controllerSnapshot) {
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
			key.Filter{Name: key.NameEscape},
		)
		if !ok {
			break
		}
		if ev, ok := event.(key.Event); ok && ev.State == key.Press {
			if s.settingsModalOpen && ev.Name != "," && ev.Name != key.NameEscape {
				continue
			}

			switch ev.Name {
			case "B":
				s.sidebarVisible = !s.sidebarVisible
			case "I":
				wideInspector := gtx.Constraints.Max.X >= gtx.Dp(inspectorWideBreakpoint)
				s.inspectorOverride = true
				s.inspectorVisible = !s.shouldShowInspector(wideInspector)
			case "N":
				if snapshot.Connection == connectionConnected && !snapshot.CreatingSession {
					s.onNewSession()
				}
			case "K", "F":
				s.sidebarVisible = true
				gtx.Execute(key.FocusCmd{Tag: &s.sidebarSearchEditor})
			case ",":
				if s.settingsModalOpen {
					s.closeSettingsModal()
					gtx.Execute(key.FocusCmd{Tag: &s.composer})
				} else {
					s.openSettingsModal()
					gtx.Execute(key.FocusCmd{Tag: nil})
				}
			case "M":
				if s.modelPopoverVisible {
					s.closePopovers()
					gtx.Execute(key.FocusCmd{Tag: &s.composer})
				} else {
					s.openModelPopover()
				}
			case "R":
				if s.reasoningPopoverVisible {
					s.closePopovers()
				} else {
					s.openReasoningPopover()
					gtx.Execute(key.FocusCmd{Tag: &s.composer})
				}
			case key.NameEscape:
				if s.settingsModalOpen {
					s.closeSettingsModal()
					gtx.Execute(key.FocusCmd{Tag: &s.composer})
					break
				}
				if s.modelPopoverVisible || s.reasoningPopoverVisible {
					s.closePopovers()
					gtx.Execute(key.FocusCmd{Tag: &s.composer})
				}
			}
		}
	}
}

func (s *shell) layout(gtx layout.Context, snapshot controllerSnapshot) layout.Dimensions {
	s.syncRevision = snapshot.Revision
	s.syncSessionButtons(snapshot.State, snapshot.Revision)
	s.syncConversation(snapshot.State)
	s.syncRuntimeEditors(snapshot.State)
	s.syncMCPIntegrationEditors(snapshot.State)
	s.syncAgentProfileEditors(snapshot)
	s.handleGlobalShortcuts(gtx, snapshot)
	paint.Fill(gtx.Ops, s.theme.surface)

	children := make([]layout.FlexChild, 0, 3)
	if s.sidebarVisible {
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
				if !s.agentSelectorVisible {
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
	if s.deletingSessionID != "" {
		s.layoutDeleteModal(gtx)
	}
	if s.settingsModalOpen {
		s.layoutSettingsModal(gtx, snapshot)
	}
	return dims
}

func (s *shell) layoutVerticalDivider(gtx layout.Context) layout.Dimensions {
	gtx.Constraints.Min.X = gtx.Dp(1)
	gtx.Constraints.Max.X = gtx.Dp(1)
	paint.FillShape(gtx.Ops, s.theme.outlineVariant, clip.Rect{
		Max: image.Pt(gtx.Dp(1), gtx.Constraints.Max.Y),
	}.Op())
	return layout.Dimensions{Size: image.Pt(gtx.Dp(1), gtx.Constraints.Max.Y)}
}

func (s *shell) layoutHorizontalDivider(gtx layout.Context) layout.Dimensions {
	gtx.Constraints.Min.Y = gtx.Dp(1)
	gtx.Constraints.Max.Y = gtx.Dp(1)
	paint.FillShape(gtx.Ops, s.theme.outlineVariant, clip.Rect{
		Max: image.Pt(gtx.Constraints.Max.X, gtx.Dp(1)),
	}.Op())
	return layout.Dimensions{Size: image.Pt(gtx.Constraints.Max.X, gtx.Dp(1))}
}

func (s *shell) layoutTopBar(gtx layout.Context, snapshot controllerSnapshot) layout.Dimensions {
	gtx.Constraints.Min.Y = gtx.Dp(60)
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			paint.FillShape(gtx.Ops, s.theme.surface, clip.Rect{Max: image.Pt(gtx.Constraints.Max.X, gtx.Dp(60))}.Op())
			return desktopInset{Top: 8, Bottom: 8, Left: 12, Right: 12}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return desktopInset{Right: 12}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
							tip := "Show sidebar"
							if s.sidebarVisible {
								tip = "Hide sidebar"
							}
							return s.layoutIconActionButton(gtx, &s.sidebarToggle, iconSidebar, tip, func() {
								s.sidebarVisible = !s.sidebarVisible
							})
						})
					}),
					layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
						session, ok := selectedSession(snapshot.State)
						title, workspace := "New conversation", "Choose a workspace to get started"
						if ok {
							title = strings.TrimSpace(session.Title)
							if title == "" {
								title = "Untitled conversation"
							}
							workspace = strings.TrimSpace(session.WorkspaceName)
							if workspace == "" {
								workspace = strings.TrimSpace(session.Workspace)
							}
							if project := projectDisplayName(snapshot.State, session.ProjectID); project != "" {
								workspace = project
							}
							if workspace == "" {
								workspace = "Workspace"
							}
						}
						return layout.Flex{Axis: layout.Vertical, Alignment: layout.Start}.Layout(gtx,
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								return s.layoutLabel(gtx, compactInspectorText(title, 68), textTitleMedium, font.SemiBold, s.theme.onSurface, 1)
							}),
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								return s.layoutLabel(gtx, compactInspectorText(workspace, 56), textLabelSmall, font.Normal, s.theme.onSurfaceVariant, 1)
							}),
						)
					}),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						wideInspector := gtx.Constraints.Max.X >= gtx.Dp(inspectorWideBreakpoint)
						showInspector := s.shouldShowInspector(wideInspector)
						agentName := compactInspectorText(activeAgentDisplayName(snapshot), 16)
						chevronSuffix := " ▾"
						if s.agentSelectorVisible {
							chevronSuffix = " ▴"
						}
						return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								return desktopInset{Right: 6}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
									return s.layoutAgentCapsuleButton(gtx, &s.agentSelectorButton, agentName+chevronSuffix, func() {
										s.agentSelectorVisible = !s.agentSelectorVisible
									})
								})
							}),
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								return desktopInset{Right: 6}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
									return s.layoutConnectionPill(gtx, snapshot)
								})
							}),
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								return s.layoutIconActionButton(gtx, &s.inspectorToggle, iconInspector, "Toggle inspector", func() {
									s.inspectorOverride = true
									s.inspectorVisible = !showInspector
								})
							}),
						)
					}),
				)
			})
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return s.layoutHorizontalDivider(gtx)
		}),
	)
}

func projectDisplayName(state desktopstate.State, projectID string) string {
	for _, project := range state.Projects {
		if project.ID == projectID {
			return strings.TrimSpace(project.Name)
		}
	}
	return ""
}

func (s *shell) layoutConnectionPill(gtx layout.Context, snapshot controllerSnapshot) layout.Dimensions {
	background := s.theme.successContainer
	foreground := s.theme.onSuccessContainer
	if snapshot.Connection != connectionConnected {
		background = s.theme.warningContainer
		foreground = s.theme.onWarningContainer
	}
	status := strings.ToLower(snapshot.Status)
	if strings.Contains(status, "permission") {
		background = s.theme.warningContainer
		foreground = s.theme.onWarningContainer
	}
	if strings.Contains(status, "failed") || (strings.Contains(status, "unavailable") && !strings.Contains(status, "session list unavailable")) || strings.Contains(status, "disconnected") {
		background = s.theme.errorContainer
		foreground = s.theme.onErrorContainer
	}
	gtx.Constraints.Min.Y = gtx.Dp(28)
	return s.roundedSurface(gtx, shapeSmall, background, func(gtx layout.Context) layout.Dimensions {
		return desktopInset{Top: 4, Bottom: 4, Left: 10, Right: 10}.Layout(gtx,
			func(gtx layout.Context) layout.Dimensions {
				semantic.DescriptionOp(strings.TrimSpace(snapshot.Status)).Add(gtx.Ops)
				return s.layoutLabel(gtx, connectionStatusLabel(snapshot.Status), textLabelMedium, font.SemiBold, foreground, 1)
			},
		)
	})
}

func connectionStatusLabel(status string) string {
	status = strings.TrimSpace(status)
	const projectSelectedPrefix = "Project selected · "
	if strings.HasPrefix(status, projectSelectedPrefix) {
		return compactInspectorText("Project · "+strings.TrimPrefix(status, projectSelectedPrefix), 30)
	}
	lower := strings.ToLower(status)
	switch {
	case strings.Contains(lower, "session list unavailable"):
		return "Connected"
	case strings.Contains(lower, "mcp settings unavailable"):
		return "MCP unavailable"
	case strings.Contains(lower, "history failed"):
		return "History failed"
	case strings.Contains(lower, "prompt failed"):
		return "Prompt failed"
	case strings.Contains(lower, "permission"):
		return "Permission required"
	case strings.Contains(lower, "unavailable"):
		return "Unavailable"
	case strings.Contains(lower, "failed"):
		return "Connection failed"
	case strings.Contains(lower, "disconnected"):
		return "Disconnected"
	case strings.Contains(lower, "reconnecting"):
		return "Reconnecting"
	case strings.Contains(lower, "connecting"):
		return "Connecting"
	case strings.Contains(lower, "connected"):
		return "Connected"
	}
	return compactInspectorText(status, 26)
}

func (s *shell) layoutMain(gtx layout.Context, snapshot controllerSnapshot) layout.Dimensions {
	session, ok := selectedSession(snapshot.State)
	if !ok {
		return s.roundedSurface(gtx, shapeNone, s.theme.surface, func(gtx layout.Context) layout.Dimensions {
			return s.layoutMainEmptyState(gtx, snapshot)
		})
	}
	wideInspector := gtx.Constraints.Max.X >= gtx.Dp(inspectorWideBreakpoint)
	showInspector := s.shouldShowInspector(wideInspector)
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

func (s *shell) layoutConversationPane(gtx layout.Context, session desktopstate.SessionState, snapshot controllerSnapshot) layout.Dimensions {
	children := make([]layout.FlexChild, 0, 4)
	if permission := activePermission(snapshot.State, session.ID); permission != nil {
		children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Stack{Alignment: layout.Center}.Layout(gtx, layout.Stacked(func(gtx layout.Context) layout.Dimensions {
				gtx.Constraints.Max.X = min(gtx.Constraints.Max.X, gtx.Dp(960))
				return desktopInset{Top: 8, Bottom: 8, Left: 16, Right: 16}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					return s.layoutPermissionPanel(gtx, *permission)
				})
			}))
		}))
	}
	if question := activeQuestion(snapshot.State, session.ID); question != nil {
		children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Stack{Alignment: layout.Center}.Layout(gtx, layout.Stacked(func(gtx layout.Context) layout.Dimensions {
				gtx.Constraints.Max.X = min(gtx.Constraints.Max.X, gtx.Dp(960))
				return desktopInset{Top: 8, Bottom: 8, Left: 16, Right: 16}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					return s.layoutQuestionPanel(gtx, *question)
				})
			}))
		}))
	}
	children = append(children,
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			return s.layoutConversation(gtx, session, snapshot.HistoryState)
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return s.layoutComposer(gtx, session, snapshot)
		}),
	)
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx, children...)
}

func (s *shell) layoutSessionHeader(gtx layout.Context, session desktopstate.SessionState, snapshot controllerSnapshot, wideInspector, showInspector bool) layout.Dimensions {
	gtx.Constraints.Min.Y = gtx.Dp(76)
	return s.roundedBorderSurface(gtx, shapeLarge, s.theme.surfaceContainer, s.theme.outlineVariant, 1, func(gtx layout.Context) layout.Dimensions {
		gtx.Constraints.Min.X = gtx.Constraints.Max.X
		return desktopInset{Top: 12, Bottom: 12, Left: 20, Right: 20}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			if gtx.Constraints.Max.X < gtx.Dp(640) {
				return s.layoutCompactSessionHeader(gtx, session, snapshot, showInspector)
			}
			return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
				layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
					return s.layoutSessionHeaderIdentity(gtx, session, snapshot)
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return s.layoutSessionHeaderActions(gtx, session, wideInspector, showInspector)
				}),
			)
		})
	})
}

func (s *shell) layoutCompactSessionHeader(gtx layout.Context, session desktopstate.SessionState, snapshot controllerSnapshot, showInspector bool) layout.Dimensions {
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return s.layoutSessionHeaderIdentity(gtx, session, snapshot)
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return s.layoutSessionHeaderActions(gtx, session, false, showInspector)
		}),
	)
}

func (s *shell) layoutSessionHeaderIdentity(gtx layout.Context, session desktopstate.SessionState, snapshot controllerSnapshot) layout.Dimensions {
	workspace := session.Workspace
	if strings.TrimSpace(workspace) == "" {
		workspace = session.WorkspaceName
	}
	agentID := session.AgentID
	if strings.TrimSpace(agentID) == "" {
		agentID = controllerAgentID
	}
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return s.layoutLabel(gtx, session.Title, textHeadlineSmall, font.SemiBold, s.theme.onSurface, 1)
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return s.layoutLabel(gtx, workspace+" · "+agentDisplayName(snapshot.AgentProfiles, agentID), textBodyMedium, font.Normal, s.theme.onSurfaceVariant, 1)
		}),
	)
}

func (s *shell) layoutSessionHeaderActions(gtx layout.Context, session desktopstate.SessionState, wideInspector, showInspector bool) layout.Dimensions {
	label := "Inspector"
	if wideInspector && showInspector {
		label = "Hide inspector"
	} else if !wideInspector && showInspector {
		label = "Conversation"
	}
	return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			gtx.Constraints.Min.Y = gtx.Dp(36)
			return s.layoutTaskStatus(gtx, displayStatus(session.Status), session.Status)
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return desktopUniformInset(4).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return s.layoutButton(gtx, &s.inspectorToggle, label, true, func() {
					s.inspectorOverride = true
					s.inspectorVisible = !showInspector
				})
			})
		}),
	)
}

func (s *shell) layoutTaskStatus(gtx layout.Context, label string, status desktopstate.TaskStatus) layout.Dimensions {
	background, foreground := s.taskStatusColors(status)
	return s.roundedSurface(gtx, shapeSmall, background, func(gtx layout.Context) layout.Dimensions {
		return desktopInset{Top: 5, Bottom: 5, Left: 8, Right: 8}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return s.layoutLabel(gtx, label, textLabelMedium, font.Medium, foreground, 1)
		})
	})
}

func (s *shell) taskStatusColors(status desktopstate.TaskStatus) (color.NRGBA, color.NRGBA) {
	switch status {
	case desktopstate.TaskRunning:
		return s.theme.primaryContainer, s.theme.onPrimaryContainer
	case desktopstate.TaskWaitingPermission:
		return s.theme.warningContainer, s.theme.onWarningContainer
	case desktopstate.TaskCompleted:
		return s.theme.successContainer, s.theme.onSuccessContainer
	case desktopstate.TaskFailed:
		return s.theme.errorContainer, s.theme.onErrorContainer
	default:
		return s.theme.secondaryContainer, s.theme.onSecondaryContainer
	}
}

func taskStatusFromLabel(label string) desktopstate.TaskStatus {
	return desktopstate.TaskStatus(strings.ReplaceAll(strings.ToLower(strings.TrimSpace(label)), " ", "_"))
}

func (s *shell) layoutMainEmptyState(gtx layout.Context, snapshot controllerSnapshot) layout.Dimensions {
	title, body, ready := mainEmptyStateCopy(snapshot.State)
	return s.layoutCenteredCard(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Vertical, Alignment: layout.Middle}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return s.layoutLabel(gtx, title, textDisplaySmall, font.SemiBold, s.theme.onSurface, 2)
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return desktopUniformInset(10).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					return s.layoutLabel(gtx, body, textBodyLarge, font.Normal, s.theme.onSurfaceVariant, 5)
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
				return desktopUniformInset(10).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					return s.layoutPrimaryButton(gtx, &s.emptyNewSessionButton, label, ready && snapshot.Connection == connectionConnected && !snapshot.CreatingSession, s.onNewSession)
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

func (s *shell) layoutSessionDetails(gtx layout.Context, session desktopstate.SessionState) layout.Dimensions {
	return s.layoutCenteredCard(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return s.layoutLabel(gtx, "Session overview", textTitleMedium, font.SemiBold, s.theme.onSurface, 1)
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return desktopUniformInset(8).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					return s.layoutLabel(gtx, "Your conversation, permissions, and session controls appear here.", textBodyMedium, font.Normal, s.theme.onSurfaceVariant, 4)
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

func (s *shell) layoutDetailRow(gtx layout.Context, label, value string) layout.Dimensions {
	if value == "" {
		value = "Not reported"
	}
	if gtx.Constraints.Max.X-gtx.Constraints.Min.X < gtx.Dp(480) {
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return s.layoutLabel(gtx, label, textLabelMedium, font.SemiBold, s.theme.onSurfaceVariant, 1)
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return s.layoutLabel(gtx, value, textBodyMedium, font.Normal, s.theme.onSurface, 2)
			}),
		)
	}
	return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Start}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			gtx.Constraints.Min.X = gtx.Dp(112)
			return s.layoutLabel(gtx, label, textLabelMedium, font.SemiBold, s.theme.onSurfaceVariant, 2)
		}),
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			return s.layoutLabel(gtx, value, textBodyMedium, font.Normal, s.theme.onSurface, 2)
		}),
	)
}

func (s *shell) layoutCenteredCard(gtx layout.Context, content layout.Widget) layout.Dimensions {
	gtx.Constraints.Max.X = min(gtx.Constraints.Max.X, gtx.Dp(680))
	gtx.Constraints.Min.X = 0
	return layout.Stack{Alignment: layout.Center}.Layout(gtx, layout.Stacked(func(gtx layout.Context) layout.Dimensions {
		gtx.Constraints.Max.X = min(gtx.Constraints.Max.X, gtx.Dp(680))
		return s.roundedSurface(gtx, shapeExtraLarge, s.theme.surfaceContainer, func(gtx layout.Context) layout.Dimensions {
			return desktopInset{Top: 24, Bottom: 24, Left: 28, Right: 28}.Layout(gtx, content)
		})
	}))
}

func (s *shell) layoutButton(gtx layout.Context, button *widget.Clickable, label string, enabled bool, action func()) layout.Dimensions {
	return s.layoutButtonStyle(gtx, button, label, enabled, action, false, false)
}

func (s *shell) layoutPrimaryButton(gtx layout.Context, button *widget.Clickable, label string, enabled bool, action func()) layout.Dimensions {
	return s.layoutButtonStyle(gtx, button, label, enabled, action, true, false)
}

func (s *shell) layoutDangerButton(gtx layout.Context, button *widget.Clickable, label string, enabled bool, action func()) layout.Dimensions {
	return s.layoutButtonStyle(gtx, button, label, enabled, action, false, true)
}

func (s *shell) layoutButtonStyle(gtx layout.Context, button *widget.Clickable, label string, enabled bool, action func(), primary, danger bool) layout.Dimensions {
	if enabled && button.Clicked(gtx) && action != nil {
		action()
		gtx.Execute(op.InvalidateCmd{})
	}
	if !enabled {
		gtx = gtx.Disabled()
	}
	minHeight := gtx.Dp(34)
	inset := desktopInset{Top: 6, Bottom: 6, Left: 12, Right: 12}
	if primary || danger {
		minHeight = gtx.Dp(40)
		inset = desktopInset{Top: 8, Bottom: 8, Left: 16, Right: 16}
	}
	gtx.Constraints.Min.Y = minHeight
	dims := button.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		gtx.Constraints.Min.Y = minHeight
		semantic.Button.Add(gtx.Ops)
		semantic.EnabledOp(gtx.Enabled()).Add(gtx.Ops)
		semantic.DescriptionOp(label).Add(gtx.Ops)
		background := s.theme.secondaryContainer
		foreground := s.theme.onSecondaryContainer
		if primary {
			background = s.theme.primary
			foreground = s.theme.onPrimary
		} else if danger {
			background = s.theme.errorContainer
			foreground = s.theme.onErrorContainer
		}
		if !gtx.Enabled() {
			background = s.theme.surfaceContainerHigh
			foreground = s.theme.onSurfaceVariant
		} else if button.Hovered() && !primary && !danger {
			background = s.theme.primaryContainer
			foreground = s.theme.onPrimaryContainer
		}
		return s.roundedSurface(gtx, shapeMedium, background, func(gtx layout.Context) layout.Dimensions {
			return inset.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				if strings.HasSuffix(label, " ▾") || strings.HasSuffix(label, " ▴") {
					clean := strings.TrimSuffix(strings.TrimSuffix(label, " ▾"), " ▴")
					chevronKind := iconChevronDown
					if strings.HasSuffix(label, " ▴") {
						chevronKind = iconChevronUp
					}
					return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							return s.layoutLabel(gtx, clean, textLabelMedium, font.SemiBold, foreground, 1)
						}),
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							return desktopInset{Left: 5}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
								return s.layoutActionIcon(gtx, chevronKind, 11, foreground)
							})
						}),
					)
				}
				return s.layoutLabel(gtx, label, textLabelLarge, font.SemiBold, foreground, 1)
			})
		})
	})
	if enabled && gtx.Focused(button) {
		widget.Border{Color: s.theme.primary, CornerRadius: shapeMedium, Width: 2}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return layout.Dimensions{Size: dims.Size}
		})
	}
	return dims
}

func (s *shell) layoutIconButton(gtx layout.Context, button *widget.Clickable, icon, tooltip string, action func()) layout.Dimensions {
	kind, ok := desktopIconKind(icon)
	if ok {
		return s.layoutIconActionButton(gtx, button, kind, tooltip, action)
	}
	if button.Clicked(gtx) && action != nil {
		action()
		gtx.Execute(op.InvalidateCmd{})
	}
	size := gtx.Dp(30)
	gtx.Constraints.Min = image.Pt(size, size)
	gtx.Constraints.Max = image.Pt(size, size)
	semantic.Button.Add(gtx.Ops)
	semantic.DescriptionOp(tooltip).Add(gtx.Ops)
	background := color.NRGBA{}
	foreground := s.theme.onSurfaceVariant
	if button.Hovered() {
		background = s.theme.surfaceContainerHigh
		foreground = s.theme.onSurface
	}
	dims := button.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return s.roundedSurface(gtx, shapeSmall, background, func(gtx layout.Context) layout.Dimensions {
			return layout.Stack{Alignment: layout.Center}.Layout(gtx, layout.Stacked(func(gtx layout.Context) layout.Dimensions {
				return s.layoutLabel(gtx, icon, textTitleMedium, font.Normal, foreground, 1)
			}))
		})
	})
	if gtx.Focused(button) {
		widget.Border{Color: s.theme.primary, CornerRadius: shapeSmall, Width: 1}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return layout.Dimensions{Size: dims.Size}
		})
	}
	return dims
}

func desktopIconKind(glyph string) (iconKind, bool) {
	switch glyph {
	case "◧":
		return iconSidebar, true
	case "◨":
		return iconInspector, true
	case "+":
		return iconAdd, true
	case "✕":
		return iconClose, true
	default:
		return 0, false
	}
}

func (s *shell) layoutAgentCapsuleButton(gtx layout.Context, button *widget.Clickable, label string, action func()) layout.Dimensions {
	if button.Clicked(gtx) && action != nil {
		action()
	}
	gtx.Constraints.Min.Y = gtx.Dp(28)
	semantic.Button.Add(gtx.Ops)
	semantic.DescriptionOp(label).Add(gtx.Ops)
	background := s.theme.surfaceContainerHigh
	foreground := s.theme.onSurface
	if button.Hovered() {
		background = s.theme.surfaceContainerHighest
	}
	dims := button.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return s.roundedSurface(gtx, shapeMedium, background, func(gtx layout.Context) layout.Dimensions {
			return desktopInset{Top: 4, Bottom: 4, Left: 10, Right: 10}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				if strings.HasSuffix(label, " ▾") || strings.HasSuffix(label, " ▴") {
					clean := strings.TrimSuffix(strings.TrimSuffix(label, " ▾"), " ▴")
					chevronKind := iconChevronDown
					if strings.HasSuffix(label, " ▴") {
						chevronKind = iconChevronUp
					}
					return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							return s.layoutLabel(gtx, clean, textLabelMedium, font.SemiBold, foreground, 1)
						}),
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							return desktopInset{Left: 4}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
								return s.layoutActionIcon(gtx, chevronKind, 10, foreground)
							})
						}),
					)
				}
				return s.layoutLabel(gtx, label, textLabelMedium, font.SemiBold, foreground, 1)
			})
		})
	})
	if gtx.Focused(button) {
		widget.Border{Color: s.theme.primary, CornerRadius: shapeMedium, Width: 1}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return layout.Dimensions{Size: dims.Size}
		})
	}
	return dims
}

func (s *shell) layoutLabel(gtx layout.Context, value string, size unit.Sp, weight font.Weight, color color.NRGBA, maxLines int) layout.Dimensions {
	material := op.Record(gtx.Ops)
	paint.ColorOp{Color: color}.Add(gtx.Ops)
	return widget.Label{MaxLines: maxLines}.Layout(gtx, s.theme.material.Shaper, s.theme.textFont(weight), size, value, material.Stop())
}

func (s *shell) roundedBorderSurface(gtx layout.Context, radius unit.Dp, background color.NRGBA, borderColor color.NRGBA, borderWidth int, content layout.Widget) layout.Dimensions {
	dims := s.roundedSurface(gtx, radius, background, content)
	if borderWidth > 0 && dims.Size.X > 0 && dims.Size.Y > 0 {
		widget.Border{Color: borderColor, CornerRadius: radius, Width: unit.Dp(borderWidth)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return layout.Dimensions{Size: dims.Size}
		})
	}
	return dims
}

func (s *shell) roundedSurface(gtx layout.Context, radius unit.Dp, background color.NRGBA, content layout.Widget) layout.Dimensions {
	material := op.Record(gtx.Ops)
	dims := content(gtx)
	call := material.Stop()
	cornerRadius := min(gtx.Dp(radius), dims.Size.X/2, dims.Size.Y/2)
	stack := clip.RRect{
		Rect: image.Rectangle{Max: dims.Size},
		SE:   cornerRadius,
		SW:   cornerRadius,
		NW:   cornerRadius,
		NE:   cornerRadius,
	}.Push(gtx.Ops)
	paint.Fill(gtx.Ops, background)
	call.Add(gtx.Ops)
	stack.Pop()
	return dims
}

func selectedSession(state desktopstate.State) (desktopstate.SessionState, bool) {
	for _, session := range state.Sessions {
		if session.ID == state.ActiveSessionID {
			return session, true
		}
	}
	return desktopstate.SessionState{}, false
}
