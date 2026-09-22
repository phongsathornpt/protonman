package runtime

import (
	"fmt"
	"strings"
	"time"

	"charm.land/bubbles/v2/key"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/phongsathornpt/protonman/internal/adapter/in/tui/runtime/permissionpolicy"
	"github.com/phongsathornpt/protonman/internal/adapter/in/tui/runtime/transcriptutil"
	"github.com/phongsathornpt/protonman/internal/adapter/in/tui/state/agentui"
	"github.com/phongsathornpt/protonman/internal/adapter/in/tui/state/runtimeui"
	tuihistory "github.com/phongsathornpt/protonman/internal/adapter/in/tui/view/history"
	agentpane "github.com/phongsathornpt/protonman/internal/adapter/in/tui/view/pane/agent"
	tuistyle "github.com/phongsathornpt/protonman/internal/adapter/in/tui/view/style"
	"github.com/phongsathornpt/protonman/internal/adapter/out/model"
	"github.com/phongsathornpt/protonman/internal/core/agentprofile"
	"github.com/phongsathornpt/protonman/internal/core/modelprofile"
	"github.com/phongsathornpt/protonman/internal/core/permission"
	"github.com/phongsathornpt/protonman/internal/core/tool"
	"github.com/phongsathornpt/protonman/internal/feature/agent"
)

func promptPlaceholder(hasRunner bool, mode permission.Mode, planMode bool) string {
	if !hasRunner {
		return "Message or /command…"
	}
	// Once a runnable model is selected, keep the idle composer visually empty.
	// The context footer carries model/thinking state and the prompt glyph itself
	// is enough affordance, matching the compact reference layout.
	_ = mode
	_ = planMode
	return ""
}

func (m *bubbleModel) resetTranscript() {
	m.ensureHistoryState().Reset()
	m.conversationViewport.setFollowing(true)
	m.refreshTranscriptViewport(true)
}

func composerUsableWidth(terminalWidth int) int {
	return max(1, terminalWidth)
}

func (m *bubbleModel) promptView() string {
	if m.panes.bottom == nil || m.panes.bottom.prompt() == nil {
		return ""
	}
	prompt := m.panes.bottom.prompt()
	dividerStyle := tuistyle.PromptDividerIdle
	focused := prompt.Focused()
	switch {
	case m.permissionView() != nil:
		dividerStyle = tuistyle.PromptDividerWarning
		focused = false
	case m.service != nil && m.service.Mode() == permission.ModeDeny:
		dividerStyle = tuistyle.PromptDividerError
		focused = false
	case focused:
		dividerStyle = tuistyle.PromptDividerFocused
	}
	usableWidth := composerUsableWidth(m.layout.width)
	border := renderPromptDivider(dividerStyle, usableWidth, focused)
	return border + "\n" + prompt.View() + "\n" + border
}

func renderPromptDivider(style lipgloss.Style, width int, focused bool) string {
	width = max(1, width)
	if !focused {
		return style.Render(strings.Repeat("─", width))
	}

	accentWidth := min(8, width)
	accent := strings.Repeat("─", accentWidth)
	neutral := strings.Repeat("─", width-accentWidth)
	return tuistyle.PromptDividerFocused.Render(accent) +
		tuistyle.PromptDividerIdle.Render(neutral)
}

func (m *bubbleModel) modeChip() string {
	mode := permission.ModeAsk
	if m.service != nil {
		mode = m.service.Mode()
	}
	return m.modeChipFor(mode)
}

func (m *bubbleModel) modeChipFor(mode permission.Mode) string {
	if m.planMode {
		return planStyle.Render("mode: plan · read-only")
	}
	if m.layoutProfile().Mode != layoutNormal {
		switch mode {
		case permission.ModeAlwaysApprove:
			return warningStyle.Render("auto")
		case permission.ModeDeny:
			return errorStyle.Render("deny")
		default:
			return mutedStyle.Render("ask")
		}
	}
	switch mode {
	case permission.ModeAlwaysApprove:
		return warningStyle.Render("mode: auto-approve")
	case permission.ModeDeny:
		return errorStyle.Render("mode: deny")
	default:
		return mutedStyle.Render("mode: ask")
	}
}

type contextualHelp []key.Binding

func (h contextualHelp) ShortHelp() []key.Binding  { return h }
func (h contextualHelp) FullHelp() [][]key.Binding { return [][]key.Binding{h} }

func (m *bubbleModel) shortcutHint() string {
	if view := m.permissionView(); view != nil {
		return m.infoView()
	}
	helpView := m.help
	helpView.ShowAll = false
	helpView.SetWidth(m.layoutProfile().ContentWidth(m.layout.width))
	if m.slashOpen() {
		return helpView.View(contextualHelp{
			key.NewBinding(key.WithKeys("tab"), key.WithHelp("tab", "accept")),
			key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "run")),
			key.NewBinding(key.WithKeys("up", "down"), key.WithHelp("↑↓", "move")),
			key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "close")),
		})
	}
	if m.panes.bottom != nil && m.panes.bottom.has(todoInspectViewID) {
		return helpView.View(contextualHelp{
			key.NewBinding(key.WithKeys("up", "down"), key.WithHelp("↑↓", "move")),
			key.NewBinding(key.WithKeys("enter", "esc"), key.WithHelp("enter/esc", "close")),
		})
	}
	if !m.conversationViewport.following() {
		return helpView.View(contextualHelp{
			m.keys.Submit,
			m.keys.PageDown,
		})
	}
	return helpView.View(contextualHelp{
		m.keys.Submit,
		m.keys.Newline,
	})
}

func (m *bubbleModel) cyclePermission() {
	mode := m.service.Mode()
	var err error
	switch {
	case m.planMode:
		m.setPlanEnabled(false)
		err = m.setPermissionMode(permission.ModeAlwaysApprove)
	case mode == permission.ModeAlwaysApprove:
		err = m.setPermissionMode(permission.ModeAsk)
	default:
		if mode != permission.ModeAsk && mode != permission.ModeAuto {
			err = m.setPermissionMode(permission.ModeAsk)
		}
		m.setPlanEnabled(true)
	}
	if err != nil {
		m.appendError(fmt.Sprintf("failed to update permission mode: %v", err))
	}
	m.syncPermissionModePane()
	m.requestRelayout()
}

func (m *bubbleModel) setPlanEnabled(enabled bool) {
	m.planMode = enabled
	m.syncPromptPlaceholder()
	if !enabled {
		m.service.SetCallGuard(nil)
		m.agents.SetCallGuard(nil)
		return
	}
	guard := permissionpolicy.NewPlanModeGuard(func() bool {
		return m != nil && m.planMode
	})
	m.service.SetCallGuard(guard)
	m.agents.SetCallGuard(guard)
}

func (m *bubbleModel) runtimeStatusState(now time.Time) runtimeui.State {
	agentSnapshot := m.turnAgentSnapshot()
	activeAgents, _, _, _ := agentActivityCounts(agentSnapshot)
	agentActivity := ""
	if activeAgents > 0 {
		agentActivity = dominantAgentActivity(agentSnapshot, m.agentActivity)
		if runtimeui.IsCanceling(m.activity) {
			agentActivity = agentui.ActivityRetreating.Label()
		}
	}

	runningTool := ""
	if running, ok := m.ensureHistoryState().LastRunningTool(); ok {
		runningTool = tool.DisplayName(strings.TrimSpace(running.Name))
		if target := strings.TrimSpace(running.Target); target != "" {
			runningTool += " " + target
		}
	}

	_, streaming := m.ensureHistoryState().Active().(*tuihistory.AssistantCell)
	return runtimeui.Project(runtimeui.Input{
		Busy:              m.busy,
		PermissionPending: m.hasPermissionView(),
		Canceling:         runtimeui.IsCanceling(m.activity),
		Streaming:         streaming,
		Retry:             m.turnProgress.Retry,
		RunningTool:       runningTool,
		ExplicitActivity:  m.activity,
		FallbackActivity:  m.rootActivityLabel(),
		AgentActivity:     agentActivity,
		ActiveAgents:      activeAgents,
		ToolCalls:         m.turnProgress.ToolCalls,
		StartedAt:         m.busyStarted,
		Now:               now,
	})
}

func (m *bubbleModel) statusView() string {
	profile := m.layoutProfile()
	maxWidth := profile.ContentWidth(m.layout.width)
	state := m.runtimeStatusState(time.Now())
	if state.Phase == runtimeui.PhaseIdle {
		return ""
	}

	meta := state.MetaText()
	if state.Phase == runtimeui.PhaseWaitingForInput {
		line := state.Activity
		if meta != "" {
			line += " · " + meta
		}
		return warningStyle.Render(truncateWithEllipsis(line, max(1, maxWidth)))
	}

	icons := tuistyle.OrUnicodeIcons(m.icons)
	indicator := brandMarkStyle.Render(icons.Status)
	if spin := m.spinnerIndicator(); spin != "" {
		indicator = spin
	}
	prefix := indicator + " "
	prefixWidth := ansi.StringWidth(prefix)

	activityText := state.Activity
	if (state.Phase == runtimeui.PhaseDelegating || state.Phase == runtimeui.PhaseCanceling) && len(state.Meta) > 0 {
		agentMeta := state.Meta[0]
		if strings.HasSuffix(agentMeta, " agent") || strings.HasSuffix(agentMeta, " agents") {
			activityText += " · " + agentMeta
			state.Meta = state.Meta[1:]
			meta = state.MetaText()
		}
	}

	suffix := ""
	if meta != "" {
		const minimumActivityWidth = 8
		metaBudget := max(0, maxWidth-prefixWidth-minimumActivityWidth-3)
		if metaBudget > 0 {
			meta = truncateWithEllipsis(meta, metaBudget)
			suffix = " · " + meta
		}
	}
	contentWidth := max(1, maxWidth-prefixWidth-ansi.StringWidth(suffix))
	activity := truncateWithEllipsis(activityText, contentWidth)
	return prefix + systemStyle.Render(activity) + mutedStyle.Render(suffix)
}

type sessionHeaderCache struct {
	workDir     string
	branch      string
	branchValid bool
	visionModel string
	visionProv  string
	vision      bool
	visionValid bool
}

func (m *bubbleModel) sessionHeaderView() string {
	if m == nil {
		return ""
	}
	profile := m.layoutProfile()
	if !profile.ShowHeader {
		return ""
	}
	return renderSessionHeader(sessionHeaderModel{
		Width:          profile.ContentWidth(m.layout.width),
		Model:          m.activeModel,
		Vision:         m.activeModelSupportsVision(),
		LowConcurrency: m.lowConcurrencyEffective(),
		GoalActive:     strings.TrimSpace(m.activeGoal) != "",
		Branch:         m.sessionHeaderBranch(),
		Workspace:      transcriptutil.FormatWorkspaceDisplay(m.workDir),
		Compact:        profile.CompactHeader(),
		Minimal:        profile.MinimalHeader(),
		Icons:          m.icons,
	})
}

// activeModelSupportsVision reads primed vision capability state; profile
// resolution runs in primeSessionHeaderCache at the Update boundary.
func (m *bubbleModel) activeModelSupportsVision() bool {
	if m == nil || strings.TrimSpace(m.activeModel) == "" {
		return false
	}
	cache := &m.sessionHeaderCache
	if cache.visionValid && cache.visionModel == m.activeModel && cache.visionProv == m.activeProvider {
		return cache.vision
	}
	return false
}

// sessionHeaderBranch returns the primed branch without touching disk;
// primeSessionHeaderCache owns the git probe at the Update boundary.
func (m *bubbleModel) sessionHeaderBranch() string {
	if m == nil || m.sessionHeaderCache.workDir != m.workDir || !m.sessionHeaderCache.branchValid {
		return ""
	}
	return m.sessionHeaderCache.branch
}

// primeSessionHeaderCache resolves header metadata that needs filesystem or
// profile lookups — git branch and model vision capability — so the header
// helpers stay pure reads. It runs at the Update boundary (resize and after
// every Update) and requests a relayout when a resolved value changes.
func (m *bubbleModel) primeSessionHeaderCache() {
	if m == nil {
		return
	}
	cache := &m.sessionHeaderCache
	if cache.workDir != m.workDir {
		*cache = sessionHeaderCache{workDir: m.workDir}
	}
	dirty := false
	if !cache.branchValid {
		branch := transcriptutil.DetectGitBranch(m.workDir)
		if branch != cache.branch {
			dirty = true
		}
		cache.branch = branch
		cache.branchValid = true
	}
	if strings.TrimSpace(m.activeModel) != "" &&
		(!cache.visionValid || cache.visionModel != m.activeModel || cache.visionProv != m.activeProvider) {
		profile := model.ResolveModelProfile(m.activeProvider, m.activeModel, nil)
		vision := profile.Capabilities.Vision == modelprofile.SupportYes
		if vision != cache.vision {
			dirty = true
		}
		cache.vision = vision
		cache.visionModel = m.activeModel
		cache.visionProv = m.activeProvider
		cache.visionValid = true
	}
	if dirty {
		m.requestRelayout()
	}
}

func (m *bubbleModel) invalidateSessionHeaderBranch() {
	if m == nil {
		return
	}
	m.sessionHeaderCache.branchValid = false
}

// rootActivityLabel is the deterministic busy label for the primary agent when
// no tool, retry, or explicit activity is available. It comes from the active
// profile's activity vocabulary rather than a generic assistant word.
func (m *bubbleModel) rootActivityLabel() string {
	profile, err := agentprofile.ParseProfile(strings.TrimSpace(m.agentProfile))
	if err != nil || !profile.Valid() {
		profile = agentprofile.ProfileUniversal
	}
	return agentui.ActivityForState(profile, agent.StateRunning).String()
}

func dominantAgentActivity(snapshot []agent.AgentStatus, activities map[string]AgentActivity) string {
	type ranked struct {
		label string
		rank  int
	}
	best := ranked{label: agentui.ActivityWaiting.Label(), rank: 0}
	for _, st := range snapshot {
		if st.State.Terminal() {
			continue
		}
		a := activities[st.ID]
		if a.String() == "" {
			a = agentui.ActivityForState(st.Profile, st.State)
		}
		rank := agentActivityRank(a.Intent)
		if rank > best.rank {
			best = ranked{label: a.Intent.Label(), rank: rank}
		}
	}
	if strings.TrimSpace(best.label) == "" {
		return agentui.ActivityRoaming.Label()
	}
	return best.label
}

func agentActivityRank(intent agentui.ActivityIntent) int {
	switch intent {
	case agentui.ActivityRetreating:
		return 90
	case agentui.ActivityCare:
		return 80
	case agentui.ActivityDefending:
		return 70
	case agentui.ActivityPushing:
		return 60
	case agentui.ActivityGanking:
		return 50
	case agentui.ActivitySticking, agentui.ActivityIntegrated:
		return 45
	case agentui.ActivitySkilling:
		return 40
	case agentui.ActivityFarming:
		return 30
	case agentui.ActivityRoaming:
		return 20
	case agentui.ActivityWaiting:
		return 10
	default:
		return 0
	}
}

func (m *bubbleModel) turnAgentSnapshot() []agent.AgentStatus {
	if m.activeTurnOwner == "" {
		return m.agentSnapshot
	}
	out := make([]agent.AgentStatus, 0, len(m.agentSnapshot))
	for _, st := range m.agentSnapshot {
		if st.ParentID == m.activeTurnOwner {
			out = append(out, st)
		}
	}
	return out
}

func agentActivityCounts(snapshot []agent.AgentStatus) (active, running, queued, canceling int) {
	for _, st := range snapshot {
		switch st.State {
		case agent.StateRunning:
			running++
		case agent.StateQueued:
			queued++
		case agent.StateCanceling:
			canceling++
		}
	}
	active = running + queued + canceling
	return active, running, queued, canceling
}

func (m *bubbleModel) infoView() string {
	width := m.layoutProfile().ContentWidth(m.layout.width)
	if view := m.permissionView(); view != nil {
		if view.parked {
			return paneKeyboardHelp(width, "tab", "Review", "y", "Once", "s", "Session", "n", "Deny")
		}
		return paneKeyboardHelp(width, "y", "Once", "s", "Session", "n", "Deny", "esc", "Review")
	}
	if m.planMode {
		return planStyle.Render("plan · read-only")
	}
	if m.service != nil {
		switch m.service.Mode() {
		case permission.ModeAlwaysApprove:
			return warningStyle.Render("auto")
		case permission.ModeDeny:
			return errorStyle.Render("deny")
		}
	}
	return ""
}

func formatElapsed(duration time.Duration) string {
	return agentpane.FormatElapsed(duration)
}
