package agentui

import (
	"context"
	"errors"
	"strings"

	"github.com/phongsathornpt/protonman/internal/adapter/in/tui/view/toolview"
	"github.com/phongsathornpt/protonman/internal/core/tool"
	"github.com/phongsathornpt/protonman/internal/feature/agent"
)

type ActivityIntent string

const (
	ActivityWaiting    ActivityIntent = "waiting"
	ActivityRoaming    ActivityIntent = "roaming"
	ActivityFarming    ActivityIntent = "farming"
	ActivitySkilling   ActivityIntent = "skilling"
	ActivityGanking    ActivityIntent = "ganking"
	ActivityPushing    ActivityIntent = "pushing"
	ActivityCare       ActivityIntent = "care"
	ActivityRetreating ActivityIntent = "retreating"
	ActivitySticking   ActivityIntent = "sticking"
	ActivityDefending  ActivityIntent = "defending"
	ActivityReady      ActivityIntent = "ready"
)

func (i ActivityIntent) Label() string {
	switch i {
	case ActivityWaiting:
		return "W8"
	case ActivityRoaming:
		return "Roaming"
	case ActivityFarming:
		return "Farming"
	case ActivitySkilling:
		return "Skilling"
	case ActivityGanking:
		return "Ganking"
	case ActivityPushing:
		return "Pushing"
	case ActivityCare:
		return "Care"
	case ActivityRetreating:
		return "B"
	case ActivitySticking:
		return "Sticking"
	case ActivityDefending:
		return "Defending"
	case ActivityReady:
		return "Ready"
	default:
		return ""
	}
}

type Activity struct {
	Intent   ActivityIntent
	ToolName string
	ToolKind tool.Kind
	Target   string
	Label    string
}

func ActivityForState(profile agent.Profile, state agent.State) Activity {
	intent := defaultIntent(profile)
	switch state {
	case agent.StateQueued:
		intent = ActivityWaiting
	case agent.StateCanceling, agent.StateCanceled:
		intent = ActivityRetreating
	case agent.StateFailed, agent.StateInterrupted:
		intent = ActivityCare
	case agent.StateCompleted, agent.StateResumed:
		intent = ActivityReady
	}
	return activity(intent, "", "", "")
}

func ActivityFromEvent(ev agent.Event) Activity {
	switch ev.Kind {
	case agent.EventAgentQueued:
		return activity(ActivityWaiting, "", "", "")
	case agent.EventAgentStarted:
		return activity(defaultIntent(ev.Profile), "", "", "")
	case agent.EventAgentResultAvailable:
		return activity(ActivitySticking, "", "", "")
	case agent.EventAgentFailed:
		if errors.Is(ev.Err, context.Canceled) {
			return activity(ActivityRetreating, "", "", "")
		}
		return activity(ActivityCare, "", "", "")
	case agent.EventAgentCompleted:
		return activity(ActivityReady, "", "", "")
	}
	if ev.Call == nil {
		return activity(defaultIntent(ev.Profile), "", "", "")
	}
	target, kind := toolview.ExtractTarget(ev.Call.Name, "", ev.Call.Arguments)
	return activity(intentForTool(kind, target, ev.Profile), ev.Call.Name, kind, target)
}

func (a Activity) String() string { return strings.TrimSpace(a.Label) }

func activity(intent ActivityIntent, toolName string, kind tool.Kind, target string) Activity {
	label := intent.Label()
	target = strings.TrimSpace(target)
	if target != "" {
		label += " · " + target
	}
	return Activity{Intent: intent, ToolName: toolName, ToolKind: kind, Target: target, Label: label}
}

func defaultIntent(profile agent.Profile) ActivityIntent {
	switch profile {
	case agent.ProfileIntelligence:
		return ActivitySkilling
	case agent.ProfileStrength:
		return ActivityPushing
	default:
		return ActivityRoaming
	}
}

func intentForTool(kind tool.Kind, target string, profile agent.Profile) ActivityIntent {
	switch kind {
	case tool.KindGrep:
		return ActivityGanking
	case tool.KindRead, tool.KindGit, tool.KindWeb, tool.KindMCP:
		return ActivityFarming
	case tool.KindEdit:
		return ActivityPushing
	case tool.KindBash:
		if looksLikeVerification(target) {
			return ActivityDefending
		}
		return ActivityPushing
	case tool.KindCompute:
		return ActivitySkilling
	case tool.KindAgent:
		return ActivitySticking
	default:
		return defaultIntent(profile)
	}
}

func looksLikeVerification(target string) bool {
	command := strings.ToLower(strings.TrimSpace(target))
	for _, marker := range []string{
		"go test", "go vet", "cargo test", "cargo clippy", "pytest", "bun test",
		"npm test", "npm run test", "pnpm test", "yarn test", "make test", "make lint",
		"golangci-lint", "eslint", "oxlint", "tsc ", "tsc --", "typecheck",
	} {
		if strings.Contains(command, marker) {
			return true
		}
	}
	return false
}
