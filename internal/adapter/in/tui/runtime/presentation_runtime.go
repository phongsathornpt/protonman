package runtime

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"charm.land/bubbles/v2/key"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/phongsathornpt/protonman/internal/adapter/in/tui/runtime/transcriptutil"
	"github.com/phongsathornpt/protonman/internal/adapter/in/tui/state/agentui"
	agentpane "github.com/phongsathornpt/protonman/internal/adapter/in/tui/view/pane/agent"
	"github.com/phongsathornpt/protonman/internal/core/permission"
	"github.com/phongsathornpt/protonman/internal/core/tool"
	"github.com/phongsathornpt/protonman/internal/feature/agent"
	sdk "github.com/phongsathornpt/protonman/proton-sdk"
)

var promptBorderStyle = lipgloss.NewStyle().Foreground(promptBorder)

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

func minInt(left int, right int) int {
	if left < right {
		return left
	}
	return right
}

func composerUsableWidth(terminalWidth int) int {
	return maxInt(1, terminalWidth)
}

func (m *bubbleModel) promptView() string {
	if m.panes.bottom == nil || m.panes.bottom.prompt() == nil {
		return ""
	}
	usableWidth := composerUsableWidth(m.layout.width)
	border := promptBorderStyle.Render(strings.Repeat("─", usableWidth))
	return border + "\n" + m.panes.bottom.prompt().View() + "\n" + border
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

func (h contextualHelp) ShortHelp() []key.Binding   { return h }
func (h contextualHelp) FullHelp() [][]key.Binding { return [][]key.Binding{h} }

func (m bubbleModel) shortcutHint() string {
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
	switch {
	case m.planMode:
		m.setPlanEnabled(false)
		_ = m.setPermissionMode(permission.ModeAlwaysApprove)
	case mode == permission.ModeAlwaysApprove:
		_ = m.setPermissionMode(permission.ModeAsk)
	default:
		if mode != permission.ModeAsk && mode != permission.ModeAuto {
			_ = m.setPermissionMode(permission.ModeAsk)
		}
		m.setPlanEnabled(true)
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
	guard := func(_ context.Context, request permission.Request) error {
		if !m.planMode {
			return nil
		}
		switch request.ToolKind {
		case permission.ToolRead, permission.ToolGrep, permission.ToolWeb, permission.ToolTask:
			return nil
		case permission.ToolBash:
			var input struct {
				Command string `json:"command"`
			}
			if json.Unmarshal(request.Arguments, &input) == nil && tool.AnalyzeCommand(input.Command).Effect == tool.CommandEffectReadOnly {
				return nil
			}
		case permission.ToolAgent:
			if request.ToolName == "subagent" {
				var input struct {
					Action string `json:"action"`
				}
				if json.Unmarshal(request.Arguments, &input) == nil && (input.Action == "wait" || input.Action == "get" || input.Action == "list") {
					return nil
				}
			}
		}
		return fmt.Errorf("plan mode is read-only; %s tool %q is blocked", request.ToolKind, request.ToolName)
	}
	m.service.SetCallGuard(guard)
	m.agents.SetCallGuard(guard)
}

func (m bubbleModel) statusView() string {
	profile := m.layoutProfile()
	maxWidth := profile.ContentWidth(m.layout.width)
	if m.hasPermissionView() {
		return warningStyle.Render(truncateWithEllipsis("action required · permission", maxInt(1, maxWidth)))
	}
	if !m.busy {
		return ""
	}
	agentSnapshot := m.turnAgentSnapshot()
	activeAgents, _, _, _ := agentActivityCounts(agentSnapshot)
	activity := strings.TrimSpace(m.activity)
	if activeAgents > 0 {
		label := "agent"
		if activeAgents != 1 {
			label = "agents"
		}
		dota := dominantAgentActivity(agentSnapshot, m.agentActivity)
		if activity == "canceling" {
			dota = agentui.ActivityRetreating.Label()
		}
		activity = fmt.Sprintf("%s · %d %s", dota, activeAgents, label)
	}
	meta := ""
	if activeAgents == 0 {
		// Only an explicit label (a named call, cancellation, or background
		// operation) outranks the derived signals. Everything else resolves
		// deterministically: retry countdown, then the running tool, then the
		// active profile's intent. No generic assistant busy word is invented.
		explicit := activity != "" && activity != "ready"
		if retryActivity, retryMeta, ok := modelRetryStatus(m.turnProgress.Retry, time.Now()); ok {
			activity, meta = retryActivity, retryMeta
		} else if !explicit {
			if running, ok := m.ensureHistoryState().LastRunningTool(); ok {
				activity = tool.DisplayName(strings.TrimSpace(running.Name))
				if target := strings.TrimSpace(running.Target); target != "" {
					activity += " " + target
				}
			} else {
				activity = m.rootActivityLabel()
			}
		}
		if meta == "" && m.turnProgress.ToolCalls > 0 {
			label := "tool"
			if m.turnProgress.ToolCalls != 1 {
				label = "tools"
			}
			meta = fmt.Sprintf(" · %d %s", m.turnProgress.ToolCalls, label)
		}
	}
	if !m.busyStarted.IsZero() {
		if elapsed := formatElapsed(time.Since(m.busyStarted)); elapsed != "" {
			meta += " · " + elapsed
		}
	}
	indicator := brandMarkStyle.Render("◌")
	if spin := m.spinnerIndicator(); spin != "" {
		indicator = spin
	}
	contentWidth := maxInt(1, maxWidth-2-ansi.StringWidth(meta))
	activity = truncateWithEllipsis(activity, contentWidth)
	busyLine := indicator + " " + systemStyle.Render(activity) + mutedStyle.Render(meta)
	return busyLine
}

type sessionHeaderCache struct {
	workDir     string
	branch      string
	branchValid bool
}

func (m *bubbleModel) sessionHeaderView() string {
	if m == nil {
		return ""
	}
	profile := m.layoutProfile()
	if !profile.ShowHeader {
		return ""
	}
	cache := &m.sessionHeaderCache
	if cache.workDir != m.workDir {
		*cache = sessionHeaderCache{workDir: m.workDir}
	}
	if !cache.branchValid {
		cache.branch = transcriptutil.DetectGitBranch(m.workDir)
		cache.branchValid = true
	}
	return renderSessionHeader(sessionHeaderModel{
		Width:          profile.ContentWidth(m.layout.width),
		Model:          m.activeModel,
		LowConcurrency: m.lowConcurrencyEffective(),
		GoalActive:     strings.TrimSpace(m.activeGoal) != "",
		Branch:         cache.branch,
		Workspace:      transcriptutil.FormatWorkspaceDisplay(m.workDir),
		Compact:        profile.CompactHeader(),
		Minimal:        profile.MinimalHeader(),
	})
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
func (m bubbleModel) rootActivityLabel() string {
	profile, err := agent.ParseProfile(strings.TrimSpace(m.agentProfile))
	if err != nil || !profile.Valid() {
		profile = agent.ProfileUniversal
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

func (m bubbleModel) turnAgentSnapshot() []agent.AgentStatus {
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

func modelRetryStatus(retry sdk.RetryEvent, now time.Time) (string, string, bool) {
	if retry.Attempt <= 0 || retry.RetryAt.IsZero() {
		return "", "", false
	}
	remaining := retry.RetryAt.Sub(now)
	wait := "now"
	if remaining > 0 {
		if remaining < time.Second {
			wait = "<1s"
		} else {
			seconds := int((remaining + time.Second - 1) / time.Second)
			wait = fmt.Sprintf("%ds", seconds)
		}
	}
	activity := "retrying " + wait
	if retry.Phase == sdk.RetryPhaseCooldown {
		if wait == "now" {
			activity = "cooldown complete"
		} else {
			activity = "cooling down " + wait
		}
	} else if wait != "now" {
		activity = "retrying in " + wait
	}
	meta := fmt.Sprintf(" · retry %d", retry.Attempt)
	if retry.MaxRetries > 0 {
		meta += fmt.Sprintf("/%d", retry.MaxRetries)
	}
	if reason := retryReasonLabel(retry.Reason); reason != "" {
		meta += " · " + reason
	}
	return activity, meta, true
}

func retryReasonLabel(reason string) string {
	switch strings.TrimSpace(reason) {
	case "incomplete_stream":
		return "stream interrupted"
	case "first_event_timeout":
		return "provider slow"
	case "idle_event_timeout":
		return "stream stalled"
	case "max_stream_duration":
		return "stream limit"
	case "rate_limit":
		return "rate limited"
	case "overloaded":
		return "provider busy"
	case "transport":
		return "connection interrupted"
	}
	return strings.ReplaceAll(strings.TrimSpace(reason), "_", " ")
}
