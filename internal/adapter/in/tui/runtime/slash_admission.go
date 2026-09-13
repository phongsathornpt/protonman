package runtime

import (
	"fmt"
	"strings"

	"github.com/phongsathornpt/protonman/internal/adapter/in/tui/runtime/cmdpolicy"
)

func (m *bubbleModel) rejectBlockedSlashCommand(line string) bool {
	if m == nil || !strings.HasPrefix(strings.TrimSpace(line), "/") {
		return false
	}
	if !m.busy && !m.hasPermissionView() {
		return false
	}

	cmd := cmdpolicy.Classify(line)
	if !slashCommandRequiresIdle(cmd) {
		return false
	}

	m.appendError(blockedSlashCommandMessage(cmd, m.busy))
	m.refreshViewport()
	return true
}

func slashCommandRequiresIdle(cmd cmdpolicy.Command) bool {
	switch cmd.Kind {
	case cmdpolicy.KindHelp, cmdpolicy.KindTodo, cmdpolicy.KindAgents, cmdpolicy.KindQuit, cmdpolicy.KindUnknown:
		return false
	case cmdpolicy.KindGoal:
		return strings.TrimSpace(cmd.Rest) != ""
	case cmdpolicy.KindSkills:
		arg := strings.ToLower(strings.TrimSpace(cmd.Argument))
		return arg != "active" && arg != "check" && arg != "verify"
	case cmdpolicy.KindProvider:
		return !strings.EqualFold(strings.TrimSpace(cmd.Argument), "list")
	case cmdpolicy.KindPermission, cmdpolicy.KindLow, cmdpolicy.KindClear, cmdpolicy.KindResume, cmdpolicy.KindModel, cmdpolicy.KindCall:
		return true
	default:
		return true
	}
}

func blockedSlashCommandMessage(cmd cmdpolicy.Command, busy bool) string {
	if !busy {
		return fmt.Sprintf("cannot run /%s while a permission request is active", cmd.Name)
	}
	switch cmd.Kind {
	case cmdpolicy.KindPermission:
		return "cannot change permission mode while a turn is running"
	case cmdpolicy.KindLow:
		return "cannot change low concurrency mode while a turn is running"
	case cmdpolicy.KindSkills:
		return "cannot change skills while a turn is running"
	case cmdpolicy.KindGoal:
		return "cannot change goal while a turn is running"
	case cmdpolicy.KindClear:
		return "cannot clear conversation while a turn is running"
	case cmdpolicy.KindResume:
		return "cannot switch session while a turn is running"
	case cmdpolicy.KindModel:
		return "cannot change model while a turn is running"
	case cmdpolicy.KindProvider:
		return "cannot change provider while a turn is running"
	case cmdpolicy.KindCall:
		return "cannot start a direct tool call while a turn is running"
	default:
		return fmt.Sprintf("cannot run /%s while a turn is running", cmd.Name)
	}
}
