package agentui

import (
	"strings"

	"github.com/phongsathornpt/protonman/internal/adapter/in/tui/view/toolview"
	"github.com/phongsathornpt/protonman/internal/core/tool"
	"github.com/phongsathornpt/protonman/internal/feature/agent"
)

type Activity struct {
	ToolName string
	ToolKind tool.Kind
	Target   string
	Label    string
}

func ActivityFromEvent(ev agent.Event) Activity {
	if ev.Call == nil {
		return Activity{Label: strings.TrimSpace(ev.Message)}
	}
	target, kind := toolview.ExtractTarget(ev.Call.Name, "", ev.Call.Arguments)
	activity := Activity{ToolName: ev.Call.Name, ToolKind: kind, Target: target, Label: tool.DisplayName(ev.Call.Name)}
	if strings.TrimSpace(target) != "" {
		activity.Label += " " + strings.TrimSpace(target)
	}
	return activity
}
func (a Activity) String() string { return strings.TrimSpace(a.Label) }
