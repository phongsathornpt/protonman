package runtime

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/phongsathornpt/protonman/internal/adapter/in/tui/runtime/paneutil"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"github.com/phongsathornpt/protonman/internal/adapter/in/tui/runtime/permissionbridge"
	"github.com/phongsathornpt/protonman/internal/adapter/in/tui/runtime/permissionpolicy"
	"github.com/phongsathornpt/protonman/internal/adapter/in/tui/state/runtimeui"
	panecommon "github.com/phongsathornpt/protonman/internal/adapter/in/tui/view/pane/common"
	permissionpane "github.com/phongsathornpt/protonman/internal/adapter/in/tui/view/pane/permission"
	"github.com/phongsathornpt/protonman/internal/base/diffutil"
	"github.com/phongsathornpt/protonman/internal/core/permission"
	"github.com/phongsathornpt/protonman/internal/core/tool"
)

const permissionViewID = "permission"

type permissionOption = permissionpolicy.Option

const (
	optionAllowOnce    = permissionpolicy.AllowOnce
	optionAllowSession = permissionpolicy.AllowSession
	optionAllowProject = permissionpolicy.AllowProject
	optionAllowGlobal  = permissionpolicy.AllowGlobal
	optionDeny         = permissionpolicy.Deny
)

type permissionOptionItem = permissionpolicy.Item

func shortcutHintFor(options []permissionOptionItem) string {
	return permissionpolicy.ShortcutHint(options)
}

type permissionPaneView struct {
	pending      permissionRequest
	parked       bool
	index        int
	initialized  bool
	title        string
	tone         panecommon.Tone
	detailExtras []string
	diffPreview  []string
	diffOmitted  int
}

func (*permissionPaneView) ID() string                             { return permissionViewID }
func (*permissionPaneView) PresentationMode() panePresentationMode { return paneBlocking }
func (v *permissionPaneView) Render(ctx paneRenderContext) string {
	return v.card(ctx)
}

func (v *permissionPaneView) HandlePaneKey(ctx paneRenderContext, message tea.KeyPressMsg) paneKeyResult {
	options := permissionpolicy.Options(v.pending.Request, ctx.projectTrusted, ctx.hasWorkDir)
	if v.index >= len(options) {
		v.index = len(options) - 1
	}
	if v.index < 0 {
		v.index = 0
	}
	if v.parked {
		switch {
		case key.Matches(message, paneutil.Keys.Tab):
			v.parked = false
			return paneKeyResult{handled: true, action: paneAction{kind: paneActionPermissionActivity, activity: runtimeui.ActivityWaitingForPermission}}
		case key.Matches(message, paneutil.Keys.Page):
			return paneKeyResult{handled: true, action: paneAction{kind: paneActionScrollPage, key: message}}
		case key.Matches(message, paneutil.Keys.Up):
			return paneKeyResult{handled: true, action: paneAction{kind: paneActionScrollLines, scrollLines: -1}}
		case key.Matches(message, paneutil.Keys.Down):
			return paneKeyResult{handled: true, action: paneAction{kind: paneActionScrollLines, scrollLines: 1}}
		case message.Text == "y" || message.Text == "s" || message.Text == "p" || message.Text == "g" || message.Text == "n" ||
			(message.Text >= "1" && message.Text <= "5") || key.Matches(message, paneutil.Keys.Confirm):
			// Decisions remain available while reviewing the transcript.
		default:
			return paneKeyResult{handled: true}
		}
	}

	resolve := func(option permissionOption) paneKeyResult {
		return paneKeyResult{handled: true, action: paneAction{kind: paneActionPermissionResolve, permission: option}}
	}
	switch {
	case key.Matches(message, paneutil.Keys.Escape):
		v.parked = true
		return paneKeyResult{handled: true, action: paneAction{kind: paneActionPermissionActivity, activity: "permission pending — tab to review"}}
	case key.Matches(message, paneutil.Keys.Up):
		if v.index > 0 {
			v.index--
		}
		return paneKeyResult{handled: true}
	case key.Matches(message, paneutil.Keys.Down):
		if v.index < len(options)-1 {
			v.index++
		}
		return paneKeyResult{handled: true}
	case message.Text >= "1" && message.Text <= "5":
		idx := int(message.Text[0] - '1')
		if idx >= 0 && idx < len(options) {
			return resolve(options[idx].Option)
		}
		return paneKeyResult{handled: true}
	case message.Text == "y":
		return resolve(optionAllowOnce)
	case message.Text == "s":
		for _, item := range options {
			if item.Option == optionAllowSession {
				return resolve(optionAllowSession)
			}
		}
		return paneKeyResult{handled: true}
	case message.Text == "p":
		for _, item := range options {
			if item.Option == optionAllowProject {
				return resolve(optionAllowProject)
			}
		}
		return paneKeyResult{handled: true}
	case message.Text == "g":
		for _, item := range options {
			if item.Option == optionAllowGlobal {
				return resolve(optionAllowGlobal)
			}
		}
		return paneKeyResult{handled: true}
	case message.Text == "n":
		return resolve(optionDeny)
	case key.Matches(message, paneutil.Keys.Confirm):
		if len(options) == 0 {
			return paneKeyResult{handled: true}
		}
		return resolve(options[v.index].Option)
	default:
		return paneKeyResult{handled: true}
	}
}

func (v *permissionPaneView) ensureInitialized(workDir string) {
	if v.initialized {
		return
	}
	v.initialized = true
	request := v.pending.Request
	v.title = "Permission required"
	v.tone = panecommon.ToneWarning
	v.detailExtras = make([]string, 0, 3)

	switch request.ToolKind {
	case permission.ToolRead, permission.ToolGrep, permission.ToolTask, permission.ToolAgent:
		switch request.ToolKind {
		case permission.ToolTask:
			v.title = "Task plan change"
		case permission.ToolAgent:
			v.title = "Agent orchestration"
		default:
			v.title = "Permission request — read only"
		}
		v.tone = panecommon.ToneUser
	case permission.ToolEdit:
		v.title = "Permission required — modifies workspace"
		v.tone = panecommon.ToneError
		var editInput permissionEditInput
		_ = json.Unmarshal(request.Arguments, &editInput)
		targetPath := editInput.FilePath
		if targetPath == "" {
			targetPath = editInput.Path
		}
		action := strings.ToLower(strings.TrimSpace(editInput.Action))
		if action == "" {
			action = "edit"
		}
		switch action {
		case "replace":
			v.describeReplaceAction(editInput, targetPath)
		case "patch":
			v.describePatchAction(editInput)
		case "write":
			v.describeWriteAction(editInput, targetPath, workDir)
		case "restore":
			v.detailExtras = append(v.detailExtras, "Action: restore checkpoint")
		}
	case permission.ToolBash:
		v.describeBashAction(request.Arguments)
	}
}

// permissionEditInput is the parsed argument set of an edit tool call.
type permissionEditInput struct {
	Action    string `json:"action"`
	FilePath  string `json:"filePath"`
	Path      string `json:"path"`
	OldString string `json:"oldString"`
	NewString string `json:"newString"`
	Patch     string `json:"patch"`
	Content   string `json:"content"`
}

func (v *permissionPaneView) describeReplaceAction(editInput permissionEditInput, targetPath string) {
	if editInput.OldString != "" || editInput.NewString != "" {
		diff := diffutil.UnifiedDiff(editInput.OldString, editInput.NewString, targetPath, 1)
		if diff != "" {
			adds, dels := diffutil.DiffStats(diff)
			badge := diffutil.StatBadge(adds, dels)
			if badge != "" {
				v.detailExtras = append(v.detailExtras, fmt.Sprintf("Action: replace · %s", badge))
			} else {
				v.detailExtras = append(v.detailExtras, "Action: replace")
			}
			v.diffPreview, v.diffOmitted = diffutil.ExtractPreview(diff, 6)
		} else {
			v.detailExtras = append(v.detailExtras, "Action: replace")
		}
	} else {
		v.detailExtras = append(v.detailExtras, "Action: replace")
	}
}

func (v *permissionPaneView) describePatchAction(editInput permissionEditInput) {
	if editInput.Patch != "" {
		adds, dels := diffutil.DiffStats(editInput.Patch)
		badge := diffutil.StatBadge(adds, dels)
		targets := extractPatchTargets(editInput.Patch)
		if len(targets) > 0 {
			targetStr := strings.Join(targets, ", ")
			if len(targets) > 3 {
				targetStr = fmt.Sprintf("%d files (%s, …)", len(targets), strings.Join(targets[:3], ", "))
			}
			if badge != "" {
				v.detailExtras = append(v.detailExtras, fmt.Sprintf("Action: patch · %s · targets: %s", badge, targetStr))
			} else {
				v.detailExtras = append(v.detailExtras, fmt.Sprintf("Action: patch · targets: %s", targetStr))
			}
		} else if badge != "" {
			v.detailExtras = append(v.detailExtras, fmt.Sprintf("Action: patch · %s", badge))
		} else {
			v.detailExtras = append(v.detailExtras, "Action: patch")
		}
		v.diffPreview, v.diffOmitted = diffutil.ExtractPreview(editInput.Patch, 6)
	} else {
		v.detailExtras = append(v.detailExtras, "Action: patch")
	}
}

// describeWriteAction builds the write detail line and diff preview. It may
// read the workspace via readExistingFileForDiff, so it must only run from
// ensureInitialized at open time — never from Render/card.
func (v *permissionPaneView) describeWriteAction(editInput permissionEditInput, targetPath, workDir string) {
	if editInput.Content != "" || targetPath != "" {
		existingBytes, exists := readExistingFileForDiff(workDir, targetPath)
		var diff string
		if exists {
			diff = diffutil.UnifiedDiff(string(existingBytes), editInput.Content, targetPath, 1)
		} else {
			diff = diffutil.NewFileDiff(targetPath, editInput.Content, 1)
		}
		if diff != "" {
			adds, dels := diffutil.DiffStats(diff)
			badge := diffutil.StatBadge(adds, dels)
			if badge != "" {
				v.detailExtras = append(v.detailExtras, fmt.Sprintf("Action: write · %s (%d bytes)", badge, len(editInput.Content)))
			} else if editInput.Content == "" {
				v.detailExtras = append(v.detailExtras, "Action: write (empty file)")
			} else {
				lineCount := strings.Count(editInput.Content, "\n") + 1
				lineWord := "lines"
				if lineCount == 1 {
					lineWord = "line"
				}
				v.detailExtras = append(v.detailExtras, fmt.Sprintf("Action: write · %d %s (%d bytes)", lineCount, lineWord, len(editInput.Content)))
			}
			v.diffPreview, v.diffOmitted = diffutil.ExtractPreview(diff, 6)
		} else {
			if exists {
				v.detailExtras = append(v.detailExtras, "Action: write (no changes)")
			} else {
				v.detailExtras = append(v.detailExtras, "Action: write (empty file)")
			}
		}
	} else {
		v.detailExtras = append(v.detailExtras, "Action: write (empty file)")
	}
}

func (v *permissionPaneView) describeBashAction(arguments json.RawMessage) {
	var input struct {
		Command string `json:"command"`
		Cwd     string `json:"cwd,omitempty"`
	}
	_ = json.Unmarshal(arguments, &input)
	analysis := tool.AnalyzeCommand(input.Command)
	switch analysis.Scope {
	case tool.CommandScopePublish:
		v.title, v.tone = "Permission required — publishes package", panecommon.ToneError
	case tool.CommandScopeDeployment:
		if analysis.Risk == tool.CommandRiskRemoteDestructive {
			v.title = "Permission required — destructive deployment change"
		} else {
			v.title = "Permission required — changes deployment"
		}
		v.tone = panecommon.ToneError
	case tool.CommandScopeRemote:
		if analysis.Risk == tool.CommandRiskRemoteDestructive {
			v.title = "Permission required — destructively modifies remote"
		} else {
			v.title = "Permission required — modifies remote"
		}
		v.tone = panecommon.ToneError
	default:
		switch analysis.Effect {
		case tool.CommandEffectReadOnly:
			v.title, v.tone = "Permission request — shell read only", panecommon.ToneUser
		case tool.CommandEffectMutating:
			v.title, v.tone = "Permission required — shell modifies state", panecommon.ToneError
		default:
			v.title = "Permission required — shell effects unknown"
		}
	}
	cwd := strings.TrimSpace(input.Cwd)
	if cwd == "" {
		cwd = "."
	}
	v.detailExtras = append(v.detailExtras, "Cwd: "+cwd)
	if analysis.Scope != tool.CommandScopeUnknown {
		v.detailExtras = append(v.detailExtras, "Scope: "+string(analysis.Scope))
	}
	if analysis.Reason != "" {
		v.detailExtras = append(v.detailExtras, fmt.Sprintf("Effect: %s · %s", analysis.Effect, analysis.Reason))
	}
}

// card renders the already-initialized permission snapshot. Initialization is
// owned by openPermission at the Update boundary; Render stays read-only.
func (v *permissionPaneView) card(ctx paneRenderContext) string {
	request := v.pending.Request
	options := permissionpolicy.Options(v.pending.Request, ctx.projectTrusted, ctx.hasWorkDir)
	labels := make([]string, 0, len(options))
	for _, option := range options {
		labels = append(labels, option.Label)
	}
	shortcutHint := shortcutHintFor(options)

	result := permissionpane.PermissionView(permissionpane.PermissionSnapshot{
		Width:        ctx.width,
		Height:       ctx.height,
		Parked:       v.parked,
		Index:        v.index,
		Title:        v.title,
		Tone:         v.tone,
		ToolName:     tool.DisplayName(request.ToolName),
		ToolKind:     string(request.ToolKind),
		Detail:       request.Detail,
		DetailExtras: v.detailExtras,
		DiffPreview:  v.diffPreview,
		DiffOmitted:  v.diffOmitted,
		Options:      labels,
		ShortcutHint: shortcutHint,
	})
	if result.Inline != "" {
		return result.Inline
	}
	rows := result.Rows
	if len(rows) > 1 && layoutModeForHeight(ctx.height) == layoutNormal {
		rows = appendPaneGroup(rows[:1], rows[1:]...)
	}
	helpBindings := []string{"↑/↓", "Navigate", "enter", "Choose", "esc", "Review"}
	if v.parked {
		helpBindings = []string{"tab", "Review", "pgup/pgdn", "Scroll", "esc", "Back"}
	}
	rows = appendPaneGroup(rows, paneKeyboardHelp(panecommon.PaneHelpWidth(ctx.width), helpBindings...))
	status := tool.DisplayName(request.ToolName)
	if len(labels) > 0 {
		index := max(0, min(v.index, len(labels)-1))
		status += " · " + labels[index]
	}
	rows = append(rows, paneRightStatus(panecommon.PaneHelpWidth(ctx.width), status))
	return renderModalRows(ctx, paneToneColor(result.Tone), rows)
}

type permissionRequest = permissionbridge.Request
type permissionResponse = permissionbridge.Response
type permissionRequestMsg = permissionbridge.RequestMsg
type permissionBridgeClosedMsg = permissionbridge.ClosedMsg

type permissionRuleSavedMsg struct {
	scope string
	rule  permission.Rule
	err   error
}

func (m *bubbleModel) updatePermissionRuleSaved(message permissionRuleSavedMsg) tea.Cmd {
	if message.err != nil {
		m.appendError(fmt.Sprintf("Failed to save permission rule to %s: %s", message.scope, message.err))
		m.refreshViewport()
		return nil
	}
	m.appendLine(successStyle.Render(fmt.Sprintf("Saved %s rule to %s config (%s: %s).", message.rule.Action, message.scope, message.rule.Tool, message.rule.Pattern)))
	m.refreshViewport()
	return nil
}

func (m *bubbleModel) permissionView() *permissionPaneView {
	if m.panes.bottom == nil {
		return nil
	}
	view, _ := m.panes.bottom.find(permissionViewID).(*permissionPaneView)
	return view
}

func (m *bubbleModel) hasPermissionView() bool { return m.permissionView() != nil }

func (m *bubbleModel) openPermission(request permissionRequest) {
	if m.panes.bottom == nil {
		return
	}
	view := &permissionPaneView{pending: request}
	// Initialize at the Update boundary: derivation may read the workspace
	// (write-action diff preview), which must never happen during Render.
	view.ensureInitialized(m.workDir)
	m.panes.bottom.push(view)
	m.requestRelayout()
	m.reconcileLayout()
	if !runtimeui.IsWaitingForPermission(m.activity) {
		m.pendingActivity = m.activity
	}
	m.activity = runtimeui.ActivityWaitingForPermission
}

func (m *bubbleModel) resolvePermission(option permissionOption) tea.Cmd {
	view := m.permissionView()
	if view == nil {
		return nil
	}
	decision := permissionpolicy.Resolve(option)
	resolution := decision.Resolution
	var saveCmd tea.Cmd
	request := view.pending.Request

	if decision.Persist != permissionpolicy.PersistNone {
		if rule, ok := permission.RuleFromRequest(request); ok {
			if ruleErr := m.service.AddRule(rule); ruleErr != nil {
				// Report instead of dropping: the follow-on "saved" notice must
				// not imply the live policy accepted a rule it rejected.
				m.appendError(fmt.Sprintf("failed to apply permission rule: %v", ruleErr))
			}
			switch decision.Persist {
			case permissionpolicy.PersistProject:
				workDir := m.workDir
				saveCmd = func() tea.Msg {
					err := m.application.Projects.SavePermissionRule(workDir, rule)
					return permissionRuleSavedMsg{scope: string(decision.Persist), rule: rule, err: err}
				}
			case permissionpolicy.PersistGlobal:
				saveCmd = func() tea.Msg {
					err := m.application.UserSettings.SavePermissionRule(rule)
					return permissionRuleSavedMsg{scope: string(decision.Persist), rule: rule, err: err}
				}
			}
		}
	}
	view.pending.Respond(resolution)
	m.panes.bottom.remove(permissionViewID)
	m.requestRelayout()
	m.reconcileLayout()
	m.activity = m.pendingActivity
	if m.activity == "" || runtimeui.IsWaitingForPermission(m.activity) {
		m.activity = "running tool"
	}
	m.syncSlashView()
	return saveCmd
}

func extractPatchTargets(patch string) []string {
	lines := strings.Split(patch, "\n")
	targets := make([]string, 0, 2)
	for _, l := range lines {
		trimmed := strings.TrimSpace(l)
		for _, prefix := range []string{"*** Update File: ", "*** Add File: ", "*** Delete File: "} {
			if strings.HasPrefix(trimmed, prefix) {
				path := strings.TrimSpace(strings.TrimPrefix(trimmed, prefix))
				if path != "" {
					targets = append(targets, path)
				}
			}
		}
	}
	return targets
}

func readExistingFileForDiff(workDir, targetPath string) ([]byte, bool) {
	if targetPath == "" {
		return nil, false
	}
	fullPath := targetPath
	if workDir != "" && !filepath.IsAbs(fullPath) {
		fullPath = filepath.Join(workDir, fullPath)
	}
	fi, err := os.Lstat(fullPath)
	if err != nil || !fi.Mode().IsRegular() {
		return nil, false
	}
	if fi.Size() > diffutil.MaxDiffInputBytes {
		return nil, false
	}
	f, err := os.Open(fullPath)
	if err != nil {
		return nil, false
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, diffutil.MaxDiffInputBytes+1))
	if err != nil || int64(len(data)) > diffutil.MaxDiffInputBytes {
		return nil, false
	}
	return data, true
}
