//go:build desktop || desktop_gio

package gioui

import (
	"fmt"
	"image"
	"image/color"
	"os/exec"
	"runtime"
	"sort"
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
	"gioui.org/widget/material"
	"gioui.org/x/markdown"

	"github.com/phongsathornpt/protonman/internal/app"
	desktopstate "github.com/phongsathornpt/protonman/internal/feature/desktop"
)

type sidebarRowKind uint8

const (
	sidebarProjectRow sidebarRowKind = iota
	sidebarSessionRow
	sidebarPinnedHeaderRow
)

type sidebarRow struct {
	Kind           sidebarRowKind
	ProjectID      string
	SessionID      string
	AgentID        string
	Title          string
	Subtitle       string
	Status         string
	SessionCount   int
	LastActivityAt time.Time
	Pinned         bool
}

type sidebarProjectCache struct {
	id   string
	name string
}

type sidebarSessionCache struct {
	id             string
	projectID      string
	agentID        string
	title          string
	subtitle       string
	status         string
	lastActivityAt time.Time
	pinned         bool
}

type sidebarRowsCache struct {
	valid      bool
	rows       []sidebarRow
	projects   []sidebarProjectCache
	sessions   []sidebarSessionCache
	filterMode string
	pinnedHash string
}

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

	filterDropdownOpen   bool
	filterDropdownButton widget.Clickable
	filterAllButton      widget.Clickable
	filterRunningButton  widget.Clickable
	filterPinnedButton   widget.Clickable

	pinnedCollapsed bool
	pinnedButton    widget.Clickable

	sidebarInspectorButton widget.Clickable
	sidebarArchiveButton   widget.Clickable
	sidebarCommunityButton widget.Clickable

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

func (s *shell) layoutSidebar(gtx layout.Context, snapshot controllerSnapshot) layout.Dimensions {
	width := unit.Dp(260)
	if gtx.Constraints.Max.X < gtx.Dp(900) {
		width = 220
	}
	gtx.Constraints.Min.X = gtx.Dp(width)
	gtx.Constraints.Max.X = gtx.Dp(width)
	paint.FillShape(gtx.Ops, s.theme.surfaceDim, clip.Rect{
		Max: image.Pt(gtx.Dp(width), gtx.Constraints.Max.Y),
	}.Op())
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return s.layoutSidebarHeader(gtx, snapshot)
		}),
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			allRows := s.sidebarRows(snapshot)
			rows := s.sidebarDisplayRows(allRows, snapshot.FilterMode)
			if len(rows) == 0 {
				emptyText := "No conversations yet. Start one from an existing workspace."
				hasFilter := false
				if strings.TrimSpace(s.sidebarSearchEditor.Text()) != "" {
					emptyText = "No conversations matching \"" + s.sidebarSearchEditor.Text() + "\""
					hasFilter = true
				} else if snapshot.FilterMode == "running" {
					emptyText = "No running conversations."
					hasFilter = true
				} else if snapshot.FilterMode == "pinned" {
					emptyText = "No pinned conversations. Click ⋮ on a session to pin it."
					hasFilter = true
				}
				if s.sidebarClearFilterButton.Clicked(gtx) {
					s.sidebarSearchEditor.SetText("")
					if s.onSetFilterMode != nil {
						s.onSetFilterMode("all")
					}
				}
				return desktopInset{Top: 24, Bottom: 24, Left: 16, Right: 16}.Layout(gtx,
					func(gtx layout.Context) layout.Dimensions {
						children := []layout.FlexChild{
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								return s.layoutLabel(gtx, emptyText, textBodySmall, font.Normal, s.theme.onSurfaceVariant, 3)
							}),
						}
						if hasFilter {
							children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								return desktopInset{Top: 12}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
									return s.layoutButton(gtx, &s.sidebarClearFilterButton, "Clear filter", true, nil)
								})
							}))
						}
						return layout.Flex{Axis: layout.Vertical, Alignment: layout.Start}.Layout(gtx, children...)
					},
				)
			}
			return s.sidebarList.Layout(gtx, len(rows), func(gtx layout.Context, index int) layout.Dimensions {
				return s.layoutSidebarRow(gtx, rows[index], snapshot.State)
			})
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return s.layoutHorizontalDivider(gtx)
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return s.layoutSidebarFooter(gtx, snapshot)
		}),
	)
}

func openBrowserURL(targetURL string) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", targetURL)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", targetURL)
	default:
		cmd = exec.Command("xdg-open", targetURL)
	}
	if cmd != nil {
		_ = cmd.Start()
	}
}

func (s *shell) layoutSidebarFooter(gtx layout.Context, snapshot controllerSnapshot) layout.Dimensions {
	if s.sidebarInspectorButton.Clicked(gtx) {
		s.openSettingsModal()
		gtx.Execute(key.FocusCmd{Tag: nil})
	}

	if s.sidebarArchiveButton.Clicked(gtx) && s.onSetFilterMode != nil {
		if snapshot.FilterMode == "pinned" {
			s.onSetFilterMode("all")
		} else {
			s.onSetFilterMode("pinned")
		}
	}

	if s.sidebarCommunityButton.Clicked(gtx) {
		openBrowserURL("https://github.com/phongsathornpt/protonman")
	}

	statusColor := s.theme.onSuccessContainer
	statusLabel := "Connected"
	switch snapshot.Connection {
	case connectionConnected:
		statusColor = s.theme.onSuccessContainer
		statusLabel = "Connected"
	case connectionConnecting, connectionReconnecting:
		statusColor = s.theme.onWarningContainer
		statusLabel = "Connecting"
	default:
		statusColor = s.theme.onErrorContainer
		statusLabel = "Offline"
	}
	status := strings.ToLower(snapshot.Status)
	if strings.Contains(status, "failed") || (strings.Contains(status, "unavailable") && !strings.Contains(status, "session list unavailable")) || strings.Contains(status, "disconnected") {
		statusColor = s.theme.onErrorContainer
		statusLabel = "Offline"
	}

	return desktopInset{Top: 8, Bottom: 8, Left: 10, Right: 10}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle, Spacing: layout.SpaceBetween}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						btnGtx := gtx
						bg := color.NRGBA{}
						fg := s.theme.onSurfaceVariant
						if s.settingsModalOpen {
							bg = s.theme.primaryContainer
							fg = s.theme.onPrimaryContainer
						} else if s.sidebarInspectorButton.Hovered() {
							bg = s.theme.surfaceContainerHigh
							fg = s.theme.onSurface
						}
						semantic.Button.Add(btnGtx.Ops)
						semantic.DescriptionOp("Open Settings (⌘,)").Add(btnGtx.Ops)
						return s.sidebarInspectorButton.Layout(btnGtx, func(gtx layout.Context) layout.Dimensions {
							return s.roundedSurface(gtx, shapeSmall, bg, func(gtx layout.Context) layout.Dimensions {
								return desktopUniformInset(6).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
									return s.layoutActionIcon(gtx, iconSettings, 16, fg)
								})
							})
						})
					}),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return desktopInset{Left: 6}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
							btnGtx := gtx
							bg := color.NRGBA{}
							fg := s.theme.onSurfaceVariant
							if snapshot.FilterMode == "pinned" {
								bg = s.theme.primaryContainer
								fg = s.theme.onPrimaryContainer
							} else if s.sidebarArchiveButton.Hovered() {
								bg = s.theme.surfaceContainerHigh
								fg = s.theme.onSurface
							}
							semantic.Button.Add(btnGtx.Ops)
							semantic.DescriptionOp("Toggle Starred Filter").Add(btnGtx.Ops)
							return s.sidebarArchiveButton.Layout(btnGtx, func(gtx layout.Context) layout.Dimensions {
								return s.roundedSurface(gtx, shapeSmall, bg, func(gtx layout.Context) layout.Dimensions {
									return desktopUniformInset(6).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
										return s.layoutActionIcon(gtx, iconArchive, 16, fg)
									})
								})
							})
						})
					}),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return desktopInset{Left: 6}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
							btnGtx := gtx
							bg := color.NRGBA{}
							fg := s.theme.onSurfaceVariant
							if s.sidebarCommunityButton.Hovered() {
								bg = s.theme.surfaceContainerHigh
								fg = s.theme.onSurface
							}
							semantic.Button.Add(btnGtx.Ops)
							semantic.DescriptionOp("Open Protonman Community").Add(btnGtx.Ops)
							return s.sidebarCommunityButton.Layout(btnGtx, func(gtx layout.Context) layout.Dimensions {
								return s.roundedSurface(gtx, shapeSmall, bg, func(gtx layout.Context) layout.Dimensions {
									return desktopUniformInset(6).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
										return s.layoutActionIcon(gtx, iconDiscord, 16, fg)
									})
								})
							})
						})
					}),
				)
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return s.roundedSurface(gtx, shapeFull, statusColor, func(gtx layout.Context) layout.Dimensions {
							gtx.Constraints.Min = image.Pt(gtx.Dp(6), gtx.Dp(6))
							gtx.Constraints.Max = image.Pt(gtx.Dp(6), gtx.Dp(6))
							return layout.Dimensions{Size: gtx.Constraints.Min}
						})
					}),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return desktopInset{Left: 6}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
							return s.layoutLabel(gtx, statusLabel, textLabelSmall, font.Normal, s.theme.onSurfaceVariant, 1)
						})
					}),
				)
			}),
		)
	})
}

func (s *shell) layoutSidebarHeader(gtx layout.Context, snapshot controllerSnapshot) layout.Dimensions {
	return desktopInset{Top: 14, Bottom: 4, Left: 12, Right: 12}.Layout(gtx,
		func(gtx layout.Context) layout.Dimensions {
			newLabel := "New chat"
			if snapshot.CreatingSession {
				newLabel = "Starting…"
			}
			canCreate := snapshot.Connection == connectionConnected && !snapshot.CreatingSession

			items := []layout.FlexChild{
				// Brand Header: [P] Protonman
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return desktopInset{Bottom: 14, Left: 4, Right: 4}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								return desktopInset{Right: 8}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
									return s.layoutActionIcon(gtx, iconBrandLogo, 20, s.theme.onSurface)
								})
							}),
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								return s.layoutLabel(gtx, "Protonman", textTitleMedium, font.Bold, s.theme.onSurface, 1)
							}),
						)
					})
				}),
				// "New chat" dedicated action row
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					btn := &s.sidebarNewSessionButton
					if btn.Clicked(gtx) && canCreate && s.onNewSession != nil {
						s.onNewSession()
					}
					bg := color.NRGBA{}
					fg := s.theme.onSurface
					if btn.Hovered() && canCreate {
						bg = s.theme.surfaceContainerHigh
					}
					semantic.Button.Add(gtx.Ops)
					semantic.DescriptionOp("Start a new chat session").Add(gtx.Ops)
					return btn.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						return s.roundedSurface(gtx, shapeSmall, bg, func(gtx layout.Context) layout.Dimensions {
							return desktopInset{Top: 7, Bottom: 7, Left: 8, Right: 8}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
								return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
									layout.Rigid(func(gtx layout.Context) layout.Dimensions {
										return desktopInset{Right: 10}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
											return s.layoutActionIcon(gtx, iconCompose, 16, fg)
										})
									}),
									layout.Rigid(func(gtx layout.Context) layout.Dimensions {
										return s.layoutLabel(gtx, newLabel, textBodyMedium, font.Medium, fg, 1)
									}),
								)
							})
						})
					})
				}),
				// Full-width search bar: Search threads
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return desktopInset{Top: 6, Bottom: 14}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						return s.layoutSidebarSearch(gtx)
					})
				}),
				// "Projects" section label
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return desktopInset{Bottom: 6, Left: 6}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						return s.layoutLabel(gtx, "Projects", textLabelSmall, font.Medium, s.theme.onSurfaceVariant, 1)
					})
				}),
			}
			return layout.Flex{Axis: layout.Vertical}.Layout(gtx, items...)
		},
	)
}

func (s *shell) layoutFilterDropdown(gtx layout.Context, snapshot controllerSnapshot) layout.Dimensions {
	currentMode := snapshot.FilterMode
	if currentMode == "" {
		currentMode = "all"
	}
	filterLabel := "All ▾"
	switch currentMode {
	case "running":
		filterLabel = "Running ▾"
	case "pinned":
		filterLabel = "Pinned ▾"
	}

	if s.filterDropdownButton.Clicked(gtx) {
		s.filterDropdownOpen = !s.filterDropdownOpen
	}
	return s.layoutButton(gtx, &s.filterDropdownButton, filterLabel, true, nil)
}

func (s *shell) layoutFilterDropdownMenu(gtx layout.Context, snapshot controllerSnapshot) layout.Dimensions {
	if s.filterAllButton.Clicked(gtx) {
		s.filterDropdownOpen = false
		s.onSetFilterMode("all")
	}
	if s.filterRunningButton.Clicked(gtx) {
		s.filterDropdownOpen = false
		s.onSetFilterMode("running")
	}
	if s.filterPinnedButton.Clicked(gtx) {
		s.filterDropdownOpen = false
		s.onSetFilterMode("pinned")
	}

	currentMode := snapshot.FilterMode
	if currentMode == "" {
		currentMode = "all"
	}

	return s.roundedBorderSurface(gtx, shapeSmall, s.theme.surfaceContainerHigh, s.theme.outlineVariant, 1, func(gtx layout.Context) layout.Dimensions {
		return desktopInset{Top: 4, Bottom: 4, Left: 6, Right: 6}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle, Spacing: layout.SpaceBetween}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return s.layoutFilterItem(gtx, &s.filterAllButton, "All", currentMode == "all")
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return s.layoutFilterItem(gtx, &s.filterRunningButton, "Running", currentMode == "running")
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return s.layoutFilterItem(gtx, &s.filterPinnedButton, "Pinned", currentMode == "pinned")
				}),
			)
		})
	})
}

func (s *shell) layoutFilterItem(gtx layout.Context, button *widget.Clickable, label string, active bool) layout.Dimensions {
	background := s.theme.surfaceContainerHigh
	foreground := s.theme.onSurfaceVariant
	if active {
		background = s.theme.primaryContainer
		foreground = s.theme.onPrimaryContainer
	} else if button.Hovered() {
		background = s.theme.surfaceContainerHighest
		foreground = s.theme.onSurface
	}
	dims := button.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		gtx.Constraints.Min.Y = gtx.Dp(24)
		return s.roundedSurface(gtx, shapeSmall, background, func(gtx layout.Context) layout.Dimensions {
			return desktopInset{Top: 3, Bottom: 3, Left: 8, Right: 8}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				weight := font.Medium
				if active {
					weight = font.SemiBold
				}
				var items []layout.FlexChild
				if label == "Pinned" {
					items = append(items, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return desktopInset{Right: 4}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
							return s.layoutActionIcon(gtx, iconStar, 10, foreground)
						})
					}))
				} else if label == "Running" {
					items = append(items, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return desktopInset{Right: 4}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
							size := gtx.Dp(6)
							defer clip.Ellipse{Max: image.Pt(size, size)}.Op(gtx.Ops).Push(gtx.Ops).Pop()
							paint.ColorOp{Color: foreground}.Add(gtx.Ops)
							paint.PaintOp{}.Add(gtx.Ops)
							return layout.Dimensions{Size: image.Pt(size, size)}
						})
					}))
				}
				items = append(items, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return s.layoutLabel(gtx, label, textLabelSmall, weight, foreground, 1)
				}))
				return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx, items...)
			})
		})
	})
	return dims
}

func (s *shell) layoutSidebarSearch(gtx layout.Context) layout.Dimensions {
	for {
		evt, ok := gtx.Event(key.Filter{Focus: &s.sidebarSearchEditor, Name: key.NameEscape})
		if !ok {
			break
		}
		if e, ok := evt.(key.Event); ok && e.State == key.Press {
			s.sidebarSearchEditor.SetText("")
			gtx.Execute(key.FocusCmd{Tag: nil})
		}
	}
	if s.sidebarSearchClearButton.Clicked(gtx) {
		s.sidebarSearchEditor.SetText("")
	}

	gtx.Constraints.Min.Y = gtx.Dp(28)
	isFocused := gtx.Focused(&s.sidebarSearchEditor)
	borderColor := color.NRGBA{}
	borderWidth := 0
	iconColor := s.theme.onSurfaceVariant
	if isFocused {
		borderColor = s.theme.primary
		borderWidth = 1
		iconColor = s.theme.primary
	}

	return s.roundedBorderSurface(gtx, shapeMedium, s.theme.surfaceContainerHigh, borderColor, borderWidth, func(gtx layout.Context) layout.Dimensions {
		return desktopInset{Top: 4, Bottom: 4, Left: 8, Right: 8}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return desktopInset{Right: 6}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						return s.layoutActionIcon(gtx, iconSearch, 16, iconColor)
					})
				}),
				layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
					ed := material.Editor(s.theme.material, &s.sidebarSearchEditor, "Search threads…")
					ed.TextSize = textBodySmall
					ed.Color = s.theme.onSurface
					ed.HintColor = s.theme.onSurfaceVariant
					return ed.Layout(gtx)
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					if s.sidebarSearchEditor.Text() == "" {
						return layout.Dimensions{}
					}
					return desktopInset{Left: 4}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						return s.layoutMiniIconButton(gtx, &s.sidebarSearchClearButton, "×", s.theme.onSurfaceVariant)
					})
				}),
			)
		})
	})
}

func (s *shell) sidebarDisplayRows(rows []sidebarRow, filterMode string) []sidebarRow {
	query := strings.ToLower(strings.TrimSpace(s.sidebarSearchEditor.Text()))
	filterMode = strings.ToLower(strings.TrimSpace(filterMode))

	display := make([]sidebarRow, 0, len(rows))
	currentProjectCollapsed := false
	inPinnedSection := false

	matchesFilter := func(row sidebarRow) bool {
		switch filterMode {
		case "running":
			statusLower := strings.ToLower(row.Status)
			return strings.Contains(statusLower, "running") ||
				strings.Contains(statusLower, "permission") ||
				strings.Contains(statusLower, "approval")
		case "pinned":
			return row.Pinned
		default:
			return true
		}
	}

	matchesQuery := func(row sidebarRow) bool {
		if query == "" {
			return true
		}
		return strings.Contains(strings.ToLower(row.Title), query) ||
			strings.Contains(strings.ToLower(row.Subtitle), query) ||
			strings.Contains(strings.ToLower(row.ProjectID), query)
	}

	for _, row := range rows {
		switch row.Kind {
		case sidebarPinnedHeaderRow:
			inPinnedSection = true
			if filterMode != "running" {
				display = append(display, row)
			}
		case sidebarProjectRow:
			inPinnedSection = false
			currentProjectCollapsed = s.projectCollapsed[row.ProjectID]
			display = append(display, row)
		case sidebarSessionRow:
			if inPinnedSection {
				if s.pinnedCollapsed {
					continue
				}
				if matchesFilter(row) && matchesQuery(row) {
					display = append(display, row)
				}
			} else {
				if currentProjectCollapsed {
					continue
				}
				if matchesFilter(row) && matchesQuery(row) {
					display = append(display, row)
				}
			}
		}
	}

	if query != "" || filterMode == "running" || filterMode == "pinned" {
		cleaned := make([]sidebarRow, 0, len(display))
		for i, r := range display {
			if r.Kind == sidebarProjectRow || r.Kind == sidebarPinnedHeaderRow {
				hasSessions := false
				for j := i + 1; j < len(display); j++ {
					if display[j].Kind == sidebarProjectRow || display[j].Kind == sidebarPinnedHeaderRow {
						break
					}
					if display[j].Kind == sidebarSessionRow {
						hasSessions = true
						break
					}
				}
				if hasSessions {
					cleaned = append(cleaned, r)
				}
			} else {
				cleaned = append(cleaned, r)
			}
		}
		return cleaned
	}

	return display
}

func (s *shell) sidebarRows(snapshot controllerSnapshot) []sidebarRow {
	if s.sidebarRowsCache.valid && s.sidebarRowsCache.matchesWithOptions(snapshot.State, snapshot.AgentProfiles, snapshot.FilterMode, snapshot.PinnedSessions, snapshot.CustomTitles) {
		return s.sidebarRowsCache.rows
	}
	rows, cache := buildSidebarRowsWithOptions(snapshot.State, snapshot.AgentProfiles, snapshot.PinnedSessions, snapshot.CustomTitles)
	cache.filterMode = snapshot.FilterMode
	s.sidebarRowsCache = cache
	return rows
}

func buildSidebarRows(state desktopstate.State, profiles []app.ACPAgentProfile) ([]sidebarRow, sidebarRowsCache) {
	return buildSidebarRowsWithOptions(state, profiles, nil, nil)
}

func buildSidebarRowsWithOptions(state desktopstate.State, profiles []app.ACPAgentProfile, pinnedSessions []string, customTitles map[string]string) ([]sidebarRow, sidebarRowsCache) {
	rows := make([]sidebarRow, 0, len(state.Sessions)+len(state.Projects)+1)
	projects := make([]sidebarProjectCache, 0, len(state.Projects))
	sessions := make([]sidebarSessionCache, 0, len(state.Sessions))
	sessionsByProject := make(map[string][]sidebarRow, len(state.Sessions))
	pinnedMap := make(map[string]bool, len(pinnedSessions))
	for _, id := range pinnedSessions {
		pinnedMap[id] = true
	}

	pinnedRows := make([]sidebarRow, 0, len(pinnedSessions))

	for _, session := range state.Sessions {
		title := session.Title
		if custom, ok := customTitles[session.ID]; ok && custom != "" {
			title = custom
		}
		pinned := pinnedMap[session.ID]
		subtitle := agentDisplayName(profiles, session.AgentID)
		sessions = append(sessions, sidebarSessionCache{
			id:             session.ID,
			projectID:      session.ProjectID,
			agentID:        session.AgentID,
			title:          title,
			subtitle:       subtitle,
			status:         string(session.Status),
			lastActivityAt: session.LastActivityAt,
			pinned:         pinned,
		})
		row := sidebarRow{
			Kind:           sidebarSessionRow,
			ProjectID:      session.ProjectID,
			SessionID:      session.ID,
			AgentID:        session.AgentID,
			Title:          title,
			Subtitle:       subtitle,
			Status:         displayStatus(session.Status),
			LastActivityAt: session.LastActivityAt,
			Pinned:         pinned,
		}
		sessionsByProject[session.ProjectID] = append(sessionsByProject[session.ProjectID], row)
		if pinned {
			pinnedRows = append(pinnedRows, row)
		}
	}

	if len(pinnedRows) > 0 {
		sort.SliceStable(pinnedRows, func(i, j int) bool {
			left, right := pinnedRows[i].LastActivityAt, pinnedRows[j].LastActivityAt
			if left.IsZero() {
				return false
			}
			if right.IsZero() {
				return true
			}
			return left.After(right)
		})
		rows = append(rows, sidebarRow{
			Kind:         sidebarPinnedHeaderRow,
			Title:        "Pinned",
			SessionCount: len(pinnedRows),
		})
		rows = append(rows, pinnedRows...)
	}

	for _, project := range state.Projects {
		projects = append(projects, sidebarProjectCache{id: project.ID, name: project.Name})
		projectSessions := sessionsByProject[project.ID]
		sort.SliceStable(projectSessions, func(i, j int) bool {
			left, right := projectSessions[i].LastActivityAt, projectSessions[j].LastActivityAt
			if left.IsZero() {
				return false
			}
			if right.IsZero() {
				return true
			}
			return left.After(right)
		})
		rows = append(rows, sidebarRow{
			Kind:         sidebarProjectRow,
			ProjectID:    project.ID,
			Title:        project.Name,
			SessionCount: len(projectSessions),
		})
		rows = append(rows, projectSessions...)
	}

	return rows, sidebarRowsCache{
		valid:      true,
		rows:       rows,
		projects:   projects,
		sessions:   sessions,
		pinnedHash: strings.Join(pinnedSessions, ","),
	}
}

func (cache sidebarRowsCache) matches(state desktopstate.State, profiles []app.ACPAgentProfile) bool {
	return cache.matchesWithOptions(state, profiles, cache.filterMode, nil, nil)
}

func (cache sidebarRowsCache) matchesWithOptions(state desktopstate.State, profiles []app.ACPAgentProfile, filterMode string, pinnedSessions []string, customTitles map[string]string) bool {
	if cache.filterMode != filterMode || cache.pinnedHash != strings.Join(pinnedSessions, ",") {
		return false
	}
	if len(cache.projects) != len(state.Projects) || len(cache.sessions) != len(state.Sessions) {
		return false
	}
	projectIndex := 0
	for _, project := range state.Projects {
		if projectIndex >= len(cache.projects) {
			return false
		}
		cachedProject := cache.projects[projectIndex]
		if cachedProject.id != project.ID || cachedProject.name != project.Name {
			return false
		}
		projectIndex++
	}
	if projectIndex != len(cache.projects) || len(state.Sessions) != len(cache.sessions) {
		return false
	}
	for sessionIndex, session := range state.Sessions {
		cachedSession := cache.sessions[sessionIndex]
		expectedTitle := session.Title
		if custom, ok := customTitles[session.ID]; ok && custom != "" {
			expectedTitle = custom
		}
		if cachedSession.id != session.ID ||
			cachedSession.projectID != session.ProjectID ||
			cachedSession.agentID != session.AgentID ||
			cachedSession.title != expectedTitle ||
			cachedSession.subtitle != agentDisplayName(profiles, session.AgentID) ||
			cachedSession.status != string(session.Status) ||
			!cachedSession.lastActivityAt.Equal(session.LastActivityAt) {
			return false
		}
	}
	return true
}

func (s *shell) layoutSidebarRow(gtx layout.Context, row sidebarRow, state desktopstate.State) layout.Dimensions {
	if row.Kind == sidebarPinnedHeaderRow {
		button := &s.pinnedButton
		if button.Clicked(gtx) {
			s.pinnedCollapsed = !s.pinnedCollapsed
		}
		gtx.Constraints.Min.Y = gtx.Dp(32)
		chevronKind := iconChevronDown
		if s.pinnedCollapsed {
			chevronKind = iconChevronRight
		}
		dims := button.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			gtx.Constraints.Min.Y = gtx.Dp(28)
			background := color.NRGBA{}
			foreground := s.theme.onSurfaceVariant
			if button.Hovered() {
				background = s.theme.surfaceContainerHigh
				foreground = s.theme.onSurface
			}
			return s.roundedSurface(gtx, shapeSmall, background, func(gtx layout.Context) layout.Dimensions {
				return desktopInset{Top: 4, Bottom: 4, Left: 8, Right: 8}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							return desktopInset{Right: 6}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
								return s.layoutActionIcon(gtx, chevronKind, 11, s.theme.onSurfaceVariant)
							})
						}),
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							return desktopInset{Right: 6}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
								return s.layoutActionIcon(gtx, iconStar, 12, s.theme.primary)
							})
						}),
						layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
							return s.layoutLabel(gtx, "Pinned", textLabelSmall, font.SemiBold, foreground, 1)
						}),
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							if row.SessionCount > 0 {
								return s.layoutCountPill(gtx, row.SessionCount)
							}
							return layout.Dimensions{}
						}),
					)
				})
			})
		})
		return desktopInset{Top: 8, Bottom: 2, Left: 8, Right: 8}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return layout.Dimensions{Size: dims.Size}
		})
	}

	if row.Kind == sidebarProjectRow {
		button := s.projectButtons[row.ProjectID]
		selected := state.ActiveProjectID == row.ProjectID && state.ActiveSessionID == ""
		isCollapsed := s.projectCollapsed[row.ProjectID]
		if button.Clicked(gtx) {
			s.projectCollapsed[row.ProjectID] = !isCollapsed
			s.onSelectProject(row.ProjectID)
		}
		gtx.Constraints.Min.Y = gtx.Dp(32)
		chevronKind := iconChevronDown
		if isCollapsed {
			chevronKind = iconChevronRight
		}
		dims := button.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			gtx.Constraints.Min.Y = gtx.Dp(28)
			semantic.Button.Add(gtx.Ops)
			semantic.SelectedOp(selected).Add(gtx.Ops)
			semantic.DescriptionOp(fmt.Sprintf("Select workspace %s, %d conversations", row.Title, row.SessionCount)).Add(gtx.Ops)
			background := color.NRGBA{}
			foreground := s.theme.onSurfaceVariant
			if selected {
				background = s.theme.surfaceContainerHigh
				foreground = s.theme.onSurface
			} else if button.Hovered() {
				background = s.theme.surfaceContainerHigh
				foreground = s.theme.onSurface
			}
			return s.roundedSurface(gtx, shapeSmall, background, func(gtx layout.Context) layout.Dimensions {
				return desktopInset{Top: 4, Bottom: 4, Left: 8, Right: 8}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							return desktopInset{Right: 6}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
								return s.layoutActionIcon(gtx, chevronKind, 11, s.theme.onSurfaceVariant)
							})
						}),
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							return desktopInset{Right: 8}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
								return s.layoutActionIcon(gtx, iconFolder, 14, s.theme.onSurfaceVariant)
							})
						}),
						layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
							return s.layoutLabel(gtx, row.Title, textLabelSmall, font.SemiBold, foreground, 1)
						}),
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							if row.SessionCount > 0 {
								return s.layoutCountPill(gtx, row.SessionCount)
							}
							return layout.Dimensions{}
						}),
					)
				})
			})
		})
		if gtx.Focused(button) {
			widget.Border{Color: s.theme.primary, CornerRadius: shapeSmall, Width: 1}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return layout.Dimensions{Size: dims.Size}
			})
		}
		return desktopInset{Top: 10, Bottom: 2, Left: 8, Right: 8}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return layout.Dimensions{Size: dims.Size}
		})
	}

	button := s.sessionButtons[row.SessionID]
	pinBtn := s.sessionPinButton(row.SessionID)
	renameBtn := s.sessionQuickRenameButton(row.SessionID)
	deleteBtn := s.sessionQuickDeleteButton(row.SessionID)
	menuBtn := s.sessionMenuButton(row.SessionID)
	selected := state.ActiveSessionID == row.SessionID
	isRenaming := s.editingSessionID == row.SessionID
	menuOpen := s.menuSessionID == row.SessionID
	isHovered := button != nil && button.Hovered()
	showActions := isHovered || selected || menuOpen

	if button != nil && button.Clicked(gtx) {
		s.onSelectSession(row.SessionID)
	}

	if pinBtn.Clicked(gtx) {
		s.onTogglePinSession(row.SessionID)
	}
	if renameBtn.Clicked(gtx) {
		s.editingSessionID = row.SessionID
		s.sessionRenameEditor.SetText(row.Title)
	}
	if deleteBtn.Clicked(gtx) {
		s.deletingSessionID = row.SessionID
		s.deletingSessionTitle = row.Title
	}

	if isRenaming {
		for {
			evt, ok := gtx.Event(key.Filter{Focus: &s.sessionRenameEditor, Name: key.NameReturn})
			if !ok {
				break
			}
			if e, ok := evt.(key.Event); ok && e.State == key.Press {
				newTitle := s.sessionRenameEditor.Text()
				s.editingSessionID = ""
				s.onRenameSession(row.SessionID, newTitle)
			}
		}
		for {
			evt, ok := gtx.Event(key.Filter{Focus: &s.sessionRenameEditor, Name: key.NameEscape})
			if !ok {
				break
			}
			if e, ok := evt.(key.Event); ok && e.State == key.Press {
				s.editingSessionID = ""
			}
		}
		if s.renameConfirmButton.Clicked(gtx) {
			newTitle := s.sessionRenameEditor.Text()
			s.editingSessionID = ""
			s.onRenameSession(row.SessionID, newTitle)
		}
		if s.renameCancelButton.Clicked(gtx) {
			s.editingSessionID = ""
		}
	}

	if menuBtn.Clicked(gtx) {
		if s.menuSessionID == row.SessionID {
			s.menuSessionID = ""
		} else {
			s.menuSessionID = row.SessionID
		}
	}

	gtx.Constraints.Min.Y = gtx.Dp(40)
	subtitle := sidebarSessionSubtitle(row, gtx.Now)
	statusLabel := sidebarStatusLabel(row.Status)

	dims := button.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		gtx.Constraints.Min.Y = gtx.Dp(36)
		semantic.Button.Add(gtx.Ops)
		semantic.SelectedOp(selected).Add(gtx.Ops)
		description := row.Title
		if subtitle != "" {
			description += ", " + subtitle
		}
		if statusLabel != "" {
			description += ", " + statusLabel
		}
		semantic.DescriptionOp(description).Add(gtx.Ops)
		background := color.NRGBA{}
		foreground := s.theme.onSurfaceVariant
		if selected {
			background = s.theme.surfaceContainerHigh
			foreground = s.theme.onSurface
		} else if isHovered {
			background = s.theme.surfaceContainer
			foreground = s.theme.onSurface
		}
		return s.roundedSurface(gtx, shapeMedium, background, func(gtx layout.Context) layout.Dimensions {
			return desktopInset{Top: 6, Bottom: 6, Left: 12, Right: 8}.Layout(gtx,
				func(gtx layout.Context) layout.Dimensions {
					cardChildren := []layout.FlexChild{
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							if isRenaming {
								return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
									layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
										ed := material.Editor(s.theme.material, &s.sessionRenameEditor, "Session title…")
										ed.TextSize = textBodySmall
										ed.Color = foreground
										return ed.Layout(gtx)
									}),
									layout.Rigid(func(gtx layout.Context) layout.Dimensions {
										return desktopInset{Left: 4}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
											return s.layoutMiniButton(gtx, &s.renameConfirmButton, "✓", s.theme.primary)
										})
									}),
									layout.Rigid(func(gtx layout.Context) layout.Dimensions {
										return desktopInset{Left: 2}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
											return s.layoutMiniButton(gtx, &s.renameCancelButton, "✕", s.theme.onSurfaceVariant)
										})
									}),
								)
							}
							weight := font.Normal
							if selected {
								weight = font.Medium
							}
							return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
								layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
									return s.layoutLabel(gtx, row.Title, textBodySmall, weight, foreground, 1)
								}),
								layout.Rigid(func(gtx layout.Context) layout.Dimensions {
									if row.Pinned && !showActions {
										return desktopInset{Left: 4}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
											return s.layoutActionIcon(gtx, iconStar, 12, s.theme.primary)
										})
									}
									if !showActions {
										return layout.Dimensions{}
									}
									var items []layout.FlexChild
									if row.Pinned {
										items = append(items, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
											return desktopInset{Right: 4}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
												return s.layoutActionIcon(gtx, iconStar, 12, s.theme.primary)
											})
										}))
									}
									items = append(items, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
										return s.layoutMiniMenuButton(gtx, menuBtn, "⋮")
									}))
									return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx, items...)
								}),
							)
						}),
					}
					if subtitle != "" || statusLabel != "" {
						cardChildren = append(cardChildren, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							return desktopInset{Top: 3}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
								return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle, Spacing: layout.SpaceBetween}.Layout(gtx,
									layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
										if subtitle == "" {
											return layout.Dimensions{}
										}
										return s.layoutLabel(gtx, subtitle, textLabelSmall, font.Normal, s.theme.onSurfaceVariant, 1)
									}),
									layout.Rigid(func(gtx layout.Context) layout.Dimensions {
										if statusLabel == "" {
											return layout.Dimensions{}
										}
										return desktopInset{Left: 4}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
											return s.layoutSidebarTaskStatus(gtx, statusLabel, taskStatusFromLabel(row.Status))
										})
									}),
								)
							})
						}))
					}
					if menuOpen {
						cardChildren = append(cardChildren, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							return desktopInset{Top: 6, Bottom: 2}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
								return s.layoutSessionMenuPopover(gtx, row)
							})
						}))
					}
					return layout.Flex{Axis: layout.Vertical}.Layout(gtx, cardChildren...)
				},
			)
		})
	})
	if gtx.Focused(button) {
		widget.Border{Color: s.theme.primary, CornerRadius: shapeMedium, Width: 1}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return layout.Dimensions{Size: dims.Size}
		})
	}
	leftPad := unit.Dp(22)
	if row.Pinned {
		leftPad = unit.Dp(8)
	}
	return desktopInset{Top: 2, Bottom: 2, Left: leftPad, Right: 8}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Dimensions{Size: dims.Size}
	})
}

func (s *shell) sessionMenuButton(sessionID string) *widget.Clickable {
	if s.sessionMenuButtons[sessionID] == nil {
		s.sessionMenuButtons[sessionID] = new(widget.Clickable)
	}
	return s.sessionMenuButtons[sessionID]
}

func (s *shell) sessionPinButton(sessionID string) *widget.Clickable {
	if s.sessionPinButtons[sessionID] == nil {
		s.sessionPinButtons[sessionID] = new(widget.Clickable)
	}
	return s.sessionPinButtons[sessionID]
}

func (s *shell) sessionQuickRenameButton(sessionID string) *widget.Clickable {
	if s.sessionQuickRenameButtons[sessionID] == nil {
		s.sessionQuickRenameButtons[sessionID] = new(widget.Clickable)
	}
	return s.sessionQuickRenameButtons[sessionID]
}

func (s *shell) sessionQuickDeleteButton(sessionID string) *widget.Clickable {
	if s.sessionQuickDeleteButtons[sessionID] == nil {
		s.sessionQuickDeleteButtons[sessionID] = new(widget.Clickable)
	}
	return s.sessionQuickDeleteButtons[sessionID]
}

func (s *shell) layoutMiniMenuButton(gtx layout.Context, button *widget.Clickable, label string) layout.Dimensions {
	background := color.NRGBA{}
	foreground := s.theme.onSurfaceVariant
	if button.Hovered() {
		background = s.theme.surfaceContainerHighest
		foreground = s.theme.onSurface
	}
	dims := button.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		gtx.Constraints.Min = image.Pt(gtx.Dp(22), gtx.Dp(22))
		gtx.Constraints.Max = image.Pt(gtx.Dp(22), gtx.Dp(22))
		return s.roundedSurface(gtx, shapeSmall, background, func(gtx layout.Context) layout.Dimensions {
			return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				if label == "⋮" {
					return s.layoutActionIcon(gtx, iconKebab, 14, foreground)
				}
				return s.layoutLabel(gtx, label, textLabelMedium, font.Bold, foreground, 1)
			})
		})
	})
	return dims
}

func (s *shell) layoutMiniIconButton(gtx layout.Context, button *widget.Clickable, label string, normalColor color.NRGBA) layout.Dimensions {
	background := color.NRGBA{}
	foreground := normalColor
	if button.Hovered() {
		background = s.theme.surfaceContainerHighest
		foreground = s.theme.onSurface
	}
	dims := button.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		gtx.Constraints.Min = image.Pt(gtx.Dp(22), gtx.Dp(22))
		gtx.Constraints.Max = image.Pt(gtx.Dp(22), gtx.Dp(22))
		return s.roundedSurface(gtx, shapeSmall, background, func(gtx layout.Context) layout.Dimensions {
			return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				if label == "✕" || label == "×" {
					return s.layoutActionIcon(gtx, iconClose, 12, foreground)
				}
				return s.layoutLabel(gtx, label, textLabelSmall, font.Bold, foreground, 1)
			})
		})
	})
	return dims
}

func (s *shell) layoutCountPill(gtx layout.Context, count int) layout.Dimensions {
	return s.roundedSurface(gtx, shapeFull, s.theme.surfaceContainerHigh, func(gtx layout.Context) layout.Dimensions {
		return desktopInset{Top: 1, Bottom: 1, Left: 6, Right: 6}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return s.layoutLabel(gtx, fmt.Sprintf("%d", count), textLabelSmall, font.Medium, s.theme.onSurfaceVariant, 1)
		})
	})
}

func (s *shell) layoutMiniButton(gtx layout.Context, button *widget.Clickable, label string, fg color.NRGBA) layout.Dimensions {
	dims := button.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		gtx.Constraints.Min.Y = gtx.Dp(24)
		background := s.theme.surfaceContainer
		if button.Hovered() {
			background = s.theme.surfaceContainerHighest
		}
		return s.roundedSurface(gtx, shapeSmall, background, func(gtx layout.Context) layout.Dimensions {
			return desktopInset{Top: 3, Bottom: 3, Left: 8, Right: 8}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return s.layoutLabel(gtx, label, textLabelSmall, font.Medium, fg, 1)
			})
		})
	})
	return dims
}

func (s *shell) layoutSessionMenuPopover(gtx layout.Context, row sidebarRow) layout.Dimensions {
	pinLabel := "Pin"
	if row.Pinned {
		pinLabel = "Unpin"
	}

	if s.menuPinButton.Clicked(gtx) {
		s.menuSessionID = ""
		s.onTogglePinSession(row.SessionID)
	}
	if s.menuRenameButton.Clicked(gtx) {
		s.menuSessionID = ""
		s.editingSessionID = row.SessionID
		s.sessionRenameEditor.SetText(row.Title)
	}
	if s.menuDeleteButton.Clicked(gtx) {
		s.menuSessionID = ""
		s.deletingSessionID = row.SessionID
		s.deletingSessionTitle = row.Title
	}

	return s.roundedBorderSurface(gtx, shapeSmall, s.theme.surfaceContainerHighest, s.theme.outlineVariant, 1, func(gtx layout.Context) layout.Dimensions {
		return desktopInset{Top: 4, Bottom: 4, Left: 6, Right: 6}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle, Spacing: layout.SpaceBetween}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return s.layoutMiniButton(gtx, &s.menuPinButton, pinLabel, s.theme.primary)
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return s.layoutMiniButton(gtx, &s.menuRenameButton, "Rename", s.theme.onSurface)
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return s.layoutMiniButton(gtx, &s.menuDeleteButton, "Delete", s.theme.onErrorContainer)
				}),
			)
		})
	})
}

func (s *shell) layoutDeleteModal(gtx layout.Context) layout.Dimensions {
	if s.deleteModalCancelButton.Clicked(gtx) || s.deleteModalScrim.Clicked(gtx) {
		s.deletingSessionID = ""
		s.deletingSessionTitle = ""
	}

	return layout.Stack{Alignment: layout.Center}.Layout(gtx,
		layout.Expanded(func(gtx layout.Context) layout.Dimensions {
			paint.FillShape(gtx.Ops, color.NRGBA{R: 0, G: 0, B: 0, A: 160}, clip.Rect{Max: gtx.Constraints.Max}.Op())
			return s.deleteModalScrim.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return layout.Dimensions{Size: gtx.Constraints.Max}
			})
		}),
		layout.Stacked(func(gtx layout.Context) layout.Dimensions {
			gtx.Constraints.Min.X = gtx.Dp(380)
			gtx.Constraints.Max.X = gtx.Dp(440)
			return s.roundedBorderSurface(gtx, shapeExtraLarge, s.theme.surfaceContainerHigh, s.theme.outlineVariant, 1, func(gtx layout.Context) layout.Dimensions {
				return desktopInset{Top: 20, Bottom: 20, Left: 20, Right: 20}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							return s.layoutLabel(gtx, "Delete Session", textTitleMedium, font.Bold, s.theme.onSurface, 1)
						}),
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							return desktopInset{Top: 8, Bottom: 16}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
								msg := fmt.Sprintf("Are you sure you want to delete %q? This will delete all conversation turns and cannot be undone.", s.deletingSessionTitle)
								return s.layoutLabel(gtx, msg, textBodyMedium, font.Normal, s.theme.onSurfaceVariant, 3)
							})
						}),
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle, Spacing: layout.SpaceEnd}.Layout(gtx,
								layout.Rigid(func(gtx layout.Context) layout.Dimensions {
									return desktopInset{Right: 8}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
										return s.layoutButton(gtx, &s.deleteModalCancelButton, "Cancel", true, func() {
											s.deletingSessionID = ""
											s.deletingSessionTitle = ""
										})
									})
								}),
								layout.Rigid(func(gtx layout.Context) layout.Dimensions {
									return s.layoutDangerButton(gtx, &s.deleteModalConfirmButton, "Delete", true, func() {
										id := s.deletingSessionID
										s.deletingSessionID = ""
										s.deletingSessionTitle = ""
										if s.onDeleteSession != nil && id != "" {
											s.onDeleteSession(id)
										}
									})
								}),
							)
						}),
					)
				})
			})
		}),
	)
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

func (s *shell) layoutSidebarTaskStatus(gtx layout.Context, label string, status desktopstate.TaskStatus) layout.Dimensions {
	background, foreground := s.taskStatusColors(status)
	return s.roundedSurface(gtx, shapeSmall, background, func(gtx layout.Context) layout.Dimensions {
		return desktopInset{Top: 1, Bottom: 1, Left: 6, Right: 6}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return s.layoutLabel(gtx, label, textLabelSmall, font.SemiBold, foreground, 1)
		})
	})
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

func (s *shell) syncSessionButtons(state desktopstate.State, revision uint64) {
	if revision != 0 && s.buttonRevisionSet && s.buttonRevision == revision {
		return
	}
	liveProjects := s.projectButtonLive
	if liveProjects == nil {
		liveProjects = make(map[string]struct{}, len(state.Projects))
		s.projectButtonLive = liveProjects
	}
	clear(liveProjects)
	for _, project := range state.Projects {
		liveProjects[project.ID] = struct{}{}
		if s.projectButtons[project.ID] == nil {
			s.projectButtons[project.ID] = new(widget.Clickable)
		}
	}
	for projectID := range s.projectButtons {
		if _, ok := liveProjects[projectID]; !ok {
			delete(s.projectButtons, projectID)
		}
	}
	live := s.sessionButtonLive
	if live == nil {
		live = make(map[string]struct{}, len(state.Sessions))
		s.sessionButtonLive = live
	}
	clear(live)
	for _, session := range state.Sessions {
		live[session.ID] = struct{}{}
		if s.sessionButtons[session.ID] == nil {
			s.sessionButtons[session.ID] = new(widget.Clickable)
		}
		if s.sessionMenuButtons[session.ID] == nil {
			s.sessionMenuButtons[session.ID] = new(widget.Clickable)
		}
		if s.sessionPinButtons[session.ID] == nil {
			s.sessionPinButtons[session.ID] = new(widget.Clickable)
		}
		if s.sessionQuickRenameButtons[session.ID] == nil {
			s.sessionQuickRenameButtons[session.ID] = new(widget.Clickable)
		}
		if s.sessionQuickDeleteButtons[session.ID] == nil {
			s.sessionQuickDeleteButtons[session.ID] = new(widget.Clickable)
		}
	}
	for sessionID := range s.sessionButtons {
		if _, ok := live[sessionID]; !ok {
			delete(s.sessionButtons, sessionID)
		}
	}
	for sessionID := range s.sessionMenuButtons {
		if _, ok := live[sessionID]; !ok {
			delete(s.sessionMenuButtons, sessionID)
		}
	}
	for sessionID := range s.sessionPinButtons {
		if _, ok := live[sessionID]; !ok {
			delete(s.sessionPinButtons, sessionID)
		}
	}
	for sessionID := range s.sessionQuickRenameButtons {
		if _, ok := live[sessionID]; !ok {
			delete(s.sessionQuickRenameButtons, sessionID)
		}
	}
	for sessionID := range s.sessionQuickDeleteButtons {
		if _, ok := live[sessionID]; !ok {
			delete(s.sessionQuickDeleteButtons, sessionID)
		}
	}
	if revision != 0 {
		s.buttonRevision = revision
		s.buttonRevisionSet = true
	}
}

func selectedSession(state desktopstate.State) (desktopstate.SessionState, bool) {
	for _, session := range state.Sessions {
		if session.ID == state.ActiveSessionID {
			return session, true
		}
	}
	return desktopstate.SessionState{}, false
}

func displayStatus(status desktopstate.TaskStatus) string {
	value := strings.ReplaceAll(strings.TrimSpace(string(status)), "_", " ")
	if value == "" {
		return "idle"
	}
	return strings.ToUpper(value[:1]) + value[1:]
}

func sidebarStatusLabel(status string) string {
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

func sidebarSessionSubtitle(row sidebarRow, now time.Time) string {
	if !row.LastActivityAt.IsZero() {
		return sidebarActivityLabel(row.LastActivityAt, now)
	}
	if row.AgentID != "" && row.AgentID != controllerAgentID {
		return row.Subtitle
	}
	return ""
}

func sidebarActivityLabel(activity, now time.Time) string {
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
