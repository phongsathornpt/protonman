package runtime

import (
	"encoding/json"
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/phongsathornpt/protonman/internal/adapter/in/tui/runtime/permissionpolicy"
	panecommon "github.com/phongsathornpt/protonman/internal/adapter/in/tui/view/pane/common"
	permissionpane "github.com/phongsathornpt/protonman/internal/adapter/in/tui/view/pane/permission"
	"github.com/phongsathornpt/protonman/internal/core/permission"
	"github.com/phongsathornpt/protonman/internal/core/tool"
)

func (v *permissionPaneView) card(ctx paneRenderContext) string {
	request := v.pending.request
	options := permissionpolicy.Options(v.pending.request, ctx.projectTrusted, ctx.hasWorkDir)
	labels := make([]string, 0, len(options))
	for _, option := range options {
		labels = append(labels, option.Label)
	}
	shortcutHint := shortcutHintFor(options)
	title := "Permission required"
	tone := panecommon.ToneWarning
	detailExtras := make([]string, 0, 3)
	switch request.ToolKind {
	case permission.ToolRead, permission.ToolGrep, permission.ToolTask, permission.ToolAgent:
		switch request.ToolKind {
		case permission.ToolTask:
			title = "Task plan change"
		case permission.ToolAgent:
			title = "Agent orchestration"
		default:
			title = "Permission request — read only"
		}
		tone = panecommon.ToneUser
	case permission.ToolEdit:
		title = "Permission required — modifies workspace"
		tone = panecommon.ToneError
	case permission.ToolBash:
		var input struct {
			Command string `json:"command"`
			Cwd     string `json:"cwd,omitempty"`
		}
		_ = json.Unmarshal(request.Arguments, &input)
		analysis := tool.AnalyzeCommand(input.Command)
		switch analysis.Scope {
		case tool.CommandScopePublish:
			title, tone = "Permission required — publishes package", panecommon.ToneError
		case tool.CommandScopeDeployment:
			if analysis.Risk == tool.CommandRiskRemoteDestructive {
				title = "Permission required — destructive deployment change"
			} else {
				title = "Permission required — changes deployment"
			}
			tone = panecommon.ToneError
		case tool.CommandScopeRemote:
			if analysis.Risk == tool.CommandRiskRemoteDestructive {
				title = "Permission required — destructively modifies remote"
			} else {
				title = "Permission required — modifies remote"
			}
			tone = panecommon.ToneError
		default:
			switch analysis.Effect {
			case tool.CommandEffectReadOnly:
				title, tone = "Permission request — shell read only", panecommon.ToneUser
			case tool.CommandEffectMutating:
				title, tone = "Permission required — shell modifies state", panecommon.ToneError
			default:
				title = "Permission required — shell effects unknown"
			}
		}
		cwd := strings.TrimSpace(input.Cwd)
		if cwd == "" {
			cwd = "."
		}
		detailExtras = append(detailExtras, "Cwd: "+cwd)
		if analysis.Scope != tool.CommandScopeUnknown {
			detailExtras = append(detailExtras, "Scope: "+string(analysis.Scope))
		}
		if analysis.Reason != "" {
			detailExtras = append(detailExtras, fmt.Sprintf("Effect: %s · %s", analysis.Effect, analysis.Reason))
		}
	}
	result := permissionpane.PermissionView(permissionpane.PermissionSnapshot{
		Width:        ctx.width,
		Height:       ctx.height,
		Parked:       v.parked,
		Index:        v.index,
		Title:        title,
		Tone:         tone,
		ToolName:     tool.DisplayName(request.ToolName),
		ToolKind:     string(request.ToolKind),
		Detail:       request.Detail,
		DetailExtras: detailExtras,
		Options:      labels,
		ShortcutHint: shortcutHint,
	})
	if result.Inline != "" {
		return result.Inline
	}
	rows := result.Rows
	if len(rows) > 1 && layoutModeForHeight(ctx.height) == layoutNormal {
		rows = append(rows[:1], append([]string{""}, rows[1:]...)...)
	}
	helpBindings := []string{"↑/↓", "Navigate", "enter", "Choose", "esc", "Review"}
	if v.parked {
		helpBindings = []string{"tab", "Review", "pgup/pgdn", "Scroll", "esc", "Back"}
	}
	rows = append(rows, "", paneKeyboardHelp(ctx.width-4, helpBindings...))
	status := tool.DisplayName(request.ToolName)
	if len(labels) > 0 {
		index := maxInt(0, minInt(v.index, len(labels)-1))
		status += " · " + labels[index]
	}
	rows = append(rows, paneRightStatus(ctx.width, status))
	return renderModalRows(ctx, paneToneColor(result.Tone), rows)
}

type permissionRequestMsg struct{ request permissionRequest }
type permissionBridgeClosedMsg struct{}

type permissionRuleSavedMsg struct {
	scope string
	rule  permission.Rule
	err   error
}

func (m *bubbleModel) updatePermissionRuleSaved(message permissionRuleSavedMsg) (tea.Model, tea.Cmd) {
	if message.err != nil {
		m.appendError(fmt.Sprintf("Failed to save permission rule to %s: %s", message.scope, message.err))
		m.refreshViewport()
		return m, nil
	}
	m.appendLine(successStyle.Render(fmt.Sprintf("Saved %s rule to %s config (%s: %s).", message.rule.Action, message.scope, message.rule.Tool, message.rule.Pattern)))
	m.refreshViewport()
	return m, nil
}
