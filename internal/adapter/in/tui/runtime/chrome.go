package runtime

import (
	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"github.com/charmbracelet/x/ansi"
	"github.com/phongsathornpt/protonman/internal/adapter/in/tui/view/pane"
	"github.com/phongsathornpt/protonman/internal/core/permission"
	"github.com/phongsathornpt/protonman/internal/core/tool"
	"github.com/phongsathornpt/protonman/internal/feature/agent"
	sdk "github.com/phongsathornpt/protonman/proton-sdk"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"sort"
	"strings"
	"time"
)

type welcomeCardCache struct {
	workDir     string
	branch      string
	branchValid bool
	width       int
	height      int
	rendered    string
	renderValid bool
}

func (m *bubbleModel) welcomeCard() string {
	if m == nil {
		return ""
	}
	cache := &m.welcomeCache
	if cache.workDir != m.workDir {
		*cache = welcomeCardCache{workDir: m.workDir}
	}
	if !cache.branchValid {
		cache.branch = detectGitBranch(m.workDir)
		cache.branchValid = true
		cache.renderValid = false
	}
	if cache.renderValid && cache.width == m.width && cache.height == m.height {
		return cache.rendered
	}
	cache.rendered = m.renderWelcomeCard(cache.branch)
	cache.width = m.width
	cache.height = m.height
	cache.renderValid = true
	return cache.rendered
}

func (m *bubbleModel) invalidateWelcomeBranch() {
	if m == nil {
		return
	}
	m.welcomeCache.branchValid = false
	m.welcomeCache.renderValid = false
}

func (m *bubbleModel) renderWelcomeCard(branch string) string {
	mode := layoutModeForHeight(m.height)
	if mode != layoutNormal || m.width < 60 {
		return brandLockup(m.width)
	}
	rows := []string{brandLockup(m.width), ""}
	if ws := formatWorkspaceDisplay(m.workDir); ws != "" {
		branchBadge := ""
		if branch != "" {
			branchBadge = " " + mutedStyle.Render("git:(") + systemStyle.Render(branch) + mutedStyle.Render(")")
		}
		rows = append(rows, heroLabelStyle.Render("Workspace ")+bodyStyle.Render(ws)+branchBadge, "")
	}
	if m.width >= 80 {
		colWidth := 36
		rows = append(rows, heroLabelStyle.Render("Quick Actions"), "  "+padToWidth(heroKeyStyle.Render("› /help")+"   "+mutedStyle.Render("Command palette"), colWidth)+heroKeyStyle.Render("› Ctrl+P")+"  "+mutedStyle.Render("Switch model"), "  "+padToWidth(heroKeyStyle.Render("› /model")+"  "+mutedStyle.Render("Choose AI provider"), colWidth)+heroKeyStyle.Render("› Ctrl+T")+"  "+mutedStyle.Render("View transcript"), "  "+padToWidth(heroKeyStyle.Render("› /skills")+" "+mutedStyle.Render("Active capabilities"), colWidth)+heroKeyStyle.Render("› ! <cmd>")+" "+mutedStyle.Render("Run bash command"))
	} else {
		rows = append(rows, heroLabelStyle.Render("Quick Actions"), "  "+heroKeyStyle.Render("› /help")+"     "+mutedStyle.Render("Command palette & shortcuts"), "  "+heroKeyStyle.Render("› /model")+"    "+mutedStyle.Render("Switch AI model (Ctrl+P)"), "  "+heroKeyStyle.Render("› /skills")+"   "+mutedStyle.Render("Inspect loaded capabilities"), "  "+heroKeyStyle.Render("› ! <cmd>")+"   "+mutedStyle.Render("Run bash command directly"))
	}
	rows = append(rows, "", mutedStyle.Render("Tip: Type a prompt to inspect or change this project, or ask for guidance."))
	return strings.Join(rows, "\n")
}

func padToWidth(s string, targetWidth int) string {
	w := ansi.StringWidth(s)
	if w >= targetWidth {
		return s
	}
	return s + strings.Repeat(" ", targetWidth-w)
}

func formatWorkspaceDisplay(dir string) string {
	cleaned := strings.TrimSpace(dir)
	if cleaned == "" {
		return ""
	}
	cleaned = filepath.Clean(cleaned)
	home, err := os.UserHomeDir()
	if err == nil && home != "" {
		if cleaned == home {
			return "~"
		}
		sep := string(filepath.Separator)
		if strings.HasPrefix(cleaned, home+sep) {
			return "~" + cleaned[len(home):]
		}
	}
	return cleaned
}

func detectGitBranch(dir string) string {
	if strings.TrimSpace(dir) == "" {
		return ""
	}
	curr := filepath.Clean(dir)
	for i := 0; i < 4; i++ {
		gitPath := filepath.Join(curr, ".git")
		fi, err := os.Stat(gitPath)
		if err == nil {
			headFile := filepath.Join(gitPath, "HEAD")
			if !fi.IsDir() {
				data, rerr := os.ReadFile(gitPath)
				if rerr == nil {
					line := strings.TrimSpace(string(data))
					if strings.HasPrefix(line, "gitdir:") {
						target := strings.TrimSpace(strings.TrimPrefix(line, "gitdir:"))
						if !filepath.IsAbs(target) {
							target = filepath.Join(curr, target)
						}
						headFile = filepath.Join(target, "HEAD")
					}
				}
			}
			headData, herr := os.ReadFile(headFile)
			if herr == nil {
				headStr := strings.TrimSpace(string(headData))
				if strings.HasPrefix(headStr, "ref: refs/heads/") {
					return strings.TrimPrefix(headStr, "ref: refs/heads/")
				}
				if len(headStr) >= 7 {
					return headStr[:7]
				}
			}
			return ""
		}
		parent := filepath.Dir(curr)
		if parent == curr {
			break
		}
		curr = parent
	}
	return ""
}

func promptPlaceholder(hasRunner bool, mode permission.Mode, planMode bool) string {
	if !hasRunner {
		return "Type a message or /command…"
	}
	if planMode {
		return "Ask Protonman to plan or inspect (plan mode · read-only)…"
	}
	switch mode {
	case permission.ModeAlwaysApprove:
		return "Ask Protonman (auto-approve active · commands run without prompt)…"
	case permission.ModeDeny:
		return "Ask Protonman to inspect (deny mode · mutations blocked)…"
	default:
		return "Ask Protonman to inspect or change this workspace…"
	}
}

func (m *bubbleModel) resetTranscript() {
	m.ensureHistoryState().Reset()
	m.followTail = true
	m.showWelcome = true
	m.refreshTranscriptViewport(true)
}

func (m *bubbleModel) resetConversation() {
	m.ensureHistoryState().Reset()
	m.messages = nil
	m.queue = nil
	m.followTail = true
	m.showWelcome = true
	if m.skills != nil {
		m.skills.ResetActivated()
	}
	m.refreshTranscriptViewport(true)
} // resetConversation clears both the visible transcript and provider history.
// Keeping this separate from resetTranscript makes Ctrl+L a safe display-only
// action while /new has the explicit semantics users expect from its name.

func minInt(left int, right int) int {
	if left < right {
		return left
	}
	return right
}

type CrashModel struct {
	errMessage   string
	stackTrace   string
	report       string
	copied       bool
	width        int
	height       int
	scrollOffset int
	restart      bool
	quitting     bool
} // CrashModel is a fullscreen terminal view shown when Protonman encounters a fatal panic or crash.
// Inspired by OpenCode's error-component.tsx.

func NewCrashModel(panicVal any, stack []byte) * // NewCrashModel creates a crash presentation model.
CrashModel {
	msg := fmt.Sprintf("%v", panicVal)
	if msg == "" {
		msg = "Unexpected runtime panic"
	}
	stackStr := strings.TrimSpace(string(stack))
	if stackStr == "" {
		stackStr = strings.TrimSpace(string(debug.Stack()))
	}
	report := BuildCrashReport(msg, stackStr)
	return &CrashModel{errMessage: msg, stackTrace: stackStr, report: report, width: 80, height: 24}
}

func BuildCrashReport(message string, stack string) string {
	var sb strings.Builder
	sb.WriteString("### Protonman Crash Report\n\n")
	sb.WriteString("The Protonman TUI encountered an unexpected error.\n\n")
	sb.WriteString(fmt.Sprintf("**Error:** `%s`\n\n", message))
	sb.WriteString(fmt.Sprintf("**Protonman Version:** `%s`\n", appVersion))
	sb.WriteString(fmt.Sprintf("**Platform:** `%s/%s`\n", runtime.GOOS, runtime.GOARCH))
	sb.WriteString(fmt.Sprintf("**Terminal:** `%s`\n\n", os.Getenv("TERM")))
	sb.WriteString("```\n")
	sb.WriteString(stack)
	sb.WriteString("\n```\n")
	return sb.String()
} // BuildCrashReport formats a GitHub issue bug report with environment context.

func (m *CrashModel) Init() tea.Cmd {
	return nil
}

func (m *CrashModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = maxInt(24, msg.Width)
		m.height = maxInt(10, msg.Height)
		return m, nil
	case tea.KeyPressMsg:
		switch msg.String() {
		case "ctrl+c", "q", "esc":
			m.quitting = true
			return m, tea.Quit
		case "r":
			m.restart = true
			m.quitting = true
			return m, tea.Quit
		case "c":
			_ = copyToClipboard(m.report)
			m.copied = true
			return m, nil
		case "up", "k":
			if m.scrollOffset > 0 {
				m.scrollOffset--
			}
			return m, nil
		case "down", "j":
			lines := strings.Split(m.stackTrace, "\n")
			maxScroll := maxInt(0, len(lines)-4)
			if m.scrollOffset < maxScroll {
				m.scrollOffset++
			}
			return m, nil
		case "pgup":
			m.scrollOffset = maxInt(0, m.scrollOffset-5)
			return m, nil
		case "pgdown":
			lines := strings.Split(m.stackTrace, "\n")
			maxScroll := maxInt(0, len(lines)-4)
			m.scrollOffset = minInt(maxScroll, m.scrollOffset+5)
			return m, nil
		}
	}
	return m, nil
}

func (m *CrashModel) View() tea.View {
	contentWidth := minInt(84, maxInt(24, m.width-4))
	innerWidth := contentWidth - 4
	var parts []string
	headline := brandStyle.Render("Protonman crashed")
	subtext := mutedStyle.Render("An unexpected error stopped the session.")
	parts = append(parts, lipgloss.JoinVertical(lipgloss.Center, headline, subtext))
	errBoxStyle := lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(accentError).Padding(0, 1).Width(contentWidth)
	errText := safeWrappedLines(m.errMessage, innerWidth)
	parts = append(parts, errBoxStyle.Render(lipgloss.NewStyle().Foreground(accentError).Bold(true).Render(strings.Join(errText, "\n"))))
	copyLabel := "[c] Copy report"
	if m.copied {
		copyLabel = "[✓ Copied to clipboard]"
	}
	copyStyle := lipgloss.NewStyle().Foreground(accentSuccess).Bold(m.copied)
	if !m.copied {
		copyStyle = lipgloss.NewStyle().Foreground(accentUser)
	}
	actions := lipgloss.JoinHorizontal(lipgloss.Center, copyStyle.Render(copyLabel), "   ", commandStyle.Render("[r] Restart"), "   ", mutedStyle.Render("[q] Quit"))
	parts = append(parts, actions)
	stackLines := strings.Split(m.stackTrace, "\n")
	availableHeight := maxInt(4, m.height-len(strings.Split(lipgloss.JoinVertical(lipgloss.Left, parts...), "\n"))-4)
	visibleLines := make([]string, 0, availableHeight)
	start := minInt(len(stackLines), m.scrollOffset)
	end := minInt(len(stackLines), start+availableHeight)
	for i := start; i < end; i++ {
		visibleLines = append(visibleLines, stackLines[i])
	}
	stackBoxStyle := lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("8")).Padding(0, 1).Width(contentWidth)
	stackHeader := mutedStyle.Render(fmt.Sprintf("Stack trace (lines %d-%d of %d, ↑/↓ scroll):", start+1, end, len(stackLines)))
	stackBody := strings.Join(visibleLines, "\n")
	parts = append(parts, stackBoxStyle.Render(lipgloss.JoinVertical(lipgloss.Left, stackHeader, mutedStyle.Render(stackBody))))
	footer := mutedStyle.Render(fmt.Sprintf("Protonman %s · %s/%s", appVersion, runtime.GOOS, runtime.GOARCH))
	parts = append(parts, footer)
	mainContent := lipgloss.JoinVertical(lipgloss.Center, parts...)
	view := tea.NewView(lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, mainContent))
	view.AltScreen = true
	return view
}

func copyToClipboard(text string) error {
	encoded := base64.StdEncoding.EncodeToString([]byte(text))
	osc52 := fmt.Sprintf("\x1b]52;c;%s\x07", encoded)
	_, _ = os.Stdout.WriteString(osc52)
	switch runtime.GOOS {
	case "darwin":
		cmd := exec.Command("pbcopy")
		cmd.Stdin = strings.NewReader(text)
		_ = cmd.Run()
	case "linux":
		if _, err := exec.LookPath("wl-copy"); err == nil {
			cmd := exec.Command("wl-copy")
			cmd.Stdin = strings.NewReader(text)
			_ = cmd.Run()
		} else if _, err := exec.LookPath("xclip"); err == nil {
			cmd := exec.Command("xclip", "-selection", "clipboard")
			cmd.Stdin = strings.NewReader(text)
			_ = cmd.Run()
		}
	}
	return nil
}

func shortcutHelp(binding key.Binding) string {
	help := binding.Help()
	return strings.TrimSpace(help.Key + " " + help.Desc)
}

func (m bubbleModel) statusView() string {
	if m.hasPermissionView() {
		return warningStyle.Render(truncateWithEllipsis("action required · permission", maxInt(1, m.width-2)))
	}
	if !m.busy {
		return ""
	}
	activeAgents, _, _, _ := agentActivityCounts(m.turnAgentSnapshot())
	activity := strings.TrimSpace(m.activity)
	if activeAgents > 0 {
		label := "agent"
		if activeAgents != 1 {
			label = "agents"
		}
		if activity == "canceling" {
			activity = fmt.Sprintf("stopping %d %s", activeAgents, label)
		} else {
			activity = fmt.Sprintf("%d %s working", activeAgents, label)
		}
	}
	if activity == "" || activity == "ready" {
		activity = "analyzing"
	}
	indicator := "● "
	if spin := m.spinner.View(); spin != "" {
		indicator = spin + " "
	}
	return statusStyle.Render(truncateWithEllipsis(indicator+activity, maxInt(1, m.width-2)))
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
	if view := m.permissionView(); view != nil {
		if view.parked {
			return mutedStyle.Render("tab review · y once · s session · n deny")
		}
		return mutedStyle.Render("y once · s session · n deny · esc review")
	}
	targetWidth := maxInt(1, m.width-2)
	parts := make([]string, 0, 3)
	if modelID := strings.TrimSpace(m.activeModel); modelID != "" {
		parts = append(parts, brandStyle.Render(truncateWithEllipsis(modelID, maxInt(8, targetWidth/2))))
	}
	if m.planMode {
		parts = append(parts, planStyle.Render("plan"))
	} else if m.service != nil && m.service.Mode() == permission.ModeAlwaysApprove {
		parts = append(parts, warningStyle.Render("auto"))
	}
	if m.reasoningEffort != sdk.ReasoningDefault && m.reasoningEffort != "" {
		parts = append(parts, mutedStyle.Render(string(m.reasoningEffort)))
	}
	return truncateWithEllipsis(strings.Join(parts, mutedStyle.Render(glyphSep)), targetWidth)
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
	if m.width < 40 {
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

func (m bubbleModel) shortcutHint() string {
	if view := m.permissionView(); view != nil {
		return m.infoView()
	}
	if m.slashOpen() {
		return mutedStyle.Render("tab accept · enter run · esc close · ↑↓ move")
	}
	helpView := m.help
	helpView.ShowAll = false
	helpView.SetWidth(maxInt(1, m.width-2))
	return helpView.View(m.keys)
}

func (m *bubbleModel) cycleMode() {
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
}

func (m *bubbleModel) setPlanMode(argument string) {
	enabled := m.planMode
	switch strings.ToLower(argument) {
	case "":
		enabled = !enabled
	case "on", "true":
		enabled = true
	case "off", "false":
		enabled = false
	default:
		m.appendError("usage: /plan [on|off]")
		return
	}
	if enabled && m.service.Mode() != permission.ModeAsk && m.service.Mode() != permission.ModeAuto {
		_ = m.setPermissionMode(permission.ModeAsk)
	}
	m.setPlanEnabled(enabled)
	state := "off"
	if m.planMode {
		state = "on (read-only)"
	}
	m.appendLine("plan mode: " + state)
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

func formatElapsed(duration time.Duration) string {
	return pane.FormatElapsed(duration)
}

func (m bubbleModel) agentsView() string {
	snapshot := m.agentSnapshot
	if m.busy && m.activeTurnOwner != "" {
		snapshot = m.turnAgentSnapshot()
	}
	live := make([]agent.AgentStatus, 0, len(snapshot))
	for _, st := range snapshot {
		if !st.State.Terminal() {
			live = append(live, st)
		}
	}
	snapshot = live
	if layoutModeForHeight(m.height) == layoutTiny || len(snapshot) == 0 {
		return ""
	}
	queued, running, canceling := 0, 0, 0
	for _, st := range snapshot {
		switch st.State {
		case agent.StateQueued:
			queued++
		case agent.StateRunning:
			running++
		case agent.StateCanceling:
			canceling++
		}
	}
	active := queued + running + canceling
	summary := fmt.Sprintf("Agents %d active", active)
	if running > 0 {
		summary += fmt.Sprintf(" · %d running", running)
	}
	if queued > 0 {
		summary += fmt.Sprintf(" · %d queued", queued)
	}
	if canceling > 0 {
		summary += fmt.Sprintf(" · %d canceling", canceling)
	}
	mode := layoutModeForHeight(m.height)
	visible := append([]agent.AgentStatus(nil), snapshot...)
	sort.SliceStable(visible, func(i, j int) bool {
		return agentDisplayPriority(visible[i].State) < agentDisplayPriority(visible[j].State)
	})
	limit := 3
	if mode == layoutCompact {
		limit = 1
	}
	if len(visible) > limit {
		visible = visible[:limit]
	}
	lines := []string{brandStyle.Render(summary)}
	for _, st := range visible {
		stateGlyph := glyphAgent
		style := mutedStyle
		switch st.State {
		case agent.StateCompleted:
			stateGlyph = glyphToolSuccess
			style = successStyle
		case agent.StateFailed, agent.StateCanceled:
			stateGlyph = glyphToolError
			style = errorStyle
		case agent.StateCanceling:
			style = warningStyle
		}
		elapsed := formatElapsed(agentDisplayDuration(st, time.Now()))
		task := strings.TrimSpace(st.Task)
		if task == "" {
			task = st.ID
		}
		identity := agentDisplayProfile(st)
		line := fmt.Sprintf("  %s%s · %s · %s", stateGlyph, identity, elapsed, truncateWithEllipsis(task, maxInt(12, m.width-30)))
		lines = append(lines, style.Render(truncateWithEllipsis(line, maxInt(1, m.width-2))))
		detail := ""
		if st.State.Terminal() {
			detail = strings.TrimSpace(st.Reason)
		} else {
			detail = m.agentActivity[st.ID].String()
		}
		if detail != "" && mode == layoutNormal {
			lines = append(lines, mutedStyle.Render("    "+truncateWithEllipsis(detail, maxInt(8, m.width-6))))
		}
	}
	if more := len(snapshot) - len(visible); more > 0 {
		lines = append(lines, mutedStyle.Render(fmt.Sprintf("  … %d older", more)))
	}
	return strings.Join(lines, "\n")
}

func agentDisplayProfile(st agent.AgentStatus) string {
	return pane.AgentDisplayProfile(st)
}

func agentDisplayPriority(state agent.State) int {
	return pane.AgentDisplayPriority(state)
}

func agentDisplayDuration(st agent.AgentStatus, now time.Time) time.Duration {
	return pane.AgentDisplayDuration(st, now)
}

func (m *bubbleModel) promptView() string {
	if m.bottom == nil || m.bottom.prompt() == nil {
		return ""
	}
	return m.bottom.prompt().View()
}
